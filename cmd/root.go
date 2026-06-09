package cmd

import (
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/tui"
	"github.com/urfave/cli/v2"
)

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
		log.Fatal(err)
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

// resolveConnections looks up the source and destination ConnectionConfig by
// name. If either name is empty, it falls back to the saved defaults.
func resolveConnections(handler *config.UserConfigHandler, sourceName, destName string) (config.ConnectionConfig, config.ConnectionConfig, error) {
	if sourceName == "" || destName == "" {
		d, err := handler.GetDefaults()
		if err != nil {
			return config.ConnectionConfig{}, config.ConnectionConfig{}, fmt.Errorf("no defaults set and --source/--dest not provided; run 'pggosync config default --source <name> --dest <name>'")
		}
		if sourceName == "" {
			sourceName = d.Source
		}
		if destName == "" {
			destName = d.Dest
		}
	}
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

func setupDatasources(src, dst *config.ConnectionConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource) {
	destination, err := datasource.NewReadWriteDataSource("destination", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", dst.Host, dst.Port),
		User:   url.UserPassword(dst.User, dst.Password),
		Path:   dst.Database,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to destination (%s:%s/%s): %v\n", dst.Host, dst.Port, dst.Database, err)
		os.Exit(1)
	}

	source, err := datasource.NewReadDataSource("source", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", src.Host, src.Port),
		User:   url.UserPassword(src.User, src.Password),
		Path:   src.Database,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to source (%s:%s/%s): %v\n", src.Host, src.Port, src.Database, err)
		os.Exit(1)
	}

	return source, destination
}
