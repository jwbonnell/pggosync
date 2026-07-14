package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtomicWriteFile guards M6: the write lands with the requested contents and permissions,
// and overwriting replaces the contents in place.
func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.yaml")

	require.NoError(t, atomicWriteFile(path, []byte("first"), 0600))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first", string(got))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Overwrite in place.
	require.NoError(t, atomicWriteFile(path, []byte("second"), 0600))
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "second", string(got))

	// No leftover temp files in the directory.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}
