package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempPathHandler points the user config dir at a per-test temp directory.
type tempPathHandler struct{ dir string }

func (t tempPathHandler) UserConfigDir() (string, error) { return t.dir, nil }

// newTestHandler creates a handler with a temp user config dir and chdirs into
// a separate temp project dir so ./.pggosync lookups are isolated too.
func newTestHandler(t *testing.T) *UserConfigHandler {
	t.Helper()
	t.Chdir(t.TempDir())
	return NewUserConfigHandler(tempPathHandler{dir: t.TempDir()})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestResolveSyncConfigPath_LiteralPath(t *testing.T) {
	handler := newTestHandler(t)
	writeFile(t, "myconfig.yaml", "description: test")

	got, err := handler.ResolveSyncConfigPath("./myconfig.yaml")
	assert.NoError(t, err)
	assert.Equal(t, "./myconfig.yaml", got)

	_, err = handler.ResolveSyncConfigPath("./missing.yaml")
	assert.ErrorContains(t, err, "file not found")
}

func TestResolveSyncConfigPath_NameLookup(t *testing.T) {
	handler := newTestHandler(t)
	userDir, _ := handler.PathHandler.UserConfigDir()
	writeFile(t, filepath.Join(userDir, "pggosync", "configs", "shared.yaml"), "description: user-level")

	got, err := handler.ResolveSyncConfigPath("shared")
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(userDir, "pggosync", "configs", "shared.yaml"), got)
}

func TestResolveSyncConfigPath_ProjectShadowsUser(t *testing.T) {
	handler := newTestHandler(t)
	userDir, _ := handler.PathHandler.UserConfigDir()
	writeFile(t, filepath.Join(userDir, "pggosync", "configs", "shared.yaml"), "description: user-level")
	writeFile(t, filepath.Join(ProjectConfigDir, "configs", "shared.yml"), "description: project-level")

	got, err := handler.ResolveSyncConfigPath("shared")
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(ProjectConfigDir, "configs", "shared.yml"), got)
}

func TestResolveSyncConfigPath_MissingNameListsSearchedPaths(t *testing.T) {
	handler := newTestHandler(t)
	_, err := handler.ResolveSyncConfigPath("nope")
	assert.ErrorContains(t, err, "\"nope\" not found")
	assert.ErrorContains(t, err, ProjectConfigDir)
}

func TestListSyncConfigs_Dedupes(t *testing.T) {
	handler := newTestHandler(t)
	userDir, _ := handler.PathHandler.UserConfigDir()
	writeFile(t, filepath.Join(userDir, "pggosync", "configs", "a.yaml"), "")
	writeFile(t, filepath.Join(userDir, "pggosync", "configs", "b.yaml"), "")
	writeFile(t, filepath.Join(ProjectConfigDir, "configs", "b.yaml"), "")

	found, err := handler.ListSyncConfigs()
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, "a", found[0].Name)
	assert.Equal(t, "user", found[0].Origin)
	assert.Equal(t, "b", found[1].Name)
	assert.Equal(t, "project", found[1].Origin, "project entry should shadow the user one")
}

func TestProfiles_SaveLoadDelete(t *testing.T) {
	handler := newTestHandler(t)

	err := handler.SaveProfile(SyncProfile{Name: "dev", Source: "prod", Dest: "local", ConfigFile: "default", Concurrency: 2})
	require.NoError(t, err)

	profiles, err := handler.LoadProfiles()
	require.NoError(t, err)
	require.Len(t, profiles.Profiles, 1)
	assert.Equal(t, "dev", profiles.Profiles[0].Name)
	assert.Equal(t, "prod", profiles.Profiles[0].Source)
	assert.Equal(t, 2, profiles.Profiles[0].Concurrency)

	p, err := handler.GetProfile("dev")
	require.NoError(t, err)
	assert.Equal(t, "local", p.Dest)

	require.NoError(t, handler.DeleteProfile("dev"))
	profiles, err = handler.LoadProfiles()
	require.NoError(t, err)
	assert.Empty(t, profiles.Profiles)
}

func TestProfiles_NameComesFromFilename(t *testing.T) {
	handler := newTestHandler(t)
	writeFile(t, filepath.Join(ProjectConfigDir, "profiles", "team.yaml"), "source: prod\ndest: local\nconfig_file: default\n")

	p, err := handler.GetProfile("team")
	require.NoError(t, err)
	assert.Equal(t, "team", p.Name)
	assert.Equal(t, "prod", p.Source)
}

func TestProfiles_MigratesLegacyJSON(t *testing.T) {
	handler := newTestHandler(t)
	userDir, _ := handler.PathHandler.UserConfigDir()
	legacy := filepath.Join(userDir, "pggosync", "profiles.json")
	writeFile(t, legacy, `{"profiles":[{"name":"old","source":"prod","dest":"local","config_file":"default","truncate":true,"concurrency":3}]}`)

	profiles, err := handler.LoadProfiles()
	require.NoError(t, err)
	require.Len(t, profiles.Profiles, 1)
	assert.Equal(t, "old", profiles.Profiles[0].Name)
	assert.True(t, profiles.Profiles[0].Truncate)
	assert.Equal(t, 3, profiles.Profiles[0].Concurrency)

	assert.FileExists(t, filepath.Join(userDir, "pggosync", "profiles", "old.yaml"))
	assert.NoFileExists(t, legacy)
	assert.FileExists(t, legacy+".bak")

	// A second load must not re-run the migration or duplicate profiles.
	profiles, err = handler.LoadProfiles()
	require.NoError(t, err)
	assert.Len(t, profiles.Profiles, 1)
}
