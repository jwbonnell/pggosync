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

// Defaults holds the names of the default source and destination connections.
type Defaults struct {
	Source string `yaml:"source"`
	Dest   string `yaml:"dest"`
}

func (uc *UserConfigHandler) configDir() (string, error) {
	dir, err := uc.PathHandler.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pggosync"), nil
}

// InitConnection creates a placeholder connection file and, if no defaults are
// set yet, sets both source and destination defaults to this connection.
func (uc *UserConfigHandler) InitConnection(name string) error {
	if err := uc.SaveConnection(name, defaultConnectionConfig(name)); err != nil {
		return err
	}
	// Set as defaults only when none exist yet.
	if _, err := uc.GetDefaults(); err != nil {
		_ = uc.SetDefaults(name, name)
	}
	return nil
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

// ListConnections returns the names of all saved connections (excluding the
// reserved defaults file).
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
		// Skip the defaults file.
		if f.Name() == "defaults.yaml" {
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

// GetDefaults returns the saved source/dest connection names.
func (uc *UserConfigHandler) GetDefaults() (Defaults, error) {
	dir, err := uc.configDir()
	if err != nil {
		return Defaults{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "defaults.yaml"))
	if err != nil {
		return Defaults{}, fmt.Errorf("no defaults set; run 'pggosync config default --source <name> --dest <name>'")
	}
	var d Defaults
	if err = yaml.Unmarshal(raw, &d); err != nil {
		return Defaults{}, fmt.Errorf("could not parse defaults file: %w", err)
	}
	return d, nil
}

// SetDefaults saves the default source and destination connection names.
func (uc *UserConfigHandler) SetDefaults(source, dest string) error {
	dir, err := uc.configDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(Defaults{Source: source, Dest: dest})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "defaults.yaml"), data, 0600)
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
