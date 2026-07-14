package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/jwbonnell/pggosync/sync/data"
	"github.com/urfave/cli/v2"
)

// syncFlags returns the flag set shared by `run` and `profile sync`.
func syncFlags() []cli.Flag {
	return []cli.Flag{
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
			Name:    "truncate",
			Aliases: []string{"tr"},
			Usage:   "Truncates or deletes all rows from table before syncing. Delete all happens when --defer-constraints is passed.",
		},
		&cli.BoolFlag{
			Name:    "cascade",
			Aliases: []string{"ca"},
			Usage:   "Use TRUNCATE ... CASCADE, which also empties tables with a foreign key to the target. Without this, TRUNCATE errors on referenced tables. Only applies to the --truncate path.",
		},
		&cli.BoolFlag{
			Name:    "preserve",
			Aliases: []string{"p"},
			Usage:   "Preserve existing tables data. Uses insert on conflict do nothing. Without this flag, an upsert is performed. Ignored if --truncate is passed.",
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
			Name:    "defer-constraints",
			Aliases: []string{"dc"},
			Usage:   "Defer constraints on destination database",
		},
		&cli.BoolFlag{
			Name:    "disable-triggers",
			Aliases: []string{"dt"},
			Usage:   "Disable triggers on destination database",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Sync config name or path to a sync config file.",
		},
		&cli.StringSliceFlag{
			Name:    "group",
			Aliases: []string{"g"},
			Usage:   "Groups to sync. Repeatable.",
		},
		&cli.StringSliceFlag{
			Name:    "table",
			Aliases: []string{"t"},
			Usage:   "Tables to sync. Format: schema.table[:filter][:col1=rule1,col2=rule2]. Repeatable.",
		},
		&cli.StringSliceFlag{
			Name:    "exclude",
			Aliases: []string{"e"},
			Usage:   "Tables to exclude. Repeatable.",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Suppress per-table progress output.",
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			Aliases: []string{"dr"},
			Usage:   "Simulate the sync without committing changes.",
		},
		&cli.IntFlag{
			Name:    "concurrency",
			Aliases: []string{"con"},
			Value:   1,
			Usage:   "Number of source tables to pre-fetch concurrently.",
		},
	}
}

// cliArgsFromFlags collects the shared sync flags into a CLIArgs struct.
func cliArgsFromFlags(cCtx *cli.Context) opts.CLIArgs {
	return opts.CLIArgs{
		Truncate:         cCtx.Bool("truncate"),
		Cascade:          cCtx.Bool("cascade"),
		Preserve:         cCtx.Bool("preserve"),
		NoSafety:         cCtx.Bool("no-safety"),
		SkipConfirmation: cCtx.Bool("skip-confirmation"),
		Quiet:            cCtx.Bool("quiet"),
		DryRun:           cCtx.Bool("dry-run"),
		Concurrency:      cCtx.Int("concurrency"),
		DeferConstraints: cCtx.Bool("defer-constraints"),
		DisableTriggers:  cCtx.Bool("disable-triggers"),
		SyncConfigPath:   cCtx.String("config"),
		Groups:           cCtx.StringSlice("group"),
		Tables:           cCtx.StringSlice("table"),
		Excluded:         cCtx.StringSlice("exclude"),
	}
}

// runCmd returns the CLI command that runs a sync with explicitly provided options.
func runCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run a sync between two databases",
		Flags: syncFlags(),
		Action: func(cCtx *cli.Context) error {
			requireConnections(handler)

			args := cliArgsFromFlags(cCtx)
			sourceName := cCtx.String("source")
			destName := cCtx.String("dest")

			if sourceName == "" {
				return usageError(cCtx, "--source is required")
			}
			if destName == "" {
				return usageError(cCtx, "--dest is required")
			}
			if args.SyncConfigPath == "" {
				return usageError(cCtx, "--config is required")
			}

			return executeSync(cCtx, handler, sourceName, destName, args)
		},
	}
}

// executeSync resolves connections and tasks, prompts for confirmation, and runs the sync.
// Shared by `run` and `profile sync`.
func executeSync(cCtx *cli.Context, handler *config.UserConfigHandler, sourceName, destName string, args opts.CLIArgs) error {
	if args.Concurrency < 1 {
		args.Concurrency = 1
	}
	srcConn, dstConn, err := resolveConnections(handler, sourceName, destName)
	if err != nil {
		return fmt.Errorf("could not resolve connections: %w", err)
	}

	configPath, err := handler.ResolveSyncConfigPath(args.SyncConfigPath)
	if err != nil {
		return fmt.Errorf("could not resolve sync config: %w", err)
	}
	sc, err := config.GetSyncConfig(configPath)
	if err != nil {
		return err
	}

	source, destination := setupDatasources(&srcConn, &dstConn)
	defer func() {
		_ = source.DB.Close(cCtx.Context)
		_ = destination.DB.Close(cCtx.Context)
	}()

	if !args.NoSafety && !destination.IsLocalHost(cCtx.Context) {
		return fmt.Errorf("destination host %q is not localhost or 127.0.0.1 — pass --no-safety to override", dstConn.Host)
	}

	var excluded []string
	if len(sc.Exclude) > 0 {
		excluded = append(args.Excluded, sc.Exclude...)
	} else {
		excluded = args.Excluded
	}
	excludedTables, err := opts.ProcessExcludedArgs(excluded)
	if err != nil {
		return fmt.Errorf("failed to process --exclude: %w", err)
	}

	resolver := sync.NewTaskResolver(source, destination, sc.Groups, args.Truncate, args.Cascade, args.Preserve, args.DeferConstraints, args.DisableTriggers, excludedTables)
	tasks, err := resolver.Resolve(cCtx.Context, args.Groups, args.Tables)
	if err != nil {
		return err
	}

	if !args.SkipConfirmation {
		reader := bufio.NewReader(os.Stdin)
		for {
			printSyncBanner(sc, &srcConn, &dstConn, args, len(tasks))

			fmt.Print("Do you want to proceed? (yes/no/more): ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("error reading input: %w", err)
			}

			switch strings.TrimSpace(response) {
			case "yes":
				fmt.Println("Starting sync")
				goto proceed
			case "no":
				fmt.Println("Sync cancelled")
				return nil
			case "more":
				fmt.Printf("\nTables to sync (%d):\n", len(tasks))
				for _, t := range tasks {
					strategy := "upsert"
					if t.Truncate && !t.Preserve {
						strategy = "truncate"
					} else if t.Preserve {
						strategy = "preserve"
					}
					rowInfo := ""
					if t.Truncate && !t.Preserve {
						rowInfo = fmt.Sprintf(" — %s dest rows will be deleted", sync.FormatCount(t.DestRowCount))
					}
					scrubInfo := ""
					if len(t.ScrubRules) > 0 {
						labels := make([]string, len(t.ScrubRules))
						for j, r := range t.ScrubRules {
							labels[j] = fmt.Sprintf("%s=%s", r.Column, data.RuleLabel(r.Rule))
						}
						scrubInfo = fmt.Sprintf("  scrub: [%s]", strings.Join(labels, ", "))
					}
					fmt.Printf("  %-40s [%s]%s%s\n", t.FullName(), strategy, rowInfo, scrubInfo)
				}
				fmt.Println()
			default:
				return fmt.Errorf("invalid input %q — expected yes, no, or more", strings.TrimSpace(response))
			}
		}
	proceed:
	}

	if _, err = sync.Sync(cCtx.Context, args.DeferConstraints, args.DisableTriggers, args.Quiet, args.DryRun, args.Concurrency, tasks, source, destination, os.Stdout); err != nil {
		return err
	}

	return nil
}

// bannerLogo and bannerMatrix hold the ASCII art for the confirmation banner. They are
// styled with bannerArtStyle (matrix green) at print time so the CLI matches the TUI.
const bannerLogo = `   ___  __________     ____
  / _ \/ ___/ ___/__  / __/_ _____  ____
 / ___/ (_ / (_ / _ \_\ \/ // / _ \/ __/
/_/   \___/\___/\___/___/\_, /_//_/\__/
                        /___/`

const bannerMatrix = `                                              :.
                 ============================:::'.
                 ============================::::::.
                 ============================::::'
                                              :'`

// printSyncBanner renders the pre-sync confirmation banner in the matrix-green palette.
// lipgloss emits plain text automatically when stdout is not a TTY or NO_COLOR is set.
func printSyncBanner(sc config.SyncConfig, src, dst *config.ConnectionConfig, args opts.CLIArgs, tableCount int) {
	sep := bannerArtStyle.Render("=================================================================")

	fmt.Printf("\n%s\n%s\n", sep, bannerArtStyle.Render(bannerLogo))
	if args.DryRun {
		fmt.Println(bannerOnStyle.Render("  *** DRY RUN — no changes will be committed ***"))
	}
	fmt.Printf("%s %s\n",
		bannerLabelStyle.Render("Config Description:"),
		bannerTextStyle.Render(sc.Description))
	fmt.Printf("%s %s                     %s %s\n",
		bannerLabelStyle.Render("Source:"),
		bannerTextStyle.Render(fmt.Sprintf("%s:%d/%s", src.Host, src.Port, src.Database)),
		bannerLabelStyle.Render("Destination:"),
		bannerTextStyle.Render(fmt.Sprintf("%s:%d/%s", dst.Host, dst.Port, dst.Database)))
	fmt.Println(bannerArtStyle.Render(bannerMatrix))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Truncate?:"), styledBool(args.Truncate))
	if args.Truncate {
		fmt.Printf("%s %s\n", bannerLabelStyle.Render("Cascade?:"), styledBool(args.Cascade))
	}
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Preserve?:"), styledBool(args.Preserve))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Disable Triggers?:"), styledBool(args.DisableTriggers))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Defer Constraints?"), styledBool(args.DeferConstraints))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("No Safety?"), styledBool(args.NoSafety))
	fmt.Printf("%s %s\n", bannerLabelStyle.Render("Tables:"), bannerTextStyle.Render(strconv.Itoa(tableCount)))
	fmt.Printf("%s\n", sep)
}
