package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	handled, _, conversionErr := runExplorerConversion(os.Args[1:])
	if handled {
		if conversionErr != nil {
			showExplorerConversionError(conversionErr.Error())
			os.Exit(1)
		}
		return
	}

	app := NewApp()
	backgroundLaunch := false
	for _, arg := range os.Args[1:] {
		if arg == "--startup" || arg == "--background" {
			backgroundLaunch = true
		}
	}

	err := wails.Run(&options.App{
		Title:       appName,
		StartHidden: backgroundLaunch,
		Width:       1180,
		Height:      760,
		MinWidth:    860,
		MinHeight:   560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 251, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               singleInstanceID,
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
