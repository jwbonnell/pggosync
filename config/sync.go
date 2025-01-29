package config

import (
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

type SyncConfig struct {
	Description string                       `yaml:"description"`
	Exclude     []string                     `yaml:"exclude"`
	Groups      map[string]map[string]string `yaml:"groups"`
}

func GetSyncConfig(syncConfigPath string) (SyncConfig, error) {
	var (
		syncConfig SyncConfig
		raw        []byte
	)

	raw, err := os.ReadFile(syncConfigPath)
	if err != nil {
		//Found a config file but could not read it
		log.Fatal(err)
	}

	if err = yaml.Unmarshal(raw, &syncConfig); err != nil {
		log.Fatalf("config.GetConfig: %v", err)
	}

	return syncConfig, nil
}
