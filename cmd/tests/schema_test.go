package tests

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/cmd"
	"github.com/jwbonnell/pggosync/config"
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

// columnExists reports whether the given column is present on schema.table in db.
func columnExists(t *testing.T, ctx context.Context, db *pgx.Conn, schema, table, column string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`, schema, table, column).Scan(&exists)
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

	// A sentinel row in an existing dest table — the default (no --clean) path must leave existing
	// objects and their data untouched, only creating what's missing.
	_, err = dst.Exec(ctx, "INSERT INTO country (country_id, country_name) VALUES (9911, 'schema sentinel') ON CONFLICT (country_id) DO UPDATE SET country_name = EXCLUDED.country_name")
	require.NoError(t, err)
	defer func() { _, _ = dst.Exec(ctx, "DELETE FROM country WHERE country_id = 9911") }()

	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--source", "source", "--dest", "dest", "--skip-confirmation")
	cmd.Execute("test", args)

	assert.True(t, tableExists(t, ctx, dst, "public", probe), "schema sync should have created the probe table on dest")

	// The existing table's data must survive — the default path does not drop/recreate.
	var name string
	err = dst.QueryRow(ctx, "SELECT country_name FROM country WHERE country_id = 9911").Scan(&name)
	require.NoError(t, err, "existing dest data must remain after a default schema sync")
	assert.Equal(t, "schema sentinel", name)
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

// TestSchemaSync_Clean exercises the destructive --clean path (drop & recreate every object so the
// destination schema matches source exactly). Because --clean operates on the whole database and
// wipes data, it runs against a **throwaway database** created just for this test — never the shared
// dest fixture. It verifies both --clean behaviours on a probe table: a drifted table (missing a
// column source has) is brought back in line, and its existing rows are wiped by the recreate.
func TestSchemaSync_Clean(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	const probe = "clean_probe"
	const throwaway = "schema_clean_test"

	// Source: a probe table WITH an `extra` column. The whole-DB dump carries it to the throwaway
	// dest; the drifted copy there lacks it.
	src, err := pgx.Connect(ctx, "postgres://source_user:source_pw@localhost:5444/postgres")
	require.NoError(t, err)
	defer src.Close(ctx)
	_, err = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe)
	require.NoError(t, err)
	_, err = src.Exec(ctx, "CREATE TABLE "+probe+" (id int PRIMARY KEY, name text, extra text)")
	require.NoError(t, err)
	defer func() { _, _ = src.Exec(ctx, "DROP TABLE IF EXISTS "+probe) }()

	// Admin connection to the dest server's maintenance DB to create/drop the throwaway database.
	admin, err := pgx.Connect(ctx, "postgres://dest_user:dest_pw@localhost:5445/postgres")
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+throwaway+" WITH (FORCE)")
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "CREATE DATABASE "+throwaway)
	require.NoError(t, err)
	defer func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+throwaway+" WITH (FORCE)") }()

	// Seed the throwaway dest with a DRIFTED probe (missing `extra`) holding a stale row.
	twURL := "postgres://dest_user:dest_pw@localhost:5445/" + throwaway
	tw, err := pgx.Connect(ctx, twURL)
	require.NoError(t, err)
	_, err = tw.Exec(ctx, "CREATE TABLE "+probe+" (id int PRIMARY KEY, name text)")
	require.NoError(t, err)
	_, err = tw.Exec(ctx, "INSERT INTO "+probe+" (id, name) VALUES (1, 'stale')")
	require.NoError(t, err)
	require.NoError(t, tw.Close(ctx)) // close before restore/reconnect so DROP DATABASE FORCE isn't needed for us

	// Register a temporary dest connection pointing at the throwaway DB, then run --clean.
	handler := config.UserConfigHandler{PathHandler: config.OSPathHandler{}}
	require.NoError(t, handler.SaveConnection("dest_cleantest", config.ConnectionConfig{
		Host: "localhost", Port: 5445, Database: throwaway, User: "dest_user", Password: "dest_pw",
	}))
	defer func() { _ = handler.DeleteConnection("dest_cleantest") }()

	args := os.Args[0:1]
	args = append(args, "schema", "sync", "--source", "source", "--dest", "dest_cleantest", "--clean", "--skip-confirmation")
	cmd.Execute("test", args)

	// Verify on the throwaway dest: drift fixed (extra column now present) and the stale row wiped.
	tw2, err := pgx.Connect(ctx, twURL)
	require.NoError(t, err)
	defer tw2.Close(ctx)

	assert.True(t, columnExists(t, ctx, tw2, "public", probe, "extra"),
		"--clean should have recreated the probe table with the source's `extra` column")

	var rows int
	err = tw2.QueryRow(ctx, "SELECT count(*) FROM "+probe).Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 0, rows, "--clean drop & recreate should have wiped the stale row")
}
