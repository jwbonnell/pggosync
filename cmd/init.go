package cmd

import (
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
)

func initCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:      "init",
		Aliases:   []string{"i"},
		Usage:     "Create a new connection config",
		ArgsUsage: "<name>",
		Action: func(cCtx *cli.Context) error {
			name := cCtx.Args().First()
			if name == "" {
				name = "default"
			}
			return handler.InitConnection(name)
		},
	}
}
