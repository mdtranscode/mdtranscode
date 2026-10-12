//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
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

// openExportedFile uses the Windows default application for this PDF/DOCX.
// ShellExecuteW avoids cmd.exe/start and their command-injection pitfalls.
func openExportedFile(path string) error {
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("open exported document: %w", err)
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
	result, _, callErr := proc.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, 1)
	if result <= 32 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return fmt.Errorf("open exported document: %w", callErr)
		}
		return fmt.Errorf("Windows could not open the document (ShellExecute code %d)", result)
	}
	return nil
}

// revealExportedFile selects the file in Explorer rather than opening a folder
// containing many similarly named exports.
func revealExportedFile(path string) error {
	if err := exec.Command("explorer.exe", "/select,", path).Start(); err != nil {
		return fmt.Errorf("show exported document in Explorer: %w", err)
	}
	return nil
}
