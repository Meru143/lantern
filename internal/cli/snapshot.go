package cli

import (
	"context"
	"time"

	snapshotpkg "github.com/Meru143/lantern/internal/snapshot"
	"github.com/spf13/cobra"
)

var (
	snapshotLast string
	snapshotOut  string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Export a portable .lantern bundle from the local database",
	RunE:  runSnapshot,
}

func init() {
	snapshotCmd.Flags().StringVar(&snapshotLast, "last", "5m", "Duration window to export")
	snapshotCmd.Flags().StringVar(&snapshotOut, "out", "./debug-session.lantern", "Output bundle path")

	rootCmd.AddCommand(snapshotCmd)
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	cfg, err := baseCLIConfig()
	if err != nil {
		return err
	}

	duration, err := time.ParseDuration(snapshotLast)
	if err != nil {
		return err
	}

	return snapshotpkg.Export(context.Background(), cfg.DBPath, duration, snapshotOut)
}
