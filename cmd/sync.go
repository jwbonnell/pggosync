package cmd

import (
	"bufio"
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
	"log"
	"os"
)

func syncCmd(handler *config.Handler) *cli.Command {
	cmd := cli.Command{
		Name:  "sync",
		Usage: "Sync one or more groups",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force sync",
			},
			&cli.BoolFlag{
				Name:  "truncate",
				Usage: "Truncate tables",
			},
			&cli.BoolFlag{
				Name:  "preserve",
				Usage: "Preserve existing tables",
			},
			&cli.BoolFlag{
				Name:  "skip-confirmation",
				Usage: "Skip confirmation",
			},
			&cli.BoolFlag{
				Name:  "defer-constraints",
				Usage: "Defer constraints",
			},
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
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Config override ID",
			},
		},
		Action: func(cCtx *cli.Context) error {
			initRequired(handler)
			truncate := cCtx.Bool("truncate")
			preserve := cCtx.Bool("preserve")
			noSafety := cCtx.Bool("no-safety")
			skipConfirmation := cCtx.Bool("skip-confirmation")
			deferConstraints := cCtx.Bool("defer-constraints")
			groups := cCtx.StringSlice("group")
			tables := cCtx.StringSlice("table")
			excluded := cCtx.StringSlice("exclude")
			configID := cCtx.String("config")

			var err error
			var c config.Config
			switch {
			case configID != "":
				c, err = handler.GetConfig(configID)
			default:
				c, err = handler.GetCurrentConfig()
			}

			if err != nil {
				log.Fatalf("Error retrieving config: %v", err)
			}

			source, destination := setupDatasources(&c)
			defer source.DB.Close(cCtx.Context)
			defer destination.DB.Close(cCtx.Context)

			if !noSafety && !destination.IsLocalHost(cCtx.Context) {
				log.Fatalf("Destination host is not localhost or 127.0.0.1, pass --no-safety to override this")
			}

			if !skipConfirmation {
				fmt.Print("Do you want to proceed? (yes/no): ")
				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					log.Fatalf("Error reading input: %v", err)
				}

				switch response {
				case "yes":
					fmt.Println("Starting sync")
				case "no":
					fmt.Println("Sync cancelled")
				default:
					log.Fatalln("Invalid input, aborting...")
				}

			}

			excludedTables, err := opts.ProcessExcludedArgs(excluded)
			if err != nil {
				log.Fatalf("Failed to process excluded flag. Usage: ${SCHEMA}.${TABLE} or ${TABLE}: %v", err)
			}

			resolver := sync.NewTaskResolver(source, destination, truncate, preserve, deferConstraints, excludedTables)
			tasks, err := resolver.Resolve(groups, tables)
			if err != nil {
				log.Fatalf("TaskResolver.Resolve: %v", err)
			}

			if err = sync.Sync(cCtx.Context, deferConstraints, tasks, source, destination); err != nil {
				log.Fatalf("sync.Sync: %v", err)
			}

			return nil
		},
	}
	return &cmd
}
