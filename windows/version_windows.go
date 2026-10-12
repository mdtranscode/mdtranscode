package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var getCurrentPackageFullName = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")

// Windows returns an AppModel error for ordinary unpackaged development builds.
// Display the authoritative MSIX version only when running inside a package.
func displayVersion() string {
	var size uint32
	status, _, _ := getCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&size)), 0)
	if uint32(status) != 122 || size < 2 || size > 32768 { // ERROR_INSUFFICIENT_BUFFER
		return appVersion
	}
	chars := make([]uint16, size)
	status, _, _ = getCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&chars[0])))
	if status != 0 {
		return appVersion
	}
	full := syscall.UTF16ToString(chars)
	parts := strings.Split(full, "_")
	if len(parts) < 2 || !validPackageVersion(parts[1]) {
		return appVersion
	}
	return parts[1]
}

func validPackageVersion(v string) bool {
	var a, b, c, d uint32
	n, err := fmt.Sscanf(v, "%d.%d.%d.%d", &a, &b, &c, &d)
	return err == nil && n == 4 && a <= 65535 && b <= 65535 && c <= 65535 && d <= 65535 && fmt.Sprintf("%d.%d.%d.%d", a, b, c, d) == v
}
