package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/tui"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// interactiveTTY reports whether both stdin and stdout are terminals, so interactive
// prompts are only shown when a human is driving — piped/scripted use stays plain.
func interactiveTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// connectionDetails renders a connection config as YAML with the password masked.
// Shared by `conn get` and the interactive `conn list` picker.
func connectionDetails(handler *config.UserConfigHandler, name string) (string, error) {
	conn, err := handler.GetConnection(name)
	if err != nil {
		return "", err
	}
	conn.Password = "***"
	out, err := yaml.Marshal(conn)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Connection: %s\n---\n%s", name, out), nil
}

// connCmd returns a CLI command with subcommands for managing database connections.
func connCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "conn",
		Usage: "Manage database connections",
		Subcommands: []*cli.Command{
			{
				Name:      "init",
				Usage:     "Create connection configs with placeholder defaults",
				ArgsUsage: "[name]",
				Action: func(cCtx *cli.Context) error {
					name := cCtx.Args().First()
					// No name: seed the default source/dest pair, never overwriting existing ones.
					if name == "" {
						created, err := handler.InitDefaultConnections()
						if err != nil {
							return err
						}
						fmt.Printf("Created connections %s with placeholder values \u2014 edit them or run 'pggosync conn new' for an interactive setup\n", strings.Join(created, " and "))
						return nil
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
				Usage: "List all connections; in a terminal, pick one to view its details",
				Action: func(cCtx *cli.Context) error {
					conns, err := handler.ListConnections()
					if err != nil {
						return err
					}
					if len(conns) == 0 {
						fmt.Println("No connections. Run 'pggosync conn init' to create the default source/dest pair.")
						return nil
					}
					// Piped/scripted: keep the plain, stable list output.
					if !interactiveTTY() {
						for _, c := range conns {
							fmt.Printf("  %s\n", c)
						}
						return nil
					}
					// Interactive: browse the list; selecting a connection replaces it with a
					// read-only detail view, and esc returns to the list.
					return tui.RunConnectionBrowser(handler)
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
					details, err := connectionDetails(handler, name)
					if err != nil {
						return err
					}
					fmt.Print(details)
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
