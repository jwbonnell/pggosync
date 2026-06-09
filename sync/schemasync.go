package sync

import "os/exec"

// Dump invokes pg_dump with --schema-only; connection must be configured via environment variables or pgpass.
func Dump() ([]byte, error) {
	cmd := exec.Command("pg_dump", "-Fc", "--verbose", "--schema-only", "--no-owner", "--no-acl")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// Restore is a placeholder; not yet implemented.
func Restore() {

}
