package datasource

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/db"
)

type ReadDataSource interface {
	GetTables(ctx context.Context) ([]db.Table, error)
	TableExists(table db.Table) bool
	GetSchemas(ctx context.Context) ([]string, error)
	GetTriggers(ctx context.Context, table string) ([]db.Trigger, error)
	StatusCheck(ctx context.Context) error
	GetNonDeferrableConstraints(ctx context.Context) ([]db.NonDeferrableConstraints, error)
	GetName() string
}

type ReaderDataSource struct {
	Url    string
	DB     *pgx.Conn
	Name   string
	Tables []db.Table
	Debug  bool
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
	ctx := context.Background()
	err = datasource.StatusCheck(ctx)
	if err != nil {
		return &ReaderDataSource{}, fmt.Errorf("db StatusCheck failed: %w", err)
	}

	_, err = datasource.GetTables(ctx)
	if err != nil {
		return nil, err
	}

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

	r.Tables = tables
	return tables, nil
}

func (r *ReaderDataSource) TableExists(table db.Table) bool {
	for _, t := range r.Tables {
		if table.Equal(t) {
			return true
		}
	}
	return false
}

func (r *ReaderDataSource) GetSchemas(ctx context.Context) ([]string, error) {
	var schemas []string
	err := pgxscan.Select(ctx, r.DB, &schemas, `SELECT schema_name FROM information_schema.schemata	ORDER BY 1`)
	if err != nil {
		return schemas, fmt.Errorf("%s GetSchemas %w", r.Name, err)
	}

	return schemas, nil
}

func (r *ReaderDataSource) GetTriggers(ctx context.Context, table string) ([]db.Trigger, error) {
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
	`, table)
	fmt.Println(triggers)
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

func (r *ReaderDataSource) GetNonDeferrableConstraints(ctx context.Context) ([]db.NonDeferrableConstraints, error) {
	var constraints []db.NonDeferrableConstraints
	err := pgxscan.Select(ctx, r.DB, &constraints, `
		SELECT
				table_schema AS schema,
				table_name AS table,
				constraint_name
			FROM
				information_schema.table_constraints
			WHERE
				constraint_type = 'FOREIGN KEY' AND
				is_deferrable = 'NO'
	`)
	if err != nil {
		return constraints, err
	}

	return constraints, nil
}

func (r *ReaderDataSource) GetColumns(ctx context.Context) ([]db.Column, error) {
	var cols []db.Column
	err := pgxscan.Select(ctx, r.DB, &cols, `
		SELECT
			table_schema AS schema,
			table_name AS table,
			column_name AS column,
			data_type AS type
		FROM information_schema.columns
		WHERE is_generated = 'NEVER'
		  AND table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY 1, 2, 3
	`)
	if err != nil {
		return nil, err
	}

	return cols, nil
}

// TODO handle pk order for multi column pks
func (r *ReaderDataSource) GetPrimaryKeys(ctx context.Context) ([]db.PrimaryKey, error) {
	var pks []db.PrimaryKey
	err := pgxscan.Select(ctx, r.DB, &pks, `
		SELECT
          nspname AS schema,
          relname AS table,
          pg_attribute.attname AS column,
          format_type(pg_attribute.atttypid, pg_attribute.atttypmod) AS format_type,
          pg_attribute.attnum,
          pg_index.indkey
        FROM
          pg_index, pg_class, pg_attribute, pg_namespace
        WHERE indrelid = pg_class.oid 
          AND nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
          AND pg_class.relnamespace = pg_namespace.oid 
          AND pg_attribute.attrelid = pg_class.oid 
          AND pg_attribute.attnum = any(pg_index.indkey) 
          AND indisprimary
	`)
	if err != nil {
		return nil, err
	}

	return pks, nil
}

func (r *ReaderDataSource) GetSequences(ctx context.Context) ([]db.Sequence, error) {
	var sequences []db.Sequence
	err := pgxscan.Select(ctx, r.DB, &sequences, `
		 SELECT
			  nt.nspname as schema,
			  t.relname as table,
			  a.attname as column,
			  n.nspname as sequence_schema,
			  s.relname as sequence
			FROM pg_class s
			INNER JOIN pg_depend d ON d.objid = s.oid
			INNER JOIN pg_class t ON d.objid = s.oid AND d.refobjid = t.oid
			INNER JOIN pg_attribute a ON (d.refobjid, d.refobjsubid) = (a.attrelid, a.attnum)
			INNER JOIN pg_namespace n ON n.oid = s.relnamespace
			INNER JOIN pg_namespace nt ON nt.oid = t.relnamespace
			WHERE s.relkind = 'S'
	`)
	if err != nil {
		return nil, err
	}

	return sequences, nil
}

func (r *ReaderDataSource) IsLocalHost(ctx context.Context) bool {
	re := regexp.MustCompile(`postgres:\/\/.*:.*@(localhost|127\.0\.0\.1)`)
	return re.MatchString(r.Url)
}

func (r *ReaderDataSource) GetName() string {
	return r.Name
}
