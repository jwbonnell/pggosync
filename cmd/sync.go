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
			initRequired(handler)
			groups := cCtx.StringSlice("group")
			tables := cCtx.StringSlice("table")

			c, err := handler.GetCurrentConfig()
			if err != nil {
				log.Fatalf("handler.GetCurrentConfig: %v", err)
			}

			resolver := sync.NewTaskResolver(&c)
			tasks, err := resolver.Resolve(groups, tables)
			if err != nil {
				log.Fatalf("TaskResolver.Resolve: %v", err)
			}

			source, destination := setupDatasources(&c)
			defer source.DB.Close(cCtx.Context)
			defer destination.DB.Close(cCtx.Context)

			if err = sync.Sync(cCtx.Context, tasks, source, destination); err != nil {
				log.Fatalf("sync.Sync: %v", err)
			}

			return nil
		},
	}
	return &cmd
}
