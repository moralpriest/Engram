//go:build android

package main

// trayStart and trayEnd are no-ops on Android (no system tray).
var trayStart, trayEnd func()

func initSystemTray() {
	// System tray is not available on Android.
}

// updateTrayMenu is a no-op on Android (no system tray).
func updateTrayMenu() {
	// No system tray on Android.
}

// updateTrayLanguage is a no-op on Android (no system tray).
func updateTrayLanguage() {
	// No system tray on Android.
}
