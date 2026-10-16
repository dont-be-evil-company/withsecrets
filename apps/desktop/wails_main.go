//go:build wails

package main

import (
	_ "embed"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed icon.png
var trayIcon []byte

// Run with: go build -tags wails,fido2 -o dist/withsecrets-desktop ./apps/desktop
// Wails v3. This file is not part of the default test build.
func main() {
	h := newHost()
	h.watchIdle()
	var quitting atomic.Bool
	app := application.New(application.Options{
		Name:        "withsecrets",
		Description: "withsecrets vault",
		Services: []application.Service{
			application.NewService(h.svc),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(desktopFiles),
			Middleware: localAPI(h),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.mistweaverco.withsecrets",
		},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "withsecrets",
		Width:  1200,
		Height: 800,
		URL:    "/",
	})
	menu := application.NewMenu()
	menu.Add("Open").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("Lock").OnClick(func(*application.Context) {
		h.svc.Lock()
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) {
		quitting.Store(true)
		app.Quit()
	})
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("withsecrets")
	tray.SetMenu(menu)
	tray.OnClick(func() {
		window.Show()
		window.Focus()
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if quitting.Load() || !h.svc.Settings().CloseToTray {
			return
		}
		event.Cancel()
		window.Hide()
	})
	app.Run()
}
