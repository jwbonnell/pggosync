package main

import (
	"github.com/jwbonnell/pggosync/cmd"
	"os"
)

var build = "development"

func main() {
	cmd.Execute(build, os.Args)
}
