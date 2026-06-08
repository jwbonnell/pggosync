package sync

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

func Sync(ctx context.Context, deferConstraints bool, disableTriggers bool, quiet bool, dryRun bool, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) error {
	maxConcurrency := 1 // Allowed to run at the same time

	taskQueue := make(chan Task, maxConcurrency)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		taskErrs []error
	)

	tx, err := dest.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			fmt.Println("Rolling back...", err)
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Println("Rollback failed:", rbErr)
			}
		} else if dryRun {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Println("Rollback failed:", rbErr)
			}
		} else {
			if !quiet {
				fmt.Println("Committing...")
			}
			if cmErr := tx.Commit(ctx); cmErr != nil {
				fmt.Println("Commit failed:", cmErr)
			}
		}
	}()

	var ndc []db.NonDeferrableConstraints
	if deferConstraints {
		ndc, err = dest.GetNonDeferrableConstraints(ctx)
		if err != nil {
			return err
		}

		if !quiet {
			fmt.Println("Deferring constraints...")
		}
		err = db.DeferConstraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Println("DeferConstraints error:", err)
			return err
		}
	}

	var triggers []db.Trigger
	if disableTriggers {
		triggers, err = dest.GetUserTriggers(ctx)
		if err != nil {
			return err
		}

		err := db.DisableUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			fmt.Println("DisableUserTriggers Error: ", err)
			return err
		}
	}

	wg.Add(len(tasks))
	for range maxConcurrency {
		go func() {
			for task := range taskQueue {
				if !quiet {
					fmt.Printf("Syncing %s...\n", task.FullName())
				}
				ts := NewTableSync(source, dest)
				rowCount, taskErr := ts.Sync(ctx, &task)
				if taskErr != nil {
					fmt.Fprintf(os.Stderr, "Task failed %s: %v\n", task.FullName(), taskErr)
					mu.Lock()
					taskErrs = append(taskErrs, taskErr)
					mu.Unlock()
				} else if !quiet {
					fmt.Printf("Done %s (%s rows)\n", task.FullName(), formatCount(rowCount))
				}
				wg.Done()
			}
		}()
	}

	for i := range tasks {
		taskQueue <- tasks[i]
	}

	wg.Wait()
	close(taskQueue)

	if len(taskErrs) > 0 {
		err = fmt.Errorf("%d task(s) failed; first error: %w", len(taskErrs), taskErrs[0])
		return err
	}

	if disableTriggers {
		err := db.RestoreUserTriggers(ctx, tx.Conn(), triggers)
		if err != nil {
			fmt.Println("RestoreUserTriggers error:", err)
			return err
		}
	}

	if deferConstraints {
		if !quiet {
			fmt.Println("Restoring constraints...")
		}
		err = db.RestoreContraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Println("RestoreConstraints error:", err)
			return err
		}
	}

	if dryRun {
		fmt.Printf("Dry run complete — %d table(s) processed, no changes committed.\n", len(tasks))
	} else {
		fmt.Println("Sync complete.")
	}
	return nil
}

func formatCount(n int64) string {
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

func getTables(tasks []Task) []db.Table {
	tables := make([]db.Table, len(tasks))
	for i := range tasks {
		tables[i] = tasks[i].Table
	}
	return tables
}
