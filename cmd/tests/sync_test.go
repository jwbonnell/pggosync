package tests

import (
	"context"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/cmd"
	"github.com/jwbonnell/pggosync/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	handler := config.UserConfigHandler{PathHandler: config.OSPathHandler{}}
	_ = handler.SaveConnection("source", config.ConnectionConfig{
		Host: "localhost", Port: 5432, Database: "postgres", User: "source_user", Password: "source_pw",
	})
	_ = handler.SaveConnection("dest", config.ConnectionConfig{
		Host: "localhost", Port: 5433, Database: "postgres", User: "dest_user", Password: "dest_pw",
	})
	_ = handler.SetDefaults("source", "dest")
	os.Exit(m.Run())
}

type Country struct {
	CountryID int    `db:"country_id"`
	Name      string `db:"country_name"`
}

type City struct {
	CityID    int    `db:"city_id"`
	Name      string `db:"city_name"`
	CountryID int    `db:"country_id"`
}

// TestTruncate syncs only the root `country` table (no FK dependents) so that
// truncate without --defer-constraints works without ordering issues.
func TestTruncate(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (7777, 'Country 7777') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--table", "country")
	args = append(args, "--truncate")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 7777")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(country))
}

func TestTruncateDeferConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (7777, 'Country 7777') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--group")
	args = append(args, "country_var_1:1000")
	args = append(args, "--truncate")
	args = append(args, "--defer-constraints")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 7777")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(country))
}

func TestTruncateDisableTriggers(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (7778, 'Country 7778') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--group")
	args = append(args, "country_var_1:1000")
	args = append(args, "--truncate")
	args = append(args, "--defer-constraints")
	args = append(args, "--disable-triggers")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 7778")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(country))
}

func TestSync(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (1001, 'Country 1001 - TEST') ON CONFLICT (country_id) DO UPDATE SET country_name = EXCLUDED.country_name")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--group", "country_var_1:1001")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 1001")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
	assert.Equal(t, "Country 1001", country[0].Name)
}

func TestSync_Preserve(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (1001, 'Country 1001 - TEST') ON CONFLICT (country_id) DO UPDATE SET country_name = EXCLUDED.country_name")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--group")
	args = append(args, "country_var_1:1001")
	args = append(args, "--skip-confirmation")
	args = append(args, "--preserve")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 1001")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
	assert.Equal(t, "Country 1001 - TEST", country[0].Name)
}

func TestSync_Table(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--table")
	args = append(args, "country")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 1002")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
	assert.Equal(t, "Country 1002", country[0].Name)
}

func TestDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()

	_, err = db.Exec(ctx, "INSERT INTO country (country_id, country_name) VALUES (8888, 'Country 8888 - DRY RUN TARGET') ON CONFLICT DO NOTHING")
	require.NoError(t, err)
	defer db.Exec(ctx, "DELETE FROM country WHERE country_id = 8888")

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--table", "country")
	args = append(args, "--truncate")
	args = append(args, "--dry-run")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	// Row must still exist — dry-run should not have committed the truncate.
	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 8888")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country), "dry-run must not commit changes")
}

func TestValidate(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}

	args := os.Args[0:1]
	args = append(args, "validate")
	args = append(args, "--config", "../../_configs/default.yml")
	// Should exit cleanly without calling log.Fatal.
	cmd.Execute("test", args)
}

func TestSync_TableMulti(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5433/postgres")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(ctx); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}()

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--config", "../../_configs/default.yml")
	args = append(args, "--table")
	args = append(args, "country")
	args = append(args, "--table")
	args = append(args, "city")
	args = append(args, "--skip-confirmation")
	cmd.Execute("test", args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 1002")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
	assert.Equal(t, "Country 1002", country[0].Name)

	var city []City
	err = pgxscan.Select(ctx, db, &city, "SELECT * FROM city WHERE country_id = 1002")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, 1, len(country))
	assert.Equal(t, "Country 1002", country[0].Name)
}
