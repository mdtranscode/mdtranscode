package transcode

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultOutputPath(t *testing.T) {
	got := DefaultOutputPath(filepath.Join("docs", "report.md"), FormatDOCX)
	want := filepath.Join("docs", "report.docx")
	if got != want {
		t.Fatalf("DefaultOutputPath = %q, want %q", got, want)
	}
}

func TestConvertFileRejectsUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "report.md")
	if err := os.WriteFile(input, []byte("# Title\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ConvertFile(FileOptions{InputPath: input, Format: "pdf"})
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("expected unsupported-format error, got %v", err)
	}
}

func TestConvertFileRejectsInputAsOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "report.md")
	if err := os.WriteFile(input, []byte("# Title\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ConvertFile(FileOptions{InputPath: input, OutputPath: input, Format: FormatDOCX})
	if err == nil || !strings.Contains(err.Error(), "must not overwrite") {
		t.Fatalf("expected overwrite-protection error, got %v", err)
	}
}

func TestConvertFileCreatesModernDOCXAndResolvesRelativeImage(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.md")
	image := filepath.Join(dir, "tiny.png")
	output := filepath.Join(dir, "sample.docx")

	markdownSource := "# Core Conversion\n\n![pixel](tiny.png)\n\n| A | B |\n|---|---|\n| 1 | 2 |\n"
	if err := os.WriteFile(input, []byte(markdownSource), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
		0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99,
		0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(FileOptions{InputPath: input, OutputPath: output, Format: FormatDOCX})
	if err != nil {
		t.Fatalf("ConvertFile returned error: %v", err)
	}
	if result.OutputPath != output {
		resolvedOutput, resolveErr := filepath.Abs(output)
		if resolveErr != nil || result.OutputPath != resolvedOutput {
			t.Fatalf("OutputPath = %q, want %q", result.OutputPath, output)
		}
	}

	parts := openDOCX(t, output)
	settings := string(parts["word/settings.xml"])
	documentXML := string(parts["word/document.xml"])
	rels := string(parts["word/_rels/document.xml.rels"])

	if !strings.Contains(settings, `w:name="compatibilityMode"`) || !strings.Contains(settings, `w:val="15"`) {
		t.Fatal("generated DOCX does not declare compatibilityMode=15")
	}
	if !strings.Contains(documentXML, `<w:tbl>`) {
		t.Fatal("generated DOCX does not contain native table structure")
	}
	if !strings.Contains(documentXML, `<w:drawing>`) {
		t.Fatal("generated DOCX does not contain image drawing")
	}
	if !strings.Contains(rels, "relationships/image") {
		t.Fatal("generated DOCX does not contain image relationship")
	}
}

func openDOCX(t *testing.T, path string) map[string][]byte {
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
