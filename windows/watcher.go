package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mdtranscode/mdtranscode/core-src/transcode"
)

// Folder monitoring is intentionally foreground-only in this increment. Startup
// registration and tray/background lifetime require separate MSIX acceptance.
const (
	watchPollInterval = 5 * time.Second
	watchSettleDelay  = 4 * time.Second
	watchRetryDelay   = 30 * time.Second
	watchHistoryLimit = 30
)

type WatcherSettings struct {
	Enabled         bool   `json:"enabled"`
	Paused          bool   `json:"paused"`
	WatchFolder     string `json:"watchFolder"`
	OutputFolder    string `json:"outputFolder"`
	Recursive       bool   `json:"recursive"`
	ConvertExisting bool   `json:"convertExisting"`
}

func defaultWatcherSettings() WatcherSettings {
	home, _ := os.UserHomeDir()
	return WatcherSettings{
		WatchFolder:     filepath.Join(home, "Documents", "Markdown"),
		OutputFolder:    filepath.Join(home, "Documents", "Converted"),
		Recursive:       true,
		ConvertExisting: true,
	}
}

func normalizeWatcherSettings(s WatcherSettings) WatcherSettings {
	s.WatchFolder = strings.TrimSpace(s.WatchFolder)
	s.OutputFolder = strings.TrimSpace(s.OutputFolder)
	return s
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func validateWatcherSettings(s WatcherSettings) (WatcherSettings, error) {
	s = normalizeWatcherSettings(s)
	if !s.Enabled {
		return s, nil
	}
	if s.WatchFolder == "" || s.OutputFolder == "" {
		return s, errors.New("watch and output folders are both required")
	}
	input, err := filepath.Abs(s.WatchFolder)
	if err != nil {
		return s, err
	}
	output, err := filepath.Abs(s.OutputFolder)
	if err != nil {
		return s, err
	}
	// Even an inactive watcher cannot point to symlink/junction directories.
	if err = rejectLinkedPath(input); err != nil {
		return s, fmt.Errorf("watch folder: %w", err)
	}
	if err = rejectLinkedPath(output); err != nil {
		return s, fmt.Errorf("output folder: %w", err)
	}
	for _, entry := range []struct {
		name string
		path string
	}{
		{"watch folder", input},
		{"output folder", output},
	} {
		info, err := os.Stat(entry.path)
		if err == nil && !info.IsDir() {
			return s, fmt.Errorf("%s must be a directory", entry.name)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return s, fmt.Errorf("check %s: %w", entry.name, err)
		}
	}
	// Identical roots are safe: the scanner accepts Markdown inputs only, never
	// generated .docx outputs. Reject *different* overlapping roots, including
	// an output subfolder, to avoid unexpected input discovery.
	if !samePath(input, output) && (isWithin(strings.ToLower(input), strings.ToLower(output)) || isWithin(strings.ToLower(output), strings.ToLower(input))) {
		return s, errors.New("watch and output folders may be identical but must not otherwise overlap")
	}
	s.WatchFolder, s.OutputFolder = filepath.Clean(input), filepath.Clean(output)
	return s, nil
}

// Create missing directories only when monitoring is enabled. Keep validation
// separate so periodic scans never silently recreate a deleted watched tree.
func prepareWatcherFolders(s WatcherSettings) error {
	if !s.Enabled {
		return nil
	}
	for _, entry := range []struct {
		name string
		path string
	}{
		{"watch folder", s.WatchFolder},
		{"output folder", s.OutputFolder},
	} {
		if err := rejectLinkedPath(entry.path); err != nil {
			return fmt.Errorf("%s: %w", entry.name, err)
		}
		if err := os.MkdirAll(entry.path, 0755); err != nil {
			return fmt.Errorf("create %s: %w", entry.name, err)
		}
		// Recheck after creation to reject junction/symlink components.
		if err := rejectLinkedPath(entry.path); err != nil {
			return fmt.Errorf("%s: %w", entry.name, err)
		}
	}
	return nil
}

// Inspect each existing component to prevent output being redirected through a
// symbolic link or Windows junction. WalkDir never follows symlink inputs.
func rejectLinkedPath(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current := filepath.VolumeName(path) + string(filepath.Separator)
	remainder := strings.TrimPrefix(path, current)
	if current == string(filepath.Separator) {
		remainder = strings.TrimPrefix(path, current)
	}
	for _, segment := range strings.Split(remainder, string(filepath.Separator)) {
		if segment == "" {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked path component is not allowed: %s", current)
		}
	}
	return nil
}

type WatchEvent struct {
	At      string `json:"at"`
	Source  string `json:"source"`
	Output  string `json:"output,omitempty"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

type WatcherStatus struct {
	Enabled   bool         `json:"enabled"`
	Paused    bool         `json:"paused"`
	Running   bool         `json:"running"`
	LastScan  string       `json:"lastScan,omitempty"`
	Pending   int          `json:"pending"`
	Converted int          `json:"converted"`
	Errors    int          `json:"errors"`
	LastError string       `json:"lastError,omitempty"`
	History   []WatchEvent `json:"history"`
}

type watchedFile struct {
	signature string
	firstSeen time.Time
}
type retryRecord struct {
	signature string
	at        time.Time
}

type folderWatcher struct {
	mu          sync.Mutex
	scanMu      sync.Mutex
	config      WatcherSettings
	status      WatcherStatus
	seen        map[string]watchedFile
	successful  map[string]string // input path -> size/mtime/SHA256 fingerprint
	retries     map[string]retryRecord
	ledgerPath  string
	initialized bool // recorded first scan, including when ConvertExisting is false
	cancel      context.CancelFunc
	done        chan struct{}
}

func newFolderWatcher(store *stateStore) *folderWatcher {
	ledger := ""
	if store != nil && store.path != "" {
		ledger = filepath.Join(filepath.Dir(store.path), "watcher-ledger.json")
	}
	return &folderWatcher{
		seen: map[string]watchedFile{}, successful: map[string]string{},
		retries: map[string]retryRecord{}, ledgerPath: ledger,
		status: WatcherStatus{History: []WatchEvent{}},
	}
}

func (w *folderWatcher) configure(config WatcherSettings) error {
	normalized, err := validateWatcherSettings(config)
	if err != nil {
		return err
	}
	if err := prepareWatcherFolders(normalized); err != nil {
		return err
	}
	w.stop()
	w.mu.Lock()
	w.config = normalized
	w.status.Enabled, w.status.Paused = normalized.Enabled, normalized.Paused
	w.status.Running, w.status.Pending = false, 0
	w.seen = map[string]watchedFile{}
	w.retries = map[string]retryRecord{}
	w.successful = map[string]string{}
	w.initialized = false
	w.status.LastScan = ""
	if normalized.Enabled {
		if err := w.loadLedgerLocked(); err != nil {
			w.recordErrorLocked("", err)
		}
	}
	if normalized.Enabled && !normalized.Paused {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel, w.done = cancel, make(chan struct{})
		w.status.Running = true
		done := w.done
		w.mu.Unlock()
		go func() {
			defer close(done)
			w.scan(time.Now())
			ticker := time.NewTicker(watchPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					w.scan(now)
				}
			}
		}()
		return nil
	}
	w.mu.Unlock()
	return nil
}

func (w *folderWatcher) stop() {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.cancel, w.done = nil, nil
	w.status.Running = false
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (w *folderWatcher) snapshot() WatcherStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := w.status
	snapshot.History = append([]WatchEvent{}, w.status.History...)
	return snapshot
}

func (w *folderWatcher) recordErrorLocked(source string, err error) {
	w.status.Errors++
	w.status.LastError = err.Error()
	w.eventLocked(WatchEvent{At: time.Now().Format(time.RFC3339), Source: source, Result: "error", Message: err.Error()})
}

func (w *folderWatcher) eventLocked(event WatchEvent) {
	w.status.History = append([]WatchEvent{event}, w.status.History...)
	if len(w.status.History) > watchHistoryLimit {
		w.status.History = w.status.History[:watchHistoryLimit]
	}
}

func (w *folderWatcher) scan(now time.Time) {
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	w.mu.Lock()
	config := w.config
	running := w.status.Running
	firstScan := !w.initialized
	w.mu.Unlock()
	if !running {
		return
	}
	config, err := validateWatcherSettings(config)
	if err != nil {
		w.mu.Lock()
		w.recordErrorLocked("", err)
		w.mu.Unlock()
		return
	}
	found := map[string]bool{}
	candidates := []string{}
	err = filepath.WalkDir(config.WatchFolder, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if !config.Recursive && path != config.WatchFolder {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		found[path] = true
		candidates = append(candidates, path)
		return nil
	})
	if err != nil {
		w.mu.Lock()
		w.recordErrorLocked("", fmt.Errorf("scan watched folder: %w", err))
		w.mu.Unlock()
		return
	}
	sort.Strings(candidates)
	pending := 0
	for _, path := range candidates {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		signature := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
		w.mu.Lock()
		previous, observed := w.seen[path]
		if !observed || previous.signature != signature {
			w.seen[path] = watchedFile{signature: signature, firstSeen: now}
			if firstScan && !config.ConvertExisting {
				if fingerprint, e := fingerprintFile(path, signature); e == nil {
					w.successful[path] = fingerprint
				} else {
					w.recordErrorLocked(path, e)
				}
			} else {
				pending++
			}
			w.mu.Unlock()
			continue
		}
		if now.Sub(previous.firstSeen) < watchSettleDelay {
			pending++
			w.mu.Unlock()
			continue
		}
		if retry, exists := w.retries[path]; exists && retry.signature == signature && now.Sub(retry.at) < watchRetryDelay {
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()
		fingerprint, err := fingerprintFile(path, signature)
		if err != nil {
			w.retry(path, signature, now, err)
			continue
		}
		w.mu.Lock()
		unchanged := w.successful[path] == fingerprint
		w.mu.Unlock()
		if unchanged {
			continue
		}
		output, err := convertWatchedFile(config, path, signature, fingerprint)
		if err != nil {
			w.retry(path, signature, now, err)
			continue
		}
		w.mu.Lock()
		w.successful[path] = fingerprint
		delete(w.retries, path)
		w.status.Converted++
		w.eventLocked(WatchEvent{At: time.Now().Format(time.RFC3339), Source: path, Output: output, Result: "converted"})
		if err := w.saveLedgerLocked(); err != nil {
			w.recordErrorLocked(path, err)
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	ledgerChanged := !w.initialized
	for path := range w.seen {
		if !found[path] {
			delete(w.seen, path)
			delete(w.retries, path)
		}
	}
	for path := range w.successful {
		if !found[path] {
			delete(w.successful, path)
			ledgerChanged = true
		}
	}
	w.initialized = true
	if ledgerChanged {
		if err := w.saveLedgerLocked(); err != nil {
			w.recordErrorLocked("", err)
		}
	}
	w.status.LastScan = now.Format(time.RFC3339)
	w.status.Pending = pending
	w.mu.Unlock()
}

func (w *folderWatcher) retry(path, signature string, now time.Time, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.retries[path] = retryRecord{signature: signature, at: now}
	w.recordErrorLocked(path, err)
}

func fingerprintFile(path, expectedSignature string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	signature := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	if signature != expectedSignature {
		return "", errors.New("source changed while being read; retrying")
	}
	return signature + ":" + hex.EncodeToString(hash.Sum(nil)), nil
}

func convertWatchedFile(config WatcherSettings, source, signature, expectedFingerprint string) (string, error) {
	if err := rejectLinkedPath(source); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(config.WatchFolder, source)
	if err != nil || relative == "." || !isWithin(config.WatchFolder, source) {
		return "", errors.New("source escapes watched folder")
	}
	target := filepath.Join(config.OutputFolder, strings.TrimSuffix(relative, filepath.Ext(relative))+".docx")
	parent := filepath.Dir(target)
	if err := rejectLinkedPath(parent); err != nil {
		return "", err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	if err := rejectLinkedPath(parent); err != nil {
		return "", err
	}
	// Reserve output atomically. Existing documents are *never* overwritten.
	for i := 0; i < 10000; i++ {
		candidate := target
		if i > 0 {
			candidate = strings.TrimSuffix(target, ".docx") + fmt.Sprintf(" (%d).docx", i)
		}
		file, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		file.Close()
		// ConvertFile writes directly to our reserved path. On failure, remove
		// the incomplete document so no half-written output is retained.
		_, err = transcode.ConvertFile(transcode.FileOptions{InputPath: source, OutputPath: candidate, Format: transcode.FormatDOCX, Creator: appName})
		if err == nil {
			var actual string
			actual, err = fingerprintFile(source, signature)
			if err == nil && actual != expectedFingerprint {
				err = errors.New("source changed during conversion; retrying")
			}
		}
		if err != nil {
			_ = os.Remove(candidate)
			return "", err
		}
		return candidate, nil
	}
	return "", errors.New("too many conflicting output filenames")
}

func (w *folderWatcher) loadLedgerLocked() error {
	if w.ledgerPath == "" {
		return nil
	}
	bytes, err := os.ReadFile(w.ledgerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var ledger struct {
		Version      int               `json:"version"`
		Initialized  bool              `json:"initialized"`
		Entries      map[string]string `json:"entries"`
		WatchFolder  string            `json:"watchFolder"`
		OutputFolder string            `json:"outputFolder"`
	}
	if err := json.Unmarshal(bytes, &ledger); err != nil {
		return fmt.Errorf("read watcher ledger: %w", err)
	}
	if (ledger.Version != 1 && ledger.Version != 2) || !samePath(ledger.WatchFolder, w.config.WatchFolder) || !samePath(ledger.OutputFolder, w.config.OutputFolder) {
		return nil
	}
	if ledger.Version >= 2 {
		w.initialized = ledger.Initialized
	} else {
		w.initialized = len(ledger.Entries) > 0
	}
	if ledger.Entries != nil {
		w.successful = ledger.Entries
	}
	return nil
}

func (w *folderWatcher) saveLedgerLocked() error {
	if w.ledgerPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(w.ledgerPath), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Version      int               `json:"version"`
		Initialized  bool              `json:"initialized"`
		WatchFolder  string            `json:"watchFolder"`
		OutputFolder string            `json:"outputFolder"`
		Entries      map[string]string `json:"entries"`
	}{2, w.initialized, w.config.WatchFolder, w.config.OutputFolder, w.successful}, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(w.ledgerPath), ".watcher-ledger-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if e := temp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	// Windows Rename cannot replace an existing destination; remove it first.
	if err = os.Rename(temp.Name(), w.ledgerPath); err != nil {
		if e := os.Remove(w.ledgerPath); e != nil && !errors.Is(e, os.ErrNotExist) {
			return err
		}
		return os.Rename(temp.Name(), w.ledgerPath)
	}
	return nil
}
