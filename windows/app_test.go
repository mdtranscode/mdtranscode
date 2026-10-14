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

// Last-export actions are constrained to a file produced by a successful manual
// export; no frontend-supplied filesystem path is accepted by either action.
func TestLastExportedFileValidation(t *testing.T) {
	a := &App{}
	if _, err := a.lastExportedFile(); err == nil {
		t.Fatal("empty export history was accepted")
	}

	dir := t.TempDir()
	for _, name := range []string{"client résumé.pdf", "report.docx", "REPORT.PDF"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test content"), 0600); err != nil {
			t.Fatal(err)
		}
		a.rememberExport(path)
		if got, err := a.lastExportedFile(); err != nil || got != path {
			t.Fatalf("lastExportedFile() = %q, %v; want %q", got, err, path)
		}
	}

	a.rememberExport(filepath.Join(dir, "missing.pdf"))
	if _, err := a.lastExportedFile(); err == nil {
		t.Fatal("removed/missing export was accepted")
	}
	a.rememberExport(dir)
	if _, err := a.lastExportedFile(); err == nil {
		t.Fatal("directory was accepted as export")
	}
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("not an export"), 0600); err != nil {
		t.Fatal(err)
	}
	a.rememberExport(other)
	if _, err := a.lastExportedFile(); err == nil {
		t.Fatal("unsupported extension was accepted")
	}
}

func TestCreatedStatusActionsAreExposedOnlyForSuccessfulExports(t *testing.T) {
	html, err := os.ReadFile("frontend/dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile("frontend/dist/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{`id="createdOpenButton"`, `id="createdRevealButton"`} {
		if !strings.Contains(string(html), id) {
			t.Fatalf("missing persistent status action %q", id)
		}
	}
	for _, snippet := range []string{
		`runExportAction("OpenLastExportedFile")`,
		`runExportAction("ShowLastExportedFileInFolder")`,
		`setCreatedStatus(outputPath)`,
		`setCreatedStatus(path)`,
		`setStatusText(message)`,
	} {
		if !strings.Contains(string(js), snippet) {
			t.Fatalf("missing export status action wiring %q", snippet)
		}
	}
}

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

func TestNormalizePDFOutputPath(t *testing.T) {
	dir := t.TempDir()

	result, err := normalizePDFOutputPath(filepath.Join(dir, "Report"))
	if err != nil || result != filepath.Join(dir, "Report.pdf") {
		t.Fatalf("PDF output suffix: %q %v", result, err)
	}

	result, err = normalizePDFOutputPath(filepath.Join(dir, "Report.PDF"))
	if err != nil || result != filepath.Join(dir, "Report.PDF") {
		t.Fatalf("existing uppercase PDF suffix changed: %q %v", result, err)
	}

	for _, input := range []string{"", "  "} {
		if _, err := normalizePDFOutputPath(input); err == nil {
			t.Fatalf("empty PDF output path %q accepted", input)
		}
	}

	for _, path := range []string{dir, dir + string(filepath.Separator)} {
		if _, err := normalizePDFOutputPath(path); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("existing directory %q was not rejected: %v", path, err)
		}
	}

	// A directory with a .pdf extension must still be rejected.
	pdfDir := filepath.Join(dir, "Folder.pdf")
	if err := os.Mkdir(pdfDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pdfDir, filepath.Join(dir, "Folder")} {
		if _, err := normalizePDFOutputPath(path); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("PDF destination resolving to directory %q was not rejected: %v", path, err)
		}
	}
}

// A native image picker is offered for the saved Word cover logo path.
// Its filter must not advertise unsupported BMP/WebP/SVG formats.
func TestWordLogoDialogOptions(t *testing.T) {
	options := wordLogoDialogOptions()
	if options.Title != "Choose cover logo" {
		t.Fatalf("unexpected logo picker title: %q", options.Title)
	}
	if len(options.Filters) != 4 {
		t.Fatalf("logo picker has %d filters; want 4", len(options.Filters))
	}
	patterns := []string{"*.png;*.jpg;*.jpeg;*.gif", "*.png", "*.jpg;*.jpeg", "*.gif"}
	for i, pattern := range patterns {
		if options.Filters[i].Pattern != pattern {
			t.Fatalf("filter %d = %q; want %q", i, options.Filters[i].Pattern, pattern)
		}
	}
}

// A template warning may be ignored by the renderer but must remain visible
// in Settings, directly above its pinned Save/Cancel controls. Conversion
// failures must not display underneath the footer as a toast.
func TestWordTemplateSettingsWarningAndErrorLayout(t *testing.T) {
	html, err := os.ReadFile("frontend/dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("frontend/dist/app.css")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile("frontend/dist/app.js")
	if err != nil {
		t.Fatal(err)
	}
	markup := string(html)
	if !strings.Contains(markup, `id="settingsFeedback"`) || strings.Index(markup, `id="settingsFeedback"`) > strings.Index(markup, `class="settings-footer"`) {
		t.Fatal("Settings validation banner missing or not positioned above Save/Cancel")
	}
	for _, fragment := range []string{`.settings-page .settings-feedback`, `.settings-page .settings-feedback.warning`} {
		if !strings.Contains(string(css), fragment) {
			t.Errorf("missing Settings notification style %s", fragment)
		}
	}
	for _, fragment := range []string{`App.InspectWordTemplate`, `inspectSelectedWordTemplate`, `showSettingsFeedback(message, "error")`, `Settings saved. Word template warning:`} {
		if !strings.Contains(string(js), fragment) {
			t.Errorf("missing Settings warning handling %s", fragment)
		}
	}
}
