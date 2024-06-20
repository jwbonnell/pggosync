package cmd

import (
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
	"log"
)

func syncCmd(handler *config.ConfigHandler) *cli.Command {
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
			deferConstraints := cCtx.Bool("defer-constraints")
			groups := cCtx.StringSlice("group")
			tables := cCtx.StringSlice("table")
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

			resolver := sync.NewTaskResolver(&c, truncate, preserve, deferConstraints)
			tasks, err := resolver.Resolve(groups, tables)
			if err != nil {
				log.Fatalf("TaskResolver.Resolve: %v", err)
			}

			source, destination := setupDatasources(&c)
			defer source.DB.Close(cCtx.Context)
			defer destination.DB.Close(cCtx.Context)

			if err = sync.Sync(cCtx.Context, deferConstraints, tasks, source, destination); err != nil {
				log.Fatalf("sync.Sync: %v", err)
			}

			return nil
		},
	}
	return &cmd
}
