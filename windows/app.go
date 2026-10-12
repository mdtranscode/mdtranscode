package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mdtranscode/mdtranscode/core-src/markdown"
	htmlrenderer "github.com/mdtranscode/mdtranscode/core-src/renderer/html"
	"github.com/mdtranscode/mdtranscode/core-src/renderer/pdf"
	"github.com/mdtranscode/mdtranscode/core-src/transcode"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OutlineItem is the UI-safe representation of one rendered document heading.
type OutlineItem struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// MarkdownFile is the UI-safe representation of an opened Markdown file.
type MarkdownFile struct {
	Path            string        `json:"path"`
	Name            string        `json:"name"`
	Source          string        `json:"source"`
	PreviewHTML     string        `json:"previewHtml"`
	Outline         []OutlineItem `json:"outline"`
	Size            int64         `json:"size"`
	Modified        string        `json:"modified"`
	SuggestedOutput string        `json:"suggestedOutput"`
}

// App owns the Windows shell and delegates document conversion to Core.
type App struct {
	ctx         context.Context
	store       *stateStore
	startupPath string
	watcher     *folderWatcher
	tray        *trayController
	exiting     atomic.Bool
	exportMu    sync.RWMutex
	lastExport  string
}

func NewApp() *App {
	return newApp(defaultStateStore(), os.Args[1:])
}

func newApp(store *stateStore, args []string) *App {
	return &App{
		watcher:     newFolderWatcher(store),
		store:       store,
		startupPath: markdownArgument(args),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	tray, trayErr := startTray(a)
	if trayErr == nil {
		a.tray = tray
	} else {
		// Never strand the user in a hidden app without a functioning tray.
		runtime.WindowShow(ctx)
	}
	settings, err := a.store.settings()
	if err == nil {
		err = a.watcher.configure(settings.Watcher)
	}
	if err != nil {
		a.watcher.mu.Lock()
		a.watcher.status.Enabled = settings.Watcher.Enabled
		a.watcher.recordErrorLocked("", fmt.Errorf("start folder monitor: %w", err))
		a.watcher.mu.Unlock()
	}
}

// beforeClose hides the window only when the user permits close-to-tray.
// When disabled (or on explicit tray Exit), Wails shuts down the watcher.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.tray == nil || a.exiting.Load() {
		return false
	}
	settings, err := a.store.settings()
	if err != nil || !settings.CloseToTray {
		// A real close stops the watcher through the normal Wails shutdown.
		return false
	}
	runtime.WindowHide(ctx)
	return true
}
func (a *App) showApp(page string) {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	if page == "settings" {
		runtime.EventsEmit(a.ctx, "mdtranscode:open-settings")
	}
	if page == "history" {
		runtime.EventsEmit(a.ctx, "mdtranscode:open-settings")
	}
	if page == "updates" {
		runtime.EventsEmit(a.ctx, "mdtranscode:open-updates")
	}
}
func (a *App) trayCommand(id uint32) {
	if a.ctx == nil {
		return
	}
	switch id {
	case idOpen:
		a.showApp("")
	case idSettings:
		a.showApp("settings")
	case idHistory:
		a.showApp("history")
	case idUpdate:
		a.showApp("updates")
	case idWatchFolder:
		if err := a.OpenWatchedFolder(); err != nil {
			a.showApp("settings")
			runtime.EventsEmit(a.ctx, "mdtranscode:tray-error", err.Error())
		}
	case idOutputFolder:
		if err := a.OpenConvertedFolder(); err != nil {
			a.showApp("settings")
			runtime.EventsEmit(a.ctx, "mdtranscode:tray-error", err.Error())
		}
	case idStartup:
		_ = a.OpenWindowsStartupSettings()
	case idPause:
		s, err := a.store.settings()
		if err == nil && s.Watcher.Enabled {
			s.Watcher.Paused = !s.Watcher.Paused
			_, _ = a.SaveSettings(s)
		}
	case idScan:
		go a.watcher.scan(time.Now())
	case idExit:
		a.exiting.Store(true)
		runtime.Quit(a.ctx)
	}
}

// OpenWindowsStartupSettings opens the Windows-managed Startup Apps page.
// Enabling/disabling a packaged startup task remains under user control.
func (a *App) OpenWindowsStartupSettings() error {
	return openWindowsStartupSettings()
}

func (a *App) OpenWatchedFolder() error {
	s, err := a.store.settings()
	if err != nil {
		return err
	}
	return openFolder(s.Watcher.WatchFolder)
}
func (a *App) OpenConvertedFolder() error {
	s, err := a.store.settings()
	if err != nil {
		return err
	}
	return openFolder(s.Watcher.OutputFolder)
}

// Version returns the current application version for the shell UI.
func (a *App) Version() string {
	return displayVersion()
}

// Settings returns the current user-facing application settings.
func (a *App) Settings() (AppSettings, error) {
	return a.store.settings()
}

// SaveSettings persists user-facing application settings.
func (a *App) SaveSettings(settings AppSettings) (AppSettings, error) {
	// Older clients do not send the watcher field; do not silently switch off
	// an existing monitor when only document preferences are updated.
	if settings.Watcher == (WatcherSettings{}) {
		previous, err := a.store.settings()
		if err != nil {
			return AppSettings{}, err
		}
		settings.Watcher = previous.Watcher
	}
	normalized, err := validateWatcherSettings(settings.Watcher)
	if err != nil {
		return AppSettings{}, err
	}
	settings.Watcher = normalized
	if settings.PDF.PageSize != "" {
		if _, err := pdf.Validate(settings.PDF); err != nil {
			return AppSettings{}, err
		}
	}
	if err := prepareWatcherFolders(normalized); err != nil {
		return AppSettings{}, err
	}
	saved, err := a.store.saveSettings(settings)
	if err != nil {
		return AppSettings{}, err
	}
	if err = a.watcher.configure(saved.Watcher); err != nil {
		return saved, err
	}
	return saved, nil
}

// WatcherStatus exposes bounded monitoring history without blocking document edits.
func (a *App) WatcherStatus() WatcherStatus { return a.watcher.snapshot() }

// BrowseWatcherFolder opens a native directory chooser for either watcher path.
func (a *App) BrowseWatcherFolder() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select folder for Markdown automatic conversion"})
}

// shutdown cancels background scans before Wails exits.
func (a *App) shutdown(_ context.Context) {
	a.watcher.stop()
	if a.tray != nil {
		a.tray.stop()
	}
}

// RecentFiles returns the recent Markdown files known to the application.
func (a *App) RecentFiles() ([]RecentFile, error) {
	return a.store.recentFiles()
}

// ClearRecentFiles removes the persisted recent-file list.
func (a *App) ClearRecentFiles() error {
	return a.store.clearRecentFiles()
}

// StartupFile opens a Markdown path passed on the process command line, if one exists.
func (a *App) StartupFile() (*MarkdownFile, error) {
	path := a.startupPath
	a.startupPath = ""
	if path == "" {
		return nil, nil
	}
	return a.openMarkdownPath(path)
}

// OpenMarkdown opens the native Windows file picker and reads one Markdown file.
func (a *App) OpenMarkdown() (*MarkdownFile, error) {
	if a.ctx == nil {
		return nil, errors.New("application is not ready")
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Open Markdown",
		DefaultDirectory: a.store.lastOpenDirectory(),
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown files (*.md;*.markdown)", Pattern: "*.md;*.markdown"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open Markdown dialog: %w", err)
	}
	if path == "" {
		return nil, nil
	}

	return a.openMarkdownPath(path)
}

// OpenMarkdownPath reads a dropped/recent Markdown file by absolute or relative path.
func (a *App) OpenMarkdownPath(path string) (*MarkdownFile, error) {
	return a.openMarkdownPath(path)
}

func (a *App) openMarkdownPath(path string) (*MarkdownFile, error) {
	file, err := loadMarkdownFile(path)
	if err != nil {
		return nil, err
	}
	_ = a.store.rememberFile(file.Path)
	return file, nil
}

// ExportPDF opens a Save dialog and delegates PDF rendering to the shared Core.
func (a *App) ExportPDF(inputPath string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}
	input, err := validateMarkdownPath(inputPath)
	if err != nil {
		return "", err
	}
	settings, err := a.store.settings()
	if err != nil {
		return "", err
	}
	if _, err := pdf.Validate(settings.PDF); err != nil {
		return "", err
	}
	suggested := transcode.DefaultOutputPath(input, transcode.FormatPDF)
	selected, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Export to PDF", DefaultDirectory: a.store.lastExportDirectory(filepath.Dir(suggested)),
		DefaultFilename: filepath.Base(suggested), Filters: []runtime.FileFilter{{DisplayName: "PDF document (*.pdf)", Pattern: "*.pdf"}},
	})
	if err != nil {
		return "", fmt.Errorf("save PDF dialog: %w", err)
	}
	if selected == "" {
		return "", nil
	}
	output, err := normalizePDFOutputPath(selected)
	if err != nil {
		return "", err
	}
	result, err := transcode.ConvertFile(transcode.FileOptions{InputPath: input, OutputPath: output, Format: transcode.FormatPDF, Creator: appName, PDF: settings.PDF})
	if err != nil {
		return "", err
	}
	_ = a.store.rememberExportDirectory(result.OutputPath)
	a.rememberExport(result.OutputPath)
	return result.OutputPath, nil
}

func normalizePDFOutputPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("PDF output path is required")
	}

	// Reject the original selection before adding an extension. Otherwise an
	// existing folder like "Reports" would silently become "Reports.pdf".
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve PDF output path: %w", err)
	}
	if info, statErr := os.Stat(absolute); statErr == nil && info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a PDF document", absolute)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect PDF output path: %w", statErr)
	}

	if !strings.EqualFold(filepath.Ext(absolute), ".pdf") {
		absolute += ".pdf"
		// The suffixed destination might itself be an existing directory.
		if info, statErr := os.Stat(absolute); statErr == nil && info.IsDir() {
			return "", fmt.Errorf("%q is a directory, not a PDF document", absolute)
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect PDF output path: %w", statErr)
		}
	}
	return filepath.Clean(absolute), nil
}

// ExportDOCX asks for a destination and exports the selected Markdown file through Core.
func (a *App) ExportDOCX(inputPath string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}
	absoluteInput, err := validateMarkdownPath(inputPath)
	if err != nil {
		return "", err
	}

	defaultOutput := transcode.DefaultOutputPath(absoluteInput, transcode.FormatDOCX)
	outputPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "Export to Word",
		DefaultDirectory: a.store.lastExportDirectory(filepath.Dir(defaultOutput)),
		DefaultFilename:  filepath.Base(defaultOutput),
		Filters: []runtime.FileFilter{
			{DisplayName: "Word document (*.docx)", Pattern: "*.docx"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save DOCX dialog: %w", err)
	}
	if outputPath == "" {
		return "", nil
	}

	outputPath, err = normalizeDOCXOutputPath(outputPath)
	if err != nil {
		return "", err
	}

	result, err := transcode.ConvertFile(transcode.FileOptions{
		InputPath:  absoluteInput,
		OutputPath: outputPath,
		Format:     transcode.FormatDOCX,
		Creator:    appName,
	})
	if err != nil {
		return "", err
	}
	_ = a.store.rememberExportDirectory(result.OutputPath)
	a.rememberExport(result.OutputPath)
	return result.OutputPath, nil
}

// rememberExport records only files successfully created by a manual export.
// The frontend never chooses which arbitrary filesystem path to open.
func (a *App) rememberExport(path string) {
	a.exportMu.Lock()
	a.lastExport = path
	a.exportMu.Unlock()
}

// lastExportedFile validates the stored result again before asking the OS to open it.
func (a *App) lastExportedFile() (string, error) {
	a.exportMu.RLock()
	path := a.lastExport
	a.exportMu.RUnlock()
	if path == "" {
		return "", errors.New("no exported document is available")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".pdf" && ext != ".docx" {
		return "", errors.New("the last export is not a PDF or Word document")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("locate exported document: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("the exported document is not a regular file")
	}
	return path, nil
}

// OpenLastExportedFile opens the most recently completed manual export using
// the user's default PDF/Word application.
func (a *App) OpenLastExportedFile() error {
	path, err := a.lastExportedFile()
	if err != nil {
		return err
	}
	return openExportedFile(path)
}

// ShowLastExportedFileInFolder selects the most recent export in File Explorer.
func (a *App) ShowLastExportedFileInFolder() error {
	path, err := a.lastExportedFile()
	if err != nil {
		return err
	}
	return revealExportedFile(path)
}

func loadMarkdownFile(path string) (*MarkdownFile, error) {
	absolutePath, err := validateMarkdownPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("open Markdown file %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a directory, not a Markdown file", path)
	}

	source, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("read Markdown file %q: %w", path, err)
	}

	parsed := markdown.Parse(string(source))
	preview := htmlrenderer.Render(parsed, htmlrenderer.Options{
		BaseDir:          filepath.Dir(absolutePath),
		EmbedLocalImages: true,
	})
	outline := make([]OutlineItem, 0, len(preview.Outline))
	for _, item := range preview.Outline {
		outline = append(outline, OutlineItem{ID: item.ID, Level: item.Level, Text: item.Text})
	}

	return &MarkdownFile{
		Path:            absolutePath,
		Name:            filepath.Base(absolutePath),
		Source:          string(source),
		PreviewHTML:     preview.BodyHTML,
		Outline:         outline,
		Size:            info.Size(),
		Modified:        info.ModTime().Local().Format(time.RFC3339),
		SuggestedOutput: transcode.DefaultOutputPath(absolutePath, transcode.FormatDOCX),
	}, nil
}

func validateMarkdownPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("Markdown file path is required")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Markdown path: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(absolutePath))
	if ext != ".md" && ext != ".markdown" {
		return "", fmt.Errorf("%q is not a supported Markdown file (.md or .markdown)", filepath.Base(absolutePath))
	}
	return filepath.Clean(absolutePath), nil
}

func normalizeDOCXOutputPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("Word output path is required")
	}

	originalPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Word output path: %w", err)
	}
	if info, statErr := os.Stat(originalPath); statErr == nil && info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a Word document", originalPath)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Word output path: %w", statErr)
	}

	if !strings.EqualFold(filepath.Ext(path), ".docx") {
		path += ".docx"
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Word output path: %w", err)
	}
	return filepath.Clean(absolutePath), nil
}

func markdownArgument(args []string) string {
	for _, arg := range args {
		value := strings.TrimSpace(strings.Trim(arg, `"`))
		if value == "" || strings.HasPrefix(value, "-") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(value))
		if ext == ".md" || ext == ".markdown" {
			return value
		}
	}
	return ""
}
