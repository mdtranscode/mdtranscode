//go:build !windows

package main

import "errors"

type trayController struct{}

func startTray(_ *App) (*trayController, error) { return nil, errors.New("tray requires Windows") }
func (t *trayController) stop()                 {}
func openFolder(_ string) error                 { return errors.New("open folder requires Windows") }
func openWindowsStartupSettings() error {
	return errors.New("Windows startup settings require Windows")
}

// Tray action identifiers are mirrored here for non-Windows source test builds.
const (
	idOpen         = 101
	idSettings     = 102
	idWatchFolder  = 103
	idOutputFolder = 104
	idPause        = 105
	idScan         = 106
	idHistory      = 107
	idUpdate       = 108
	idStartup      = 109
	idExit         = 110
)
