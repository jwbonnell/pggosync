package cmd

import (
	"context"
	"fmt"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/urfave/cli/v2"
)

func listCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:    "list",
		Aliases: []string{"l"},
		Usage:   "List tables in source and destination databases",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "source",
				Aliases: []string{"s"},
				Usage:   "Source connection name (defaults to saved default).",
			},
			&cli.StringFlag{
				Name:    "dest",
				Aliases: []string{"d"},
				Usage:   "Destination connection name (defaults to saved default).",
			},
		},
		Action: func(cCtx *cli.Context) error {
			requireConnections(handler)

			srcConn, dstConn, err := resolveConnections(handler, cCtx.String("source"), cCtx.String("dest"))
			if err != nil {
				return fmt.Errorf("could not resolve connections: %w", err)
			}

			source, dest := setupDatasources(&srcConn, &dstConn)
			defer func() {
				_ = source.DB.Close(cCtx.Context)
				_ = dest.DB.Close(cCtx.Context)
			}()

			ctx := context.Background()
			srcTables, err := source.GetTables(ctx)
			if err != nil {
				return err
			}
			dstTables, err := dest.GetTables(ctx)
			if err != nil {
				return err
			}

			srcSet := tableSet(srcTables)
			dstSet := tableSet(dstTables)

			var shared, srcOnly, dstOnly []string
			for name := range srcSet {
				if dstSet[name] {
					shared = append(shared, name)
				} else {
					srcOnly = append(srcOnly, name)
				}
			}
			for name := range dstSet {
				if !srcSet[name] {
					dstOnly = append(dstOnly, name)
				}
			}

			printSection("Both", shared)
			printSection("Source only", srcOnly)
			printSection("Destination only", dstOnly)

			fmt.Printf("\nTotal: %d source, %d destination, %d shared\n",
				len(srcTables), len(dstTables), len(shared))
			return nil
		},
	}
}

func tableSet(tables []db.Table) map[string]bool {
	s := make(map[string]bool, len(tables))
	for _, t := range tables {
		s[t.FullName()] = true
	}
	return s
}

func printSection(label string, names []string) {
	if len(names) == 0 {
		return
	}
	fmt.Printf("\n%s (%d):\n", label, len(names))
	for _, n := range names {
		fmt.Printf("  %s\n", n)
	}
}
