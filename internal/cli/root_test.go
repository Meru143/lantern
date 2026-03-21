package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestResolveLogLevelDefaultsToWarn(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	level, err := resolveLogLevel(false)
	require.NoError(t, err)
	require.Equal(t, zapcore.WarnLevel, level)
}

func TestResolveLogLevelReadsEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "info")

	level, err := resolveLogLevel(false)
	require.NoError(t, err)
	require.Equal(t, zapcore.InfoLevel, level)
}

func TestResolveLogLevelVerboseOverridesEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")

	level, err := resolveLogLevel(true)
	require.NoError(t, err)
	require.Equal(t, zapcore.DebugLevel, level)
}

func TestResolveLogLevelRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "loud")

	_, err := resolveLogLevel(false)
	require.Error(t, err)
}
