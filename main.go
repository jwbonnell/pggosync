package main

import (
	"context"
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

func main() {
	app := &cli.App{
		Action: func(cCtx *cli.Context) error {
			fmt.Println("BASE ACTION")
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:    "init",
				Aliases: []string{"i"},
				Usage:   "initialize a new config",
				Action: func(cCtx *cli.Context) error {
					fmt.Println("TODO INIT NEW CONFIG")
					return nil
				},
			},
			{
				Name:  "sync",
				Usage: "Sync one or more groups",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "group",
						Aliases: []string{"g"},
						Usage:   "TODO g FLAG",
					},
					&cli.StringSliceFlag{
						Name:    "table",
						Aliases: []string{"t"},
						Usage:   "TODO t FLAG",
					},
				},
				Action: func(cCtx *cli.Context) error {
					fmt.Println("sync groups: ", cCtx.FlagNames())
					fmt.Println("sync groups: ", cCtx.StringSlice("group"))
					fmt.Println(cCtx.Args())

					groups := cCtx.StringSlice("group")
					tables := cCtx.StringSlice("table")

					var c config.Config
					data, _ := os.ReadFile("config.yml")
					err := yaml.Unmarshal(data, &c)
					if err != nil {
						log.Fatalf("error: %v", err)
					}
					fmt.Printf("--- t:\n%v\n\n", c)

					resolver := sync.NewTaskResolver(&c)
					tasks, err := resolver.Resolve(groups, tables)
					if err != nil {
						log.Fatalf("TaskResolver.Resolve: %v", err)
					}

					source, destination := tempGetDataSources(cCtx.Context)
					defer source.DB.Close(cCtx.Context)
					defer destination.DB.Close(cCtx.Context)

					err = sync.Sync(cCtx.Context, tasks, source, destination)
					if err != nil {
						log.Fatalf("sync.Sync: %v", err)
					}

					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "add a task to the list",
				Action: func(cCtx *cli.Context) error {
					fmt.Println("TODO LIST TABLES")
					return nil
				},
			},
			{
				Name:    "example sub command",
				Aliases: []string{"t"},
				Usage:   "options for task templates",
				Subcommands: []*cli.Command{
					{
						Name:  "add",
						Usage: "add a new template",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("new task template: ", cCtx.Args().First())
							return nil
						},
					},
					{
						Name:  "remove",
						Usage: "remove an existing template",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("removed task template: ", cCtx.Args().First())
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
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
