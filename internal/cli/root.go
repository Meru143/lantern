// Package cli defines the lantern Cobra command tree.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Meru143/lantern/internal/config"
	"github.com/Meru143/lantern/internal/store"
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

	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd, clearCmd)
}

// Execute runs the root lantern command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect lantern configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the active lantern configuration",
	RunE:  runConfigShow,
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Wipe all stored lantern data",
	RunE:  runClear,
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := baseCLIConfig()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), `otlp_grpc_port = %d
otlp_http_port = %d
ui_port        = %d
retention      = %q
db_path        = %q

[ui]
theme = %q
`, cfg.OTLPGRPCPort, cfg.OTLPHTTPPort, cfg.UIPort, cfg.Retention, cfg.DBPath, cfg.UI.Theme)
	return err
}

func runClear(cmd *cobra.Command, args []string) error {
	cfg, err := baseCLIConfig()
	if err != nil {
		return err
	}

	paths := []string{
		cfg.DBPath,
		cfg.DBPath + "-wal",
		cfg.DBPath + "-shm",
		filepath.Join(filepath.Dir(cfg.DBPath), filepath.Base(cfg.DBPath)+".tmp"),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return store.RunMigrations(db)
}
