package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run scans against controlled timestamps so settling and retries are deterministic.
func newTestWatcher(t *testing.T, existing bool) (*folderWatcher, WatcherSettings, string) {
	t.Helper()
	root := t.TempDir()
	in, out := filepath.Join(root, "Markdown"), filepath.Join(root, "Converted")
	if err := os.MkdirAll(in, 0755); err != nil {
		t.Fatal(err)
	}
	w := newFolderWatcher(newStateStore(filepath.Join(root, "settings.json")))
	s := WatcherSettings{Enabled: true, WatchFolder: in, OutputFolder: out, Recursive: true, ConvertExisting: existing}
	normalized, err := validateWatcherSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	w.config, w.status.Running, w.status.Enabled = normalized, true, true
	return w, normalized, root
}

func putMarkdown(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func requireExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
}

func TestWatcherConvertsRecursiveAndNeverOverwrites(t *testing.T) {
	w, settings, _ := newTestWatcher(t, true)
	nested := filepath.Join(settings.WatchFolder, "Client", "Report.md")
	putMarkdown(t, nested, "# Version One\n")
	out := filepath.Join(settings.OutputFolder, "Client", "Report.docx")
	putMarkdown(t, out, "CUSTOM EXISTING CONTENT")

	now := time.Now()
	w.scan(now)
	if got := w.snapshot().Converted; got != 0 {
		t.Fatalf("converted unstable input: %d", got)
	}
	w.scan(now.Add(watchSettleDelay + time.Second))
	result := filepath.Join(settings.OutputFolder, "Client", "Report (1).docx")
	requireExists(t, result)
	bytes, err := os.ReadFile(out)
	if err != nil || string(bytes) != "CUSTOM EXISTING CONTENT" {
		t.Fatalf("overwrote existing document: %q, %v", bytes, err)
	}
	w.scan(now.Add(2*watchSettleDelay + time.Second))
	if got := w.snapshot().Converted; got != 1 {
		t.Fatalf("dedupe failed: %d", got)
	}

	// Restart: stored fingerprint suppresses duplicates, but edits reconvert.
	other := newFolderWatcher(newStateStore(filepath.Join(filepath.Dir(w.ledgerPath), "settings.json")))
	other.config = settings
	other.status.Running = true
	if err := other.loadLedgerLocked(); err != nil {
		t.Fatal(err)
	}
	other.scan(now.Add(3*watchSettleDelay + time.Second))
	other.scan(now.Add(4*watchSettleDelay + time.Second))
	if got := other.snapshot().Converted; got != 0 {
		t.Fatalf("reconverted after restart: %d", got)
	}

	putMarkdown(t, nested, "# Version Two and changed content\n")
	other.scan(now.Add(5*watchSettleDelay + time.Second))
	other.scan(now.Add(6*watchSettleDelay + 2*time.Second))
	requireExists(t, filepath.Join(settings.OutputFolder, "Client", "Report (2).docx"))
}

func TestWatcherWaitsForStableFiles(t *testing.T) {
	w, s, _ := newTestWatcher(t, true)
	file := filepath.Join(s.WatchFolder, "incoming.md")
	putMarkdown(t, file, "# Partial")
	now := time.Now()
	w.scan(now)
	putMarkdown(t, file, "# Completed copy with more bytes\n")
	w.scan(now.Add(watchSettleDelay + time.Second))
	if got := w.snapshot().Converted; got != 0 {
		t.Fatalf("converted file before settling: %d", got)
	}
	w.scan(now.Add(2*watchSettleDelay + 2*time.Second))
	requireExists(t, filepath.Join(s.OutputFolder, "incoming.docx"))
}

func TestWatcherSkipsInitialAndDetectsNewFilesAfterRestart(t *testing.T) {
	w, s, _ := newTestWatcher(t, false)
	original := filepath.Join(s.WatchFolder, "existing.md")
	putMarkdown(t, original, "# Existing")
	now := time.Now()
	w.scan(now)
	w.scan(now.Add(watchSettleDelay + time.Second))
	if got := w.snapshot().Converted; got != 0 {
		t.Fatalf("converted existing unexpectedly: %d", got)
	}
	if !w.initialized {
		t.Fatal("initial scan not recorded")
	}

	restart := newFolderWatcher(newStateStore(filepath.Join(filepath.Dir(w.ledgerPath), "settings.json")))
	restart.config, restart.status.Running = s, true
	if err := restart.loadLedgerLocked(); err != nil {
		t.Fatal(err)
	}
	if !restart.initialized {
		t.Fatal("initial scan state not restored")
	}
	fresh := filepath.Join(s.WatchFolder, "new-after-shutdown.md")
	putMarkdown(t, fresh, "# New file")
	restart.scan(now.Add(2*watchSettleDelay + time.Second))
	restart.scan(now.Add(3*watchSettleDelay + 2*time.Second))
	requireExists(t, filepath.Join(s.OutputFolder, "new-after-shutdown.docx"))
	if _, err := os.Stat(filepath.Join(s.OutputFolder, "existing.docx")); !os.IsNotExist(err) {
		t.Fatalf("existing file incorrectly converted: %v", err)
	}
}

func TestWatcherRejectsUnsafePaths(t *testing.T) {
	_, s, root := newTestWatcher(t, true)
	bad := s
	bad.OutputFolder = filepath.Join(s.WatchFolder, "Converted")
	if _, err := validateWatcherSettings(bad); err == nil {
		t.Fatal("accepted output inside watched tree")
	}
	bad = s
	bad.WatchFolder = s.OutputFolder
	bad.OutputFolder = root
	if _, err := validateWatcherSettings(bad); err == nil {
		t.Fatal("accepted output enclosing watch folder")
	}
	if _, err := convertWatchedFile(s, filepath.Join(root, "escape.md"), "", ""); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaped watch folder: %v", err)
	}

	target := filepath.Join(root, "outside")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.WatchFolder, "linked")
	if err := os.Symlink(target, link); err == nil {
		if err := rejectLinkedPath(filepath.Join(link, "input.md")); err == nil {
			t.Fatal("accepted linked input")
		}
	}
}

func TestWatcherNonRecursiveIgnoresChildren(t *testing.T) {
	w, s, _ := newTestWatcher(t, true)
	s.Recursive = false
	w.config = s
	putMarkdown(t, filepath.Join(s.WatchFolder, "root.markdown"), "# Root")
	putMarkdown(t, filepath.Join(s.WatchFolder, "Nested", "ignored.md"), "# Nested")
	now := time.Now()
	w.scan(now)
	w.scan(now.Add(watchSettleDelay + time.Second))
	requireExists(t, filepath.Join(s.OutputFolder, "root.docx"))
	if _, err := os.Stat(filepath.Join(s.OutputFolder, "Nested", "ignored.docx")); !os.IsNotExist(err) {
		t.Fatalf("converted nested file unexpectedly: %v", err)
	}
}

func TestWatcherAllowsSameFolderWithoutFeedbackLoop(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Documents")
	settings := WatcherSettings{
		Enabled: true, WatchFolder: folder, OutputFolder: folder,
		Recursive: true, ConvertExisting: true,
	}
	normalized, err := validateWatcherSettings(settings)
	if err != nil {
		t.Fatalf("same input/output should be valid: %v", err)
	}
	if err := prepareWatcherFolders(normalized); err != nil {
		t.Fatal(err)
	}
	w := newFolderWatcher(newStateStore(filepath.Join(root, "settings.json")))
	w.config, w.status.Enabled, w.status.Running = normalized, true, true
	putMarkdown(t, filepath.Join(folder, "Report.md"), "# Hello\n")
	putMarkdown(t, filepath.Join(folder, "Nested", "Details.markdown"), "# Details\n")
	putMarkdown(t, filepath.Join(folder, "Report.docx"), "Existing output")

	now := time.Now()
	w.scan(now)
	w.scan(now.Add(watchSettleDelay + time.Second))
	requireExists(t, filepath.Join(folder, "Report (1).docx"))
	requireExists(t, filepath.Join(folder, "Nested", "Details.docx"))
	bytes, err := os.ReadFile(filepath.Join(folder, "Report.docx"))
	if err != nil || string(bytes) != "Existing output" {
		t.Fatalf("original Word file was overwritten: %q, %v", bytes, err)
	}
	w.scan(now.Add(3 * watchSettleDelay))
	if got := w.snapshot().Converted; got != 2 {
		t.Fatalf("generated Word files caused extra conversions: got %d", got)
	}
}

func TestWatcherCreatesMissingFoldersOnSaveAndStartup(t *testing.T) {
	root := t.TempDir()
	watch := filepath.Join(root, "new", "in", "Markdown")
	out := filepath.Join(root, "new", "out", "Word")
	store := newStateStore(filepath.Join(root, "settings.json"))
	app := newApp(store, nil)
	defer app.watcher.stop()
	settings := defaultPersistedState().Settings
	settings.Watcher = WatcherSettings{
		Enabled: true, Paused: true, WatchFolder: watch,
		OutputFolder: out, Recursive: true,
	}
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatalf("save should create missing folders: %v", err)
	}
	for _, path := range []string{watch, out} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("folder not created: %s, %v", path, err)
		}
	}
	if err := os.Remove(out); err != nil {
		t.Fatal(err)
	}
	// Reopening an enabled watcher should recreate a missing output folder.
	reopened := newApp(store, nil)
	defer reopened.watcher.stop()
	reopened.startup(nil)
	requireExists(t, out)
	if reopened.watcher.snapshot().Errors != 0 {
		t.Fatalf("startup error: %+v", reopened.watcher.snapshot())
	}
}

func TestWatcherDoesNotCreateInvalidOrDisabledFolders(t *testing.T) {
	root := t.TempDir()
	watch := filepath.Join(root, "should-not-exist")
	out := filepath.Join(watch, "nested-output")
	store := newStateStore(filepath.Join(root, "settings.json"))
	app := newApp(store, nil)
	defer app.watcher.stop()
	settings := defaultPersistedState().Settings
	settings.Watcher = WatcherSettings{Enabled: true, WatchFolder: watch, OutputFolder: out}
	if _, err := app.SaveSettings(settings); err == nil {
		t.Fatal("accepted different but overlapping paths")
	}
	if _, err := os.Stat(watch); !os.IsNotExist(err) {
		t.Fatalf("created a folder from invalid settings: %v", err)
	}

	settings.Watcher.Enabled = false
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(watch); !os.IsNotExist(err) {
		t.Fatalf("created disabled watch folder: %v", err)
	}

	file := filepath.Join(root, "regular-file")
	putMarkdown(t, file, "not a directory")
	settings.Watcher = WatcherSettings{Enabled: true, WatchFolder: file, OutputFolder: filepath.Join(root, "Word")}
	if _, err := app.SaveSettings(settings); err == nil {
		t.Fatal("accepted regular file as watch directory")
	}
	if _, err := os.Stat(settings.Watcher.OutputFolder); !os.IsNotExist(err) {
		t.Fatalf("created output after invalid input: %v", err)
	}
}
