package main

import (
	"fmt"
	"image/color"
	"net/url"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

// daemonInfoUI holds refreshable canvas text references for the daemon info panel.
type daemonInfoUI struct {
	mu         sync.Mutex
	status     *canvas.Text
	height     *canvas.Text
	topo       *canvas.Text
	difficulty *canvas.Text
	peers      *canvas.Text
	version    *canvas.Text
	txpool     *canvas.Text
}

var infoUI daemonInfoUI

type daemonMinerState struct {
	daemonState int
	minerState  int
}

var dmState = daemonMinerState{daemonState: 0, minerState: 0}

const (
	dmStateStopped = iota
	dmStateRunning
	dmStateSyncing
	dmStateError
	dmStateExternal
)

func stateColorDM(s int) color.Color {
	switch s {
	case dmStateRunning:
		return apptheme.C.Green
	case dmStateSyncing:
		return apptheme.C.Yellow
	case dmStateError:
		return apptheme.C.Red
	case dmStateExternal:
		return apptheme.C.Green
	default:
		return apptheme.C.Gray
	}
}

func stateLabelDM(s int) string {
	switch s {
	case dmStateRunning:
		return i18n.T("daemon_miner.state_running")
	case dmStateSyncing:
		return i18n.T("daemon_miner.state_syncing")
	case dmStateError:
		return i18n.T("daemon_miner.state_error")
	case dmStateExternal:
		return i18n.T("daemon_miner.state_external")
	default:
		return i18n.T("daemon_miner.state_stopped")
	}
}

func openDownloadURL(urlStr string) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	fyne.CurrentApp().OpenURL(u)
}

// makeSmallStatusRow returns a compact HBox with a tiny icon and label for use
// in cramped spaces such as the network settings page.
func makeSmallStatusRow(icon fyne.Resource, label, status string, statusColor color.Color) fyne.CanvasObject {
	iconWidget := widget.NewIcon(icon)
	iconSizer := container.NewGridWrap(fyne.NewSize(18, 18), iconWidget)

	lbl := canvas.NewText(label+": ", buttonTextColor())
	lbl.TextSize = scaleFont(11)

	statusText := canvas.NewText(status, statusColor)
	statusText.TextSize = scaleFont(11)

	return container.NewHBox(
		iconSizer,
		lbl,
		statusText,
	)
}

func newDaemonMinerToggleButton(label string, iconResource fyne.Resource, state *int, onToggle func()) *fyne.Container {
	iconWidget := widget.NewIcon(iconResource)
	themedIcon := container.NewThemeOverride(iconWidget, apptheme.NewTintTheme(apptheme.Main, stateColorDM(*state)))
	iconSizer := container.NewGridWrap(scalePoint(100, 100), themedIcon)

	stateLabel := canvas.NewText(stateLabelDM(*state), buttonTextColor())
	stateLabel.TextSize = scaleFont(9)
	stateLabel.Alignment = fyne.TextAlignCenter

	labelText := canvas.NewText(label, buttonTextColor())
	labelText.TextSize = scaleFont(11)
	labelText.TextStyle = fyne.TextStyle{Bold: true}
	labelText.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(
		container.NewCenter(iconSizer),
		stateLabel,
		labelText,
	)

	btn := newHoverButton(onToggle, func() {
	}, func() {
	})
	btn.Importance = widget.LowImportance

	sizeEnforcer := canvas.NewRectangle(color.Transparent)
	sizeEnforcer.SetMinSize(fyne.NewSize(ui.Width*0.47, content.MinSize().Height))

	return container.NewStack(sizeEnforcer, content, container.NewMax(btn))
}

var refreshDaemonInfoOnce sync.Once

// startBackgroundDaemonRefresh starts a global background goroutine that
// periodically fetches daemon info and updates the cache. Runs forever.
func startBackgroundDaemonRefresh() {
	refreshDaemonInfoOnce.Do(func() {
		go func() {
			for {
				info, err := fetchDaemonInfo()
				if err == nil && infoUI.status != nil {
					uiDo(func() { updateInfoUILabels(info) })
				}
				time.Sleep(5 * time.Second)
			}
		}()
	})
}

// updateInfoUILabels pushes the fetched info into the current UI label references.
// Must be called on the UI goroutine.
func updateInfoUILabels(info DaemonInfo) {
	infoUI.mu.Lock()
	defer infoUI.mu.Unlock()
	if infoUI.status != nil {
		infoUI.status.Text = stateLabelDM(dmState.daemonState)
		infoUI.status.Color = stateColorDM(dmState.daemonState)
		infoUI.status.Refresh()
	}
	if infoUI.height != nil {
		infoUI.height.Text = fmt.Sprintf("%d", info.Height)
		infoUI.height.Refresh()
	}
	if infoUI.topo != nil {
		infoUI.topo.Text = fmt.Sprintf("%d", info.Topoheight)
		infoUI.topo.Refresh()
	}
	if infoUI.difficulty != nil {
		infoUI.difficulty.Text = fmt.Sprintf("%d", info.Difficulty)
		infoUI.difficulty.Refresh()
	}
	if infoUI.peers != nil {
		infoUI.peers.Text = fmt.Sprintf("%d in / %d out", info.InPeers, info.OutPeers)
		infoUI.peers.Refresh()
	}
	if infoUI.version != nil {
		infoUI.version.Text = info.Version
		infoUI.version.Refresh()
	}
	if infoUI.txpool != nil {
		infoUI.txpool.Text = fmt.Sprintf("%d", info.TxPoolSize)
		infoUI.txpool.Refresh()
	}
}

func layoutDaemonMiner() fyne.CanvasObject {
	updateDaemonStateFromDetection()

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if engram.Disk != nil {
			session.Window.SetContent(layoutDashboard())
		} else {
			session.Window.SetContent(layoutMain())
		}
		removeOverlays()
	})

	heading := canvas.NewText(i18n.T("daemon_miner.heading"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.Alignment = fyne.TextAlignCenter

	daemonBtn := newDaemonMinerToggleButton(i18n.T("daemon_miner.daemon"), daemonIconResource(), &dmState.daemonState, func() {
		switch dmState.daemonState {
		case dmStateStopped, dmStateError:
			go startDaemon()
		default:
			go stopDaemon()
		}
	})
	minerBtn := newDaemonMinerToggleButton(i18n.T("daemon_miner.miner"), minerOffIconResource(), &dmState.minerState, func() {
		switch dmState.minerState {
		case dmStateStopped, dmStateError:
			go startMiner()
		default:
			go stopMiner()
		}
	})

	toggleRow := container.NewHBox(
		layout.NewSpacer(),
		daemonBtn,
		layout.NewSpacer(),
		minerBtn,
		layout.NewSpacer(),
	)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 4))

	baseFontSize := float32(14)
	makeStatRow := func(label string, valueObj fyne.CanvasObject) *fyne.Container {
		lbl := canvas.NewText(label, buttonTextColor())
		lbl.TextSize = scaleFont(baseFontSize)
		return container.NewHBox(lbl, valueObj)
	}
	makeStatValue := func(text string, c color.Color) *canvas.Text {
		t := canvas.NewText(text, c)
		t.TextSize = scaleFont(baseFontSize)
		return t
	}

	// Fetch daemon info (non-blocking, uses cached on error)
	info, infoErr := fetchDaemonInfo()
	if infoErr == nil && dmState.daemonState == dmStateExternal {
		dmState.daemonState = dmStateRunning
	}

	// ---- Daemon Info Panel ----
	daemonSectionLabel := canvas.NewText(i18n.T("daemon_miner.daemon")+" INFO", apptheme.C.Gray)
	daemonSectionLabel.TextSize = scaleFont(14)
	daemonSectionLabel.TextStyle = fyne.TextStyle{Bold: true}

	daemonStatusText := makeStatValue(stateLabelDM(dmState.daemonState), stateColorDM(dmState.daemonState))
	daemonStatusRow := makeStatRow("Status: ", daemonStatusText)

	var daemonHeightText, daemonTopoText, daemonDiffText, daemonPeersText, daemonVersionText, daemonTxPoolText *canvas.Text

	if infoErr == nil {
		daemonHeightText = makeStatValue(fmt.Sprintf("%d", info.Height), apptheme.C.Green)
		daemonTopoText = makeStatValue(fmt.Sprintf("%d", info.Topoheight), apptheme.C.Green)
		daemonDiffText = makeStatValue(fmt.Sprintf("%d", info.Difficulty), apptheme.C.Green)
		daemonPeersText = makeStatValue(fmt.Sprintf("%d in / %d out", info.InPeers, info.OutPeers), apptheme.C.Green)
		daemonVersionText = makeStatValue(info.Version, apptheme.C.Green)
		daemonTxPoolText = makeStatValue(fmt.Sprintf("%d", info.TxPoolSize), apptheme.C.Green)
	} else {
		unavail := makeStatValue("—", apptheme.C.Gray)
		daemonHeightText, daemonTopoText, daemonDiffText = unavail, unavail, unavail
		daemonPeersText, daemonVersionText, daemonTxPoolText = unavail, unavail, unavail
	}

	daemonInfoBox := container.NewVBox(
		daemonSectionLabel,
		daemonStatusRow,
		makeStatRow("Height: ", daemonHeightText),
		makeStatRow("Topoheight: ", daemonTopoText),
		makeStatRow("Difficulty: ", daemonDiffText),
		makeStatRow("Peers: ", daemonPeersText),
		makeStatRow("Version: ", daemonVersionText),
		makeStatRow("Tx Pool: ", daemonTxPoolText),
	)

	infoUI.mu.Lock()
	infoUI.status = daemonStatusText
	infoUI.height = daemonHeightText
	infoUI.topo = daemonTopoText
	infoUI.difficulty = daemonDiffText
	infoUI.peers = daemonPeersText
	infoUI.version = daemonVersionText
	infoUI.txpool = daemonTxPoolText
	infoUI.mu.Unlock()

	daemonSeparator := canvas.NewRectangle(apptheme.C.Gray)
	daemonSeparator.SetMinSize(fyne.NewSize(ui.Width*0.9, 1))

	// ---- Miner Info Panel (basic state for now) ----
	minerSectionLabel := canvas.NewText(i18n.T("daemon_miner.miner")+" INFO", apptheme.C.Gray)
	minerSectionLabel.TextSize = scaleFont(14)
	minerSectionLabel.TextStyle = fyne.TextStyle{Bold: true}

	minerStatusText := makeStatValue(stateLabelDM(dmState.minerState), stateColorDM(dmState.minerState))
	minerStatusRow := makeStatRow("Status: ", minerStatusText)

	minerInfoBox := container.NewVBox(
		minerSectionLabel,
		minerStatusRow,
	)

	minerSeparator := canvas.NewRectangle(apptheme.C.Gray)
	minerSeparator.SetMinSize(fyne.NewSize(ui.Width*0.9, 1))

	// Collapsible wrappers for info panels
	daemonInfoContainer := container.NewVBox(daemonInfoBox)
	minerInfoContainer := container.NewVBox(minerInfoBox)

	btnToggleDaemonDetails := widget.NewButton(i18n.T("daemon_miner.hide_output"), nil)
	btnToggleDaemonDetails.OnTapped = func() {
		daemonInfoContainer.Hidden = !daemonInfoContainer.Hidden
		if daemonInfoContainer.Hidden {
			btnToggleDaemonDetails.SetText(i18n.T("daemon_miner.details"))
		} else {
			btnToggleDaemonDetails.SetText(i18n.T("daemon_miner.hide_output"))
		}
	}

	btnToggleMinerDetails := widget.NewButton(i18n.T("daemon_miner.hide_output"), nil)
	btnToggleMinerDetails.OnTapped = func() {
		minerInfoContainer.Hidden = !minerInfoContainer.Hidden
		if minerInfoContainer.Hidden {
			btnToggleMinerDetails.SetText(i18n.T("daemon_miner.details"))
		} else {
			btnToggleMinerDetails.SetText(i18n.T("daemon_miner.hide_output"))
		}
	}

	// Download buttons shown when binaries are missing
	if findBinary(daemonBinary()) == "" {
		btnDownloadDerod := widget.NewButton(i18n.T("daemon_miner.download_derod"), func() {
			openDownloadURL("https://github.com/deroproject/derohe/releases")
		})
		daemonInfoContainer.Add(container.NewCenter(btnDownloadDerod))
	}
	if findBinary(minerBinary()) == "" {
		btnDownloadMiner := widget.NewButton(i18n.T("daemon_miner.download_miner"), func() {
			openDownloadURL("https://github.com/deroproject/derohe/releases")
		})
		minerInfoContainer.Add(container.NewCenter(btnDownloadMiner))
	}

	topSection := container.NewVBox(
		container.NewCenter(toggleRow),
		rectSpacer,
		container.NewCenter(btnToggleDaemonDetails),
		container.NewCenter(daemonSeparator),
		daemonInfoContainer,
		rectSpacer,
		container.NewCenter(btnToggleMinerDetails),
		container.NewCenter(minerSeparator),
		minerInfoContainer,
	)

	if isMobile() {
		mobileLabel := canvas.NewText(i18n.T("daemon_miner.unavailable"), apptheme.C.Gray)
		mobileLabel.TextSize = scaleFont(10)
		mobileLabel.Alignment = fyne.TextAlignCenter
		mobileOverlay := container.NewCenter(mobileLabel)

		daemonBtn = container.NewStack(daemonBtn, mobileOverlay)
		minerBtn = container.NewStack(minerBtn, mobileOverlay)
		toggleRow = container.NewHBox(
			layout.NewSpacer(),
			daemonBtn,
			layout.NewSpacer(),
			minerBtn,
			layout.NewSpacer(),
		)
		topSection = container.NewVBox(
			container.NewCenter(toggleRow),
			rectSpacer,
			container.NewCenter(btnToggleDaemonDetails),
			container.NewCenter(daemonSeparator),
			daemonInfoContainer,
			rectSpacer,
			container.NewCenter(btnToggleMinerDetails),
			container.NewCenter(minerSeparator),
			minerInfoContainer,
		)
	}

	// Start global background refresh goroutine (runs once, forever)
	startBackgroundDaemonRefresh()

	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))

	scrollContent := container.NewVBox(topSection)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				scrollContent,
			),
			layout.NewSpacer(),
		),
	)
	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(heading),
		rectSpacer,
	)

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	return container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)
}
