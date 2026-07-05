package main

import (
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

// minerStatsUI holds canvas.Text references for the miner statistics section.
type minerStatsUI struct {
	mu              sync.Mutex
	hashrate        *canvas.Text
	netHashrate     *canvas.Text
	netShare        *canvas.Text
	minisPerDay     *canvas.Text
	eta             *canvas.Text
	miniBlocks      *canvas.Text
	lastReward      *canvas.Text
	sessionDuration *canvas.Text
	rejected        *canvas.Text
}

var minerStatsUIInstance minerStatsUI

type daemonMinerState struct {
	daemonState int
	minerState  int
}

var dmState = daemonMinerState{daemonState: 0, minerState: 0}

var (
	daemonToggle   *toggleSwitch
	minerToggle    *toggleSwitch
	daemonStateImg *canvas.Image
	minerStateImg  *canvas.Image
	daemonStateLbl *canvas.Text
	minerStateLbl  *canvas.Text
)

const (
	dmStateStopped = iota
	dmStateRunning
	dmStateSyncing
	dmStateError
	dmStateExternal
	dmStateCorrupt
)

func stateColorDM(s int) color.Color {
	switch s {
	case dmStateRunning:
		return apptheme.C.Green
	case dmStateSyncing:
		return apptheme.C.Yellow
	case dmStateError, dmStateCorrupt:
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
	case dmStateCorrupt:
		return i18n.T("daemon_miner.state_corrupt")
	default:
		return i18n.T("daemon_miner.state_stopped")
	}
}

func daemonIsRunning() bool {
	return dmState.daemonState == dmStateRunning ||
		dmState.daemonState == dmStateExternal ||
		dmState.daemonState == dmStateSyncing
}

func minerIsRunning() bool {
	return dmState.minerState == dmStateRunning
}

func syncToggleStates() {
	if daemonToggle != nil {
		daemonToggle.setChecked(daemonIsRunning())
	}
	if minerToggle != nil {
		minerToggle.setChecked(minerIsRunning())
	}
	syncStateIndicators()
}

// syncStateIndicators updates the state indicator icons and labels to match
// the current dmState values. Must be called on the UI goroutine.
func syncStateIndicators() {
	if daemonStateLbl != nil {
		daemonStateLbl.Text = stateLabelDM(dmState.daemonState)
		daemonStateLbl.Color = stateColorDM(dmState.daemonState)
		daemonStateLbl.Refresh()
	}
	if daemonStateImg != nil {
		daemonStateImg.Resource = daemonIconForState(dmState.daemonState)
		daemonStateImg.Refresh()
	}
	if minerStateLbl != nil {
		minerStateLbl.Text = stateLabelDM(dmState.minerState)
		minerStateLbl.Color = stateColorDM(dmState.minerState)
		minerStateLbl.Refresh()
	}
	if minerStateImg != nil {
		minerStateImg.Resource = minerIconForState(dmState.minerState)
		minerStateImg.Refresh()
	}
}

func openDownloadURL(urlStr string) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	fyne.CurrentApp().OpenURL(u)
}

// showBinaryDownloadConfirm asks the user whether to download a missing binary,
// then launches the download progress dialog.  onComplete is called on success.
func showBinaryDownloadConfirm(_ string, onComplete func()) {
	name := daemonBinary()

	msg := fmt.Sprintf(i18n.T("daemon_miner.download_prompt"), name)
	dlg := dialog.NewConfirm(i18n.T("daemon_miner.download_title"), msg, func(confirmed bool) {
		if confirmed {
			showBinaryDownloadProgress(name, onComplete, nil)
		}
	}, session.Window)
	dlg.Show()
}

// showBinaryDownloadProgress displays a progress dialog and downloads the binary
// in the background.  onComplete is called (on the UI goroutine) on success;
// onError is called on failure (before the error dialog is shown).
func showBinaryDownloadProgress(name string, onComplete func(), onError func()) {
	source := daemonDownloadSource

	// No auto-download source configured – open browser instead.
	if source.Owner == "" || source.Repo == "" {
		openDownloadURL("https://github.com/deroproject/derohe/releases")
		return
	}

	progress := widget.NewProgressBar()
	status := widget.NewLabel(i18n.T("common.loading"))

	content := container.NewVBox(status, progress)
	dlg := dialog.NewCustomWithoutButtons(i18n.T("daemon_miner.download_title"), content, session.Window)
	dlg.Show()

	go func() {
		destDir := filepath.Join(AppPath(), "bin")
		err := downloadLatestBinary(source.Owner, source.Repo, name, destDir, func(down, total int64) {
			uiDo(func() {
				if total > 0 {
					progress.SetValue(float64(down) / float64(total))
				}
			})
		})

		uiDo(func() {
			dlg.Hide()
			if err != nil {
				if onError != nil {
					onError()
				}
				errDlg := dialog.NewError(fmt.Errorf(i18n.T("daemon_miner.download_failed")+": %v", err), session.Window)
				errDlg.Show()
			} else if onComplete != nil {
				onComplete()
			}
		})
	}()
}

func newStateIndicator(state *int, label string, res fyne.Resource, indicatorWidth float32) fyne.CanvasObject {
	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(40, 40))

	stateLabel := canvas.NewText(stateLabelDM(*state), stateColorDM(*state))
	stateLabel.TextSize = scaleFont(12)
	stateLabel.Alignment = fyne.TextAlignCenter

	// Store references for live updates via syncStateIndicators()
	if label == i18n.T("daemon_miner.daemon") {
		daemonStateImg = img
		daemonStateLbl = stateLabel
	} else if label == i18n.T("daemon_miner.miner") {
		minerStateImg = img
		minerStateLbl = stateLabel
	}

	labelText := canvas.NewText(label, buttonTextColor())
	labelText.TextSize = scaleFont(14)
	labelText.TextStyle = fyne.TextStyle{Bold: true}
	labelText.Alignment = fyne.TextAlignCenter

	widthAnchor := canvas.NewRectangle(color.Transparent)
	widthAnchor.SetMinSize(fyne.NewSize(indicatorWidth, 1))

	return container.NewVBox(
		widthAnchor,
		container.NewCenter(container.NewGridWrap(fyne.NewSize(50, 50), img)),
		stateLabel,
		labelText,
	)
}

func newRectSpacer() *canvas.Rectangle {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(10, 4))
	return r
}

var refreshDaemonInfoOnce sync.Once

// startBackgroundDaemonRefresh starts a global background goroutine that
// periodically fetches daemon info and updates the cache. Runs forever.
func startBackgroundDaemonRefresh() {
	refreshDaemonInfoOnce.Do(func() {
		go func() {
			for {
				info, err := fetchDaemonInfo()
				if err == nil {
					uiDo(func() { updateInfoUILabels(info) })
				}
				uiDo(syncToggleStates)
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

var refreshMinerStatsOnce sync.Once

// startMinerStatsRefresh starts a background goroutine that periodically
// reads mining stats and refreshes the UI.
func startMinerStatsRefresh() {
	refreshMinerStatsOnce.Do(func() {
		go func() {
			for {
				time.Sleep(2 * time.Second)
				uiDo(func() {
					updateMinerStatsUI()
				})
			}
		}()
	})
}

// updateMinerStatsUI updates the miner statistics canvas text elements with the latest stats.
func updateMinerStatsUI() {
	stats := GetMiningStats()

	// Fallback: use daemon's direct hashrate estimate, or derive from difficulty
	if stats.NetHashrate <= 0 {
		info := getCachedDaemonInfo()
		if info.Hashrate1hr > 0 {
			stats.NetHashrate = info.Hashrate1hr
			stats.NetHashStr = formatHashrate(stats.NetHashrate)
		} else if info.Difficulty > 0 {
			stats.NetHashrate = float64(info.Difficulty) / 1.8
			stats.NetHashStr = formatHashrate(stats.NetHashrate)
		}
	}

	minerStatsUIInstance.mu.Lock()
	defer minerStatsUIInstance.mu.Unlock()

	// Format session duration
	var durationStr string
	if d := stats.SessionDuration(); d > 0 {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		s := int(d.Seconds()) % 60
		if h > 0 {
			durationStr = fmt.Sprintf("%dh %dm", h, m)
		} else if m > 0 {
			durationStr = fmt.Sprintf("%dm %ds", m, s)
		} else {
			durationStr = fmt.Sprintf("%ds", s)
		}
	} else {
		durationStr = "—"
	}

	// Format ETA
	var etaStr string
	if d := stats.ETA(); d > 0 {
		if d < time.Hour {
			m := int(d.Minutes())
			s := int(d.Seconds()) % 60
			etaStr = fmt.Sprintf("~%dm %ds", m, s)
		} else if d < 24*time.Hour {
			h := int(d.Hours())
			m := int(d.Minutes()) % 60
			etaStr = fmt.Sprintf("~%dh %dm", h, m)
		} else {
			days := int(d.Hours()) / 24
			hours := int(d.Hours()) % 24
			etaStr = fmt.Sprintf("~%dd %dh", days, hours)
		}
	} else {
		etaStr = "—"
	}

	// Format last reward time
	var rewardStr string
	if !stats.LastRewardTime.IsZero() {
		ago := time.Since(stats.LastRewardTime)
		if ago < time.Minute {
			rewardStr = fmt.Sprintf("%.0fs ago", ago.Seconds())
		} else if ago < time.Hour {
			rewardStr = fmt.Sprintf("%.0fm ago", ago.Minutes())
		} else {
			rewardStr = fmt.Sprintf("%.0fh ago", ago.Hours())
		}
	} else {
		rewardStr = "—"
	}

	// Speed string
	speedStr := stats.SpeedStr
	if speedStr == "" {
		speedStr = "—"
	}

	// Net hash string
	netHashStr := stats.NetHashStr
	if netHashStr == "" {
		netHashStr = "—"
	}

	// Calculate net share and expected mini-blocks per day
	shareStr := "—"
	minisPerDayStr := "—"
	if stats.CurrentHashrate > 0 && stats.NetHashrate > 0 {
		shareVal := (stats.CurrentHashrate / stats.NetHashrate) * 100.0
		if shareVal < 0.0001 {
			shareStr = fmt.Sprintf("%.6f%%", shareVal)
		} else if shareVal < 0.01 {
			shareStr = fmt.Sprintf("%.4f%%", shareVal)
		} else {
			shareStr = fmt.Sprintf("%.3f%%", shareVal)
		}

		minisVal := 48000.0 * (stats.CurrentHashrate / stats.NetHashrate)
		if minisVal < 0.01 {
			minisPerDayStr = fmt.Sprintf("%.4f", minisVal)
		} else {
			minisPerDayStr = fmt.Sprintf("%.2f", minisVal)
		}
	}

	// Mini blocks count
	miniBlocksStr := fmt.Sprintf("%d", stats.MiniBlocks)
	if stats.MiniBlocks == 0 {
		miniBlocksStr = "0"
	}

	// Rejected count
	rejectedStr := fmt.Sprintf("%d", stats.Rejected)

	// Update the canvas text objects
	if minerStatsUIInstance.hashrate != nil {
		minerStatsUIInstance.hashrate.Text = speedStr
		minerStatsUIInstance.hashrate.Refresh()
	}
	if minerStatsUIInstance.netHashrate != nil {
		minerStatsUIInstance.netHashrate.Text = netHashStr
		minerStatsUIInstance.netHashrate.Refresh()
	}
	if minerStatsUIInstance.netShare != nil {
		minerStatsUIInstance.netShare.Text = shareStr
		minerStatsUIInstance.netShare.Refresh()
	}
	if minerStatsUIInstance.minisPerDay != nil {
		minerStatsUIInstance.minisPerDay.Text = minisPerDayStr
		minerStatsUIInstance.minisPerDay.Refresh()
	}
	if minerStatsUIInstance.eta != nil {
		minerStatsUIInstance.eta.Text = etaStr
		minerStatsUIInstance.eta.Refresh()
	}
	if minerStatsUIInstance.miniBlocks != nil {
		minerStatsUIInstance.miniBlocks.Text = miniBlocksStr
		minerStatsUIInstance.miniBlocks.Refresh()
	}
	if minerStatsUIInstance.lastReward != nil {
		minerStatsUIInstance.lastReward.Text = rewardStr
		minerStatsUIInstance.lastReward.Refresh()
	}
	if minerStatsUIInstance.sessionDuration != nil {
		minerStatsUIInstance.sessionDuration.Text = durationStr
		minerStatsUIInstance.sessionDuration.Refresh()
	}
	if minerStatsUIInstance.rejected != nil {
		minerStatsUIInstance.rejected.Text = rejectedStr
		minerStatsUIInstance.rejected.Refresh()
	}
}

func layoutDaemonMiner() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

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

	gap := canvas.NewRectangle(color.Transparent)
	gap.SetMinSize(fyne.NewSize(60, 1))

	indicatorWidth := float32(80)
	for _, s := range []int{dmState.daemonState, dmState.minerState} {
		t := canvas.NewText(stateLabelDM(s), color.Black)
		t.TextSize = scaleFont(12)
		if w := t.MinSize().Width; w > indicatorWidth {
			indicatorWidth = w
		}
	}
	for _, l := range []string{i18n.T("daemon_miner.daemon"), i18n.T("daemon_miner.miner")} {
		t := canvas.NewText(l, color.Black)
		t.TextSize = scaleFont(14)
		t.TextStyle = fyne.TextStyle{Bold: true}
		if w := t.MinSize().Width; w > indicatorWidth {
			indicatorWidth = w
		}
	}

	daemonToggle = newToggleSwitch(daemonIsRunning(), func(checked bool) {
		go func() {
			if checked {
				startDaemon()
			} else {
				stopDaemon()
			}
			uiDo(syncToggleStates)
		}()
	})

	minerToggle = newToggleSwitch(minerIsRunning(), func(checked bool) {
		if checked {
			dmState.minerState = dmStateRunning
			syncToggleStates()
		}
		go func() {
			if checked {
				startMiner()
			} else {
				stopMiner()
			}
			uiDo(syncToggleStates)
		}()
	})

	indicatorRow := container.NewHBox(
		layout.NewSpacer(),
		container.NewVBox(
			newStateIndicator(&dmState.daemonState, i18n.T("daemon_miner.daemon"), daemonIconForState(dmState.daemonState), indicatorWidth),
			container.NewCenter(daemonToggle),
		),
		gap,
		container.NewVBox(
			newStateIndicator(&dmState.minerState, i18n.T("daemon_miner.miner"), minerIconForState(dmState.minerState), indicatorWidth),
			container.NewCenter(minerToggle),
		),
		layout.NewSpacer(),
	)

	tightSpacer := func() *canvas.Rectangle {
		r := canvas.NewRectangle(color.Transparent)
		r.SetMinSize(fyne.NewSize(10, 2))
		return r
	}

	makeStatField := func(label string, valueObj fyne.CanvasObject) fyne.CanvasObject {
		lbl := canvas.NewText(label, apptheme.C.Gray)
		lbl.TextSize = scaleFont(13)
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		return container.NewVBox(
			lbl,
			valueObj,
		)
	}
	makeStatValue := func(text string, c color.Color) *canvas.Text {
		t := canvas.NewText(text, c)
		t.TextSize = scaleFont(18)
		t.TextStyle = fyne.TextStyle{Bold: true}
		return t
	}

	// Fetch daemon info (non-blocking, uses cached on error)
	info, infoErr := fetchDaemonInfo()
	if infoErr == nil && dmState.daemonState == dmStateStopped {
		dmState.daemonState = dmStateExternal
	}

	// ---- Daemon Info Panel ----
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
		makeStatField("Height", daemonHeightText),
		tightSpacer(),
		makeStatField("Topoheight", daemonTopoText),
		tightSpacer(),
		makeStatField("Difficulty", daemonDiffText),
		tightSpacer(),
		makeStatField("Peers", daemonPeersText),
		tightSpacer(),
		func() fyne.CanvasObject {
			v := daemonVersionText
			if infoErr == nil && len(info.Version) > 20 {
				s := container.NewScroll(v)
				s.SetMinSize(fyne.NewSize(ui.Width*0.35, v.MinSize().Height))
				return makeStatField("Version", s)
			}
			return makeStatField("Version", v)
		}(),
		tightSpacer(),
		makeStatField("Tx Pool", daemonTxPoolText),
	)

	infoUI.mu.Lock()
	infoUI.status = nil
	infoUI.height = daemonHeightText
	infoUI.topo = daemonTopoText
	infoUI.difficulty = daemonDiffText
	infoUI.peers = daemonPeersText
	infoUI.version = daemonVersionText
	infoUI.txpool = daemonTxPoolText
	infoUI.mu.Unlock()

	// Daemon section header (Settings style)
	daemonSectionLabel := canvas.NewText(i18n.T("daemon_miner.daemon")+" INFO", apptheme.C.Gray)
	daemonSectionLabel.TextSize = scaleFont(14)
	daemonSectionLabel.Alignment = fyne.TextAlignCenter
	daemonSectionLabel.TextStyle = fyne.TextStyle{Bold: true}

	daemonLine1 := canvas.NewRectangle(apptheme.C.Gray)
	daemonLine1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	daemonLineBox1 := container.NewVBox(layout.NewSpacer(), daemonLine1, layout.NewSpacer())
	daemonLine2 := canvas.NewRectangle(apptheme.C.Gray)
	daemonLine2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	daemonLineBox2 := container.NewVBox(layout.NewSpacer(), daemonLine2, layout.NewSpacer())

	daemonSectionHeader := container.NewHBox(
		layout.NewSpacer(),
		daemonLineBox1,
		layout.NewSpacer(),
		daemonSectionLabel,
		layout.NewSpacer(),
		daemonLineBox2,
		layout.NewSpacer(),
	)

	// ---- Miner Config Panel ----
	cpus := runtime.NumCPU()

	makeField := func(label string) *canvas.Text {
		t := canvas.NewText(label, apptheme.C.Gray)
		t.TextSize = scaleFont(13)
		t.TextStyle = fyne.TextStyle{Bold: true}
		return t
	}

	// Wallet Address
	entryWallet := widget.NewEntry()
	entryWallet.PlaceHolder = "Enter your DERO wallet address"
	if engram.Disk != nil {
		entryWallet.SetText(engram.Disk.GetAddress().String())
	} else if minerWalletAddr != "" {
		entryWallet.SetText(minerWalletAddr)
	}
	entryWallet.OnChanged = func(s string) {
		minerWalletAddr = s
	}

	// Custom thread count entry (declared before radio group that references it)
	entryCustomThreads := widget.NewEntry()
	entryCustomThreads.PlaceHolder = "Thread count (e.g. 8)"
	entryCustomThreads.Disable()
	entryCustomThreads.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			minerThreads = n
		}
	}

	// Thread Presets
	lowThreads := max(1, cpus/2)
	medThreads := max(1, cpus-4)
	highThreads := max(1, cpus-2)

	presetLabels := []string{
		fmt.Sprintf("Low (%d threads)", lowThreads),
		fmt.Sprintf("Medium (%d threads)", medThreads),
		fmt.Sprintf("High (%d threads)", highThreads),
		"Custom",
	}

	wThreadPreset := widget.NewRadioGroup(presetLabels, func(s string) {
		switch {
		case strings.HasPrefix(s, "Low"):
			minerThreads = lowThreads
		case strings.HasPrefix(s, "Medium"):
			minerThreads = medThreads
		case strings.HasPrefix(s, "High"):
			minerThreads = highThreads
		case s == "Custom":
			if n, err := strconv.Atoi(entryCustomThreads.Text); err == nil && n > 0 {
				minerThreads = n
			}
		}
		if s == "Custom" {
			entryCustomThreads.Enable()
		} else {
			entryCustomThreads.Disable()
		}
	})
	wThreadPreset.Horizontal = false

	// Determine which preset to select based on current minerThreads
	selectedPreset := 0
	if minerThreads == lowThreads {
		selectedPreset = 0
	} else if minerThreads == medThreads {
		selectedPreset = 1
	} else if minerThreads == highThreads {
		selectedPreset = 2
	} else {
		selectedPreset = 3
	}
	wThreadPreset.SetSelected(presetLabels[selectedPreset])
	if selectedPreset == 3 {
		entryCustomThreads.SetText(fmt.Sprintf("%d", minerThreads))
		entryCustomThreads.Enable()
	}

	// Build miner config section
	minerConfigBox := container.NewVBox(
		newRectSpacer(),
		makeField("Wallet Address"),
		entryWallet,
		newRectSpacer(),
		makeField("Threads"),
		wThreadPreset,
		entryCustomThreads,
	)

	// Low-thread warning (shown when system has ≤6 CPUs)
	if cpus <= 6 {
		warnLabel := widget.NewLabel(
			fmt.Sprintf("⚠ Your system has only %d threads. Mining may impact system responsiveness.", cpus),
		)
		warnLabel.Wrapping = fyne.TextWrapWord
		minerConfigBox.Add(newRectSpacer())
		minerConfigBox.Add(warnLabel)
	}

	minerInfoBox := minerConfigBox

	// Miner statistics section
	minerStatsLabel := canvas.NewText(i18n.T("daemon_miner.miner")+" STATS", apptheme.C.Gray)
	minerStatsLabel.TextSize = scaleFont(14)
	minerStatsLabel.Alignment = fyne.TextAlignCenter
	minerStatsLabel.TextStyle = fyne.TextStyle{Bold: true}

	statsLine1 := canvas.NewRectangle(apptheme.C.Gray)
	statsLine1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	statsLineBox1 := container.NewVBox(layout.NewSpacer(), statsLine1, layout.NewSpacer())
	statsLine2 := canvas.NewRectangle(apptheme.C.Gray)
	statsLine2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	statsLineBox2 := container.NewVBox(layout.NewSpacer(), statsLine2, layout.NewSpacer())

	statsSectionHeader := container.NewHBox(
		layout.NewSpacer(),
		statsLineBox1,
		layout.NewSpacer(),
		minerStatsLabel,
		layout.NewSpacer(),
		statsLineBox2,
		layout.NewSpacer(),
	)

	// Miner statistics fields
	statColor := apptheme.C.Gray

	hashrateVal := canvas.NewText("—", statColor)
	hashrateVal.TextSize = scaleFont(16)
	netHashVal := canvas.NewText("—", statColor)
	netHashVal.TextSize = scaleFont(16)
	netShareVal := canvas.NewText("—", statColor)
	netShareVal.TextSize = scaleFont(16)
	minisPerDayVal := canvas.NewText("—", statColor)
	minisPerDayVal.TextSize = scaleFont(16)
	etaVal := canvas.NewText("—", statColor)
	etaVal.TextSize = scaleFont(16)
	miniBlocksVal := canvas.NewText("0", statColor)
	miniBlocksVal.TextSize = scaleFont(16)
	lastRewardVal := canvas.NewText("—", statColor)
	lastRewardVal.TextSize = scaleFont(16)
	sessionDurVal := canvas.NewText("—", statColor)
	sessionDurVal.TextSize = scaleFont(16)
	rejectedVal := canvas.NewText("0", statColor)
	rejectedVal.TextSize = scaleFont(16)

	// Store canvas text references for live updates
	minerStatsUIInstance.mu.Lock()
	minerStatsUIInstance.hashrate = hashrateVal
	minerStatsUIInstance.netHashrate = netHashVal
	minerStatsUIInstance.netShare = netShareVal
	minerStatsUIInstance.minisPerDay = minisPerDayVal
	minerStatsUIInstance.eta = etaVal
	minerStatsUIInstance.miniBlocks = miniBlocksVal
	minerStatsUIInstance.lastReward = lastRewardVal
	minerStatsUIInstance.sessionDuration = sessionDurVal
	minerStatsUIInstance.rejected = rejectedVal
	minerStatsUIInstance.mu.Unlock()

	minerStatsBox := container.NewVBox(
		newRectSpacer(),
		makeStatField("Hashrate", hashrateVal),
		tightSpacer(),
		makeStatField("Network Hashrate", netHashVal),
		tightSpacer(),
		makeStatField("Your Share", netShareVal),
		tightSpacer(),
		makeStatField("Est. Minis / Day", minisPerDayVal),
		tightSpacer(),
		makeStatField("ETA to Next Reward", etaVal),
		tightSpacer(),
		makeStatField("Mini Blocks (Session)", miniBlocksVal),
		tightSpacer(),
		makeStatField("Last Reward", lastRewardVal),
		tightSpacer(),
		makeStatField("Session Duration", sessionDurVal),
		tightSpacer(),
		makeStatField("Rejected Shares", rejectedVal),
		newRectSpacer(),
	)

	// Start the background stats refresh
	startMinerStatsRefresh()

	// Miner section header (Settings style)
	minerSectionLabel := canvas.NewText(i18n.T("daemon_miner.miner")+" CONFIG", apptheme.C.Gray)
	minerSectionLabel.TextSize = scaleFont(14)
	minerSectionLabel.Alignment = fyne.TextAlignCenter
	minerSectionLabel.TextStyle = fyne.TextStyle{Bold: true}

	minerLine1 := canvas.NewRectangle(apptheme.C.Gray)
	minerLine1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	minerLineBox1 := container.NewVBox(layout.NewSpacer(), minerLine1, layout.NewSpacer())
	minerLine2 := canvas.NewRectangle(apptheme.C.Gray)
	minerLine2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	minerLineBox2 := container.NewVBox(layout.NewSpacer(), minerLine2, layout.NewSpacer())

	minerSectionHeader := container.NewHBox(
		layout.NewSpacer(),
		minerLineBox1,
		layout.NewSpacer(),
		minerSectionLabel,
		layout.NewSpacer(),
		minerLineBox2,
		layout.NewSpacer(),
	)

	// Download links shown when binaries are missing
	if findBinary(daemonBinary()) == "" {
		linkDerod := widget.NewButton(i18n.T("daemon_miner.download_derod"), func() {
			showBinaryDownloadProgress(daemonBinary(), nil, nil)
		})
		linkDerod.Importance = widget.LowImportance
		daemonInfoBox.Add(linkDerod)
	}
	// Miner binary auto-downloads when the toggle is switched ON (see minerToggle callback).
	// No separate download button — the toggle handles it.

	topSection := container.NewVBox(
		indicatorRow,
		newRectSpacer(),
		daemonSectionHeader,
		daemonInfoBox,
		newRectSpacer(),
		minerSectionHeader,
		minerInfoBox,
		newRectSpacer(),
		statsSectionHeader,
		minerStatsBox,
	)

	if isMobile() {
		daemonToggle.Disable()
		minerToggle.Disable()

		mobileLabel := canvas.NewText(i18n.T("daemon_miner.unavailable"), apptheme.C.Gray)
		mobileLabel.TextSize = scaleFont(10)
		mobileLabel.Alignment = fyne.TextAlignCenter
		mobileOverlay := container.NewCenter(mobileLabel)

		mobileGap := canvas.NewRectangle(color.Transparent)
		mobileGap.SetMinSize(fyne.NewSize(40, 1))

		indicators := container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				container.NewVBox(
					newStateIndicator(&dmState.daemonState, i18n.T("daemon_miner.daemon"), daemonIconForState(dmState.daemonState), indicatorWidth),
					container.NewCenter(daemonToggle),
				),
				mobileOverlay,
			),
			mobileGap,
			container.NewStack(
				container.NewVBox(
					newStateIndicator(&dmState.minerState, i18n.T("daemon_miner.miner"), minerIconForState(dmState.minerState), indicatorWidth),
					container.NewCenter(minerToggle),
				),
				mobileOverlay,
			),
			layout.NewSpacer(),
		)
		topSection = container.NewVBox(
			indicators,
			newRectSpacer(),
			daemonSectionHeader,
			daemonInfoBox,
			newRectSpacer(),
			minerSectionHeader,
			minerInfoBox,
			newRectSpacer(),
			statsSectionHeader,
			minerStatsBox,
		)
	}

	// Start global background refresh goroutine (runs once, forever)
	startBackgroundDaemonRefresh()

	scrollContent := container.NewVBox(topSection)

	scrollWidthAnchor := canvas.NewRectangle(color.Transparent)
	scrollWidthAnchor.SetMinSize(fyne.NewSize(ui.Width, 1))

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(scrollWidthAnchor, scrollContent),
			layout.NewSpacer(),
		),
	)
	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	top := container.NewVBox(
		newRectSpacer(),
		newRectSpacer(),
		container.NewCenter(heading),
		newRectSpacer(),
	)

	footer := container.NewStack(
		container.NewVBox(
			newRectSpacer(),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			newRectSpacer(),
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
