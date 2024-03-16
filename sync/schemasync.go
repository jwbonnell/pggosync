package sync

import "os/exec"

func Dump() {
	cmd := exec.Command("pg_dump", "-Fc", "--verbose", "--schema-only", "--no-owner", "--no-acl")
}

func Restore() {

}
