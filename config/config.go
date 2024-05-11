package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
)

const initialConfig string = `
# Example: postgres://${USERNAME}:${PASSWORD}@${HOST}:${PORT}/${DATABASE}
source: 
destination: 

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
	Exclude []string                     `yaml:"exclude"`
	Groups  map[string]map[string]string `yaml:"groups"`
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
	err := c.SetDefault(name)
	if err != nil {
		return err
	}
	err = c.saveConfig(name, initialConfig)
	if err != nil {
		return err
	}
	return nil
}

func (c *ConfigHandler) saveConfig(name string, configYaml string) error {
	dir, err := c.PathHandler.UserConfigDir()
	configPath := filepath.Join(dir, "pggosync", fmt.Sprintf("%s.yaml", name))
	err = os.MkdirAll(filepath.Dir(configPath), 0700)
	if err != nil {
		return err
	}

	err = os.WriteFile(configPath, []byte(configYaml), 0600)
	if err != nil {
		return err
	}
	return nil
}

func (c *ConfigHandler) GetConfig(name string) (Config, error) {
	def, err := c.GetDefault()
	if err != nil {
		return Config{}, err
	}

	return c.getConfig(def)
}

func (c *ConfigHandler) getConfig(name string) (Config, error) {
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

	err = yaml.Unmarshal(raw, &config)
	if err != nil {
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
	if err != nil && !os.IsNotExist(err) {
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
	err = os.MkdirAll(filepath.Dir(configPath), 0700)
	if err != nil {
		return err
	}

	err = os.WriteFile(configPath, []byte(def), 0600)
	if err != nil {
		return err
	}

	return nil
}
