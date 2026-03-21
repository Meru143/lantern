package cli

import (
	snapshotpkg "github.com/Meru143/lantern/internal/snapshot"
	"github.com/spf13/cobra"
)

var (
	replayUIPort int
	replayDBPath string
	replayNoTUI  bool
)

var replayCmd = &cobra.Command{
	Use:   "replay <bundle.lantern>",
	Short: "Import a snapshot bundle and serve it locally",
	Args:  cobra.ExactArgs(1),
	RunE:  runReplay,
}

func init() {
	replayCmd.Flags().IntVar(&replayUIPort, "ui-port", 4000, "Web UI HTTP port")
	replayCmd.Flags().StringVar(&replayDBPath, "db", "~/.lantern/data.db", "SQLite database path")
	replayCmd.Flags().BoolVar(&replayNoTUI, "no-tui", false, "Disable TUI and run headless")

	rootCmd.AddCommand(replayCmd)
}

func runReplay(cmd *cobra.Command, args []string) error {
	cfg, err := baseCLIConfig()
	if err != nil {
		return err
	}

	if cmd.Flags().Changed("ui-port") {
		cfg.UIPort = replayUIPort
	}
	if cmd.Flags().Changed("db") {
		cfg.DBPath = replayDBPath
	}

	cfg.DBPath, err = expandCLIPath(cfg.DBPath)
	if err != nil {
		return err
	}

	if err := snapshotpkg.Import(args[0], cfg.DBPath); err != nil {
		return err
	}

	return runRuntime(cfg, false, !replayNoTUI)
}
