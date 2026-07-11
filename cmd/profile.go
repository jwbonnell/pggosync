package cmd

import (
	"fmt"

	"github.com/jwbonnell/pggosync/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// profileCmd returns a CLI command with subcommands for listing, inspecting,
// validating, and running saved sync profiles.
func profileCmd(handler *config.UserConfigHandler) *cli.Command {
	return &cli.Command{
		Name:  "profile",
		Usage: "Manage and run sync profiles",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List profiles from the project and user profile directories",
				Action: func(cCtx *cli.Context) error {
					profiles, err := handler.LoadProfiles()
					if err != nil {
						return err
					}
					if len(profiles.Profiles) == 0 {
						fmt.Println("No profiles found. Save one from the TUI or drop a YAML file in ./.pggosync/profiles/.")
						return nil
					}
					for _, p := range profiles.Profiles {
						fmt.Printf("  %-24s %s → %s · %s\n", p.Name, p.Source, p.Dest, p.ConfigFile)
					}
					return nil
				},
			},
			{
				Name:      "show",
				Usage:     "Show a profile's contents",
				ArgsUsage: "<name-or-path>",
				Action: func(cCtx *cli.Context) error {
					name, err := requireSingleArg(cCtx, "profile name or path")
					if err != nil {
						return err
					}
					profile, err := handler.GetProfile(name)
					if err != nil {
						return err
					}
					out, err := yaml.Marshal(profile)
					if err != nil {
						return err
					}
					fmt.Printf("Profile: %s\n---\n%s", profile.Name, out)
					return nil
				},
			},
			{
				Name:      "validate",
				Usage:     "Validate a profile and check that its connections and sync config exist",
				ArgsUsage: "<name-or-path>",
				Action: func(cCtx *cli.Context) error {
					name, err := requireSingleArg(cCtx, "profile name or path")
					if err != nil {
						return err
					}
					profile, err := handler.GetProfile(name)
					if err != nil {
						return err
					}
					var problems []string
					if profile.Source == "" {
						problems = append(problems, "source connection is not set")
					} else if _, err := handler.GetConnection(profile.Source); err != nil {
						problems = append(problems, fmt.Sprintf("source connection: %v", err))
					}
					if profile.Dest == "" {
						problems = append(problems, "dest connection is not set")
					} else if _, err := handler.GetConnection(profile.Dest); err != nil {
						problems = append(problems, fmt.Sprintf("dest connection: %v", err))
					}
					if profile.ConfigFile == "" {
						problems = append(problems, "config_file is not set")
					} else if path, err := handler.ResolveSyncConfigPath(profile.ConfigFile); err != nil {
						problems = append(problems, fmt.Sprintf("sync config: %v", err))
					} else if _, err := config.GetSyncConfig(path); err != nil {
						problems = append(problems, fmt.Sprintf("sync config: %v", err))
					}

					if len(problems) > 0 {
						for _, p := range problems {
							fmt.Printf("  ✗ %s\n", p)
						}
						return fmt.Errorf("profile %q has %d problem(s)", profile.Name, len(problems))
					}
					fmt.Printf("Profile %q OK — connections and sync config are valid\n", profile.Name)
					return nil
				},
			},
			{
				Name:      "sync",
				Usage:     "Run a sync using a profile as defaults; explicit flags override profile values",
				ArgsUsage: "<name-or-path>",
				Flags:     syncFlags(),
				Action: func(cCtx *cli.Context) error {
					name, err := requireSingleArg(cCtx, "profile name or path")
					if err != nil {
						return err
					}
					requireConnections(handler)

					profile, err := handler.GetProfile(name)
					if err != nil {
						return err
					}

					// Start with flag values; the profile fills in any unset fields.
					args := cliArgsFromFlags(cCtx)
					sourceName := cCtx.String("source")
					destName := cCtx.String("dest")
					if !cCtx.IsSet("source") {
						sourceName = profile.Source
					}
					if !cCtx.IsSet("dest") {
						destName = profile.Dest
					}
					if !cCtx.IsSet("config") {
						args.SyncConfigPath = profile.ConfigFile
					}
					if !cCtx.IsSet("truncate") {
						args.Truncate = profile.Truncate
					}
					if !cCtx.IsSet("preserve") {
						args.Preserve = profile.Preserve
					}
					if !cCtx.IsSet("defer-constraints") {
						args.DeferConstraints = profile.DeferConstraints
					}
					if !cCtx.IsSet("disable-triggers") {
						args.DisableTriggers = profile.DisableTriggers
					}
					if !cCtx.IsSet("dry-run") {
						args.DryRun = profile.DryRun
					}
					if !cCtx.IsSet("no-safety") {
						args.NoSafety = profile.NoSafety
					}
					if !cCtx.IsSet("concurrency") {
						args.Concurrency = profile.Concurrency
					}
					if len(args.Groups) == 0 {
						args.Groups = profile.Groups
					}

					if sourceName == "" {
						return fmt.Errorf("profile %q has no source connection and --source was not passed", profile.Name)
					}
					if destName == "" {
						return fmt.Errorf("profile %q has no dest connection and --dest was not passed", profile.Name)
					}
					if args.SyncConfigPath == "" {
						return fmt.Errorf("profile %q has no config_file and --config was not passed", profile.Name)
					}

					return executeSync(cCtx, handler, sourceName, destName, args)
				},
			},
		},
	}
}
