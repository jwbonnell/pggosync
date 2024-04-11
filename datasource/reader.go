package datasource

import (
	"context"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/db"
)

type ReaderDataSource struct {
	Url   string
	DB    *pgx.Conn
	Name  string
	Debug bool
}

func NewReadDataSource(Name string, Url string) (*ReaderDataSource, error) {
	var datasource ReaderDataSource
	db, err := pgx.Connect(context.Background(), Url)
	if err != nil {
		return &ReaderDataSource{}, fmt.Errorf("unable to connect to database: %w", err)
	}

	datasource = ReaderDataSource{
		Url:   Url,
		DB:    db,
		Name:  Name,
		Debug: false,
	}

	err = datasource.StatusCheck(context.Background())
	if err != nil {
		return &ReaderDataSource{}, fmt.Errorf("db StatusCheck failed: %w", err)
	}

	fmt.Printf("%s DB connection successful\n", datasource.Name)

	return &datasource, nil
}

func (r *ReaderDataSource) GetTables(ctx context.Context) ([]db.Table, error) {
	var tables []db.Table
	err := pgxscan.Select(ctx, r.DB, &tables, `
		SELECT
				table_schema AS schema,
				table_name AS name
			FROM information_schema.tables
			WHERE	table_type = 'BASE TABLE'
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
			ORDER BY 1, 2
	`)
	if err != nil {
		return tables, fmt.Errorf("%s GetTables %w", r.Name, err)
	}

	return tables, nil
}

func (r *ReaderDataSource) GetSchemas(ctx context.Context) ([]string, error) {
	var schemas []string
	err := pgxscan.Select(ctx, r.DB, &schemas, `SELECT schema_name FROM information_schema.schemata	ORDER BY 1`)
	if err != nil {
		return schemas, fmt.Errorf("%s GetSchemas %w", r.Name, err)
	}

	return schemas, nil
}

func (r *ReaderDataSource) GetTriggers(ctx context.Context) ([]db.Trigger, error) {
	var triggers []db.Trigger
	err := pgxscan.Select(ctx, r.DB, &triggers, `
		SELECT
				tgname AS name,
				tgisinternal AS internal,
				tgenabled != 'D' AS enabled,
				tgconstraint != 0 AS integrity
			FROM
				pg_trigger
			WHERE
				pg_trigger.tgrelid = $1::regclass
	`)
	if err != nil {
		return triggers, fmt.Errorf("%s GetTriggers %w", r.Name, err)
	}

	return triggers, nil
}

func (r *ReaderDataSource) StatusCheck(ctx context.Context) error {

	// If the user doesn't give us a deadline set 1 seconr.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Second)
		defer cancel()
	}

	var pingError error
	for attempts := 1; ; attempts++ {
		pingError = r.DB.Ping(ctx)
		if pingError == nil {
			break
		}
		time.Sleep(time.Duration(attempts) * 100 * time.Millisecond)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Run a simple query to determine connectivity.
	// Running this query forces a round trip through the database.
	var tmp bool
	return r.DB.QueryRow(context.Background(), "SELECT true").Scan(&tmp)
}

func (r *ReaderDataSource) Version(ctx context.Context) (string, error) {
	var version string
	err := r.DB.QueryRow(context.Background(), "SELECT VERSION()").Scan(&version)
	if err != nil {
		return "", err
	}

	return version, nil
}

func (r *ReaderDataSource) GetNonDeferrableConstraints() ([]db.NonDeferrableConstraints, error) {
	var constraints []db.NonDeferrableConstraints
	err := r.DB.QueryRow(context.Background(), `
		SELECT
				table_schema AS schema,
				table_name AS table,
				constraint_name
			FROM
				information_schema.table_constraints
			WHERE
				constraint_type = 'FOREIGN KEY' AND
				is_deferrable = 'NO'
	`).Scan(&constraints)
	if err != nil {
		return constraints, err
	}

	return constraints, nil
}
