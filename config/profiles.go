package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type SyncProfile struct {
	Name             string    `json:"name"`
	Source           string    `json:"source"`
	Dest             string    `json:"dest"`
	ConfigFile       string    `json:"config_file"`
	Groups           []string  `json:"groups,omitempty"`
	RawTableInput    string    `json:"raw_table_input,omitempty"`
	Truncate         bool      `json:"truncate,omitempty"`
	Preserve         bool      `json:"preserve,omitempty"`
	DeferConstraints bool      `json:"defer_constraints,omitempty"`
	DisableTriggers  bool      `json:"disable_triggers,omitempty"`
	Concurrency      int       `json:"concurrency"`
	DryRun           bool      `json:"dry_run,omitempty"`
	NoSafety         bool      `json:"no_safety,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type SyncProfiles struct {
	Profiles []SyncProfile `json:"profiles"`
}

func (uc *UserConfigHandler) profilesPath() (string, error) {
	dir, err := uc.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles.json"), nil
}

// LoadProfiles reads profiles from disk; returns an empty SyncProfiles if the file does not exist.
func (uc *UserConfigHandler) LoadProfiles() (SyncProfiles, error) {
	path, err := uc.profilesPath()
	if err != nil {
		return SyncProfiles{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SyncProfiles{}, nil
		}
		return SyncProfiles{}, err
	}
	var p SyncProfiles
	if err = json.Unmarshal(raw, &p); err != nil {
		return SyncProfiles{}, err
	}
	return p, nil
}

// SaveProfile upserts a profile by name.
func (uc *UserConfigHandler) SaveProfile(p SyncProfile) error {
	profiles, err := uc.LoadProfiles()
	if err != nil {
		profiles = SyncProfiles{}
	}
	found := false
	for i, existing := range profiles.Profiles {
		if existing.Name == p.Name {
			profiles.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		profiles.Profiles = append(profiles.Profiles, p)
	}
	return uc.writeProfiles(profiles)
}

// DeleteProfile removes a profile by name.
func (uc *UserConfigHandler) DeleteProfile(name string) error {
	profiles, err := uc.LoadProfiles()
	if err != nil {
		return err
	}
	filtered := profiles.Profiles[:0]
	for _, p := range profiles.Profiles {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	profiles.Profiles = filtered
	return uc.writeProfiles(profiles)
}

func (uc *UserConfigHandler) writeProfiles(profiles SyncProfiles) error {
	data, err := json.Marshal(profiles)
	if err != nil {
		return err
	}
	path, err := uc.profilesPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
