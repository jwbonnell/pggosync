package tests

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableExists reports whether a base table with the given schema/name is present on db.
func tableExists(t *testing.T, ctx context.Context, db *pgx.Conn, schema, name string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_type = 'BASE TABLE' AND table_schema = $1 AND table_name = $2
		)`, schema, name).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// TestSchemaSync_CreatesMissingTable creates a throwaway table on the source only, runs
// `schema sync`, and verifies the table is created on the destination. Existing shared tables
// (country/city) trigger non-fatal "already exists" errors and are skipped, leaving the fixture
// intact — the probe table is the only thing this test asserts on.
func TestSchemaSync_CreatesMissingTable(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()

	src, err := pgx.Connect(ctx, "postgres://source_user:source_pw@localhost:5444/postgres")
	require.NoError(t, err)
	defer src.Close(ctx)
	dst, err := pgx.Connect(ctx, "postgres://dest_user:dest_pw@localhost:5445/postgres")
	require.NoError(t, err)
	defer dst.Close(ctx)

	const probe = "schema_sync_probe"
	// Clean slate on both, then create the probe on source only.
	_, err = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	require.NoError(t, err)
	_, err = dst.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	require.NoError(t, err)
	_, err = src.Exec(ctx, "CREATE TABLE "+probe+" (id int PRIMARY KEY, name text)")
	require.NoError(t, err)
	defer func() {
		_, _ = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
		_, _ = dst.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	}()

	require.False(t, tableExists(t, ctx, dst, "public", probe), "precondition: probe must not exist on dest yet")

	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--source", "source", "--dest", "dest", "--skip-confirmation")
	cmd.Execute("test", args)

	assert.True(t, tableExists(t, ctx, dst, "public", probe), "schema sync should have created the probe table on dest")
}

// TestSchemaSync_DryRun verifies a dry run applies nothing to the destination.
func TestSchemaSync_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()

	src, err := pgx.Connect(ctx, "postgres://source_user:source_pw@localhost:5444/postgres")
	require.NoError(t, err)
	defer src.Close(ctx)
	dst, err := pgx.Connect(ctx, "postgres://dest_user:dest_pw@localhost:5445/postgres")
	require.NoError(t, err)
	defer dst.Close(ctx)

	const probe = "schema_sync_probe_dryrun"
	_, err = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	require.NoError(t, err)
	_, err = dst.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	require.NoError(t, err)
	_, err = src.Exec(ctx, "CREATE TABLE "+probe+" (id int PRIMARY KEY)")
	require.NoError(t, err)
	defer func() {
		_, _ = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
		_, _ = dst.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	}()

	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--source", "source", "--dest", "dest", "--dry-run", "--skip-confirmation")
	cmd.Execute("test", args)

	assert.False(t, tableExists(t, ctx, dst, "public", probe), "dry run must not apply the probe table to dest")
}
