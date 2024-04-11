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

	defer source.DB.Close(ctx)

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

	defer destination.DB.Close(ctx)

	version, err := source.Version(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Version failed %v\n", err)
		os.Exit(1)
	}

	fmt.Println(version)

	schemas, err := source.GetSchemas(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetSchemas failed %v\n", err)
		os.Exit(1)
	}

	fmt.Println(schemas)

	tables, err := source.GetTables(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetTables failed %v\n", err)
		os.Exit(1)
	}

	fmt.Println(tables)

	sync := sync.NewTableSync(source, destination)
	err = sync.Sync(ctx, "country", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed %v\n", err)
		os.Exit(1)
	}

	err = sync.Sync(ctx, "city", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed %v\n", err)
		os.Exit(1)
	}
}
