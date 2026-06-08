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

func Sync(ctx context.Context, deferConstraints bool, disableTriggers bool, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) error {
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
		} else {
			fmt.Println("Committing...")
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

		fmt.Println("Defer Contraints")
		err = db.DeferConstraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Println("DeferContraints Error: ", err)
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
				fmt.Printf("Processing Task %s: syncing...\n", task.FullName())
				ts := NewTableSync(source, dest)
				if taskErr := ts.Sync(ctx, &task); taskErr != nil {
					fmt.Fprintf(os.Stderr, "Task failed %s: %v\n", task.FullName(), taskErr)
					mu.Lock()
					taskErrs = append(taskErrs, taskErr)
					mu.Unlock()
				}
				fmt.Printf("Task Complete %s \n", task.FullName())
				wg.Done()
			}
		}()
	}

	for i := range tasks {
		fmt.Printf("Write task to queue: %s \n", tasks[i].FullName())
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
			fmt.Println("RestoreUserTriggers Error: ", err)
			return err
		}
	}

	if deferConstraints {
		fmt.Println("Restore Contraints")
		err = db.RestoreContraints(ctx, tx.Conn(), ndc)
		if err != nil {
			fmt.Println("RestoreContraints Error: ", err)
			return err
		}
	}

	fmt.Println("All tasks have completed")
	return nil
}

func getTables(tasks []Task) []db.Table {
	tables := make([]db.Table, len(tasks))
	for i := range tasks {
		tables[i] = tasks[i].Table
	}
	return tables
}
