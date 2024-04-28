package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

func Sync(ctx context.Context, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) error {
	maxConcurrency := 1 // Allowed to run at the same time

	// Create a buffered channel with a capacity of maxConcurrency
	throttle := make(chan struct{}, maxConcurrency)

	var wg sync.WaitGroup

	tx, err := dest.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		} else {
			tx.Commit(ctx)
		}
	}()

	ndc, err := dest.GetNonDeferrableConstraints(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Defer Contraints")
	err = db.DeferConstraints(ctx, tx.Conn(), ndc)
	if err != nil {
		fmt.Println("DeferContraints Error: ", err)
		return err
	}

	for i := range tasks {
		wg.Add(1)
		go tasks[i].Run(ctx, throttle, &wg, source, dest)
	}

	wg.Wait()
	close(throttle)

	fmt.Println("Restore Contraints")
	err = db.RestoreContraints(ctx, tx.Conn(), ndc)
	if err != nil {
		fmt.Println("RestoreContraints Error: ", err)
		return err
	}

	fmt.Println("All tasks have completed")
	return nil
}
