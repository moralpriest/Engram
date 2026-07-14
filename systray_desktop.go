//go:build !android

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/systray"

	"github.com/DEROFDN/engram/i18n"
)

// trayStart and trayEnd manage the desktop system tray lifecycle.
var trayStart, trayEnd func()

// All system tray menu items (visible state depends on wallet connection).
var (
	// Connected-wallet items (shown when engram.Disk != nil)
	trayExit      *systray.MenuItem
	trayContracts *systray.MenuItem
	trayMessages  *systray.MenuItem
	trayTELA      *systray.MenuItem
	trayDashboard *systray.MenuItem

	// Not-logged-in items (shown when engram.Disk == nil)
	trayConnect            *systray.MenuItem
	trayConnectionSettings *systray.MenuItem

	// Settings (shown only when wallet connected, positioned between groups)
	traySettings    *systray.MenuItem
	trayDaemonMiner *systray.MenuItem
	traySeparator   *systray.MenuItem
	trayQuit        *systray.MenuItem
)

// initSystemTray initializes the desktop system tray icon and menu.
// Called from main() after the app and session are initialized.
func initSystemTray() {
	onReady := func() {
		systray.SetIcon(resourceEngramWhitePng.Content())
		systray.SetTitle("Engram")
		systray.SetTooltip("Engram - DERO Wallet")

		// Menu order (top to bottom):
		//   wallet connected: Exit, Contracts+, Messages, TELA, Dashboard, Settings, Daemon & Miner, —, Quit
		//   no wallet:        Connect, Connection Settings, Daemon & Miner, —, Quit		// --- Connected-wallet items ---
		trayExit = systray.AddMenuItem(i18n.T("system_tray.exit"), "")
		trayExit.SetIcon(trayIconExitSvg.Content())
		trayContracts = systray.AddMenuItem(i18n.T("system_tray.contracts"), "")
		trayContracts.SetIcon(trayIconContractsSvg.Content())
		trayMessages = systray.AddMenuItem(i18n.T("system_tray.messages"), "")
		trayMessages.SetIcon(trayIconMessagesSvg.Content())
		trayTELA = systray.AddMenuItem(i18n.T("system_tray.tela"), "")
		trayTELA.SetIcon(trayIconTelaSvg.Content())
		trayDashboard = systray.AddMenuItem(i18n.T("system_tray.dashboard"), "")
		trayDashboard.SetIcon(trayIconDashboardSvg.Content())

		// --- Not-logged-in items (same position) ---
		trayConnect = systray.AddMenuItem(i18n.T("system_tray.connect"), "")
		trayConnect.SetIcon(trayIconConnectSvg.Content())
		trayConnectionSettings = systray.AddMenuItem(i18n.T("system_tray.connection_settings"), "")
		trayConnectionSettings.SetIcon(trayIconConnectionSettingsSvg.Content())

		// --- Wallet-settings item ---
		traySettings = systray.AddMenuItem(i18n.T("system_tray.settings"), "")
		traySettings.SetIcon(trayIconSettingsSvg.Content())

		// --- Always-visible items ---
		trayDaemonMiner = systray.AddMenuItem(i18n.T("system_tray.daemon_miner"), "")
		trayDaemonMiner.SetIcon(trayIconDaemonMinerSvg.Content())
		traySeparator = systray.AddMenuItem("-", "")
		traySeparator.Disable()
		trayQuit = systray.AddMenuItem(i18n.T("system_tray.quit"), "")
		trayQuit.SetIcon(trayIconQuitSvg.Content())

		// Set initial visibility based on current wallet state
		applyTrayVisibility()

		// Wire click handlers
		go func() {
			for {
				select {
				case <-trayExit.ClickedCh:
					fyne.Do(func() {
						if engram.Disk != nil {
							closeWallet()
						}
					})
				case <-trayContracts.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						if engram.Disk != nil {
							session.LastDomain = session.Window.Content()
							session.Domain = "app.files"
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutFilesAndContracts())
						}
					})
				case <-trayMessages.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						if engram.Disk != nil {
							session.LastDomain = session.Window.Content()
							session.Domain = "app.messages"
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutMessages())
						}
					})
				case <-trayTELA.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						if engram.Disk != nil {
							session.LastDomain = session.Window.Content()
							session.Domain = "app.tela"
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutTELA())
						}
					})
				case <-trayDashboard.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						if engram.Disk != nil {
							session.LastDomain = session.Window.Content()
							session.Domain = "app.wallet"
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutDashboard())
						}
					})
				case <-trayConnect.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						session.LastDomain = session.Window.Content()
						session.Domain = "app.main"
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutMain())
					})
				case <-trayConnectionSettings.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						session.LastDomain = session.Window.Content()
						session.Domain = "app.settings"
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutSettings())
					})
				case <-traySettings.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						session.LastDomain = session.Window.Content()
						session.Domain = "app.settings"
						session.Window.SetContent(layoutTransition())
						if engram.Disk != nil {
							session.Window.SetContent(layoutAppSettings())
						} else {
							session.Window.SetContent(layoutSettings())
						}
					})
				case <-trayDaemonMiner.ClickedCh:
					fyne.Do(func() {
						session.Window.Show()
						session.LastDomain = session.Window.Content()
						session.Domain = "app.nodeminer"
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutDaemonMiner())
					})
				case <-trayQuit.ClickedCh:
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

// applyTrayVisibility shows/hides menu items based on wallet connection state.
func applyTrayVisibility() {
	// Skip if app is exiting — DBus connection may already be closed.
	if appExiting {
		return
	}

	walletOpen := engram.Disk != nil

	setVisible := func(items []*systray.MenuItem, show bool) {
		for _, item := range items {
			if show {
				item.Show()
			} else {
				item.Hide()
			}
		}
	}

	setVisible([]*systray.MenuItem{trayExit, trayContracts, trayMessages, trayTELA, trayDashboard}, walletOpen)
	setVisible([]*systray.MenuItem{trayConnect, trayConnectionSettings}, !walletOpen)
	setVisible([]*systray.MenuItem{traySettings}, walletOpen)
	// Daemon & Miner, Separator, and Quit are always visible
}

// updateTrayMenu refreshes the tray menu visibility to match the current wallet state.
// Called from closeWallet() and login() when wallet state changes.
func updateTrayMenu() {
	// Skip if app is exiting — DBus connection may already be closed.
	if appExiting {
		return
	}

	if engram.Disk != nil {
		systray.SetTooltip("Engram - Wallet Connected")
	} else {
		systray.SetTooltip("Engram - DERO Wallet")
	}
	applyTrayVisibility()
}
