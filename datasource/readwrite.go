package datasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ReadWriteDatasource struct {
	ReaderDataSource
}

func NewReadWriteDataSource(Name string, Url string) (*ReadWriteDatasource, error) {
	var datasource ReadWriteDatasource
	db, err := pgx.Connect(context.Background(), Url)
	if err != nil {
		return &ReadWriteDatasource{}, fmt.Errorf("unable to connect to database: %w", err)
	}

	datasource = ReadWriteDatasource{
		ReaderDataSource: ReaderDataSource{
			Url:   Url,
			DB:    db,
			Name:  Name,
			Debug: false,
		},
	}

	ctx := context.Background()
	err = datasource.StatusCheck(ctx)
	if err != nil {
		return &ReadWriteDatasource{}, fmt.Errorf("db StatusCheck failed: %w", err)
	}

	_, err = datasource.GetTables(ctx)
	if err != nil {
		return nil, err
	}

	return &datasource, nil
}

func (rw *ReadWriteDatasource) Truncate(ctx context.Context, table string) error {
	_, err := rw.DB.Exec(ctx, fmt.Sprintf("TRUNCATE %s CASCADE", table))
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) DeleteAll(ctx context.Context, table string) error {
	_, err := rw.DB.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
	if err != nil {
		return err
	}

	return nil
}

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

func (rw *ReadWriteDatasource) GetTempTableRowCount(ctx context.Context, table string) (int64, error) {
	var count int64
	err := rw.DB.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (rw *ReadWriteDatasource) SetSequence(ctx context.Context, sequence string, value int) error {
	_, err := rw.DB.Exec(ctx, "SELECT setval($1, $2)", sequence, value)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) InsertFromTempTable(ctx context.Context, tempTable string, destTable string, sourceFields []string, destFields []string, onConflict string, action string) error {
	sql := fmt.Sprintf("INSERT INTO %s (%s) (SELECT %s FROM %s) ON CONFLICT (%s) DO %s", destTable, strings.Join(sourceFields, ","), strings.Join(destFields, ","), tempTable, onConflict, action)
	_, err := rw.DB.Exec(ctx, sql)
	if err != nil {
		return err
	}
	return nil
}
