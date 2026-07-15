package main

import (
	"fmt"
	"os"

	"github.com/jwbonnell/pggosync/cmd"
)

var build = "development"

func main() {
	if err := cmd.Execute(build, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
