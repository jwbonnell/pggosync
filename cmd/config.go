package cmd

import (
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"slices"
	"strings"
)

func configCmd(handler *config.UserConfigHandler) *cli.Command {

	cmd := cli.Command{
		Name:    "config",
		Aliases: []string{"t"},
		Usage:   "commands for accessing and switching configs",
		Action: func(c *cli.Context) error {
			fmt.Println("config base action")
			return nil
		},
		Subcommands: []*cli.Command{
			{
				Name:  "get",
				Usage: "get currently selected config information",
				Action: func(cCtx *cli.Context) error {
					initRequired(handler)
					configID, err := handler.GetDefault()
					if err != nil {
						return err
					}
					c, err := handler.GetConfig(configID)
					if err != nil {
						return err
					}

					out, err := yaml.Marshal(c)
					if err != nil {
						return err
					}

					fmt.Printf("Current: %s\n", configID)
					fmt.Println("--------------------------")
					fmt.Printf("Config: %s\n", out)
					return nil
				},
			},
			{
				Name:  "set",
				Usage: "set current config",
				Action: func(cCtx *cli.Context) error {
					initRequired(handler)
					configID := cCtx.Args().First()
					if configID == "" {
						return fmt.Errorf("please provide a config ID")
					}

					configs, err := handler.ListConfigs()
					if err != nil {
						return err
					}

					if !slices.Contains(configs, configID) {
						return fmt.Errorf("config %s does not exist", configID)
					}

					if err := handler.SetDefault(configID); err != nil {
						return err
					}
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "list configs",
				Action: func(cCtx *cli.Context) error {
					initRequired(handler)
					configs, err := handler.ListConfigs()
					if err != nil {
						return err
					}

					fmt.Println(strings.Join(configs, " | "))
					return nil
				},
			},
		},
	}
	return &cmd
}
