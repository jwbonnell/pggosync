package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type PathHandler interface {
	UserConfigDir() (string, error)
}

type OSPathHandler struct{}

// UserConfigDir delegates to os.UserConfigDir to locate the OS-level user configuration directory.
func (ph OSPathHandler) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

type UserConfigHandler struct {
	PathHandler PathHandler
}

// NewUserConfigHandler creates a UserConfigHandler backed by the given PathHandler.
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

// configDir returns the pggosync-specific subdirectory inside the OS user config dir.
func (uc *UserConfigHandler) configDir() (string, error) {
	dir, err := uc.PathHandler.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pggosync"), nil
}

// InitConnection creates a placeholder connection file. It refuses to overwrite an
// existing connection so a stray `conn init` cannot clobber saved credentials.
func (uc *UserConfigHandler) InitConnection(name string) error {
	exists, err := uc.ConnectionExists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("connection %q already exists; choose a different name or edit it directly", name)
	}
	return uc.SaveConnection(name, defaultConnectionConfig(name))
}

// InitDefaultConnections creates the default "source"/"dest" connection pair. If that
// pair is already taken it steps to the next free suffix (source1/dest1, source2/dest2, …)
// so existing connections are never overwritten. It returns the names it created.
func (uc *UserConfigHandler) InitDefaultConnections() ([]string, error) {
	for i := 0; ; i++ {
		suffix := ""
		if i > 0 {
			suffix = strconv.Itoa(i)
		}
		source, dest := "source"+suffix, "dest"+suffix
		sourceExists, err := uc.ConnectionExists(source)
		if err != nil {
			return nil, err
		}
		destExists, err := uc.ConnectionExists(dest)
		if err != nil {
			return nil, err
		}
		if sourceExists || destExists {
			continue
		}
		if err := uc.SaveConnection(source, defaultConnectionConfig(source)); err != nil {
			return nil, err
		}
		if err := uc.SaveConnection(dest, defaultConnectionConfig(dest)); err != nil {
			return nil, err
		}
		return []string{source, dest}, nil
	}
}

// ConnectionExists reports whether a connection config with the given name is saved.
func (uc *UserConfigHandler) ConnectionExists(name string) (bool, error) {
	dir, err := uc.configDir()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(dir, fmt.Sprintf("%s.yaml", name)))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// DeleteConnection removes a saved connection's YAML file. It returns nil if the file does not exist.
func (uc *UserConfigHandler) DeleteConnection(name string) error {
	dir, err := uc.configDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, fmt.Sprintf("%s.yaml", name)))
	if err != nil && !os.IsNotExist(err) {
		return err
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
			return ConnectionConfig{}, fmt.Errorf("connection %q not found; run 'pggosync conn init %s' to create it", name, name)
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
	return atomicWriteFile(filepath.Join(dir, fmt.Sprintf("%s.yaml", name)), data, 0600)
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
		name := f.Name()
		// Only .yaml files are connections; skip reserved files (prefs.yaml) that share the dir.
		if !strings.HasSuffix(name, ".yaml") || name == PrefsFile {
			continue
		}
		// TrimSuffix (not SplitN on ".") so connection names containing dots stay visible.
		names = append(names, strings.TrimSuffix(name, ".yaml"))
	}
	return names, nil
}

// defaultConnectionConfig returns a placeholder config; names that suggest a local destination default to port 5445.
func defaultConnectionConfig(name string) ConnectionConfig {
	port := 5444
	if strings.HasPrefix(name, "dest") || name == "destination" || name == "local" {
		port = 5445
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
