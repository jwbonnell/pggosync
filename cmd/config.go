package cmd

import (
	"fmt"

	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// configCmd returns a CLI command with subcommands for inspecting saved connection configs.
func configCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:    "config",
		Aliases: []string{"t"},
		Usage:   "Manage connection configs",
		Action: func(c *cli.Context) error {
			fmt.Println("Use a subcommand: list, get")
			return nil
		},
		Subcommands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Show a connection config",
				ArgsUsage: "<name>",
				Action: func(cCtx *cli.Context) error {
					name := cCtx.Args().First()
					if name == "" {
						return fmt.Errorf("provide a connection name")
					}
					conn, err := handler.GetConnection(name)
					if err != nil {
						return err
					}
					conn.Password = "***"
					out, err := yaml.Marshal(conn)
					if err != nil {
						return err
					}
					fmt.Printf("Connection: %s\n", name)
					fmt.Println("---")
					fmt.Printf("%s", out)
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List all connection configs",
				Action: func(cCtx *cli.Context) error {
					conns, err := handler.ListConnections()
					if err != nil {
						return err
					}
					if len(conns) == 0 {
						fmt.Println("No connections. Run 'pggosync init <name>' to create one.")
						return nil
					}
					for _, c := range conns {
						fmt.Printf("  %s\n", c)
					}
					return nil
				},
			},
		},
	}
}
