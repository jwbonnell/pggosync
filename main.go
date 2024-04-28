package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/sync"
)

func main() {
	ctx := context.Background()

	source, destination := tempGetDataSources(ctx)
	defer source.DB.Close(ctx)
	defer destination.DB.Close(ctx)

	//FAKE TASK RESOLVER
	tasks := []sync.Task{
		{
			Table:           db.Table{Schema: "public", Name: "country"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "city"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		/* {
			Table:           db.Table{Schema: "public", Name: "store"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "users"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "product"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "sale"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "order_status"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		},
		{
			Table:           db.Table{Schema: "public", Name: "status_name"},
			Preserve:        false,
			Truncate:        true,
			DeferContraints: true,
		}, */
	}

	sync.Sync(ctx, tasks, source, destination)
}

func tempGetDataSources(ctx context.Context) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource) {
	d := struct {
		Host     string
		Port     int
		UserName string
		Password string
		Database string
	}{
		Host:     "localhost",
		Port:     5438,
		UserName: "dest_user",
		Password: "dest_pw",
		Database: "postgres",
	}

	destination, err := datasource.NewReadWriteDataSource("destination", db.BuildUrl(d.Host, d.Port, d.UserName, d.Password, d.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error datasource.NewDataSource %v\n", err)
		os.Exit(1)
	}

	s := struct {
		Host     string
		Port     int
		UserName string
		Password string
		Database string
	}{
		Host:     "localhost",
		Port:     5437,
		UserName: "source_user",
		Password: "source_pw",
		Database: "postgres",
	}

	source, err := datasource.NewReadDataSource("source", db.BuildUrl(s.Host, s.Port, s.UserName, s.Password, s.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error datasource.NewDataSource %v\n", err)
		os.Exit(1)
	}

	return source, destination
}
