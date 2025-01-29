package cmd

import (
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
)

func listCmd(handler *config.UserConfigHandler) *cli.Command {
	cmd := cli.Command{
		Name:    "list",
		Aliases: []string{"l"},
		Usage:   "add a task to the list",
		Action: func(cCtx *cli.Context) error {
			initRequired(handler)
			fmt.Println("TODO LIST TABLES")
			return nil
		},
	}
	return &cmd
}
