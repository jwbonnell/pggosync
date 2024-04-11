package sync

import "os/exec"

func Dump() ([]byte, error) {
	cmd := exec.Command("pg_dump", "-Fc", "--verbose", "--schema-only", "--no-owner", "--no-acl")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return output, nil
}

func Restore() {

}
