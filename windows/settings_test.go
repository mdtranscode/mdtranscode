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
	if !settings.CloseToTray {
		t.Fatal("CloseToTray should default to true")
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

func TestSettingsVersionOneMigrationDefaultsWatcherDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"version":1,"settings":{"defaultView":"source","rememberRecentFiles":true,"recentFilesLimit":5}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStateStore(path).load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != settingsVersion || loaded.Settings.DefaultView != "source" || loaded.Settings.RecentFilesLimit != 5 {
		t.Fatalf("older settings not preserved: %#v", loaded)
	}
	watcher := loaded.Settings.Watcher
	if watcher.Enabled || watcher.WatchFolder == "" || watcher.OutputFolder == "" || !watcher.Recursive || !watcher.ConvertExisting {
		t.Fatalf("watcher did not migrate to safe disabled defaults: %#v", watcher)
	}
}

func TestAppLegacySettingsSaveDoesNotDisableWatcher(t *testing.T) {
	store := newStateStore(filepath.Join(t.TempDir(), "settings.json"))
	prior := defaultPersistedState().Settings
	prior.Watcher.Paused = true
	// Keep the watcher disabled so the regression test doesn't rely on an
	// actual Documents directory or launch a background scanner.
	prior.Watcher.Enabled = false
	app := newApp(store, nil)
	if _, err := store.saveSettings(prior); err != nil {
		t.Fatal(err)
	}
	next, err := app.SaveSettings(AppSettings{DefaultView: "source", RememberRecentFiles: true, RecentFilesLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if next.Watcher != prior.Watcher {
		t.Fatalf("legacy UI overwrote watcher configuration: got %#v want %#v", next.Watcher, prior.Watcher)
	}
}

func TestVersionTwoSettingsMigrationPreservesWatcherAndEnablesSourceNotices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"version":2,"settings":{"defaultView":"source","rememberRecentFiles":true,"recentFilesLimit":10,"watcher":{"enabled":true,"paused":true,"watchFolder":"C:\\\\Markdown","outputFolder":"C:\\\\Word","recursive":true}}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	store := newStateStore(path)
	settings, err := store.settings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.UpdateNotifications {
		t.Fatal("existing users should receive opt-in source notice default")
	}
	if !settings.Watcher.Enabled || !settings.Watcher.Paused || settings.Watcher.WatchFolder == "" {
		t.Fatalf("version 2 watcher configuration lost: %#v", settings.Watcher)
	}
}

func TestCloseToTrayPreferencePersists(t *testing.T) {
	store := newStateStore(filepath.Join(t.TempDir(), "settings.json"))
	settings := defaultPersistedState().Settings
	settings.CloseToTray = false
	if _, err := store.saveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.settings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CloseToTray {
		t.Fatal("explicit CloseToTray=false was lost")
	}
	settings.CloseToTray = true
	if _, err := store.saveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.settings()
	if err != nil || !loaded.CloseToTray {
		t.Fatalf("CloseToTray=true not persisted: %#v, %v", loaded, err)
	}
}

func TestCloseToTrayMigrationDefaultsOnWithoutChangingOtherPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// v3 predates closeToTray. It should preserve the watcher and update settings.
	data := []byte(`{"version":3,"settings":{"defaultView":"source","rememberRecentFiles":true,"recentFilesLimit":5,"updateNotifications":false,"watcher":{"enabled":false,"paused":true}}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := newStateStore(path).settings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.CloseToTray || settings.UpdateNotifications || settings.DefaultView != "source" || !settings.Watcher.Paused {
		t.Fatalf("v3 migration changed preferences: %#v", settings)
	}
}

func TestPDFSettingsDefaultsAndUpgrade(t *testing.T) {
	state := defaultPersistedState()
	if state.Settings.PDF.PageSize != "Letter" || state.Settings.PDF.MarginTop != 19 || !state.Settings.PDF.Bookmarks || !state.Settings.PDF.PageNumbers {
		t.Fatalf("default PDF options: %+v", state.Settings.PDF)
	}
	path := filepath.Join(t.TempDir(), "prefs.json")
	old := `{"version":4,"settings":{"defaultView":"source","rememberRecentFiles":true,"recentFilesLimit":10,"closeToTray":true}}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := newStateStore(path).settings()
	if err != nil {
		t.Fatal(err)
	}
	if saved.PDF.PageSize != "Letter" || !saved.PDF.Bookmarks || saved.DefaultView != "source" {
		t.Fatalf("v4 PDF migration: %+v", saved)
	}
	saved.PDF.PageSize = "A4"
	saved.PDF.Orientation = "landscape"
	saved.PDF.MarginLeft = 12
	store := newStateStore(path)
	if _, err = store.saveSettings(saved); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.settings()
	if err != nil || reloaded.PDF.PageSize != "A4" || reloaded.PDF.MarginLeft != 12 {
		t.Fatalf("PDF settings persistence: %+v %v", reloaded.PDF, err)
	}
}
