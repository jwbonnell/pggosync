package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddIncludePath_ValidatesAndPersists(t *testing.T) {
	handler := newTestHandler(t)

	// A directory with a configs/ subdir is valid.
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "configs"), 0700))

	abs, err := handler.AddIncludePath(base)
	require.NoError(t, err)
	assert.Equal(t, base, abs)

	paths, err := handler.IncludePaths()
	require.NoError(t, err)
	require.Equal(t, []string{base}, paths)

	// Adding the same path again is a no-op, not a duplicate.
	_, err = handler.AddIncludePath(base)
	require.NoError(t, err)
	paths, err = handler.IncludePaths()
	require.NoError(t, err)
	assert.Len(t, paths, 1)
}

func TestAddIncludePath_Rejects(t *testing.T) {
	handler := newTestHandler(t)

	_, err := handler.AddIncludePath(filepath.Join(t.TempDir(), "missing"))
	assert.ErrorContains(t, err, "does not exist")

	// Exists but has neither configs/ nor profiles/.
	empty := t.TempDir()
	_, err = handler.AddIncludePath(empty)
	assert.ErrorContains(t, err, "subdirectory")
}

func TestSearchDirs_IncludePathsAreSearched(t *testing.T) {
	handler := newTestHandler(t)

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "configs", "extra.yaml"), "description: from-include")
	_, err := handler.AddIncludePath(base)
	require.NoError(t, err)

	got, err := handler.ResolveSyncConfigPath("extra")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "configs", "extra.yaml"), got)

	found, err := handler.ListSyncConfigs()
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "extra", found[0].Name)
	assert.Equal(t, "include", found[0].Origin)
}

func TestSearchDirs_UserShadowsInclude(t *testing.T) {
	handler := newTestHandler(t)
	userDir, _ := handler.PathHandler.UserConfigDir()
	writeFile(t, filepath.Join(userDir, "pggosync", "configs", "shared.yaml"), "description: user-level")

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "configs", "shared.yaml"), "description: from-include")
	_, err := handler.AddIncludePath(base)
	require.NoError(t, err)

	got, err := handler.ResolveSyncConfigPath("shared")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(userDir, "pggosync", "configs", "shared.yaml"), got, "user dir should shadow include paths")
}
