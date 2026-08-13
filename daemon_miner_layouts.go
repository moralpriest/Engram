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

	"github.com/civilware/tela/logger"

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
	daemonToggle         *toggleSwitch
	minerToggle          *toggleSwitch
	daemonStateImg       *canvas.Image
	daemonStateImgParent *fyne.Container // stack container holding the daemon icon; used to swap images
	minerStateImg        *canvas.Image
	daemonStateLbl       *canvas.Text
	minerStateLbl        *canvas.Text
)

// daemonModeLabel is the UI text showing the current daemon mode (FULL / PRUNED).
// It is refreshed live when the mode changes via the mode dialog.
var daemonModeLabel *canvas.Text

// pendingNodeModeHealth and pendingNodeModeAutoStart carry state into the
// layoutNodeModeSelection page, since LayoutFunc takes no parameters.
var pendingNodeModeHealth SystemHealth
var pendingNodeModeAutoStart bool

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
		return apptheme.C.Green
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

// syncStateIndicators updates the state indicator icons and labels to match
// the current dmState values. Must be called on the UI goroutine.
func syncStateIndicators() {
	if daemonStateLbl != nil {
		daemonStateLbl.Text = stateLabelDM(dmState.daemonState)
		daemonStateLbl.Color = stateColorDM(dmState.daemonState)
		daemonStateLbl.Refresh()
	}
	if daemonStateImg != nil {
		// Build a fresh image with the correct tint. Using a new canvas.Image
		// object bypasses Fyne's SVG rasterizer cache (the old workaround
		// stacked a translucent green icon on top of a gray one, but that
		// prevented theme-color updates from taking effect).
		tinted := daemonIconForState(dmState.daemonState)
		fresh := canvas.NewImageFromResource(tinted)
		fresh.FillMode = canvas.ImageFillContain
		fresh.SetMinSize(daemonStateImg.MinSize())
		// Swap into the parent stack so the old image is replaced.
		if p := daemonStateImgParent; p != nil {
			p.Remove(daemonStateImg)
			p.Add(fresh)
		}
		daemonStateImg = fresh
		canvas.Refresh(fresh)
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

	labelText := canvas.NewText(label, buttonTextColor())
	labelText.TextSize = scaleFont(14)
	labelText.TextStyle = fyne.TextStyle{Bold: true}
	labelText.Alignment = fyne.TextAlignCenter

	widthAnchor := canvas.NewRectangle(color.Transparent)
	widthAnchor.SetMinSize(fyne.NewSize(indicatorWidth, 1))

	// Store references for live updates via syncStateIndicators()
	var iconContainer *fyne.Container
	if label == i18n.T("daemon_miner.daemon") {
		// Single image — the resource is swapped on every syncStateIndicators
		// tick via a fresh canvas.Image to force Fyne to re-rasterize the
		// tinted SVG with the current theme color.
		single := canvas.NewImageFromResource(daemonIconForState(dmState.daemonState))
		single.FillMode = canvas.ImageFillContain
		single.SetMinSize(fyne.NewSize(40, 40))
		daemonStateImg = single
		daemonStateImgParent = container.NewStack(single)
		daemonStateLbl = stateLabel
		iconContainer = daemonStateImgParent
	} else {
		minerStateImg = img
		minerStateLbl = stateLabel
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
			// Log only when something meaningful changes (height advanced or
			// sync state flipped). The loop polls every 3 seconds, so logging
			// every iteration would spam the debug log constantly.
			var lastHeight uint64
			var lastSynced bool
			for {
				info, err := fetchDaemonInfo()
				if err == nil {
					if info.Height != lastHeight || info.Synchronized != lastSynced {
						lastHeight = info.Height
						lastSynced = info.Synchronized
						logger.Printf("[Daemon RPC] Background refresh: height=%d topo=%d synced=%v", info.Height, info.Topoheight, info.Synchronized)
					}
					uiDo(func() { updateInfoUILabels(info) })
				} else {
					logger.Printf("[Daemon RPC] Background refresh: fetchDaemonInfo failed - %v", err)
				}
				updateDaemonStateFromDetection()
				uiDo(syncToggleStates)
				time.Sleep(3 * time.Second)
			}
		}()
	})
}

// updateInfoUILabels pushes the fetched info into the current UI label references.
// Must be called on the UI goroutine.
func updateInfoUILabels(info DaemonInfo) {
	infoUI.mu.Lock()
	defer infoUI.mu.Unlock()
	// Use the daemon state to pick the right text color — green when active,
	// gray when stopped, red on error. This color was set once at page-build
	// time but needs to be reapplied on every update or it stays gray forever.
	textColor := stateColorDM(dmState.daemonState)

	if infoUI.status != nil {
		switch {
		case dmState.daemonState == dmStateSyncing && info.Height == 0:
			// Chain initializing — fastsync was disabled, so sync starts
			// from scratch. "0/0" is misleading; show a friendlier message.
			infoUI.status.Text = i18n.T("daemon_miner.initializing_sync")
		case dmState.daemonState == dmStateSyncing && info.Height > 0 && info.Topoheight > info.Height:
			infoUI.status.Text = fmt.Sprintf(i18n.T("daemon_miner.syncing_progress"), info.Height, info.Topoheight)
		default:
			infoUI.status.Text = stateLabelDM(dmState.daemonState)
		}
		infoUI.status.Color = textColor
		infoUI.status.Refresh()
	}
	if infoUI.height != nil {
		infoUI.height.Text = fmt.Sprintf("%d", info.Height)
		infoUI.height.Color = textColor
		infoUI.height.Refresh()
	}
	if infoUI.topo != nil {
		infoUI.topo.Text = fmt.Sprintf("%d", info.Topoheight)
		infoUI.topo.Color = textColor
		infoUI.topo.Refresh()
	}
	if infoUI.difficulty != nil {
		infoUI.difficulty.Text = fmt.Sprintf("%d", info.Difficulty)
		infoUI.difficulty.Color = textColor
		infoUI.difficulty.Refresh()
	}
	if infoUI.peers != nil {
		infoUI.peers.Text = fmt.Sprintf("%d in / %d out", info.InPeers, info.OutPeers)
		infoUI.peers.Color = textColor
		infoUI.peers.Refresh()
	}
	if infoUI.version != nil {
		infoUI.version.Text = info.Version
		infoUI.version.Color = textColor
		infoUI.version.Refresh()
	}
	if infoUI.txpool != nil {
		infoUI.txpool.Text = fmt.Sprintf("%d", info.TxPoolSize)
		infoUI.txpool.Color = textColor
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
		modeDisplay = i18n.T("daemon_miner.mode_not_configured")
	} else {
		switch {
		case daemonMode == "pruned":
			modeDisplay = i18n.T("daemon_miner.mode_pruned_fs")
		case daemonMode == "full" && daemonFastSync:
			modeDisplay = i18n.T("daemon_miner.mode_full_fs")
		default:
			modeDisplay = i18n.T("daemon_miner.mode_full")
		}
	}
	daemonModeLabel.Text = i18n.T("daemon_miner.mode_prefix") + modeDisplay
	daemonModeLabel.Refresh()
}

// UIState holds non-sensitive UI preferences that persist across sessions
// in a plain JSON file (not wallet-encrypted, so they survive wallet resets).
type UIState struct {
	DaemonConfigCollapsed bool     `json:"daemon_config_collapsed"`
	DaemonInfoCollapsed   bool     `json:"daemon_info_collapsed"`
	MinerConfigCollapsed  bool     `json:"miner_config_collapsed"`
	MinerStatsCollapsed   bool     `json:"miner_stats_collapsed"`
	SectionOrder          []string `json:"section_order"`
}

var (
	uiState       UIState
	uiStateMu     sync.Mutex
	uiStateLoaded bool // true after a valid file was read from disk
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
		uiState = UIState{SectionOrder: cloneSlice(defaultSectionOrder)}
		uiStateLoaded = false
		return
	}
	if err := json.Unmarshal(data, &uiState); err != nil {
		uiState = UIState{SectionOrder: cloneSlice(defaultSectionOrder)}
		uiStateLoaded = false
		return
	}
	// Ensure section order is initialized, even when loading from an older file
	if len(uiState.SectionOrder) == 0 {
		uiState.SectionOrder = cloneSlice(defaultSectionOrder)
	}
	uiStateLoaded = true
}

// cloneSlice returns a fresh copy of a string slice.
func cloneSlice(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// Section persistence keys for collapsible section state.
const (
	sectionKeyDaemonConfig = "daemon_config"
	sectionKeyDaemonInfo   = "daemon_info"
	sectionKeyMinerConfig  = "miner_config"
	sectionKeyMinerStats   = "miner_stats"
)

// defaultSectionOrder is the initial order sections appear in.
var defaultSectionOrder = []string{
	sectionKeyDaemonConfig,
	sectionKeyDaemonInfo,
	sectionKeyMinerConfig,
	sectionKeyMinerStats,
}

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

// sectionDragHandle is a hamburger-menu icon widget that functions as a drag handle
// for reordering collapsible sections. Hold-and-drag vertically to swap sections
// when the threshold (~60px) is exceeded.
type sectionDragHandle struct {
	widget.Button
	onMoveUp   func()
	onMoveDown func()
	dragAccum  float32
}

var _ fyne.Draggable = (*sectionDragHandle)(nil)

func newSectionDragHandle(onMoveUp, onMoveDown func()) *sectionDragHandle {
	h := &sectionDragHandle{onMoveUp: onMoveUp, onMoveDown: onMoveDown}
	h.Icon = theme.MenuIcon()
	h.Importance = widget.LowImportance
	h.ExtendBaseWidget(h)
	return h
}

func (h *sectionDragHandle) Tapped(e *fyne.PointEvent) {
	menu := fyne.NewMenu("")
	if h.onMoveUp != nil {
		item := fyne.NewMenuItem("", h.onMoveUp)
		item.Icon = theme.MoveUpIcon()
		menu.Items = append(menu.Items, item)
	}
	if h.onMoveDown != nil {
		item := fyne.NewMenuItem("", h.onMoveDown)
		item.Icon = theme.MoveDownIcon()
		menu.Items = append(menu.Items, item)
	}
	if len(menu.Items) > 0 && session.Window != nil {
		widget.ShowPopUpMenuAtPosition(menu, session.Window.Canvas(), e.AbsolutePosition)
	}
}

func (h *sectionDragHandle) Dragged(e *fyne.DragEvent) {
	h.dragAccum += e.Dragged.DY
	const threshold float32 = 60
	if h.dragAccum > threshold && h.onMoveDown != nil {
		h.dragAccum = 0
		h.onMoveDown()
	} else if h.dragAccum < -threshold && h.onMoveUp != nil {
		h.dragAccum = 0
		h.onMoveUp()
	}
}

func (h *sectionDragHandle) DragEnd() {
	h.dragAccum = 0
}

// newCollapsibleSection creates a section header that acts as a toggle button.
// Clicking the header collapses/expands the content below.
// The arrow icon points down when collapsed and up when expanded.
// If persistKey is non-empty, the collapsed state is saved across sessions.
// If onMoveUp/onMoveDown are non-nil, a MenuIcon drag handle appears on the
// left of the header. Pass nil to omit the handle.
func newCollapsibleSection(title string, content fyne.CanvasObject, startCollapsed bool, persistKey string, onMoveUp, onMoveDown func()) *fyne.Container {
	arrowIcon := widget.NewIcon(theme.MenuDropDownIcon())

	label := canvas.NewText(title, apptheme.C.Gray)
	label.TextSize = scaleFont(14)
	label.Alignment = fyne.TextAlignCenter
	label.TextStyle = fyne.TextStyle{Bold: true}

	// Decorative lines
	line1 := canvas.NewRectangle(apptheme.C.Gray)
	line1.SetMinSize(fyne.NewSize(ui.Width*0.18, 2))
	lineBox1 := container.NewVBox(layout.NewSpacer(), line1, layout.NewSpacer())
	line2 := canvas.NewRectangle(apptheme.C.Gray)
	line2.SetMinSize(fyne.NewSize(ui.Width*0.18, 2))
	lineBox2 := container.NewVBox(layout.NewSpacer(), line2, layout.NewSpacer())

	// Build header components — drag handle on the left, then centered label
	var components []fyne.CanvasObject

	if onMoveUp != nil || onMoveDown != nil {
		handle := newSectionDragHandle(onMoveUp, onMoveDown)
		components = append(components, handle)
	}

	components = append(components,
		layout.NewSpacer(),
		lineBox1,
		layout.NewSpacer(),
		arrowIcon,
		label,
		layout.NewSpacer(),
		lineBox2,
		layout.NewSpacer(),
	)

	headerRow := container.NewHBox(components...)

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
						pendingNodeModeHealth = health
						pendingNodeModeAutoStart = true
						session.LastDomain = session.Window.Content()
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutNodeModeSelection())
						removeOverlays()
					})
					return
				}
				// Start daemon (embedded, runs in-process from derohe library source).
				// The eager polling for RPC availability is handled inside
				// startEmbeddedDaemon().
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

	iconRow := container.NewHBox(
		layout.NewSpacer(),
		newStateIndicator(&dmState.daemonState, i18n.T("daemon_miner.daemon"), daemonIconForState(dmState.daemonState), indicatorWidth),
		gap,
		newStateIndicator(&dmState.minerState, i18n.T("daemon_miner.miner"), minerIconForState(dmState.minerState), indicatorWidth),
		layout.NewSpacer(),
	)
	toggleRow := container.NewHBox(
		layout.NewSpacer(),
		container.NewCenter(daemonToggle),
		gap,
		container.NewCenter(minerToggle),
		layout.NewSpacer(),
	)
	indicatorRow := container.NewVBox(iconRow, newRectSpacer(), toggleRow)

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
		// Create separate canvas.Text for each info field — they must NOT share
		// the same object, or updateInfoUILabels would overwrite all fields with
		// whichever value was written last, making the display appear stuck.
		daemonHeightText = makeStatValue("—", apptheme.C.Gray)
		daemonTopoText = makeStatValue("—", apptheme.C.Gray)
		daemonDiffText = makeStatValue("—", apptheme.C.Gray)
		daemonPeersText = makeStatValue("—", apptheme.C.Gray)
		daemonVersionText = makeStatValue("—", apptheme.C.Gray)
		daemonTxPoolText = makeStatValue("—", apptheme.C.Gray)
	}

	daemonInfoBox := container.NewVBox(
		makeStatField("Height", daemonHeightText),
		tightSpacer(),
		makeStatField("Topoheight", daemonTopoText),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.difficulty"), daemonDiffText),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.peers"), daemonPeersText),
		tightSpacer(),
		func() fyne.CanvasObject {
			v := daemonVersionText
			if infoErr == nil && len(info.Version) > 20 {
				s := container.NewScroll(v)
				s.SetMinSize(fyne.NewSize(ui.Width*0.35, v.MinSize().Height))
				return makeStatField(i18n.T("daemon_miner.version"), s)
			}
			return makeStatField(i18n.T("daemon_miner.version"), v)
		}(),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.tx_pool"), daemonTxPoolText),
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

	// If the daemon is already running (e.g. auto-started) but fetchDaemonInfo()
	// failed at page-build time (RPC not ready yet), eagerly poll for it so
	// the user sees data immediately instead of waiting for background refresh.
	if globalChain != nil && infoErr != nil {
		go func() {
			for i := 0; i < 10; i++ {
				time.Sleep(500 * time.Millisecond)
				if info, err := fetchDaemonInfo(); err == nil {
					uiDo(func() {
						updateInfoUILabels(info)
						updateDaemonStateFromDetection()
						syncToggleStates()
					})
					return
				}
			}
		}()
	}

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
	entryWallet.PlaceHolder = i18n.T("daemon_miner.wallet_address_ph")
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
	entryCustomThreads.PlaceHolder = i18n.T("daemon_miner.threads_count_ph")
	entryCustomThreads.Disable()
	entryCustomThreads.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			minerThreads = n
		}
	}

	// Thread Presets
	// Thread presets: proportional to CPU count so low < med < high always holds.
	// Original formulas (cpus/2, cpus-4, cpus-2) produced equal low/medium values
	// on systems with 8 CPUs or fewer.
	lowThreads := max(1, cpus/3)  // ~1/3 of cores
	medThreads := max(2, cpus/2)  // ~1/2 of cores
	highThreads := max(2, cpus-2) // reserve 2 threads for the system

	presetLabels := []string{
		fmt.Sprintf("%s (%d %s)", i18n.T("daemon_miner.threads_low"), lowThreads, i18n.T("daemon_miner.threads")),
		fmt.Sprintf("%s (%d %s)", i18n.T("daemon_miner.threads_medium"), medThreads, i18n.T("daemon_miner.threads")),
		fmt.Sprintf("%s (%d %s)", i18n.T("daemon_miner.threads_high"), highThreads, i18n.T("daemon_miner.threads")),
		i18n.T("daemon_miner.threads_custom"),
	}

	wThreadPreset := widget.NewRadioGroup(presetLabels, func(s string) {
		switch {
		case strings.HasPrefix(s, i18n.T("daemon_miner.threads_low")):
			minerThreads = lowThreads
		case strings.HasPrefix(s, i18n.T("daemon_miner.threads_medium")):
			minerThreads = medThreads
		case strings.HasPrefix(s, i18n.T("daemon_miner.threads_high")):
			minerThreads = highThreads
		case s == i18n.T("daemon_miner.threads_custom"):
			if n, err := strconv.Atoi(entryCustomThreads.Text); err == nil && n > 0 {
				minerThreads = n
			}
		}
		if s == i18n.T("daemon_miner.threads_custom") {
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
		makeField(i18n.T("daemon_miner.wallet_address_label")),
		addressBox,
		newRectSpacer(),
		makeField(i18n.T("daemon_miner.threads_label")),
		wThreadPreset,
		entryCustomThreads,
	)

	// Low-thread warning (shown when system has ≤6 CPUs)
	if cpus <= 8 {
		warnLabel := widget.NewLabel(
			fmt.Sprintf(i18n.T("daemon_miner.thread_low_warning"), cpus),
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
		makeStatField(i18n.T("daemon_miner.miner")+" Hashrate", hashrateVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.network_hashrate"), netHashVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.your_share"), netShareVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.est_minis_day"), minisPerDayVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.eta_next_reward"), etaVal),
		tightSpacer(),
		makeStatField("Mini Blocks (Session)", miniBlocksVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.last_reward"), lastRewardVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.session_duration"), sessionDurVal),
		tightSpacer(),
		makeStatField(i18n.T("daemon_miner.rejected_shares"), rejectedVal),
		newRectSpacer(),
	)

	// Start the background stats refresh
	startMinerStatsRefresh()

	// Mode display (clickable to change) — stored in global so dialogs can refresh it live
	daemonModeLabel = canvas.NewText(i18n.T("daemon_miner.mode_prefix"), apptheme.C.Green)
	daemonModeLabel.TextSize = scaleFont(12)
	refreshDaemonModeLabel()

	changeModeBtn := widget.NewButton(i18n.T("daemon_miner.change"), func() {
		// Recompute health in background goroutine to avoid blocking the UI
		go func() {
			modeHealth := checkSystemHealth()
			uiDo(func() {
				pendingNodeModeHealth = modeHealth
				pendingNodeModeAutoStart = false
				session.LastDomain = session.Window.Content()
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutNodeModeSelection())
				removeOverlays()
			})
		}()
	})
	changeModeBtn.Importance = widget.LowImportance

	// Integrator address entry
	entryIntegrator := widget.NewEntry()
	entryIntegrator.PlaceHolder = i18n.T("daemon_miner.integrator_address_ph")
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
	nodeDataPath := filepath.Join(AppPath(), "node")
	hasNodeData := false
	if fi, err := os.Stat(nodeDataPath); err == nil && fi.IsDir() {
		hasNodeData = true
	}
	deleteNodeBtn := widget.NewButton(i18n.T("daemon_miner.delete_node_data"), func() {
		confirmDlg := dialog.NewConfirm(
			i18n.T("daemon_miner.delete_node_confirm_title"),
			i18n.T("daemon_miner.delete_node_confirm_body"),
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
						dialog.NewInformation(i18n.T("daemon_miner.node_data_deleted"), i18n.T("daemon_miner.node_data_deleted_body"), session.Window).Show()
					})
				}()
			},
			session.Window,
		)
		confirmDlg.Show()
	})
	deleteNodeBtn.Importance = widget.DangerImportance
	if !hasNodeData {
		deleteNodeBtn.Disable()
	}

	// DAEMON CONFIG (collapsible — starts collapsed when external daemon is detected)
	daemonConfigContent := container.NewVBox(
		newRectSpacer(),
		container.NewBorder(nil, nil, daemonModeLabel, changeModeBtn),
		newRectSpacer(),
		makeField(i18n.T("daemon_miner.integrator_address_label")),
		entryIntegrator,
		newRectSpacer(),
		deleteNodeBtn,
	)
	// Build section order from persisted state
	sectionOrder := cloneSlice(uiState.SectionOrder)
	if len(sectionOrder) == 0 {
		sectionOrder = cloneSlice(defaultSectionOrder)
	}

	// Build name/label map for section titles
	sectionTitles := map[string]string{
		sectionKeyDaemonConfig: i18n.T("daemon_miner.section_daemon_config"),
		sectionKeyDaemonInfo:   i18n.T("daemon_miner.section_daemon_info"),
		sectionKeyMinerConfig:  i18n.T("daemon_miner.section_miner_config"),
		sectionKeyMinerStats:   i18n.T("daemon_miner.section_miner_stats"),
	}

	// Build content map for each section key
	sectionContents := map[string]fyne.CanvasObject{
		sectionKeyDaemonConfig: daemonConfigContent,
		sectionKeyDaemonInfo:   daemonInfoBox,
		sectionKeyMinerConfig:  minerInfoBox,
		sectionKeyMinerStats:   minerStatsBox,
	}

	// Build collapsed state map
	sectionCollapsed := map[string]bool{
		sectionKeyDaemonConfig: daemonConfigCollapsed,
		sectionKeyDaemonInfo:   daemonInfoCollapsed,
		sectionKeyMinerConfig:  minerConfigCollapsed,
		sectionKeyMinerStats:   minerStatsCollapsed,
	}

	// Dynamic section container — declared before closures so move callbacks can reference it
	sectionContainer := container.NewVBox()

	// Declare rebuildSectionOrder before the section-building loop so move callbacks
	// within the loop can reference it (Go scope starts at the var declaration).
	var rebuildSectionOrder func()

	// Build wrapped sections (collapsible with drag handles)
	type sectionEntry struct {
		obj fyne.CanvasObject
	}
	sectionWraps := make(map[string]*sectionEntry)

	for _, key := range sectionOrder {
		// Always create both callbacks so the popup menu always shows both
		// Move Up and Move Down items. The callbacks check the current position
		// at runtime and are no-ops when at the boundary.
		k := key
		onMoveUp := func() {
			uiStateMu.Lock()
			// Find current position (may have changed since build)
			cur := -1
			for ci, v := range uiState.SectionOrder {
				if v == k {
					cur = ci
					break
				}
			}
			if cur > 0 {
				uiState.SectionOrder[cur], uiState.SectionOrder[cur-1] = uiState.SectionOrder[cur-1], uiState.SectionOrder[cur]
				data, _ := json.MarshalIndent(uiState, "", "  ")
				uiStateMu.Unlock()
				go func() { _ = os.WriteFile(uiStatePath(), data, 0644) }()
				uiDo(func() { rebuildSectionOrder() })
				return
			}
			uiStateMu.Unlock()
		}
		onMoveDown := func() {
			uiStateMu.Lock()
			cur := -1
			for ci, v := range uiState.SectionOrder {
				if v == k {
					cur = ci
					break
				}
			}
			if cur >= 0 && cur < len(uiState.SectionOrder)-1 {
				uiState.SectionOrder[cur], uiState.SectionOrder[cur+1] = uiState.SectionOrder[cur+1], uiState.SectionOrder[cur]
				data, _ := json.MarshalIndent(uiState, "", "  ")
				uiStateMu.Unlock()
				go func() { _ = os.WriteFile(uiStatePath(), data, 0644) }()
				uiDo(func() { rebuildSectionOrder() })
				return
			}
			uiStateMu.Unlock()
		}

		content := sectionContents[key]
		collapsed := sectionCollapsed[key]
		title := sectionTitles[key]
		wrapped := newCollapsibleSection(title, content, collapsed, key, onMoveUp, onMoveDown)
		sectionWraps[key] = &sectionEntry{obj: wrapped}

		// Assign to the named variables so legacy references still work
		switch key {
		case sectionKeyDaemonInfo:
			daemonInfoBox = wrapped
		case sectionKeyMinerConfig:
			minerInfoBox = wrapped
		case sectionKeyMinerStats:
			minerStatsBox = wrapped
		}
	}

	// rebuildSectionOrder arranges the sections in the sectionContainer according to
	// the current uiState.SectionOrder.
	rebuildSectionOrder = func() {
		sectionContainer.RemoveAll()
		uiStateMu.Lock()
		order := cloneSlice(uiState.SectionOrder)
		uiStateMu.Unlock()

		for i, key := range order {
			if i > 0 {
				sectionContainer.Add(newRectSpacer())
			}
			if entry, ok := sectionWraps[key]; ok && entry.obj != nil {
				sectionContainer.Add(entry.obj)
			}
		}
	}

	// Initial build of the section container
	rebuildSectionOrder()

	topSection := container.NewVBox(
		indicatorRow,
		newRectSpacer(),
		sectionContainer,
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
func (r *fixedWidthRenderer) MinSize() fyne.Size           { return r.widget.MinSize() }
func (r *fixedWidthRenderer) Refresh()                     { canvas.Refresh(r.text) }
func (r *fixedWidthRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.text} }
func (r *fixedWidthRenderer) Destroy()                     {}

// marqueeDisclaimer auto-scrolls a fixedWidthBox horizontally (marquee / news
// ticker). Text enters from the right and scrolls left so the user can read
// the full disclosure sequentially.
type marqueeDisclaimer struct {
	box      *fixedWidthBox
	width    float32
	done     chan struct{}
	stopOnce sync.Once
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
	m.stopOnce.Do(func() {
		close(m.done)
	})
}

// layoutNodeModeSelection is a full page for choosing the node mode.
// It reads pendingNodeModeHealth and pendingNodeModeAutoStart set by the caller
// before navigating here, since LayoutFunc takes no parameters.
func layoutNodeModeSelection() fyne.CanvasObject {
	health := pendingNodeModeHealth
	autoStart := pendingNodeModeAutoStart

	disclaimerWidth := ui.Width * 0.9

	selIdx := currentModeIndex()
	startIdx := selIdx
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

	navigateBack := func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDaemonMiner())
		removeOverlays()
	}

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
		navigateBack()
		afterSave()
	}

	// Save button
	saveBtn := widget.NewButton(i18n.T("common.save"), func() {
		opt := modeOptions[selIdx]
		if opt.mode == "full" && !health.HasSpaceForFull && !forceFullMode {
			showForceFullWarningDialog(func() {
				saveSelectedMode()
			}, nil)
			return
		}
		saveSelectedMode()
	})

	restoreBtn := widget.NewButton(i18n.T("daemon_miner.restore_defaults"), func() {
		saveDaemonMode("pruned")
		saveDaemonFastSync(true)
		refreshDaemonModeLabel()
		disclaimer.stop()
		navigateBack()
		if autoStart {
			startDaemon()
		}
		uiDo(syncToggleStates)
	})

	// showDiscardPopup shows a confirmation overlay when the user presses back
	// without saving.
	showDiscardPopup := func() {
		header := canvas.NewText(i18n.T("daemon_miner.discard_mode_title"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("settings.are_you_sure"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkCancel := widget.NewHyperlinkWithStyle(i18n.T("common.cancel"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkCancel.OnTapped = func() {
			removeOverlays()
		}

		btnDiscard := widget.NewButton(i18n.T("datapad.discard"), func() {
			disclaimer.stop()
			if autoStart {
				daemonToggle.setChecked(false)
				uiDo(syncToggleStates)
			}
			removeOverlays()
			navigateBack()
		})
		btnDiscard.Importance = widget.DangerImportance

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay := session.Window.Canvas().Overlays()
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
			),
		)
		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(header),
						newRectSpacer(),
						newRectSpacer(),
						subHeader,
						widget.NewLabel(""),
						wrapMobileButton(btnDiscard),
						newRectSpacer(),
						newRectSpacer(),
						container.NewHBox(
							layout.NewSpacer(),
							linkCancel,
							layout.NewSpacer(),
						),
						newRectSpacer(),
						newRectSpacer(),
					),
				),
			),
		)
	}

	modeContent := container.NewVBox(
		widget.NewLabel(i18n.T("daemon_miner.choose_how_node")),
		rows,
		container.NewHBox(layout.NewSpacer(), disclaimer.box, layout.NewSpacer()),
	)

	// Force Full Mode checkbox when space check fails
	if !health.HasSpaceForFull {
		forceCheck := widget.NewCheck(i18n.T("daemon_miner.force_full_mode"), func(checked bool) {
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
	modeContent.Add(restoreBtn)

	scrollContent := container.NewVBox(modeContent)

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

	heading := canvas.NewText(i18n.T("daemon_miner.node_mode_selection"), color.White)
	heading.TextSize = scaleFont(22)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.Alignment = fyne.TextAlignCenter

	top := container.NewVBox(
		newRectSpacer(),
		newRectSpacer(),
		container.NewCenter(heading),
		newRectSpacer(),
	)

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		if selIdx != startIdx {
			showDiscardPopup()
			return
		}
		disclaimer.stop()
		if autoStart {
			daemonToggle.setChecked(false)
			uiDo(syncToggleStates)
		}
		navigateBack()
	})

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

// showForceFullWarningDialog shows a warning before forcing full mode.
func showForceFullWarningDialog(onConfirm, onCancel func()) {
	warningDialog := dialog.NewConfirm(
		i18n.T("daemon_miner.force_full_warning_title"),
		i18n.T("daemon_miner.force_full_warning_body"),
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

	header := canvas.NewText(i18n.T("daemon_miner.wallet_required"), apptheme.C.Green)
	header.TextSize = 18
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	msg1 := widget.NewLabel(i18n.T("daemon_miner.integrator_reward_info"))
	msg1.Alignment = fyne.TextAlignCenter
	msg1.Wrapping = fyne.TextWrapWord

	msg2 := widget.NewLabel(i18n.T("daemon_miner.create_wallet_first"))
	msg2.Alignment = fyne.TextAlignCenter
	msg2.Wrapping = fyne.TextWrapWord

	msg3 := widget.NewLabel(i18n.T("daemon_miner.run_without_integrator"))
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

	btnOk := widget.NewButton(i18n.T("common.ok"), func() {
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
