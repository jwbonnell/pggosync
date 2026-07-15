package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/urfave/cli/v2"
)

// schemaCmd returns a CLI command with subcommands for copying the database schema (DDL).
func schemaCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "schema",
		Usage: "Copy the database schema (DDL) between databases",
		Subcommands: []*cli.Command{
			{
				Name:  "sync",
				Usage: "Dump the whole source schema and apply it to the destination (pg_dump | psql)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "source",
						Aliases: []string{"s"},
						Usage:   "Source connection name.",
					},
					&cli.StringFlag{
						Name:    "dest",
						Aliases: []string{"d"},
						Usage:   "Destination connection name.",
					},
					&cli.BoolFlag{
						Name:  "clean",
						Usage: "Drop and recreate every object so the destination schema exactly matches source (pg_dump --clean --if-exists). Destructive — wipes data in recreated tables. Without it, missing objects are created and existing ones are left untouched.",
					},
					&cli.BoolFlag{
						Name:    "no-safety",
						Aliases: []string{"ns"},
						Usage:   "Remove destination safety checks. Without passing this flag, only localhost is allowed.",
					},
					&cli.BoolFlag{
						Name:    "skip-confirmation",
						Aliases: []string{"sc"},
						Usage:   "Skip confirmation prompt. Useful for scripting.",
					},
					&cli.BoolFlag{
						Name:    "dry-run",
						Aliases: []string{"dr"},
						Usage:   "Print the schema DDL that would be applied without touching the destination.",
					},
					&cli.BoolFlag{
						Name:    "quiet",
						Aliases: []string{"q"},
						Usage:   "Suppress the wrapper's status output (pg_dump/psql diagnostics still print).",
					},
				},
				Action: func(cCtx *cli.Context) error {
					return executeSchemaSync(cCtx, handler)
				},
			},
		},
	}
}

// executeSchemaSync resolves connections, runs the safety check, prompts for confirmation, and
// copies the schema from source to destination.
func executeSchemaSync(cCtx *cli.Context, handler *config.UserConfigHandler) error {
	requireConnections(handler)

	sourceName := cCtx.String("source")
	destName := cCtx.String("dest")
	if sourceName == "" {
		return usageError(cCtx, "--source is required")
	}
	if destName == "" {
		return usageError(cCtx, "--dest is required")
	}

	clean := cCtx.Bool("clean")
	dryRun := cCtx.Bool("dry-run")
	quiet := cCtx.Bool("quiet")
	noSafety := cCtx.Bool("no-safety")

	srcConn, dstConn, err := resolveConnections(handler, sourceName, destName)
	if err != nil {
		return fmt.Errorf("could not resolve connections: %w", err)
	}

	source, destination := setupDatasources(&srcConn, &dstConn)
	defer func() {
		_ = source.DB.Close(cCtx.Context)
		_ = destination.DB.Close(cCtx.Context)
	}()

	if !noSafety && !destination.IsLocalHost(cCtx.Context) {
		return fmt.Errorf("destination host %q is not localhost or 127.0.0.1 — pass --no-safety to override", dstConn.Host)
	}

	if !cCtx.Bool("skip-confirmation") {
		reader := bufio.NewReader(os.Stdin)
		for {
			printSchemaSyncBanner(source, &srcConn, &dstConn, clean, dryRun, noSafety)
			fmt.Print("Do you want to proceed? (yes/no): ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("could not read confirmation (%w) — pass --skip-confirmation for non-interactive use", err)
			}
			switch strings.ToLower(strings.TrimSpace(response)) {
			case "yes", "y":
				fmt.Println("Starting schema sync")
				goto proceed
			case "no", "n":
				fmt.Println("Schema sync cancelled")
				return nil
			default:
				fmt.Printf("Please answer yes or no (got %q)\n", strings.TrimSpace(response))
			}
		}
	proceed:
	}

	params := func(c config.ConnectionConfig) sync.SchemaSyncParams {
		return sync.SchemaSyncParams{
			Host:     c.Host,
			Port:     c.Port,
			User:     c.User,
			Password: c.Password,
			Database: c.Database,
			SSLMode:  c.SSLMode,
		}
	}

	if !quiet && !dryRun {
		fmt.Println("Applying schema to destination...")
	}
	err = sync.SchemaSync(cCtx.Context, params(srcConn), params(dstConn), sync.SchemaSyncOptions{Clean: clean, DryRun: dryRun}, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println("\nDry run — nothing was applied to the destination.")
	} else if !quiet {
		fmt.Println("Schema sync complete.")
	}
	return nil
}

// distinctSchemaCount returns the number of distinct schemas among the source's loaded tables.
func distinctSchemaCount(source *datasource.ReaderDataSource) int {
	seen := make(map[string]struct{}, len(source.Tables))
	for _, t := range source.Tables {
		seen[t.Schema] = struct{}{}
	}
	return len(seen)
}

// printSchemaSyncBanner renders the pre-sync confirmation banner in the matrix-green palette.
func printSchemaSyncBanner(source *datasource.ReaderDataSource, src, dst *config.ConnectionConfig, clean, dryRun, noSafety bool) {
	sep := bannerArtStyle.Render("=================================================================")

	fmt.Printf("\n%s\n%s\n", sep, bannerArtStyle.Render(bannerLogo))
	fmt.Println(bannerLabelStyle.Render("  Schema sync — copies the whole database schema (DDL)"))
	if dryRun {
		fmt.Println(bannerOnStyle.Render("  *** DRY RUN — no changes will be applied ***"))
	}
	fmt.Printf("%s %s                     %s %s\n",
		bannerLabelStyle.Render("Source:"),
		bannerTextStyle.Render(fmt.Sprintf("%s:%d/%s", src.Host, src.Port, src.Database)),
		bannerLabelStyle.Render("Destination:"),
		bannerTextStyle.Render(fmt.Sprintf("%s:%d/%s", dst.Host, dst.Port, dst.Database)))
	fmt.Println(bannerArtStyle.Render(bannerMatrix))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Source tables:"),
		bannerTextStyle.Render(fmt.Sprintf("%s across %d schema(s)", strconv.Itoa(len(source.Tables)), distinctSchemaCount(source))))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Clean (drop & recreate)?:"), styledBool(clean))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("No Safety?"), styledBool(noSafety))
	if clean {
		fmt.Println(bannerFailStyle.Render("  WARNING: --clean DROPs and recreates destination objects — their data will be wiped."))
	}
	fmt.Printf("%s\n", sep)
}
