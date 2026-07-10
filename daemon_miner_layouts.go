package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
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

// daemonPulseColor returns the color for the daemon sync pulse animation.
// For Derotopia it uses Purple (per user preference), for other themes it matches
// the balance color (Green for Engram, etc.)
func daemonPulseColor() color.Color {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		return apptheme.C.Purple
	case apptheme.ThemeElDorado:
		return apptheme.C.Yellow
	case apptheme.ThemeCrystallina:
		return apptheme.C.Purple
	case apptheme.ThemeAtlantis:
		return apptheme.C.Yellow
	}
	return apptheme.C.Green // Engram and default
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
	daemonToggle      *toggleSwitch
	minerToggle       *toggleSwitch
	daemonStateImg    *canvas.Image
	minerStateImg     *canvas.Image
	daemonStateLbl    *canvas.Text
	minerStateLbl     *canvas.Text
	daemonPulseCircle *canvas.Circle
)

var (
	daemonSyncPulseStop chan struct{}
)

// daemonModeLabel is the UI text showing the current daemon mode (FULL / PRUNED).
// It is refreshed live when the mode changes via the mode dialog.
var daemonModeLabel *canvas.Text

const (
	dmStateStopped = iota
	dmStateRunning
	dmStateSyncing
	dmStateError
	dmStateExternal
	dmStateCorrupt
	dmStateConnecting
)

func stateColorDM(s int) color.Color {
	switch s {
	case dmStateRunning:
		return apptheme.C.Green
	case dmStateSyncing, dmStateConnecting:
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
	case dmStateConnecting:
		return i18n.T("daemon_miner.state_connecting")
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
		dmState.daemonState == dmStateSyncing ||
		dmState.daemonState == dmStateConnecting
}

func minerIsRunning() bool {
	return dmState.minerState == dmStateRunning
}

func syncToggleStates() {
	if daemonToggle != nil {
		daemonToggle.setChecked(daemonIsRunning())
		if dmState.daemonState == dmStateExternal {
			daemonToggle.Disable()
		} else {
			daemonToggle.Enable()
		}
	}
	if minerToggle != nil {
		minerToggle.setChecked(minerIsRunning())
	}
	syncStateIndicators()
}

var previousDaemonState int = -1
var daemonPulseAnim *fyne.Animation

func stopDaemonSyncPulse() {
	if daemonSyncPulseStop != nil {
		close(daemonSyncPulseStop)
		daemonSyncPulseStop = nil
	}
	if daemonPulseAnim != nil {
		daemonPulseAnim.Stop()
		daemonPulseAnim = nil
	}
	// Reset pulse circle to transparent when stopping
	if daemonPulseCircle != nil {
		daemonPulseCircle.FillColor = color.Transparent
		daemonPulseCircle.Refresh()
	}
}

func startDaemonSyncPulse() {
	stopDaemonSyncPulse()
	daemonSyncPulseStop = make(chan struct{})

	// Create smooth pulse animation using ColorRGBAAnimation
	pulseColor := daemonPulseColor()
	daemonPulseAnim = canvas.NewColorRGBAAnimation(
		color.Transparent,
		pulseColor,
		1500*time.Millisecond, // smooth 1.5s fade in/out cycle
		func(c color.Color) {
			if daemonPulseCircle != nil {
				daemonPulseCircle.FillColor = c
				daemonPulseCircle.Refresh()
			}
		})

	daemonPulseAnim.RepeatCount = fyne.AnimationRepeatForever
	daemonPulseAnim.AutoReverse = true
	daemonPulseAnim.Start()

	// Keep pulse active for up to 5 minutes, extend if still syncing
	go func() {
		timeout := time.After(5 * time.Minute)
		for {
			select {
			case <-daemonSyncPulseStop:
				return
			case <-timeout:
				syncCheck := make(chan bool, 1)
				uiDo(func() {
					syncCheck <- (dmState.daemonState == dmStateSyncing || dmState.daemonState == dmStateConnecting)
				})
				if <-syncCheck {
					timeout = time.After(5 * time.Minute)
					continue
				}
				return
			}
		}
	}()
}

// syncStateIndicators updates the state indicator icons and labels to match
// the current dmState values. Must be called on the UI goroutine.
func syncStateIndicators() {
	// Handle animation start/stop based on state transitions
	// Pulse during both connecting and syncing states
	wasActive := previousDaemonState == dmStateSyncing || previousDaemonState == dmStateConnecting
	isActive := dmState.daemonState == dmStateSyncing || dmState.daemonState == dmStateConnecting

	if wasActive && !isActive {
		stopDaemonSyncPulse()
	} else if !wasActive && isActive {
		startDaemonSyncPulse()
	}

	previousDaemonState = dmState.daemonState

	if daemonStateLbl != nil {
		daemonStateLbl.Text = stateLabelDM(dmState.daemonState)
		daemonStateLbl.Color = stateColorDM(dmState.daemonState)
		daemonStateLbl.Refresh()
	}
	// Pulse circle handles the visual feedback; no need to manipulate image translucency
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

// showBinaryDownloadConfirm and showBinaryDownloadProgress were removed.
// The daemon and miner both run as embedded processes compiled from source.
// No external binary downloads are needed.

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
		// Create pulse circle for smooth animation behind the daemon icon
		daemonPulseCircle = canvas.NewCircle(color.Transparent)
		daemonPulseCircle.StrokeWidth = 0
		daemonPulseCircle.Hidden = false
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

	// Build icon container: stack with pulse circle behind image
	iconContainer := container.NewStack()
	if label == i18n.T("daemon_miner.daemon") && daemonPulseCircle != nil {
		iconContainer = container.NewStack(daemonPulseCircle, img)
	} else {
		iconContainer = container.NewStack(img)
	}

	return container.NewVBox(
		widthAnchor,
		container.NewCenter(container.NewGridWrap(fyne.NewSize(50, 50), iconContainer)),
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
					fmt.Printf("[Daemon RPC] Background refresh: height=%d topo=%d synced=%v", info.Height, info.Topoheight, info.Synchronized)
					uiDo(func() { updateInfoUILabels(info) })
				} else {
					fmt.Printf("[Daemon RPC] Background refresh: fetchDaemonInfo failed - %v", err)
				}
				updateDaemonStateFromDetection()
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
		if dmState.daemonState == dmStateSyncing && info.Height > 0 && info.Topoheight > info.Height {
			infoUI.status.Text = fmt.Sprintf("Syncing %d/%d", info.Height, info.Topoheight)
		} else {
			infoUI.status.Text = stateLabelDM(dmState.daemonState)
		}
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

// refreshDaemonModeLabel updates the mode label text to reflect the current
// daemonMode and daemonFastSync. Must be called on the UI goroutine.
func refreshDaemonModeLabel() {
	if daemonModeLabel == nil {
		return
	}
	var modeDisplay string
	if daemonMode == "" {
		modeDisplay = "NOT CONFIGURED"
	} else {
		switch {
		case daemonMode == "pruned":
			modeDisplay = "PRUNED (FS)"
		case daemonMode == "full" && daemonFastSync:
			modeDisplay = "FULL (FS)"
		default:
			modeDisplay = "FULL"
		}
	}
	daemonModeLabel.Text = "Mode: " + modeDisplay
	daemonModeLabel.Refresh()
}

// UIState holds non-sensitive UI preferences that persist across sessions
// in a plain JSON file (not wallet-encrypted, so they survive wallet resets).
type UIState struct {
	DaemonConfigCollapsed bool `json:"daemon_config_collapsed"`
	DaemonInfoCollapsed   bool `json:"daemon_info_collapsed"`
	MinerConfigCollapsed  bool `json:"miner_config_collapsed"`
	MinerStatsCollapsed   bool `json:"miner_stats_collapsed"`
}

var (
	uiState        UIState
	uiStateMu      sync.Mutex
	uiStateLoaded  bool   // true after a valid file was read from disk
)

// uiStatePath returns the path to the UI state JSON file.
func uiStatePath() string {
	return filepath.Join(AppPath(), "ui_state.json")
}

// loadUIState reads the UI state from disk. Missing or corrupt files are silently
// treated as defaults (zero-value struct -> all sections expanded).
func loadUIState() {
	uiStateMu.Lock()
	defer uiStateMu.Unlock()

	data, err := os.ReadFile(uiStatePath())
	if err != nil {
		uiState = UIState{}
		uiStateLoaded = false
		return
	}
	if err := json.Unmarshal(data, &uiState); err != nil {
		uiState = UIState{}
		uiStateLoaded = false
		return
	}
	uiStateLoaded = true
}

// Section persistence keys for collapsible section state.
const (
	sectionKeyDaemonConfig = "daemon_config"
	sectionKeyDaemonInfo   = "daemon_info"
	sectionKeyMinerConfig  = "miner_config"
	sectionKeyMinerStats   = "miner_stats"
)

// loadSectionCollapsed reads the persisted collapsed state for a section.
// On first run (no saved file yet) returns defaultCollapsed, preserving the
// external-daemon auto-collapse for DAEMON CONFIG. Once the user has saved
// a preference, that preference is always returned.
func loadSectionCollapsed(key string, defaultCollapsed bool) bool {
	uiStateMu.Lock()
	defer uiStateMu.Unlock()

	if !uiStateLoaded {
		return defaultCollapsed
	}

	switch key {
	case sectionKeyDaemonConfig:
		return uiState.DaemonConfigCollapsed
	case sectionKeyDaemonInfo:
		return uiState.DaemonInfoCollapsed
	case sectionKeyMinerConfig:
		return uiState.MinerConfigCollapsed
	case sectionKeyMinerStats:
		return uiState.MinerStatsCollapsed
	default:
		return defaultCollapsed
	}
}

// saveSectionCollapsed persists the collapsed state for a section to the JSON file.
func saveSectionCollapsed(key string, collapsed bool) {
	uiStateMu.Lock()
	switch key {
	case sectionKeyDaemonConfig:
		uiState.DaemonConfigCollapsed = collapsed
	case sectionKeyDaemonInfo:
		uiState.DaemonInfoCollapsed = collapsed
	case sectionKeyMinerConfig:
		uiState.MinerConfigCollapsed = collapsed
	case sectionKeyMinerStats:
		uiState.MinerStatsCollapsed = collapsed
	}
	data, err := json.MarshalIndent(uiState, "", "  ")
	uiStateMu.Unlock()

	if err != nil {
		return
	}
	// Async write — non-blocking for UI responsiveness
	go func() {
		_ = os.WriteFile(uiStatePath(), data, 0644)
	}()
}

// newCollapsibleSection creates a section header that acts as a toggle button.
// Clicking the header collapses/expands the content below.
// The arrow icon points down when collapsed and up when expanded.
// If persistKey is non-empty, the collapsed state is saved across sessions.
func newCollapsibleSection(title string, content fyne.CanvasObject, startCollapsed bool, persistKey string) *fyne.Container {
	arrowIcon := widget.NewIcon(theme.MenuDropDownIcon())

	label := canvas.NewText(title, apptheme.C.Gray)
	label.TextSize = scaleFont(14)
	label.Alignment = fyne.TextAlignCenter
	label.TextStyle = fyne.TextStyle{Bold: true}

	// Decorative lines
	line1 := canvas.NewRectangle(apptheme.C.Gray)
	line1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	lineBox1 := container.NewVBox(layout.NewSpacer(), line1, layout.NewSpacer())
	line2 := canvas.NewRectangle(apptheme.C.Gray)
	line2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))
	lineBox2 := container.NewVBox(layout.NewSpacer(), line2, layout.NewSpacer())

	// Build header row with arrow icon + label in center
	headerRow := container.NewHBox(
		layout.NewSpacer(),
		lineBox1,
		layout.NewSpacer(),
		arrowIcon,
		label,
		layout.NewSpacer(),
		lineBox2,
		layout.NewSpacer(),
	)

	collapsed := startCollapsed
	if collapsed {
		content.Hide()
		// Icon already initialized with MenuDropDownIcon above
	} else {
		content.Show()
		arrowIcon.SetResource(theme.MenuDropUpIcon())
	}

	// Clickable transparent button over the header row
	headerBtn := widget.NewButton("", func() {
		collapsed = !collapsed
		if persistKey != "" {
			saveSectionCollapsed(persistKey, collapsed)
		}
		if collapsed {
			content.Hide()
			arrowIcon.SetResource(theme.MenuDropDownIcon())
		} else {
			content.Show()
			arrowIcon.SetResource(theme.MenuDropUpIcon())
		}
	})
	headerBtn.Importance = widget.LowImportance

	// Stack the button over the header visuals so clicks are captured
	header := container.NewStack(headerBtn, headerRow)

	return container.NewVBox(header, content)
}

func layoutDaemonMiner() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	updateDaemonStateFromDetection()

	// Load UI state from disk (survives across sessions, independent of wallet)
	loadUIState()

	// Load persisted collapsed states for collapsible sections
	daemonConfigCollapsed := loadSectionCollapsed(sectionKeyDaemonConfig, dmState.daemonState == dmStateExternal)
	daemonInfoCollapsed := loadSectionCollapsed(sectionKeyDaemonInfo, false)
	minerConfigCollapsed := loadSectionCollapsed(sectionKeyMinerConfig, false)
	minerStatsCollapsed := loadSectionCollapsed(sectionKeyMinerStats, false)

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
				health := checkSystemHealth()
				if !health.Passed {
					uiDo(func() {
						ShowHealthWarning(health)
					})
					return
				}
				// Check if user has a wallet before starting node (runs regardless of mode)
				if engram.Disk == nil && daemonIntegratorAddress == "" {
					uiDo(func() {
						daemonToggle.setChecked(false)
						ShowIntegratorInfoPopup()
					})
					return
				}
				if daemonMode == "" {
					uiDo(func() {
						ShowDaemonModeDialog(health, true)
					})
					return
				}
				// Start daemon (embedded, runs in-process from derohe library source)
				startDaemon()
			} else {
				stopDaemon()
			}
			uiDo(syncToggleStates)
		}()
	})

	// Disable daemon toggle immediately if an external node is detected on page load
	if dmState.daemonState == dmStateExternal {
		daemonToggle.Disable()
	}

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

	// Wrap daemon info in a collapsible section
	daemonInfoBox = newCollapsibleSection(i18n.T("daemon_miner.daemon")+" INFO", daemonInfoBox, daemonInfoCollapsed, sectionKeyDaemonInfo)

	// ---- Miner Config Panel ----
	cpus := runtime.NumCPU()

	makeField := func(label string) *canvas.Text {
		t := canvas.NewText(label, apptheme.C.Gray)
		t.TextSize = scaleFont(13)
		t.TextStyle = fyne.TextStyle{Bold: true}
		return t
	}

	// Wallet Address (hidden by default for privacy)
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
	entryWallet.Hide()

	// Masked label shown instead of the raw address
	addressHiddenLabel := canvas.NewText("● ● ● ● ● ● ● ● ● ●", apptheme.C.Gray)
	addressHiddenLabel.TextSize = scaleFont(13)

	// Visibility toggle (only from dashboard where wallet is open)
	var addressBox *fyne.Container
	if session.WalletOpen {
		var addressToggleBtn *widget.Button
		// Start hidden, set initial icon based on persisted state
		if !session.AddressHidden {
			entryWallet.Show()
			addressHiddenLabel.Hide()
		}
		addressToggleBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
			session.AddressHidden = !session.AddressHidden
			if session.AddressHidden {
				addressToggleBtn.SetIcon(theme.VisibilityIcon())
				entryWallet.Hide()
				addressHiddenLabel.Show()
			} else {
				addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
				entryWallet.Show()
				addressHiddenLabel.Hide()
			}
		})
		// Set initial icon based on persisted state
		if !session.AddressHidden {
			addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
		}
		addressToggleBtn.Importance = widget.LowImportance
		addressBox = container.NewBorder(nil, nil, nil, addressToggleBtn, container.NewStack(addressHiddenLabel, entryWallet))
	} else {
		addressBox = container.NewHBox(addressHiddenLabel)
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
		addressBox,
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

	// Wrap miner stats in a collapsible section
	minerStatsBox = newCollapsibleSection(i18n.T("daemon_miner.miner")+" STATS", minerStatsBox, minerStatsCollapsed, sectionKeyMinerStats)

	// Wrap miner config in a collapsible section
	minerInfoBox = newCollapsibleSection(i18n.T("daemon_miner.miner")+" CONFIG", minerInfoBox, minerConfigCollapsed, sectionKeyMinerConfig)

	// Mode display (clickable to change) — stored in global so dialogs can refresh it live
	daemonModeLabel = canvas.NewText("Mode: ", apptheme.C.Green)
	daemonModeLabel.TextSize = scaleFont(12)
	refreshDaemonModeLabel()

	changeModeBtn := widget.NewButton("Change", func() {
		// Recompute health in background goroutine to avoid blocking the UI
		go func() {
			modeHealth := checkSystemHealth()
			uiDo(func() {
				ShowDaemonModeDialog(modeHealth, false)
			})
		}()
	})
	changeModeBtn.Importance = widget.LowImportance

	// Integrator address entry
	entryIntegrator := widget.NewEntry()
	entryIntegrator.PlaceHolder = "Integrator address (optional)"
	entryIntegrator.SetText(daemonIntegratorAddress)

	// Auto-populate from wallet if no integrator set yet
	if daemonIntegratorAddress == "" && engram.Disk != nil {
		addr := engram.Disk.GetAddress().String()
		entryIntegrator.SetText(addr)
		saveIntegratorAddress(addr)
	}

	entryIntegrator.OnChanged = func(s string) {
		saveIntegratorAddress(s)
	}

	// Delete node data button
	deleteNodeBtn := widget.NewButton("Delete Node Data", func() {
		confirmDlg := dialog.NewConfirm(
			"Delete Node Data",
			"This will stop the daemon and remove all downloaded blockchain data. The app will need to re-sync from scratch. Continue?",
			func(confirmed bool) {
				if !confirmed {
					return
				}
				go func() {
					stopDaemon()
					nodePath := filepath.Join(AppPath(), "node")
					if err := os.RemoveAll(nodePath); err != nil {
						fmt.Printf("[Daemon] Failed to delete node data: %v", err)
						uiDo(func() {
							dialog.NewError(fmt.Errorf("Failed to delete node data: %v", err), session.Window).Show()
						})
						return
					}
					fmt.Printf("[Daemon] Node data deleted: %s", nodePath)
					dmState.daemonState = dmStateStopped
					uiDo(func() {
						syncToggleStates()
						dialog.NewInformation("Node Data Deleted", "Blockchain data has been removed. Toggle the daemon ON to start a fresh sync.", session.Window).Show()
					})
				}()
			},
			session.Window,
		)
		confirmDlg.Show()
	})
	deleteNodeBtn.Importance = widget.DangerImportance

	// DAEMON CONFIG (collapsible — starts collapsed when external daemon is detected)
	daemonConfigContent := container.NewVBox(
		newRectSpacer(),
		container.NewBorder(nil, nil, daemonModeLabel, changeModeBtn),
		newRectSpacer(),
		makeField("Integrator Address"),
		entryIntegrator,
		newRectSpacer(),
		deleteNodeBtn,
	)
	daemonConfigBox := newCollapsibleSection(i18n.T("daemon_miner.daemon")+" CONFIG", daemonConfigContent, daemonConfigCollapsed, sectionKeyDaemonConfig)

	topSection := container.NewVBox(
		indicatorRow,
		newRectSpacer(),
		daemonConfigBox,
		newRectSpacer(),
		daemonInfoBox,
		newRectSpacer(),
		minerInfoBox,
		newRectSpacer(),
		minerStatsBox,
	)

	if isMobile() {
		// Mobile: daemon runs embedded via derohe library (no external binary needed).
		// Miner toggle connects to the user's configured remote node.
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

// ShowHealthWarning displays a dialog when system health checks fail.
func ShowHealthWarning(health SystemHealth) {
	var warnDialog *widget.PopUp
	warnDialog = widget.NewModalPopUp(
		container.NewVBox(
			canvas.NewText("System Health Warning", apptheme.C.Red),
			widget.NewLabel(formatHealthMessage(health)),
			widget.NewButton("OK", func() {
				warnDialog.Hide()
			}),
		),
		session.Window.Canvas(),
	)
	warnDialog.Show()
}

type modeOption struct {
	label    string
	mode     string
	fastSync bool
}

var modeOptions = []modeOption{
	{i18n.T("daemon_miner.pruned_fastsync"), "pruned", true},
	{i18n.T("daemon_miner.full_fastsync"), "full", true},
	{i18n.T("daemon_miner.full_standard"), "full", false},
}

func modeDisclaimer(opt modeOption) string {
	switch {
	case opt.mode == "pruned":
		return i18n.T("daemon_miner.pruned_fastsync_desc")
	case opt.mode == "full" && opt.fastSync:
		return i18n.T("daemon_miner.fastsync_desc")
	default:
		return i18n.T("daemon_miner.standard_desc")
	}
}

func currentModeIndex() int {
	for i, opt := range modeOptions {
		if opt.mode == daemonMode && opt.fastSync == daemonFastSync {
			return i
		}
	}
	// Fallback to pruned
	return 0
}

// fixedWidthBox is a widget that presents a canvas.Text without resizing it,
// exposing only a fixed MinSize width. The text is clipped to the widget's
// allocated area, making it suitable for marquee-style animations.
type fixedWidthBox struct {
	widget.BaseWidget
	text     *canvas.Text
	boxWidth float32
}

func newFixedWidthBox(text string, width float32, textColor color.Color) *fixedWidthBox {
	t := canvas.NewText(text, textColor)
	t.TextSize = scaleFont(11)
	w := &fixedWidthBox{text: t, boxWidth: width}
	w.ExtendBaseWidget(w)
	return w
}

func (w *fixedWidthBox) SetText(s string) {
	w.text.Text = s
	w.text.Move(fyne.NewPos(0, 0))
	w.text.Refresh()
}

func (w *fixedWidthBox) TextObject() *canvas.Text {
	return w.text
}

func (w *fixedWidthBox) MinSize() fyne.Size {
	return fyne.NewSize(w.boxWidth, w.text.MinSize().Height)
}

func (w *fixedWidthBox) CreateRenderer() fyne.WidgetRenderer {
	return &fixedWidthRenderer{text: w.text, widget: w}
}

type fixedWidthRenderer struct {
	text   *canvas.Text
	widget *fixedWidthBox
}

func (r *fixedWidthRenderer) Layout(_ fyne.Size) {
	// Intentionally empty — canvas.Text keeps its natural size; the widget
	// clips to its own allocated bounds in the standard Fyne paint pass.
}
func (r *fixedWidthRenderer) MinSize() fyne.Size        { return r.widget.MinSize() }
func (r *fixedWidthRenderer) Refresh()                   { canvas.Refresh(r.text) }
func (r *fixedWidthRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.text} }
func (r *fixedWidthRenderer) Destroy()                   {}

// marqueeDisclaimer auto-scrolls a fixedWidthBox horizontally (marquee / news
// ticker). Text enters from the right and scrolls left so the user can read
// the full disclosure sequentially.
type marqueeDisclaimer struct {
	box   *fixedWidthBox
	width float32
	done  chan struct{}
}

func newMarqueeDisclaimer(initial string, width float32) *marqueeDisclaimer {
	m := &marqueeDisclaimer{
		box:   newFixedWidthBox(initial, width, apptheme.C.Gray),
		width: width,
		done:  make(chan struct{}),
	}
	go m.animate()
	return m
}

func (m *marqueeDisclaimer) SetText(s string) {
	m.box.SetText(s)
}

func (m *marqueeDisclaimer) animate() {
	// Let the dialog finish laying out before starting
	time.Sleep(800 * time.Millisecond)

	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()

	text := m.box.TextObject()
	offset := m.width // start fully off-screen to the right

	for {
		select {
		case <-m.done:
			return
		case <-tick.C:
			tw := text.MinSize().Width
			if tw <= m.width {
				// Text fits — no scrolling needed, static position
				continue
			}

			offset--

			if offset+tw < 0 {
				offset = m.width
			}

			pos := offset
			fyne.Do(func() {
				text.Move(fyne.NewPos(pos, 0))
				canvas.Refresh(text)
			})
		}
	}
}

func (m *marqueeDisclaimer) stop() {
	close(m.done)
}

// ShowDaemonModeDialog prompts the user to choose a node mode.
// If autoStart is true, the daemon will start automatically after saving.
// If autoStart is false, only the mode is saved and the dialog closes.
func ShowDaemonModeDialog(health SystemHealth, autoStart bool) {
	var modeDialog *widget.PopUp

	const disclaimerWidth = float32(300)

	selIdx := currentModeIndex()
	disclaimer := newMarqueeDisclaimer(modeDisclaimer(modeOptions[selIdx]), disclaimerWidth)

	var modeBtns []*widget.Button
	refreshBtnStyles := func() {
		for i, btn := range modeBtns {
			if i == selIdx {
				btn.Importance = widget.HighImportance
			} else {
				btn.Importance = widget.LowImportance
			}
			btn.Refresh()
		}
	}

	for i, opt := range modeOptions {
		idx := i
		btn := widget.NewButton(opt.label, func() {
			selIdx = idx
			refreshBtnStyles()
			disclaimer.SetText(modeDisclaimer(opt))
		})
		modeBtns = append(modeBtns, btn)
	}
	refreshBtnStyles()

	var objs []fyne.CanvasObject
	for _, b := range modeBtns {
		objs = append(objs, b)
	}
	rows := container.NewVBox(objs...)

	modeContent := container.NewVBox(
		canvas.NewText("Node Mode Selection", apptheme.C.Green),
		widget.NewLabel("Choose how to run this node:"),
		widget.NewSeparator(),
		rows,
		container.NewHBox(layout.NewSpacer(), disclaimer.box, layout.NewSpacer()),
	)

	afterSave := func() {
		if autoStart {
			startDaemon()
		}
		uiDo(syncToggleStates)
	}

	saveSelectedMode := func() {
		opt := modeOptions[selIdx]
		saveDaemonMode(opt.mode)
		saveDaemonFastSync(opt.fastSync)
		refreshDaemonModeLabel()
		disclaimer.stop()
		modeDialog.Hide()
		afterSave()
	}

	// Save button
	saveBtn := widget.NewButton("Save", func() {
		opt := modeOptions[selIdx]
		if opt.mode == "full" && !health.HasSpaceForFull && !forceFullMode {
			showForceFullWarningDialog(func() {
				saveSelectedMode()
			}, nil)
			return
		}
		saveSelectedMode()
	})
	saveBtn.Importance = widget.HighImportance

	// Force Full Mode checkbox when space check fails
	if !health.HasSpaceForFull {
		forceCheck := widget.NewCheck("Force Full Mode (I have 250GB+ available)", func(checked bool) {
			saveForceFullMode(checked)
			if checked {
				saveBtn.Enable()
			}
		})
		if forceFullMode {
			forceCheck.SetChecked(true)
		}
		modeContent.Add(forceCheck)
	}

	modeContent.Add(saveBtn)

	// Cancel / Close button
	if autoStart {
		modeContent.Add(widget.NewButton("Cancel", func() {
			disclaimer.stop()
			daemonToggle.setChecked(false)
			uiDo(syncToggleStates)
			modeDialog.Hide()
		}))
	} else {
		modeContent.Add(widget.NewButton("Close", func() {
			disclaimer.stop()
			modeDialog.Hide()
		}))
	}

	modeContent.Add(widget.NewSeparator())
	restoreBtn := widget.NewButton("Restore Defaults (Pruned + Fast Sync)", func() {
		saveDaemonMode("pruned")
		saveDaemonFastSync(true)
		refreshDaemonModeLabel()
		disclaimer.stop()
		modeDialog.Hide()
		if autoStart {
			startDaemon()
		}
		uiDo(syncToggleStates)
	})
	restoreBtn.Importance = widget.LowImportance
	modeContent.Add(restoreBtn)

	modeDialog = widget.NewModalPopUp(modeContent, session.Window.Canvas())
	modeDialog.Show()
}

// showForceFullWarningDialog shows a warning before forcing full mode.
func showForceFullWarningDialog(onConfirm, onCancel func()) {
	warningDialog := dialog.NewConfirm(
		"Warning: Force Full Mode",
		"You have enabled Force Full Mode. This will ONLY work if you actually have 250GB+ available on your disk.\n\nIf you don't have enough space, the daemon will fail to start.\n\nContinue?",
		func(confirmed bool) {
			if confirmed {
				onConfirm()
			} else if onCancel != nil {
				onCancel()
			}
		},
		session.Window,
	)
	warningDialog.Show()
}

// ShowIntegratorInfoPopup displays info about integrator rewards when no wallet exists.
func ShowIntegratorInfoPopup() {
	if session.Window == nil {
		return
	}

	header := canvas.NewText("Wallet Required", apptheme.C.Green)
	header.TextSize = 18
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	msg1 := widget.NewLabel("Get 10% extra mining rewards when running a node with your wallet address.")
	msg1.Alignment = fyne.TextAlignCenter
	msg1.Wrapping = fyne.TextWrapWord

	msg2 := widget.NewLabel("Create or recover a wallet first, then enable the node.")
	msg2.Alignment = fyne.TextAlignCenter
	msg2.Wrapping = fyne.TextWrapWord

	msg3 := widget.NewLabel("You can also run the node without an integrator address.")
	msg3.Alignment = fyne.TextAlignCenter
	msg3.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer1 := canvas.NewRectangle(color.Transparent)
	rectSpacer1.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer2 := canvas.NewRectangle(color.Transparent)
	rectSpacer2.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer3 := canvas.NewRectangle(color.Transparent)
	rectSpacer3.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer4 := canvas.NewRectangle(color.Transparent)
	rectSpacer4.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer5 := canvas.NewRectangle(color.Transparent)
	rectSpacer5.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer6 := canvas.NewRectangle(color.Transparent)
	rectSpacer6.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer7 := canvas.NewRectangle(color.Transparent)
	rectSpacer7.SetMinSize(fyne.NewSize(0, 10))

	var overlay *fyne.Container
	var blocker *fyne.Container

	btnOk := widget.NewButton("OK", func() {
		uiDo(func() {
			overlays := session.Window.Canvas().Overlays()
			overlays.Remove(overlay)
			overlays.Remove(blocker)
		})
	})
	btnOk.Importance = widget.MediumImportance

	btnRow := container.NewHBox(layout.NewSpacer(), btnOk, layout.NewSpacer())

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer1,
				rectSpacer2,
				btnRow,
				rectSpacer3,
				rectSpacer4,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						msg1,
						rectSpacer5,
						msg2,
						rectSpacer5,
						msg3,
						rectSpacer5,
					),
				),
			),
		),
	)

	span1 := canvas.NewRectangle(color.Transparent)
	span1.SetMinSize(fyne.NewSize(ui.Width, 10))

	blocker = container.NewStack(
		&iframe{},
		canvas.NewRectangle(apptheme.C.DarkMatter),
	)

	overlay = container.NewStack(
		&iframe{},
		container.NewCenter(
			container.NewVBox(
				span1,
				container.NewCenter(
					header,
				),
				rectSpacer6,
				rectSpacer7,
				content,
			),
		),
	)

	uiDo(func() {
		overlays := session.Window.Canvas().Overlays()
		overlays.Add(blocker)
		overlays.Add(overlay)
	})
}

// formatHealthMessage formats the health check results into a readable message.
func formatHealthMessage(health SystemHealth) string {
	var parts []string
	if !health.TimeSynced {
		parts = append(parts, "Time sync: FAILED - "+health.TimeSyncError)
	}
	if health.DiskSpaceGB >= 0 && health.DiskSpaceGB < 5 {
		parts = append(parts, fmt.Sprintf("Disk space: %.0f GB available (need at least 5 GB)", health.DiskSpaceGB))
	}
	if health.DiskIOError != "" {
		parts = append(parts, "Disk I/O: "+health.DiskIOError)
	}
	if health.InodeError != "" {
		parts = append(parts, "Inodes: "+health.InodeError)
	}
	if len(parts) == 0 {
		return "System health check failed for an unknown reason."
	}
	return strings.Join(parts, "\n")
}
