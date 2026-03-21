package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Meru143/lantern/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a lantern config file",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	targetPath := cfgFile
	if targetPath == "" {
		targetPath = config.DefaultConfigPath
	}

	expandedPath, err := expandCLIPath(targetPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(expandedPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(expandedPath); err == nil {
		return fmt.Errorf("config file already exists: %s", expandedPath)
	}

	cfg, err := baseCLIConfig()
	if err != nil {
		return err
	}

	content := fmt.Sprintf(`otlp_grpc_port = %d
otlp_http_port = %d
ui_port        = %d
retention      = %q
db_path        = %q

[ui]
theme = %q
`, cfg.OTLPGRPCPort, cfg.OTLPHTTPPort, cfg.UIPort, cfg.Retention, cfg.DBPath, cfg.UI.Theme)

	return os.WriteFile(expandedPath, []byte(content), 0o644)
}
