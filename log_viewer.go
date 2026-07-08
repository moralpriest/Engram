// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

const (
	maxLogLines     = 1000
	maxLogBytes     = 256 * 1024 // 256KB cap for the entire log buffer
	logPollInterval = 500 * time.Millisecond
)

var (
	logViewerActive bool
	logViewerPopUp  *widget.PopUp
	logViewerMu     sync.Mutex
	logLines        []string
	logStopChan     chan struct{}
	logDisplay      *widget.Label
	logScroll       *container.Scroll
	logPollTicker   *time.Ticker

	// Status bar canvas text references (live-updated by a background goroutine)
	logViewerStatus = &logStatusBar{
		daemonState: canvas.NewText("—", apptheme.C.Gray),
		height:      canvas.NewText("—", apptheme.C.Gray),
		miner:       canvas.NewText("—", apptheme.C.Gray),
		peers:       canvas.NewText("—", apptheme.C.Gray),
		network:     canvas.NewText("—", apptheme.C.Gray),
	}
	logStatusStopChan chan struct{}
)

// logStatusBar holds canvas.Text references for the compact status bar.
type logStatusBar struct {
	daemonState *canvas.Text
	height      *canvas.Text
	miner       *canvas.Text
	peers       *canvas.Text
	network     *canvas.Text
}

// deroAddressRegex matches DERO mainnet (dero1...) and testnet (deto1...) addresses.
var deroAddressRegex = regexp.MustCompile(`(dero1[a-z0-9]{90,91}|deto1[a-z0-9]{90,91})`) // dero1+90-91 = 95-96 chars total

// toggleLogViewer shows or hides the CLI log viewer overlay.
func toggleLogViewer() {
	logViewerMu.Lock()
	defer logViewerMu.Unlock()

	if logViewerActive {
		hideLogViewerLocked()
	} else {
		showLogViewerLocked()
	}
}

// showLogViewerLocked creates and displays the log viewer overlay.
// Must be called with logViewerMu held.
func showLogViewerLocked() {
	if logViewerActive || session.Window == nil {
		return
	}

	logLines = make([]string, 0, maxLogLines)

	logDisplay = widget.NewLabel("")
	logDisplay.Wrapping = fyne.TextWrapWord

	logScroll = container.NewScroll(logDisplay)
	logScroll.SetMinSize(fyne.NewSize(ui.MaxWidth*0.95, ui.MaxHeight*0.75))

	// Header bar
	title := canvas.NewText(i18n.T("log_viewer.title"), apptheme.C.Green)
	title.TextSize = scaleFont(16)
	title.TextStyle = fyne.TextStyle{Bold: true}

	hint := canvas.NewText(i18n.T("log_viewer.hint"), apptheme.C.Gray)
	hint.TextSize = scaleFont(12)

	var privacyNote string
	if engram.Disk != nil {
		privacyNote = i18n.T("log_viewer.privacy_hidden")
	} else {
		privacyNote = i18n.T("log_viewer.privacy_visible")
	}
	privacyText := canvas.NewText(privacyNote, apptheme.C.Gray)
	privacyText.TextSize = scaleFont(11)

	// Status bar — compact terminal-style line with key metrics
	statusSep1 := canvas.NewText("│", apptheme.C.Gray)
	statusSep1.TextSize = scaleFont(11)
	statusSep2 := canvas.NewText("│", apptheme.C.Gray)
	statusSep2.TextSize = scaleFont(11)
	statusSep3 := canvas.NewText("│", apptheme.C.Gray)
	statusSep3.TextSize = scaleFont(11)
	statusSep4 := canvas.NewText("│", apptheme.C.Gray)
	statusSep4.TextSize = scaleFont(11)

	for _, t := range []*canvas.Text{
		logViewerStatus.daemonState, logViewerStatus.height,
		logViewerStatus.miner, logViewerStatus.peers, logViewerStatus.network,
	} {
		t.TextSize = scaleFont(11)
	}

	statusBar := container.NewHBox(
		layout.NewSpacer(),
		canvas.NewText("⚙", apptheme.C.Gray), logViewerStatus.daemonState,
		statusSep1,
		canvas.NewText("⬆", apptheme.C.Gray), logViewerStatus.height,
		statusSep2,
		canvas.NewText("⛏", apptheme.C.Gray), logViewerStatus.miner,
		statusSep3,
		canvas.NewText("⇄", apptheme.C.Gray), logViewerStatus.peers,
		statusSep4,
		canvas.NewText("🌐", apptheme.C.Gray), logViewerStatus.network,
		layout.NewSpacer(),
	)

	// Separator line
	sep := canvas.NewRectangle(apptheme.C.Gray)
	sep.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, 1))

	headerBox := container.NewVBox(
		container.NewHBox(title, layout.NewSpacer(), hint),
		privacyText,
		statusBar,
		sep,
	)

	content := container.NewBorder(headerBox, nil, nil, nil, logScroll)

	popUp := widget.NewModalPopUp(content, session.Window.Canvas())
	popUp.Show()

	logViewerPopUp = popUp
	logViewerActive = true

	// Load initial logs from file
	loadInitialLogs()
	updateLogDisplay()

	// Start polling for new log entries
	logStopChan = make(chan struct{})
	go pollLogUpdates()

	// Show live data immediately, then start periodic refresh
	updateStatusBar()
	logStatusStopChan = make(chan struct{})
	go startStatusBarRefresh()
}

// hideLogViewerLocked removes the log viewer overlay and stops the poll loop.
// Must be called with logViewerMu held.
func hideLogViewerLocked() {
	if !logViewerActive {
		return
	}

	if logStopChan != nil {
		close(logStopChan)
		logStopChan = nil
	}

	if logStatusStopChan != nil {
		close(logStatusStopChan)
		logStatusStopChan = nil
	}

	if logPollTicker != nil {
		logPollTicker.Stop()
		logPollTicker = nil
	}

	if logViewerPopUp != nil {
		logViewerPopUp.Hide()
		logViewerPopUp = nil
	}

	logViewerActive = false
	logDisplay = nil
	logScroll = nil
	logLines = nil
}

// loadInitialLogs reads the last N lines from the debug log file into the buffer.
func loadInitialLogs() {
	path := getDebugLogPath()
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Size() == 0 {
		return
	}

	// Read last 128KB for initial history
	readSize := int64(128 * 1024)
	startPos := info.Size() - readSize
	if startPos < 0 {
		startPos = 0
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(startPos, io.SeekStart); err != nil {
		return
	}

	tmpLines := make([]string, 0, 500)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		line = filterLogLine(line)
		if line != "" {
			tmpLines = append(tmpLines, line)
		}
	}

	// Trim to max lines/bytes from the end
	tmpLines = trimLogBuffer(tmpLines)
	logLines = tmpLines
}

// pollLogUpdates periodically checks the debug log file for new content.
func pollLogUpdates() {
	path := getDebugLogPath()

	stat, err := os.Stat(path)
	var lastSize int64
	if err == nil {
		lastSize = stat.Size()
	}

	logPollTicker = time.NewTicker(logPollInterval)
	defer logPollTicker.Stop()

	for {
		select {
		case <-logStopChan:
			return
		case <-logPollTicker.C:
			stat, err := os.Stat(path)
			if err != nil {
				continue
			}

			if stat.Size() <= lastSize {
				continue
			}

			f, err := os.Open(path)
			if err != nil {
				continue
			}

			if _, err := f.Seek(lastSize, io.SeekStart); err != nil {
				f.Close()
				continue
			}

			var newLines []string
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				line = filterLogLine(line)
				if line != "" {
					newLines = append(newLines, line)
				}
			}

			logViewerMu.Lock()
			logLines = append(logLines, newLines...)
			logLines = trimLogBuffer(logLines)
			logViewerMu.Unlock()
			f.Close()

			lastSize = stat.Size()
			updateLogDisplay()
		}
	}
}

// trimLogBuffer trims the log buffer to stay within maxLogLines and maxLogBytes limits.
func trimLogBuffer(lines []string) []string {
	if len(lines) <= maxLogLines {
		// Check total bytes
		var total int
		for _, l := range lines {
			total += len(l)
		}
		if total <= maxLogBytes {
			return lines
		}
	}

	// Trim from the front until both constraints are satisfied
	for len(lines) > 0 && (len(lines) > maxLogLines || totalBytes(lines) > maxLogBytes) {
		lines = lines[1:]
	}
	return lines
}

func totalBytes(lines []string) int {
	t := 0
	for _, l := range lines {
		t += len(l)
	}
	return t
}

// filterLogLine applies privacy filtering based on wallet state.
// If a wallet is open, all DERO addresses are redacted.
func filterLogLine(line string) string {
	if engram.Disk == nil {
		// No wallet — show everything
		return line
	}

	// Wallet is open — redact all DERO addresses for privacy
	// First, redact the user's own address by exact match
	userAddr := engram.Disk.GetAddress().String()
	if userAddr != "" && strings.Contains(line, userAddr) {
		line = strings.ReplaceAll(line, userAddr, "[ADDR_HIDDEN]")
	}

	// Then redact any other DERO addresses via regex (catch-all)
	line = deroAddressRegex.ReplaceAllString(line, "[ADDR_HIDDEN]")

	return line
}

// startStatusBarRefresh periodically updates the status bar from daemon & miner data.
func startStatusBarRefresh() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-logStatusStopChan:
			return
		case <-ticker.C:
			updateStatusBar()
		}
	}
}

// updateStatusBar reads the latest daemon info, miner stats, and session state
// then pushes them into the status bar canvas text elements on the UI goroutine.
func updateStatusBar() {
	info := getCachedDaemonInfo()
	stats := GetMiningStats()

	stateStr := stateLabelDM(dmState.daemonState)
	stateColor := stateColorDM(dmState.daemonState)

	// Height
	heightStr := "—"
	if info.Height > 0 {
		heightStr = fmt.Sprintf("%d", info.Height)
		if info.Topoheight > info.Height {
			heightStr = fmt.Sprintf("%d/%d", info.Height, info.Topoheight)
		}
	}

	// Network hashrate (more useful than personal hashrate for a quick-reference status bar)
	minerStr := "—"
	if stats.NetHashStr != "" {
		minerStr = stats.NetHashStr
	} else if stats.SpeedStr != "" {
		minerStr = stats.SpeedStr
	} else if dmState.minerState == dmStateRunning {
		minerStr = "starting..."
	}

	// Peers
	peersStr := "—"
	if info.InPeers > 0 || info.OutPeers > 0 {
		peersStr = fmt.Sprintf("%d/%d", info.OutPeers, info.InPeers)
	}

	// Network
	netStr := session.Network
	if netStr == "" {
		netStr = "—"
	}

	fyne.Do(func() {
		logViewerStatus.daemonState.Text = stateStr
		logViewerStatus.daemonState.Color = stateColor
		logViewerStatus.daemonState.Refresh()

		logViewerStatus.height.Text = heightStr
		logViewerStatus.height.Refresh()

		logViewerStatus.miner.Text = minerStr
		logViewerStatus.miner.Refresh()

		logViewerStatus.peers.Text = peersStr
		logViewerStatus.peers.Refresh()

		logViewerStatus.network.Text = netStr
		logViewerStatus.network.Refresh()
	})
}

// updateLogDisplay refreshes the log label with current buffer contents.
func updateLogDisplay() {
	logViewerMu.Lock()
	displayText := strings.Join(logLines, "\n")
	logViewerMu.Unlock()

	fyne.Do(func() {
		logViewerMu.Lock()
		defer logViewerMu.Unlock()
		if logDisplay != nil && logViewerActive {
			logDisplay.SetText(displayText)
			// Auto-scroll to bottom
			if logScroll != nil {
				logScroll.ScrollToBottom()
			}
		}
	})
}
