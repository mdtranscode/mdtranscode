package transcode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdtranscode/mdtranscode/core-src/markdown"
	"github.com/mdtranscode/mdtranscode/core-src/renderer/docx"
)

const (
	// FormatDOCX is the currently supported file conversion format.
	FormatDOCX = "docx"
)

// FileOptions describes one Markdown file conversion.
type FileOptions struct {
	InputPath  string
	OutputPath string
	Format     string
	Title      string
	Creator    string
}

// FileResult describes the resolved paths and format used for a successful conversion.
type FileResult struct {
	InputPath  string
	OutputPath string
	Format     string
}

// DefaultOutputPath returns the native-path output filename beside the input file.
func DefaultOutputPath(inputPath, format string) string {
	format = normalizeFormat(format)
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(inputPath, ext)
	if base == "" {
		base = inputPath
	}
	return filepath.Clean(base + "." + format)
}

// ConvertFile reads Markdown from disk and renders it using the Core pipeline.
func ConvertFile(options FileOptions) (FileResult, error) {
	format := normalizeFormat(options.Format)
	if format != FormatDOCX {
		return FileResult{}, fmt.Errorf("unsupported output format %q; currently supported: docx", format)
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return FileResult{}, errors.New("input Markdown file is required")
	}

	inputAbs, err := filepath.Abs(options.InputPath)
	if err != nil {
		return FileResult{}, fmt.Errorf("resolve input path: %w", err)
	}

	outputPath := options.OutputPath
	if strings.TrimSpace(outputPath) == "" {
		outputPath = DefaultOutputPath(inputAbs, format)
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return FileResult{}, fmt.Errorf("resolve output path: %w", err)
	}

	if samePath(inputAbs, outputAbs) {
		return FileResult{}, errors.New("output path must not overwrite the input Markdown file")
	}

	info, err := os.Stat(inputAbs)
	if err != nil {
		return FileResult{}, fmt.Errorf("open input %q: %w", options.InputPath, err)
	}
	if info.IsDir() {
		return FileResult{}, fmt.Errorf("input %q is a directory, not a Markdown file", options.InputPath)
	}

	source, err := os.ReadFile(inputAbs)
	if err != nil {
		return FileResult{}, fmt.Errorf("read input %q: %w", options.InputPath, err)
	}

	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(inputAbs), filepath.Ext(inputAbs))
	}
	creator := strings.TrimSpace(options.Creator)
	if creator == "" {
		creator = "MDTranscode"
	}

	parsed := markdown.Parse(string(source))
	renderOptions := docx.Options{
		BaseDir: filepath.Dir(inputAbs),
		Title:   title,
		Creator: creator,
	}

	if err := docx.WriteFile(parsed, outputAbs, renderOptions); err != nil {
		return FileResult{}, fmt.Errorf("write DOCX %q: %w", outputPath, err)
	}

	return FileResult{
		InputPath:  inputAbs,
		OutputPath: outputAbs,
		Format:     format,
	}, nil
}

func normalizeFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return FormatDOCX
	}
	return format
}

func samePath(a, b string) bool {
	cleanA := filepath.Clean(a)
	cleanB := filepath.Clean(b)
	if strings.EqualFold(cleanA, cleanB) {
		return true
	}

	aInfo, aErr := os.Stat(cleanA)
	bInfo, bErr := os.Stat(cleanB)
	if aErr == nil && bErr == nil {
		return os.SameFile(aInfo, bInfo)
	}
	return false
}
