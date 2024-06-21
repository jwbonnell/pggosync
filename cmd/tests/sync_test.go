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
	_, err = db.Exec(context.Background(), "INSERT INTO country (country_id, country_name) VALUES (8888, 'Country 8888') ON CONFLICT DO NOTHING")
	assert.NoError(t, err)

	args := os.Args[0:1]
	args = append(args, "sync")
	args = append(args, "--group")
	args = append(args, "country_var_1:1001")
	cmd.Execute(args)

	var country []Country
	err = pgxscan.Select(ctx, db, &country, "SELECT * FROM country WHERE country_id = 8888")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(country))
}
