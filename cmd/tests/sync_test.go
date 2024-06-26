package tests

import (
	"context"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/cmd"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type Country struct {
	CountryID int    `db:"country_id"`
	Name      string `db:"country_name"`
}

type City struct {
	CityID    int    `db:"city_id"`
	Name      string `db:"city_name"`
	CountryID int    `db:"country_id"`
}

func TestTruncate(t *testing.T) {
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (7777, 'Country 7777') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--group")
	args = append(args, "country_var_1:1000")
	args = append(args, "--truncate")
	args = append(args, "--skip-confirmation")
	cmd.Execute(args)

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
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (7777, 'Country 7777') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--group")
	args = append(args, "country_var_1:1000")
	args = append(args, "--truncate")
	args = append(args, "--defer-constraints")
	args = append(args, "--skip-confirmation")
	cmd.Execute(args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 7777")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(country))
}

func TestSync(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (1001, 'Country 1001 - TEST') ON CONFLICT (country_id) DO UPDATE SET country_name = EXCLUDED.country_name")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--group")
	args = append(args, "country_var_1:1001")
	args = append(args, "--skip-confirmation")
	cmd.Execute(args)

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
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (1001, 'Country 1001 - TEST') ON CONFLICT (country_id) DO UPDATE SET country_name = EXCLUDED.country_name")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--group")
	args = append(args, "country_var_1:1001")
	args = append(args, "--skip-confirmation")
	args = append(args, "--preserve")
	cmd.Execute(args)

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
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--table")
	args = append(args, "country")
	args = append(args, "--skip-confirmation")
	cmd.Execute(args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 1002")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
	assert.Equal(t, "Country 1002", country[0].Name)
}

func TestSync_TableMulti(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	db, err := pgx.Connect(context.Background(), "postgres://dest_user:dest_pw@localhost:5438/postgres")
	assert.NoError(t, err)
	defer db.Close(ctx)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--table")
	args = append(args, "country")
	args = append(args, "--table")
	args = append(args, "city")
	args = append(args, "--skip-confirmation")
	cmd.Execute(args)

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
