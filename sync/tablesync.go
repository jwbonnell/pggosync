package sync

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

type TableSync struct {
	source      *datasource.ReaderDataSource
	destination *datasource.ReadWriteDatasource
}

func NewTableSync(source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) *TableSync {
	return &TableSync{
		source:      source,
		destination: dest,
	}
}

func (t *TableSync) Sync(ctx context.Context, task *Task) error {
	if !task.Truncate {
		//TODO PK CHECK
		ttName := db.GenTempTableName(0)
		err := t.destination.CreateTempTable(ctx, ttName, task.Table.FullName())
		if err != nil {
			return fmt.Errorf("datasource.CreateTempTable: %w", err)
		}

		err = t.copy(ctx, task.Table.FullName(), ttName, "")
		if err != nil {
			return fmt.Errorf("TableSync.copyTo temp table %s: %w", ttName, err)
		}

		action := ""
		onConflict := "NOTHING"
		if !task.Preserve {
			onConflict = ""
		}

		err = t.destination.InsertFromTempTable(ctx, ttName, task.Table.FullName(), []string{}, onConflict, action)
		if err != nil {
			return fmt.Errorf("TableSync.InsertFromTempTable %w", err)
		}

	} else {
		if task.DeferContraints {
			err := t.destination.DeleteAll(ctx, task.Table.FullName())
			if err != nil {
				return fmt.Errorf("TableSync DeleteAll %w", err)
			}
		} else {
			err := t.destination.Truncate(ctx, task.Table.FullName())
			if err != nil {
				return fmt.Errorf("TableSync Truncate %w", err)
			}
		}

		err := t.copy(ctx, task.Table.FullName(), task.Table.FullName(), task.Filter)
		if err != nil {
			return fmt.Errorf("TableSync.copy %w", err)
		}
	}

	return nil
}

func (t *TableSync) copy(ctx context.Context, sourceTable string, destTable string, filter string) error {
	//TODO consider merging copy anc copyTo, keeping separate for now
	var buf bytes.Buffer
	sconn := t.source.DB.PgConn()
	_, err := sconn.CopyTo(ctx, &buf, fmt.Sprintf("COPY (SELECT * FROM %s %s ) TO STDOUT", sourceTable, filter))
	if err != nil {
		return err
	}

	dconn := t.destination.DB.PgConn()
	_, err = dconn.CopyFrom(ctx, &buf, fmt.Sprintf("COPY %s FROM STDIN", destTable))
	if err != nil {
		return err
	}

	return nil
}
