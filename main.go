package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
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
		UserName: "source_dev_db_user",
		Password: "source_dev_db",
		Database: "source_dev_db_user",
	}

	source, err := datasource.NewDataSource("source", db.BuildUrl(s.Host, s.Port, s.UserName, s.Password, s.Database))
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
		UserName: "dest_dev_db_user",
		Password: "dest_dev_db",
		Database: "dest_dev_db_user",
	}

	destination, err := datasource.NewDataSource("destination", db.BuildUrl(d.Host, d.Port, d.UserName, d.Password, d.Database))
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

	var buf bytes.Buffer
	conn := source.DB.PgConn()
	conn.CopyTo(ctx, &buf, "COPY (SELECT * FROM city WHERE country_id > 20 ) TO STDOUT")
	if err != nil {
		fmt.Fprintf(os.Stderr, "CopyTo failed %v\n", err)
		os.Exit(1)
	}

	dconn := destination.DB.PgConn()
	_, err = dconn.CopyFrom(ctx, &buf, "COPY city (city_id, city_name, country_id) FROM STDIN")
	if err != nil {
		fmt.Fprintf(os.Stderr, "CopyFrom failed %v\n", err)
		os.Exit(1)
	}
	//res, _ := source.DB.Query(ctx, "COPY (SELECT * FROM city WHERE country_id > 20 ) TO STDOUT")
	//res, _ := source.DB.Query(ctx, "COPY (SELECT * FROM city WHERE country_id > 20 ) TO  '/tmp/foo.csv' CSV")
	fmt.Println(buf.String())
}
