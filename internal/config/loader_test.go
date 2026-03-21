package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadReturnsDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	require.Equal(t, 4317, cfg.OTLPGRPCPort)
	require.Equal(t, 4318, cfg.OTLPHTTPPort)
	require.Equal(t, 4000, cfg.UIPort)
	require.Equal(t, "24h", cfg.Retention)
	require.NotEmpty(t, cfg.DBPath)
}

func TestLoadEnvOverridesDefault(t *testing.T) {
	t.Setenv("LANTERN_UI_PORT", "9000")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	require.Equal(t, 9000, cfg.UIPort)
}

func TestLoadExpandsTildeInDBPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOME", homeDir)

	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`db_path = "~/.lantern/custom.db"`), 0o644))

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(homeDir, ".lantern", "custom.db"), cfg.DBPath)
}
