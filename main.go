package main

import (
	"os"

	"github.com/jwbonnell/pggosync/cmd"
)

var build = "development"

func main() {
	cmd.Execute(build, os.Args)
}
