package sync

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

type TableSync struct {
	source      *datasource.ReaderDataSource
	destination *datasource.ReadWriteDatasource
}

// NewTableSync creates a TableSync holding references to both datasources.
func NewTableSync(source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) *TableSync {
	return &TableSync{
		source:      source,
		destination: dest,
	}
}

// SyncFromBuffer performs the destination-side operations for a task, reading
// source data from buf (pre-fetched COPY TO STDOUT format) instead of querying
// the source database directly. The source CopyTo must run concurrently in a
// separate goroutine writing into buf.
func (t *TableSync) SyncFromBuffer(ctx context.Context, task *Task, buf io.Reader) (int64, error) {
	sharedColumns := task.GetSharedColumnNames()
	scrubbedColumns := task.ScrubColumns(sharedColumns)

	if !task.Truncate || task.Preserve {
		if len(task.DestPK) == 0 {
			return 0, fmt.Errorf("no primary key found for table %s", task.Table.FullName())
		}

		ttName := db.QuoteIdentifier(db.GenTempTableName(0, task.Table.Name))
		if err := t.destination.CreateTempTable(ctx, ttName, task.Table.SQLName()); err != nil {
			return 0, fmt.Errorf("datasource.CreateTempTable: %w", err)
		}

		dconn := t.destination.DB.PgConn()
		cftag, err := dconn.CopyFrom(ctx, buf, fmt.Sprintf("COPY %s (%s) FROM STDIN", ttName, strings.Join(scrubbedColumns, ",")))
		if err != nil {
			return 0, fmt.Errorf("CopyFrom temp table %s: %w", ttName, err)
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

		if err = t.destination.InsertFromTempTable(ctx, ttName, task.Table.SQLName(), sharedColumns, sharedColumns, strings.Join(destPKs, ","), action); err != nil {
			return 0, fmt.Errorf("TableSync.InsertFromTempTable %w", err)
		}
		rows := cftag.RowsAffected()
		if err := t.syncSequences(ctx, task); err != nil {
			return rows, err
		}
		return rows, nil

	}

	if task.DeferConstraints {
		if err := t.destination.DeleteAll(ctx, task.Table.SQLName()); err != nil {
			return 0, fmt.Errorf("TableSync DeleteAll %w", err)
		}
	} else {
		if err := t.destination.Truncate(ctx, task.Table.SQLName(), task.Cascade); err != nil {
			return 0, fmt.Errorf("TableSync Truncate %s (pass --cascade to also empty referencing tables, or --defer-constraints): %w", task.Table.FullName(), err)
		}
	}

	dconn := t.destination.DB.PgConn()
	cftag, err := dconn.CopyFrom(ctx, buf, fmt.Sprintf("COPY %s (%s) FROM STDIN", task.Table.SQLName(), strings.Join(scrubbedColumns, ",")))
	if err != nil {
		return 0, fmt.Errorf("CopyFrom %w", err)
	}
	rows := cftag.RowsAffected()
	if err := t.syncSequences(ctx, task); err != nil {
		return rows, err
	}
	return rows, nil
}

// syncSequences copies current sequence last_values from source to destination for all sequences owned by the task's table.
func (t *TableSync) syncSequences(ctx context.Context, task *Task) error {
	for _, seq := range task.SourceSequences {
		val, err := t.source.GetSequenceValue(ctx, seq.SequenceSchema, seq.Sequence)
		if err != nil {
			return fmt.Errorf("syncSequences read %s.%s: %w", seq.SequenceSchema, seq.Sequence, err)
		}
		qualifiedName := db.QuoteIdentifier(seq.SequenceSchema) + "." + db.QuoteIdentifier(seq.Sequence)
		if err := t.destination.SetSequence(ctx, qualifiedName, int(val)); err != nil {
			return fmt.Errorf("syncSequences set %s: %w", qualifiedName, err)
		}
	}
	return nil
}
