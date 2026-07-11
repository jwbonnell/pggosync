package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectConfigDir is the project-local directory searched before the user config dir,
// so teams can commit shared sync configs and profiles to a repo.
const ProjectConfigDir = ".pggosync"

// FoundFile is a named YAML file discovered in one of the search directories.
type FoundFile struct {
	Name   string // filename without extension
	Path   string
	Origin string // "project" or "user"
}

// looksLikePath reports whether arg should be treated as a literal file path
// rather than a name to look up in the search directories.
func looksLikePath(arg string) bool {
	return strings.ContainsRune(arg, '/') ||
		strings.ContainsRune(arg, os.PathSeparator) ||
		strings.HasSuffix(arg, ".yaml") ||
		strings.HasSuffix(arg, ".yml")
}

// searchDirs returns the search directories for the given kind ("configs" or "profiles"):
// project-local first, then the user config dir, then any extra include paths from prefs.
//
// TODO: this ordering is also the precedence for duplicate bare names — earlier dirs win in
// resolveNamed and shadow later ones in listNamed. Include paths are currently lowest priority
// and there is no warning when a name is shadowed. Revisit whether include paths should be able
// to outrank the defaults and whether collisions should be surfaced. See CLAUDE.md.
func (uc *UserConfigHandler) searchDirs(kind string) ([]FoundFile, error) {
	dir, err := uc.configDir()
	if err != nil {
		return nil, err
	}
	dirs := []FoundFile{
		{Path: filepath.Join(ProjectConfigDir, kind), Origin: "project"},
		{Path: filepath.Join(dir, kind), Origin: "user"},
	}
	includePaths, err := uc.IncludePaths()
	if err != nil {
		return nil, err
	}
	for _, base := range includePaths {
		dirs = append(dirs, FoundFile{Path: filepath.Join(base, kind), Origin: "include"})
	}
	return dirs, nil
}

// resolveNamed resolves nameOrPath to an existing file: literal paths are used
// verbatim, bare names are searched as <dir>/<name>.yaml|.yml across dirs in order.
func resolveNamed(dirs []FoundFile, nameOrPath string) (string, error) {
	if looksLikePath(nameOrPath) {
		if _, err := os.Stat(nameOrPath); err != nil {
			return "", fmt.Errorf("file not found: %s", nameOrPath)
		}
		return nameOrPath, nil
	}
	var searched []string
	for _, d := range dirs {
		for _, ext := range []string{".yaml", ".yml"} {
			p := filepath.Join(d.Path, nameOrPath+ext)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
			searched = append(searched, p)
		}
	}
	return "", fmt.Errorf("%q not found; searched: %s", nameOrPath, strings.Join(searched, ", "))
}

// listNamed enumerates YAML files across dirs; earlier dirs shadow later ones by name.
func listNamed(dirs []FoundFile) ([]FoundFile, error) {
	seen := map[string]bool{}
	var found []FoundFile
	for _, d := range dirs {
		entries, err := os.ReadDir(d.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ext)
			if seen[name] {
				continue
			}
			seen[name] = true
			found = append(found, FoundFile{
				Name:   name,
				Path:   filepath.Join(d.Path, e.Name()),
				Origin: d.Origin,
			})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// ResolveSyncConfigPath resolves a sync config name or path to a file path.
func (uc *UserConfigHandler) ResolveSyncConfigPath(nameOrPath string) (string, error) {
	dirs, err := uc.searchDirs("configs")
	if err != nil {
		return "", err
	}
	return resolveNamed(dirs, nameOrPath)
}

// ResolveProfilePath resolves a profile name or path to a file path.
func (uc *UserConfigHandler) ResolveProfilePath(nameOrPath string) (string, error) {
	dirs, err := uc.searchDirs("profiles")
	if err != nil {
		return "", err
	}
	return resolveNamed(dirs, nameOrPath)
}

// ListSyncConfigs enumerates sync configs in the search directories.
func (uc *UserConfigHandler) ListSyncConfigs() ([]FoundFile, error) {
	dirs, err := uc.searchDirs("configs")
	if err != nil {
		return nil, err
	}
	return listNamed(dirs)
}

// ListProfileFiles enumerates profiles in the search directories.
func (uc *UserConfigHandler) ListProfileFiles() ([]FoundFile, error) {
	dirs, err := uc.searchDirs("profiles")
	if err != nil {
		return nil, err
	}
	return listNamed(dirs)
}
