package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type TableHistoryEntry struct {
	Table    string `json:"table"`
	Rows     int64  `json:"rows"`
	Strategy string `json:"strategy"`
	Error    string `json:"error,omitempty"`
}

type SyncHistoryEntry struct {
	Timestamp  time.Time           `json:"timestamp"`
	Source     string              `json:"source"`
	Dest       string              `json:"dest"`
	ConfigFile string              `json:"config_file"`
	Tables     []TableHistoryEntry `json:"tables"`
	TotalRows  int64               `json:"total_rows"`
	DryRun     bool                `json:"dry_run"`
	Error      string              `json:"error,omitempty"`
}

type SyncHistory struct {
	Entries []SyncHistoryEntry `json:"entries"`
}

const maxHistoryEntries = 20

func (uc *UserConfigHandler) historyPath() (string, error) {
	dir, err := uc.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

// LoadSyncHistory reads the history file from disk; returns an empty SyncHistory if the file does not exist.
func (uc *UserConfigHandler) LoadSyncHistory() (SyncHistory, error) {
	path, err := uc.historyPath()
	if err != nil {
		return SyncHistory{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SyncHistory{}, nil
		}
		return SyncHistory{}, err
	}
	var h SyncHistory
	if err = json.Unmarshal(raw, &h); err != nil {
		return SyncHistory{}, err
	}
	return h, nil
}

// SaveSyncHistory appends entry to the history file, trimming to the last maxHistoryEntries entries.
func (uc *UserConfigHandler) SaveSyncHistory(entry SyncHistoryEntry) error {
	h, err := uc.LoadSyncHistory()
	if err != nil {
		h = SyncHistory{}
	}
	h.Entries = append(h.Entries, entry)
	if len(h.Entries) > maxHistoryEntries {
		h.Entries = h.Entries[len(h.Entries)-maxHistoryEntries:]
	}
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	path, err := uc.historyPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
