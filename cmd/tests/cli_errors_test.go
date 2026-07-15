package tests

import (
	"os"
	"testing"

	"github.com/jwbonnell/pggosync/cmd"
	"github.com/jwbonnell/pggosync/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These CLI error-path tests exercise validation that returns before any database connection is
// opened, so they need no Docker and run in short mode too. They rely on cmd.Execute returning its
// error (rather than calling os.Exit) so the failure can be asserted. The source/dest connections
// they reference are created in TestMain.

func TestSchemaSync_RequiresSource(t *testing.T) {
	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--dest", "dest")
	err := cmd.Execute("test", args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--source is required")
}

func TestRun_RequiresConfig(t *testing.T) {
	args := os.Args[0:1]
	args = append(args, "run", "--source", "source", "--dest", "dest")
	err := cmd.Execute("test", args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config is required")
}

// TestSchemaSync_SafetyRejectsRemoteDest and TestRun_SafetyRejectsRemoteDest verify the localhost
// safety gate now fires before any connection is opened: a non-loopback destination is refused
// without --no-safety, and no connection to the (unreachable) remote host is attempted.
func TestSchemaSync_SafetyRejectsRemoteDest(t *testing.T) {
	handler := config.UserConfigHandler{PathHandler: config.OSPathHandler{}}
	require.NoError(t, handler.SaveConnection("dest_remote_schema", config.ConnectionConfig{
		Host: "db.example.com", Port: 5432, Database: "postgres", User: "u", Password: "p",
	}))
	defer func() { _ = handler.DeleteConnection("dest_remote_schema") }()

	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--source", "source", "--dest", "dest_remote_schema", "--skip-confirmation")
	err := cmd.Execute("test", args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not localhost")
}

func TestRun_SafetyRejectsRemoteDest(t *testing.T) {
	handler := config.UserConfigHandler{PathHandler: config.OSPathHandler{}}
	require.NoError(t, handler.SaveConnection("dest_remote_run", config.ConnectionConfig{
		Host: "db.example.com", Port: 5432, Database: "postgres", User: "u", Password: "p",
	}))
	defer func() { _ = handler.DeleteConnection("dest_remote_run") }()

	args := os.Args[0:1]
	args = append(args, "run", "--source", "source", "--dest", "dest_remote_run",
		"--config", "../../_configs/configs/default.yml", "--skip-confirmation")
	err := cmd.Execute("test", args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not localhost")
}
