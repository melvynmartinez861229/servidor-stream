package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"servidor-stream/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Crear instancia de la aplicación
	application := app.NewApp()

	// Crear opciones de la aplicación Wails
	err := wails.Run(&options.App{
		Title:     "Server Stream",
		Width:     1400,
		Height:    900,
		MinWidth:  1200,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        application.Startup,
		OnShutdown:       application.Shutdown,
		OnDomReady:       application.DomReady,
		Bind: []interface{}{
			application,
		},
		Logger:             nil,
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.INFO,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
