package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
)

func versionCmd(build string) *cli.Command {
	cmd := cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "version of CLI",
		Action: func(cCtx *cli.Context) error {
			fmt.Println(build)
			return nil
		},
	}
	return &cmd
}
