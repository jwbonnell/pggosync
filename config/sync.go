package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type TableEntry struct {
	Table    string `yaml:"table"`
	Filter   string `yaml:"filter,omitempty"`
	Truncate *bool  `yaml:"truncate,omitempty"`
	Preserve *bool  `yaml:"preserve,omitempty"`
}

type Group struct {
	Tables []TableEntry `yaml:"tables"`
}

type SyncConfig struct {
	Description string           `yaml:"description"`
	Exclude     []string         `yaml:"exclude"`
	Groups      map[string]Group `yaml:"groups"`
}

// GetSyncConfig reads and unmarshals a sync config YAML file, returning a descriptive error on missing or invalid files.
func GetSyncConfig(syncConfigPath string) (SyncConfig, error) {
	var syncConfig SyncConfig

	raw, err := os.ReadFile(syncConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncConfig{}, fmt.Errorf("sync config file not found: %s", syncConfigPath)
		}
		return SyncConfig{}, fmt.Errorf("could not read sync config %s: %w", syncConfigPath, err)
	}

	if err = yaml.Unmarshal(raw, &syncConfig); err != nil {
		return SyncConfig{}, fmt.Errorf("invalid YAML in sync config %s: %w", syncConfigPath, err)
	}

	return syncConfig, nil
}
