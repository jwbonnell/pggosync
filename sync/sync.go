package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

// TableResult holds per-table sync outcome.
type TableResult struct {
	Table    string
	Rows     int64
	Strategy string
	Err      error
}

// SyncResult is returned by Sync and carries per-table stats.
type SyncResult struct {
	Tables []TableResult
}

// Sync opens a single destination transaction, pre-fetches source rows into SafeBuffers concurrently (bounded by
// concurrency), drains each buffer sequentially, and commits. Rolls back on any error or when dryRun is true.
func Sync(ctx context.Context, deferConstraints bool, disableTriggers bool, quiet bool, dryRun bool, concurrency int, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource, out io.Writer) (SyncResult, error) {
	bufs := make([]*SafeBuffer, len(tasks))
	for i := range bufs {
		bufs[i] = NewSafeBuffer(&bytes.Buffer{})
	}

	// Launch a goroutine per task to CopyTo from source into its SafeBuffer.
	// A semaphore caps simultaneous source connections to concurrency.
	sem := make(chan struct{}, concurrency)
	var prefetchWg sync.WaitGroup

	go func() {
		for i := range tasks {
			sem <- struct{}{}
			prefetchWg.Add(1)
			go func(i int) {
				defer prefetchWg.Done()
				defer func() { <-sem }()

				task := &tasks[i]
				cols := task.GetSharedColumnNames()
				filterClause := ""
				if task.Filter != "" {
					filterClause = "WHERE " + task.Filter
				}
				query := fmt.Sprintf("COPY (SELECT %s FROM %s %s) TO STDOUT",
					strings.Join(cols, ","), task.FullName(), filterClause)

				pgConn, err := source.NewPgConn(ctx)
				if err != nil {
					bufs[i].SetDoneWithError(fmt.Errorf("source connection: %w", err))
					return
				}
				defer pgConn.Close(ctx)

				_, err = pgConn.CopyTo(ctx, bufs[i], query)
				if err != nil {
					bufs[i].SetDoneWithError(err)
				} else {
					bufs[i].SetDone()
				}
			}(i)
		}
	}()

	var result SyncResult

	tx, err := dest.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return result, err
	}

	defer func() {
		if err != nil {
			fmt.Fprintln(out, "Rolling back...", err)
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Fprintln(out, "Rollback failed:", rbErr)
			}
		} else if dryRun {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Fprintln(out, "Rollback failed:", rbErr)
			}
		} else {
			if !quiet {
				fmt.Fprintln(out, "Committing...")
			}
			if cmErr := tx.Commit(ctx); cmErr != nil {
				fmt.Fprintln(out, "Commit failed:", cmErr)
			}
		}
	}()

	var ndc []db.NonDeferrableConstraints
	if deferConstraints {
		ndc, err = dest.GetNonDeferrableConstraints(ctx)
		if err != nil {
			return result, err
		}

		if !quiet {
			fmt.Fprintln(out, "Deferring constraints...")
		}
		err = db.DeferConstraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Fprintln(out, "DeferConstraints error:", err)
			return result, err
		}
	}

	var triggers []db.Trigger
	if disableTriggers {
		triggers, err = dest.GetUserTriggers(ctx)
		if err != nil {
			return result, err
		}

		err := db.DisableUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			fmt.Fprintln(out, "DisableUserTriggers Error: ", err)
			return result, err
		}
	}

	// Sequential write loop: drain each SafeBuffer into the destination transaction.
	var taskErrs []error
	ts := NewTableSync(source, dest)

	for i := range tasks {
		if !quiet {
			fmt.Fprintf(out, "Syncing %s...\n", tasks[i].FullName())
		}

		strategy := taskStrategy(&tasks[i])
		rowCount, taskErr := ts.SyncFromBuffer(ctx, &tasks[i], bufs[i])
		result.Tables = append(result.Tables, TableResult{
			Table:    tasks[i].FullName(),
			Rows:     rowCount,
			Strategy: strategy,
			Err:      taskErr,
		})
		if taskErr != nil {
			fmt.Fprintf(out, "Task failed %s: %v\n", tasks[i].FullName(), taskErr)
			taskErrs = append(taskErrs, taskErr)
		} else if !quiet {
			fmt.Fprintf(out, "Done %s (%s rows)\n", tasks[i].FullName(), FormatCount(rowCount))
		}
	}

	prefetchWg.Wait()

	if len(taskErrs) > 0 {
		err = fmt.Errorf("%d task(s) failed; first error: %w", len(taskErrs), taskErrs[0])
		return result, err
	}

	if disableTriggers {
		err := db.RestoreUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			fmt.Fprintln(out, "RestoreUserTriggers error:", err)
			return result, err
		}
	}

	if deferConstraints {
		if !quiet {
			fmt.Fprintln(out, "Restoring constraints...")
		}
		err = db.RestoreContraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Fprintln(out, "RestoreConstraints error:", err)
			return result, err
		}
	}

	if dryRun {
		fmt.Fprintf(out, "Dry run complete — %d table(s) processed, no changes committed.\n", len(tasks))
	} else {
		fmt.Fprintln(out, "Sync complete.")
	}
	return result, nil
}

// FormatCount formats an integer with comma separators for human-readable output (e.g. 1,234,567).
func FormatCount(n int64) string {
	if n == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// taskStrategy returns the display label for the copy strategy a task will use.
func taskStrategy(t *Task) string {
	if t.Truncate && !t.Preserve {
		return "truncate"
	}
	if t.Preserve {
		return "preserve"
	}
	return "upsert"
}

// getTables extracts the embedded db.Table from each task into a flat slice.
func getTables(tasks []Task) []db.Table {
	tables := make([]db.Table, len(tasks))
	for i := range tasks {
		tables[i] = tasks[i].Table
	}
	return tables
}
