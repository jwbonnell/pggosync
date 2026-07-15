package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/tui"
	"github.com/urfave/cli/v2"
)

// Execute builds the CLI app and runs it; a bare invocation launches the TUI,
// unrecognized subcommands are an error.
func Execute(build string, args []string) {
	handler := config.UserConfigHandler{
		PathHandler: config.OSPathHandler{},
	}
	app := &cli.App{
		Usage: "Sync data between two PostgreSQL databases",
		Action: func(cCtx *cli.Context) error {
			if cCtx.Args().Present() {
				return fmt.Errorf("unknown command %q — run 'pggosync help' for usage", cCtx.Args().First())
			}
			return tui.Run(&handler)
		},
		Commands: []*cli.Command{
			versionCmd(build),
			runCmd(&handler),
			tablesCmd(&handler),
			connCmd(&handler),
			profileCmd(&handler),
			configCmd(&handler),
			schemaCmd(&handler),
		},
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// usageError prints the current command's usage/help (to stderr, alongside the error) and returns
// the formatted error, so a missing/invalid flag shows how to invoke the command rather than just a
// bare message. Writing help to stderr keeps stdout clean for downstream capture. ShowSubcommandHelp
// renders the command's own name, usage, and flags directly from cCtx.Command.
func usageError(cCtx *cli.Context, format string, a ...any) error {
	cCtx.App.Writer = os.Stderr
	_ = cli.ShowSubcommandHelp(cCtx)
	return fmt.Errorf(format, a...)
}

// requireSingleArg returns the command's one positional argument. It errors when
// anything follows it, because urfave/cli stops flag parsing at the first
// positional arg and would otherwise silently ignore trailing flags.
func requireSingleArg(cCtx *cli.Context, what string) (string, error) {
	if cCtx.Args().First() == "" {
		return "", fmt.Errorf("provide a %s", what)
	}
	if cCtx.NArg() > 1 {
		return "", fmt.Errorf("unexpected arguments after %q: %s — flags must come before the %s",
			cCtx.Args().First(), strings.Join(cCtx.Args().Tail(), " "), what)
	}
	return cCtx.Args().First(), nil
}

// requireConnections exits if no connections have been created yet. A real IO error listing them is
// distinct from "none exist": the former exits 1 (so scripts see a failure), the latter exits 0.
func requireConnections(handler *config.UserConfigHandler) {
	conns, err := handler.ListConnections()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: could not list connections:", err)
		os.Exit(1)
	}
	if len(conns) == 0 {
		fmt.Println("No connections found. Run 'pggosync conn init' to create the default source/dest pair.")
		os.Exit(0)
	}
}

// resolveConnections looks up the source and destination ConnectionConfig by name.
func resolveConnections(handler *config.UserConfigHandler, sourceName, destName string) (config.ConnectionConfig, config.ConnectionConfig, error) {
	src, err := handler.GetConnection(sourceName)
	if err != nil {
		return config.ConnectionConfig{}, config.ConnectionConfig{}, err
	}
	dst, err := handler.GetConnection(destName)
	if err != nil {
		return config.ConnectionConfig{}, config.ConnectionConfig{}, err
	}
	return src, dst, nil
}

// connURL builds a postgres connection URL from a connection config.
func connURL(c *config.ConnectionConfig) url.URL {
	return url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		User:     url.UserPassword(c.User, c.Password),
		Path:     c.Database,
		RawQuery: sslmodeQuery(c.SSLMode),
	}
}

// setupDatasources opens connections to both databases and calls os.Exit(1) on any connection failure.
func setupDatasources(src, dst *config.ConnectionConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource) {
	destination, err := datasource.NewReadWriteDataSource("destination", connURL(dst))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to destination (%s:%d/%s): %v\n", dst.Host, dst.Port, dst.Database, err)
		os.Exit(1)
	}

	source, err := datasource.NewReadDataSource("source", connURL(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to source (%s:%d/%s): %v\n", src.Host, src.Port, src.Database, err)
		os.Exit(1)
	}

	return source, destination
}

// sslmodeQuery returns a URL query string for the SSL mode, or an empty string when mode is unset.
func sslmodeQuery(mode string) string {
	if mode == "" {
		return ""
	}
	return "sslmode=" + url.QueryEscape(mode)
}
