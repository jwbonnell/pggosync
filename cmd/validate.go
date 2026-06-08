package cmd

import (
	"fmt"
	"log"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
)

func validateCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "Validate a sync config against both databases without syncing",
		Flags: []cli.Flag{
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
			initRequired(handler)

			c, err := handler.GetCurrentConfig()
			if err != nil {
				log.Fatalf("Could not load connection config: %v", err)
			}

			sc, err := config.GetSyncConfig(cCtx.String("config"))
			if err != nil {
				log.Fatalf("%v", err)
			}

			source, destination := setupDatasources(&c)
			defer func() {
				source.DB.Close(cCtx.Context)
				destination.DB.Close(cCtx.Context)
			}()

			var excluded []string
			if len(sc.Exclude) > 0 {
				excluded = append(cCtx.StringSlice("exclude"), sc.Exclude...)
			} else {
				excluded = cCtx.StringSlice("exclude")
			}
			excludedTables, err := opts.ProcessExcludedArgs(excluded)
			if err != nil {
				log.Fatalf("Failed to process excluded flag: %v", err)
			}

			truncate := cCtx.Bool("truncate")
			preserve := cCtx.Bool("preserve")
			resolver := sync.NewTaskResolver(source, destination, sc.Groups, truncate, preserve, false, false, excludedTables)
			tasks, err := resolver.Resolve(cCtx.Context, cCtx.StringSlice("group"), cCtx.StringSlice("table"))
			if err != nil {
				log.Fatalf("Validation failed: %v", err)
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
