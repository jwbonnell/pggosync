package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PrefsFile is the general-settings file stored alongside connections in the user config dir.
const PrefsFile = "prefs.yaml"

// Prefs holds general, user-level settings that are not tied to a single connection,
// sync config, or profile. It is stored at <configDir>/prefs.yaml.
type Prefs struct {
	Include struct {
		// Paths are extra base directories searched for configs and profiles, in
		// addition to the project-local ./.pggosync and the user config dir. Each
		// path is expected to contain a "configs" and/or "profiles" subdirectory.
		Paths []string `yaml:"paths"`
	} `yaml:"include"`
}

// ConfigDir returns the pggosync-specific subdirectory inside the OS user config dir.
func (uc *UserConfigHandler) ConfigDir() (string, error) {
	return uc.configDir()
}

// prefsPath returns the path to the general-settings file.
func (uc *UserConfigHandler) prefsPath() (string, error) {
	dir, err := uc.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PrefsFile), nil
}

// LoadPrefs reads prefs.yaml, returning zero-value Prefs when the file does not exist.
func (uc *UserConfigHandler) LoadPrefs() (Prefs, error) {
	path, err := uc.prefsPath()
	if err != nil {
		return Prefs{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Prefs{}, nil
		}
		return Prefs{}, fmt.Errorf("could not read %s: %w", PrefsFile, err)
	}
	var prefs Prefs
	if err := yaml.Unmarshal(raw, &prefs); err != nil {
		return Prefs{}, fmt.Errorf("invalid YAML in %s: %w", PrefsFile, err)
	}
	return prefs, nil
}

// SavePrefs writes prefs.yaml, creating the config directory if needed.
func (uc *UserConfigHandler) SavePrefs(prefs Prefs) error {
	dir, err := uc.configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := yaml.Marshal(prefs)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, PrefsFile)
	return os.WriteFile(path, raw, 0600)
}

// IncludePaths returns the extra search base directories from prefs.yaml.
func (uc *UserConfigHandler) IncludePaths() ([]string, error) {
	prefs, err := uc.LoadPrefs()
	if err != nil {
		return nil, err
	}
	return prefs.Include.Paths, nil
}

// AddIncludePath validates and records an extra search base directory. The path must
// exist and contain a "configs" and/or "profiles" subdirectory. The stored value is
// absolute; adding an already-recorded path is a no-op.
func (uc *UserConfigHandler) AddIncludePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve %q: %w", path, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", abs)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	if !hasKindDir(abs) {
		return "", fmt.Errorf("%s must contain a \"configs\" and/or \"profiles\" subdirectory", abs)
	}

	prefs, err := uc.LoadPrefs()
	if err != nil {
		return "", err
	}
	for _, existing := range prefs.Include.Paths {
		if existing == abs {
			return abs, nil
		}
	}
	prefs.Include.Paths = append(prefs.Include.Paths, abs)
	if err := uc.SavePrefs(prefs); err != nil {
		return "", err
	}
	return abs, nil
}

// hasKindDir reports whether base contains a "configs" or "profiles" subdirectory.
func hasKindDir(base string) bool {
	for _, kind := range []string{"configs", "profiles"} {
		if info, err := os.Stat(filepath.Join(base, kind)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
