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
			Usage:   "Truncates or deletes all rows from table before syncing. Delete all happens when --defer-constraints is passed. Cannot be combined with --preserve.",
		},
		&cli.BoolFlag{
			Name:    "cascade",
			Aliases: []string{"ca"},
			Usage:   "Use TRUNCATE ... CASCADE, which also empties tables with a foreign key to the target. Without this, TRUNCATE errors on referenced tables. Only applies to the --truncate path.",
		},
		&cli.BoolFlag{
			Name:    "preserve",
			Aliases: []string{"p"},
			Usage:   "Preserve existing tables data. Uses insert on conflict do nothing. Without this flag, an upsert is performed. Cannot be combined with --truncate.",
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
		&cli.IntFlag{
			Name:    "buffer-size",
			Aliases: []string{"bs"},
			Value:   32,
			Usage:   "Per-table prefetch buffer cap in MiB (peak memory ≈ concurrency × this).",
		},
		&cli.BoolFlag{
			Name:    "verify",
			Aliases: []string{"vf"},
			Usage:   "After a successful sync, re-count each table on source and destination and fail if they don't match. Row-count sanity check only — not a value/checksum comparison.",
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
		Verify:           cCtx.Bool("verify"),
		Concurrency:      cCtx.Int("concurrency"),
		BufferSize:       cCtx.Int("buffer-size"),
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
	// Checked here (not per command) so profile-supplied values merged with explicit
	// flags are covered too. Truncate replaces destination rows, preserve keeps them —
	// there is no sensible meaning for both.
	if args.Truncate && args.Preserve {
		return fmt.Errorf("--truncate and --preserve cannot be combined — choose one strategy (use per-table truncate/preserve overrides in the sync config to mix strategies within a run)")
	}
	if args.Concurrency < 1 {
		args.Concurrency = 1
	}
	// Non-positive (including a legacy profile with no buffer_size) falls back to the 32 MiB default.
	if args.BufferSize < 1 {
		args.BufferSize = 32
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
				// EOF here usually means input was piped without an answer; point at the scripting flag.
				return fmt.Errorf("could not read confirmation (%w) — pass --skip-confirmation for non-interactive use", err)
			}

			switch strings.ToLower(strings.TrimSpace(response)) {
			case "yes", "y":
				fmt.Println("Starting sync")
				goto proceed
			case "no", "n":
				fmt.Println("Sync cancelled")
				return nil
			case "more", "m":
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
				// Re-prompt on unrecognized input rather than aborting the whole command.
				fmt.Printf("Please answer yes, no, or more (got %q)\n", strings.TrimSpace(response))
			}
		}
	proceed:
	}

	if _, err = sync.Sync(cCtx.Context, args.DeferConstraints, args.DisableTriggers, args.Quiet, args.DryRun, args.Concurrency, args.BufferSize<<20, tasks, source, destination, os.Stdout); err != nil {
		return err
	}

	if args.Verify {
		if args.DryRun {
			// A dry run rolls the transaction back, so the destination is unchanged — there is
			// nothing meaningful to verify against the source slice.
			fmt.Println("Skipping verification (dry run committed no changes).")
			return nil
		}
		if !args.Quiet {
			fmt.Println("\nVerifying row counts...")
		}
		vr := sync.Verify(cCtx.Context, tasks, source, destination)
		printVerifyResults(vr)
		if !vr.OK() {
			// The sync already committed; verify runs afterwards, so this cannot roll anything
			// back. Return an error so scripts and CI see a non-zero exit.
			return fmt.Errorf("verification failed — the sync committed, but destination row counts do not match the source (see above)")
		}
	}

	return nil
}

// printVerifyResults renders the post-sync row-count verification table in the matrix palette.
func printVerifyResults(vr sync.VerifyResult) {
	for _, tv := range vr.Tables {
		var status, detail string
		switch {
		case tv.Err != nil:
			status = bannerFailStyle.Render("ERROR")
			detail = tv.Err.Error()
		case tv.OK:
			status = bannerOnStyle.Render("OK")
			detail = tv.Detail
		default:
			status = bannerFailStyle.Render("FAIL")
			detail = tv.Detail
		}
		fmt.Printf("  %-40s [%s] %s\n", tv.Table, status, detail)
	}
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
