package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/urfave/cli/v2"
)

// versionCmd returns a CLI command that prints the build version.
func versionCmd(build string) *cli.Command {
	cmd := cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "Print the pggosync version",
		Action: func(cCtx *cli.Context) error {
			fmt.Println(resolveVersion(build))
			return nil
		},
	}
	return &cmd
}

// resolveVersion prefers the ldflags-injected build string; when absent
// (e.g. `go install`), it falls back to the module version from build info.
func resolveVersion(build string) string {
	if build != "" && build != "development" {
		return build
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return build
}
