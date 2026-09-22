package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMarkdownArgumentUsesSecondInstanceWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	got := resolveMarkdownArgument([]string{"report.md"}, workingDirectory)
	want := filepath.Join(workingDirectory, "report.md")
	if got != want {
		t.Fatalf("resolveMarkdownArgument = %q, want %q", got, want)
	}
}

func TestResolveMarkdownArgumentPreservesAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.markdown")
	got := resolveMarkdownArgument([]string{path}, t.TempDir())
	if got != path {
		t.Fatalf("resolveMarkdownArgument = %q, want %q", got, path)
	}
}

func TestResolveMarkdownArgumentRejectsUnsupportedFile(t *testing.T) {
	if got := resolveMarkdownArgument([]string{"report.txt"}, t.TempDir()); got != "" {
		t.Fatalf("resolveMarkdownArgument accepted unsupported file: %q", got)
	}
}

func TestWailsConfigDeclaresMarkdownFileAssociations(t *testing.T) {
	raw, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("read wails.json: %v", err)
	}

	var config struct {
		Info struct {
			ProductVersion   string `json:"productVersion"`
			FileAssociations []struct {
				Ext         string `json:"ext"`
				Name        string `json:"name"`
				Description string `json:"description"`
				IconName    string `json:"iconName"`
				Role        string `json:"role"`
			} `json:"fileAssociations"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse wails.json: %v", err)
	}
	if config.Info.ProductVersion != appVersion {
		t.Fatalf("wails productVersion = %q, want %q", config.Info.ProductVersion, appVersion)
	}

	associations := make(map[string]bool)
	for _, association := range config.Info.FileAssociations {
		if association.Name != "MDTranscode.Markdown" || association.Description != "Markdown Document" || association.IconName != "appicon" || association.Role != "Editor" {
			t.Fatalf("invalid file association: %#v", association)
		}
		associations[strings.ToLower(association.Ext)] = true
	}
	for _, ext := range []string{"md", "markdown"} {
		if !associations[ext] {
			t.Fatalf("wails.json does not declare .%s association", ext)
		}
	}
}

func TestPermanentWindowsPackagingInputsExist(t *testing.T) {
	iconPath := filepath.Join("build", "appicon.png")
	info, err := os.Stat(iconPath)
	if err != nil {
		t.Fatalf("stat %s: %v", iconPath, err)
	}
	if info.Size() < 1024 {
		t.Fatalf("%s is unexpectedly small: %d bytes", iconPath, info.Size())
	}

	manifestPath := filepath.Join("build", "windows", "wails.exe.manifest")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	if !strings.Contains(string(raw), "longPathAware") || !strings.Contains(string(raw), ">true<") {
		t.Fatalf("%s does not enable longPathAware", manifestPath)
	}
}
