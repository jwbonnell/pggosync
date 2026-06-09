package cmd

import (
	"fmt"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func configCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:    "config",
		Aliases: []string{"t"},
		Usage:   "Manage connection configs",
		Action: func(c *cli.Context) error {
			fmt.Println("Use a subcommand: list, get, default")
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
					d, _ := handler.GetDefaults()
					for _, c := range conns {
						markers := []string{}
						if c == d.Source {
							markers = append(markers, "source")
						}
						if c == d.Dest {
							markers = append(markers, "dest")
						}
						if len(markers) > 0 {
							fmt.Printf("  %s  [default %s]\n", c, strings.Join(markers, ", "))
						} else {
							fmt.Printf("  %s\n", c)
						}
					}
					return nil
				},
			},
			{
				Name:  "default",
				Usage: "Get or set the default source and destination connections",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "source",
						Usage: "Default source connection name",
					},
					&cli.StringFlag{
						Name:  "dest",
						Usage: "Default destination connection name",
					},
				},
				Action: func(cCtx *cli.Context) error {
					src := cCtx.String("source")
					dst := cCtx.String("dest")
					if src == "" && dst == "" {
						// Show current defaults.
						d, err := handler.GetDefaults()
						if err != nil {
							return err
						}
						fmt.Printf("source: %s\ndest:   %s\n", d.Source, d.Dest)
						return nil
					}
					// Get current defaults to fill in any unspecified side.
					current, _ := handler.GetDefaults()
					if src == "" {
						src = current.Source
					}
					if dst == "" {
						dst = current.Dest
					}
					if err := handler.SetDefaults(src, dst); err != nil {
						return err
					}
					fmt.Printf("Defaults set — source: %s  dest: %s\n", src, dst)
					return nil
				},
			},
		},
	}
}
