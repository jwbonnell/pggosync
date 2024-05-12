package cmd

import (
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func configCmd(handler *config.ConfigHandler) *cli.Command {

	cmd := cli.Command{
		Name:    "config",
		Aliases: []string{"t"},
		Usage:   "options for task templates",
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
					config, err := handler.GetConfig(configID)
					if err != nil {
						return err
					}

					out, err := yaml.Marshal(config)
					if err != nil {
						return err
					}

					fmt.Printf("Current: %s\n", configID)
					fmt.Println("--------------------------")
					fmt.Printf("Config: %s\n", out)
					return nil
				},
			},
		},
	}
	return &cmd
}
