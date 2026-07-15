package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SyncProfile is a named bundle of sync options stored as one YAML file per
// profile in a profiles search directory. Name comes from the filename stem
// and is never written into the file. The json tags exist only to read the
// legacy profiles.json during migration.
type SyncProfile struct {
	Name             string    `json:"name" yaml:"-"`
	Source           string    `json:"source" yaml:"source"`
	Dest             string    `json:"dest" yaml:"dest"`
	ConfigFile       string    `json:"config_file" yaml:"config_file"`
	Groups           []string  `json:"groups,omitempty" yaml:"groups,omitempty"`
	RawTableInput    string    `json:"raw_table_input,omitempty" yaml:"raw_table_input,omitempty"`
	Truncate         bool      `json:"truncate,omitempty" yaml:"truncate,omitempty"`
	Cascade          bool      `json:"cascade,omitempty" yaml:"cascade,omitempty"`
	Preserve         bool      `json:"preserve,omitempty" yaml:"preserve,omitempty"`
	DeferConstraints bool      `json:"defer_constraints,omitempty" yaml:"defer_constraints,omitempty"`
	DisableTriggers  bool      `json:"disable_triggers,omitempty" yaml:"disable_triggers,omitempty"`
	Concurrency      int       `json:"concurrency" yaml:"concurrency"`
	BufferSize       int       `json:"buffer_size,omitempty" yaml:"buffer_size,omitempty"`
	DryRun           bool      `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`
	Verify           bool      `json:"verify,omitempty" yaml:"verify,omitempty"`
	NoSafety         bool      `json:"no_safety,omitempty" yaml:"no_safety,omitempty"`
	CreatedAt        time.Time `json:"created_at" yaml:"created_at,omitempty"`
}

type SyncProfiles struct {
	Profiles []SyncProfile `json:"profiles"`
}

// userProfilesDir returns the user-level profiles directory (where profiles are saved).
func (uc *UserConfigHandler) userProfilesDir() (string, error) {
	dir, err := uc.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

// LoadProfiles returns all profiles found in the search directories,
// migrating the legacy profiles.json first if present.
func (uc *UserConfigHandler) LoadProfiles() (SyncProfiles, error) {
	if err := uc.migrateLegacyProfiles(); err != nil {
		return SyncProfiles{}, err
	}
	files, err := uc.ListProfileFiles()
	if err != nil {
		return SyncProfiles{}, err
	}
	var profiles SyncProfiles
	for _, f := range files {
		p, err := readProfileFile(f.Path)
		if err != nil {
			return SyncProfiles{}, err
		}
		p.Name = f.Name
		profiles.Profiles = append(profiles.Profiles, p)
	}
	return profiles, nil
}

// GetProfile resolves a profile name or path and loads it.
func (uc *UserConfigHandler) GetProfile(nameOrPath string) (SyncProfile, error) {
	if err := uc.migrateLegacyProfiles(); err != nil {
		return SyncProfile{}, err
	}
	path, err := uc.ResolveProfilePath(nameOrPath)
	if err != nil {
		return SyncProfile{}, err
	}
	p, err := readProfileFile(path)
	if err != nil {
		return SyncProfile{}, err
	}
	ext := filepath.Ext(path)
	p.Name = strings.TrimSuffix(filepath.Base(path), ext)
	return p, nil
}

// SaveProfile writes a profile to <user profiles dir>/<name>.yaml.
func (uc *UserConfigHandler) SaveProfile(p SyncProfile) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	dir, err := uc.userProfilesDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, p.Name+".yaml"), data, 0600)
}

// DeleteProfile removes a profile file from the user profiles directory.
// Project-local profiles are files in the repo and are not touched.
func (uc *UserConfigHandler) DeleteProfile(name string) error {
	dir, err := uc.userProfilesDir()
	if err != nil {
		return err
	}
	for _, ext := range []string{".yaml", ".yml"} {
		err := os.Remove(filepath.Join(dir, name+ext))
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// readProfileFile reads and unmarshals a single profile YAML file.
func readProfileFile(path string) (SyncProfile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SyncProfile{}, fmt.Errorf("could not read profile %s: %w", path, err)
	}
	var p SyncProfile
	if err = yaml.Unmarshal(raw, &p); err != nil {
		return SyncProfile{}, fmt.Errorf("invalid YAML in profile %s: %w", path, err)
	}
	return p, nil
}

// migrateLegacyProfiles splits a legacy profiles.json into one YAML file per
// profile and renames the original to profiles.json.bak. Existing profile
// files are never overwritten.
func (uc *UserConfigHandler) migrateLegacyProfiles() error {
	dir, err := uc.configDir()
	if err != nil {
		return err
	}
	legacyPath := filepath.Join(dir, "profiles.json")
	raw, err := os.ReadFile(legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var legacy SyncProfiles
	if err = json.Unmarshal(raw, &legacy); err != nil {
		return fmt.Errorf("could not migrate legacy %s: %w", legacyPath, err)
	}
	profilesDir, err := uc.userProfilesDir()
	if err != nil {
		return err
	}
	for _, p := range legacy.Profiles {
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(profilesDir, p.Name+".yaml")); statErr == nil {
			continue
		}
		if err = uc.SaveProfile(p); err != nil {
			return fmt.Errorf("could not migrate profile %q: %w", p.Name, err)
		}
	}
	return os.Rename(legacyPath, legacyPath+".bak")
}
