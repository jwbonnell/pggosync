package cmd

import (
	"fmt"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
)

// validateCmd returns a CLI command that resolves tasks and reports what would be synced without running the sync.
func validateCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "Validate a sync config against both databases without syncing",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "source",
				Aliases:  []string{"s"},
				Required: true,
				Usage:    "Source connection name.",
			},
			&cli.StringFlag{
				Name:     "dest",
				Aliases:  []string{"d"},
				Required: true,
				Usage:    "Destination connection name.",
			},
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Required: true,
				Usage:    "Path to sync config file.",
			},
			&cli.StringSliceFlag{
				Name:    "group",
				Aliases: []string{"g"},
				Usage:   "Limit validation to specific groups.",
			},
			&cli.StringSliceFlag{
				Name:    "table",
				Aliases: []string{"t"},
				Usage:   "Limit validation to specific tables.",
			},
			&cli.StringSliceFlag{
				Name:    "exclude",
				Aliases: []string{"e"},
				Usage:   "Tables to exclude.",
			},
			&cli.BoolFlag{
				Name:    "truncate",
				Aliases: []string{"tr"},
				Usage:   "Validate as if --truncate were passed (relaxes PK requirement).",
			},
			&cli.BoolFlag{
				Name:    "preserve",
				Aliases: []string{"p"},
				Usage:   "Validate as if --preserve were passed.",
			},
		},
		Action: func(cCtx *cli.Context) error {
			requireConnections(handler)

			srcConn, dstConn, err := resolveConnections(handler, cCtx.String("source"), cCtx.String("dest"))
			if err != nil {
				return fmt.Errorf("could not resolve connections: %w", err)
			}

			sc, err := config.GetSyncConfig(cCtx.String("config"))
			if err != nil {
				return err
			}

			source, destination := setupDatasources(&srcConn, &dstConn)
			defer func() {
				_ = source.DB.Close(cCtx.Context)
				_ = destination.DB.Close(cCtx.Context)
			}()

			var excluded []string
			if len(sc.Exclude) > 0 {
				excluded = append(cCtx.StringSlice("exclude"), sc.Exclude...)
			} else {
				excluded = cCtx.StringSlice("exclude")
			}
			excludedTables, err := opts.ProcessExcludedArgs(excluded)
			if err != nil {
				return fmt.Errorf("failed to process --exclude: %w", err)
			}

			truncate := cCtx.Bool("truncate")
			preserve := cCtx.Bool("preserve")
			resolver := sync.NewTaskResolver(source, destination, sc.Groups, truncate, preserve, false, false, excludedTables)
			tasks, err := resolver.Resolve(cCtx.Context, cCtx.StringSlice("group"), cCtx.StringSlice("table"))
			if err != nil {
				return err
			}

			fmt.Printf("Config OK — %d table(s) would be synced:\n", len(tasks))
			for _, t := range tasks {
				strategy := "upsert"
				if t.Truncate && !t.Preserve {
					strategy = "truncate"
				} else if t.Preserve {
					strategy = "preserve"
				}
				fmt.Printf("  %-40s [%s]\n", t.FullName(), strategy)
			}
			return nil
		},
	}
}
