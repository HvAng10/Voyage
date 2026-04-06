package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"voyage/internal/scheduler"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "Voyage · 远航",
		Width:             1440,
		Height:            900,
		MinWidth:          1200,
		MinHeight:         700,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 240, G: 242, B: 245, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			// 数据库初始化成功后启动定时调度器
			if app.db != nil {
				app.sched = scheduler.New(app.db, app.alertsSvc, app.loadCredentials)
				app.sched.Start(ctx)
			}
		},
		OnShutdown: func(ctx context.Context) {
			if app.sched != nil {
				app.sched.Stop()
			}
			app.shutdown(ctx)
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisablePinchZoom:     true,
			DisableWindowIcon:    false,
			IsZoomControlEnabled: false,
			EnableSwipeGestures:  false,
			ResizeDebounceMS:     0,
			Theme:                windows.Light,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:   windows.RGB(26, 39, 68),
				DarkModeTitleText:  windows.RGB(200, 212, 232),
				DarkModeBorder:     windows.RGB(26, 39, 68),
				LightModeTitleBar:  windows.RGB(26, 39, 68),
				LightModeTitleText: windows.RGB(255, 255, 255),
				LightModeBorder:    windows.RGB(26, 39, 68),
			},
			BackdropType: windows.None,
		},
	})

	if err != nil {
		println("错误:", err.Error())
	}
}
