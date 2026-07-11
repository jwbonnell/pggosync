package cmd

import (
	"fmt"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/jwbonnell/pggosync/sync/data"
	"github.com/urfave/cli/v2"
)

// configCmd returns a CLI command with subcommands for sync configs.
func configCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Manage sync configs",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List sync configs from the project and user config directories",
				Action: func(cCtx *cli.Context) error {
					configs, err := handler.ListSyncConfigs()
					if err != nil {
						return err
					}
					if len(configs) == 0 {
						fmt.Printf("No sync configs found in ./%s/configs or the user config directory.\n", config.ProjectConfigDir)
						return nil
					}
					for _, c := range configs {
						fmt.Printf("  %-24s [%s] %s\n", c.Name, c.Origin, c.Path)
					}
					return nil
				},
			},
			{
				Name:      "validate",
				Usage:     "Validate a sync config; with --source and --dest it also resolves tasks against both databases",
				ArgsUsage: "<name-or-path>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "source",
						Aliases: []string{"s"},
						Usage:   "Source connection name (enables validation against the databases).",
					},
					&cli.StringFlag{
						Name:    "dest",
						Aliases: []string{"d"},
						Usage:   "Destination connection name (enables validation against the databases).",
					},
					&cli.StringSliceFlag{
						Name:    "group",
						Aliases: []string{"g"},
						Usage:   "Limit validation to specific groups.",
					},
					&cli.StringSliceFlag{
						Name:    "table",
						Aliases: []string{"t"},
						Usage:   "Limit validation to specific tables.",
					},
					&cli.StringSliceFlag{
						Name:    "exclude",
						Aliases: []string{"e"},
						Usage:   "Tables to exclude.",
					},
					&cli.BoolFlag{
						Name:    "truncate",
						Aliases: []string{"tr"},
						Usage:   "Validate as if --truncate were passed (relaxes PK requirement).",
					},
					&cli.BoolFlag{
						Name:    "preserve",
						Aliases: []string{"p"},
						Usage:   "Validate as if --preserve were passed.",
					},
				},
				Action: func(cCtx *cli.Context) error {
					nameOrPath, err := requireSingleArg(cCtx, "sync config name or path")
					if err != nil {
						return err
					}
					path, err := handler.ResolveSyncConfigPath(nameOrPath)
					if err != nil {
						return err
					}
					sc, err := config.GetSyncConfig(path)
					if err != nil {
						return err
					}
					if err := validateSyncConfigStructure(sc); err != nil {
						return fmt.Errorf("%s: %w", path, err)
					}

					sourceName := cCtx.String("source")
					destName := cCtx.String("dest")
					if sourceName == "" && destName == "" {
						tableCount := 0
						for _, g := range sc.Groups {
							tableCount += len(g.Tables)
						}
						fmt.Printf("Config OK — %d group(s), %d table entries. Pass --source and --dest to validate against the databases.\n", len(sc.Groups), tableCount)
						return nil
					}
					if sourceName == "" || destName == "" {
						return fmt.Errorf("--source and --dest must be passed together")
					}
					return validateAgainstDatabases(cCtx, handler, sc, sourceName, destName)
				},
			},
		},
	}
}

// validateSyncConfigStructure checks group/table entries and scrub rule IDs without touching a database.
func validateSyncConfigStructure(sc config.SyncConfig) error {
	var problems []string
	for groupName, group := range sc.Groups {
		if len(group.Tables) == 0 {
			problems = append(problems, fmt.Sprintf("group %q has no tables", groupName))
		}
		for i, entry := range group.Tables {
			if entry.Table == "" {
				problems = append(problems, fmt.Sprintf("group %q table entry %d has no table name", groupName, i+1))
			}
			for _, rule := range entry.Scrub {
				if rule.Column == "" {
					problems = append(problems, fmt.Sprintf("group %q table %q has a scrub rule with no column", groupName, entry.Table))
				}
				if !data.IsValidRule(rule.Rule) {
					problems = append(problems, fmt.Sprintf("group %q table %q column %q has unknown scrub rule %q", groupName, entry.Table, rule.Column, rule.Rule))
				}
			}
		}
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Printf("  ✗ %s\n", p)
		}
		return fmt.Errorf("sync config has %d problem(s)", len(problems))
	}
	return nil
}

// validateAgainstDatabases resolves tasks against both databases and reports what would be synced.
func validateAgainstDatabases(cCtx *cli.Context, handler *config.UserConfigHandler, sc config.SyncConfig, sourceName, destName string) error {
	requireConnections(handler)

	srcConn, dstConn, err := resolveConnections(handler, sourceName, destName)
	if err != nil {
		return fmt.Errorf("could not resolve connections: %w", err)
	}

	source, destination := setupDatasources(&srcConn, &dstConn)
	defer func() {
		_ = source.DB.Close(cCtx.Context)
		_ = destination.DB.Close(cCtx.Context)
	}()

	var excluded []string
	if len(sc.Exclude) > 0 {
		excluded = append(cCtx.StringSlice("exclude"), sc.Exclude...)
	} else {
		excluded = cCtx.StringSlice("exclude")
	}
	excludedTables, err := opts.ProcessExcludedArgs(excluded)
	if err != nil {
		return fmt.Errorf("failed to process --exclude: %w", err)
	}

	truncate := cCtx.Bool("truncate")
	preserve := cCtx.Bool("preserve")
	resolver := sync.NewTaskResolver(source, destination, sc.Groups, truncate, preserve, false, false, excludedTables)
	tasks, err := resolver.Resolve(cCtx.Context, cCtx.StringSlice("group"), cCtx.StringSlice("table"))
	if err != nil {
		return err
	}

	fmt.Printf("Config OK — %d table(s) would be synced:\n", len(tasks))
	for _, t := range tasks {
		strategy := "upsert"
		if t.Truncate && !t.Preserve {
			strategy = "truncate"
		} else if t.Preserve {
			strategy = "preserve"
		}
		fmt.Printf("  %-40s [%s]\n", t.FullName(), strategy)
	}
	return nil
}
