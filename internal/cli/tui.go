package cli

import (
	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the fullscreen terminal UI",
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := mergedServeConfig(serveCmd)
	if err != nil {
		return err
	}

	db, err := openRuntimeDB(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	program := tea.NewProgram(tui.NewModel(db, bus.NewEventBus()), tea.WithAltScreen())
	_, err = program.Run()
	return err
}
