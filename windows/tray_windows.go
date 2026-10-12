//go:build windows

package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

//go:embed tray.ico
var trayResources embed.FS

const (
	wmTray               = 0x8001
	wmCommand            = 0x0111
	wmDestroy            = 0x0002
	wmRButtonUp          = 0x0205
	wmLButtonDoubleClick = 0x0203
	wmContextMenu        = 0x007b
	nimAdd               = 0
	nimDelete            = 2
	nifMessage           = 1
	nifIcon              = 2
	nifTip               = 4
	mfString             = 0
	mfSeparator          = 0x800
	mfGray               = 1
	mfChecked            = 8
	tpmRightButton       = 2
	tpmBottomAlign       = 0x20
	imageIcon            = 1
	lrLoadFromFile       = 0x10
	lrDefaultSize        = 0x40
	idOpen               = 101
	idSettings           = 102
	idWatchFolder        = 103
	idOutputFolder       = 104
	idPause              = 105
	idScan               = 106
	idHistory            = 107
	idUpdate             = 108
	idStartup            = 109
	idExit               = 110
)

type point struct{ x, y int32 }
type windowMsg struct {
	hwnd           uintptr
	message        uint32
	_              uint32
	wParam, lParam uintptr
	time           uint32
	pt             point
	private        uint32
}
type wndClassEx struct {
	size, style                        uint32
	wndProc                            uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menuName, className                *uint16
	iconSmall                          uintptr
}
type notifyIconData struct {
	size                uint32
	hwnd                uintptr
	id, flags, callback uint32
	icon                uintptr
	tip                 [128]uint16
	state, stateMask    uint32
	info                [256]uint16
	timeout             uint32
	infoTitle           [64]uint16
	infoFlags           uint32
	guid                [16]byte
	balloonIcon         uintptr
}
type trayController struct {
	app   *App
	hwnd  uintptr
	icon  uintptr
	ready chan error
}

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	shell32             = syscall.NewLazyDLL("shell32.dll")
	kernel32Tray        = syscall.NewLazyDLL("kernel32.dll")
	registerClassEx     = user32.NewProc("RegisterClassExW")
	createWindowEx      = user32.NewProc("CreateWindowExW")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	postMessage         = user32.NewProc("PostMessageW")
	loadImage           = user32.NewProc("LoadImageW")
	destroyIcon         = user32.NewProc("DestroyIcon")
	createPopupMenu     = user32.NewProc("CreatePopupMenu")
	appendMenu          = user32.NewProc("AppendMenuW")
	destroyMenu         = user32.NewProc("DestroyMenu")
	trackPopupMenu      = user32.NewProc("TrackPopupMenu")
	getCursorPos        = user32.NewProc("GetCursorPos")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getModuleHandle     = kernel32Tray.NewProc("GetModuleHandleW")
	shellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
)

// The window callback must be kept alive for the lifetime of the native window.
var activeTray *trayController
var trayCallback = syscall.NewCallback(func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if activeTray != nil && activeTray.hwnd == hwnd {
		switch message {
		case wmTray:
			switch uint32(lParam) {
			case wmRButtonUp, wmContextMenu:
				activeTray.popup()
			case wmLButtonDoubleClick:
				go activeTray.app.showApp("")
			}
			return 0
		case wmCommand:
			id := uint32(wParam & 0xffff)
			go activeTray.app.trayCommand(id)
			return 0
		case wmDestroy:
			postQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
})

func startTray(app *App) (*trayController, error) {
	t := &trayController{app: app, ready: make(chan error, 1)}
	go t.run()
	select {
	case err := <-t.ready:
		if err != nil {
			return nil, err
		}
		return t, nil
	case <-time.After(5 * time.Second):
		return nil, errors.New("tray initialization timed out")
	}
}
func (t *trayController) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	klass, _ := syscall.UTF16PtrFromString(fmt.Sprintf("MDTranscodeTray-%d", os.Getpid()))
	mod, _, _ := getModuleHandle.Call(0)
	wc := wndClassEx{size: uint32(unsafe.Sizeof(wndClassEx{})), wndProc: trayCallback, instance: mod, className: klass}
	ok, _, _ := registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if ok == 0 {
		t.ready <- errors.New("cannot register tray window class")
		return
	}
	hwnd, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(klass)), uintptr(unsafe.Pointer(klass)), 0, 0, 0, 0, 0, 0, 0, mod, 0)
	if hwnd == 0 {
		t.ready <- errors.New("cannot create tray window")
		return
	}
	t.hwnd = hwnd
	activeTray = t
	iconData, err := trayResources.ReadFile("tray.ico")
	if err == nil {
		file, e := os.CreateTemp("", "mdtranscode-tray-*.ico")
		if e == nil {
			_, err = file.Write(iconData)
			_ = file.Close()
			if err == nil {
				name, _ := syscall.UTF16PtrFromString(file.Name())
				t.icon, _, _ = loadImage.Call(0, uintptr(unsafe.Pointer(name)), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize)
			}
			_ = os.Remove(file.Name())
		}
	}
	if t.icon == 0 {
		t.ready <- errors.New("cannot load MDTranscode tray icon")
		destroyWindow.Call(hwnd)
		activeTray = nil
		return
	}
	nid := t.notifyData()
	value, _, _ := shellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if value == 0 {
		t.ready <- errors.New("Windows rejected MDTranscode tray icon")
		destroyIcon.Call(t.icon)
		destroyWindow.Call(hwnd)
		activeTray = nil
		return
	}
	t.ready <- nil
	var msg windowMsg
	for {
		code, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(code) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	shellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	destroyIcon.Call(t.icon)
	activeTray = nil
}
func (t *trayController) notifyData() notifyIconData {
	nid := notifyIconData{size: uint32(unsafe.Sizeof(notifyIconData{})), hwnd: t.hwnd, id: 1, flags: nifMessage | nifIcon | nifTip, callback: wmTray, icon: t.icon}
	copy(nid.tip[:], syscall.StringToUTF16("MDTranscode - Automatic Word conversion"))
	return nid
}
func (t *trayController) popup() {
	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		return
	}
	defer destroyMenu.Call(menu)
	status := t.app.watcher.snapshot()
	opts, err := t.app.store.settings()
	if err != nil {
		opts = defaultPersistedState().Settings
	}
	condition := "Monitoring disabled"
	if status.Running {
		condition = "Monitoring active"
	} else if status.Paused {
		condition = "Monitoring paused"
	}
	t.item(menu, 0, condition, mfGray)
	t.sep(menu)
	t.item(menu, idOpen, "Open MDTranscode", 0)
	t.item(menu, idSettings, "Settings", 0)
	t.item(menu, idWatchFolder, "Open watched folder", disabledIf(opts.Watcher.WatchFolder == ""))
	t.item(menu, idOutputFolder, "Open converted folder", disabledIf(opts.Watcher.OutputFolder == ""))
	t.sep(menu)
	flags := uint32(0)
	if !status.Enabled {
		flags = mfGray
	} else if status.Paused {
		flags = mfChecked
	}
	t.item(menu, idPause, "Pause monitoring", flags)
	t.item(menu, idScan, "Scan for Markdown now", disabledIf(!status.Running))
	t.item(menu, idHistory, fmt.Sprintf("Recent conversions/errors (%d / %d)", status.Converted, status.Errors), 0)
	t.sep(menu)
	t.item(menu, idUpdate, "Microsoft Store updates", 0)
	t.item(menu, idStartup, "Windows startup settings", 0)
	t.sep(menu)
	t.item(menu, idExit, "Exit MDTranscode", 0)
	var position point
	getCursorPos.Call(uintptr(unsafe.Pointer(&position)))
	setForegroundWindow.Call(t.hwnd)
	trackPopupMenu.Call(menu, tpmRightButton|tpmBottomAlign, uintptr(position.x), uintptr(position.y), 0, t.hwnd, 0)
	postMessage.Call(t.hwnd, 0, 0, 0)
}
func disabledIf(v bool) uint32 {
	if v {
		return mfGray
	}
	return 0
}
func (t *trayController) sep(menu uintptr) { appendMenu.Call(menu, mfSeparator, 0, 0) }
func (t *trayController) item(menu uintptr, id uint32, label string, flags uint32) {
	text, _ := syscall.UTF16PtrFromString(label)
	appendMenu.Call(menu, uintptr(mfString|flags), uintptr(id), uintptr(unsafe.Pointer(text)))
}
func (t *trayController) stop() {
	if t != nil && t.hwnd != 0 {
		postMessage.Call(t.hwnd, wmDestroy, 0, 0)
	}
}
