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
	"runtime"
	"time"

	_ "image/gif"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"


	_ "golang.org/x/image/webp"

	"github.com/blang/semver"
	"github.com/civilware/tela/logger"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
)

// Constants
const (
	DEFAULT_SIMULATOR_WALLET_PORT    = 30025
	DEFAULT_SIMULATOR_DAEMON_PORT    = 20000
	DEFAULT_TESTNET_WALLET_PORT      = 40403
	DEFAULT_TESTNET_DAEMON_PORT      = 40402
	DEFAULT_TESTNET_WORK_PORT        = 40400
	DEFAULT_WALLET_PORT              = 10103
	DEFAULT_DAEMON_PORT              = 10102
	DEFAULT_WORK_PORT                = 10100
	DEFAULT_LOCAL_TESTNET_DAEMON     = "127.0.0.1:40402"
	DEFAULT_LOCAL_TESTNET_P2P        = "127.0.0.1:40401"
	DEFAULT_LOCAL_TESTNET_WORK       = "0.0.0.0:40400"
	DEFAULT_REMOTE_TESTNET_DAEMON    = "69.30.234.163:40402"
	DEFAULT_LOCAL_DAEMON             = "127.0.0.1:10102"
	DEFAULT_LOCAL_P2P                = "127.0.0.1:10101"
	DEFAULT_LOCAL_WORK               = "0.0.0.0:10100"
	DEFAULT_REMOTE_DAEMON            = "dero.rabidmining.com:10102"
	DEFAULT_CONFIRMATION_TIMEOUT     = 5
	DEFAULT_DAEMON_RECONNECT_TIMEOUT = 10
	DEFAULT_USERADDR_SHORTEN_LENGTH  = 10
	NETWORK_MAINNET                  = "Mainnet"
	NETWORK_TESTNET                  = "Testnet"
	NETWORK_SIMULATOR                = "Simulator"
)

// Version info - injected at build time via ldflags
// Build with: go build -ldflags "-X main.versionString=1.0.0"
var versionString = "0.6.9"
var version semver.Version
var a fyne.App
var engram Engram
var session Session
var gnomon Gnomon
var msgbox MessageBox
var messages Messages
var status Status
var tx Transfers
var res Res
var remoteAccess RemoteAccess
var rpc_client Client
var Connected bool
var nav Navigation
var ui UI
var appExiting bool
var previousDomain string
var lastForegroundTime int64           // unix timestamp of last foreground event (for cooldown)
var currentScrollBox *container.Scroll // tracks current page's scroll container for mobile input scrolling

func main() {
	// Parse version from ldflags-injected string
	var err error
	version, err = semver.Parse(versionString)
	if err != nil {
		version = semver.MustParse("0.0.0-dev")
	}

	// Check for command line flags
	var safeMode bool
	args := os.Args
	for _, arg := range args {
		if arg == "--safe-mode" || arg == "-s" {
			safeMode = true
			fmt.Println("Starting Engram in SAFE MODE - Gnomon disabled")
			break
		}
	}

	// Initialize application
	a = app.NewWithID("Engram")
	appDriver = a.Driver().Device()
	if err := initDebugLog(); err != nil {
		fmt.Printf("failed to initialize debug log: %s\n", err)
	} else {
		defer closeDebugLog()
	}
	defer func() {
		if r := recover(); r != nil {
			writeCrashLog(r)
			panic(r)
		}
	}()

	apptheme.Main = &apptheme.ETheme{
		Regular:       resourceRegularTtf,
		Bold:          resourceBoldTtf,
		Italic:        resourceItalicTtf,
		BoldItalic:    resourceBoldItalicTtf,
		Astrolyt:      resourceAstrolytTtf,
		GoNoto:        resourceGoNotoCurrentTtf,
		ScaleFontSize: scaleFont,
	}
	apptheme.Alt = &apptheme.ETheme2{
		Regular:       resourceRegularTtf,
		Bold:          resourceBoldTtf,
		Italic:        resourceItalicTtf,
		BoldItalic:    resourceBoldItalicTtf,
		Astrolyt:      resourceAstrolytTtf,
		GoNoto:        resourceGoNotoCurrentTtf,
		ScaleFontSize: scaleFont,
	}

	a.Settings().SetTheme(apptheme.Main)

	// Load saved theme setting
	if themeData, err := GetValue("settings", []byte("theme")); err == nil && len(themeData) > 0 {
		apptheme.Activate(string(themeData))
	}
	RasterizeEnigmaLogo()

	if safeMode {
		// Disable Gnomon in safe mode
		gnomon.Active = 0
	}
	session.Window = a.NewWindow("Engram")
	session.Window.SetMaster()
	session.Window.SetCloseIntercept(func() {
		// Mobile: exit directly (no system tray)
		if a.Driver().Device().IsMobile() {
			appExiting = true
			appExitFlag.Store(true)
			if engram.Disk != nil {
				go func() {
					closeWallet()
					time.Sleep(2 * time.Second)
					os.Exit(0)
				}()
			} else {
				os.Exit(0)
			}
			session.Window.Close()
			return
		}
		// Desktop: minimize to system tray
		session.Window.Hide()
	})
	session.Window.SetPadded(false)
	session.Domain = "app.main.loading"
	session.Window.CenterOnScreen()

	// Initialize navigation stack
	session.NavStack = NewNavigationStack()

	// Load resources
	loadResources()
	UpdateThemeLogo()

	a.SetIcon(resourceIconPng)
	session.Window.SetIcon(resourceIconPng)

	// System tray setup (desktop only)
	if !a.Driver().Device().IsMobile() {
		initSystemTray()
	}

	// Init objects
	status.Canvas = canvas.NewText("", apptheme.C.Network)
	status.Network = canvas.NewText("", apptheme.C.Network)
	session.BalanceText = canvas.NewText("", apptheme.C.Account)
	status.Connection = canvas.NewCircle(apptheme.C.Red)
	status.Connection.StrokeColor = apptheme.C.Red
	status.Connection.StrokeWidth = 0
	status.Connection.Refresh()
	status.Sync = canvas.NewCircle(apptheme.C.Red)
	status.Sync.StrokeColor = apptheme.C.Red
	status.Sync.StrokeWidth = 0
	status.Sync.Refresh()
	status.RemoteAccess = canvas.NewCircle(apptheme.C.Red)
	status.RemoteAccess.StrokeColor = apptheme.C.Red
	status.RemoteAccess.StrokeWidth = 0
	status.RemoteAccess.Refresh()
	status.Gnomon = canvas.NewCircle(apptheme.C.Red)
	status.Gnomon.StrokeColor = apptheme.C.Red
	status.Gnomon.StrokeWidth = 0
	status.Gnomon.Refresh()
	status.EPOCH = canvas.NewCircle(apptheme.C.Red)
	status.EPOCH.StrokeColor = apptheme.C.Red
	status.EPOCH.StrokeWidth = 0
	status.EPOCH.Refresh()

	fmt.Printf("Engram v%s (Beta)\n", version)
	fmt.Printf("Copyright 2023-2026 DERO Foundation. All rights reserved.\n")
	fmt.Printf("OS: %s ARCH: %s GOMAXPROCS: %d\n\n", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))

	// Map arguments for DERO network (TODO: Fully support console arguments)
	globals.Arguments = make(map[string]interface{})
	globals.Arguments["--debug"] = false
	globals.Arguments["--testnet"] = false
	globals.Arguments["--daemon-address"] = "127.0.0.1:10102"
	globals.Arguments["--p2p-bind"] = "127.0.0.1:10101"
	globals.Arguments["--rpc-server"] = true
	globals.Arguments["--rpc-bind"] = "127.0.0.1:10103"
	globals.Arguments["--allow-rpc-password-change"] = true
	globals.Arguments["--rpc-login"] = newRPCUsername() + ":" + newRPCPassword()
	globals.Arguments["--offline"] = false
	globals.Arguments["--remote"] = false

	initSettings()
	globals.Initialize()

	session.Domain = "app.main"

	// Set up mobile back button handling
	// Note: Fyne's mobile.KeyBack captures software back button on some devices
	// Hardware back buttons and gesture navigation may vary by Android version/manufacturer
	session.Window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		logger.Printf("[KeyEvent] Received key: %s", ev.Name)
		if ev.Name == mobile.KeyBack {
			logger.Printf("[KeyEvent] Back key detected, handling navigation")
			handleBackNavigation()
		}
	})

	// Set up mobile lifecycle handling - handle app background/foreground transitions
	// This prevents crashes when switching apps on mobile
	a.Lifecycle().SetOnExitedForeground(func() {
		if session.Domain != "" {
			previousDomain = session.Domain
		}
	})

	a.Lifecycle().SetOnEnteredForeground(func() {
		isMobileDevice := a.Driver().Device().IsMobile()

		now := time.Now().Unix()
		if telaViewActive.Load() {
			logger.Printf("[Lifecycle] Active TELA bootstrap foreground event - bypassing cooldown")
		} else if now-lastForegroundTime < 30 && lastForegroundTime > 0 {
			return
		}
		lastForegroundTime = now

		if session.WalletOpen && engram.Disk != nil {
			go func() {
				generation := currentWalletGeneration()

				refreshForegroundUI := func(msg string) {
					fyne.Do(func() {
						if !isWalletGenerationActive(generation) {
							return
						}
						if telaViewActive.Load() {
							logger.Printf("[Lifecycle] Skipping foreground refresh during active TELA bootstrap")
							return
						}
						if session.Window != nil && session.Window.Content() != nil {
							session.Window.Content().Refresh()
							logger.Printf("%s", msg)
						}
					})
				}

				foregroundDelay := 500 * time.Millisecond
				if isMobileDevice {
					foregroundDelay = 1500 * time.Millisecond
				}
				time.Sleep(foregroundDelay)

				if !isWalletGenerationActive(generation) || globals.Exit_In_Progress {
					return
				}

				if !session.Offline && rpc_client.RPC == nil {
					logger.Printf("[Lifecycle] RPC connection lost, will reconnect naturally")
				}

				if isMobileDevice {
					// Skip StartPulse if pulse is already running to avoid
					// disrupting active XSWD/EPOCH connections during TELA use.
					if pulseRunning {
						logger.Printf("[Lifecycle] Mobile foreground - pulse already running, skipping reconnection")
					} else {
						logger.Printf("[Lifecycle] Mobile foreground - triggering reconnection")
						go StartPulse()
					}
					refreshForegroundUI("[Lifecycle] UI refreshed after foreground (mobile)")
					return
				}

				refreshForegroundUI("[Lifecycle] UI refreshed after foreground")

				if !isWalletGenerationActive(generation) {
					return
				}
				refreshMessageHistoryAsync(false)
			}()
		} else if previousDomain != "" {
			fyne.Do(func() {
				if telaViewActive.Load() {
					logger.Printf("[Lifecycle] Skipping foreground refresh during active TELA bootstrap")
					return
				}
				if session.Window != nil && session.Window.Content() != nil {
					session.Window.Content().Refresh()
				}
			})
		}
	})

	// Check if mobile device
	if a.Driver().Device().IsMobile() {
		go walletapi.Initialize_LookupTable(1, 1<<21)

		// Initial placeholder values - layoutFrame will get actual screen dimensions
		ui.MaxWidth = 3600
		ui.MaxHeight = 6800

		ui.Width = ui.MaxWidth * 0.9
		ui.Height = ui.MaxHeight
		ui.Padding = ui.MaxWidth * 0.05

		// Always use layoutFrame on mobile - it gets actual screen dimensions
		// and handles orientation changes.
		session.Window.SetContent(layoutFrame())
		session.Window.SetFixedSize(true)

		session.Window.ShowAndRun()
	} else {
		go walletapi.Initialize_LookupTable(1, 1<<24)
		ui.MaxWidth = 360
		ui.MaxHeight = 680

		ui.Width = ui.MaxWidth * 0.9
		ui.Height = ui.MaxHeight
		ui.Padding = ui.MaxWidth * 0.05

		resizeWindow(ui.MaxWidth, ui.MaxHeight)

		if langData, err := GetValue("settings", []byte("language")); err == nil && len(langData) > 0 {
			i18n.SetLanguage(string(langData))
			session.Window.SetContent(layoutMain())
		} else {
			session.Window.SetContent(layoutLanguageSelector())
		}
		session.Window.SetFixedSize(true)

		// Window resize check disabled - see comment above for details
		// go func() {
		// 	for {
		// 		time.Sleep(5000 * time.Millisecond)
		// 		if session.Window == nil {
		// 			return
		// 		}
		// 		currentSize := session.Window.Canvas().Size()
		// 		if math.Abs(float64(currentSize.Width - ui.MaxWidth)) > 10 {
		// 			session.Window.Resize(fyne.NewSize(ui.MaxWidth, ui.MaxHeight))
		// 		}
		// 	}
		// }()
		session.Window.ShowAndRun()
	}
}

// handleBackNavigation handles back navigation using the navigation stack or LastDomain fallback
func handleBackNavigation() {
	logger.Printf("[Navigation] handleBackNavigation called, stack size: %d", session.NavStack.Size())

	// First check if we have a navigation stack with history
	if session.NavStack != nil && session.NavStack.CanGoBack() {
		if entry, ok := session.NavStack.Pop(); ok {
			logger.Printf("[Navigation] Popped to domain: %s", entry.Domain)

			// Get the layout function for this domain
			if layoutFn := getLayoutForDomain(entry.Domain); layoutFn != nil {
				fyne.Do(func() {
					session.Domain = entry.Domain
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutFn())
				})
				return
			}

			// Fallback: try to use the old LastDomain method
			logger.Printf("[Navigation] No layout function found for domain: %s", entry.Domain)
		}
	}

	// Fallback to LastDomain for backward compatibility
	if session.LastDomain != nil {
		logger.Printf("[Navigation] Using LastDomain fallback")
		fyne.Do(func() {
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(session.LastDomain)
		})
		return
	}

	// If we're on main screen with wallet open, go to dashboard
	if engram.Disk != nil {
		logger.Printf("[Navigation] Going to dashboard (wallet open)")
		fyne.Do(func() {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
		})
		return
	}

	// On main/login screen - use mobile driver's GoBack to exit app (mobile only)
	logger.Printf("[Navigation] Exiting app via GoBack()")
	if a.Driver().Device().IsMobile() {
		if mobileDriver, ok := a.(mobile.Driver); ok {
			mobileDriver.GoBack()
		}
	}
}

// showQuitConfirmation displays a dialog with stop-options before exiting.
func showQuitConfirmation() {
	fyne.Do(func() {
		session.Window.Show()

		stopDaemonCB := widget.NewCheck(i18n.T("system_tray.stop_daemon"), nil)
		stopMinerCB := widget.NewCheck(i18n.T("system_tray.stop_miner"), nil)

		var dlg dialog.Dialog

		cancelBtn := widget.NewButton(i18n.T("system_tray.cancel"), func() {
			dlg.Hide()
		})

		quitBtn := widget.NewButton(i18n.T("system_tray.quit"), func() {
			if stopDaemonCB.Checked {
				stopDaemon()
			}
			if stopMinerCB.Checked {
				stopMiner()
			}
			dlg.Hide()
			performAppExit()
		})

		content := container.NewVBox(
			widget.NewLabel(i18n.T("system_tray.quit_message")),
			widget.NewSeparator(),
			stopDaemonCB,
			stopMinerCB,
			container.NewHBox(cancelBtn, quitBtn),
		)

		dlg = dialog.NewCustomWithoutButtons(i18n.T("system_tray.quit_title"), content, session.Window)
		dlg.Show()
	})
}

// performAppExit performs the final shutdown sequence and terminates the process.
func performAppExit() {
	appExiting = true
	appExitFlag.Store(true)

	if trayEnd != nil {
		trayEnd()
	}

	if engram.Disk != nil {
		go func() {
			closeWallet()
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}()
	} else {
		os.Exit(0)
	}
}
