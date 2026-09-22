package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	settingsVersion         = 1
	defaultRecentFilesLimit = 10
	minRecentFilesLimit     = 3
	maxRecentFilesLimit     = 20
)

// AppSettings contains user-facing settings that are safe to expose to the UI.
type AppSettings struct {
	DefaultView         string `json:"defaultView"`
	RememberRecentFiles bool   `json:"rememberRecentFiles"`
	RecentFilesLimit    int    `json:"recentFilesLimit"`
}

// RecentFile is one recent Markdown document displayed by the Windows shell.
type RecentFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

type persistedState struct {
	Version             int         `json:"version"`
	Settings            AppSettings `json:"settings"`
	RecentFiles         []string    `json:"recentFiles,omitempty"`
	LastOpenDirectory   string      `json:"lastOpenDirectory,omitempty"`
	LastExportDirectory string      `json:"lastExportDirectory,omitempty"`
}

type stateStore struct {
	path string
	mu   sync.Mutex
}

func defaultStateStore() *stateStore {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return &stateStore{}
	}
	return &stateStore{path: filepath.Join(configDir, appName, "settings.json")}
}

func newStateStore(path string) *stateStore {
	return &stateStore{path: path}
}

func defaultPersistedState() persistedState {
	return persistedState{
		Version: settingsVersion,
		Settings: AppSettings{
			DefaultView:         "preview",
			RememberRecentFiles: true,
			RecentFilesLimit:    defaultRecentFilesLimit,
		},
	}
}

func normalizeSettings(value AppSettings) AppSettings {
	view := strings.ToLower(strings.TrimSpace(value.DefaultView))
	if view != "source" {
		view = "preview"
	}

	limit := value.RecentFilesLimit
	if limit < minRecentFilesLimit {
		limit = minRecentFilesLimit
	}
	if limit > maxRecentFilesLimit {
		limit = maxRecentFilesLimit
	}

	return AppSettings{
		DefaultView:         view,
		RememberRecentFiles: value.RememberRecentFiles,
		RecentFilesLimit:    limit,
	}
}

func normalizePersistedState(value persistedState) persistedState {
	if value.Version <= 0 {
		value.Version = settingsVersion
	}
	if strings.TrimSpace(value.Settings.DefaultView) == "" && value.Settings.RecentFilesLimit == 0 && !value.Settings.RememberRecentFiles {
		// A zero-value settings object from an older or incomplete file should use product defaults.
		value.Settings = defaultPersistedState().Settings
	} else {
		value.Settings = normalizeSettings(value.Settings)
	}
	if !value.Settings.RememberRecentFiles {
		value.RecentFiles = nil
	}
	value.RecentFiles = trimRecentFiles(value.RecentFiles, value.Settings.RecentFilesLimit)
	return value
}

func (s *stateStore) load() (persistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *stateStore) loadUnlocked() (persistedState, error) {
	defaults := defaultPersistedState()
	if s == nil || strings.TrimSpace(s.path) == "" {
		return defaults, nil
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("read settings: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		// Settings should never prevent document conversion. Recover with defaults;
		// the next successful settings/recent-file write will replace the bad file.
		return defaults, nil
	}
	return normalizePersistedState(state), nil
}

func (s *stateStore) save(state persistedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(normalizePersistedState(state))
}

func (s *stateStore) saveUnlocked(state persistedState) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

func (s *stateStore) update(update func(*persistedState)) (persistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadUnlocked()
	if err != nil {
		return defaultPersistedState(), err
	}
	update(&state)
	state = normalizePersistedState(state)
	if err := s.saveUnlocked(state); err != nil {
		return state, err
	}
	return state, nil
}

func (s *stateStore) settings() (AppSettings, error) {
	state, err := s.load()
	if err != nil {
		return defaultPersistedState().Settings, err
	}
	return state.Settings, nil
}

func (s *stateStore) saveSettings(settings AppSettings) (AppSettings, error) {
	state, err := s.update(func(state *persistedState) {
		state.Settings = normalizeSettings(settings)
		if !state.Settings.RememberRecentFiles {
			state.RecentFiles = nil
		}
	})
	return state.Settings, err
}

func (s *stateStore) rememberFile(path string) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve recent Markdown path: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)

	_, err = s.update(func(state *persistedState) {
		state.LastOpenDirectory = filepath.Dir(absolutePath)
		if !state.Settings.RememberRecentFiles {
			return
		}

		recent := make([]string, 0, len(state.RecentFiles)+1)
		recent = append(recent, absolutePath)
		for _, existing := range state.RecentFiles {
			if samePath(existing, absolutePath) {
				continue
			}
			recent = append(recent, existing)
		}
		state.RecentFiles = trimRecentFiles(recent, state.Settings.RecentFilesLimit)
	})
	return err
}

func (s *stateStore) recentFiles() ([]RecentFile, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	if !state.Settings.RememberRecentFiles {
		return []RecentFile{}, nil
	}

	result := make([]RecentFile, 0, len(state.RecentFiles))
	for _, path := range state.RecentFiles {
		info, statErr := os.Stat(path)
		available := statErr == nil && !info.IsDir()
		result = append(result, RecentFile{
			Path:      path,
			Name:      filepath.Base(path),
			Available: available,
		})
	}
	return result, nil
}

func (s *stateStore) clearRecentFiles() error {
	_, err := s.update(func(state *persistedState) {
		state.RecentFiles = nil
	})
	return err
}

func (s *stateStore) lastOpenDirectory() string {
	state, err := s.load()
	if err != nil {
		return ""
	}
	return existingDirectory(state.LastOpenDirectory)
}

func (s *stateStore) lastExportDirectory(fallback string) string {
	state, err := s.load()
	if err == nil {
		if dir := existingDirectory(state.LastExportDirectory); dir != "" {
			return dir
		}
	}
	return existingDirectory(fallback)
}

func (s *stateStore) rememberExportDirectory(path string) error {
	_, err := s.update(func(state *persistedState) {
		state.LastExportDirectory = filepath.Dir(path)
	})
	return err
}

func trimRecentFiles(paths []string, limit int) []string {
	if limit < minRecentFilesLimit {
		limit = minRecentFilesLimit
	}
	if limit > maxRecentFilesLimit {
		limit = maxRecentFilesLimit
	}
	if len(paths) <= limit {
		return paths
	}
	return paths[:limit]
}

func samePath(left, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	return strings.EqualFold(left, right)
}

func existingDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}
