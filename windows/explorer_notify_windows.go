//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var explorerUser32 = syscall.NewLazyDLL("user32.dll")
var explorerMessageBoxW = explorerUser32.NewProc("MessageBoxW")

func showExplorerConversionError(message string) {
	title, titleErr := syscall.UTF16PtrFromString(appName)
	text, textErr := syscall.UTF16PtrFromString("Could not convert Markdown to Word.\n\n" + message)
	if titleErr != nil || textErr != nil {
		return
	}

	const mbOK = 0x00000000
	const mbIconError = 0x00000010
	explorerMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		mbOK|mbIconError,
	)
}
