package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the basic structure and flags of all commands
func TestCommands_Structure(t *testing.T) {
	tests := []struct {
		name          string
		cmd           interface{}
		use           string
		shortContains string
		requiredFlags []string
		optionalFlags []string
	}{
		{
			name:          "admin command",
			cmd:           adminCmd,
			use:           "admin",
			shortContains: "Initialize GPU pool",
			requiredFlags: []string{"gpus"},
			optionalFlags: []string{"force"},
		},
		{
			name:          "status command",
			cmd:           statusCmd,
			use:           "status",
			shortContains: "Show current GPU allocation status",
			requiredFlags: []string{},
			optionalFlags: []string{},
		},
		{
			name:          "run command",
			cmd:           runCmd,
			use:           "run",
			shortContains: "Reserve GPUs and run a command",
			requiredFlags: []string{"gpus"},
			optionalFlags: []string{},
		},
		{
			name:          "reserve command",
			cmd:           reserveCmd,
			use:           "reserve",
			shortContains: "Reserve GPUs manually",
			requiredFlags: []string{},
			optionalFlags: []string{"gpus", "duration"},
		},
		{
			name:          "release command",
			cmd:           releaseCmd,
			use:           "release",
			shortContains: "Release manually reserved GPUs",
			requiredFlags: []string{},
			optionalFlags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Type assertion to get the actual command
			var cmd *cobra.Command
			switch c := tt.cmd.(type) {
			case *cobra.Command:
				cmd = c
			default:
				t.Fatalf("Expected *cobra.Command, got %T", tt.cmd)
			}

			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.shortContains)
			assert.NotNil(t, cmd.RunE)

			// Check required flags
			for _, flagName := range tt.requiredFlags {
				flag := cmd.Flags().Lookup(flagName)
				require.NotNil(t, flag, "Required flag %s should exist", flagName)
			}

			// Check optional flags exist (if specified)
			for _, flagName := range tt.optionalFlags {
				flag := cmd.Flags().Lookup(flagName)
				require.NotNil(t, flag, "Optional flag %s should exist", flagName)
			}
		})
	}
}

func TestRootCommand_Structure(t *testing.T) {
	cmd := rootCmd

	assert.Equal(t, "canhazgpu", cmd.Use)
	assert.Contains(t, cmd.Short, "GPU reservation tool")

	// Check global flags
	redisHostFlag := cmd.PersistentFlags().Lookup("redis-host")
	require.NotNil(t, redisHostFlag)
	assert.Equal(t, "string", redisHostFlag.Value.Type())

	redisPortFlag := cmd.PersistentFlags().Lookup("redis-port")
	require.NotNil(t, redisPortFlag)
	assert.Equal(t, "int", redisPortFlag.Value.Type())

	redisDBFlag := cmd.PersistentFlags().Lookup("redis-db")
	require.NotNil(t, redisDBFlag)
	assert.Equal(t, "int", redisDBFlag.Value.Type())
}

func TestCommands_NoCompletion(t *testing.T) {
	// Verify that completion command is disabled
	assert.True(t, rootCmd.CompletionOptions.DisableDefaultCmd)
}

func TestCommands_HasSubcommands(t *testing.T) {
	// Verify all expected subcommands are present
	expectedCommands := []string{"admin", "status", "run", "reserve", "release"}

	actualCommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		actualCommands[cmd.Use] = true
	}

	for _, expected := range expectedCommands {
		assert.True(t, actualCommands[expected], "Command %s should be available", expected)
	}

	// Verify completion command is NOT present
	assert.False(t, actualCommands["completion"], "Completion command should be disabled")
}
