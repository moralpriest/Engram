//go:build !android

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/systray"

	"github.com/DEROFDN/engram/i18n"
)

// trayStart and trayEnd manage the desktop system tray lifecycle.
var trayStart, trayEnd func()

// initSystemTray initializes the desktop system tray icon and menu.
// Called from main() after the app and session are initialized.
func initSystemTray() {
	onReady := func() {
		systray.SetIcon(resourceIconPng.Content())
		systray.SetTitle("Engram")
		systray.SetTooltip("Engram - DERO Wallet")

		showItem := systray.AddMenuItem(i18n.T("system_tray.show_engram"), "")
		systray.AddSeparator()
		quitItem := systray.AddMenuItem(i18n.T("system_tray.quit"), "")

		go func() {
			for {
				select {
				case <-showItem.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
					})
				case <-quitItem.ClickedCh:
					fyne.Do(func() {
						if session.Window != nil {
							showQuitConfirmation()
						}
					})
				}
			}
		}()
	}

	trayStart, trayEnd = systray.RunWithExternalLoop(onReady, nil)
	trayStart()
}
