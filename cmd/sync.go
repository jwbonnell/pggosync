package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
)

func syncCmd(handler *config.UserConfigHandler) *cli.Command {
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
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "Suppress per-table progress output. Only errors and the final summary are printed.",
			},
			&cli.BoolFlag{
				Name:    "dry-run",
				Aliases: []string{"dr"},
				Usage:   "Simulate the sync without committing changes. Shows row counts per table.",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"con"},
				Value:   1,
				Usage:   "Number of source tables to pre-fetch concurrently. Each uses its own source connection. Destination writes remain sequential and atomic.",
			},
		},
		Action: func(cCtx *cli.Context) error {
			initRequired(handler)
			args := opts.CLIArgs{
				Truncate:         cCtx.Bool("truncate"),
				Preserve:         cCtx.Bool("preserve"),
				NoSafety:         cCtx.Bool("no-safety"),
				SkipConfirmation: cCtx.Bool("skip-confirmation"),
				Quiet:            cCtx.Bool("quiet"),
				DryRun:           cCtx.Bool("dry-run"),
				Concurrency:      cCtx.Int("concurrency"),
				DeferConstraints: cCtx.Bool("defer-constraints"),
				DisableTriggers:  cCtx.Bool("disable-triggers"),
				SyncConfigPath:   cCtx.String("config"),
				Groups:           cCtx.StringSlice("group"),
				Tables:           cCtx.StringSlice("table"),
				Excluded:         cCtx.StringSlice("exclude"),
			}

			var err error
			c, err := handler.GetCurrentConfig()
			if err != nil {
				log.Fatalf("Could not load connection config: %v", err)
			}

			sc, err := config.GetSyncConfig(args.SyncConfigPath)
			if err != nil {
				log.Fatalf("%v", err)
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

			if !args.NoSafety && !destination.IsLocalHost(cCtx.Context) {
				log.Fatalf("Destination host is not localhost or 127.0.0.1, pass --no-safety to override this")
			}

			var excluded []string
			if len(sc.Exclude) > 0 {
				excluded = append(args.Excluded, sc.Exclude...)
			}
			excludedTables, err := opts.ProcessExcludedArgs(excluded)
			if err != nil {
				log.Fatalf("Failed to process excluded flag. Usage: ${SCHEMA}.${TABLE} or ${TABLE}: %v", err)
			}

			resolver := sync.NewTaskResolver(source, destination, sc.Groups, args.Truncate, args.Preserve, args.DeferConstraints, args.DisableTriggers, excludedTables)
			tasks, err := resolver.Resolve(cCtx.Context, args.Groups, args.Tables)
			if err != nil {
				log.Fatalf("TaskResolver.Resolve: %v", err)
			}

			if !args.SkipConfirmation {
				reader := bufio.NewReader(os.Stdin)
				for {
					dryRunLabel := ""
					if args.DryRun {
						dryRunLabel = "  *** DRY RUN — no changes will be committed ***\n"
					}
					fmt.Printf(`
=================================================================
   ___  __________     ____
  / _ \/ ___/ ___/__  / __/_ _____  ____
 / ___/ (_ / (_ / _ \_\ \/ // / _ \/ __/
/_/   \___/\___/\___/___/\_, /_//_/\__/
                        /___/
%sConfig Description: %s
Source: %s:%s/%s                     Destination: %s:%s/%s
                                              :.
                 ============================:::'.
                 ============================::::::.
                 ============================::::'
                                              :'
Truncate?: %s
Preserve?: %s
Disable Triggers?: %s
Defer Constraints? %s
No Safety? %s
Tables: %d
=================================================================
`, dryRunLabel,
						sc.Description,
						c.Source.Host,
						c.Source.Port,
						c.Source.Database,
						c.Destination.Host,
						c.Destination.Port,
						c.Destination.Database,
						strconv.FormatBool(args.Truncate),
						strconv.FormatBool(args.Preserve),
						strconv.FormatBool(args.DisableTriggers),
						strconv.FormatBool(args.DeferConstraints),
						strconv.FormatBool(args.NoSafety),
						len(tasks),
					)

					fmt.Print("Do you want to proceed? (yes/no/more): ")
					response, err := reader.ReadString('\n')
					if err != nil {
						log.Fatalf("Error reading input: %v", err)
					}

					switch strings.TrimSpace(response) {
					case "yes":
						fmt.Println("Starting sync")
						goto proceed
					case "no":
						fmt.Println("Sync cancelled")
						os.Exit(0)
					case "more":
						fmt.Printf("\nTables to sync (%d):\n", len(tasks))
						for _, t := range tasks {
							strategy := "upsert"
							if t.Truncate && !t.Preserve {
								strategy = "truncate"
							} else if t.Preserve {
								strategy = "preserve"
							}
							rowInfo := ""
							if t.Truncate && !t.Preserve {
								rowInfo = fmt.Sprintf(" — %s dest rows will be deleted", sync.FormatCount(t.DestRowCount))
							}
							fmt.Printf("  %-40s [%s]%s\n", t.FullName(), strategy, rowInfo)
						}
						fmt.Println()
					default:
						log.Fatalln("Invalid input, aborting...")
					}
				}
			proceed:
			}

			if err = sync.Sync(cCtx.Context, args.DeferConstraints, args.DisableTriggers, args.Quiet, args.DryRun, args.Concurrency, tasks, source, destination); err != nil {
				log.Fatalf("sync.Sync: %v", err)
			}

			return nil
		},
	}
	return &cmd
}
