package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdtranscode/mdtranscode/core-src/markdown"
	htmlrenderer "github.com/mdtranscode/mdtranscode/core-src/renderer/html"
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
}

func NewApp() *App {
	return newApp(defaultStateStore(), os.Args[1:])
}

func newApp(store *stateStore, args []string) *App {
	return &App{
		store:       store,
		startupPath: markdownArgument(args),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Version returns the current application version for the shell UI.
func (a *App) Version() string {
	return appVersion
}

// Settings returns the current user-facing application settings.
func (a *App) Settings() (AppSettings, error) {
	return a.store.settings()
}

// SaveSettings persists user-facing application settings.
func (a *App) SaveSettings(settings AppSettings) (AppSettings, error) {
	return a.store.saveSettings(settings)
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
	return result.OutputPath, nil
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
