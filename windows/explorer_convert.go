package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdtranscode/mdtranscode/core-src/transcode"
)

const (
	explorerConvertDOCXFlag = "--convert-to-docx"
	explorerConvertPDFFlag  = "--convert-to-pdf"
)

// runExplorerConversion handles the Explorer-only Windows conversion command
// before the Wails application starts. Successful conversions are intentionally
// silent; Explorer will surface the newly-created document beside the Markdown file.
func runExplorerConversion(args []string) (bool, string, error) {
	inputPath, format, handled, err := explorerConversionInput(args)
	if !handled || err != nil {
		return handled, "", err
	}

	absoluteInput, err := validateMarkdownPath(inputPath)
	if err != nil {
		return true, "", err
	}

	outputPath, err := nextAvailablePath(transcode.DefaultOutputPath(absoluteInput, format))
	if err != nil {
		return true, "", err
	}

	convertOptions := transcode.FileOptions{
		InputPath:  absoluteInput,
		OutputPath: outputPath,
		Format:     format,
		Creator:    appName,
	}
	if format == transcode.FormatPDF {
		settings, settingsErr := defaultStateStore().settings()
		if settingsErr != nil {
			return true, "", fmt.Errorf("load PDF settings: %w", settingsErr)
		}
		convertOptions.PDF = settings.PDF
	}
	result, err := transcode.ConvertFile(convertOptions)
	if err != nil {
		return true, "", err
	}
	return true, result.OutputPath, nil
}

func explorerConversionInput(args []string) (string, string, bool, error) {
	for i, arg := range args {
		format := transcode.FormatDOCX
		if strings.TrimSpace(arg) == explorerConvertPDFFlag {
			format = transcode.FormatPDF
		} else if strings.TrimSpace(arg) != explorerConvertDOCXFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", format, true, fmt.Errorf("%s requires a Markdown file path", arg)
		}

		path := strings.TrimSpace(strings.Trim(args[i+1], `"`))
		if path == "" {
			return "", format, true, fmt.Errorf("%s requires a Markdown file path", arg)
		}
		return path, format, true, nil
	}
	return "", "", false, nil
}

func nextAvailableDOCXPath(inputPath string) (string, error) {
	defaultPath := transcode.DefaultOutputPath(inputPath, transcode.FormatDOCX)
	return nextAvailablePath(defaultPath)
}

func nextAvailablePath(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect output path %q: %w", path, err)
	}

	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	for i := 1; i <= 9999; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, extension)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect output path %q: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("could not find an available output name beside %q", path)
}
