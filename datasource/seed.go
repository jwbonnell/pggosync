package datasource

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func seedDummyTable(ctx context.Context, db *pgx.Conn, table string, rowCount int) error {
	start := 1
	db.QueryRow(ctx, fmt.Sprintf("SELECT MAX(id) FROM %s", table)).Scan(&start)

	rowCount = rowCount + start
	_, err := db.Exec(ctx, fmt.Sprintf("INSERT INTO %s SELECT id, concat('DUMMY_', id) FROM GENERATE_SERIES($1::int, $2::int) as id", table), start+1, rowCount)

	if err != nil {
		return err
	}
	return nil
}
