package db

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

type Table struct {
	Schema string `db:"schema"`
	Name   string `db:"name"`
}

// FullName returns the schema-qualified table name as "schema.name". It is for display, logging,
// error messages, and map keys — NOT for interpolation into SQL. Use SQLName for that.
func (t *Table) FullName() string {
	return fmt.Sprintf("%s.%s", t.Schema, t.Name)
}

// SQLName returns the schema-qualified table name with each identifier safely quoted for use in
// SQL (e.g. `"public"."my table"`). This is the form that must be interpolated into queries so
// mixed-case, reserved-word, or special-character schema/table names do not break or inject.
func (t *Table) SQLName() string {
	return pgx.Identifier{t.Schema, t.Name}.Sanitize()
}

// QuoteIdentifier safely quotes a single SQL identifier (e.g. a generated temp table name).
func QuoteIdentifier(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// Equal reports whether two tables share the same schema and name (case-sensitive).
func (t *Table) Equal(other Table) bool {
	return t.Schema == other.Schema && t.Name == other.Name
}

// GetTables fetches all non-system base tables from information_schema ordered by schema then name.
func GetTables(ctx context.Context, db *pgx.Conn) ([]Table, error) {
	var tables []Table
	err := pgxscan.Select(ctx, db, &tables, `
		SELECT
				table_schema AS schema,
				table_name AS name
			FROM information_schema.tables
			WHERE	table_type = 'BASE TABLE'
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
			ORDER BY 1, 2
	`)
	if err != nil {
		return nil, err
	}

	return tables, nil
}
