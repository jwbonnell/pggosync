package sync

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

type Task struct {
	Table           db.Table
	Filter          string
	Preserve        bool
	Truncate        bool
	DeferContraints bool
}

func (t *Task) Run(ctx context.Context, throttle chan struct{}, wg *sync.WaitGroup, source *datasource.ReaderDataSource, destination *datasource.ReadWriteDatasource) error {
	defer wg.Done()

	fmt.Printf("Processing Task %s: Aquiring semaphore\n", t.Table.FullName())
	throttle <- struct{}{}

	// Do work
	fmt.Printf("Task %s: Semaphore acquired, syncing\n", t.Table.FullName())
	time.Sleep(10 * time.Millisecond)

	sync := NewTableSync(source, destination)
	err := sync.Sync(ctx, t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed %v\n", err)
	}

	// Release semaphore
	<-throttle
	fmt.Printf("Task %s: Semaphore released\n", t.Table.FullName())
	return nil
}
