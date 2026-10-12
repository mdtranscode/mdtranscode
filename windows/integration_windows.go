//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func openFolder(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("folder has not been configured")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	if !info.IsDir() {
		return errors.New("configured path is not a directory")
	}
	return exec.Command("explorer.exe", path).Start()
}
func openWindowsStartupSettings() error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "ms-settings:startupapps").Start()
}
