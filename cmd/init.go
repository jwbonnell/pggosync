package cmd

import (
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
)

func initCmd(handler *config.Handler) *cli.Command {
	cmd := cli.Command{
		Name:    "init",
		Aliases: []string{"i"},
		Usage:   "initialize a new config",
		Action: func(cCtx *cli.Context) error {
			configID := cCtx.Args().First()
			if configID == "" {
				configID = "default"
			}
			return handler.InitConfig(configID)
		},
	}
	return &cmd
}
