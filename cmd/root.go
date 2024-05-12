package cmd

import (
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"log"
	"os"
)

func Execute() {
	handler := config.ConfigHandler{
		PathHandler: config.OSPathHandler{},
	}
	app := &cli.App{
		Action: func(cCtx *cli.Context) error {
			fmt.Println("BASE ACTION")
			return nil
		},
		Commands: []*cli.Command{
			initCmd(&handler),
			syncCmd(&handler),
			listCmd(&handler),
			configCmd(&handler),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func initRequired(handler *config.ConfigHandler) {
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
