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

func (c *Client) SetAvailableProvider(ctx context.Context, provider string) error {
	return c.rdb.Set(ctx, types.RedisKeyProvider, provider, 0).Err()
}

func (c *Client) GetAvailableProvider(ctx context.Context) (string, error) {
	val, err := c.rdb.Get(ctx, types.RedisKeyProvider).Result()
	if err == redis.Nil {
		// Check if this is a pre-provider deployment by looking for existing GPU count
		gpuCount, countErr := c.GetGPUCount(ctx)
		if countErr == nil && gpuCount > 0 {
			// This is a pre-provider deployment - auto-migrate to NVIDIA for backward compatibility
			provider := "nvidia"
			if setErr := c.SetAvailableProvider(ctx, provider); setErr != nil {
				return "", fmt.Errorf("failed to auto-migrate pre-provider deployment to NVIDIA: %v", setErr)
			}
			return provider, nil
		}
		return "", fmt.Errorf("GPU provider not initialized - run 'canhazgpu admin --gpus <count>' first")
	}
	if err != nil {
		return "", err
	}

	return val, nil
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

	// MRU-per-user logic for allocating by count
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

		-- Query usage history for this user to get their most recently used GPUs
		local user_gpu_history = {}
		local history_key = "canhazgpu:usage_history_sorted"

		-- Get recent usage records for this user (last 100 records should be plenty)
		local recent_records = redis.call('ZREVRANGE', history_key, 0, 99, 'WITHSCORES')
		for i = 1, #recent_records, 2 do
			local record_json = recent_records[i]
			local timestamp = tonumber(recent_records[i + 1])

			local success, record = pcall(cjson.decode, record_json)
			if success and record and record.user == user and record.gpu_id ~= nil then
				local gpu_id = tonumber(record.gpu_id)
				-- Only keep the most recent timestamp for each GPU
				if not user_gpu_history[gpu_id] or user_gpu_history[gpu_id] < timestamp then
					user_gpu_history[gpu_id] = timestamp
				end
			end
		end

		-- Get available GPUs with MRU-per-user ranking
		local available_gpus = {}
		for i = 0, gpu_count - 1 do
			local key = "canhazgpu:gpu:" .. i
			local gpu_data = redis.call('GET', key)

			-- Skip unreserved GPUs
			if not unreserved_gpus[i] then
				if not gpu_data then
					-- GPU is available (never used)
					table.insert(available_gpus, {
						id = i,
						last_released = 0,
						user_last_used = user_gpu_history[i] or 0
					})
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

						table.insert(available_gpus, {
							id = i,
							last_released = last_released,
							user_last_used = user_gpu_history[i] or 0
						})
					end
				end
			end
		end

		-- Sort by MRU-per-user: prefer GPUs this user used most recently
		-- If user never used a GPU, fall back to global LRU
		table.sort(available_gpus, function(a, b)
			-- If both have user history, prefer more recent
			if a.user_last_used > 0 and b.user_last_used > 0 then
				return a.user_last_used > b.user_last_used
			end
			-- If only one has user history, prefer it
			if a.user_last_used > 0 then
				return true
			end
			if b.user_last_used > 0 then
				return false
			end
			-- Neither has user history, use global LRU (oldest first)
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
					elseif state.type == "run" and state.last_heartbeat and (current_time - tonumber(state.last_heartbeat)) > 300 then
						-- Run reservation heartbeat timeout (5 minutes), continue
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
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	// Write to new sorted set format for efficient range queries
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	score := float64(record.EndTime.ToTime().Unix())

	// Add to sorted set with timestamp as score
	if err := c.rdb.ZAdd(ctx, sortedSetKey, &redis.Z{
		Score:  score,
		Member: string(data),
	}).Err(); err != nil {
		return fmt.Errorf("failed to add to sorted set: %v", err)
	}

	// Set expiration on sorted set (90 days) if not already set
	if err := c.rdb.Expire(ctx, sortedSetKey, 90*24*time.Hour).Err(); err != nil {
		// Log warning but don't fail - expiration might already be set
		fmt.Printf("Warning: failed to set expiration on usage history: %v\n", err)
	}

	return nil
}

// GetUsageHistory retrieves usage history for the specified time range
func (c *Client) GetUsageHistory(ctx context.Context, startTime, endTime time.Time) ([]*types.UsageRecord, error) {
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"

	// Check if new sorted set format exists
	exists, err := c.rdb.Exists(ctx, sortedSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check sorted set existence: %v", err)
	}

	if exists > 0 {
		// Use efficient sorted set range query
		results, err := c.rdb.ZRangeByScore(ctx, sortedSetKey, &redis.ZRangeBy{
			Min: fmt.Sprintf("%d", startTime.Unix()),
			Max: fmt.Sprintf("%d", endTime.Unix()),
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to query sorted set: %v", err)
		}

		var records []*types.UsageRecord
		for _, result := range results {
			var record types.UsageRecord
			if err := json.Unmarshal([]byte(result), &record); err != nil {
				continue
			}
			records = append(records, &record)
		}

		return records, nil
	}

	// TODO: Remove backwards compatibility fallback after migration is complete
	// New format doesn't exist - check old format and migrate
	oldRecords, err := c.getUsageHistoryOldFormat(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve usage history from old format: %v", err)
	}

	// If we found old records, migrate them to new format but leave old data in place
	// TODO: Add cleanup of old data after confident migration is successful
	if len(oldRecords) > 0 {
		if err := c.migrateOldUsageRecords(ctx, oldRecords); err != nil {
			// Log warning but still return the old records
			fmt.Printf("Warning: failed to migrate old usage records: %v\n", err)
		}
	}

	return oldRecords, nil
}

// getUsageHistoryOldFormat retrieves usage history using the old KEYS-based approach
// This function is used for backwards compatibility during migration
func (c *Client) getUsageHistoryOldFormat(ctx context.Context, startTime, endTime time.Time) ([]*types.UsageRecord, error) {
	// Get all usage history keys using the old pattern
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

		// Filter by time range - use <= and >= to be inclusive
		if record.EndTime.ToTime().After(startTime) && record.EndTime.ToTime().Before(endTime.Add(time.Second)) {
			records = append(records, &record)
		}
	}

	return records, nil
}

// migrateOldUsageRecords migrates old format usage records to the new sorted set format
func (c *Client) migrateOldUsageRecords(ctx context.Context, records []*types.UsageRecord) error {
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"

	// Batch add records to sorted set
	var members []*redis.Z
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			continue
		}

		members = append(members, &redis.Z{
			Score:  float64(record.EndTime.ToTime().Unix()),
			Member: string(data),
		})
	}

	if len(members) > 0 {
		if err := c.rdb.ZAdd(ctx, sortedSetKey, members...).Err(); err != nil {
			return fmt.Errorf("failed to migrate records to sorted set: %v", err)
		}

		// Set expiration on sorted set (90 days)
		if err := c.rdb.Expire(ctx, sortedSetKey, 90*24*time.Hour).Err(); err != nil {
			// Log warning but don't fail
			fmt.Printf("Warning: failed to set expiration on usage history: %v\n", err)
		}
	}

	return nil
}
