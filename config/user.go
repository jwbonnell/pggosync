package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PathHandler interface {
	UserConfigDir() (string, error)
}

type OSPathHandler struct{}

func (ph OSPathHandler) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

type UserConfigHandler struct {
	PathHandler PathHandler
}

func NewUserConfigHandler(pathHandler PathHandler) *UserConfigHandler {
	return &UserConfigHandler{
		PathHandler: pathHandler,
	}
}

type UserConfig struct {
	Source      DBConnection `yaml:"source"`
	Destination DBConnection `yaml:"destination"`
}

type DBConnection struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

func (uc *UserConfigHandler) InitConfig(name string) error {
	if err := uc.SetDefault(name); err != nil {
		return err
	}

	if err := uc.saveConfig(name, getInitialUserConfig()); err != nil {
		return err
	}
	return nil
}

func (uc *UserConfigHandler) GetDefault() (string, error) {
	var def string
	dir, err := uc.PathHandler.UserConfigDir()
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

func (uc *UserConfigHandler) SetDefault(def string) error {
	dir, err := uc.PathHandler.UserConfigDir()
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

func (uc *UserConfigHandler) saveConfig(name string, configYaml UserConfig) error {
	dir, err := uc.PathHandler.UserConfigDir()
	configPath := filepath.Join(dir, "pggosync", fmt.Sprintf("%s.yaml", name))

	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}

	yamlBytes, err := yaml.Marshal(configYaml)
	if err != nil {
		return err
	}

	if err = os.WriteFile(configPath, yamlBytes, 0600); err != nil {
		return err
	}
	return nil
}

func (uc *UserConfigHandler) GetCurrentConfig() (UserConfig, error) {
	def, err := uc.GetDefault()
	if err != nil {
		return UserConfig{}, err
	}

	return uc.GetConfig(def)
}

func (uc *UserConfigHandler) GetConfig(name string) (UserConfig, error) {
	dir, err := uc.PathHandler.UserConfigDir()
	if err != nil {
		return UserConfig{}, err
	}

	var (
		configPath string
		config     UserConfig
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

func (uc *UserConfigHandler) ListConfigs() ([]string, error) {
	dir, err := uc.PathHandler.UserConfigDir()
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

func getInitialUserConfig() UserConfig {
	return UserConfig{
		Source: DBConnection{
			Host:     "localhost",
			Port:     "5432",
			Database: "postgres",
			User:     "source_user",
			Password: "source_pw",
		},
		Destination: DBConnection{
			Host:     "localhost",
			Port:     "5433",
			Database: "postgres",
			User:     "dest_user",
			Password: "dest_pw",
		},
	}
}
