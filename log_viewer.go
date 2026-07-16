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
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/civilware/tela/logger"
)

// logViewerLastOpened prevents rapid re-launches of the terminal.
var logViewerLastOpened time.Time

// toggleLogViewer opens the debug log in the system's default terminal/shell
// with a tail -f (or equivalent) so the user can watch logs in real-time.
func toggleLogViewer() {
	// Rate-limit: ignore rapid presses to prevent spawning duplicate terminals
	if time.Since(logViewerLastOpened) < 3*time.Second {
		return
	}
	logViewerLastOpened = time.Now()

	openLogInTerminal()
}

// openLogInTerminal launches a new terminal window running tail -f on the debug log.
func openLogInTerminal() {
	path := getDebugLogPath()

	// Verify the log file exists before opening
	if _, err := os.Stat(path); err != nil {
		logger.Printf("[LogViewer] No debug log file found at %s\n", path)
		return
	}

	switch runtime.GOOS {
	case "darwin":
		// macOS: use osascript to tell Terminal.app to run tail -f
		script := fmt.Sprintf(
			`tell application "Terminal" to do script "tail -f '%s'"`,
			path,
		)
		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Start(); err != nil {
			logger.Printf("[LogViewer] Failed to open Terminal: %v\n", err)
		}

	case "linux":
		// Linux: try common terminal emulators with tail -f
		termCmds := []struct {
			bin  string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", "tail", "-f", path}},
			{"gnome-terminal", []string{"--", "tail", "-f", path}},
			{"xterm", []string{"-e", "tail", "-f", path}},
			{"konsole", []string{"-e", "tail", "-f", path}},
			{"lxterminal", []string{"-e", "tail", "-f", path}},
			{"xfce4-terminal", []string{"-e", "tail", "-f", path}},
		}

		launched := false
		for _, tc := range termCmds {
			if _, err := exec.LookPath(tc.bin); err == nil {
				cmd := exec.Command(tc.bin, tc.args...)
				if err := cmd.Start(); err == nil {
					launched = true
					break
				}
			}
		}

		if !launched {
			logger.Printf("[LogViewer] No compatible terminal emulator found for tail -f\n")
		}

	case "windows":
		// Windows: use PowerShell with Get-Content -Wait (equivalent to tail -f)
		cmd := exec.Command("powershell",
			"-NoExit",
			"-Command",
			fmt.Sprintf("Get-Content -Wait \"%s\"", path),
		)
		if err := cmd.Start(); err != nil {
			logger.Printf("[LogViewer] Failed to start PowerShell log viewer: %v\n", err)
		}

	default:
		logger.Printf("[LogViewer] Unsupported OS: %s\n", runtime.GOOS)
	}

	logger.Printf("[LogViewer] Opened debug log in system terminal: %s\n", path)
}
