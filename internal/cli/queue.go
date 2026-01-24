package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show the GPU reservation queue",
	Long: `Display the current GPU reservation queue.

When GPUs are not immediately available, 'run' and 'reserve' commands
will add entries to a queue and wait for resources to become available.
This command shows all entries currently waiting in the queue.

The queue operates on a First Come First Served (FCFS) basis. Only the
first entry in the queue can acquire newly available GPUs. As GPUs become
available, they are allocated to the first entry (greedy partial allocation)
until all requested GPUs are allocated.

Example usage:
  canhazgpu queue
  canhazgpu queue --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOutput := viper.GetBool("queue.json")
		return runQueue(cmd.Context(), jsonOutput)
	},
}

func init() {
	queueCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(queueCmd)
}

func runQueue(ctx context.Context, jsonOutput bool) error {
	config := getConfig()
	client := redis_client.NewClient(config)
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Printf("Warning: failed to close Redis client: %v\n", err)
		}
	}()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine
	engine := gpu.NewAllocationEngine(client, config)

	// Get queue status
	status, err := engine.GetQueueStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue status: %v", err)
	}

	if jsonOutput {
		return printQueueJSON(status)
	}

	return printQueueTable(status)
}

func printQueueJSON(status *types.QueueStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal queue status: %v", err)
	}
	fmt.Println(string(data))
	return nil
}

func printQueueTable(status *types.QueueStatus) error {
	if status.TotalWaiting == 0 {
		fmt.Println("GPU Reservation Queue")
		fmt.Println("=====================")
		fmt.Println("No entries waiting in queue.")
		return nil
	}

	fmt.Println("GPU Reservation Queue")
	fmt.Println("=====================")
	fmt.Println()

	// Print header
	fmt.Printf("%-10s %-15s %-15s %-12s %s\n",
		"Position", "User", "Requested", "Allocated", "Waiting")
	fmt.Printf("%-10s %-15s %-15s %-12s %s\n",
		"--------", "----", "---------", "---------", "-------")

	// Print entries
	now := time.Now()
	for i, entry := range status.Entries {
		waitTime := now.Sub(entry.EnqueueTime.ToTime())
		requested := fmt.Sprintf("%d GPUs", entry.GetRequestedGPUCount())
		if len(entry.RequestedIDs) > 0 {
			requested = fmt.Sprintf("IDs: %v", entry.RequestedIDs)
		}
		allocated := fmt.Sprintf("%d/%d", len(entry.AllocatedGPUs), entry.GetRequestedGPUCount())

		fmt.Printf("%-10d %-15s %-15s %-12s %s\n",
			i+1,
			truncateString(entry.User, 15),
			truncateString(requested, 15),
			allocated,
			utils.FormatDuration(waitTime))
	}

	fmt.Println()
	fmt.Printf("Total: %d entries waiting for %d GPUs (%d partially allocated)\n",
		status.TotalWaiting,
		status.TotalGPUsRequested-status.TotalGPUsAllocated,
		status.TotalGPUsAllocated)

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
