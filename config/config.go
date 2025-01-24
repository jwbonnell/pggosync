package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const initialConfig string = `
source:
	host: localhost
	port: 5432
	database: postgres
	user: source_user
	password: source_pw

destination:
	host: localhost
	port: 5433
	database: postgres
	user: dest_user
	password: dest_pw
`

type Config struct {
	Source struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Database string `yaml:"database"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"source"`
	Destination struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Database string `yaml:"database"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"destination"`
}

type SyncConfig struct {
	Description string                       `yaml:"description"`
	Exclude     []string                     `yaml:"exclude"`
	Groups      map[string]map[string]string `yaml:"groups"`
}

type PathHandler interface {
	UserConfigDir() (string, error)
}

type OSPathHandler struct{}

func (ph OSPathHandler) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

type Handler struct {
	PathHandler PathHandler
}

func NewConfigHandler(path PathHandler) *Handler {
	return &Handler{
		PathHandler: path,
	}
}

func (c *Handler) InitConfig(name string) error {
	if err := c.SetDefault(name); err != nil {
		return err
	}

	if err := c.saveConfig(name, initialConfig); err != nil {
		return err
	}
	return nil
}

func (c *Handler) saveConfig(name string, configYaml string) error {
	dir, err := c.PathHandler.UserConfigDir()
	configPath := filepath.Join(dir, "pggosync", fmt.Sprintf("%s.yaml", name))

	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}

	if err = os.WriteFile(configPath, []byte(configYaml), 0600); err != nil {
		return err
	}
	return nil
}

func (c *Handler) GetCurrentConfig() (Config, error) {
	def, err := c.GetDefault()
	if err != nil {
		return Config{}, err
	}

	return c.GetConfig(def)
}

func (c *Handler) GetConfig(name string) (Config, error) {
	dir, err := c.PathHandler.UserConfigDir()
	if err != nil {
		return Config{}, err
	}

	var (
		configPath string
		config     Config
		raw        []byte
	)

	configPath = filepath.Join(dir, "pggosync", fmt.Sprintf("%s.yaml", name))
	raw, err = os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		//Found a config file but could not read it
		log.Fatal(err)
	}

	if err = yaml.Unmarshal(raw, &config); err != nil {
		log.Fatalf("config.GetConfig: %v", err)
	}

	return config, nil
}

func (c *Handler) GetSyncConfig(syncConfigPath string) (SyncConfig, error) {
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

func (c *Handler) GetDefault() (string, error) {
	var def string
	dir, err := c.PathHandler.UserConfigDir()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(dir, "pggosync", "default")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	def = string(raw)
	return def, nil
}

func (c *Handler) SetDefault(def string) error {
	dir, err := c.PathHandler.UserConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(dir, "pggosync", "default")
	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}

	if err = os.WriteFile(configPath, []byte(def), 0600); err != nil {
		return err
	}

	return nil
}

func (c *Handler) ListConfigs() ([]string, error) {
	dir, err := c.PathHandler.UserConfigDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "pggosync")
	files, err := os.ReadDir(configPath)
	if err != nil {
		return nil, err
	}

	var configs []string
	for _, f := range files {
		if f.Name() == "default" {
			continue
		}

		parts := strings.Split(f.Name(), ".")
		switch {
		case len(parts) != 2:
			return nil, fmt.Errorf("listConfigs: invalid config file name: %s", f.Name())
		case parts[1] != "yaml":
			return nil, fmt.Errorf("listConfigs: invalid config file type: %s", f.Name())
		default:
			configs = append(configs, parts[0])
		}
	}

	return configs, nil
}
