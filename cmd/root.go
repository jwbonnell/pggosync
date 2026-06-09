package cmd

import (
	"fmt"
	"net/url"
	"os"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/tui"
	"github.com/urfave/cli/v2"
)

// Execute builds the CLI app and runs it; when invoked with no subcommand it launches the TUI.
func Execute(build string, args []string) {
	handler := config.UserConfigHandler{
		PathHandler: config.OSPathHandler{},
	}
	app := &cli.App{
		Action: func(cCtx *cli.Context) error {
			return tui.Run(&handler)
		},
		Commands: []*cli.Command{
			versionCmd(build),
			initCmd(&handler),
			syncCmd(&handler),
			validateCmd(&handler),
			listCmd(&handler),
			configCmd(&handler),
		},
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// requireConnections exits if no connections have been created yet.
func requireConnections(handler *config.UserConfigHandler) {
	conns, err := handler.ListConnections()
	if err != nil || len(conns) == 0 {
		fmt.Println("No connections found. Run 'pggosync init <name>' to create one.")
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

// setupDatasources opens connections to both databases and calls os.Exit(1) on any connection failure.
func setupDatasources(src, dst *config.ConnectionConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource) {
	destination, err := datasource.NewReadWriteDataSource("destination", url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", dst.Host, dst.Port),
		User:     url.UserPassword(dst.User, dst.Password),
		Path:     dst.Database,
		RawQuery: sslmodeQuery(dst.SSLMode),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to destination (%s:%d/%s): %v\n", dst.Host, dst.Port, dst.Database, err)
		os.Exit(1)
	}

	source, err := datasource.NewReadDataSource("source", url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", src.Host, src.Port),
		User:     url.UserPassword(src.User, src.Password),
		Path:     src.Database,
		RawQuery: sslmodeQuery(src.SSLMode),
	})
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
