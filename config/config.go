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
# Example: postgres://${USERNAME}:${PASSWORD}@${HOST}:${PORT}/${DATABASE}
source: postgres://source_user:source_pw@localhost:5437/postgres
destination: postgres://dest_user:dest_pw@localhost:5438/postgres

exclude:
  - products

groups:
  country:
    city: "where country_id = 10"
    store: "where city_id IN (SELECT city_id FROM city WHERE country_id = 10)"
    country: "where country_id = 10"

  country_var_1:
    city: "where country_id = {1}"
    store: "where city_id IN (SELECT city_id FROM city WHERE country_id = {1})"
    country: "where country_id = {1}"
`

type Config struct {
	Source      string                       `yaml:"source"`
	Destination string                       `yaml:"destination"`
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

type ConfigHandler struct {
	PathHandler PathHandler
}

func NewConfigHandler(path PathHandler) *ConfigHandler {
	return &ConfigHandler{
		PathHandler: path,
	}
}

func (c *ConfigHandler) InitConfig(name string) error {
	if err := c.SetDefault(name); err != nil {
		return err
	}

	if err := c.saveConfig(name, initialConfig); err != nil {
		return err
	}
	return nil
}

func (c *ConfigHandler) saveConfig(name string, configYaml string) error {
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

func (c *ConfigHandler) GetCurrentConfig() (Config, error) {
	def, err := c.GetDefault()
	if err != nil {
		return Config{}, err
	}

	return c.GetConfig(def)
}

func (c *ConfigHandler) GetConfig(name string) (Config, error) {
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

func (c *ConfigHandler) GetDefault() (string, error) {
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

func (c *ConfigHandler) SetDefault(def string) error {
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

func (c *ConfigHandler) ListConfigs() ([]string, error) {
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
