package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"log"
	"os"
)

func Execute() {
	app := &cli.App{
		Action: func(cCtx *cli.Context) error {
			fmt.Println("BASE ACTION")
			return nil
		},
		Commands: []*cli.Command{
			initCmd(),
			syncCmd(),
			listCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
