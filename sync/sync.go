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
func Sync(ctx context.Context, deferConstraints bool, disableTriggers bool, quiet bool, dryRun bool, concurrency int, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource, out io.Writer) (res SyncResult, err error) {
	// logf serializes writes to out so goroutine-originated progress lines don't interleave.
	var outMu sync.Mutex
	logf := func(format string, args ...any) {
		outMu.Lock()
		fmt.Fprintf(out, format, args...)
		outMu.Unlock()
	}

	// Guard against a caller passing a non-positive concurrency: 0 gives an unbuffered
	// semaphore that deadlocks the prefetch launcher, and a negative value panics make().
	if concurrency < 1 {
		concurrency = 1
	}

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
				cols := task.GetSelectColumns()
				filterClause := ""
				if task.Filter != "" {
					filterClause = "WHERE " + task.Filter
				}
				query := fmt.Sprintf("COPY (SELECT %s FROM %s %s) TO STDOUT",
					strings.Join(cols, ", "), task.SQLName(), filterClause)

				if !quiet {
					logf("Prefetching %s...\n", task.FullName())
				}

				pgConn, err := source.NewPgConn(ctx)
				if err != nil {
					bufs[i].SetDoneWithError(fmt.Errorf("source connection: %w", err))
					if !quiet {
						logf("Prefetch ready %s\n", task.FullName())
					}
					return
				}
				defer pgConn.Close(ctx)

				_, err = pgConn.CopyTo(ctx, bufs[i], query)
				if err != nil {
					bufs[i].SetDoneWithError(err)
				} else {
					bufs[i].SetDone()
				}
				if !quiet {
					logf("Prefetch ready %s\n", task.FullName())
				}
			}(i)
		}
	}()

	tx, err := dest.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return res, err
	}

	// The named return err drives this cleanup: any earlier return that set err (or a
	// commit failure recorded here) results in a rollback / a surfaced error rather than
	// a silently-swallowed commit.
	defer func() {
		switch {
		case err != nil:
			logf("Rolling back... %v\n", err)
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logf("Rollback failed: %v\n", rbErr)
			}
		case dryRun:
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logf("Rollback failed: %v\n", rbErr)
			}
		default:
			if !quiet {
				logf("Committing...\n")
			}
			if cmErr := tx.Commit(ctx); cmErr != nil {
				logf("Commit failed: %v\n", cmErr)
				err = fmt.Errorf("commit failed: %w", cmErr)
			} else {
				logf("Sync complete.\n")
			}
		}
	}()

	var ndc []db.NonDeferrableConstraints
	if deferConstraints {
		ndc, err = dest.GetNonDeferrableConstraints(ctx)
		if err != nil {
			return res, err
		}

		if !quiet {
			logf("Deferring constraints...\n")
		}
		err = db.DeferConstraints(ctx, tx.Conn(), ndc)
		if err != nil {
			logf("DeferConstraints error: %v\n", err)
			return res, err
		}
	}

	var triggers []db.Trigger
	if disableTriggers {
		triggers, err = dest.GetUserTriggers(ctx)
		if err != nil {
			return res, err
		}

		err = db.DisableUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			logf("DisableUserTriggers Error: %v\n", err)
			return res, err
		}
	}

	// Sequential write loop: drain each SafeBuffer into the destination transaction.
	var taskErrs []error
	ts := NewTableSync(source, dest)

	for i := range tasks {
		if !quiet {
			logf("Syncing %s...\n", tasks[i].FullName())
		}

		strategy := taskStrategy(&tasks[i])
		rowCount, taskErr := ts.SyncFromBuffer(ctx, &tasks[i], bufs[i])
		res.Tables = append(res.Tables, TableResult{
			Table:    tasks[i].FullName(),
			Rows:     rowCount,
			Strategy: strategy,
			Err:      taskErr,
		})
		if taskErr != nil {
			logf("Task failed %s: %v\n", tasks[i].FullName(), taskErr)
			taskErrs = append(taskErrs, taskErr)
		} else if !quiet {
			logf("Done %s (%s rows)\n", tasks[i].FullName(), FormatCount(rowCount))
		}
	}

	prefetchWg.Wait()

	if len(taskErrs) > 0 {
		err = fmt.Errorf("%d task(s) failed; first error: %w", len(taskErrs), taskErrs[0])
		return res, err
	}

	if disableTriggers {
		err = db.RestoreUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			logf("RestoreUserTriggers error: %v\n", err)
			return res, err
		}
	}

	if deferConstraints {
		if !quiet {
			logf("Restoring constraints...\n")
		}
		err = db.RestoreContraints(ctx, tx.Conn(), ndc)
		if err != nil {
			logf("RestoreConstraints error: %v\n", err)
			return res, err
		}
	}

	if dryRun {
		logf("Dry run complete — %d table(s) processed, no changes committed.\n", len(tasks))
	}
	return res, nil
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
