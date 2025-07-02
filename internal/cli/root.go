package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/russellb/canhazgpu/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	config     *types.Config
	configFile string
	rootCmd    = &cobra.Command{
		Use:   "canhazgpu",
		Short: "A GPU reservation tool for single host shared development systems",
		Long: `canhazgpu provides a simple reservation system that coordinates GPU access 
across multiple users and processes on a single machine, ensuring exclusive access 
to requested GPUs while automatically handling cleanup when jobs complete or crash.`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.canhazgpu.yaml)")
	rootCmd.PersistentFlags().String("redis-host", "localhost", "Redis host")
	rootCmd.PersistentFlags().Int("redis-port", 6379, "Redis port")
	rootCmd.PersistentFlags().Int("redis-db", 0, "Redis database")
	rootCmd.PersistentFlags().Int("memory-threshold", types.MemoryThresholdMB, "Memory threshold in MB to consider a GPU as 'in use' (default: 1024)")

	viper.BindPFlag("redis.host", rootCmd.PersistentFlags().Lookup("redis-host"))
	viper.BindPFlag("redis.port", rootCmd.PersistentFlags().Lookup("redis-port"))
	viper.BindPFlag("redis.db", rootCmd.PersistentFlags().Lookup("redis-db"))
	viper.BindPFlag("memory.threshold", rootCmd.PersistentFlags().Lookup("memory-threshold"))

	// Set defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("memory.threshold", types.MemoryThresholdMB)
}

func initConfig() {
	if configFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(configFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not find home directory: %v\n", err)
		} else {
			// Search config in home directory with name ".canhazgpu" (without extension)
			viper.AddConfigPath(home)
			viper.AddConfigPath(".")
			viper.SetConfigType("yaml")
			viper.SetConfigName(".canhazgpu")
		}
	}

	// Enable reading from environment variables
	viper.SetEnvPrefix("CANHAZGPU")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}

	config = &types.Config{
		RedisHost:       viper.GetString("redis.host"),
		RedisPort:       viper.GetInt("redis.port"),
		RedisDB:         viper.GetInt("redis.db"),
		MemoryThreshold: viper.GetInt("memory.threshold"),
	}
}

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func SetVersion(v string) {
	rootCmd.Version = v
}

func getConfig() *types.Config {
	if config == nil {
		initConfig()
	}
	return config
}

func getCurrentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}

func formatError(err error) error {
	return fmt.Errorf("%v", err)
}
