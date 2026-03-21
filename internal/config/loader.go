// Package config loads lantern configuration from files, environment variables, and defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const DefaultConfigPath = "~/.lantern/config.toml"

// UIConfig stores user interface preferences from config files.
type UIConfig struct {
	Theme string `mapstructure:"theme"`
}

// Config stores lantern runtime configuration.
type Config struct {
	OTLPGRPCPort int      `mapstructure:"otlp_grpc_port"`
	OTLPHTTPPort int      `mapstructure:"otlp_http_port"`
	UIPort       int      `mapstructure:"ui_port"`
	Retention    string   `mapstructure:"retention"`
	DBPath       string   `mapstructure:"db_path"`
	UI           UIConfig `mapstructure:"ui"`
}

// Load reads lantern configuration from defaults, file, and environment variables.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")
	v.SetDefault("otlp_grpc_port", 4317)
	v.SetDefault("otlp_http_port", 4318)
	v.SetDefault("ui_port", 4000)
	v.SetDefault("retention", "24h")
	v.SetDefault("db_path", "~/.lantern/data.db")
	v.SetDefault("ui.theme", "dark")

	if err := v.BindEnv("otlp_grpc_port", "LANTERN_OTLP_GRPC_PORT"); err != nil {
		return nil, fmt.Errorf("bind otlp_grpc_port env: %w", err)
	}
	if err := v.BindEnv("otlp_http_port", "LANTERN_OTLP_HTTP_PORT"); err != nil {
		return nil, fmt.Errorf("bind otlp_http_port env: %w", err)
	}
	if err := v.BindEnv("ui_port", "LANTERN_UI_PORT"); err != nil {
		return nil, fmt.Errorf("bind ui_port env: %w", err)
	}
	if err := v.BindEnv("retention", "LANTERN_RETENTION"); err != nil {
		return nil, fmt.Errorf("bind retention env: %w", err)
	}
	if err := v.BindEnv("db_path", "LANTERN_DB_PATH"); err != nil {
		return nil, fmt.Errorf("bind db_path env: %w", err)
	}
	v.AutomaticEnv()

	configPath := cfgFile
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	expandedConfigPath, err := expandTilde(configPath)
	if err != nil {
		return nil, fmt.Errorf("expand config path: %w", err)
	}
	v.SetConfigFile(expandedConfigPath)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config file %s: %w", expandedConfigPath, err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	cfg.DBPath, err = expandTilde(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("expand db path: %w", err)
	}

	return cfg, nil
}

func expandTilde(path string) (string, error) {
	if path == "" || path == "~" {
		if path == "" {
			return "", nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("lookup user home directory: %w", err)
		}
		return home, nil
	}

	if !strings.HasPrefix(path, "~"+string(os.PathSeparator)) && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("lookup user home directory: %w", err)
	}

	trimmed := strings.TrimPrefix(path, "~")
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "\\")
	return filepath.Join(home, trimmed), nil
}
