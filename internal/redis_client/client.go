package redis_client

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/russellb/canhazgpu/internal/types"
)

type Client struct {
	rdb *redis.Client
}

func NewClient(config *types.Config) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", config.RedisHost, config.RedisPort),
		DB:   config.RedisDB,
	})

	return &Client{rdb: rdb}
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// GPU State Management

func (c *Client) SetGPUCount(ctx context.Context, count int) error {
	return c.rdb.Set(ctx, types.RedisKeyGPUCount, count, 0).Err()
}

func (c *Client) GetGPUCount(ctx context.Context) (int, error) {
	val, err := c.rdb.Get(ctx, types.RedisKeyGPUCount).Int()
	if err == redis.Nil {
		return 0, fmt.Errorf("GPU pool not initialized - run 'canhazgpu admin --gpus <count>' first")
	}
	return val, err
}

func (c *Client) GetGPUState(ctx context.Context, gpuID int) (*types.GPUState, error) {
	key := fmt.Sprintf("%sgpu:%d", types.RedisKeyPrefix, gpuID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		// GPU is available
		return &types.GPUState{}, nil
	}
	if err != nil {
		return nil, err
	}

	var state types.GPUState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, fmt.Errorf("corrupted GPU state for GPU %d: %v", gpuID, err)
	}

	return &state, nil
}

func (c *Client) SetGPUState(ctx context.Context, gpuID int, state *types.GPUState) error {
	key := fmt.Sprintf("%sgpu:%d", types.RedisKeyPrefix, gpuID)

	if state.User == "" {
		// GPU is available, just store last_released timestamp if it exists
		if !state.LastReleased.ToTime().IsZero() {
			availableState := types.GPUState{LastReleased: state.LastReleased}
			data, err := json.Marshal(availableState)
			if err != nil {
				return err
			}
			return c.rdb.Set(ctx, key, data, 0).Err()
		}
		// Delete the key if no useful state
		return c.rdb.Del(ctx, key).Err()
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return c.rdb.Set(ctx, key, data, 0).Err()
}

func (c *Client) DeleteGPUState(ctx context.Context, gpuID int) error {
	key := fmt.Sprintf("%sgpu:%d", types.RedisKeyPrefix, gpuID)
	return c.rdb.Del(ctx, key).Err()
}

// Allocation Lock Management

func (c *Client) AcquireAllocationLock(ctx context.Context) error {
	for attempt := 0; attempt < types.MaxLockRetries; attempt++ {
		acquired, err := c.rdb.SetNX(ctx, types.RedisKeyAllocationLock, "locked", types.LockTimeout).Result()
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}

		// Exponential backoff with jitter
		sleepTime := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Intn(1000))*time.Millisecond
		time.Sleep(sleepTime)
	}

	return fmt.Errorf("failed to acquire allocation lock after %d attempts", types.MaxLockRetries)
}

func (c *Client) ReleaseAllocationLock(ctx context.Context) error {
	return c.rdb.Del(ctx, types.RedisKeyAllocationLock).Err()
}

// Atomic GPU Allocation using Lua script
func (c *Client) AtomicReserveGPUs(ctx context.Context, request *types.AllocationRequest, unreservedGPUs []int) ([]int, error) {
	// Check if specific GPU IDs are requested
	if len(request.GPUIDs) > 0 {
		return c.atomicReserveSpecificGPUs(ctx, request, unreservedGPUs)
	}

	// Original logic for allocating by count
	luaScript := `
		local gpu_count = tonumber(ARGV[1])
		local requested = tonumber(ARGV[2])
		local user = ARGV[3]
		local reservation_type = ARGV[4]
		local current_time = tonumber(ARGV[5])
		local expiry_time = ARGV[6]
		local unreserved_gpus_json = ARGV[7]
		
		-- Parse unreserved GPUs
		local unreserved_gpus = {}
		if unreserved_gpus_json and unreserved_gpus_json ~= "" and unreserved_gpus_json ~= "[]" and unreserved_gpus_json ~= "null" then
			local success, unreserved_list = pcall(cjson.decode, unreserved_gpus_json)
			if success and unreserved_list and type(unreserved_list) == "table" then
				for _, gpu_id in ipairs(unreserved_list) do
					unreserved_gpus[tonumber(gpu_id)] = true
				end
			end
		end
		
		-- Get available GPUs with LRU ranking
		local available_gpus = {}
		for i = 0, gpu_count - 1 do
			local key = "canhazgpu:gpu:" .. i
			local gpu_data = redis.call('GET', key)
			
			-- Skip unreserved GPUs
			if not unreserved_gpus[i] then
				if not gpu_data then
					-- GPU is available (never used)
					table.insert(available_gpus, {id = i, last_released = 0})
				else
					local state = cjson.decode(gpu_data)
					if not state.user then
						-- GPU is available
						local last_released = 0
						
						-- Parse last_released timestamp
						if state.last_released and state.last_released ~= "" then
							-- RFC3339 format: extract Unix timestamp
							-- Try to convert RFC3339 to seconds since epoch
							-- For simplicity, we'll use the current_time as a reference
							-- and assign a large value to indicate it was previously used
							last_released = current_time - 86400 -- Default to 24 hours ago
							
							-- Better approach: extract year, month, day, hour, minute, second from RFC3339
							-- Format: 2025-06-30T16:34:38.372177993Z
							local year, month, day, hour, min, sec = string.match(state.last_released, 
								"(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)")
							
							if year then
								-- Convert to Unix timestamp (approximate)
								-- This is a simplified conversion that works for recent dates
								local days_since_epoch = (tonumber(year) - 1970) * 365 + 
									(tonumber(month) - 1) * 30 + 
									tonumber(day)
								last_released = days_since_epoch * 86400 + 
									tonumber(hour) * 3600 + 
									tonumber(min) * 60 + 
									tonumber(sec)
							end
						end
						
						table.insert(available_gpus, {id = i, last_released = last_released})
					end
				end
			end
		end
		
		-- Sort by last_released (oldest first)
		table.sort(available_gpus, function(a, b) 
			return a.last_released < b.last_released 
		end)
		
		-- Check if we have enough GPUs
		if #available_gpus < requested then
			return {error = "Not enough GPUs available"}
		end
		
		-- Allocate requested GPUs
		local allocated = {}
		for i = 1, requested do
			local gpu_id = available_gpus[i].id
			table.insert(allocated, gpu_id)
			
			-- Create reservation state
			local state = {
				user = user,
				start_time = current_time,
				type = reservation_type
			}
			
			if reservation_type == "run" then
				state.last_heartbeat = current_time
			elseif reservation_type == "manual" and expiry_time ~= "nil" then
				state.expiry_time = tonumber(expiry_time)
			end
			
			-- Set GPU state
			local key = "canhazgpu:gpu:" .. gpu_id
			redis.call('SET', key, cjson.encode(state))
		end
		
		return allocated
	`

	// Convert unreserved GPUs to JSON
	unreservedJSON, err := json.Marshal(unreservedGPUs)
	if err != nil {
		return nil, err
	}

	// Get GPU count
	gpuCount, err := c.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	// Prepare arguments
	currentTime := time.Now().Unix()
	expiryTime := "nil"
	if request.ExpiryTime != nil {
		expiryTime = fmt.Sprintf("%d", request.ExpiryTime.Unix())
	}

	// Execute Lua script
	result, err := c.rdb.Eval(ctx, luaScript, []string{},
		gpuCount,
		request.GPUCount,
		request.User,
		request.ReservationType,
		currentTime,
		expiryTime,
		string(unreservedJSON),
	).Result()

	if err != nil {
		return nil, err
	}

	// Parse result
	switch v := result.(type) {
	case []interface{}:
		// Check if first element is an error map
		if len(v) > 0 {
			if errMap, ok := v[0].(map[string]interface{}); ok {
				if errorMsg, hasError := errMap["error"]; hasError {
					return nil, fmt.Errorf("%v", errorMsg)
				}
			}
		}

		// Parse allocated GPU IDs
		var allocated []int
		for _, item := range v {
			if gpuID, ok := item.(int64); ok {
				allocated = append(allocated, int(gpuID))
			}
		}
		return allocated, nil
	case map[string]interface{}:
		// Handle error result directly as a map
		if errorMsg, hasError := v["error"]; hasError {
			return nil, fmt.Errorf("%v", errorMsg)
		}
		return nil, fmt.Errorf("unexpected map result from Lua script: %v", v)
	default:
		return nil, fmt.Errorf("unexpected result type from Lua script: %T", result)
	}
}

// atomicReserveSpecificGPUs reserves specific GPU IDs if they are available
func (c *Client) atomicReserveSpecificGPUs(ctx context.Context, request *types.AllocationRequest, unreservedGPUs []int) ([]int, error) {
	luaScript := `
		local requested_gpus_json = ARGV[1]
		local user = ARGV[2]
		local reservation_type = ARGV[3]
		local current_time = tonumber(ARGV[4])
		local expiry_time = ARGV[5]
		local unreserved_gpus_json = ARGV[6]
		local gpu_count = tonumber(ARGV[7])
		
		-- Parse requested GPU IDs
		local requested_gpus = {}
		if requested_gpus_json and requested_gpus_json ~= "" and requested_gpus_json ~= "[]" and requested_gpus_json ~= "null" then
			local success, gpu_list = pcall(cjson.decode, requested_gpus_json)
			if not success or not gpu_list or type(gpu_list) ~= "table" then
				return redis.error_reply("Invalid GPU IDs format")
			end
			requested_gpus = gpu_list
		else
			return redis.error_reply("No GPU IDs specified")
		end
		
		-- Parse unreserved GPUs (GPUs in use without reservation)
		local unreserved_gpus = {}
		if unreserved_gpus_json and unreserved_gpus_json ~= "" and unreserved_gpus_json ~= "[]" and unreserved_gpus_json ~= "null" then
			local success, unreserved_list = pcall(cjson.decode, unreserved_gpus_json)
			if success and unreserved_list and type(unreserved_list) == "table" then
				for _, gpu_id in ipairs(unreserved_list) do
					unreserved_gpus[tonumber(gpu_id)] = true
				end
			end
		end
		
		-- Validate all requested GPUs
		for _, gpu_id in ipairs(requested_gpus) do
			local gpu_id_num = tonumber(gpu_id)
			
			-- Check if GPU ID is valid (within range)
			if gpu_id_num < 0 or gpu_id_num >= gpu_count then
				return redis.error_reply("GPU ID " .. gpu_id .. " is out of range (0-" .. (gpu_count-1) .. ")")
			end
			
			-- Check if GPU is unreserved (in use without reservation)
			if unreserved_gpus[gpu_id_num] then
				return redis.error_reply("GPU " .. gpu_id .. " is in use without reservation")
			end
			
			-- Check if GPU is already reserved
			local key = "canhazgpu:gpu:" .. gpu_id
			local gpu_data = redis.call('GET', key)
			
			if gpu_data then
				local state = cjson.decode(gpu_data)
				if state.user then
					-- GPU is already reserved
					if state.type == "manual" and state.expiry_time and tonumber(state.expiry_time) < current_time then
						-- Manual reservation has expired, continue
					elseif state.type == "run" and state.last_heartbeat and (current_time - tonumber(state.last_heartbeat)) > 900 then
						-- Run reservation heartbeat timeout (15 minutes), continue
					else
						-- GPU is actively reserved
						return redis.error_reply("GPU " .. gpu_id .. " is already reserved by user '" .. state.user .. "'")
					end
				end
			end
		end
		
		-- All GPUs are available, reserve them
		local allocated = {}
		for _, gpu_id in ipairs(requested_gpus) do
			local gpu_id_num = tonumber(gpu_id)
			table.insert(allocated, gpu_id_num)
			
			-- Create reservation state
			local state = {
				user = user,
				start_time = current_time,
				type = reservation_type
			}
			
			if reservation_type == "run" then
				state.last_heartbeat = current_time
			elseif reservation_type == "manual" and expiry_time ~= "nil" then
				state.expiry_time = tonumber(expiry_time)
			end
			
			-- Set GPU state
			local key = "canhazgpu:gpu:" .. gpu_id
			redis.call('SET', key, cjson.encode(state))
		end
		
		return allocated
	`

	// Convert requested GPU IDs to JSON
	requestedGPUsJSON, err := json.Marshal(request.GPUIDs)
	if err != nil {
		return nil, err
	}

	// Convert unreserved GPUs to JSON
	unreservedJSON, err := json.Marshal(unreservedGPUs)
	if err != nil {
		return nil, err
	}

	// Get GPU count for validation
	gpuCount, err := c.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	// Prepare arguments
	currentTime := time.Now().Unix()
	expiryTime := "nil"
	if request.ExpiryTime != nil {
		expiryTime = fmt.Sprintf("%d", request.ExpiryTime.Unix())
	}

	// Execute Lua script
	result, err := c.rdb.Eval(ctx, luaScript, []string{},
		string(requestedGPUsJSON),
		request.User,
		request.ReservationType,
		currentTime,
		expiryTime,
		string(unreservedJSON),
		gpuCount,
	).Result()

	if err != nil {
		return nil, err
	}

	// Parse result
	switch v := result.(type) {
	case []interface{}:
		// Check if first element is an error map
		if len(v) > 0 {
			if errorMap, ok := v[0].(map[string]interface{}); ok {
				if errorMsg, hasError := errorMap["error"]; hasError {
					return nil, fmt.Errorf("%v", errorMsg)
				}
			}
		}

		// Convert to int slice
		var allocatedGPUs []int
		for _, id := range v {
			if gpuID, ok := id.(int64); ok {
				allocatedGPUs = append(allocatedGPUs, int(gpuID))
			}
		}
		return allocatedGPUs, nil
	case map[string]interface{}:
		// Handle error result directly as a map
		if errorMsg, hasError := v["error"]; hasError {
			return nil, fmt.Errorf("%v", errorMsg)
		}
		return nil, fmt.Errorf("unexpected map result from Lua script: %v", v)
	default:
		return nil, fmt.Errorf("unexpected result type from Lua script: %T", result)
	}
}

// Clear all GPU states (for admin --force)
func (c *Client) ClearAllGPUStates(ctx context.Context) error {
	// Get all GPU keys
	keys, err := c.rdb.Keys(ctx, types.RedisKeyPrefix+"gpu:*").Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return c.rdb.Del(ctx, keys...).Err()
	}

	return nil
}

// RecordUsageHistory records a GPU usage entry when a reservation is released
func (c *Client) RecordUsageHistory(ctx context.Context, record *types.UsageRecord) error {
	// Create a unique key based on timestamp and user
	key := fmt.Sprintf("%s%d:%s:%d", types.RedisKeyUsageHistory,
		record.EndTime.ToTime().Unix(), record.User, record.GPUID)

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	// Store with 90 day expiration to prevent unbounded growth
	return c.rdb.Set(ctx, key, data, 90*24*time.Hour).Err()
}

// GetUsageHistory retrieves usage history for the specified time range
func (c *Client) GetUsageHistory(ctx context.Context, startTime, endTime time.Time) ([]*types.UsageRecord, error) {
	// Get all usage history keys
	pattern := types.RedisKeyUsageHistory + "*"
	keys, err := c.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	var records []*types.UsageRecord
	for _, key := range keys {
		data, err := c.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var record types.UsageRecord
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			continue
		}

		// Filter by time range
		if record.EndTime.ToTime().After(startTime) && record.EndTime.ToTime().Before(endTime) {
			records = append(records, &record)
		}
	}

	return records, nil
}
