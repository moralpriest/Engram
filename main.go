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
	"image/color"
	"os"
	"runtime"
	"time"

	_ "image/gif"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"

	_ "golang.org/x/image/webp"

	"github.com/blang/semver"
	"github.com/civilware/tela/logger"

	"github.com/DEROFDN/engram/i18n"

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
	DEFAULT_REMOTE_DAEMON            = "node.derofoundation.org:11012"
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
var colors Colors
var remoteAccess RemoteAccess
var themes Theme
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
	a.Settings().SetTheme(themes.main)

	if safeMode {
		// Disable Gnomon in safe mode
		gnomon.Active = 0
	}
	session.Window = a.NewWindow("Engram")
	session.Window.SetMaster()
	session.Window.SetCloseIntercept(func() {
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
	})
	session.Window.SetPadded(false)
	session.Domain = "app.main.loading"
	session.Window.CenterOnScreen()

	// Initialize navigation stack
	session.NavStack = NewNavigationStack()

	// Load resources
	loadResources()

	a.SetIcon(resourceIconPng)
	session.Window.SetIcon(resourceIconPng)

	// Init colors
	colors.Network = color.RGBA{R: 67, G: 239, B: 67, A: 255}
	colors.Account = color.RGBA{R: 233, G: 228, B: 233, A: 0xff}
	colors.DarkMatter = color.RGBA{21, 23, 30, 255}
	colors.Red = color.RGBA{R: 214, B: 74, G: 70, A: 255}
	colors.DarkGreen = color.RGBA{17, 127, 78, 0xff}
	colors.Green = color.RGBA{19, 202, 105, 0xff}
	colors.Blue = color.RGBA{R: 27, B: 249, G: 127, A: 255}
	colors.Gray = color.RGBA{R: 99, B: 110, G: 99, A: 0xff}
	colors.Yellow = color.RGBA{244, 208, 11, 255}
	colors.Cold = color.RGBA{60, 73, 92, 255}
	colors.Flint = color.RGBA{44, 44, 52, 0xff}
	colors.Purple = color.RGBA{191, 64, 191, 0xff}
	colors.LightBlue = color.RGBA{56, 182, 255, 255}
	colors.SoftRed = color.RGBA{R: 240, G: 110, B: 110, A: 255}

	// Init objects
	status.Canvas = canvas.NewText("", colors.Network)
	status.Network = canvas.NewText("", colors.Network)
	session.BalanceText = canvas.NewText("", colors.Account)
	status.Connection = canvas.NewCircle(colors.Red)
	status.Connection.StrokeColor = colors.Red
	status.Connection.StrokeWidth = 0
	status.Connection.Refresh()
	status.Sync = canvas.NewCircle(colors.Red)
	status.Sync.StrokeColor = colors.Red
	status.Sync.StrokeWidth = 0
	status.Sync.Refresh()
	status.RemoteAccess = canvas.NewCircle(colors.Red)
	status.RemoteAccess.StrokeColor = colors.Red
	status.RemoteAccess.StrokeWidth = 0
	status.RemoteAccess.Refresh()
	status.Gnomon = canvas.NewCircle(colors.Red)
	status.Gnomon.StrokeColor = colors.Red
	status.Gnomon.StrokeWidth = 0
	status.Gnomon.Refresh()
	status.EPOCH = canvas.NewCircle(colors.Red)
	status.EPOCH.StrokeColor = colors.Red
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
		logger.Printf("[Lifecycle] App losing focus (background)")
		// Save current domain before going to background
		if session.Domain != "" {
			previousDomain = session.Domain
		}
	})

	a.Lifecycle().SetOnEnteredForeground(func() {
		logger.Printf("[Lifecycle] App gaining focus (foreground), previous domain: %s", previousDomain)

		isMobileDevice := a.Driver().Device().IsMobile()

		now := time.Now().Unix()
		if telaViewActive.Load() {
			logger.Printf("[Lifecycle] Active TELA bootstrap foreground event - bypassing cooldown")
		} else if now-lastForegroundTime < 30 && lastForegroundTime > 0 {
			logger.Printf("[Lifecycle] Foreground cooldown active, skipping heavy refresh (mobile: %v)", isMobileDevice)
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
					logger.Printf("[Lifecycle] Mobile foreground - triggering reconnection")
					go StartPulse()
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
