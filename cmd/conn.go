package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/tui"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// connCmd returns a CLI command with subcommands for managing database connections.
func connCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "conn",
		Usage: "Manage database connections",
		Subcommands: []*cli.Command{
			{
				Name:      "init",
				Usage:     "Create a connection config with placeholder defaults",
				ArgsUsage: "<name>",
				Action: func(cCtx *cli.Context) error {
					name := cCtx.Args().First()
					if name == "" {
						name = "default"
					}
					if err := handler.InitConnection(name); err != nil {
						return err
					}
					fmt.Printf("Created connection %q with placeholder values — edit it or run 'pggosync conn new' for an interactive setup\n", name)
					return nil
				},
			},
			{
				Name:  "new",
				Usage: "Create a connection interactively",
				Action: func(cCtx *cli.Context) error {
					name, err := tui.RunConnectionForm(handler)
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							fmt.Println("Cancelled")
							return nil
						}
						return err
					}
					fmt.Printf("Saved connection %q\n", name)
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List all connections",
				Action: func(cCtx *cli.Context) error {
					conns, err := handler.ListConnections()
					if err != nil {
						return err
					}
					if len(conns) == 0 {
						fmt.Println("No connections. Run 'pggosync conn init <name>' to create one.")
						return nil
					}
					for _, c := range conns {
						fmt.Printf("  %s\n", c)
					}
					return nil
				},
			},
			{
				Name:      "get",
				Usage:     "Show a connection config",
				ArgsUsage: "<name>",
				Action: func(cCtx *cli.Context) error {
					name, err := requireSingleArg(cCtx, "connection name")
					if err != nil {
						return err
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
					fmt.Printf("Connection: %s\n---\n%s", name, out)
					return nil
				},
			},
			{
				Name:      "test",
				Usage:     "Test that a connection can reach its database",
				ArgsUsage: "<name>",
				Action: func(cCtx *cli.Context) error {
					name, err := requireSingleArg(cCtx, "connection name")
					if err != nil {
						return err
					}
					conn, err := handler.GetConnection(name)
					if err != nil {
						return err
					}
					ds, err := datasource.NewReadDataSource(name, connURL(&conn))
					if err != nil {
						return fmt.Errorf("connection %q failed (%s:%d/%s): %w", name, conn.Host, conn.Port, conn.Database, err)
					}
					defer func() { _ = ds.DB.Close(cCtx.Context) }()
					fmt.Printf("Connection %q OK (%s:%d/%s)\n", name, conn.Host, conn.Port, conn.Database)
					return nil
				},
			},
		},
	}
}
