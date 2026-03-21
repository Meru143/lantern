package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServeCommandRegistersExpectedFlags(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"serve"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "serve", command.Name())

	expectedFlags := []string{"otlp-grpc", "otlp-http", "ui-port", "retention", "db", "no-tui"}
	for _, flagName := range expectedFlags {
		flag := command.Flags().Lookup(flagName)
		require.NotNilf(t, flag, "missing flag %s", flagName)
	}
}

func TestTUICommandExists(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"tui"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "tui", command.Name())
}
