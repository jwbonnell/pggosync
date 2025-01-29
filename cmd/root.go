package cmd

import (
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/urfave/cli/v2"
	"log"
	"net/url"
	"os"
)

func Execute(build string, args []string) {
	handler := config.UserConfigHandler{
		PathHandler: config.OSPathHandler{},
	}
	app := &cli.App{
		Action: func(cCtx *cli.Context) error {
			fmt.Println("BASE ACTION TODO")
			return nil
		},
		Commands: []*cli.Command{
			versionCmd(build),
			initCmd(&handler),
			syncCmd(&handler),
			listCmd(&handler),
			configCmd(&handler),
		},
	}

	if err := app.Run(args); err != nil {
		log.Fatal(err)
	}
}

func initRequired(handler *config.UserConfigHandler) {
	forceInit := false
	_, err := handler.GetDefault()
	if err != nil {
		forceInit = true
	}

	if forceInit {
		fmt.Println("Run the init command to initialize the cli")
		os.Exit(0)
	}
}

func setupDatasources(c *config.UserConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource) {
	destination, err := datasource.NewReadWriteDataSource("destination", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", c.Destination.Host, c.Destination.Port),
		User:   url.UserPassword(c.Destination.User, c.Destination.Password),
		Path:   c.Destination.Database,
	})
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "Error datasource.NewDataSource %v\n", err)
		if err != nil {
			return nil, nil
		}
		os.Exit(1)
	}

	source, err := datasource.NewReadDataSource("source", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", c.Source.Host, c.Source.Port),
		User:   url.UserPassword(c.Source.User, c.Source.Password),
		Path:   c.Source.Database,
	})
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "Error datasource.NewDataSource %v\n", err)
		if err != nil {
			return nil, nil
		}
		os.Exit(1)
	}

	return source, destination
}
