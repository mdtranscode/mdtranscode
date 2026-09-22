package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdtranscode/mdtranscode/core-src/transcode"
)

func TestLoadMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("# Report\n\nHello.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	file, err := loadMarkdownFile(path)
	if err != nil {
		t.Fatalf("loadMarkdownFile returned error: %v", err)
	}
	if file.Name != "report.md" {
		t.Fatalf("Name = %q, want report.md", file.Name)
	}
	if !strings.Contains(file.Source, "# Report") {
		t.Fatal("source was not loaded")
	}
	if filepath.Ext(file.SuggestedOutput) != ".docx" {
		t.Fatalf("SuggestedOutput = %q", file.SuggestedOutput)
	}
	if !strings.Contains(file.PreviewHTML, `<h1 id="report">Report</h1>`) {
		t.Fatalf("preview HTML was not rendered from Core: %s", file.PreviewHTML)
	}
	if len(file.Outline) != 1 || file.Outline[0].ID != "report" || file.Outline[0].Text != "Report" {
		t.Fatalf("unexpected outline: %#v", file.Outline)
	}
}

func TestValidateMarkdownPathRejectsNonMarkdown(t *testing.T) {
	_, err := validateMarkdownPath("report.txt")
	if err == nil || !strings.Contains(err.Error(), "not a supported Markdown file") {
		t.Fatalf("expected Markdown validation error, got %v", err)
	}
}

func TestCoreExportPathUsedByWindowsShell(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "report.md")
	output := filepath.Join(dir, "report.docx")
	if err := os.WriteFile(input, []byte("# Windows Shell Test\n\n- one\n- two\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := transcode.ConvertFile(transcode.FileOptions{
		InputPath:  input,
		OutputPath: output,
		Format:     transcode.FormatDOCX,
		Creator:    appName,
	}); err != nil {
		t.Fatalf("Core export returned error: %v", err)
	}

	parts := openWindowsTestDOCX(t, output)
	settings := string(parts["word/settings.xml"])
	if !strings.Contains(settings, `w:name="compatibilityMode"`) || !strings.Contains(settings, `w:val="15"`) {
		t.Fatal("Windows shell Core export does not declare compatibilityMode=15")
	}
}

func openWindowsTestDOCX(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	defer zr.Close()

	parts := make(map[string][]byte)
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open part %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("read part %s: %v", f.Name, err)
		}
		parts[f.Name] = data
	}
	return parts
}

func TestMarkdownArgumentFindsLaunchFile(t *testing.T) {
	path := filepath.Join("C:\\", "Docs", "Résumé.md")
	got := markdownArgument([]string{"--some-wails-flag", path})
	if got != path {
		t.Fatalf("markdownArgument = %q, want %q", got, path)
	}
	if got := markdownArgument([]string{"report.txt"}); got != "" {
		t.Fatalf("markdownArgument accepted unsupported file: %q", got)
	}
}

func TestStartupFileOpensArgumentOnceAndAddsRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "startup.md")
	if err := os.WriteFile(path, []byte("# Startup\n"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newStateStore(filepath.Join(dir, "settings.json"))
	app := newApp(store, []string{path})

	file, err := app.StartupFile()
	if err != nil {
		t.Fatalf("StartupFile returned error: %v", err)
	}
	if file == nil || file.Path != path {
		t.Fatalf("StartupFile = %#v, want %q", file, path)
	}
	second, err := app.StartupFile()
	if err != nil || second != nil {
		t.Fatalf("second StartupFile = %#v, %v; want nil, nil", second, err)
	}
	recent, err := app.RecentFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Name != "startup.md" {
		t.Fatalf("unexpected recent files: %#v", recent)
	}
}

func TestLoadMarkdownFileUnicodeLongPath(t *testing.T) {
	dir := t.TempDir()
	longDir := dir
	for i := 0; len(longDir) < 285; i++ {
		longDir = filepath.Join(longDir, "資料-very-long-folder-name")
	}
	if err := os.MkdirAll(longDir, 0755); err != nil {
		t.Fatalf("create long Unicode path: %v", err)
	}
	path := filepath.Join(longDir, "Résumé-Δοκιμή-日本語.md")
	if err := os.WriteFile(path, []byte("# Unicode path\n\nWorks.\n"), 0644); err != nil {
		t.Fatalf("write long Unicode Markdown path: %v", err)
	}

	file, err := loadMarkdownFile(path)
	if err != nil {
		t.Fatalf("loadMarkdownFile long Unicode path returned error: %v", err)
	}
	if file.Name != filepath.Base(path) || !strings.Contains(file.PreviewHTML, "Unicode path") {
		t.Fatalf("unexpected file loaded from long Unicode path: %#v", file)
	}
}

func TestNormalizeDOCXOutputPath(t *testing.T) {
	dir := t.TempDir()
	path, err := normalizeDOCXOutputPath(filepath.Join(dir, "Résumé"))
	if err != nil {
		t.Fatalf("normalizeDOCXOutputPath returned error: %v", err)
	}
	if filepath.Ext(path) != ".docx" {
		t.Fatalf("normalized output = %q, want .docx extension", path)
	}

	_, err = normalizeDOCXOutputPath(dir)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory rejection, got %v", err)
	}
}
