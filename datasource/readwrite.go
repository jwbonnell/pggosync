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

	fmt.Printf("%s DB connection successful\n", datasource.Name)

	return &datasource, nil
}

func (rw *ReadWriteDatasource) Truncate(ctx context.Context, table string) error {
	_, err := rw.DB.Exec(ctx, "TRUNCATE $1 CASCADE", table)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) DeleteAll(ctx context.Context, table string) error {
	_, err := rw.DB.Exec(ctx, "DELETE FROM $1", table)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) CreateTempTable(ctx context.Context, name string, sourceTable string) error {
	_, err := rw.DB.Exec(ctx, "CREATE TEMPORARY TABLE $1 AS TABLE $2 WITH NO DATA", name, sourceTable)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) SetSequence(ctx context.Context, sequence string, value int) error {
	_, err := rw.DB.Exec(ctx, "SELECT setval(%s, %d)", sequence, value)
	if err != nil {
		return err
	}

	return nil
}

func (rw *ReadWriteDatasource) InsertFromTempTable(ctx context.Context, tempTable string, destTable string, fieldSlice []string, onConflict string, action string) error {
	fields := strings.Join(fieldSlice[:], ",")
	_, err := rw.DB.Exec(ctx, `
		INSERT INTO %s %s (SELECT %s FROM %s)
				ON CONFLICT %s DO %s
	`, destTable, fields, fields, tempTable, onConflict, action)
	if err != nil {
		return err
	}

	return nil
}
