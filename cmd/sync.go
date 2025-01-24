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
	"strings"
)

func syncCmd(handler *config.Handler) *cli.Command {
	cmd := cli.Command{
		Name:  "sync",
		Usage: "Sync one or more groups",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "truncate",
				Aliases: []string{"tr"},
				Usage:   "Truncates or deletes all rows from table before syncing. Delete all happens when --defer-constraints is passed.",
			},
			&cli.BoolFlag{
				Name:    "preserve",
				Aliases: []string{"p"},
				Usage:   "Preserve existing tables data. Uses insert on conflict do nothing. Without this flag, an upsert is performed. Ignored if --truncate is passed.",
			},
			&cli.BoolFlag{
				Name:    "no-safety",
				Aliases: []string{"ns"},
				Usage:   "Remove destination safety checks. Without passing this flag, only localhost is allowed.",
			},
			&cli.BoolFlag{
				Name:    "skip-confirmation",
				Aliases: []string{"sc"},
				Usage:   "Skip confirmation prompt. Useful for scripting.",
			},
			&cli.BoolFlag{
				Name:    "defer-constraints",
				Aliases: []string{"dc"},
				Usage:   "Defer constraints on destination database",
			},
			&cli.BoolFlag{
				Name:    "disable-triggers",
				Aliases: []string{"dt"},
				Usage:   "Disable triggers on destination database",
			},
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Required: true,
				Usage:    "Flag to specify the path to the sync config file.",
			},
			&cli.StringSliceFlag{
				Name:    "group",
				Aliases: []string{"g"},
				Usage:   "Flag to specify which groups will be synced. This can be passed multiple times for multiple groups to be synced.",
			},
			&cli.StringSliceFlag{
				Name:    "table",
				Aliases: []string{"t"},
				Usage:   "Flag to specify which tables will be synced. This can be passed multiple times for multiple tables to be synced.",
			},
			&cli.StringSliceFlag{
				Name:    "exclude",
				Aliases: []string{"e"},
				Usage:   "Flag to specify which tables to exclude from syncing.",
			},
		},
		Action: func(cCtx *cli.Context) error {
			initRequired(handler)
			truncate := cCtx.Bool("truncate")
			preserve := cCtx.Bool("preserve")
			noSafety := cCtx.Bool("no-safety")
			skipConfirmation := cCtx.Bool("skip-confirmation")
			deferConstraints := cCtx.Bool("defer-constraints")
			disableTriggers := cCtx.Bool("disable-triggers")
			syncConfigPath := cCtx.String("config")
			groups := cCtx.StringSlice("group")
			tables := cCtx.StringSlice("table")
			excluded := cCtx.StringSlice("exclude")

			var err error
			c, err := handler.GetCurrentConfig()
			if err != nil {
				log.Fatalf("Error retrieving DB connection config: %v", err)
			}

			sc, err := handler.GetSyncConfig(syncConfigPath)
			if err != nil {
				log.Fatalf("Error retrieving sync config: %v", err)
			}

			source, destination := setupDatasources(&c)
			defer func() {
				err := source.DB.Close(cCtx.Context)
				if err != nil {
					log.Fatalf("Error closing source DB connection: %v", err)
				}
				err = destination.DB.Close(cCtx.Context)
				if err != nil {
					log.Fatalf("Error closing destination DB connection: %v", err)
				}
			}()

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

				switch strings.TrimSpace(response) {
				case "yes":
					fmt.Println("Starting sync")
				case "no":
					fmt.Println("Sync cancelled")
					os.Exit(0)
				default:
					log.Fatalln("Invalid input, aborting...")
				}

			}

			if len(sc.Exclude) > 0 {
				excluded = append(excluded, sc.Exclude...)
			}
			excludedTables, err := opts.ProcessExcludedArgs(excluded)
			if err != nil {
				log.Fatalf("Failed to process excluded flag. Usage: ${SCHEMA}.${TABLE} or ${TABLE}: %v", err)
			}

			resolver := sync.NewTaskResolver(source, destination, sc.Groups, truncate, preserve, deferConstraints, disableTriggers, excludedTables)
			tasks, err := resolver.Resolve(cCtx.Context, groups, tables)
			if err != nil {
				log.Fatalf("TaskResolver.Resolve: %v", err)
			}

			if err = sync.Sync(cCtx.Context, deferConstraints, disableTriggers, tasks, source, destination); err != nil {
				log.Fatalf("sync.Sync: %v", err)
			}

			return nil
		},
	}
	return &cmd
}
