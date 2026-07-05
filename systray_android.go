//go:build android

package main

// trayStart and trayEnd are no-ops on Android (no system tray).
var trayStart, trayEnd func()

func initSystemTray() {
	// System tray is not available on Android.
}
