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

func (t *Table) FullName() string {
	return fmt.Sprintf("%s.%s", t.Schema, t.Name)
}

func (t *Table) Equal(other Table) bool {
	return t.Schema == other.Schema && t.Name == other.Name
}

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
