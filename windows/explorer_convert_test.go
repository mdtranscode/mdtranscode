package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplorerConversionInput(t *testing.T) {
	input := filepath.Join("C:\\", "Docs", "Quarterly Report.md")
	path, handled, err := explorerConversionInput([]string{explorerConvertDOCXFlag, input})
	if err != nil {
		t.Fatalf("explorerConversionInput returned error: %v", err)
	}
	if !handled {
		t.Fatal("explorerConversionInput did not handle the conversion flag")
	}
	if path != input {
		t.Fatalf("path = %q, want %q", path, input)
	}

	path, handled, err = explorerConversionInput([]string{"report.md"})
	if err != nil || handled || path != "" {
		t.Fatalf("non-conversion args = %q, %v, %v; want empty, false, nil", path, handled, err)
	}
}

func TestExplorerConversionInputRequiresPath(t *testing.T) {
	_, handled, err := explorerConversionInput([]string{explorerConvertDOCXFlag})
	if !handled || err == nil || !strings.Contains(err.Error(), "requires a Markdown file path") {
		t.Fatalf("expected handled missing-path error, got handled=%v err=%v", handled, err)
	}
}

func TestNextAvailableDOCXPathDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "report.md")
	if err := os.WriteFile(input, []byte("# Report\n"), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := nextAvailableDOCXPath(input)
	if err != nil {
		t.Fatalf("nextAvailableDOCXPath returned error: %v", err)
	}
	wantFirst := filepath.Join(dir, "report.docx")
	if first != wantFirst {
		t.Fatalf("first output = %q, want %q", first, wantFirst)
	}

	if err := os.WriteFile(wantFirst, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := nextAvailableDOCXPath(input)
	if err != nil {
		t.Fatalf("nextAvailableDOCXPath returned error: %v", err)
	}
	wantSecond := filepath.Join(dir, "report (1).docx")
	if second != wantSecond {
		t.Fatalf("second output = %q, want %q", second, wantSecond)
	}

	if err := os.WriteFile(wantSecond, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	third, err := nextAvailableDOCXPath(input)
	if err != nil {
		t.Fatalf("nextAvailableDOCXPath returned error: %v", err)
	}
	wantThird := filepath.Join(dir, "report (2).docx")
	if third != wantThird {
		t.Fatalf("third output = %q, want %q", third, wantThird)
	}
}

func TestRunExplorerConversionUsesCoreAndPreservesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "Résumé.md")
	if err := os.WriteFile(input, []byte("# Explorer Convert\n\n- one\n- two\n"), 0644); err != nil {
		t.Fatal(err)
	}

	handled, firstOutput, err := runExplorerConversion([]string{explorerConvertDOCXFlag, input})
	if err != nil {
		t.Fatalf("first runExplorerConversion returned error: %v", err)
	}
	if !handled {
		t.Fatal("first runExplorerConversion did not handle the command")
	}
	if firstOutput != filepath.Join(dir, "Résumé.docx") {
		t.Fatalf("first output = %q", firstOutput)
	}

	parts := openWindowsTestDOCX(t, firstOutput)
	if !bytes.Contains(parts["word/document.xml"], []byte("Explorer Convert")) {
		t.Fatal("first DOCX does not contain the converted Markdown content")
	}
	firstBefore, err := os.ReadFile(firstOutput)
	if err != nil {
		t.Fatal(err)
	}

	handled, secondOutput, err := runExplorerConversion([]string{explorerConvertDOCXFlag, input})
	if err != nil {
		t.Fatalf("second runExplorerConversion returned error: %v", err)
	}
	if !handled {
		t.Fatal("second runExplorerConversion did not handle the command")
	}
	if secondOutput != filepath.Join(dir, "Résumé (1).docx") {
		t.Fatalf("second output = %q", secondOutput)
	}

	firstAfter, err := os.ReadFile(firstOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBefore, firstAfter) {
		t.Fatal("second Explorer conversion modified the existing DOCX")
	}
	openWindowsTestDOCX(t, secondOutput)
}

func TestRunExplorerConversionRejectsUnsupportedInput(t *testing.T) {
	handled, _, err := runExplorerConversion([]string{explorerConvertDOCXFlag, "report.txt"})
	if !handled || err == nil || !strings.Contains(err.Error(), "not a supported Markdown file") {
		t.Fatalf("expected unsupported Markdown error, got handled=%v err=%v", handled, err)
	}
}
