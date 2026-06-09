package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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

// ConnectionConfig holds credentials for a single database.
type ConnectionConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode,omitempty"`
}

func (uc *UserConfigHandler) configDir() (string, error) {
	dir, err := uc.PathHandler.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pggosync"), nil
}

// InitConnection creates a placeholder connection file.
func (uc *UserConfigHandler) InitConnection(name string) error {
	return uc.SaveConnection(name, defaultConnectionConfig(name))
}

// GetConnection loads a named connection config.
func (uc *UserConfigHandler) GetConnection(name string) (ConnectionConfig, error) {
	dir, err := uc.configDir()
	if err != nil {
		return ConnectionConfig{}, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.yaml", name))
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ConnectionConfig{}, fmt.Errorf("connection %q not found; run 'pggosync init %s' to create it", name, name)
		}
		return ConnectionConfig{}, fmt.Errorf("could not read connection %q: %w", name, err)
	}
	var conn ConnectionConfig
	if err = yaml.Unmarshal(raw, &conn); err != nil {
		return ConnectionConfig{}, fmt.Errorf("invalid YAML in connection %q: %w", name, err)
	}
	return conn, nil
}

// SaveConnection writes a connection config to disk.
func (uc *UserConfigHandler) SaveConnection(name string, conn ConnectionConfig) error {
	dir, err := uc.configDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(conn)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s.yaml", name)), data, 0600)
}

// ListConnections returns the names of all saved connections.
func (uc *UserConfigHandler) ListConnections() ([]string, error) {
	dir, err := uc.configDir()
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		parts := strings.SplitN(f.Name(), ".", 2)
		if len(parts) != 2 || parts[1] != "yaml" {
			continue
		}
		names = append(names, parts[0])
	}
	return names, nil
}

func defaultConnectionConfig(name string) ConnectionConfig {
	port := 5432
	if name == "dest" || name == "destination" || name == "local" {
		port = 5433
	}
	return ConnectionConfig{
		Host:     "localhost",
		Port:     port,
		Database: "postgres",
		User:     name + "_user",
		Password: "",
		SSLMode:  "disable",
	}
}
