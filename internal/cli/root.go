// Package cli defines the lantern Cobra command tree.
package cli

import (
	"os"

	"github.com/Meru143/lantern/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	verbose   bool
	appConfig *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "lantern",
	Short: "Local observability platform",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			cobra.CheckErr(err)
		}
		appConfig = cfg
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", config.DefaultConfigPath, "Config file path")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose internal logging")
}

// Execute runs the root lantern command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
