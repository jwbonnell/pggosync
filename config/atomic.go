package config

import (
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path by first writing a temp file in the same directory and then
// renaming it into place. A crash or a concurrent reader therefore never observes a half-written
// file, and the previous contents survive if the write fails partway through.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Remove the temp file if we return before a successful rename (no-op once renamed away).
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
