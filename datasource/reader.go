package datasource

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jwbonnell/pggosync/db"
)

type ReadDataSource interface {
	GetTables(ctx context.Context) ([]db.Table, error)
	TableExists(table db.Table) bool
	GetSchemas(ctx context.Context) ([]string, error)
	GetUserTriggers(ctx context.Context) ([]db.Trigger, error)
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

// NewReadDataSource connects to the database, runs a status check, and pre-loads the table list into r.Tables.
func NewReadDataSource(Name string, u url.URL) (*ReaderDataSource, error) {
	var datasource ReaderDataSource
	conn, err := pgx.Connect(context.Background(), u.String())
	if err != nil {
		return &ReaderDataSource{}, fmt.Errorf("unable to connect to database: %w", err)
	}

	datasource = ReaderDataSource{
		Url:   u.String(),
		DB:    conn,
		Name:  Name,
		Debug: false,
	}
	ctx := context.Background()
	err = datasource.StatusCheck(ctx)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("db StatusCheck failed: %w", err)
	}

	_, err = datasource.GetTables(ctx)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, err
	}

	return &datasource, nil
}

// GetTables fetches all non-system base tables from information_schema and caches them in r.Tables.
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

// TableExists scans the in-memory Tables cache; GetTables must have been called first.
func (r *ReaderDataSource) TableExists(table db.Table) bool {
	for _, t := range r.Tables {
		if table.Equal(t) {
			return true
		}
	}
	return false
}

// GetSchemas returns all schema names including system schemas (pg_catalog, information_schema, etc.).
func (r *ReaderDataSource) GetSchemas(ctx context.Context) ([]string, error) {
	var schemas []string
	err := pgxscan.Select(ctx, r.DB, &schemas, `SELECT schema_name FROM information_schema.schemata	ORDER BY 1`)
	if err != nil {
		return schemas, fmt.Errorf("%s GetSchemas %w", r.Name, err)
	}

	return schemas, nil
}

// GetUserTriggers returns only non-internal triggers (tgisinternal = false), which are safe to disable during sync.
func (r *ReaderDataSource) GetUserTriggers(ctx context.Context) ([]db.Trigger, error) {
	var triggers []db.Trigger
	err := pgxscan.Select(ctx, r.DB, &triggers, `
		SELECT
				tgname AS name,
				tgisinternal AS internal,
				tgenabled != 'D' AS enabled,
				tgconstraint != 0 AS integrity,
				tgrelid::regclass::text AS tgrelid
			FROM
				pg_trigger
			WHERE tgisinternal = false
	`)
	if err != nil {
		return triggers, fmt.Errorf("%s GetUserTriggers %w", r.Name, err)
	}

	return triggers, nil
}

// StatusCheck pings with exponential back-off then runs a round-trip SELECT to confirm full connectivity.
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

// GetNonDeferrableConstraints returns FK constraints that must be altered to DEFERRABLE before they can be deferred.
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

// GetColumns fetches non-generated columns; names that are reserved SQL keywords (order, limit, offset) are double-quoted.
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

	// Quote every column identifier so reserved words, mixed-case, and special-character
	// names are all safe to interpolate into COPY/INSERT column lists. Scrub-rule matching
	// trims the quotes back off (see task_resolver.validateScrubColumns), so callers that
	// compare against user input are unaffected.
	for i := range cols {
		cols[i].Name = db.QuoteIdentifier(cols[i].Name)
	}

	return cols, nil
}

// GetPrimaryKeys fetches primary key definitions from pg_index for all non-system schemas.
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

	// Quote PK column identifiers to match the quoting applied in GetColumns, so the
	// ON CONFLICT target and the "is this column a PK?" comparison in TableSync stay consistent.
	for i := range pks {
		pks[i].Column = db.QuoteIdentifier(pks[i].Column)
	}

	return pks, nil
}

// GetSequences fetches all sequences and their owning table/column relationships via pg_depend.
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

// GetSequenceValue reads the current last_value of a sequence without advancing it.
func (r *ReaderDataSource) GetSequenceValue(ctx context.Context, schema, sequence string) (int64, error) {
	var value int64
	qualified := pgx.Identifier{schema, sequence}.Sanitize()
	err := r.DB.QueryRow(ctx, fmt.Sprintf("SELECT last_value FROM %s", qualified)).Scan(&value)
	if err != nil {
		return 0, fmt.Errorf("GetSequenceValue %s.%s: %w", schema, sequence, err)
	}
	return value, nil
}

// GetRowCount issues a full COUNT(*); only call when an exact count is needed (e.g. the truncate confirmation banner).
func (r *ReaderDataSource) GetRowCount(ctx context.Context, tableName string) (int64, error) {
	return r.GetRowCountFiltered(ctx, tableName, "")
}

// GetRowCountFiltered issues COUNT(*) with an optional WHERE filter clause.
func (r *ReaderDataSource) GetRowCountFiltered(ctx context.Context, tableName, filter string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	if filter != "" {
		query += " WHERE " + filter
	}
	var count int64
	err := r.DB.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("GetRowCountFiltered %s: %w", tableName, err)
	}
	return count, nil
}

// loopbackHosts are the destination hostnames the safety check treats as local.
var loopbackHosts = map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}

// IsLocalHost returns true when the connection URL targets a loopback host, used for the safety check.
// It parses the URL and compares the host exactly, so hosts like "localhost.evil.com" do not pass.
func (r *ReaderDataSource) IsLocalHost(ctx context.Context) bool {
	u, err := url.Parse(r.Url)
	if err != nil {
		return false
	}
	return loopbackHosts[u.Hostname()]
}

// GetName returns the datasource label (e.g. "source" or "destination") used in error messages.
func (r *ReaderDataSource) GetName() string {
	return r.Name
}

// NewPgConn opens a new low-level connection to the source database.
// Callers are responsible for closing it with pgConn.Close(ctx).
// Used by pre-fetch goroutines so each gets its own independent connection.
func (r *ReaderDataSource) NewPgConn(ctx context.Context) (*pgconn.PgConn, error) {
	return pgconn.Connect(ctx, r.Url)
}
