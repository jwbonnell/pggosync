package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type SyncConfig struct {
	Description string                       `yaml:"description"`
	Exclude     []string                     `yaml:"exclude"`
	Groups      map[string]map[string]string `yaml:"groups"`
}

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
