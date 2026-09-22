package main

import (
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const singleInstanceID = "c6892d2d-7b86-4db4-9dd4-5d953183293e"
const openMarkdownEventName = "mdtranscode:open-markdown"

// onSecondInstanceLaunch routes a Markdown file opened by Explorer to the existing app window.
func (a *App) onSecondInstanceLaunch(data options.SecondInstanceData) {
	if a == nil || a.ctx == nil {
		return
	}

	path := resolveMarkdownArgument(data.Args, data.WorkingDirectory)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	if path != "" {
		runtime.EventsEmit(a.ctx, openMarkdownEventName, path)
	}
}

func resolveMarkdownArgument(args []string, workingDirectory string) string {
	path := markdownArgument(args)
	if path == "" || filepath.IsAbs(path) {
		return path
	}

	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory == "" {
		return path
	}
	return filepath.Clean(filepath.Join(workingDirectory, path))
}
