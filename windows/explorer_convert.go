package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdtranscode/mdtranscode/core-src/transcode"
)

const explorerConvertDOCXFlag = "--convert-to-docx"

// runExplorerConversion handles the Explorer-only Windows conversion command
// before the Wails application starts. Successful conversions are intentionally
// silent; Explorer will surface the newly-created DOCX beside the Markdown file.
func runExplorerConversion(args []string) (bool, string, error) {
	inputPath, handled, err := explorerConversionInput(args)
	if !handled || err != nil {
		return handled, "", err
	}

	absoluteInput, err := validateMarkdownPath(inputPath)
	if err != nil {
		return true, "", err
	}

	outputPath, err := nextAvailableDOCXPath(absoluteInput)
	if err != nil {
		return true, "", err
	}

	result, err := transcode.ConvertFile(transcode.FileOptions{
		InputPath:  absoluteInput,
		OutputPath: outputPath,
		Format:     transcode.FormatDOCX,
		Creator:    appName,
	})
	if err != nil {
		return true, "", err
	}
	return true, result.OutputPath, nil
}

func explorerConversionInput(args []string) (string, bool, error) {
	for i, arg := range args {
		if strings.TrimSpace(arg) != explorerConvertDOCXFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", true, fmt.Errorf("%s requires a Markdown file path", explorerConvertDOCXFlag)
		}

		path := strings.TrimSpace(strings.Trim(args[i+1], `"`))
		if path == "" {
			return "", true, fmt.Errorf("%s requires a Markdown file path", explorerConvertDOCXFlag)
		}
		return path, true, nil
	}
	return "", false, nil
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
	return "", fmt.Errorf("could not find an available Word output name beside %q", path)
}
