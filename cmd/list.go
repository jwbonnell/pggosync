package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
)

func listCmd() *cli.Command {
	cmd := cli.Command{
		Name:    "list",
		Aliases: []string{"l"},
		Usage:   "add a task to the list",
		Action: func(cCtx *cli.Context) error {
			fmt.Println("TODO LIST TABLES")
			return nil
		},
	}
	return &cmd
}
