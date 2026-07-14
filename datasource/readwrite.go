package datasource

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ReadWriteDatasource struct {
	ReaderDataSource
}

// NewReadWriteDataSource connects, status-checks, and pre-loads tables for a read-write connection.
func NewReadWriteDataSource(Name string, u url.URL) (*ReadWriteDatasource, error) {
	var datasource ReadWriteDatasource
	db, err := pgx.Connect(context.Background(), u.String())
	if err != nil {
		return &ReadWriteDatasource{}, fmt.Errorf("unable to connect to database: %w", err)
	}

	datasource = ReadWriteDatasource{
		ReaderDataSource: ReaderDataSource{
			Url:   u.String(),
			DB:    db,
			Name:  Name,
			Debug: false,
		},
	}

	ctx := context.Background()
	err = datasource.StatusCheck(ctx)
	if err != nil {
		_ = db.Close(ctx)
		return nil, fmt.Errorf("db StatusCheck failed: %w", err)
	}

	_, err = datasource.GetTables(ctx)
	if err != nil {
		_ = db.Close(ctx)
		return nil, err
	}

	return &datasource, nil
}

// Truncate issues TRUNCATE on the table. When cascade is true it appends CASCADE, which also empties
// any table with a foreign key to the target; otherwise a plain TRUNCATE errors if the table is
// referenced, so cascading data loss must be explicitly opted into by the caller.
func (rw *ReadWriteDatasource) Truncate(ctx context.Context, table string, cascade bool) error {
	stmt := fmt.Sprintf("TRUNCATE %s", table)
	if cascade {
		stmt += " CASCADE"
	}
	_, err := rw.DB.Exec(ctx, stmt)
	if err != nil {
		return err
	}

	return nil
}

// DeleteAll issues an unconditional DELETE; used instead of TRUNCATE when constraints are deferred (TRUNCATE cannot run inside a deferred-constraint transaction).
func (rw *ReadWriteDatasource) DeleteAll(ctx context.Context, table string) error {
	_, err := rw.DB.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
	if err != nil {
		return err
	}

	return nil
}

// CreateTempTable creates a schema-only copy of sourceTable and verifies it landed in the session's temp schema.
func (rw *ReadWriteDatasource) CreateTempTable(ctx context.Context, name string, sourceTable string) error {
	_, err := rw.DB.Exec(ctx, fmt.Sprintf("CREATE TEMPORARY TABLE %s AS TABLE %s WITH NO DATA", name, sourceTable))
	if err != nil {
		return err
	}

	var cnt int
	err = rw.DB.QueryRow(ctx, "select count(*) FROM pg_namespace where oid  =  pg_my_temp_schema()").Scan(&cnt)
	if err != nil {
		return err
	}

	if cnt == 0 {
		return fmt.Errorf("no temp table found - Source:%s TTName:%s\n", sourceTable, name)
	}

	return nil
}

// GetTempTableRowCount counts rows in a temp table by name; used to verify COPY completeness.
func (rw *ReadWriteDatasource) GetTempTableRowCount(ctx context.Context, table string) (int64, error) {
	var count int64
	err := rw.DB.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// SetSequence calls setval to advance (or reset) a sequence to match the source database.
func (rw *ReadWriteDatasource) SetSequence(ctx context.Context, sequence string, value int) error {
	_, err := rw.DB.Exec(ctx, "SELECT setval($1, $2)", sequence, value)
	if err != nil {
		return err
	}

	return nil
}

// InsertFromTempTable runs the final upsert or preserve INSERT from the staging temp table into the
// destination and returns the number of rows actually inserted or updated (excluding conflicts that
// hit DO NOTHING), which is the meaningful figure to report — not the staging COPY row count.
func (rw *ReadWriteDatasource) InsertFromTempTable(ctx context.Context, tempTable string, destTable string, sourceFields []string, destFields []string, onConflict string, action string) (int64, error) {
	sql := fmt.Sprintf("INSERT INTO %s (%s) (SELECT %s FROM %s) ON CONFLICT (%s) DO %s", destTable, strings.Join(sourceFields, ","), strings.Join(destFields, ","), tempTable, onConflict, action)
	tag, err := rw.DB.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
