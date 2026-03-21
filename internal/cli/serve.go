package cli

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Meru143/lantern/internal/api"
	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/config"
	"github.com/Meru143/lantern/internal/receiver"
	"github.com/Meru143/lantern/internal/store"
	"github.com/Meru143/lantern/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	serveOTLPGRPC  int
	serveOTLPHTTP  int
	serveUIPort    int
	serveRetention string
	serveDBPath    string
	serveNoTUI     bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OTLP receiver, web UI, and optional TUI",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntVar(&serveOTLPGRPC, "otlp-grpc", 4317, "OTLP gRPC port")
	serveCmd.Flags().IntVar(&serveOTLPHTTP, "otlp-http", 4318, "OTLP HTTP port")
	serveCmd.Flags().IntVar(&serveUIPort, "ui-port", 4000, "Web UI HTTP port")
	serveCmd.Flags().StringVar(&serveRetention, "retention", "24h", "Data retention window")
	serveCmd.Flags().StringVar(&serveDBPath, "db", "~/.lantern/data.db", "SQLite database path")
	serveCmd.Flags().BoolVar(&serveNoTUI, "no-tui", false, "Disable TUI and run headless")

	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := mergedServeConfig(cmd)
	if err != nil {
		return err
	}

	return runRuntime(cfg, true, !serveNoTUI)
}

func runRuntime(cfg *config.Config, withReceiver bool, enableTUI bool) error {
	db, err := openRuntimeDB(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	eventBus := bus.NewEventBus()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if withReceiver {
		consumer := receiver.NewSignalConsumer(db, eventBus)
		if err := receiver.Start(ctx, cfg, consumer); err != nil {
			return err
		}
		defer func() {
			_ = receiver.Shutdown(context.Background())
		}()
	}

	if err := api.Start(ctx, db, eventBus, cfg.UIPort); err != nil {
		return err
	}
	defer func() {
		_ = api.Shutdown(context.Background())
	}()

	retention, err := store.ParseRetention(cfg.Retention)
	if err != nil {
		return err
	}

	cleanupStop := make(chan struct{})
	defer close(cleanupStop)
	go runCleanupLoop(ctx, cleanupStop, db, retention)

	var program *tea.Program
	if enableTUI {
		program = tea.NewProgram(tui.NewModel(db, eventBus), tea.WithAltScreen())
		go func() {
			_, _ = program.Run()
		}()
		defer program.Send(tea.Quit())
	}

	<-ctx.Done()

	var shutdownErr error
	if program != nil {
		program.Send(tea.Quit())
	}
	if withReceiver {
		if err := receiver.Shutdown(context.Background()); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if err := api.Shutdown(context.Background()); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}

	return shutdownErr
}

func mergedServeConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := baseCLIConfig()
	if err != nil {
		return nil, err
	}

	if cmd.Flags().Changed("otlp-grpc") {
		cfg.OTLPGRPCPort = serveOTLPGRPC
	}
	if cmd.Flags().Changed("otlp-http") {
		cfg.OTLPHTTPPort = serveOTLPHTTP
	}
	if cmd.Flags().Changed("ui-port") {
		cfg.UIPort = serveUIPort
	}
	if cmd.Flags().Changed("retention") {
		cfg.Retention = serveRetention
	}
	if cmd.Flags().Changed("db") {
		cfg.DBPath = serveDBPath
	}

	expandedDBPath, err := expandCLIPath(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	cfg.DBPath = expandedDBPath

	return cfg, nil
}

func baseCLIConfig() (*config.Config, error) {
	cfg := &config.Config{}
	if appConfig != nil {
		*cfg = *appConfig
	} else {
		cfg = &config.Config{
			OTLPGRPCPort: 4317,
			OTLPHTTPPort: 4318,
			UIPort:       4000,
			Retention:    "24h",
			DBPath:       "~/.lantern/data.db",
		}
	}

	expandedDBPath, err := expandCLIPath(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	cfg.DBPath = expandedDBPath

	return cfg, nil
}

func openRuntimeDB(dbPath string) (*sql.DB, error) {
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := store.RunMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runCleanupLoop(ctx context.Context, stop <-chan struct{}, db *sql.DB, retention time.Duration) {
	_ = store.RunCleanup(ctx, db, retention)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			_ = store.RunCleanup(ctx, db, retention)
		}
	}
}

func expandCLIPath(path string) (string, error) {
	if path == "" || path == "~" {
		if path == "" {
			return "", nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}

	if !strings.HasPrefix(path, "~"+string(os.PathSeparator)) && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimPrefix(path, "~")
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "\\")
	return filepath.Join(home, trimmed), nil
}
