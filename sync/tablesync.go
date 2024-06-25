package sync

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

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
	sharedColumns := task.GetSharedColumnNames()
	scrubbedColumns := sharedColumns[:] ///TODO implement scrubbing

	if !task.Truncate || task.Preserve {
		if len(task.DestPK) == 0 {
			return fmt.Errorf("no primary key found for table %s", task.Table.FullName())
		}

		ttName := db.GenTempTableName(0, task.Table.Name)
		err := t.destination.CreateTempTable(ctx, ttName, task.Table.FullName())
		if err != nil {
			return fmt.Errorf("datasource.CreateTempTable: %w", err)
		}

		err = t.copy(ctx, task.Table.FullName(), ttName, task.Filter, scrubbedColumns, sharedColumns)
		if err != nil {
			return fmt.Errorf("TableSync.copy temp table %s: %w", ttName, err)
		}

		destPKs := task.GetDestPKs()
		action := "NOTHING"
		if !task.Preserve {
			var onConflictAction []string
			for i := range sharedColumns {
				if !slices.Contains(destPKs, sharedColumns[i]) {
					onConflictAction = append(onConflictAction, fmt.Sprintf("%s = EXCLUDED.%s", sharedColumns[i], sharedColumns[i]))
				}
			}

			if len(onConflictAction) > 0 {
				action = fmt.Sprintf("UPDATE SET %s", strings.Join(onConflictAction, ","))
			}
		}

		err = t.destination.InsertFromTempTable(ctx, ttName, task.Table.FullName(), sharedColumns, sharedColumns, strings.Join(destPKs, ","), action)
		if err != nil {
			return fmt.Errorf("TableSync.InsertFromTempTable %w", err)
		}

	} else {
		if task.DeferConstraints {
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

		err := t.copy(ctx, task.Table.FullName(), task.Table.FullName(), task.Filter, scrubbedColumns, sharedColumns)
		if err != nil {
			return fmt.Errorf("TableSync.copy %w", err)
		}
	}

	return nil
}

func (t *TableSync) copy(ctx context.Context, sourceTable string, destTable string, sourceFilter string, sourceFields []string, destFields []string) error {
	var buf bytes.Buffer
	sconn := t.source.DB.PgConn()
	cttag, err := sconn.CopyTo(ctx, &buf, fmt.Sprintf("COPY (SELECT %s FROM %s %s ) TO STDOUT", strings.Join(sourceFields, ","), sourceTable, sourceFilter))
	if err != nil {
		return fmt.Errorf("CopyTo - tag:%s  err:%w", cttag, err)
	}

	dconn := t.destination.DB.PgConn()
	cftag, err := dconn.CopyFrom(ctx, &buf, fmt.Sprintf("COPY %s (%s) FROM STDIN", destTable, strings.Join(destFields, ",")))
	if err != nil {
		return fmt.Errorf("CopyFrom - tag:%s err:%w cttag:%s", cftag, err, cttag)
	}

	return nil
}
