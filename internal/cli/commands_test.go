package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotCommandRegistersExpectedFlags(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"snapshot"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "snapshot", command.Name())
	require.NotNil(t, command.Flags().Lookup("last"))
	require.NotNil(t, command.Flags().Lookup("out"))
}

func TestReplayCommandExists(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"replay"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "replay", command.Name())
}

func TestInitCommandExists(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"init"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "init", command.Name())
}

func TestClearCommandExists(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"clear"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "clear", command.Name())
}

func TestConfigShowCommandExists(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"config", "show"})
	require.NoError(t, err)
	require.NotNil(t, command)
	require.Equal(t, "show", command.Name())
}
