package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
)

// BackupInfo describes one stored config backup.
type BackupInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"` // unix seconds
}

// Path returns the config file path.
func (m *Manager) Path() string { return m.path }

// BackupDir returns the directory where config backups are written.
func (m *Manager) BackupDir() string { return m.backupDir }

// Backup writes a timestamped copy of the current config into the backup dir
// and returns the new file's path. Callers may follow it with PruneBackups.
func (m *Manager) Backup() (string, error) {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("config-backup-%s.json", time.Now().Format("20060102-150405"))
	dst := filepath.Join(m.backupDir, name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// ListBackups returns the stored backups, newest first.
func (m *Manager) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if os.IsNotExist(err) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// PruneBackups keeps the newest `keep` backups and deletes the rest.
func (m *Manager) PruneBackups(keep int) error {
	if keep < 1 {
		keep = 1
	}
	list, err := m.ListBackups()
	if err != nil {
		return err
	}
	for i := keep; i < len(list); i++ {
		_ = os.Remove(filepath.Join(m.backupDir, list[i].Name))
	}
	return nil
}

// LastBackupTime returns the modification time of the newest backup.
func (m *Manager) LastBackupTime() (time.Time, bool) {
	list, err := m.ListBackups()
	if err != nil || len(list) == 0 {
		return time.Time{}, false
	}
	return time.Unix(list[0].ModTime, 0), true
}

// Import replaces the entire config with the supplied JSON. The current config
// is backed up first (safety net for this destructive action). The raw JSON
// must contain a "settings" object to be accepted as a Yata config.
func (m *Manager) Import(data []byte) error {
	// Sanity check: reject arbitrary JSON that isn't a Yata config.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := probe["settings"]; !ok {
		return fmt.Errorf("not a Yata config (missing \"settings\")")
	}
	var incoming models.Config
	if err := json.Unmarshal(data, &incoming); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Always back up the current config before overwriting it.
	if _, err := m.Backup(); err != nil {
		return fmt.Errorf("backup before import failed: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = incoming
	// Reuse the same defaulting the loader applies.
	if m.cfg.Server.Host == "" {
		m.cfg.Server.Host = "0.0.0.0"
	}
	if m.cfg.Server.Port == 0 {
		m.cfg.Server.Port = 8420
	}
	if m.cfg.Settings.ScrapeIntervalMinutes < 60 {
		m.cfg.Settings.ScrapeIntervalMinutes = 60
	}
	if m.cfg.Settings.QUIEnabledInstances == nil {
		m.cfg.Settings.QUIEnabledInstances = []int{}
	}
	if m.cfg.Trackers == nil {
		m.cfg.Trackers = []models.Tracker{}
	}
	normalizeBackup(&m.cfg.Settings)
	return m.saveLocked()
}
