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

	err = datasource.StatusCheck(context.Background())
	if err != nil {
		return &ReadWriteDatasource{}, fmt.Errorf("db StatusCheck failed: %w", err)
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

	return nil
}

func (rw *ReadWriteDatasource) SetSequence(ctx context.Context, sequence string, value int) error {
	_, err := rw.DB.Exec(ctx, "SELECT setval($1, $2)", sequence, value)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) InsertFromTempTable(ctx context.Context, tempTable string, destTable string, fieldSlice []string, onConflict string, action string) error {
	fields := strings.Join(fieldSlice[:], ",")
	_, err := rw.DB.Exec(ctx, `INSERT INTO $1 $2 (SELECT $3 FROM $4) ON CONFLICT $5 DO $6`, destTable, fields, fields, tempTable, onConflict, action)
	if err != nil {
		return err
	}

	return nil
}
