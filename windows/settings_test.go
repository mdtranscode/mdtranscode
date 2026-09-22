package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateStoreDefaults(t *testing.T) {
	store := newStateStore(filepath.Join(t.TempDir(), "settings.json"))
	settings, err := store.settings()
	if err != nil {
		t.Fatalf("settings returned error: %v", err)
	}
	if settings.DefaultView != "preview" {
		t.Fatalf("DefaultView = %q, want preview", settings.DefaultView)
	}
	if !settings.RememberRecentFiles {
		t.Fatal("RememberRecentFiles should default to true")
	}
	if settings.RecentFilesLimit != defaultRecentFilesLimit {
		t.Fatalf("RecentFilesLimit = %d, want %d", settings.RecentFilesLimit, defaultRecentFilesLimit)
	}
}

func TestStateStoreSettingsRoundTrip(t *testing.T) {
	store := newStateStore(filepath.Join(t.TempDir(), "state", "settings.json"))
	settings, err := store.saveSettings(AppSettings{
		DefaultView:         "source",
		RememberRecentFiles: false,
		RecentFilesLimit:    3,
	})
	if err != nil {
		t.Fatalf("saveSettings returned error: %v", err)
	}
	if settings.DefaultView != "source" || settings.RememberRecentFiles || settings.RecentFilesLimit != 3 {
		t.Fatalf("unexpected saved settings: %#v", settings)
	}

	reloaded, err := store.settings()
	if err != nil {
		t.Fatalf("reload settings returned error: %v", err)
	}
	if reloaded != settings {
		t.Fatalf("reloaded settings = %#v, want %#v", reloaded, settings)
	}
}

func TestStateStoreRecentFilesOrderDeduplicateAndLimit(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(filepath.Join(dir, "settings.json"))
	if _, err := store.saveSettings(AppSettings{
		DefaultView:         "preview",
		RememberRecentFiles: true,
		RecentFilesLimit:    3,
	}); err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 4)
	for i := range paths {
		paths[i] = filepath.Join(dir, string(rune('a'+i))+".md")
		if err := os.WriteFile(paths[i], []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := store.rememberFile(paths[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.rememberFile(paths[2]); err != nil {
		t.Fatal(err)
	}

	recent, err := store.recentFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 3 {
		t.Fatalf("len(recent) = %d, want 3", len(recent))
	}
	want := []string{paths[2], paths[3], paths[1]}
	for i, item := range recent {
		if !samePath(item.Path, want[i]) {
			t.Fatalf("recent[%d] = %q, want %q", i, item.Path, want[i])
		}
		if !item.Available {
			t.Fatalf("recent[%d] should be available", i)
		}
	}
}

func TestDisablingRecentFilesClearsPersistedHistory(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(filepath.Join(dir, "settings.json"))
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("# Report\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.rememberFile(path); err != nil {
		t.Fatal(err)
	}

	if _, err := store.saveSettings(AppSettings{
		DefaultView:         "preview",
		RememberRecentFiles: false,
		RecentFilesLimit:    10,
	}); err != nil {
		t.Fatal(err)
	}
	recent, err := store.recentFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 0 {
		t.Fatalf("recent files were not cleared: %#v", recent)
	}
}

func TestRecentFileReportsMissingPath(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(filepath.Join(dir, "settings.json"))
	missing := filepath.Join(dir, "missing.md")
	if err := store.rememberFile(missing); err != nil {
		t.Fatal(err)
	}
	recent, err := store.recentFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Available {
		t.Fatalf("missing recent file should be retained but unavailable: %#v", recent)
	}
}
