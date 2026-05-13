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
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	x "fyne.io/x/fyne/widget"
	"github.com/DEROFDN/engram/internal/camera"
	"github.com/civilware/Gnomon/structures"
	"github.com/civilware/epoch"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/transaction"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/mnemonics"
	"github.com/deroproject/derohe/walletapi/xswd"
	"github.com/deroproject/graviton"
	qrcode "github.com/skip2/go-qrcode"
)

var telaNavigationStack struct {
	sync.Mutex
	history []string
}

var telaLaunchingSCIDsGlobal struct {
	sync.Mutex
	m map[string]bool
}

var telaLaunchCancelChansGlobal struct {
	sync.Mutex
	m map[string]chan struct{}
}

var telaActiveServersGlobal struct {
	sync.RWMutex
	info   []tela.ServerInfo
	active map[string]bool
}

var telaStoppingSCIDsGlobal struct {
	sync.Mutex
	m map[string]bool
}

var telaLaunchStartTimesGlobal struct {
	sync.Mutex
	m map[string]time.Time
}

func RefreshVillagerLogo() {
	if res.logoContainer == nil {
		return
	}

	res.logoContainer.Objects = nil
	res.villagerMu.Lock()
	vImg := res.villager
	res.villagerMu.Unlock()

	if vImg != nil && !session.VillagerHidden {
		vImg.SetMinSize(fyne.NewSize(ui.Width, scaleSize(150)))
		res.logoContainer.Add(vImg)
	} else {
		res.logoContainer.Add(res.gram)
	}
	res.logoContainer.Refresh()
}

var telaBackfillActive atomic.Bool
var telaBackfillFailed atomic.Bool
var lastBackfillHeight int64 // Tracks the Gnomon height at which backfill last ran

var introShownThisSession bool
var appFirstOpenDone bool
var marqueeMu sync.Mutex

func init() {
	telaLaunchingSCIDsGlobal.m = make(map[string]bool)
	telaLaunchCancelChansGlobal.m = make(map[string]chan struct{})
	telaActiveServersGlobal.active = make(map[string]bool)
	telaStoppingSCIDsGlobal.m = make(map[string]bool)
	telaLaunchStartTimesGlobal.m = make(map[string]time.Time)

	// Background goroutine to periodically update TELA server state
	// This prevents blocking the UI thread if the tela package lock is held
	go func() {
		for {
			if appExitFlag.Load() {
				return
			}

			// This call might block if ServeTELA is holding the exclusive lock,
			// but since we're in a goroutine, it won't freeze the UI.
			servers := tela.GetServerInfo()

			activeMap := make(map[string]bool)
			for _, s := range servers {
				activeMap[s.SCID] = true
			}

			telaActiveServersGlobal.Lock()
			telaActiveServersGlobal.info = servers
			telaActiveServersGlobal.active = activeMap
			telaActiveServersGlobal.Unlock()

			time.Sleep(2 * time.Second)
		}
	}()
}

func isMobileDevice() bool {
	return isMobile()
}

func getTelaActiveServers() []tela.ServerInfo {
	telaActiveServersGlobal.RLock()
	defer telaActiveServersGlobal.RUnlock()

	if telaActiveServersGlobal.info == nil {
		return []tela.ServerInfo{}
	}

	// Return a copy to avoid race conditions if the caller modifies the slice
	res := make([]tela.ServerInfo, len(telaActiveServersGlobal.info))
	copy(res, telaActiveServersGlobal.info)
	return res
}

func isTelaActive(scid string) bool {
	telaActiveServersGlobal.RLock()
	defer telaActiveServersGlobal.RUnlock()
	return telaActiveServersGlobal.active[scid]
}

func getTelaPath() string {
	return tela.GetPath()
}

func areTelaUpdatesAllowed() bool {
	return tela.UpdatesAllowed()
}

// walletBtn is a custom tappable button for wallet selection with colored background
type walletBtn struct {
	widget.BaseWidget
	BgColor  color.Color
	TxtColor color.Color
	Label    string
	onTapped func()
	bg       *canvas.Rectangle
	lbl      *canvas.Text
}

func newWalletBtn(label string, onTap func()) *walletBtn {
	w := &walletBtn{
		BgColor:  colors.DarkMatter,
		TxtColor: colors.Gray,
		Label:    label,
		onTapped: onTap,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *walletBtn) CreateRenderer() fyne.WidgetRenderer {
	w.bg = canvas.NewRectangle(w.BgColor)
	w.bg.CornerRadius = scaleSize(12)
	w.lbl = canvas.NewText(w.Label, w.TxtColor)
	w.lbl.Alignment = fyne.TextAlignCenter
	w.lbl.TextStyle = fyne.TextStyle{Bold: true}
	return &walletBtnRenderer{btn: w}
}

func (w *walletBtn) Tapped(_ *fyne.PointEvent) {
	if w.onTapped != nil {
		w.onTapped()
	}
}

func (w *walletBtn) SetColors(bg, txt color.Color) {
	w.BgColor = bg
	w.TxtColor = txt
	if w.bg != nil {
		w.bg.FillColor = bg
		w.bg.Refresh()
	}
	if w.lbl != nil {
		w.lbl.Color = txt
		w.lbl.Refresh()
	}
}

type walletBtnRenderer struct {
	btn *walletBtn
}

func (r *walletBtnRenderer) Layout(s fyne.Size) {
	r.btn.bg.Resize(s)
	r.btn.lbl.Resize(s)
}

func (r *walletBtnRenderer) MinSize() fyne.Size {
	return fyne.NewSize(scaleSize(100), scaleSize(36))
}

func (r *walletBtnRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.btn.bg, r.btn.lbl}
}

func (r *walletBtnRenderer) Refresh() {
	r.btn.bg.FillColor = r.btn.BgColor
	r.btn.bg.Refresh()
	r.btn.lbl.Text = r.btn.Label
	r.btn.lbl.Color = r.btn.TxtColor
	r.btn.lbl.Refresh()
}

func (r *walletBtnRenderer) Destroy() {}

func newAdaptiveButton(label string, icon fyne.Resource, tapped func()) fyne.CanvasObject {
	return wrapMobileButton(widget.NewButtonWithIcon(label, icon, tapped))
}

func wrapMobileButton(obj fyne.CanvasObject) fyne.CanvasObject {
	if isMobile() {
		sizeEnforcer := canvas.NewRectangle(color.Transparent)
		sizeEnforcer.SetMinSize(scalePoint(48, 48))
		return container.NewStack(sizeEnforcer, obj)
	}
	return obj
}

func pushTELANavigation(scid string) {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	telaNavigationStack.history = append(telaNavigationStack.history, scid)
}

func popTELANavigation() string {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	if len(telaNavigationStack.history) == 0 {
		return ""
	}
	index := len(telaNavigationStack.history) - 1
	scid := telaNavigationStack.history[index]
	telaNavigationStack.history = telaNavigationStack.history[:index]
	return scid
}

func hasTELANavigationHistory() bool {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	return len(telaNavigationStack.history) > 0
}

func clearTELANavigationHistory() {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	telaNavigationStack.history = nil
}

func showVillagerPopup(parent *fyne.Container) {
	rect := canvas.NewRectangle(color.NRGBA{R: 21, G: 23, B: 30, A: 220})
	rect.CornerRadius = scaleSize(10)
	rect.SetMinSize(fyne.NewSize(scaleSize(220), scaleSize(40)))

	text := canvas.NewText("Click here to edit villager", colors.Green)
	text.Alignment = fyne.TextAlignCenter
	text.TextSize = scaleFont(14)

	popup := container.NewCenter(container.NewStack(rect, text))

	parent.Add(popup)
	parent.Refresh()

	go func() {
		time.Sleep(5 * time.Second)
		fyne.Do(func() {
			parent.Remove(popup)
			parent.Refresh()
		})
	}()
}

func showVillagerMenu(updateLogo func()) {
	res.villagerMu.Lock()
	hasVillager := res.villager != nil
	res.villagerMu.Unlock()

	overlay := session.Window.Canvas().Overlays()

	var menu *fyne.Container

	btnEdit := widget.NewButtonWithIcon("Edit Villager", theme.DocumentCreateIcon(), func() {
		overlay.Remove(menu)
		showLoadingOverlay()
		go func() {
			// For Villager Edit, if XSWD is not enabled, always prompt.
			alreadyEnabled := remoteAccess.WS.global.enabled
			if !alreadyEnabled {
				if showXSWDPrompt() {
					remoteAccess.WS.global.enabled = true
					if remoteAccess.WS.port == "" {
						remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
						setRemoteAccessDual(remoteAccess.WS.port, "WS")
					}
					setPermissions()
					if remoteAccess.WS.server == nil {
						toggleXSWD(remoteAccess.WS.port)
					}
				} else {
					remoteAccess.WS.global.enabled = false
					setPermissions()
					if remoteAccess.WS.server != nil {
						toggleXSWD(remoteAccess.WS.port)
					}
				}
				setAskedXSWD()
			}

			scid := "986fc20fefeda2227e5722af66390c57f3606468a485215f773326aa872697c8"
			index, err := tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Villager] Error getting index for %s: %v", scid, err)
				removeOverlays()
				return
			}
			fyne.Do(func() {
				session.LastDomain = session.Window.Content()
				session.Window.SetContent(layoutTELAManager(index, func() {
					session.Window.SetContent(layoutDashboard())
				}, true))
				removeOverlays()
			})
		}()
	})

	hideText := "Hide"
	hideIcon := theme.VisibilityOffIcon()
	if session.VillagerHidden {
		hideText = "Show"
		hideIcon = theme.VisibilityIcon()
	}
	btnHide := widget.NewButtonWithIcon(hideText, hideIcon, func() {
		session.VillagerHidden = !session.VillagerHidden
		updateLogo()
		val := "false"
		if session.VillagerHidden {
			val = "true"
		}
		go setTELADual("VillagerHidden", []byte(val))
		overlay.Remove(menu)
	})
	if !hasVillager {
		btnHide.Disable()
	}

	btnBgToggle := widget.NewButtonWithIcon("Background Toggle", theme.ViewRefreshIcon(), func() {
		session.VillagerBackground = !session.VillagerBackground
		val := "false"
		if session.VillagerBackground {
			val = "true"
		}
		go setTELADual("VillagerBackground", []byte(val))

		// Re-render villager using cached pixels if available for instant response
		go func() {
			if engram.Disk != nil {
				address := engram.Disk.GetAddress().String()
				pixels := session.VillagerPixels
				if pixels == "" {
					// Fallback to fetching if cache is empty for some reason
					var err error
					pixels, err = fetchVillagerPixels(address)
					if err != nil {
						logger.Errorf("[Villager] Error fetching pixels for toggle: %v", err)
						return
					}
					session.VillagerPixels = pixels
				}

				villagerImg := renderVillager(address, pixels)
				res.villagerMu.Lock()
				res.villager = villagerImg
				res.villagerMu.Unlock()
				fyne.Do(func() {
					updateLogo()
				})
			}
		}()
		overlay.Remove(menu)
	})
	if !hasVillager {
		btnBgToggle.Disable()
	}

	btnClose := widget.NewButtonWithIcon("Close", theme.CancelIcon(), func() {
		overlay.Remove(menu)
	})

	btnSize := fyne.NewSize(scaleSize(240), scaleSize(40))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	menuContent := container.NewVBox(
		rectSpacer,
		container.NewHBox(layout.NewSpacer(), container.NewGridWrap(btnSize, btnEdit), layout.NewSpacer()),
		rectSpacer,
		container.NewHBox(layout.NewSpacer(), container.NewGridWrap(btnSize, btnHide), layout.NewSpacer()),
		rectSpacer,
		container.NewHBox(layout.NewSpacer(), container.NewGridWrap(btnSize, btnBgToggle), layout.NewSpacer()),
		rectSpacer,
		container.NewHBox(layout.NewSpacer(), container.NewGridWrap(btnSize, btnClose), layout.NewSpacer()),
	)

	// Use a transparent container for the dimming effect to avoid corner artifacts
	background := canvas.NewRectangle(color.NRGBA{0, 0, 0, 150})

	menuBg := canvas.NewRectangle(theme.BackgroundColor())
	menuBg.CornerRadius = scaleSize(10)

	// Combine everything into a stack that fills the window
	menu = container.NewStack(
		background,
		container.NewCenter(
			container.NewStack(
				menuBg,
				container.NewPadded(menuContent),
			),
		),
	)

	overlay.Add(menu)
	menu.Resize(session.Window.Canvas().Size())
}

// Add package variable to remember settings caller domain across sub-page visits
var settingsCallerDomain string

// Add package variable to remember TELA inspection page content for back navigation
var cachedTelaManagerContent fyne.CanvasObject

func layoutMain() fyne.CanvasObject {
	// Set theme
	a.Settings().SetTheme(themes.main)
	session.Domain = "app.main"
	session.Path = ""
	session.Password = ""

	// Define objects

	btnLogin := widget.NewButtonWithIcon("Connect", theme.LoginIcon(), nil)

	if session.Error != "" {
		btnLogin.Text = session.Error
		btnLogin.Disable()
		btnLogin.Refresh()
		session.Error = ""
	}

	btnLogin.OnTapped = func() {
		if session.Path == "" {
			btnLogin.Text = "No account selected..."
			btnLogin.Disable()
			btnLogin.Refresh()
		} else if session.Password == "" {
			btnLogin.Text = "Invalid password..."
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			if !session.Offline {
				btnLogin.Text = "Connect"
			} else {
				btnLogin.Text = "Decrypt"
			}
			btnLogin.Enable()
			btnLogin.Refresh()
			login()
			btnLogin.Text = session.Error
			btnLogin.Disable()
			btnLogin.Refresh()
			session.Error = ""
		}
	}

	btnLogin.Disable()

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain == "app.main" || session.Domain == "app.register" {
			if k.Name == fyne.KeyReturn {
				if session.Path == "" {
					btnLogin.Text = "No account selected..."
					btnLogin.Disable()
					btnLogin.Refresh()
				} else if session.Password == "" {
					btnLogin.Text = "Invalid password..."
					btnLogin.Disable()
					btnLogin.Refresh()
				} else {
					if !session.Offline {
						btnLogin.Text = "Connect"
					} else {
						btnLogin.Text = "Decrypt"
					}
					btnLogin.Enable()
					btnLogin.Refresh()
					login()
					btnLogin.Text = "Invalid password..."
					btnLogin.Disable()
					btnLogin.Refresh()
					session.Error = ""
				}
			}
		} else {
			return
		}
	})

	// New Account button with icon
	btnNewAccount := newBorderedButtonWithIcon("New Account", theme.ContentAddIcon(), color.White, func() {
		session.Domain = "app.create"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutNewAccount())
		removeOverlays()
	}, ui.Width*0.9)

	// Recover Account button with icon
	btnRecoverAccount := newBorderedButtonWithIcon("Recover Account", theme.DocumentIcon(), color.White, func() {
		session.Domain = "app.restore"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutRestore())
		removeOverlays()
	}, ui.Width*0.9)

	// Connection Settings button with icon
	btnConnectionSettings := newGunmetalButtonWithIcon("Connection Settings", theme.SettingsIcon(), colors.Green, func() {
		session.Domain = "app.settings"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSettings())
		removeOverlays()
	}, ui.Width*0.9)

	modeData := binding.BindBool(&session.Offline)
	mode := widget.NewCheckWithData(" Offline Mode", modeData)
	mode.OnChanged = func(b bool) {
		if b {
			session.Offline = true
			btnLogin.Text = "Decrypt"
			btnLogin.Refresh()
		} else {
			session.Offline = false
			btnLogin.Text = "Connect"
			btnLogin.Refresh()
		}
	}

	wPassword := NewReturnEntry()
	wPassword.OnReturn = btnLogin.OnTapped
	wPassword.Password = true
	wPassword.OnChanged = func(s string) {
		session.Error = ""
		if !session.Offline {
			btnLogin.Text = "Connect"
		} else {
			btnLogin.Text = "Decrypt"
		}
		btnLogin.Enable()
		btnLogin.Refresh()
		session.Password = s

		if len(s) < 1 {
			btnLogin.Disable()
			btnLogin.Refresh()
		} else if session.Path == "" {
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			btnLogin.Enable()
		}

		btnLogin.Refresh()
	}
	wPassword.SetPlaceHolder("Password")

	// Get account databases in app directory
	list, err := GetAccounts()
	if err != nil {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAlert(2))
	}

	// Wallet selection buttons (up to 3) with dropdown for remaining
	walletButtons := container.NewVBox()
	var walletBtns []*walletBtn

	selectWallet := func(walletName string) {
		session.Error = ""
		switch session.Network {
		case NETWORK_TESTNET:
			session.Path = filepath.Join(AppPath(), "testnet") + string(filepath.Separator) + walletName
		case NETWORK_SIMULATOR:
			session.Path = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator) + walletName
		default:
			session.Path = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator) + walletName
		}
		if !session.Offline {
			btnLogin.Text = "Connect"
		} else {
			btnLogin.Text = "Decrypt"
		}
		btnLogin.Enable()
		safeCanvasFocus(wPassword)
		lastWalletKey := "last_wallet_" + session.Network
		StoreValue("settings", []byte(lastWalletKey), []byte(walletName))
	}

	unselectButtons := func() {
		for _, b := range walletBtns {
			b.SetColors(colors.DarkMatter, colors.Gray)
		}
	}

	logoGreen := color.RGBA{R: 70, G: 184, B: 104, A: 0xff}

	for i, walletName := range list {
		if i >= 3 {
			break
		}
		selectedWallet := walletName
		btn := newWalletBtn(strings.TrimSuffix(walletName, ".db"), nil)
		btn.onTapped = func() {
			unselectButtons()
			btn.SetColors(logoGreen, color.Black)
			selectWallet(selectedWallet)
		}
		walletBtns = append(walletBtns, btn)
		walletButtons.Add(container.New(layout.NewGridLayout(1), btn))
	}

	if len(list) > 3 {
		extraList := list[3:]
		displayNames := make([]string, len(extraList))
		displayToWallet := make(map[string]string, len(extraList))
		for i, name := range extraList {
			display := strings.TrimSuffix(name, ".db")
			displayNames[i] = display
			displayToWallet[display] = name
		}
		extraDropdown := widget.NewSelect(displayNames, func(s string) {
			unselectButtons()
			selectWallet(displayToWallet[s])
		})
		extraDropdown.PlaceHolder = fmt.Sprintf("More wallets (%d)", len(extraList))
		walletButtons.Add(extraDropdown)
	}

	// Auto-select last used wallet if available, otherwise first wallet
	if len(list) >= 1 {
		autoSelectWallet := list[0]
		lastWalletKey := "last_wallet_" + session.Network
		if lastWallet, err := GetValue("settings", []byte(lastWalletKey)); err == nil && len(lastWallet) > 0 {
			lastWalletName := string(lastWallet)
			for _, name := range list {
				if name == lastWalletName {
					autoSelectWallet = lastWalletName
					break
				}
			}
		}
		for i, btn := range walletBtns {
			if list[i] == autoSelectWallet {
				btn.SetColors(logoGreen, color.Black)
				break
			}
		}
		selectWallet(autoSelectWallet)
	} else {
		wPassword.Disable()
	}

	wSpacer := widget.NewLabel(" ")

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	headerBlock := canvas.NewRectangle(color.Transparent)
	headerBlock.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))

	headerBox := canvas.NewRectangle(color.Transparent)
	headerBox.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	frame := &iframe{}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	status.Connection.FillColor = colors.Gray
	status.RemoteAccess.FillColor = colors.Gray
	status.Gnomon.FillColor = colors.Gray
	status.EPOCH.FillColor = colors.Gray
	status.Sync.FillColor = colors.Gray

	// Create uniform button container - each button same size
	buttonGrid := container.NewVBox(
		container.New(layout.NewGridLayout(1),
			btnNewAccount,
		),
		rectSpacer,
		container.New(layout.NewGridLayout(1),
			btnRecoverAccount,
		),
		rectSpacer,
		container.New(layout.NewGridLayout(1),
			btnConnectionSettings,
		),
	)

	// Footer text
	copyrightLabel := canvas.NewText("Copyright 2023-2026 DERO Foundation. All rights reserved.", colors.Gray)
	copyrightLabel.TextSize = scaleFont(10)
	copyrightLabel.Alignment = fyne.TextAlignCenter

	versionLabel := canvas.NewText(fmt.Sprintf("Engram v%s", versionString), colors.Gray)
	versionLabel.TextSize = scaleFont(10)
	versionLabel.Alignment = fyne.TextAlignCenter

	footer := container.NewVBox(
		copyrightLabel,
		versionLabel,
	)

	form := container.NewStack(
		res.mainBg,
		container.NewVBox(
			wSpacer,
			container.NewStack(
				headerBlock,
			),
			rectSpacer,
			rectSpacer,
			walletButtons,
			rectSpacer,
			wPassword,
			rectSpacer,
			mode,
			rectSpacer,
			btnLogin,
			rectSpacer,
			buttonGrid,
			footer,
		),
	)

	if isMobile() {
		form = container.NewStack(
			res.mainBg,
			container.NewVBox(
				wSpacer,
				container.NewStack(
					headerBlock,
				),
				rectSpacer,
				rectSpacer,
				walletButtons,
				rectSpacer,
				wPassword,
				rectSpacer,
				mode,
				rectSpacer,
				wrapMobileButton(btnLogin),
				rectSpacer,
				container.NewVBox(
					container.New(layout.NewGridLayout(1),
						wrapMobileButton(btnNewAccount),
					),
					rectSpacer,
					container.New(layout.NewGridLayout(1),
						wrapMobileButton(btnRecoverAccount),
					),
					rectSpacer,
					container.New(layout.NewGridLayout(1),
						wrapMobileButton(btnConnectionSettings),
					),
				),
				footer,
			),
		)
	}

	layout := container.NewStack(
		frame,
		container.NewBorder(
			container.NewVBox(
				container.NewCenter(
					form,
				),
			),
			nil,
			nil,
			nil,
		),
	)

	// Register with navigation stack (main screen does not allow back)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, false)
	}

	// Focus password field after render for quick sign-in when wallet is auto-selected
	if len(list) >= 1 {
		go func() {
			time.Sleep(100 * time.Millisecond)
			safeCanvasFocus(wPassword)
		}()
	}

	return NewVScroll(layout)
}

// layoutSingleWalletLogin shows a simplified login screen for when only 1 wallet exists
func layoutSingleWalletLogin(walletName string) fyne.CanvasObject {
	a.Settings().SetTheme(themes.main)
	session.Domain = "app.main"
	session.Password = ""

	// Set the wallet path automatically
	switch session.Network {
	case NETWORK_TESTNET:
		session.Path = filepath.Join(AppPath(), "testnet") + string(filepath.Separator) + walletName
	case NETWORK_SIMULATOR:
		session.Path = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator) + walletName
	default:
		session.Path = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator) + walletName
	}

	// Display wallet name
	lblWalletName := canvas.NewText(strings.TrimSuffix(walletName, ".db"), colors.Green)
	lblWalletName.TextSize = scaleFont(16)
	lblWalletName.Alignment = fyne.TextAlignCenter
	lblWalletName.TextStyle = fyne.TextStyle{Bold: true}

	// Password entry
	wPassword := NewReturnEntry()
	wPassword.Password = true
	wPassword.SetPlaceHolder("Password")

	// Login button
	btnLogin := widget.NewButtonWithIcon("Connect", theme.LoginIcon(), nil)
	btnLogin.Disable()

	if session.Error != "" {
		btnLogin.Text = session.Error
		btnLogin.Disable()
		btnLogin.Refresh()
		session.Error = ""
	}

	btnLogin.OnTapped = func() {
		if session.Password == "" {
			btnLogin.Text = "Invalid password..."
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			if !session.Offline {
				btnLogin.Text = "Connect"
			} else {
				btnLogin.Text = "Decrypt"
			}
			btnLogin.Enable()
			btnLogin.Refresh()
			login()
			btnLogin.Text = session.Error
			btnLogin.Disable()
			btnLogin.Refresh()
			session.Error = ""
		}
	}

	wPassword.OnReturn = btnLogin.OnTapped
	wPassword.OnChanged = func(s string) {
		session.Error = ""
		if !session.Offline {
			btnLogin.Text = "Connect"
		} else {
			btnLogin.Text = "Decrypt"
		}
		btnLogin.Enable()
		btnLogin.Refresh()
		session.Password = s

		if len(s) < 1 {
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			btnLogin.Enable()
		}

		btnLogin.Refresh()
	}

	// Auto-focus password field
	go func() {
		time.Sleep(100 * time.Millisecond)
		fyne.Do(func() {
			safeCanvasFocus(wPassword)
		})
	}()

	// Offline mode toggle
	modeData := binding.BindBool(&session.Offline)
	mode := widget.NewCheckWithData(" Offline Mode", modeData)
	mode.OnChanged = func(b bool) {
		if b {
			session.Offline = true
			btnLogin.Text = "Decrypt"
			btnLogin.Refresh()
		} else {
			session.Offline = false
			btnLogin.Text = "Connect"
			btnLogin.Refresh()
		}
	}

	// Switch Account button
	btnSwitchAccount := newGunmetalButtonWithIcon("Switch Account", theme.AccountIcon(), colors.Green, func() {
		session.Domain = "app.main"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	}, ui.Width*0.9)

	// Connection Settings button
	btnConnectionSettings := newGunmetalButtonWithIcon("Connection Settings", theme.SettingsIcon(), colors.Green, func() {
		session.Domain = "app.settings"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSettings())
		removeOverlays()
	}, ui.Width*0.9)

	// Handle return key
	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain == "app.main" {
			if k.Name == fyne.KeyReturn {
				if session.Password == "" {
					btnLogin.Text = "Invalid password..."
					btnLogin.Disable()
					btnLogin.Refresh()
				} else {
					if !session.Offline {
						btnLogin.Text = "Connect"
					} else {
						btnLogin.Text = "Decrypt"
					}
					btnLogin.Enable()
					btnLogin.Refresh()
					login()
					btnLogin.Text = session.Error
					btnLogin.Disable()
					btnLogin.Refresh()
					session.Error = ""
				}
			}
		}
	})

	// Layout
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))

	// Reserve space for logo (matches layoutMain headerBlock)
	headerBlock := canvas.NewRectangle(color.Transparent)
	headerBlock.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))

	isMobile := a.Driver().Device().IsMobile()

	frame := &iframe{}

	var form *fyne.Container
	if isMobile {
		buttonGroup := container.NewVBox(
			container.New(layout.NewGridLayout(1),
				wrapMobileButton(btnSwitchAccount),
			),
			rectSpacer,
			container.New(layout.NewGridLayout(1),
				wrapMobileButton(btnConnectionSettings),
			),
		)

		form = container.NewVBox(
			rectSpacer,
			container.NewStack(headerBlock),
			rectSpacer,
			rectSpacer,
			lblWalletName,
			rectSpacer,
			wPassword,
			rectSpacer,
			mode,
			rectSpacer,
			wrapMobileButton(btnLogin),
			rectSpacer,
			buttonGroup,
		)
	} else {
		buttonGroup := container.NewVBox(
			btnSwitchAccount,
			rectSpacer,
			btnConnectionSettings,
		)

		form = container.NewVBox(
			rectSpacer,
			container.NewStack(headerBlock),
			rectSpacer,
			lblWalletName,
			rectSpacer,
			wPassword,
			rectSpacer,
			mode,
			rectSpacer,
			btnLogin,
			rectSpacer,
			buttonGroup,
		)
	}

	layout := container.NewStack(
		frame,
		res.mainBg,
		container.NewCenter(form),
	)

	return layout
}

func layoutDashboardMarquee() fyne.CanvasObject {
	marqueeMu.Lock()
	if introShownThisSession {
		marqueeMu.Unlock()
		return layout.NewSpacer()
	}
	// Mark as shown for this login session
	introShownThisSession = true

	marqueeMu.Unlock()

	messages := []string{
		"0.6.9",
		"DERO PRIVACY TOGETHER",
	}

	text := canvas.NewText(messages[0], colors.LightBlue)
	text.TextStyle = fyne.TextStyle{Symbol: true}
	text.TextSize = scaleFont(14)
	text.Alignment = fyne.TextAlignCenter

	go func() {
		// Initial display
		time.Sleep(4 * time.Second)

		// Loop through remaining messages once
		for i := 1; i < len(messages); i++ {
			if appExiting {
				return
			}

			// Fade out
			fadeOut := canvas.NewColorRGBAAnimation(
				colors.LightBlue.(color.RGBA),
				color.RGBA{0, 0, 0, 0},
				400*time.Millisecond,
				func(c color.Color) {
					text.Color = c
					text.Refresh()
				})
			fadeOut.Start()
			time.Sleep(450 * time.Millisecond)

			// Swap text
			fyne.Do(func() {
				text.Text = messages[i]
				text.Refresh()
			})
			time.Sleep(100 * time.Millisecond)

			// Fade in
			fadeIn := canvas.NewColorRGBAAnimation(
				color.RGBA{0, 0, 0, 0},
				colors.LightBlue.(color.RGBA),
				400*time.Millisecond,
				func(c color.Color) {
					text.Color = c
					text.Refresh()
				})
			fadeIn.Start()
			time.Sleep(4 * time.Second)
		}

		// Final fade to black
		if appExiting {
			return
		}
		fadeOutFinal := canvas.NewColorRGBAAnimation(
			colors.LightBlue.(color.RGBA),
			color.RGBA{0, 0, 0, 0},
			400*time.Millisecond,
			func(c color.Color) {
				text.Color = c
				text.Refresh()
			})
		fadeOutFinal.Start()
	}()

	return container.NewCenter(text)
}

func layoutDashboard() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	session.Dashboard = "main"
	session.Domain = "app.wallet"

	session.BalanceText = canvas.NewText("...", colors.Green)
	session.BalanceText.TextSize = scaleFont(28)
	session.BalanceText.TextStyle = fyne.TextStyle{Bold: true}

	if balanceHiddenVal, err := GetEncryptedValue("settings", []byte("BalanceHidden")); err == nil {
		session.BalanceHidden = string(balanceHiddenVal) == "true"
	} else {
		session.BalanceHidden = true
	}

	if session.BalanceHidden {
		session.BalanceText.Text = "••••••"
	} else {
		go func() {
			if engram.Disk != nil {
				session.Balance, _ = engram.Disk.Get_Balance()
				fyne.Do(func() {
					session.BalanceText.Text = walletapi.FormatMoney(session.Balance)
					session.BalanceText.Refresh()
				})
			}
		}()
	}

	var balanceToggleBtn *widget.Button
	balanceToggleBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		session.BalanceHidden = !session.BalanceHidden

		if session.BalanceHidden {
			balanceToggleBtn.SetIcon(theme.VisibilityOffIcon())
			session.BalanceText.Text = "••••••"
			session.BalanceText.Refresh()
			balanceToggleBtn.Refresh()
			go StoreEncryptedValue("settings", []byte("BalanceHidden"), []byte("true"))
		} else {
			balanceToggleBtn.SetIcon(theme.VisibilityIcon())
			session.BalanceText.Text = "..."
			session.BalanceText.Refresh()
			balanceToggleBtn.Refresh()
			go func() {
				if engram.Disk != nil {
					currentBalance, _ := engram.Disk.Get_Balance()
					fyne.Do(func() {
						session.BalanceText.Text = walletapi.FormatMoney(currentBalance)
						session.BalanceText.Refresh()
					})
				}
				StoreEncryptedValue("settings", []byte("BalanceHidden"), []byte("false"))
			}()
		}
	})
	balanceToggleBtn.Importance = widget.LowImportance

	if session.BalanceHidden {
		balanceToggleBtn.SetIcon(theme.VisibilityOffIcon())
	}

	if addressHiddenVal, err := GetEncryptedValue("settings", []byte("AddressHidden")); err == nil {
		session.AddressHidden = string(addressHiddenVal) == "true"
	} else {
		session.AddressHidden = true
	}

	network := ""
	switch session.Network {
	case NETWORK_TESTNET:
		network = " T  E  S  T  N  E  T "
	case NETWORK_SIMULATOR:
		network = " S  I  M  U  L  A  T  O  R "
	default:
		network = " M  A  I  N  N  E  T "
	}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	frame := &iframe{}

	balanceCenter := container.NewCenter(
		container.NewHBox(
			session.BalanceText,
			balanceToggleBtn,
		),
	)

	path := strings.Split(session.Path, string(filepath.Separator))
	accountName := canvas.NewText(path[len(path)-1], colors.Green)
	accountName.TextStyle = fyne.TextStyle{Bold: true}
	accountName.TextSize = scaleFont(18)

	gramSend := newLargeIconButton(" Send ", theme.UploadIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSend())
		removeOverlays()
	}, 140)

	heading := canvas.NewText("B A L A N C E", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	sendDesc := canvas.NewText("Add Transfer Details", colors.Gray)
	sendDesc.TextSize = scaleFont(18)
	sendDesc.Alignment = fyne.TextAlignCenter
	sendDesc.TextStyle = fyne.TextStyle{Bold: true}

	sendHeading := canvas.NewText("Send Money", colors.Green)
	sendHeading.TextSize = scaleFont(22)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	headerLabel := canvas.NewText("  "+network+"  ", colors.Gray)
	headerLabel.TextSize = scaleFont(11)
	headerLabel.Alignment = fyne.TextAlignCenter
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}

	statusLabel := canvas.NewText("  S T A T U S  ", colors.Gray)
	statusLabel.TextSize = scaleFont(11)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	daemonLabel := canvas.NewText("OFFLINE", colors.Gray)
	daemonLabel.TextSize = scaleFont(12)
	daemonLabel.Alignment = fyne.TextAlignCenter
	daemonLabel.TextStyle = fyne.TextStyle{Bold: false}

	remoteAccessText := "REMOTE ACCESS"
	if remoteAccess.WS.server != nil {
		remoteAccessText = "REMOTE ACCESS (WS)"
	} else if remoteAccess.RPC.server != nil {
		remoteAccessText = "REMOTE ACCESS (RPC)"
	} else {
		status.RemoteAccess.FillColor = colors.Gray
		status.RemoteAccess.Refresh()
	}

	remoteAccessLabel := canvas.NewText(remoteAccessText, colors.Gray)
	remoteAccessLabel.TextSize = scaleFont(12)
	remoteAccessLabel.Alignment = fyne.TextAlignTrailing
	remoteAccessLabel.TextStyle = fyne.TextStyle{Bold: false}

	gnomonLabel := canvas.NewText("GNOMON", colors.Gray)
	gnomonLabel.TextSize = scaleFont(12)
	gnomonLabel.Alignment = fyne.TextAlignCenter
	gnomonLabel.TextStyle = fyne.TextStyle{Bold: false}

	epochLabel := canvas.NewText("EPOCH", colors.Gray)
	epochLabel.TextSize = scaleFont(12)
	epochLabel.Alignment = fyne.TextAlignTrailing
	epochLabel.TextStyle = fyne.TextStyle{Bold: false}
	if !epoch.IsActive() {
		if remoteAccess.EPOCH.err != nil {
			status.EPOCH.FillColor = colors.Red
			status.EPOCH.Refresh()
		} else {
			status.EPOCH.FillColor = colors.Gray
			status.EPOCH.Refresh()
		}
	}

	telaLabel := canvas.NewText("TELA", colors.Gray)
	telaLabel.TextSize = scaleFont(12)
	telaLabel.Alignment = fyne.TextAlignCenter
	telaLabel.TextStyle = fyne.TextStyle{Bold: false}

	telaStatus := canvas.NewCircle(colors.Gray)
	if len(getTelaActiveServers()) > 0 {
		telaStatus.FillColor = colors.Green
	}

	syncAnimationCanvas := canvas.NewCircle(color.Transparent)
	gnomonAnimationCanvas := canvas.NewCircle(color.Transparent)
	epochAnimationCanvas := canvas.NewCircle(color.Transparent)

	if !session.Offline {
		if len(session.Daemon) > 30 {
			daemonLabel.Text = "..." + session.Daemon[len(session.Daemon)-27:]
		} else {
			daemonLabel.Text = session.Daemon
		}

		animationStatus := canvas.NewColorRGBAAnimation(
			color.Transparent,
			colors.Yellow,
			2*time.Second,
			func(c color.Color) {
				syncAnimationCanvas.FillColor = c
				syncAnimationCanvas.Refresh()
				gnomonAnimationCanvas.FillColor = c
				gnomonAnimationCanvas.Refresh()
				epochAnimationCanvas.FillColor = c
				epochAnimationCanvas.Refresh()
			})

		animationStatus.RepeatCount = fyne.AnimationRepeatForever
		animationStatus.AutoReverse = true
		animationStatus.Start()
	}

	session.WalletHeight = engram.Disk.Get_Height()
	session.StatusText = canvas.NewText(fmt.Sprintf("%d", session.WalletHeight), colors.Gray)
	session.StatusText.TextSize = scaleFont(12)
	session.StatusText.Alignment = fyne.TextAlignTrailing
	session.StatusText.TextStyle = fyne.TextStyle{Bold: false}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, scaleSize(2)))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, scaleSize(2)))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	buttonWidth := ui.Width * 0.9 / 3

	btnExit := newIconLabelButtonWithColor("Exit", theme.LogoutIcon(), colors.SoftRed, func() {
		if session.Navigating {
			return
		}
		session.Navigating = true
		defer func() { session.Navigating = false }()
		closeWallet()
	}, buttonWidth)

	btnSettings := newIconLabelButton("Settings", theme.SettingsIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	}, buttonWidth)

	btnNotes := newIconLabelButton("Notes", theme.DocumentIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDatapad())
		removeOverlays()
	}, buttonWidth)

	btnMessages := newIconLabelButton("Messages", theme.MailComposeIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMessages())
		removeOverlays()
	}, buttonWidth)

	btnContracts := newIconLabelButton("Contracts", theme.FolderIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutFilesAndContracts())
		removeOverlays()
	}, buttonWidth)

	linkHistory := newBorderedButtonWithIcon("History", theme.HistoryIcon(), color.White, func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutHistory())
		removeOverlays()
	}, 140)

	linkMyAccount := newBorderedButtonWithIcon("My Account", theme.AccountIcon(), color.White, func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAccount())
		removeOverlays()
	}, 140)

	btnReceive := newLargeIconButton("Receive", theme.DownloadIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutReceive())
		removeOverlays()
	}, 140)

	separator := canvas.NewText(" | ", colors.Gray)
	separator.TextSize = scaleFont(14)
	separator.Alignment = fyne.TextAlignCenter

	res.gram.SetMinSize(fyne.NewSize(ui.Width, scaleSize(150)))

	res.logoContainer = container.NewStack()
	var updateLogo func()
	updateLogo = func() {
		if res.logoContainer == nil {
			return
		}
		res.logoContainer.Objects = nil
		res.villagerMu.Lock()
		vImg := res.villager
		res.villagerMu.Unlock()

		if vImg != nil && !session.VillagerHidden {
			vImg.SetMinSize(fyne.NewSize(ui.Width, scaleSize(150)))
			res.logoContainer.Add(vImg)
		} else {
			res.logoContainer.Add(res.gram)
		}
		res.logoContainer.Refresh()
	}

	updateLogo()

	// Create a transparent button over the logo for toggling or refreshing
	logoBtn := widget.NewButton("", func() {
		showVillagerMenu(updateLogo)
	})
	logoBtn.Importance = widget.LowImportance

	logoStack := container.NewStack(res.logoContainer, logoBtn)

	res.villagerMu.Lock()
	noVillager := res.villager == nil
	res.villagerMu.Unlock()

	if noVillager && !session.VillagerPopupShown {
		seen, found := getTELADual("VillagerPopupSeen")
		if !found || seen != "true" {
			session.VillagerPopupShown = true
			go setTELADual("VillagerPopupSeen", []byte("true"))
			showVillagerPopup(logoStack)
		}
	}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectSpacerLarge := canvas.NewRectangle(color.Transparent)
	rectSpacerLarge.SetMinSize(scalePoint(10, 25))

	rectSquare := canvas.NewRectangle(color.Transparent)
	rectSquare.SetMinSize(smallSpacerSize())

	dotSz := statusDotSize()
	connectionDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, status.Connection))
	syncDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, syncAnimationCanvas, status.Sync))
	gnomonDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, gnomonAnimationCanvas, status.Gnomon))
	epochDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, epochAnimationCanvas, status.EPOCH))
	telaDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, telaStatus))
	remoteAccessDot := container.NewGridWrap(dotSz, container.NewStack(rectStatus, status.RemoteAccess))

	rectOffset := canvas.NewRectangle(color.Transparent)
	rectOffset.SetMinSize(scalePoint(81, 1))

	deroForm := container.NewVBox(
		rectSpacer,
		logoStack,
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				line1,
				layout.NewSpacer(),
				headerLabel,
				layout.NewSpacer(),
				line2,
			),
		),
		rectSpacer,
		rectSpacer,
		heading,
		rectSpacer,
		balanceCenter,
		rectSpacer,
		container.NewCenter(
			container.NewHBox(
				gramSend,
				scaleSpacer(10),
				btnReceive,
			),
		),
		rectSpacer,
		container.NewCenter(
			container.NewHBox(
				linkMyAccount,
				NewSpacer(10, 5),
				linkHistory,
			),
		),
		rectSpacer,
		container.NewHBox(
			line1,
			layout.NewSpacer(),
			statusLabel,
			layout.NewSpacer(),
			line2,
		),
		rectSpacer,
		container.NewVBox(
			container.NewHBox(
				connectionDot,
				rectSquare,
				daemonLabel,
				layout.NewSpacer(),
				container.NewStack(
					rectOffset,
					session.StatusText,
				),
				rectSquare,
				syncDot,
			),
			rectOffset,
			container.NewHBox(
				gnomonDot,
				rectSquare,
				gnomonLabel,
				layout.NewSpacer(),
				container.NewStack(
					rectOffset,
					epochLabel,
				),
				rectSquare,
				epochDot,
			),
			rectOffset,
			container.NewHBox(
				telaDot,
				rectSquare,
				telaLabel,
				layout.NewSpacer(),
				container.NewStack(
					rectOffset,
					remoteAccessLabel,
				),
				rectSquare,
				remoteAccessDot,
			),
		),
	)

	grid := container.NewCenter(
		deroForm,
	)

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.wallet" {
			return
		}

		if k.Name == fyne.KeyRight {
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutRemoteAccess())
			removeOverlays()
		} else if k.Name == fyne.KeyLeft {
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutFilesAndContracts())
			removeOverlays()
		}
	})

	top := container.NewCenter(
		layout.NewSpacer(),
		grid,
		layout.NewSpacer(),
	)

	bottomRowWidth := ui.Width * 0.9

	btnTELAWeb := newTELAButton(func() {
		logger.Printf("[TELA-BUTTON] === ENTRY - button callback started ===\n")

		if session.Navigating {
			logger.Printf("[TELA-BUTTON] Already navigating, returning early\n")
			return
		}

		session.Navigating = true
		logger.Printf("[TELA-BUTTON] Set Navigating=true\n")

		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[TELA-BUTTON] === PANIC RECOVERED ===\n")
				logger.Errorf("[TELA-BUTTON] Panic value: %v\n", r)
				logger.Errorf("[TELA-BUTTON] Stack: %s\n", debug.Stack())
				session.Navigating = false
				session.Domain = "app.wallet"

				fyne.Do(func() {
					if session.Window != nil {
						session.Window.SetContent(layoutDashboard())
					}
				})
				logger.Printf("[TELA-BUTTON] === PANIC RECOVERY COMPLETE ===\n")
			}
		}()

		defer func() {
			session.Navigating = false
			logger.Printf("[TELA-BUTTON] Reset Navigating=false\n")
		}()

		logger.Printf("[TELA-BUTTON] Checking state - gnomon.Index=%v walletapi.Connected=%v engram.Disk=%v session.WalletOpen=%v\n",
			gnomon.Index != nil, walletapi.Connected, engram.Disk != nil, session.WalletOpen)

		if gnomon.Index == nil {
			logger.Printf("[TELA-BUTTON] Guard triggered - gnomon.Index is nil\n")
			showLoadingOverlay()
			fyne.Do(func() {
				errLabel := canvas.NewText("Gnomon is initializing, please wait...", colors.Yellow)
				errLabel.Alignment = fyne.TextAlignCenter
				content := container.NewCenter(container.NewVBox(errLabel))
				if session.Window != nil {
					session.Window.SetContent(content)
				}
			})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[TELA-BUTTON] Retry goroutine panic: %v\n", r)
					}
				}()
				time.Sleep(3 * time.Second)
				logger.Printf("[TELA-BUTTON] Retry check - gnomon.Index=%v walletapi.Connected=%v\n", gnomon.Index != nil, walletapi.Connected)
				if gnomon.Index != nil {
					fyne.Do(func() {
						if session.Window != nil && session.WalletOpen {
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutTELA())
							removeOverlays()
						}
					})
				} else {
					fyne.Do(func() {
						if session.Window != nil && session.WalletOpen {
							session.Window.SetContent(layoutDashboard())
						}
					})
				}
			}()
			return
		}

		if !walletapi.Connected {
			logger.Printf("[TELA-BUTTON] Guard triggered - walletapi.Connected is false\n")
			showLoadingOverlay()
			fyne.Do(func() {
				errLabel := canvas.NewText("Waiting for connection...", colors.Yellow)
				errLabel.Alignment = fyne.TextAlignCenter
				content := container.NewCenter(container.NewVBox(errLabel))
				if session.Window != nil {
					session.Window.SetContent(content)
				}
			})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[TELA-BUTTON] Connection retry goroutine panic: %v\n", r)
					}
				}()
				time.Sleep(3 * time.Second)
				logger.Printf("[TELA-BUTTON] Connection retry check - gnomon.Index=%v walletapi.Connected=%v\n", gnomon.Index != nil, walletapi.Connected)
				if walletapi.Connected && gnomon.Index != nil {
					fyne.Do(func() {
						if session.Window != nil && session.WalletOpen {
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutTELA())
							removeOverlays()
						}
					})
				} else {
					fyne.Do(func() {
						if session.Window != nil && session.WalletOpen {
							session.Window.SetContent(layoutDashboard())
							removeOverlays()
						}
					})
				}
			}()
			return
		}

		if session.Window == nil {
			logger.Errorf("[TELA-BUTTON] ERROR: session.Window is nil, cannot proceed\n")
			return
		}

		if !session.WalletOpen {
			logger.Errorf("[TELA-BUTTON] ERROR: session.WalletOpen is false, wallet not open\n")
			return
		}

		if engram.Disk == nil {
			logger.Errorf("[TELA-BUTTON] ERROR: engram.Disk is nil, wallet not loaded\n")
			return
		}

		logger.Printf("[TELA-BUTTON] All guards passed, proceeding to open TELA...\n")

		asked := hasAskedXSWD()
		alreadyEnabled := remoteAccess.WS.global.enabled
		logger.Printf("[TELA-BUTTON] hasAskedXSWD returned: %v, WS enabled: %v\n", asked, alreadyEnabled)
		if !asked && !alreadyEnabled {
			logger.Printf("[TELA-BUTTON] First-time XSWD prompt for wallet\n")
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			go func() {
				logger.Printf("[TELA-BUTTON] showXSWDPrompt() called from goroutine\n")
				if showXSWDPrompt() {
					logger.Printf("[TELA-BUTTON] User allowed XSWD\n")
					remoteAccess.WS.global.enabled = true
					if remoteAccess.WS.port == "" {
						remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
						setRemoteAccessDual(remoteAccess.WS.port, "WS")
					}
					setPermissions()
					if remoteAccess.WS.server == nil {
						toggleXSWD(remoteAccess.WS.port)
					}
				} else {
					logger.Printf("[TELA-BUTTON] User denied XSWD\n")
					remoteAccess.WS.global.enabled = false
					setPermissions()
					if remoteAccess.WS.server != nil {
						toggleXSWD(remoteAccess.WS.port)
					}
				}
				setAskedXSWD()

				fyne.Do(func() {
					if session.Window == nil || !session.WalletOpen {
						return
					}
					removeOverlays()
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutTELA())
					removeOverlays()
					session.Navigating = false
				})
			}()
			return
		}

		logger.Printf("[TELA-BUTTON] Capturing session.Window.Content()\n")
		currentContent := session.Window.Content()
		if currentContent == nil {
			logger.Printf("[TELA-BUTTON] Warning: current content is nil\n")
		}
		session.LastDomain = currentContent

		logger.Printf("[TELA-BUTTON] Setting transition content\n")
		session.Window.SetContent(layoutTransition())

		logger.Printf("[TELA-BUTTON] Calling layoutTELA()\n")
		telaLayout := layoutTELA()
		logger.Printf("[TELA-BUTTON] layoutTELA() returned, setting content\n")

		if telaLayout == nil {
			logger.Errorf("[TELA-BUTTON] ERROR: layoutTELA() returned nil\n")
			session.Window.SetContent(layoutDashboard())
			return
		}

		session.Window.SetContent(telaLayout)
		logger.Printf("[TELA-BUTTON] TELA content set, removing overlays\n")

		removeOverlays()
		logger.Printf("[TELA-BUTTON] === TELA opened successfully ===\n")
	}, buttonWidth)

	bottom := container.NewStack(
		container.NewVBox(
			container.NewCenter(
				container.NewHBox(
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnNotes.MinSize().Height), btnNotes),
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnTELAWeb.MinSize().Height), btnTELAWeb),
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnMessages.MinSize().Height), btnMessages),
				),
			),
			container.NewCenter(
				container.NewHBox(
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnContracts.MinSize().Height), btnContracts),
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnExit.MinSize().Height), btnExit),
					container.NewGridWrap(fyne.NewSize(bottomRowWidth/3, btnSettings.MinSize().Height), btnSettings),
				),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		container.NewCenter(layoutDashboardMarquee()),
	)

	layout := container.NewStack(
		frame,
		c,
	)

	// Register with navigation stack (dashboard allows back to main)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutSend() fyne.CanvasObject {
	session.Domain = "app.send"

	wSpacer := widget.NewLabel(" ")
	frame := &iframe{}

	btnSend := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), nil)
	btnSend.Disable()

	btnSendNow := widget.NewButtonWithIcon("Send", theme.UploadIcon(), nil)
	btnSendNow.Disable()

	wAmount := widget.NewEntry()
	wAmount.SetPlaceHolder("Amount")

	wMessage := widget.NewEntry()
	wMessage.SetValidationError(nil)
	wMessage.SetPlaceHolder("Message")
	wMessage.Validator = func(s string) error {
		bytes := []byte(s)
		if len(bytes) <= 130 {
			tx.Comment = s
			wMessage.SetValidationError(nil)
			return nil
		} else {
			err := errors.New("message too long")
			wMessage.SetValidationError(err)
			return err
		}
	}

	wPaymentID := widget.NewEntry()
	wPaymentID.Validator = func(s string) (err error) {
		if strings.TrimSpace(s) == "" {
			tx.PaymentID = 0
			wPaymentID.SetValidationError(nil)
			return nil
		}

		tx.PaymentID, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			wPaymentID.SetValidationError(err)
			tx.PaymentID = 0
		}

		return
	}
	wPaymentID.SetPlaceHolder("Payment ID / Service Port")

	options := []string{"Anonymity Set:   2  (None)", "Anonymity Set:   4  (Low)", "Anonymity Set:   8  (Low)", "Anonymity Set:   16  (Recommended)", "Anonymity Set:   32  (Medium)", "Anonymity Set:   64  (High)", "Anonymity Set:   128  (High)"}
	wRings := widget.NewSelect(options, nil)
	wRings.SetSelected("Anonymity Set:   16  (Recommended)")

	wReceiver := widget.NewEntry()
	wReceiver.SetPlaceHolder("Receiver username or address")
	wReceiver.SetValidationError(nil)
	wReceiver.Validator = func(s string) error {
		address, err := globals.ParseValidateAddress(s)
		if err != nil {
			tx.Address = nil
			addr, _ := checkUsername(s, -1)
			if addr == "" {
				btnSend.Disable()
				btnSendNow.Disable()
				err = errors.New("invalid username or address")
				wReceiver.SetValidationError(err)
				tx.Address = nil
				return err
			} else {
				wReceiver.SetValidationError(nil)
				tx.Address, _ = globals.ParseValidateAddress(addr)
				if tx.Amount != 0 {
					balance, _ := engram.Disk.Get_Balance()
					if tx.Amount <= balance {
						btnSend.Enable()
						btnSendNow.Enable()
					}
				}
			}
		} else {
			if address.IsIntegratedAddress() {
				tx.Address = address

				if address.Arguments.HasValue(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
					amount := address.Arguments[address.Arguments.Index(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64)].Value
					tx.Amount = amount.(uint64)
					wAmount.Text = globals.FormatMoney(amount.(uint64))
					if amount.(uint64) != 0.00000 {
						wAmount.Disable()
					}
					wAmount.Refresh()
				}

				if address.Arguments.HasValue(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
					port := address.Arguments[address.Arguments.Index(rpc.RPC_DESTINATION_PORT, rpc.DataUint64)].Value
					tx.PaymentID = port.(uint64)
					wPaymentID.Text = strconv.FormatUint(port.(uint64), 10)
					wPaymentID.Disable()
					wPaymentID.Refresh()
				}

				if address.Arguments.HasValue(rpc.RPC_COMMENT, rpc.DataString) {
					comment := address.Arguments[address.Arguments.Index(rpc.RPC_COMMENT, rpc.DataString)].Value
					tx.Comment = comment.(string)
					wMessage.Text = comment.(string)
					if comment.(string) != "" {
						wMessage.Disable()
					}
					wMessage.Refresh()
				}

				if tx.Ringsize == 0 {
					wRings.SetSelected("Anonymity Set:   16  (Recommended)")
				}

				if tx.Amount != 0 {
					balance, _ := engram.Disk.Get_Balance()
					if tx.Amount <= balance {
						btnSend.Enable()
					}
				}
			} else {
				tx.Address = address
				wReceiver.SetValidationError(nil)
				if tx.Amount != 0 {
					balance, _ := engram.Disk.Get_Balance()
					if tx.Amount <= balance {
						btnSend.Enable()
					}
				}
			}
		}
		return nil
	}

	/*
		// TODO
		wAll := widget.NewCheck(" All", func(b bool) {
			if b {
				tx.Amount = engram.Disk.GetAccount().Balance_Mature
				wAmount.SetText(walletapi.FormatMoney(tx.Amount))
			} else {
				tx.Amount = 0
				wAmount.SetText("")
			}
		})
	*/

	wAmount.Validator = func(s string) error {
		if s == "" {
			tx.Amount = 0
			wAmount.SetValidationError(errors.New("invalid transaction amount"))
			btnSend.Disable()
			btnSendNow.Disable()
		} else {
			balance, _ := engram.Disk.Get_Balance()
			entry, err := globals.ParseAmount(s)
			if err != nil {
				tx.Amount = 0
				wAmount.SetValidationError(errors.New("invalid transaction amount"))
				btnSend.Disable()
				btnSendNow.Disable()
				return errors.New("invalid transaction amount")
			}

			if entry == 0 {
				tx.Amount = 0
				wAmount.SetValidationError(errors.New("invalid transaction amount"))
				btnSend.Disable()
				btnSendNow.Disable()
				return errors.New("invalid transaction amount")
			}

			if entry <= balance {
				tx.Amount = entry
				wAmount.SetValidationError(nil)
				if wReceiver.Validate() == nil {
					btnSend.Enable()
					btnSendNow.Enable()
				}
			} else {
				tx.Amount = 0
				btnSend.Disable()
				btnSendNow.Disable()
				wAmount.SetValidationError(errors.New("insufficient funds"))
			}
			return nil
		}
		return errors.New("invalid transaction amount")
	}

	wAmount.SetValidationError(nil)

	wRings.PlaceHolder = "(Select Anonymity Set)"
	if tx.Ringsize < 2 {
		tx.Ringsize = 16
	} else if len(tx.Pending) > 0 {
		rsIndex := 3
		switch tx.Ringsize {
		case 2:
			rsIndex = 0
		case 4:
			rsIndex = 1
		case 8:
			rsIndex = 2
		case 16:
			rsIndex = 3
		case 32:
			rsIndex = 4
		case 64:
			rsIndex = 5
		case 128:
			rsIndex = 6
		}
		wRings.SetSelectedIndex(rsIndex)
	}

	wRings.OnChanged = func(s string) {
		var err error
		regex := regexp.MustCompile("[0-9]+")
		result := regex.FindAllString(s, -1)
		tx.Ringsize, err = strconv.ParseUint(result[0], 10, 64)
		if err != nil {
			tx.Ringsize = 16
			wRings.SetSelected(options[3])
		}
		safeCanvasFocus(wReceiver)
	}

	btnSend.OnTapped = func() {
		_, err := globals.ParseAmount(wAmount.Text)
		if tx.Address != nil {
			if wRings != nil && err == nil && tx.Address != nil {
				err = addTransfer()
				if err == nil {
					session.LastDomain = session.Window.Content()
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutTransfers())
					removeOverlays()
				}
			} else {
				wReceiver.SetValidationError(errors.New("invalid address"))
				wReceiver.Refresh()
			}
		}
	}

	btnTransfers := widget.NewButtonWithIcon("Transfers", theme.ListIcon(), nil)
	if len(tx.Pending) == 0 {
		btnTransfers.Disable()
	}
	btnTransfers.OnTapped = func() {
		if len(tx.Pending) == 0 {
			return
		}
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
		removeOverlays()
	}

	btnSendNow.OnTapped = func() {
		_, err := globals.ParseAmount(wAmount.Text)
		if tx.Address != nil {
			if wRings != nil && err == nil && tx.Address != nil {
				err = addTransfer()
				if err == nil {
					showLoadingOverlay()
					txid, err := sendTransfers()
					if err != nil {
						log.Printf("[Send] Error: %v\n", err)
						removeOverlays()
						dialog.ShowError(err, session.Window)
						return
					}
					log.Printf("[Send] Transaction sent: %s\n", txid)
					removeOverlays()
					session.LastDomain = session.Window.Content()
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutDashboard())
					removeOverlays()
				}
			} else {
				wReceiver.SetValidationError(errors.New("invalid address"))
				wReceiver.Refresh()
			}
		}
	}

	sendHeading := canvas.NewText("S E N D    D E R O", colors.Gray)
	sendHeading.TextSize = scaleFont(16)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	optionalLabel := canvas.NewText("  O P T I O N A L  ", colors.Gray)
	optionalLabel.TextSize = scaleFont(11)
	optionalLabel.Alignment = fyne.TextAlignCenter
	optionalLabel.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	linkCancel := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
		if len(tx.Pending) == 0 {
			tx = Transfers{}
		}
	})

	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, 260))

	rect300 := canvas.NewRectangle(color.Transparent)
	rect300.SetMinSize(fyne.NewSize(ui.Width, scaleSize(30)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			sendHeading,
		),
		rectSpacer,
		rectSpacer,
	)

	form := container.NewVBox(
		wRings,
		rectSpacer,
		wrapMobileButton(widget.NewButtonWithIcon("Scan QR Code", theme.MediaPhotoIcon(), func() {
			log.Println("QR: Scan button tapped")
			s := camera.NewScanner(session.Window, func(code string) {
				log.Println("QR: Scan result received:", code)
				wReceiver.SetText(code)
				wReceiver.SetValidationError(nil)
				wReceiver.Refresh()
			})
			err := s.Start()
			if err != nil {
				log.Printf("QR: Start error: %v", err)
				dialog.ShowError(err, session.Window)
			}
		})),
		rectSpacer,
		wReceiver,
		wAmount,
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			line1,
			layout.NewSpacer(),
			optionalLabel,
			layout.NewSpacer(),
			line2,
		),
		rectSpacer,
		rectSpacer,
		wPaymentID,
		wMessage,
		wSpacer,
	)

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	grid := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			container.NewVScroll(form),
		),
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.NewStack(
					rect300,
					wrapMobileButton(btnSendNow),
				),
			),
			rectSpacer,
			container.NewCenter(
				container.NewStack(
					rect300,
					wrapMobileButton(btnTransfers),
				),
			),
			rectSpacer,
			container.NewCenter(
				container.NewStack(
					rect300,
					wrapMobileButton(btnSend),
				),
			),
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), linkCancel),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		grid,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	// Register with navigation stack (send allows back navigation)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutReceive() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))
	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	heading := canvas.NewText("R E C E I V E    D E R O", colors.DarkGreen)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	var activeAddress string
	if session.ReceivingAddress != "" {
		activeAddress = session.ReceivingAddress
	} else {
		activeAddress = engram.Disk.GetAddress().String()
	}

	addressLabel := canvas.NewText("", colors.DarkGreen)
	addressLabel.TextSize = scaleFont(22)
	addressLabel.Alignment = fyne.TextAlignCenter
	addressLabel.TextStyle = fyne.TextStyle{Bold: true}

	var addressToggleBtn *widget.Button
	var imageQR *canvas.Image

	updateView := func() {
		session.ReceivingAddress = activeAddress
		if session.AddressHidden {
			addressLabel.Text = "dE...••••••••"
			addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
		} else {
			addressLabel.Text = activeAddress[0:5] + "..." + activeAddress[len(activeAddress)-10:]
			addressToggleBtn.SetIcon(theme.VisibilityIcon())
		}
		addressLabel.Refresh()

		if imageQR != nil {
			qr, err := qrcode.New(activeAddress, qrcode.Highest)
			if err == nil {
				qr.BackgroundColor = color.White
				qr.ForegroundColor = color.Black
				imageQR.Image = qr.Image(int(ui.Width * 0.85))
				imageQR.Refresh()
			}
		}
	}

	addressToggleBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		session.AddressHidden = !session.AddressHidden
		if session.AddressHidden {
			StoreEncryptedValue("settings", []byte("AddressHidden"), []byte("true"))
		} else {
			StoreEncryptedValue("settings", []byte("AddressHidden"), []byte("false"))
		}
		updateView()
	})
	addressToggleBtn.Importance = widget.HighImportance

	addressCopyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(activeAddress)
	})
	addressCopyBtn.Importance = widget.HighImportance

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.ReceivingAddress = "" // Reset on exit
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})
	if len(btnBack.Objects) > 1 {
		if btn, ok := btnBack.Objects[1].(*widget.Button); ok {
			btn.Importance = widget.HighImportance
		}
	}

	qr, err := qrcode.New(activeAddress, qrcode.Highest)
	if err != nil {
		logger.Errorf("[Receive] Error generating QR: %v\n", err)
	} else {
		qr.BackgroundColor = color.White
		qr.ForegroundColor = color.Black
	}
	imageQR = canvas.NewImageFromImage(qr.Image(int(ui.Width * 0.85)))
	imageQR.SetMinSize(fyne.NewSize(ui.Width*0.85, ui.Width*0.85))

	updateView()

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	content := container.NewVBox(
		container.NewCenter(
			container.NewHBox(
				addressLabel,
				addressToggleBtn,
				addressCopyBtn,
			),
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			imageQR,
		),
		rectSpacer,
	)

	rectBox.FillColor = color.White
	features := container.NewStack(
		rectBox,
		NewVScroll(content),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layoutObj := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		features,
	)

	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(container.NewStack(canvas.NewRectangle(color.White), layoutObj))
}

func layoutServiceAddress() fyne.CanvasObject {
	session.Domain = "app.service"

	wSpacer := widget.NewLabel(" ")
	frame := &iframe{}

	btnCreate := widget.NewButton("Create", nil)

	wPaymentID := widget.NewEntry()

	wReceiver := widget.NewEntry()
	wReceiver.Text = engram.Disk.GetAddress().String()
	wReceiver.Disable()

	tx.Address, _ = globals.ParseValidateAddress(engram.Disk.GetAddress().String())

	wReceiver.SetPlaceHolder("Receiver username or address")
	wReceiver.SetValidationError(nil)

	wAmount := widget.NewEntry()
	wAmount.SetPlaceHolder("Amount")

	wMessage := widget.NewEntry()
	wMessage.SetPlaceHolder("Message")
	wMessage.Validator = func(s string) (err error) {
		bytes := []byte(s)
		if len(bytes) <= 130 {
			tx.Comment = s
		} else {
			err = errors.New("message too long")
			wMessage.SetValidationError(err)
		}

		return
	}

	wAmount.Validator = func(s string) error {
		if s == "" {
			tx.Amount = 0
			wAmount.SetValidationError(nil)
			btnCreate.Enable()
			return nil
		} else {
			amount, err := globals.ParseAmount(s)
			if err != nil {
				tx.Amount = 0
				wAmount.SetValidationError(errors.New("invalid transaction amount"))
				btnCreate.Disable()
				return errors.New("invalid transaction amount")
			}
			wAmount.SetValidationError(nil)
			tx.Amount = amount
			btnCreate.Enable()

			return nil
		}
	}

	wAmount.SetValidationError(nil)

	wPaymentID.Validator = func(s string) (err error) {
		if s == "" {
			tx.PaymentID = 0
			wPaymentID.SetValidationError(nil)
			btnCreate.Enable()
			return nil
		}
		tx.PaymentID, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			tx.PaymentID = 0
			btnCreate.Disable()
			wPaymentID.SetValidationError(err)
			return
		}
		wPaymentID.SetValidationError(nil)
		btnCreate.Enable()
		return
	}
	wPaymentID.SetPlaceHolder("Payment ID / Service Port")

	sendHeading := canvas.NewText("P A Y M E N T    R E Q U E S T", colors.Gray)
	sendHeading.TextSize = scaleFont(16)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	optionalLabel := canvas.NewText("  O P T I O N A L  ", colors.Gray)
	optionalLabel.TextSize = scaleFont(11)
	optionalLabel.Alignment = fyne.TextAlignCenter
	optionalLabel.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAccount())
		removeOverlays()
	})

	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, 260))

	rect300 := canvas.NewRectangle(color.Transparent)
	rect300.SetMinSize(fyne.NewSize(ui.Width, scaleSize(30)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	btnCreate.OnTapped = func() {
		var err error
		if tx.Address != nil {
			if wAmount.Text != "" {
				_, err = globals.ParseAmount(wAmount.Text)
			}

			if err == nil {
				header := canvas.NewText("CREATE  PAYMENT  REQUEST", colors.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				subHeader := canvas.NewText("Successfully Created", colors.Account)
				subHeader.TextSize = scaleFont(22)
				subHeader.Alignment = fyne.TextAlignCenter
				subHeader.TextStyle = fyne.TextStyle{Bold: true}

				labelAddress := canvas.NewText("-------------    INTEGRATED  ADDRESS    -------------", colors.Gray)
				labelAddress.TextSize = scaleFont(12)
				labelAddress.Alignment = fyne.TextAlignCenter
				labelAddress.TextStyle = fyne.TextStyle{Bold: true}

				btnCopy := widget.NewButton("Copy Payment Request", nil)

				valueAddress := widget.NewRichTextFromMarkdown("")
				valueAddress.Wrapping = fyne.TextWrapBreak

				address := engram.Disk.GetRandomIAddress8()
				address.Arguments = nil
				address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataUint64, Value: uint64(1)})
				if tx.Amount != 0 {
					address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: tx.Amount})
				}
				if tx.PaymentID != 0 {
					address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.PaymentID})
				}
				if tx.Comment != "" {
					address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Comment})
				}

				err := address.Arguments.Validate_Arguments()
				if err != nil {
					logger.Errorf("[Service Address] Error: %s\n", err)
					subHeader.Text = "Error"
					subHeader.Refresh()
					btnCopy.Disable()
				} else {
					logger.Printf("[Service Address] New Integrated Address: %s\n", address.String())
					logger.Printf("[Service Address] Arguments: %s\n", address.Arguments)

					valueAddress.ParseMarkdown("" + address.String())
					valueAddress.Refresh()
				}

				btnCopy.OnTapped = func() {
					a.Clipboard().SetContent(address.String())
				}

				btnClose := newSizedIconButton(theme.NavigateBackIcon(), func() {
					overlay := session.Window.Canvas().Overlays()
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
					overlay.Remove(overlay.Top())
				})

				var imageQR *canvas.Image

				qr, err := qrcode.New(address.String(), qrcode.Highest)
				if err != nil {

				} else {
					qr.BackgroundColor = colors.DarkMatter
					qr.ForegroundColor = colors.Green
				}

				imageQR = canvas.NewImageFromImage(qr.Image(int(ui.Width * 0.65)))
				imageQR.SetMinSize(fyne.NewSize(ui.Width*0.65, ui.Width*0.65))

				span := canvas.NewRectangle(color.Transparent)
				span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

				overlay := session.Window.Canvas().Overlays()

				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)

				overlay.Add(
					container.NewStack(
						&iframe{},
						container.NewCenter(
							container.NewVBox(
								span,
								container.NewCenter(
									header,
								),
								rectSpacer,
								rectSpacer,
								subHeader,
								rectSpacer,
								rectSpacer,
								rectSpacer,
								labelAddress,
								rectSpacer,
								valueAddress,
								rectSpacer,
								rectSpacer,
								container.NewHBox(
									layout.NewSpacer(),
									imageQR,
									layout.NewSpacer(),
								),
								widget.NewLabel(""),
								btnCopy,
								rectSpacer,
								rectSpacer,
								container.NewHBox(
									layout.NewSpacer(),
									btnClose,
									layout.NewSpacer(),
								),
								rectSpacer,
								rectSpacer,
							),
						),
					),
				)
			} else {
				wReceiver.SetValidationError(errors.New("invalid address"))
				wReceiver.Refresh()
			}
		}
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			sendHeading,
		),
		rectSpacer,
		rectSpacer,
	)

	form := container.NewVBox(
		wReceiver,
		wPaymentID,
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			line1,
			layout.NewSpacer(),
			optionalLabel,
			layout.NewSpacer(),
			line2,
		),
		rectSpacer,
		rectSpacer,
		wAmount,
		wMessage,
		wSpacer,
	)

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	grid := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			container.NewVScroll(form),
		),
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.NewStack(
					rect300,
					wrapMobileButton(btnCreate),
				),
			),
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		grid,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutNewAccount() fyne.CanvasObject {
	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}
	a.Settings().SetTheme(themes.alt)

	session.Domain = "app.register"
	session.Language = -1
	session.Error = ""
	session.Name = ""
	session.Password = ""
	session.PasswordConfirm = ""

	languages := mnemonics.Language_List()

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	btnCreate := widget.NewButton("Create", nil)
	btnCreate.Disable()

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Domain = "app.main"
		session.Error = ""
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	})

	btnCopySeed := widget.NewButton("Copy Recovery Words", nil)
	btnCopyAddress := widget.NewButton("Copy Address", nil)

	if !a.Driver().Device().IsMobile() {
		session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
			if session.Domain != "app.register" {
				return
			}

			if k.Name == fyne.KeyReturn {
				errorText.Text = ""
				errorText.Refresh()
				create()
				errorText.Text = session.Error
				errorText.Refresh()
			}
		})
	}

	wPassword := widget.NewEntry()
	wPassword.Password = true
	wPassword.OnChanged = func(s string) {
		session.Error = ""
		errorText.Text = ""
		errorText.Refresh()
		session.Password = s

		if len(session.Password) > 0 && session.Password == session.PasswordConfirm && !findAccount() && session.Language != -1 {
			btnCreate.Enable()
			btnCreate.Refresh()
		} else {
			btnCreate.Disable()
			btnCreate.Refresh()
		}
	}
	wPassword.SetPlaceHolder("Password")
	wPassword.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wPasswordConfirm := widget.NewEntry()
	wPasswordConfirm.Password = true
	wPasswordConfirm.OnChanged = func(s string) {
		session.Error = ""
		errorText.Text = ""
		errorText.Refresh()
		session.PasswordConfirm = s

		if len(session.Password) > 0 && session.Password == session.PasswordConfirm && !findAccount() && session.Language != -1 {
			btnCreate.Enable()
			btnCreate.Refresh()
		} else {
			btnCreate.Disable()
			btnCreate.Refresh()
		}
	}
	wPasswordConfirm.SetPlaceHolder("Confirm Password")
	wPasswordConfirm.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wAccount := widget.NewEntry()
	wAccount.SetPlaceHolder("Account Name")
	wAccount.Validator = func(s string) (err error) {
		session.Error = ""
		errorText.Text = ""
		errorText.Refresh()

		if len(s) > 25 {
			err = errors.New("account name is too long")
			wAccount.SetText(session.Name)
			wAccount.Refresh()
			return
		}

		err = checkDir()
		if err != nil {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutAlert(2))
			return
		}

		switch getNetwork() {
		case NETWORK_TESTNET:
			session.Path = filepath.Join(AppPath(), "testnet", s+".db")
		case NETWORK_SIMULATOR:
			session.Path = filepath.Join(AppPath(), "testnet_simulator", s+".db")
		default:
			session.Path = filepath.Join(AppPath(), "mainnet", s+".db")
		}
		session.Name = s

		if findAccount() {
			err = errors.New("account name already exists")
			errorText.Text = err.Error()
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		} else {
			errorText.Text = ""
			errorText.Refresh()
		}

		if len(session.Password) > 0 && session.Password == session.PasswordConfirm && !findAccount() && session.Language != -1 {
			btnCreate.Enable()
			btnCreate.Refresh()
		} else {
			btnCreate.Disable()
			btnCreate.Refresh()
		}
		return nil
	}

	wAccount.OnChanged = func(s string) {
		wAccount.Validate()
	}

	wLanguage := widget.NewSelect(languages, nil)
	wLanguage.OnChanged = func(s string) {
		index := wLanguage.SelectedIndex()
		session.Language = index
		safeCanvasFocus(wAccount)

		if len(session.Password) > 0 && session.Password == session.PasswordConfirm && !findAccount() && session.Language != -1 {
			btnCreate.Enable()
			btnCreate.Refresh()
		} else {
			btnCreate.Disable()
			btnCreate.Refresh()
		}
	}
	wLanguage.PlaceHolder = "(Select Language)"

	wSpacer := widget.NewLabel(" ")
	heading := canvas.NewText("New Account", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	heading2 := canvas.NewText("Recovery", colors.Green)
	heading2.TextSize = scaleFont(22)
	heading2.Alignment = fyne.TextAlignCenter
	heading2.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	grid := container.NewVBox()
	grid.Objects = nil

	header := container.NewVBox(
		wSpacer,
		heading,
		rectSpacer,
		rectSpacer,
	)

	form := container.NewVBox(
		wLanguage,
		rectSpacer,
		wAccount,
		wPassword,
		wPasswordConfirm,
		rectSpacer,
		errorText,
		rectSpacer,
		wrapMobileButton(btnCreate),
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

	body := widget.NewLabel("Please save the following 25 recovery words in a safe place. These are the keys to your account, so never share them with anyone.")
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	btnEnter := widget.NewButtonWithIcon("Enter", theme.NavigateNextIcon(), func() {
		fyne.Do(func() {
			// Call login() to properly initialize wallet (network, daemon, gnomon, etc.)
			// login() checks if engram.Disk is nil and skips wallet opening if already open
			login()
			// Password will be cleared by login() after successful initialization
		})
	})

	formSuccess := container.NewVBox(
		body,
		wSpacer,
		container.NewCenter(grid),
		rectSpacer,
		errorText,
		rectSpacer,
		wrapMobileButton(btnEnter),
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			wrapMobileButton(btnCopyAddress),
			layout.NewSpacer(),
			wrapMobileButton(btnCopySeed),
			layout.NewSpacer(),
		),
		rectSpacer,
	)

	formSuccess.Hide()

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				form,
				formSuccess,
				func() fyne.CanvasObject {
					if isMobile() {
						return NewSpacer(0, ui.Height*0.4)
					}
					return layout.NewSpacer()
				}(),
			),
			layout.NewSpacer(),
		),
	)
	scrollBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.85))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	btnCreate.OnTapped = func() {
		if findAccount() {
			errorText.Text = "Account name already exists."
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		} else {
			errorText.Text = ""
			errorText.Refresh()
		}

		address, seed, err := create()
		if err != nil {
			errorText.Text = session.Error
			errorText.Refresh()
			return
		}

		formatted := strings.Split(seed, " ")

		rect := canvas.NewRectangle(color.RGBA{21, 27, 36, 255})
		rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(25)))

		for i := 0; i < len(formatted); i++ {
			pos := fmt.Sprintf("%d", i+1)
			word := strings.ReplaceAll(formatted[i], " ", "")
			grid.Add(container.NewStack(
				rect,
				container.NewHBox(
					widget.NewLabel(" "),
					widget.NewLabelWithStyle(pos, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
					layout.NewSpacer(),
					widget.NewLabelWithStyle(word, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewLabel(" "),
				),
			),
			)
		}

		btnCopySeed.OnTapped = func() {
			a.Clipboard().SetContent(seed)
		}

		btnCopyAddress.OnTapped = func() {
			a.Clipboard().SetContent(address)
		}

		form.Hide()
		form.Refresh()
		formSuccess.Show()
		formSuccess.Refresh()
		btnEnter.Refresh()
		grid.Refresh()
		scrollBox.Refresh()
		scrollBox.Offset = fyne.NewPos(0, 0)
		session.Window.Canvas().Content().Refresh()
		session.Window.Canvas().Refresh(session.Window.Content())
	}

	layout := container.NewBorder(
		header,
		footer,
		nil,
		nil,
		scrollBox,
	)
	return layout
}

func layoutRestore() fyne.CanvasObject {
	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}
	a.Settings().SetTheme(themes.alt)

	session.Domain = "app.restore"
	session.Language = -1
	session.Error = ""
	session.Name = ""
	session.Password = ""
	session.PasswordConfirm = ""

	// Cache network result to avoid repeated calls
	cachedNetwork := getNetwork()

	// Debouncer for account name validation (300ms)
	accountDebouncer := NewDebouncer(300 * time.Millisecond)

	scrollBox := container.NewVScroll(nil)

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	// Password strength indicator
	strengthText := canvas.NewText(" ", colors.Gray)
	strengthText.TextSize = scaleFont(11)
	strengthText.Alignment = fyne.TextAlignCenter

	btnCreate := widget.NewButton("Recover", nil)
	btnCreate.Disable()

	seedValid := false
	hexValid := false
	selectedRecoveryType := 0

	updateRecoveryButtonState := func() {
		formValid := validateRecoveryForm(session.Name, session.Password, session.PasswordConfirm)
		keyValid := (selectedRecoveryType == 0 && seedValid) || (selectedRecoveryType == 1 && hexValid)
		if formValid && keyValid {
			btnCreate.Enable()
		} else {
			btnCreate.Disable()
		}
		btnCreate.Refresh()
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Domain = "app.main"
		session.Error = ""
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	})

	btnCopyAddress := widget.NewButtonWithIcon("Copy Address", theme.ContentCopyIcon(), nil)

	wPassword := NewMobileEntry()
	wPassword.Password = true
	wPassword.OnChanged = func(s string) {
		session.Error = ""
		clearFormText(errorText)
		session.Password = s

		// Update password strength indicator
		if len(s) > 0 {
			strength := getPasswordStrength(s)
			strengthText.Text = getPasswordStrengthText(strength)
			strengthText.Color = getPasswordStrengthColor(strength)
			strengthText.Refresh()
		} else {
			clearFormText(strengthText)
		}

		updateRecoveryButtonState()
	}
	wPassword.SetPlaceHolder("Password")
	wPassword.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wPasswordConfirm := NewMobileEntry()
	wPasswordConfirm.Password = true
	wPasswordConfirm.OnChanged = func(s string) {
		session.Error = ""
		clearFormText(errorText)
		session.PasswordConfirm = s

		updateRecoveryButtonState()
	}
	wPasswordConfirm.SetPlaceHolder("Confirm Password")
	wPasswordConfirm.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	// Card selection for recovery type
	selectedRecoveryType = 0 // 0=Words, 1=Hex, 2=Import

	// Card backgrounds for selection state
	cardBgWords := canvas.NewRectangle(colors.Green)
	cardBgHex := canvas.NewRectangle(colors.Gray)
	cardBgImport := canvas.NewRectangle(colors.Gray)

	cardBgWords.CornerRadius = scaleSize(12)
	cardBgHex.CornerRadius = scaleSize(12)
	cardBgImport.CornerRadius = scaleSize(12)

	// Card labels
	lblWords := canvas.NewText("Words", colors.DarkMatter)
	lblWords.TextSize = scaleFont(14)
	lblWords.Alignment = fyne.TextAlignCenter
	lblWords.TextStyle = fyne.TextStyle{Bold: true}

	lblHex := canvas.NewText("Hex Key", color.White)
	lblHex.TextSize = scaleFont(14)
	lblHex.Alignment = fyne.TextAlignCenter
	lblHex.TextStyle = fyne.TextStyle{Bold: true}

	lblImport := canvas.NewText("Import", color.White)
	lblImport.TextSize = scaleFont(14)
	lblImport.Alignment = fyne.TextAlignCenter
	lblImport.TextStyle = fyne.TextStyle{Bold: true}

	// Card descriptions
	descWords := canvas.NewText("25 words", colors.DarkMatter)
	descWords.TextSize = scaleFont(11)
	descWords.Alignment = fyne.TextAlignCenter

	descHex := canvas.NewText("64 chars", color.White)
	descHex.TextSize = scaleFont(11)
	descHex.Alignment = fyne.TextAlignCenter

	descImport := canvas.NewText(".db file", color.White)
	descImport.TextSize = scaleFont(11)
	descImport.Alignment = fyne.TextAlignCenter

	// Card icons - larger size
	iconWords := canvas.NewImageFromResource(theme.DocumentIcon())
	iconWords.SetMinSize(fyne.NewSize(32, 32))
	iconWords.FillMode = canvas.ImageFillContain

	iconHex := canvas.NewImageFromResource(theme.VisibilityOffIcon())
	iconHex.SetMinSize(fyne.NewSize(32, 32))
	iconHex.FillMode = canvas.ImageFillContain

	iconImport := canvas.NewImageFromResource(theme.FolderOpenIcon())
	iconImport.SetMinSize(fyne.NewSize(32, 32))
	iconImport.FillMode = canvas.ImageFillContain

	// Card dimensions determined by padded content

	// Animate card selection with color pulse
	animateCardPulse := func(card *canvas.Rectangle, finalColor color.Color) {
		// Start with bright highlight color
		highlightColor := color.RGBA{R: 150, G: 255, B: 150, A: 255}
		card.FillColor = highlightColor
		card.Refresh()

		// Animate to final color
		anim := fyne.NewAnimation(150*time.Millisecond, func(progress float32) {
			// Interpolate from highlight to final
			hr, hg, hb, _ := highlightColor.RGBA()
			fr, fg, fb, _ := finalColor.RGBA()
			r := uint8((float32(hr>>8) * (1 - progress)) + (float32(fr>>8) * progress))
			g := uint8((float32(hg>>8) * (1 - progress)) + (float32(fg>>8) * progress))
			b := uint8((float32(hb>>8) * (1 - progress)) + (float32(fb>>8) * progress))
			card.FillColor = color.RGBA{R: r, G: g, B: b, A: 255}
			card.Refresh()
		})
		anim.Start()
	}

	// Function to update card styles based on selection
	updateRecoveryTypeCards := func() {
		// Reset all to unselected
		cardBgWords.FillColor = colors.Gray
		cardBgHex.FillColor = colors.Gray
		cardBgImport.FillColor = colors.Gray
		lblWords.Color = color.White
		lblHex.Color = color.White
		lblImport.Color = color.White
		descWords.Color = color.White
		descHex.Color = color.White
		descImport.Color = color.White

		// Highlight selected with animation
		switch selectedRecoveryType {
		case 0:
			animateCardPulse(cardBgWords, colors.Green)
			lblWords.Color = colors.DarkMatter
			descWords.Color = colors.DarkMatter
		case 1:
			animateCardPulse(cardBgHex, colors.Green)
			lblHex.Color = colors.DarkMatter
			descHex.Color = colors.DarkMatter
		case 2:
			animateCardPulse(cardBgImport, colors.Green)
			lblImport.Color = colors.DarkMatter
			descImport.Color = colors.DarkMatter
		}

		cardBgWords.Refresh()
		cardBgHex.Refresh()
		cardBgImport.Refresh()
		lblWords.Refresh()
		lblHex.Refresh()
		lblImport.Refresh()
		descWords.Refresh()
		descHex.Refresh()
		descImport.Refresh()
	}

	// Handler function for recovery type change (defined later after form is created)
	var onRecoveryTypeChanged func(int)

	// Create tappable card buttons
	btnCardWords := widget.NewButton("", func() {
		if selectedRecoveryType != 0 {
			selectedRecoveryType = 0
			updateRecoveryTypeCards()
			updateRecoveryButtonState()
			if onRecoveryTypeChanged != nil {
				onRecoveryTypeChanged(0)
			}
		}
	})
	btnCardWords.Importance = widget.LowImportance

	btnCardHex := widget.NewButton("", func() {
		if selectedRecoveryType != 1 {
			selectedRecoveryType = 1
			updateRecoveryTypeCards()
			updateRecoveryButtonState()
			if onRecoveryTypeChanged != nil {
				onRecoveryTypeChanged(1)
			}
		}
	})
	btnCardHex.Importance = widget.LowImportance

	btnCardImport := widget.NewButton("", func() {
		if selectedRecoveryType != 2 {
			selectedRecoveryType = 2
			updateRecoveryTypeCards()
			updateRecoveryButtonState()
			if onRecoveryTypeChanged != nil {
				onRecoveryTypeChanged(2)
			}
		}
	})
	btnCardImport.Importance = widget.LowImportance

	// Small spacer for card internal padding
	cardSpacer := func() fyne.CanvasObject {
		r := canvas.NewRectangle(color.Transparent)
		r.SetMinSize(fyne.NewSize(1, 4))
		return r
	}

	// Create card containers with background, icon, label, and invisible button overlay
	cardWords := container.NewStack(
		cardBgWords,
		container.NewPadded(
			container.NewVBox(
				cardSpacer(),
				container.NewCenter(iconWords),
				cardSpacer(),
				container.NewCenter(lblWords),
				container.NewCenter(descWords),
			),
		),
		btnCardWords,
	)

	cardHex := container.NewStack(
		cardBgHex,
		container.NewPadded(
			container.NewVBox(
				cardSpacer(),
				container.NewCenter(iconHex),
				cardSpacer(),
				container.NewCenter(lblHex),
				container.NewCenter(descHex),
			),
		),
		btnCardHex,
	)

	cardImport := container.NewStack(
		cardBgImport,
		container.NewPadded(
			container.NewVBox(
				cardSpacer(),
				container.NewCenter(iconImport),
				cardSpacer(),
				container.NewCenter(lblImport),
				container.NewCenter(descImport),
			),
		),
		btnCardImport,
	)

	// Spacer between cards
	cardGap := canvas.NewRectangle(color.Transparent)
	cardGap.SetMinSize(fyne.NewSize(8, 1))
	cardGap2 := canvas.NewRectangle(color.Transparent)
	cardGap2.SetMinSize(fyne.NewSize(8, 1))

	recoveryTypeControl := container.NewHBox(
		layout.NewSpacer(),
		cardWords,
		cardGap,
		cardHex,
		cardGap2,
		cardImport,
		layout.NewSpacer(),
	)

	wAccount := NewMobileEntry()
	wAccount.OnFocusGained = func() {
		scrollToFieldOnMobile(wAccount, scrollBox)
	}

	wLanguage := widget.NewSelect(mnemonics.Language_List(), nil)
	wLanguage.OnChanged = func(s string) {
		index := wLanguage.SelectedIndex()
		session.Language = index
		safeCanvasFocus(wAccount)
		clearFormText(errorText)
	}
	wLanguage.PlaceHolder = "(Select Language)"
	wLanguage.Hide()

	wAccount.SetPlaceHolder("Account Name")
	wAccount.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	wAccount.Validator = func(s string) (err error) {
		session.Error = ""
		clearFormText(errorText)

		if len(s) > MaxAccountNameLength {
			err = errors.New("account name is too long")
			wAccount.SetText(session.Name)
			wAccount.Refresh()
			showFormError(errorText, err.Error())
			return
		}

		// Update session name immediately for form validation
		session.Name = s

		updateRecoveryButtonState()

		if s != "" {
			btnCardWords.Disable()
			btnCardHex.Disable()
			btnCardImport.Disable()
		} else {
			btnCardWords.Enable()
			btnCardHex.Enable()
			btnCardImport.Enable()
		}

		// Debounce filesystem operations to avoid excessive I/O on each keystroke
		accountDebouncer.Debounce(func() {
			if s == "" {
				return
			}

			err = checkDir()
			if err != nil {
				fyne.Do(func() {
					session.LastDomain = session.Window.Content()
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutAlert(2))
				})
				return
			}

			switch cachedNetwork {
			case NETWORK_TESTNET:
				session.Path = filepath.Join(AppPath(), "testnet") + string(filepath.Separator) + s + ".db"
			case NETWORK_SIMULATOR:
				session.Path = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator) + s + ".db"
			default:
				session.Path = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator) + s + ".db"
			}

			if findAccount() {
				fyne.Do(func() {
					showFormError(errorText, "account name already exists")
					btnCreate.Disable()
					btnCreate.Refresh()
				})
			}
		})

		return nil
	}

	// Enter key handlers for form submission
	wPassword.OnSubmitted = func(s string) {
		safeCanvasFocus(wPasswordConfirm)
	}
	wPasswordConfirm.OnSubmitted = func(s string) {
		if !btnCreate.Disabled() {
			btnCreate.OnTapped()
		} else if session.Name == "" {
			safeCanvasFocus(wAccount)
		}
	}
	wAccount.OnSubmitted = func(s string) {
		if !btnCreate.Disabled() {
			btnCreate.OnTapped()
		} else {
			safeCanvasFocus(wPassword)
		}
	}

	heading := canvas.NewText("Recover Account", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	// Network indicator
	networkName := strings.ToUpper(cachedNetwork)
	networkColor := colors.Green
	if cachedNetwork != NETWORK_MAINNET {
		networkColor = colors.Yellow
	}
	networkIndicator := canvas.NewText(networkName, networkColor)
	networkIndicator.TextSize = scaleFont(12)
	networkIndicator.Alignment = fyne.TextAlignCenter

	heading2 := canvas.NewText("Success", colors.Green)
	heading2.TextSize = scaleFont(22)
	heading2.Alignment = fyne.TextAlignCenter
	heading2.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	status.Connection.FillColor = colors.Gray
	status.RemoteAccess.FillColor = colors.Gray
	status.Gnomon.FillColor = colors.Gray
	status.Sync.FillColor = colors.Gray

	grid := container.NewVBox()
	grid.Objects = nil

	seedEntry := NewMobileEntry()
	seedEntry.SetPlaceHolder("Recovery Phrase (25 words)")
	seedEntry.MultiLine = true
	seedEntry.Wrapping = fyne.TextWrapWord
	seedEntry.SetMinRowsVisible(6)
	seedEntry.Password = false
	seedEntry.OnFocusGained = func() {
		scrollToFieldOnMobile(seedEntry, scrollBox)
	}

	btnToggleSeed := widget.NewButtonWithIcon("", theme.VisibilityIcon(), nil)
	btnToggleSeed.OnTapped = func() {
		seedEntry.Password = !seedEntry.Password
		if seedEntry.Password {
			btnToggleSeed.SetIcon(theme.VisibilityOffIcon())
		} else {
			btnToggleSeed.SetIcon(theme.VisibilityIcon())
		}
		seedEntry.Refresh()
	}

	btnPasteSeed := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), nil)
	btnPasteSeed.OnTapped = func() {
		clipboardText := a.Clipboard().Content()
		if clipboardText != "" {
			seedEntry.SetText(clipboardText)
		}
	}

	seedInfo := canvas.NewText(" ", colors.Gray)
	seedInfo.TextSize = scaleFont(11)
	seedInfo.Alignment = fyne.TextAlignCenter

	// Show word count as user types
	seedEntry.OnChanged = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			seedInfo.Text = " "
			seedInfo.Color = colors.Gray
		} else {
			wordCount := len(strings.Fields(s))
			seedInfo.Text = fmt.Sprintf("%d/25 words", wordCount)
			if wordCount == 24 || wordCount == 25 {
				seedInfo.Color = colors.Green
			} else if wordCount > 25 {
				seedInfo.Color = colors.Red
			} else {
				seedInfo.Color = colors.Gray
			}
		}
		seedInfo.Refresh()
		seedEntry.Validate()
	}

	seedEntry.Validator = func(s string) (err error) {
		clearFormText(errorText)

		s = strings.TrimSpace(s)
		if s == "" {
			seedValid = false
			updateRecoveryButtonState()
			return nil
		}

		words := strings.Fields(s)
		wordCount := len(words)

		if wordCount != SeedWordCount24 && wordCount != SeedWordCount25 {
			seedValid = false
			updateRecoveryButtonState()
			return nil // OnChanged handles the count display
		}

		invalidWords := []string{}
		for _, word := range words {
			if !checkSeedWord(word) {
				invalidWords = append(invalidWords, word)
			}
		}

		if len(invalidWords) > 0 {
			seedValid = false
			updateRecoveryButtonState()
			err = errors.New("invalid seed words detected")
			if len(invalidWords) <= 3 {
				showFormError(errorText, fmt.Sprintf("Invalid: %s", strings.Join(invalidWords, ", ")))
			} else {
				showFormError(errorText, fmt.Sprintf("%d invalid words", len(invalidWords)))
			}
			return err
		}

		seedValid = true
		updateRecoveryButtonState()
		return nil
	}

	hexEntry := NewMobileEntry()
	hexEntry.SetPlaceHolder("Secret Key (64 character hex)")
	hexEntry.MultiLine = true
	hexEntry.Wrapping = fyne.TextWrapWord
	hexEntry.SetMinRowsVisible(2)
	hexEntry.Password = true
	hexEntry.OnFocusGained = func() {
		scrollToFieldOnMobile(hexEntry, scrollBox)
	}
	hexEntry.Validator = func(s string) (err error) {
		clearFormText(errorText)

		if s == "" {
			hexValid = false
			updateRecoveryButtonState()
			return nil
		}

		_, err = hex.DecodeString(s)
		if err != nil {
			showFormError(errorText, "invalid hex characters")
			hexValid = false
			updateRecoveryButtonState()
			return err
		}

		if len(s) != HexKeyLength {
			showFormError(errorText, fmt.Sprintf("key must be exactly %d characters (%d entered)", HexKeyLength, len(s)))
			hexValid = false
			updateRecoveryButtonState()
			return errors.New("invalid key length")
		}

		hexValid = true
		updateRecoveryButtonState()
		return nil
	}

	btnToggleHex := widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), nil)
	btnToggleHex.OnTapped = func() {
		hexEntry.Password = !hexEntry.Password
		if hexEntry.Password {
			btnToggleHex.SetIcon(theme.VisibilityOffIcon())
		} else {
			btnToggleHex.SetIcon(theme.VisibilityIcon())
		}
		hexEntry.Refresh()
	}

	btnPasteHex := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), nil)
	btnPasteHex.OnTapped = func() {
		clipboardText := a.Clipboard().Content()
		if clipboardText != "" {
			hexEntry.SetText(strings.TrimSpace(clipboardText))
		}
	}

	hexSpacer := canvas.NewRectangle(color.Transparent)
	hexSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	hexForm := container.NewVBox(
		hexEntry,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnToggleHex,
			btnPasteHex,
			layout.NewSpacer(),
		),
		hexSpacer,
		errorText,
	)

	seedForm := container.NewVBox(
		seedEntry,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnToggleSeed,
			btnPasteSeed,
			layout.NewSpacer(),
		),
		seedInfo,
		rectSpacer,
		errorText,
	)

	// Create a new form for account/password inputs
	recoveryForm := container.NewVBox(
		wLanguage,
		rectSpacer,
		wAccount,
		wPassword,
		strengthText,
		wPasswordConfirm,
		rectSpacer,
		seedForm,
		wrapMobileButton(btnCreate),
	)

	importFileText := canvas.NewText(" ", colors.Green)
	importFileText.TextSize = scaleFont(12)
	importFileText.Alignment = fyne.TextAlignCenter

	// Button to open file picker for import - OnTapped set after formSuccess is defined
	btnSelectFile := widget.NewButtonWithIcon("Select Wallet File", theme.FolderOpenIcon(), nil)

	importFileForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		wrapMobileButton(btnSelectFile),
		rectSpacer,
		rectSpacer,
		importFileText,
		rectSpacer,
		errorText,
		rectSpacer,
	)

	form := container.NewHBox(
		layout.NewSpacer(),
		container.NewVBox(
			recoveryTypeControl,
			rectSpacer,
			recoveryForm,
		),
		layout.NewSpacer(),
	)

	onRecoveryTypeChanged = func(typeIndex int) {
		clearFormText(errorText)

		switch typeIndex {
		case 1: // Hex Key
			wLanguage.Hide()
			form.Objects[1].(*fyne.Container).Objects[2] = recoveryForm
			recoveryForm.Objects[7] = hexForm
			safeCanvasFocus(hexEntry)
		case 0: // Recovery Words
			wLanguage.Hide()
			form.Objects[1].(*fyne.Container).Objects[2] = recoveryForm
			recoveryForm.Objects[7] = seedForm
			safeCanvasFocus(seedEntry)
		case 2: // Import File
			btnCreate.Disable()
			clearFormText(importFileText)
			clearFormText(errorText)
			form.Objects[1].(*fyne.Container).Objects[2] = importFileForm
		}

		recoveryForm.Refresh()
		form.Refresh()
	}

	body := widget.NewLabel("Your account has been successfully recovered.")
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	// QR code placeholder for success screen
	successQR := canvas.NewImageFromImage(nil)
	successQR.SetMinSize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))

	// Address text for success screen
	successAddress := widget.NewLabel("")
	successAddress.Wrapping = fyne.TextWrapBreak
	successAddress.Alignment = fyne.TextAlignCenter
	successAddress.TextStyle = fyne.TextStyle{Monospace: true}

	btnEnter := widget.NewButtonWithIcon("Enter", theme.NavigateNextIcon(), func() {
		fyne.Do(func() {
			// Call login() to properly initialize wallet (network, daemon, gnomon, etc.)
			// login() checks if engram.Disk is nil and skips wallet opening if already open
			login()
			// Password will be cleared by login() after successful initialization
		})
	})

	formSuccess := container.NewVBox(
		rectSpacer,
		heading2,
		rectSpacer,
		body,
		rectSpacer,
		container.NewCenter(successQR),
		rectSpacer,
		successAddress,
		rectSpacer,
		container.NewCenter(grid),
		rectSpacer,
		wrapMobileButton(btnEnter),
		rectSpacer,
		wrapMobileButton(btnCopyAddress),
		rectSpacer,
	)

	formSuccess.Hide()

	btnSelectFile.OnTapped = func() {
		a.Settings().SetTheme(themes.alt)
		dialogFileImport := dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
			if err != nil {
				logger.Errorf("[Engram] File dialog: %s\n", err)
				showFormError(errorText, "could not import wallet file")
				return
			}

			if uri == nil {
				return // Canceled
			}

			fileName := uri.URI().String()
			if uri.URI().MimeType() != "text/plain" {
				logger.Errorf("[Engram] Cannot import file %s\n", fileName)
				showFormError(errorText, "cannot import file")
				return
			}

			if a.Driver().Device().IsMobile() {
				fileName = uri.URI().Name()
			} else {
				fileName = filepath.Base(strings.Replace(fileName, "file://", "", -1))
			}

			if !strings.HasSuffix(fileName, ".db") {
				logger.Errorf("[Engram] Engram requires .db wallet file\n")
				showFormError(errorText, "invalid wallet file (must be .db)")
				return
			}

			filedata, err := readFromURI(uri)
			if err != nil {
				logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", fileName, err)
				showFormError(errorText, "cannot read file data")
				return
			}

			// Ensure directories exist before import
			if err := checkDir(); err != nil {
				logger.Errorf("[Engram] Creating directories for import: %s\n", err)
				showFormError(errorText, "error importing wallet file")
				return
			}
			filePath := ""
			switch cachedNetwork {
			case NETWORK_TESTNET:
				filePath = filepath.Join(AppPath(), "testnet", fileName)
			case NETWORK_SIMULATOR:
				filePath = filepath.Join(AppPath(), "testnet_simulator", fileName)
			default:
				filePath = filepath.Join(AppPath(), "mainnet", fileName)
			}

			if _, err = os.Stat(filePath); !os.IsNotExist(err) {
				logger.Errorf("[Engram] Wallet file %q already exists\n", fileName)
				showFormError(errorText, "wallet file already exists")
				return
			}

			err = os.WriteFile(filePath, filedata, 0600)
			if err != nil {
				logger.Errorf("[Engram] Importing file %s: %s\n", fileName, err)
				showFormError(errorText, "error importing wallet file")
				return
			}

			// Close any previously open wallet before importing
			if engram.Disk != nil {
				closeWallet()
			}

			// Show password overlay to unlock the imported wallet
			overlay := session.Window.Canvas().Overlays()

			rectPassSpacer := canvas.NewRectangle(color.Transparent)
			rectPassSpacer.SetMinSize(fyne.NewSize(10, 5))

			passHeader := canvas.NewText("ENTER WALLET PASSWORD", colors.Gray)
			passHeader.TextSize = scaleFont(14)
			passHeader.Alignment = fyne.TextAlignCenter
			passHeader.TextStyle = fyne.TextStyle{Bold: true}

			passSubHeader := canvas.NewText("Unlock Imported Wallet", colors.Account)
			passSubHeader.TextSize = scaleFont(22)
			passSubHeader.Alignment = fyne.TextAlignCenter
			passSubHeader.TextStyle = fyne.TextStyle{Bold: true}

			btnUnlock := widget.NewButton("Unlock", nil)
			btnUnlock.Disable()

			entryWalletPass := NewReturnEntry()
			entryWalletPass.Password = true
			entryWalletPass.PlaceHolder = "Password"
			entryWalletPass.OnFocusGained = func() {
				showVirtualKeyboard(entryWalletPass)
			}
			entryWalletPass.OnChanged = func(s string) {
				if s == "" {
					btnUnlock.Text = "Unlock"
					btnUnlock.Disable()
					btnUnlock.Refresh()
				} else {
					btnUnlock.Text = "Unlock"
					btnUnlock.Enable()
					btnUnlock.Refresh()
				}
			}

			btnBackImport := newSizedIconButton(theme.NavigateBackIcon(), func() {
				if overlay.Top() != nil {
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
				}
				if overlay.Top() != nil {
					overlay.Remove(overlay.Top())
				}
			})

			btnUnlock.OnTapped = func() {
				btnUnlock.Disable()
				btnUnlock.Text = "Unlocking..."
				btnUnlock.Refresh()

				enteredPassword := entryWalletPass.Text

				temp, err := walletapi.Open_Encrypted_Wallet(filePath, enteredPassword)
				if err != nil {
					logger.Errorf("[Engram] Cannot open imported wallet: %s\n", err)
					btnUnlock.Text = "Invalid Password..."
					btnUnlock.Refresh()
					entryWalletPass.SetText("")
					return
				}

				engram.Disk = temp
				session.Password = ""

				if cachedNetwork == NETWORK_MAINNET || cachedNetwork == NETWORK_SIMULATOR {
					engram.Disk.SetNetwork(true)
				} else {
					engram.Disk.SetNetwork(false)
				}

				engram.Disk.Get_Balance_Rescan()
				if err := engram.Disk.Save_Wallet(); err != nil {
					logger.Errorf("[Import] Failed to save wallet after import: %s\n", err)
				}

				session.WalletOpen = true
				beginWalletSession()

				// Reset exit flag so Gnomon can start in this session
				globals.Exit_In_Progress = false

				// Delete Gnomon database to ensure clean sync state after import
				gnomonPath := filepath.Join(AppPath(), "datashards", "gnomon")
				switch session.Network {
				case NETWORK_TESTNET:
					gnomonPath = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
				case NETWORK_SIMULATOR:
					gnomonPath = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
				}
				os.RemoveAll(gnomonPath)

				initSettings()

				// Generate QR code for success screen
				address := engram.Disk.GetAddress().String()
				qr, err := qrcode.New(address, qrcode.Highest)
				if err == nil {
					qrSize := ui.Width * 0.45
					if qrSize > ui.Height*0.3 {
						qrSize = ui.Height * 0.3
					}
					qr.BackgroundColor = colors.DarkMatter
					qr.ForegroundColor = colors.Green
					successQR.Image = qr.Image(int(qrSize))
					successQR.SetMinSize(fyne.NewSize(qrSize, qrSize))
					successQR.Refresh()
				}

				successAddress.SetText(address)

				btnCopyAddress.OnTapped = func() {
					a.Clipboard().SetContent(address)
				}

				// Dismiss password overlay
				if overlay.Top() != nil {
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
				}
				if overlay.Top() != nil {
					overlay.Remove(overlay.Top())
				}

				// Hide the import form and show the success form with Enter button
				btnCreate.Hide()
				form.Hide()
				form.Refresh()
				formSuccess.Show()
				formSuccess.Refresh()
				btnEnter.Refresh()
				grid.Refresh()
				scrollBox.ScrollToTop()
				scrollBox.Refresh()
				session.Window.Canvas().Content().Refresh()
				session.Window.Canvas().Refresh(session.Window.Content())
			}

			entryWalletPass.OnReturn = btnUnlock.OnTapped

			passSpan := canvas.NewRectangle(color.Transparent)
			passSpan.SetMinSize(fyne.NewSize(ui.Width, 10))

			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)

			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewCenter(
						container.NewVBox(
							passSpan,
							container.NewCenter(passHeader),
							rectPassSpacer,
							rectPassSpacer,
							passSubHeader,
							widget.NewLabel(""),
							entryWalletPass,
							rectPassSpacer,
							rectPassSpacer,
							wrapMobileButton(btnUnlock),
							rectPassSpacer,
							rectPassSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								btnBackImport,
								layout.NewSpacer(),
							),
							rectPassSpacer,
							rectPassSpacer,
						),
					),
				),
			)

			safeCanvasFocus(entryWalletPass)
			showVirtualKeyboard(entryWalletPass)

		}, session.Window)

		if !a.Driver().Device().IsMobile() {
			uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
			if err == nil {
				dialogFileImport.SetLocation(uri)
			}
		}

		dialogFileImport.SetFilter(storage.NewExtensionFileFilter([]string{".db"}))
		dialogFileImport.SetView(dialog.ListView)
		dialogFileImport.Resize(fyne.NewSize(ui.Width, ui.Height))
		dialogFileImport.Show()
	}

	scrollBox = container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				form,
				formSuccess,
				func() fyne.CanvasObject {
					if isMobile() {
						return NewSpacer(0, ui.Height*0.4)
					}
					return layout.NewSpacer()
				}(),
			),
			layout.NewSpacer(),
		),
	)

	scrollBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.85))
	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	btnCreate.OnTapped = func() {
		if engram.Disk != nil {
			closeWallet()
		}

		var err error

		if findAccount() {
			showFormError(errorText, "account name already exists")
			return
		}
		clearFormText(errorText)

		var language string
		var temp *walletapi.Wallet_Disk

		if selectedRecoveryType == 1 {
			// Hex key recovery
			if wAccount.Text == "" {
				showFormError(errorText, "enter account name")
				return
			}

			if wPassword.Text == "" {
				showFormError(errorText, "enter and confirm a password")
				return
			}

			if session.Password != session.PasswordConfirm {
				showFormError(errorText, "passwords do not match")
				return
			}

			if hexEntry.Text == "" {
				showFormError(errorText, "enter a valid hex key")
				return
			}

			if len(hexEntry.Text) != HexKeyLength {
				showFormError(errorText, fmt.Sprintf("key must be exactly %d characters", HexKeyLength))
				return
			}

			hexKey, err := hex.DecodeString(hexEntry.Text)
			if err != nil {
				showFormError(errorText, "invalid hex characters")
				return
			}

			temp, err = walletapi.Create_Encrypted_Wallet(session.Path, session.Password, new(crypto.BNRed).SetBytes(hexKey))
			if err != nil {
				showFormError(errorText, err.Error())
				return
			}

			// Default to English for hex key recovery
			language = "English"

		} else {
			// Seed phrase recovery
			if wAccount.Text == "" {
				showFormError(errorText, "enter account name")
				return
			}

			if wPassword.Text == "" {
				showFormError(errorText, "enter and confirm a password")
				return
			}

			if session.Password != session.PasswordConfirm {
				showFormError(errorText, "passwords do not match")
				return
			}

			words := strings.TrimSpace(seedEntry.Text)

			// Auto-detect language from seed words
			language, _, err = mnemonics.Words_To_Key(words)
			if err != nil {
				showFormError(errorText, err.Error())
				return
			}

			temp, err = walletapi.Create_Encrypted_Wallet_From_Recovery_Words(session.Path, session.Password, words)
			if err != nil {
				showFormError(errorText, err.Error())
				return
			}
		}

		engram.Disk = temp

		if cachedNetwork == NETWORK_MAINNET || cachedNetwork == NETWORK_SIMULATOR {
			engram.Disk.SetNetwork(true)
		} else {
			engram.Disk.SetNetwork(false)
		}

		engram.Disk.SetSeedLanguage(language)

		address := engram.Disk.GetAddress().String()

		// Generate QR code for success screen
		qr, err := qrcode.New(address, qrcode.Highest)
		if err == nil {
			qrSize := ui.Width * 0.45
			if qrSize > ui.Height*0.3 {
				qrSize = ui.Height * 0.3
			}
			qr.BackgroundColor = colors.DarkMatter
			qr.ForegroundColor = colors.Green
			successQR.Image = qr.Image(int(qrSize))
			successQR.SetMinSize(fyne.NewSize(qrSize, qrSize))
			successQR.Refresh()
		}

		// Show address text
		successAddress.SetText(address)

		btnCopyAddress.OnTapped = func() {
			a.Clipboard().SetContent(address)
		}

		engram.Disk.Get_Balance_Rescan()
		if err := engram.Disk.Save_Wallet(); err != nil {
			logger.Errorf("[Register] Failed to save wallet after creation: %s\n", err)
		}

		// Wallet remains open for immediate transition via "Enter" button
		session.WalletOpen = true
		beginWalletSession()

		// Reset exit flag so Gnomon can start in this session
		globals.Exit_In_Progress = false

		// Delete Gnomon database to ensure clean sync state after recovery
		gnomonPath := filepath.Join(AppPath(), "datashards", "gnomon")
		switch session.Network {
		case NETWORK_TESTNET:
			gnomonPath = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
		case NETWORK_SIMULATOR:
			gnomonPath = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
		}
		os.RemoveAll(gnomonPath)

		// FIX: Initialize settings after recovery so daemon/gnomon connections work
		initSettings()

		tx = Transfers{}

		// Clear sensitive password data from memory and UI
		wPassword.SetText("")
		wPasswordConfirm.SetText("")
		seedEntry.SetText("")
		hexEntry.SetText("")
		// Password kept for login() call via Enter button
		session.PasswordConfirm = ""

		btnCreate.Hide()
		form.Hide()
		form.Refresh()
		formSuccess.Show()
		formSuccess.Refresh()
		btnEnter.Refresh()
		grid.Refresh()
		scrollBox.ScrollToTop()
		scrollBox.Refresh()
		session.Window.Canvas().Content().Refresh()
		session.Window.Canvas().Refresh(session.Window.Content())
	}

	header := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
		networkIndicator,
		rectSpacer,
	)

	rect1 := canvas.NewRectangle(color.Transparent)
	rect1.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewBorder(
		header,
		footer,
		nil,
		nil,
		scrollBox,
	)
	return layout
}

func layoutAssetExplorer() fyne.CanvasObject {
	session.Domain = "app.explorer"

	frame := &iframe{}

	heading := canvas.NewText("A S S E T    E X P L O R E R", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSend())
		removeOverlays()
	})

	content := createAssetExplorerTabContent()

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			content,
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
		),
	)

	return NewVScroll(layout)
}

func layoutMyAssets() fyne.CanvasObject {
	var data []string
	var listData binding.StringList
	var listBox *widget.List

	frame := &iframe{}
	rectLeft := canvas.NewRectangle(color.Transparent)
	rectLeft.SetMinSize(fyne.NewSize(ui.Width*0.40, 35))
	rectRight := canvas.NewRectangle(color.Transparent)
	rectRight.SetMinSize(fyne.NewSize(ui.Width*0.59, 35))
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.56))
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth, 10))

	heading := canvas.NewText("M Y    A S S E T S", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	results := canvas.NewText("", colors.Green)
	results.TextSize = scaleFont(13)

	labelLastScan := canvas.NewText("", colors.Green)
	labelLastScan.TextSize = scaleFont(13)

	listData = binding.BindStringList(&data)
	listBox = widget.NewListWithData(listData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectLeft,
						widget.NewLabel(""),
					),
					container.NewStack(
						rectRight,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, ";;;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
			//co.(*fyne.Container).Objects[3].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[3])
		})

	entrySCID := widget.NewEntry()
	entrySCID.PlaceHolder = "Search by SCID"
	entrySCID.SetIcon(theme.SearchIcon())

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAssetExplorer())
		removeOverlays()
	})

	btnRescan := widget.NewButton("Rescan Blockchain", nil)
	btnRescan.Disable()

	layoutAssets := container.NewStack(
		rectWidth,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				rectSpacer,
				container.NewHBox(
					results,
					layout.NewSpacer(),
					labelLastScan,
				),
				rectSpacer,
				rectSpacer,
				container.NewStack(
					rectList,
					listBox,
				),
				rectSpacer,
				rectSpacer,
				wrapMobileButton(btnRescan),
			),
			layout.NewSpacer(),
		),
	)

	listing := layoutAssets

	var assetData []string
	assetCount := 0
	assetTotal := 0
	owned := 0

	owned = 0
	assetData = nil
	listData.Set(nil)

	if session.Offline {
		results.Text = "  Asset tracking is disabled in offline mode."
		results.Color = colors.Gray
		results.Refresh()
	} else if gnomon.Index == nil {
		results.Text = "  Asset tracking is disabled. Gnomon is inactive."
		results.Color = colors.Gray
		results.Refresh()
	}

	go func() {
		if engram.Disk != nil && gnomon.Index != nil {
			if gnomon.Index.LastIndexedHeight < int64(engram.Disk.Get_Daemon_Height()) {
				fyne.Do(func() {
					btnRescan.Disable()
				})
			} else {
				fyne.Do(func() {
					btnRescan.Enable()
				})
			}

			results.Text = "  Gathering an index of smart contracts... "
			results.Color = colors.Yellow
			fyne.Do(func() {
				results.Refresh()
			})

			for gnomon.Index.LastIndexedHeight < int64(engram.Disk.Get_Daemon_Height()) {
				results.Text = fmt.Sprintf("  Gnomon is syncing... [%d / %d]", gnomon.Index.LastIndexedHeight, int64(engram.Disk.Get_Daemon_Height()))
				results.Color = colors.Yellow

				fyne.Do(func() {
					results.Refresh()
				})

				time.Sleep(time.Second * 1)
			}

			results.Text = "  Loading previous scan results..."
			results.Color = colors.Yellow

			fyne.Do(func() {
				results.Refresh()
			})

			var assetList map[string]string
			var zerobal uint64

			shard, err := GetShard()
			if err != nil {
				return
			}

			store, err := graviton.NewDiskStore(shard)
			if err != nil {
				return
			}

			ss, err := store.LoadSnapshot(0)

			if err != nil {
				return
			}

			tree, err := ss.GetTree("My Assets")
			if err != nil {
				return
			}

			c := tree.Cursor()

			for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
				scid := string(k)

				hash := crypto.HashHexToHash(scid)

				bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(hash, -1, engram.Disk.GetAddress().String())
				if err != nil {
					return
				} else {
					title, desc, _, _, _ := getContractHeader(hash)

					if title == "" {
						title = scid
					}

					if len(title) > 18 {
						title = title[0:18] + "..."
					}

					if desc == "" {
						desc = "N/A"
					}

					if len(desc) > 40 {
						desc = desc[0:40] + "..."
					}

					balance := globals.FormatMoney(bal)
					assetData = append(data, balance+";;;"+title+";;;"+desc+";;;;;;"+scid)
					listData.Set(assetData)
					owned += 1
				}
			}

			rescan := func() {
				fyne.Do(func() {
					btnRescan.Disable()
				})
				assetTotal = 0
				assetCount = 0

				t := time.Now()
				timeNow := string(t.Format(time.RFC822))
				StoreEncryptedValue("Asset Scan", []byte("Last Scan"), []byte(timeNow))

				results.Text = "  Indexing..."
				results.Color = colors.Yellow

				fyne.Do(func() {
					results.Refresh()
				})

				owned = 0

				assetData = []string{}
				listBox.UnselectAll()
				listData.Set(assetData)

				if gnomon.Index != nil {
					switch gnomon.Index.DBType {
					case "gravdb":
						assetList = gnomon.Index.GravDBBackend.GetAllOwnersAndSCIDs()
					case "boltdb":
						assetList = gnomon.Index.BBSBackend.GetAllOwnersAndSCIDs()
					}

					for len(assetList) < 5 {
						logger.Printf("[Gnomon] Asset Scan Status: [%d / %d / %d]\n", gnomon.Index.LastIndexedHeight, engram.Disk.Get_Daemon_Height(), len(assetList))
						results.Color = colors.Yellow
						switch gnomon.Index.DBType {
						case "gravdb":
							assetList = gnomon.Index.GravDBBackend.GetAllOwnersAndSCIDs()
						case "boltdb":
							assetList = gnomon.Index.BBSBackend.GetAllOwnersAndSCIDs()
						}
						time.Sleep(time.Second * 5)
					}
				}

				results.Text = "  Scanning results..."
				results.Color = colors.Yellow

				fyne.Do(func() {
					results.Refresh()
				})

				if gnomon.Index != nil {
					switch gnomon.Index.DBType {
					case "gravdb":
						assetList = gnomon.Index.GravDBBackend.GetAllOwnersAndSCIDs()
					case "boltdb":
						assetList = gnomon.Index.BBSBackend.GetAllOwnersAndSCIDs()
					}
				}

				contracts := []crypto.Hash{}

				for sc := range assetList {
					scid := crypto.HashHexToHash(sc)

					if !scid.IsZero() {
						assetCount += 1
						contracts = append(contracts, scid)
					}
				}

				wg := sync.WaitGroup{}
				maxWorkers := 50
				lastJob := 0

			parse:

				if lastJob+maxWorkers > len(contracts) {
					maxWorkers = assetCount - lastJob
				}

				wg.Add(maxWorkers)

				// Parse each smart contract ID and check for a balance
				for i := 0; i < maxWorkers; i++ {
					index := lastJob
					go func(i int) {
						defer wg.Done()

						scid := contracts[index]

						desc := ""
						title := ""

						assetTotal += 1

						results.Text = "  Scanning... " + fmt.Sprintf("%d / %d", assetTotal, assetCount)
						results.Color = colors.Yellow

						fyne.Do(func() {
							results.Refresh()
						})

						bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(scid, -1, engram.Disk.GetAddress().String())
						if err != nil {
							return
						} else {
							balance := globals.FormatMoney(bal)

							if bal != zerobal {
								err = StoreEncryptedValue("My Assets", []byte(scid.String()), []byte(balance))
								if err != nil {
									logger.Errorf("[History] Failed to store asset: %s\n", err)
								}

								title, desc, _, _, _ = getContractHeader(scid)

								if title == "" {
									title = scid.String()
								}

								if len(title) > 20 {
									title = title[0:20] + "..."
								}

								if desc == "" {
									desc = "N/A"
								}

								if len(desc) > 40 {
									desc = desc[0:40] + "..."
								}

								owned += 1
								assetData = append(assetData, balance+";;;"+title+";;;"+desc+";;;;;;"+scid.String())
								listData.Set(assetData)
								logger.Printf("[Assets] Found asset: %s\n", scid.String())
							}
						}
					}(i)

					lastJob += 1
				}

				wg.Wait()

				if lastJob < len(contracts) {
					goto parse
				}

				results.Text = fmt.Sprintf("  Owned Assets:  %d", owned)
				results.Color = colors.Green

				labelLastScan.Text = fmt.Sprintf("  %s", timeNow)
				labelLastScan.Color = colors.Green

				fyne.Do(func() {
					listData.Set(assetData)
					btnRescan.Enable()

					results.Refresh()
					labelLastScan.Refresh()
				})
			}

			btnRescan.OnTapped = func() {
				go rescan()
			}

			lastScan, _ := GetEncryptedValue("Asset Scan", []byte("Last Scan"))

			if len(assetData) == 0 && len(lastScan) == 0 {
				rescan()
			}

			if len(lastScan) > 0 {
				results.Text = fmt.Sprintf("  Owned Assets:  %d", owned)
				labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
			} else {
				results.Text = fmt.Sprintf("  Owned Assets:  %d", owned)
				labelLastScan.Text = ""
			}

			results.Color = colors.Green

			uiDo(func() {
				results.Refresh()
				labelLastScan.Refresh()
				_ = listData.Set(assetData)
			})

			listBox.OnSelected = func(id widget.ListItemID) {
				split := strings.Split(assetData[id], ";;;")

				/*
					overlay := session.Window.Canvas().Overlays()
					overlay.Add(
						container.NewStack(
							&iframe{},
							canvas.NewRectangle(colors.DarkMatter),
						),
					)
					overlay.Add(
						container.NewStack(
							&iframe{},
							layoutAssetManager(split[4]),
						),
					)
					overlay.Top().Show()
					listBox.UnselectAll()
				*/

				uiDo(func() {
					listBox.UnselectAll()
					session.LastDomain = session.Window.Content()
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutAssetManager(split[4]))
				})
			}

			uiDo(func() {
				listBox.Refresh()
				btnRescan.Enable()
			})
		}
	}()

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			listing,
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
		),
	)

	return NewVScroll(layout)
}

func layoutAssetManager(scid string) fyne.CanvasObject {
	captureDomain := session.Domain
	session.Domain = "app.manager"

	wSpacer := widget.NewLabel(" ")

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	heading := canvas.NewText("Asset Manager", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelSigner := canvas.NewText("SMART  CONTRACT  AUTHOR", colors.Gray)
	labelSigner.TextSize = scaleFont(14)
	labelSigner.Alignment = fyne.TextAlignCenter
	labelSigner.TextStyle = fyne.TextStyle{Bold: true}

	labelOwner := canvas.NewText("SMART  CONTRACT  OWNER", colors.Gray)
	labelOwner.TextSize = scaleFont(14)
	labelOwner.Alignment = fyne.TextAlignCenter
	labelOwner.TextStyle = fyne.TextStyle{Bold: true}

	labelSCID := canvas.NewText("SMART  CONTRACT  ID", colors.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignCenter
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	labelBalance := canvas.NewText("ASSET  BALANCE", colors.Gray)
	labelBalance.TextSize = scaleFont(14)
	labelBalance.Alignment = fyne.TextAlignCenter
	labelBalance.TextStyle = fyne.TextStyle{Bold: true}

	labelTransfer := canvas.NewText("TRANSFER  ASSET", colors.Gray)
	labelTransfer.TextSize = scaleFont(14)
	labelTransfer.Alignment = fyne.TextAlignCenter
	labelTransfer.TextStyle = fyne.TextStyle{Bold: true}

	labelExecute := canvas.NewText("EXECUTE  ACTION", colors.Gray)
	labelExecute.TextSize = scaleFont(14)
	labelExecute.Alignment = fyne.TextAlignCenter
	labelExecute.TextStyle = fyne.TextStyle{Bold: true}

	var ringsize uint64
	var err error

	options := []string{"Anonymity Set:   2  (None)", "Anonymity Set:   4  (Low)", "Anonymity Set:   8  (Low)", "Anonymity Set:   16  (Recommended)", "Anonymity Set:   32  (Medium)", "Anonymity Set:   64  (High)", "Anonymity Set:   128  (High)"}

	selectRingSize := widget.NewSelect(options, nil)
	selectRingSize.OnChanged = func(s string) {
		regex := regexp.MustCompile("[0-9]+")
		result := regex.FindAllString(selectRingSize.Selected, -1)
		ringsize, err = strconv.ParseUint(result[0], 10, 64)
		if err != nil {
			ringsize = 2
		}
	}

	selectRingSize.SetSelectedIndex(3)

	entryAddress := widget.NewEntry()
	entryAddress.PlaceHolder = "Username or Address"

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	sc := widget.NewLabel(scid)
	sc.Wrapping = fyne.TextWrap(fyne.TextWrapWord)

	hash := crypto.HashHexToHash(scid)
	name, desc, icon, owner, code := getContractHeader(hash)

	if owner == "" {
		owner = "--"
	}

	signer := "--"

	result, err := getTxData(scid)
	if err != nil {
		signer = "--"
	} else {
		signer = result.Txs[0].Signer
	}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")

	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")

	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")

	labelSeparator5 := widget.NewRichTextFromMarkdown("")
	labelSeparator5.Wrapping = fyne.TextWrapOff
	labelSeparator5.ParseMarkdown("---")

	labelSeparator6 := widget.NewRichTextFromMarkdown("")
	labelSeparator6.Wrapping = fyne.TextWrapOff
	labelSeparator6.ParseMarkdown("---")

	if name == "" {
		name = "--"
	}

	labelName := widget.NewRichText(&widget.TextSegment{
		Text: name,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		}})
	labelName.Wrapping = fyne.TextWrapWord

	labelDesc := widget.NewRichText(&widget.TextSegment{
		Text: desc,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			TextStyle: fyne.TextStyle{Bold: false},
		}})
	labelDesc.Wrapping = fyne.TextWrapWord

	textSigner := widget.NewRichTextFromMarkdown(owner)
	textSigner.Wrapping = fyne.TextWrapWord
	textSigner.ParseMarkdown(signer)

	textOwner := widget.NewRichTextFromMarkdown(owner)
	textOwner.Wrapping = fyne.TextWrapWord
	textOwner.ParseMarkdown(owner)

	btnSend := widget.NewButton("Send Asset", nil)

	entryAddress.Validator = func(s string) error {
		btnSend.Text = "Send Asset"
		btnSend.Refresh()
		_, err := globals.ParseValidateAddress(s)
		if err != nil {
			go func() {
				exists, err := checkUsername(s, -1)
				if err != nil && exists == "" {
					uiDo(func() {
						btnSend.Disable()
						entryAddress.SetValidationError(errors.New("invalid username or address"))
					})
				} else {
					uiDo(func() {
						entryAddress.SetValidationError(nil)
						btnSend.Enable()
					})
				}
			}()
		} else {
			entryAddress.SetValidationError(nil)
			btnSend.Enable()
		}
		return nil
	}

	entryAmount := widget.NewEntry()
	entryAmount.PlaceHolder = "Asset Amount (Numbers Only)"
	entryAmount.Validator = func(s string) error {
		if s != "" {
			amount, err := globals.ParseAmount(s)
			if err != nil {
				btnSend.Disable()
				entryAmount.SetValidationError(errors.New("invalid amount entered"))
				return err
			} else {
				bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(hash, -1, engram.Disk.GetAddress().String())
				if err != nil {
					btnSend.Disable()
					entryAmount.SetValidationError(errors.New("error parsing asset balance"))
					return err
				} else {
					if amount > bal || amount == 0 {
						err = errors.New("insufficient asset balance")
						btnSend.Text = "Insufficient transfer amount..."
						btnSend.Disable()
						entryAmount.SetValidationError(err)
						return err
					}
				}
			}
		}

		btnSend.Text = "Send Asset"
		btnSend.Enable()
		entryAmount.SetValidationError(nil)

		return nil
	}

	var zerobal uint64

	balance := canvas.NewText(fmt.Sprintf("  %d", zerobal), colors.Green)
	balance.TextSize = scaleFont(20)
	balance.TextStyle = fyne.TextStyle{Bold: true}

	btnSend.OnTapped = func() {
		btnSend.Text = "Setting up transfer..."
		btnSend.Disable()
		btnSend.Refresh()
		entryAddress.Disable()
		entryAmount.Disable()
		selectRingSize.Disable()

		txid, err := transferAsset(hash, ringsize, entryAddress.Text, entryAmount.Text)
		if err != nil {
			entryAddress.Text = ""
			entryAddress.Refresh()
			entryAmount.Text = ""
			entryAmount.Refresh()
			btnSend.Text = "Transaction Failed..."
			btnSend.Disable()
			btnSend.Refresh()
		} else {
			entryAddress.Text = ""
			entryAddress.Refresh()
			entryAmount.Text = ""
			entryAmount.Refresh()
			btnSend.Text = "Confirming..."
			btnSend.Disable()
			btnSend.Refresh()

			go func() {
				walletapi.WaitNewHeightBlock()
				sHeight := walletapi.Get_Daemon_Height()

				for session.Domain == "app.manager" {
					if !safeWalletOpen() {
						return
					}

					var zeroscid crypto.Hash
					_, result := engram.Disk.Get_Payments_TXID(zeroscid, txid.String())

					if result.TXID != txid.String() {
						time.Sleep(time.Second * 1)
					} else {
						break
					}
				}

				// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
				if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
					uiDo(func() {
						entryAddress.Text = ""
						entryAddress.Refresh()
						entryAmount.Text = ""
						entryAmount.Refresh()
						btnSend.Text = "Transaction Failed..."
						btnSend.Disable()
						btnSend.Refresh()
					})

					return
				}

				// If daemon height has incremented, print retry counters into button space
				if walletapi.Get_Daemon_Height()-sHeight > 0 {
					uiDo(func() {
						btnSend.Text = fmt.Sprintf("Confirming... (%d/%d)", walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
						btnSend.Refresh()
					})
				}

				bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(hash, -1, engram.Disk.GetAddress().String())
				if err == nil {
					err = StoreEncryptedValue("My Assets", []byte(hash.String()), []byte(globals.FormatMoney(bal)))
					if err != nil {
						logger.Errorf("[Asset] Error storing new asset balance for: %s\n", hash)
					}
					balance.Text = "  " + globals.FormatMoney(bal)

					uiDo(func() {
						balance.Refresh()
					})
				}

				if bal != zerobal {
					uiDo(func() {
						btnSend.Text = "Send Asset"
						btnSend.Enable()
						btnSend.Refresh()
						entryAddress.Text = ""
						entryAddress.Enable()
						entryAddress.Refresh()
						entryAmount.Text = ""
						entryAmount.Enable()
						entryAmount.Refresh()
						selectRingSize.Enable()
					})
				} else {
					uiDo(func() {
						btnSend.Text = "You do not own this asset"
						btnSend.Disable()
						btnSend.Refresh()
					})
				}
			}()
		}
	}

	bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(hash, -1, engram.Disk.GetAddress().String())
	if err == nil {
		balance.Text = "  " + globals.FormatMoney(bal)
		balance.Refresh()

		if bal == zerobal {
			entryAddress.Disable()
			entryAmount.Disable()
			selectRingSize.Disable()
			btnSend.Text = "You do not own this asset"
			btnSend.Disable()
		}
	}

	if captureDomain == "app.manager" { // was already on manager and opened it again so go back option is to explorer
		captureDomain = "app.explorer"
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if captureDomain == "app.explorer" {
			session.Window.SetContent(layoutAssetExplorer())
		} else {
			session.Window.SetContent(session.LastDomain)
			session.Domain = captureDomain
		}
		session.LastDomain = capture
	})

	image := canvas.NewImageFromResource(resourceBlankPng)
	image.SetMinSize(fyne.NewSize(ui.Width*0.3, ui.Width*0.3))
	image.FillMode = canvas.ImageFillContain

	if icon != "" {
		var path fyne.Resource
		path, err = fyne.LoadResourceFromURLString(icon)
		if err != nil {
			image.Resource = resourceBlankPng
		} else {
			image.Resource = path
		}

		image.SetMinSize(fyne.NewSize(ui.Width*0.3, ui.Width*0.3))
		image.FillMode = canvas.ImageFillContain
		image.Refresh()
	}

	if name == "" {
		labelName.ParseMarkdown("## --")
	}

	if desc == "" {
		labelDesc = widget.NewRichText(&widget.TextSegment{
			Text: "No description provided",
			Style: widget.RichTextStyle{
				Alignment: fyne.TextAlignCenter,
				ColorName: theme.ColorNameForeground,
				TextStyle: fyne.TextStyle{Italic: true},
			}})
		labelDesc.Wrapping = fyne.TextWrapWord
	}

	if bal != zerobal {
		btnSend.Text = "Send Asset"
		btnSend.Enable()
	} else {
		btnSend.Text = "You do not own this asset"
		btnSend.Disable()
	}
	btnSend.Refresh()

	linkCopySigner := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopySigner.OnTapped = func() {
		a.Clipboard().SetContent(signer)
	}

	linkCopyOwner := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopyOwner.OnTapped = func() {
		a.Clipboard().SetContent(owner)
	}

	linkMessageAuthor := widget.NewHyperlinkWithStyle("Message the Author", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkMessageAuthor.OnTapped = func() {
		if signer != "" && signer != "--" {
			messages.Contact = signer
			session.PreviousDomain = session.Domain
			session.LastDomain = session.Window.Content()
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	}

	linkMessageOwner := widget.NewHyperlinkWithStyle("Message the Owner", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkMessageOwner.OnTapped = func() {
		if owner != "" && owner != "--" {
			messages.Contact = owner
			session.PreviousDomain = session.Domain
			session.LastDomain = session.Window.Content()
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	}

	linkCopySCID := widget.NewHyperlinkWithStyle("Copy SCID", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopySCID.OnTapped = func() {
		a.Clipboard().SetContent(scid)
	}

	linkView := widget.NewHyperlinkWithStyle("View in Explorer", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkView.OnTapped = func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + scid)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + scid)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	}

	// Now let's parse the smart contract code for exported functions

	var contract dvm.SmartContract
	var signerFunctions []string
	var deroFunctions []string
	var assetFunctions []string

	contract, _, err = dvm.ParseSmartContract(strings.ReplaceAll(code, "\x00", ""))
	if err != nil {
		contract = dvm.SmartContract{}
	}

	data := []string{}

	for f := range contract.Functions {
		r, _ := utf8.DecodeRuneInString(contract.Functions[f].Name)

		if !unicode.IsUpper(r) {
			logger.Debugf("[DVM] Function %s is not an exported function - skipping it\n", contract.Functions[f].Name)
		} else if contract.Functions[f].Name == "Initialize" || contract.Functions[f].Name == "InitializePrivate" {
			logger.Debugf("[DVM] Function %s is an initialization function - skipping it\n", contract.Functions[f].Name)
		} else {
			data = append(data, contract.Functions[f].Name)
		}

		for l := range contract.Functions[f].Lines {
			for i := range contract.Functions[f].Lines[l] {
				if contract.Functions[f].Lines[l][i] == "SIGNER" && contract.Functions[f].Lines[l][i+1] == "(" {
					signerFunctions = append(signerFunctions, contract.Functions[f].Name)
				}

				if contract.Functions[f].Lines[l][i] == "DEROVALUE" && contract.Functions[f].Lines[l][i+1] == "(" {
					deroFunctions = append(deroFunctions, contract.Functions[f].Name)
				}

				if contract.Functions[f].Lines[l][i] == "ASSETVALUE" && contract.Functions[f].Lines[l][i+1] == "(" {
					assetFunctions = append(assetFunctions, contract.Functions[f].Name)
				}
			}
		}
	}

	sort.Strings(data)
	data = append(data, " ")

	var paramList []fyne.Widget
	var dero_amount uint64
	var asset_amount uint64

	functionList := widget.NewSelect(data, nil)
	functionList.OnChanged = func(s string) {
		if s == " " {
			functionList.ClearSelected()
			return
		}

		var params []dvm.Variable

		overlay := session.Window.Canvas().Overlays()

		options := []string{"Anonymity Set:   2  (None)", "Anonymity Set:   4  (Low)", "Anonymity Set:   8  (Low)", "Anonymity Set:   16  (Recommended)", "Anonymity Set:   32  (Medium)", "Anonymity Set:   64  (High)", "Anonymity Set:   128  (High)"}

		var ringsize uint64

		signerRequired := false

		selectRingMembers := widget.NewSelect(options, nil)
		selectRingMembers.PlaceHolder = "(Select Anonymity Set)"

		for f := range contract.Functions {
			if contract.Functions[f].Name == s {
				params = contract.Functions[f].Params

				header := canvas.NewText("EXECUTE  CONTRACT  FUNCTION", colors.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				funcName := canvas.NewText(s, colors.Account)
				funcName.TextSize = scaleFont(22)
				funcName.Alignment = fyne.TextAlignCenter
				funcName.TextStyle = fyne.TextStyle{Bold: true}

				linkClose := widget.NewHyperlinkWithStyle("Close", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
				linkClose.OnTapped = func() {
					dero_amount = 0
					asset_amount = 0
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
					overlay.Remove(overlay.Top())
				}

				span := canvas.NewRectangle(color.Transparent)
				span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)

				entryDEROValue := widget.NewEntry()
				entryDEROValue.PlaceHolder = "DERO Amount (Numbers Only)"
				entryDEROValue.Validator = func(s string) error {
					dero_amount, err = globals.ParseAmount(s)
					if err != nil {
						entryDEROValue.SetValidationError(err)
						return err
					}

					return nil
				}

				entryAssetValue := widget.NewEntry()
				entryAssetValue.PlaceHolder = "Asset Amount (Numbers Only)"
				entryAssetValue.Validator = func(s string) error {
					asset_amount, err = globals.ParseAmount(s)
					if err != nil {
						entryAssetValue.SetValidationError(err)
						return err
					}

					return nil
				}

				a := container.NewStack(
					span,
					entryAssetValue,
				)

				d := container.NewStack(
					span,
					entryDEROValue,
				)

				paramsContainer := container.NewVBox()

				existsDEROValue := false
				existsAssetValue := false

				// Scan code for ASSETVALUE and DEROVALUE
				for l := range contract.Functions[f].Lines {
					for i := range contract.Functions[f].Lines[l] {

						for v := range paramList {
							if paramList[v] == entryDEROValue {
								existsDEROValue = true
							} else if paramList[v] == entryAssetValue {
								existsAssetValue = true
							}
						}

						if contract.Functions[f].Lines[l][i] == "DEROVALUE" && contract.Functions[f].Lines[l][i+1] == "(" && !existsDEROValue {
							paramList = append(paramList, entryDEROValue)
							paramsContainer.Add(d)
							paramsContainer.Refresh()
							existsDEROValue = true
							logger.Debugf("[DVM] Added DEROVALUE: %s\n", contract.Functions[f].Lines[l][i])
						} else if len(deroFunctions) > 0 {
							for df := range deroFunctions {
								if contract.Functions[f].Lines[l][i] == deroFunctions[df] && contract.Functions[f].Lines[l][i+1] == "(" && !existsDEROValue {
									paramList = append(paramList, entryDEROValue)
									paramsContainer.Add(d)
									paramsContainer.Refresh()
									existsDEROValue = true
									logger.Debugf("[DVM] Added DEROVALUE: %s - Func: %s\n", contract.Functions[f].Lines[l][i], deroFunctions[df])
								}
							}
						}

						if contract.Functions[f].Lines[l][i] == "ASSETVALUE" && contract.Functions[f].Lines[l][i+1] == "(" && !existsAssetValue {
							paramList = append(paramList, entryAssetValue)
							paramsContainer.Add(a)
							paramsContainer.Refresh()
							existsAssetValue = true
							logger.Debugf("[DVM] Added ASSETVALUE: %s\n", contract.Functions[f].Lines[l][i])
						} else if len(assetFunctions) > 0 {
							for af := range assetFunctions {
								if contract.Functions[f].Lines[l][i] == assetFunctions[af] && contract.Functions[f].Lines[l][i+1] == "(" && !existsAssetValue {
									paramList = append(paramList, entryAssetValue)
									paramsContainer.Add(a)
									paramsContainer.Refresh()
									existsAssetValue = true
									logger.Debugf("[DVM] Added ASSETVALUE: %s\n", contract.Functions[f].Lines[l][i])
								}
							}
						}

						for si := range signerFunctions {
							if contract.Functions[f].Lines[l][i] == "SIGNER" && contract.Functions[f].Lines[l][i+1] == "(" {
								signerRequired = true
							} else if contract.Functions[f].Lines[l][i] == signerFunctions[si] && contract.Functions[f].Lines[l][i+1] == "(" {
								signerRequired = true
							}
						}
					}
				}

				selectRingMembers.OnChanged = func(s string) {
					if signerRequired {
						ringsize = 2
					} else {
						regex := regexp.MustCompile("[0-9]+")
						result := regex.FindAllString(selectRingMembers.Selected, -1)
						ringsize, err = strconv.ParseUint(result[0], 10, 64)
						if err != nil {
							ringsize = 2
						}
					}
				}

				if signerRequired {
					selectRingMembers.SetSelectedIndex(0)
				} else {
					selectRingMembers.SetSelectedIndex(3)
				}

				btnExecuteBase := widget.NewButton("Execute", nil)
				var btnExecuteObj fyne.CanvasObject = btnExecuteBase
				if isMobile() {
					btnExecuteBase.Importance = widget.MediumImportance
					sizeEnforcer := canvas.NewRectangle(color.Transparent)
					sizeEnforcer.SetMinSize(scalePoint(100, 48))
					btnExecuteObj = container.NewStack(sizeEnforcer, btnExecuteBase)
				}

				overlay.Add(
					container.NewStack(
						&iframe{},
						container.NewCenter(
							container.NewVBox(
								span,
								container.NewCenter(
									header,
								),
								rectSpacer,
								rectSpacer,
								container.NewCenter(
									funcName,
								),
								wSpacer,
								selectRingMembers,
								rectSpacer,
								rectSpacer,
								paramsContainer,
								rectSpacer,
								rectSpacer,
								btnExecuteObj,
								rectSpacer,
								rectSpacer,
								container.NewHBox(
									layout.NewSpacer(),
									linkClose,
									layout.NewSpacer(),
								),
								rectSpacer,
								rectSpacer,
							),
						),
					),
				)

				for p := range params {
					entry := widget.NewEntry()
					entry.PlaceHolder = params[p].Name
					if params[p].Type == 0x4 {
						entry.PlaceHolder = params[p].Name + " (Numbers Only)"
					}
					entry.Validator = func(s string) error {
						for p := range params {
							if params[p].Type == 0x5 {
								if params[p].Name == entry.PlaceHolder {
									logger.Debugf("[%s] String: %s\n", params[p].Name, s)
									params[p].ValueString = s
								}
							} else if params[p].Type == 0x4 {
								if params[p].Name+" (Numbers Only)" == entry.PlaceHolder {
									amount, err := globals.ParseAmount(s)
									if err != nil {
										logger.Debugf("[%s] Param error: %s\n", params[p].Name, err)
										entry.SetValidationError(err)
										return err
									} else {
										logger.Debugf("[%s] Amount: %d\n", params[p].Name, amount)
										params[p].ValueUint64 = amount
									}
								}
							}
						}

						return nil
					}

					c := container.NewStack(
						span,
						entry,
					)

					paramList = append(paramList, entry)
					paramsContainer.Add(c)
					paramsContainer.Refresh()

				}

				btnExecuteBase.OnTapped = func() {
					for f := range contract.Functions {
						if contract.Functions[f].Name == funcName.Text {
							params = contract.Functions[f].Params
						}
					}

					var err error

					if signerRequired {
						ringsize = 2
					} else {
						regex := regexp.MustCompile("[0-9]+")
						result := regex.FindAllString(selectRingMembers.Selected, -1)
						ringsize, err = strconv.ParseUint(result[0], 10, 64)
						if err != nil {
							ringsize = 2
							selectRingMembers.SetSelected(options[3])
						}
					}

					logger.Printf("[Engram] Ringsize: %d\n", ringsize)

					btnExecuteBase.Text = "Executing..."
					btnExecuteBase.Disable()
					btnExecuteBase.Refresh()

					storage, err := executeContractFunction(hash, ringsize, dero_amount, asset_amount, funcName.Text, params)
					if err != nil {
						if strings.Contains(err.Error(), "somehow the tx could not be built") {
							btnExecuteBase.Text = fmt.Sprintf("Insufficient Balance: Need %v", globals.FormatMoney(storage))
						} else if strings.Contains(err.Error(), "Discarded knowingly") {
							btnExecuteBase.Text = "Error: Check wallet registration, daemon sync, and network status"
						} else if strings.Contains(err.Error(), "Recovered in function") {
							btnExecuteBase.Text = "Error... invalid input"
						} else {
							btnExecuteBase.Text = "Error executing function..."
						}
						btnExecuteBase.Disable()
						btnExecuteBase.Refresh()
					} else {
						btnExecuteBase.Text = "Function executed successfully!"
						btnExecuteBase.Disable()
						btnExecuteBase.Refresh()
					}
				}

				if signerRequired {
					selectRingMembers.SetSelectedIndex(0)
					selectRingMembers.Disable()
				}

				paramsContainer.Refresh()
				overlay.Top().Show()
				functionList.ClearSelected()
			}
		}
	}

	center := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewStack(
				rectWidth90,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						/*
							container.NewHBox(
								image,
								rectSpacer,
								container.NewVBox(
									layout.NewSpacer(),
									labelName,
									layout.NewSpacer(),
								),
								layout.NewSpacer(),
							),
						*/
						container.NewHBox(
							layout.NewSpacer(),
							image,
							layout.NewSpacer(),
						),
						rectSpacer,

						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelName,
							),
							layout.NewSpacer(),
						),
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelDesc,
							),
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelSigner,
						rectSpacer,
						textSigner,
						container.NewHBox(
							linkMessageAuthor,
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkCopySigner,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						labelOwner,
						rectSpacer,
						textOwner,
						container.NewHBox(
							linkMessageOwner,
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkCopyOwner,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelSCID,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							sc,
						),
						container.NewHBox(
							linkView,
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkCopySCID,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator6,
						rectSpacer,
						rectSpacer,
						labelExecute,
						rectSpacer,
						rectSpacer,
						functionList,
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelBalance,
						rectSpacer,
						balance,
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelTransfer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						selectRingSize,
						rectSpacer,
						entryAddress,
						rectSpacer,
						entryAmount,
						rectSpacer,
						wrapMobileButton(btnSend),
						wSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
		rectSpacer,
		rectSpacer,
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return layout
}

func layoutTransfers() fyne.CanvasObject {
	session.Domain = "app.transfers"

	sendHeading := canvas.NewText("T R A N S F E R S", colors.Gray)
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}
	sendHeading.TextSize = scaleFont(16)

	top := container.NewVBox(
		NewSpacer(0, scaleSize(10)),
		container.NewCenter(sendHeading),
		NewSpacer(0, scaleSize(10)),
	)

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))
	frame := &iframe{}
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(30)))
	rect.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.53))

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSend())
		removeOverlays()
	})

	var pendingList []string

	for i := 0; i < len(tx.Pending); i++ {
		pendingList = append(pendingList, strconv.Itoa(i)+","+globals.FormatMoney(tx.Pending[i].Amount)+","+tx.Pending[i].Destination)
	}

	data := binding.BindStringList(&pendingList)

	scrollBox := widget.NewListWithData(data,
		func() fyne.CanvasObject {
			c := container.NewStack(
				rectList,
				container.NewHBox(
					canvas.NewText("", colors.Account),
					layout.NewSpacer(),
					canvas.NewText("", colors.Account),
				),
			)
			return c
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}
			dataItem := strings.SplitN(str, ",", 3)
			dest := dataItem[2]
			dest = "   " + dest[0:4] + " ... " + dest[len(dataItem[2])-10:]
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*canvas.Text).Text = dest
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*canvas.Text).TextSize = scaleFont(17)
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*canvas.Text).TextStyle.Bold = true
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[2].(*canvas.Text).Text = dataItem[1] + "   "
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[2].(*canvas.Text).TextSize = scaleFont(17)
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[2].(*canvas.Text).TextStyle.Bold = true
		})

	scrollBox.OnSelected = func(id widget.ListItemID) {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfersDetail(id))
	}

	btnSend := widget.NewButtonWithIcon("Send Transfers", theme.UploadIcon(), nil)

	btnClear := widget.NewButtonWithIcon("Clear", theme.WindowCloseIcon(), func() {
		pendingList = pendingList[:0]
		tx = Transfers{}
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
	})

	if len(pendingList) > 0 {
		btnClear.Enable()
		btnSend.Enable()
	} else {
		btnClear.Disable()
		btnSend.Disable()
	}

	if session.Offline {
		btnSend.Text = "Disabled in Offline Mode"
		btnSend.Disable()
	}

	btnSend.OnTapped = func() {
		overlay := session.Window.Canvas().Overlays()

		header := canvas.NewText("ACCOUNT  VERIFICATION  REQUIRED", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Confirm Password", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkClose := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkClose.OnTapped = func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}

		btnSubmit := widget.NewButton("Submit", nil)

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = "Password"
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnSubmit.Text = "Submit"
				btnSubmit.Disable()
				btnSubmit.Refresh()
			} else {
				btnSubmit.Text = "Submit"
				btnSubmit.Enable()
				btnSubmit.Refresh()
			}
		}

		btnSubmit.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				removeOverlays()
				if len(tx.Pending) == 0 {
					return
				} else {
					btnSend.Text = "Setting up transfer..."
					btnSend.Disable()
					btnSend.Refresh()
					txid, err := sendTransfers()
					if err != nil {
						btnSend.Text = "Send Transfers"
						btnSend.Enable()
						btnSend.Refresh()
						return
					}

					go func() {
						generation := currentWalletGeneration()
						uiDo(func() {
							if !isWalletGenerationActive(generation) {
								return
							}
							btnClear.Disable()
							btnSend.Text = "Confirming..."
							btnSend.Refresh()
						})

						walletapi.WaitNewHeightBlock()
						sHeight := walletapi.Get_Daemon_Height()

						for session.Domain == "app.transfers" {
							if !isWalletGenerationActive(generation) {
								return
							}

							var zeroscid crypto.Hash
							_, result := engram.Disk.Get_Payments_TXID(zeroscid, txid.String())

							if result.TXID == txid.String() {
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = "Transfer Successful!"
									btnSend.Refresh()
								})

								break
							}

							// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
							if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = "Transfer failed..."
									btnSend.Disable()
									btnSend.Refresh()
								})
								break
							}

							// If daemon height has incremented, print retry counters into button space
							if walletapi.Get_Daemon_Height()-sHeight > 0 {
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = fmt.Sprintf("Confirming... (%d/%d)", walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
									btnSend.Refresh()
								})
							}

							time.Sleep(time.Second * 1)
						}
					}()

					pendingList = pendingList[:0]
					uiDo(func() {
						_ = data.Reload()
						btnSend.Disable()
						btnClear.Disable()
					})
				}
			} else {
				uiDo(func() {
					btnSubmit.Text = "Invalid Password..."
					btnSubmit.Disable()
					btnSubmit.Refresh()
				})
			}
		}

		btnSubmit.Disable()

		entryPassword.OnReturn = btnSubmit.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						NewSpacer(0, scaleSize(10)),
						NewSpacer(0, scaleSize(10)),
						subHeader,
						widget.NewLabel(""),
						container.NewCenter(
							container.NewStack(
								span,
								entryPassword,
							),
						),
						NewSpacer(0, scaleSize(10)),
						NewSpacer(0, scaleSize(10)),
						btnSubmit,
						NewSpacer(0, scaleSize(10)),
						NewSpacer(0, scaleSize(10)),
						container.NewHBox(
							layout.NewSpacer(),
							linkClose,
							layout.NewSpacer(),
						),
						NewSpacer(0, scaleSize(10)),
						NewSpacer(0, scaleSize(10)),
					),
				),
			),
		)

		safeCanvasFocus(entryPassword)
	}

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.transfers" {
			return
		}

		if k.Name == fyne.KeyDown {
			session.Dashboard = "main"
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutSend())
			removeOverlays()
		}
	})

	sendForm := container.NewVBox(
		container.NewStack(
			rectListBox,
			scrollBox,
		),
		widget.NewLabel(" "),
		wrapMobileButton(btnSend),
		NewSpacer(0, scaleSize(10)),
		wrapMobileButton(btnClear),
		NewSpacer(0, scaleSize(20)),
	)

	gridItem1 := container.NewCenter(
		sendForm,
	)

	gridItem2 := container.NewCenter()

	gridItem3 := container.NewCenter()

	gridItem4 := container.NewCenter()

	gridItem1.Hidden = false
	gridItem2.Hidden = true
	gridItem3.Hidden = true
	gridItem4.Hidden = true

	features := container.NewCenter(
		layout.NewSpacer(),
		gridItem1,
		layout.NewSpacer(),
		gridItem2,
		layout.NewSpacer(),
		gridItem3,
		layout.NewSpacer(),
		gridItem4,
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			NewSpacer(0, scaleSize(10)),
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		features,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return layout
}

func layoutTransfersDetail(index int) fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("T R A N S F E R    D E T A I L", colors.Gray)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	labelDestination := canvas.NewText("   RECEIVER  ADDRESS", colors.Gray)
	labelDestination.TextSize = scaleFont(14)
	labelDestination.Alignment = fyne.TextAlignLeading
	labelDestination.TextStyle = fyne.TextStyle{Bold: true}

	labelAmount := canvas.NewText("   AMOUNT", colors.Gray)
	labelAmount.TextSize = scaleFont(14)
	labelAmount.Alignment = fyne.TextAlignLeading
	labelAmount.TextStyle = fyne.TextStyle{Bold: true}

	labelService := canvas.NewText("   PAYMENT  REQUEST", colors.Gray)
	labelService.TextSize = scaleFont(14)
	labelService.Alignment = fyne.TextAlignLeading
	labelService.TextStyle = fyne.TextStyle{Bold: true}

	labelDestPort := canvas.NewText("   DESTINATION  PORT", colors.Gray)
	labelDestPort.TextSize = scaleFont(14)
	labelDestPort.TextStyle = fyne.TextStyle{Bold: true}

	labelSourcePort := canvas.NewText("   SOURCE  PORT", colors.Gray)
	labelSourcePort.TextSize = scaleFont(14)
	labelSourcePort.TextStyle = fyne.TextStyle{Bold: true}

	labelFees := canvas.NewText("   TRANSACTION  FEES", colors.Gray)
	labelFees.TextSize = scaleFont(14)
	labelFees.TextStyle = fyne.TextStyle{Bold: true}

	labelPayload := canvas.NewText("   PAYLOAD", colors.Gray)
	labelPayload.TextSize = scaleFont(14)
	labelPayload.TextStyle = fyne.TextStyle{Bold: true}

	labelReply := canvas.NewText("   REPLY  ADDRESS", colors.Gray)
	labelReply.TextSize = scaleFont(14)
	labelReply.TextStyle = fyne.TextStyle{Bold: true}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")

	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")

	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")

	labelSeparator5 := widget.NewRichTextFromMarkdown("")
	labelSeparator5.Wrapping = fyne.TextWrapOff
	labelSeparator5.ParseMarkdown("---")

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	details := tx.Pending[index]

	valueDestination := widget.NewRichTextFromMarkdown("--")
	valueDestination.Wrapping = fyne.TextWrapBreak

	valueType := widget.NewRichTextFromMarkdown("--")
	valueType.Wrapping = fyne.TextWrapOff

	if details.Destination != "" {
		address, _ := globals.ParseValidateAddress(details.Destination)
		if address.IsIntegratedAddress() {
			valueDestination.ParseMarkdown(address.BaseAddress().String())
			valueType.ParseMarkdown("### SERVICE")
		} else {
			valueDestination.ParseMarkdown(details.Destination)
			valueType.ParseMarkdown("### NORMAL")
		}
	}

	valueReply := widget.NewRichTextFromMarkdown("--")
	valueReply.Wrapping = fyne.TextWrapBreak

	if details.Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		if details.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string) != "" {
			valueReply.ParseMarkdown("" + details.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string))
		}
	}

	valuePayload := widget.NewRichTextFromMarkdown("--")
	valuePayload.Wrapping = fyne.TextWrapBreak

	if details.Payload_RPC.HasValue(rpc.RPC_COMMENT, rpc.DataString) {
		if details.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string) != "" {
			valuePayload.ParseMarkdown("" + details.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string))
		}
	}

	valueAmount := canvas.NewText("", colors.Account)
	valueAmount.TextSize = scaleFont(22)
	valueAmount.TextStyle = fyne.TextStyle{Bold: true}
	valueAmount.Text = "  " + globals.FormatMoney(details.Amount)

	valueDestPort := canvas.NewText("", colors.Account)
	valueDestPort.TextSize = scaleFont(22)
	valueDestPort.TextStyle = fyne.TextStyle{Bold: true}

	if details.Payload_RPC.HasValue(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
		port := fmt.Sprintf("%d", details.Payload_RPC.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64))
		valueDestPort.Text = "  " + port
	} else {
		valueDestPort.Text = "  0"
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
	})

	btnDelete := widget.NewButton("Cancel Transfer", nil)
	btnDelete.OnTapped = func() {
		if len(tx.Pending) > index+1 {
			tx.Pending = append(tx.Pending[:index], tx.Pending[index+1:]...)
		} else if len(tx.Pending) == 1 {
			tx = Transfers{}
		} else {
			tx.Pending = tx.Pending[:index]
		}

		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
	}

	top = container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		container.NewCenter(
			valueType,
		),
		rectSpacer,
		rectSpacer,
	)

	center := container.NewStack(
		container.NewVScroll(
			container.NewStack(
				rectWidth,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						rectSpacer,
						rectSpacer,
						labelDestination,
						rectSpacer,
						valueDestination,
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelAmount,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueAmount,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						labelReply,
						rectSpacer,
						valueReply,
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelPayload,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valuePayload,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelDestPort,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueDestPort,
						),
						wSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewStack(
				rectWidth,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewStack(
						rectWidth90,
						btnDelete,
					),
					layout.NewSpacer(),
				),
			),
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

func layoutTransition() fyne.CanvasObject {
	frame := &iframe{}
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	res.transitionMu.Lock()
	defer res.transitionMu.Unlock()

	if res.cachedTransition == nil {
		rect := canvas.NewRectangle(color.Transparent)
		rect.SetMinSize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))

		if res.loading == nil {
			res.loading, _ = x.NewAnimatedGifFromResource(resourceLoadingGif)
		}
		if res.loading != nil {
			res.loading.SetMinSize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))
			res.loading.Resize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))
		}

		res.cachedTransition = container.NewStack(
			frame,
			container.NewCenter(
				rect,
				res.loading,
			),
		)
	}

	if res.loading != nil {
		res.loading.Start()
	}

	return NewVScroll(res.cachedTransition)
}

func layoutSettings() fyne.CanvasObject {
	stopGnomon()
	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	heading := canvas.NewText("Settings", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelNetwork := canvas.NewText("NETWORK", colors.Gray)
	labelNetwork.TextStyle = fyne.TextStyle{Bold: true}
	labelNetwork.TextSize = scaleFont(14)

	labelNode := canvas.NewText("CONNECTION", colors.Gray)
	labelNode.TextStyle = fyne.TextStyle{Bold: true}
	labelNode.TextSize = scaleFont(14)

	labelSecurity := canvas.NewText("SECURITY", colors.Gray)
	labelSecurity.TextStyle = fyne.TextStyle{Bold: true}
	labelSecurity.TextSize = scaleFont(14)

	textRemoteAccess := widget.NewRichTextWithText("A username and password is required in order to allow application connectivity.")
	textRemoteAccess.Wrapping = fyne.TextWrapWord

	btnRestore := widget.NewButton("Restore Defaults", nil)
	btnDelete := widget.NewButton("Clear Local Data", nil)

	type NodeItem struct {
		Address string
		Status  string
	}

	mainnetNodes := []NodeItem{
		{Address: "node.derofoundation.org:11012", Status: "unknown"},
		{Address: "community-pools.mysrv.cloud:10102", Status: "unknown"},
		{Address: "127.0.0.1:10102", Status: "unknown"},
	}
	testnetNodes := []NodeItem{
		{Address: "69.30.234.163:40402", Status: "unknown"},
		{Address: "testnet.derofoundation.co:40402", Status: "unknown"},
		{Address: "127.0.0.1:40402", Status: "unknown"},
	}
	simulatorNodes := []NodeItem{
		{Address: "127.0.0.1:20000", Status: "unknown"},
	}

	getNodesKey := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "testnet_nodes"
		case NETWORK_SIMULATOR:
			return "simulator_nodes"
		default:
			return "mainnet_nodes"
		}
	}

	getDefaultNodes := func(network string) []NodeItem {
		switch network {
		case NETWORK_TESTNET:
			return testnetNodes
		case NETWORK_SIMULATOR:
			return simulatorNodes
		default:
			return mainnetNodes
		}
	}

	loadNodesForNetwork := func(network string) []NodeItem {
		nodesKey := getNodesKey(network)
		if data, err := GetValue("settings", []byte(nodesKey)); err == nil && len(data) > 0 {
			var savedNodes []NodeItem
			if err := json.Unmarshal(data, &savedNodes); err == nil && len(savedNodes) > 0 {
				return savedNodes
			}
		}
		return getDefaultNodes(network)
	}

	currentNetwork := getNetwork()
	nodeData := loadNodesForNetwork(currentNetwork)

	nodeContainer := container.NewVBox()

	var updateNodeContainer func()

	updateNodeContainer = func() {
		nodeContainer.Objects = nil

		for i := range nodeData {
			i := i // capture loop variable
			item := &nodeData[i]

			var iconResource fyne.Resource
			switch item.Status {
			case "connected":
				iconResource = theme.ConfirmIcon()
			case "failed":
				iconResource = theme.CancelIcon()
			}

			rowIcon := widget.NewIcon(iconResource)

			removeBtn := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
				if len(nodeData) <= 1 {
					return
				}

				removedAddress := item.Address
				wasConnected := item.Status == "connected" || getDaemon() == removedAddress

				nodeData = append(nodeData[:i], nodeData[i+1:]...)

				if wasConnected {
					newIndex := i - 1
					if newIndex < 0 {
						newIndex = 0
					}
					if newIndex >= len(nodeData) {
						newIndex = len(nodeData) - 1
					}
					nodeData[newIndex].Status = "connected"
					setDaemon(nodeData[newIndex].Address)

					for j := range nodeData {
						if j != newIndex {
							nodeData[j].Status = "unknown"
						}
					}
				}

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				updateNodeContainer()
			})
			removeBtn.Importance = widget.MediumImportance
			if len(nodeData) <= 1 {
				removeBtn.Disable()
			}

			addressLabel := widget.NewLabel(item.Address)
			addressLabel.Truncation = fyne.TextTruncateEllipsis

			row := container.NewBorder(
				nil, nil, nil,
				container.NewHBox(rowIcon, wrapMobileButton(removeBtn)),
				addressLabel,
			)

			tapBtn := widget.NewButton("", func() {
				if testNodeConnection(item.Address) {
					item.Status = "connected"
					setDaemon(item.Address)

					for j := range nodeData {
						if j != i {
							nodeData[j].Status = "unknown"
						}
					}

					if data, err := json.Marshal(nodeData); err == nil {
						StoreValue("settings", []byte(getNodesKey(session.Network)), data)
					}
				} else {
					item.Status = "failed"
				}
				updateNodeContainer()
			})
			tapBtn.Importance = widget.LowImportance
			tapBtn.Alignment = widget.ButtonAlignLeading
			tapBtn.Text = ""

			clickableRow := container.NewMax(
				wrapMobileButton(tapBtn),
				row,
			)

			nodeContainer.Add(clickableRow)
		}
		nodeContainer.Refresh()
	}

	currentDaemon := getDaemon()
	for i := range nodeData {
		if nodeData[i].Address == currentDaemon {
			nodeData[i].Status = "connected"
		}
	}
	updateNodeContainer()

	getNodePlaceholder := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "hostname:40402"
		case NETWORK_SIMULATOR:
			return "hostname:20000"
		default:
			return "hostname:10102"
		}
	}

	entryCustomNode := widget.NewEntry()
	entryCustomNode.PlaceHolder = getNodePlaceholder(currentNetwork)

	showNodeError := func(err error) {
		entryCustomNode.Validator = func(s string) error {
			return err
		}
		entryCustomNode.SetValidationError(err)
		entryCustomNode.FocusGained()
		entryCustomNode.FocusLost()
	}

	clearNodeError := func() {
		entryCustomNode.Validator = nil
		entryCustomNode.SetValidationError(nil)
		entryCustomNode.Refresh()
	}

	btnAddNode := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		nodeAddress := strings.TrimSpace(entryCustomNode.Text)
		nodeAddress = strings.ReplaceAll(nodeAddress, " ", "")
		if nodeAddress != "" {
			// Check for duplicate
			for _, node := range nodeData {
				if node.Address == nodeAddress {
					showNodeError(errors.New("node already exists"))
					return
				}
			}

			if testNodeConnectionTimeout(nodeAddress, 500*time.Millisecond) {
				clearNodeError()

				for i := range nodeData {
					nodeData[i].Status = "unknown"
				}

				nodeData = append(nodeData, NodeItem{
					Address: nodeAddress,
					Status:  "connected",
				})
				setDaemon(nodeAddress)

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				entryCustomNode.Text = ""
				entryCustomNode.Refresh()
				updateNodeContainer()
			} else {
				showNodeError(errors.New("node unreachable"))
			}
		}
	})
	btnAddNode.Importance = widget.MediumImportance
	btnAddNode.Disable()

	entryCustomNode.OnChanged = func(s string) {
		clearNodeError()
		s = strings.TrimSpace(s)
		if s != "" {
			btnAddNode.Enable()
		} else {
			btnAddNode.Disable()
		}
	}

	entrySection := container.NewBorder(nil, nil, nil, wrapMobileButton(btnAddNode), entryCustomNode)
	entryWrapper := container.NewStack(
		canvas.NewRectangle(color.Transparent),
		entrySection,
	)
	entryWrapper.Resize(fyne.NewSize(ui.Width*0.9, 35))

	labelScan := widget.NewRichTextFromMarkdown("Enter the number of past blocks that the wallet should scan:")
	labelScan.Wrapping = fyne.TextWrapWord

	entryScan := widget.NewEntry()
	entryScan.PlaceHolder = "# of Latest Blocks (Optional)"
	entryScan.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		_, err := strconv.ParseInt(s, 10, 64)
		return err
	}
	entryScan.OnChanged = func(s string) {
		if s == "" {
			session.TrackRecentBlocks = 0
			return
		}
		if blocks, err := strconv.ParseInt(s, 10, 64); err == nil && blocks > 0 {
			session.TrackRecentBlocks = blocks
		} else {
			session.TrackRecentBlocks = 0
		}
	}

	if session.TrackRecentBlocks > 0 {
		blocks := strconv.FormatInt(session.TrackRecentBlocks, 10)
		entryScan.Text = blocks
		entryScan.Refresh()
	}

	tabsNetwork := container.NewAppTabs(
		container.NewTabItem(NETWORK_MAINNET, container.NewVBox()),
		container.NewTabItem(NETWORK_TESTNET, container.NewVBox()),
		container.NewTabItem(NETWORK_SIMULATOR, container.NewVBox()),
	)

	tabsNetwork.OnSelected = func(tab *container.TabItem) {
		s := tab.Text
		if s != NETWORK_TESTNET && s != NETWORK_SIMULATOR {
			s = NETWORK_MAINNET
		}
		setNetwork(s)

		nodeData = loadNodesForNetwork(s)

		for i := range nodeData {
			if nodeData[i].Address == getDaemon() {
				nodeData[i].Status = "connected"
			} else {
				nodeData[i].Status = "unknown"
			}
		}

		globals.InitNetwork()

		entryCustomNode.PlaceHolder = getNodePlaceholder(s)
		clearNodeError()

		updateNodeContainer()
	}

	net, _ := GetValue("settings", []byte("network"))
	switch string(net) {
	case NETWORK_TESTNET:
		tabsNetwork.SelectTabIndex(1)
	case NETWORK_SIMULATOR:
		tabsNetwork.SelectTabIndex(2)
	default:
		tabsNetwork.SelectTabIndex(0)
	}

	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "Username"
	entryUser.SetText(remoteAccess.RPC.user)

	entryPass := widget.NewEntry()
	entryPass.PlaceHolder = "Password"
	entryPass.Password = true
	entryPass.SetText(remoteAccess.RPC.pass)

	entryUser.OnChanged = func(s string) {
		remoteAccess.RPC.user = s
		StoreValue("settings", []byte("rpc_user"), []byte(s))
	}

	entryPass.OnChanged = func(s string) {
		remoteAccess.RPC.pass = s
		StoreValue("settings", []byte("rpc_pass"), []byte(s))
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		initSettings()

		resizeWindow(ui.MaxWidth, ui.MaxHeight)
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	})

	btnRestore.OnTapped = func() {
		verificationOverlay(
			false,
			"SETTINGS",
			"Reset all settings to defaults?",
			"Confirm",
			func(b bool) {
				if b {
					setNetwork(NETWORK_MAINNET)
					setDaemon(DEFAULT_REMOTE_DAEMON)
					setAuthMode("true")
					setGnomon("1")

					StoreValue("settings", []byte("mainnet_nodes"), []byte{})
					StoreValue("settings", []byte("testnet_nodes"), []byte{})
					StoreValue("settings", []byte("simulator_nodes"), []byte{})

					remoteAccess.RPC.user = newRPCUsername()
					remoteAccess.RPC.pass = newRPCPassword()
					StoreValue("settings", []byte("rpc_user"), []byte(remoteAccess.RPC.user))
					StoreValue("settings", []byte("rpc_pass"), []byte(remoteAccess.RPC.pass))

					resizeWindow(ui.MaxWidth, ui.MaxHeight)
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutSettings())
					removeOverlays()
				}
			},
		)
	}

	statusText := canvas.NewText("", colors.Account)
	statusText.TextSize = scaleFont(12)

	btnDelete.OnTapped = func() {
		verificationOverlay(
			false,
			"SETTINGS",
			fmt.Sprintf("Delete all local %s data?", strings.ToLower(session.Network)),
			"Confirm",
			func(b bool) {
				if b {
					err := cleanGnomonData()
					if err != nil {
						if parseError, ok := err.(*os.PathError); !ok {
							err = fmt.Errorf("error clearing local %s data", session.Network)
						} else {
							err = parseError.Err
						}

						statusText.Color = colors.Red
						statusText.Text = err.Error()
						statusText.Refresh()
						return
					}

					statusText.Color = colors.Green
					statusText.Text = fmt.Sprintf("Gnomon %s data successfully deleted.", strings.ToLower(session.Network))
					statusText.Refresh()
				}
			},
		)
	}

	formSettings := container.NewVBox(
		labelNetwork,
		rectSpacer,
		tabsNetwork,
		widget.NewLabel(""),
		labelNode,
		rectSpacer,
		rectSpacer,
		entryWrapper,
		rectSpacer,
		nodeContainer,
		rectSpacer,
		labelScan,
		rectSpacer,
		entryScan,
		widget.NewLabel(""),
		labelSecurity,
		rectSpacer,
		textRemoteAccess,
		rectSpacer,
		entryUser,
		rectSpacer,
		entryPass,
		rectSpacer,
		statusText,
		wrapMobileButton(btnDelete),
		wrapMobileButton(btnRestore),
	)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				formSettings,
			),
			layout.NewSpacer(),
		),
	)

	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

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

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)

	return c
}

// layoutAppSettings creates the centralized settings page with 3 tabs:
// Remote Access, TELA, and Advanced
func layoutAppSettings() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	previousDomain := session.Domain // Save before overwriting

	// Track the actual caller if we aren't coming from a settings sub-page
	if previousDomain != "app.remoteaccess.manager" && previousDomain != "app.remoteaccess.permissions" {
		settingsCallerDomain = previousDomain
	}

	session.Domain = "app.appsettings"

	frame := &iframe{}

	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	heading := canvas.NewText("SETTINGS", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	// Remote Access Tab Content
	go refreshXSWDList()

	wSpacer := widget.NewLabel(" ")

	title := canvas.NewText("R E M O T E   A C C E S S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.20))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	rpcLabel := canvas.NewText("      C O N F I G U R A T I O N      ", colors.Gray)
	rpcLabel.TextSize = scaleFont(11)
	rpcLabel.Alignment = fyne.TextAlignCenter
	rpcLabel.TextStyle = fyne.TextStyle{Bold: true}

	wsLabel := canvas.NewText("      C O N F I G U R A T I O N      ", colors.Gray)
	wsLabel.TextSize = scaleFont(11)
	wsLabel.Alignment = fyne.TextAlignCenter
	wsLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelConnections := canvas.NewText("  C O N N E C T I O N S  ", colors.Gray)
	labelConnections.TextSize = scaleFont(11)
	labelConnections.Alignment = fyne.TextAlignCenter
	labelConnections.TextStyle = fyne.TextStyle{Bold: true}

	sep1 := canvas.NewRectangle(colors.Gray)
	sep1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep1,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	shortShard := canvas.NewText("APPLICATION  CONNECTIONS", colors.Gray)
	shortShard.TextStyle = fyne.TextStyle{Bold: true}
	shortShard.TextSize = scaleFont(12)

	linkColor := colors.Green

	if remoteAccess.RPC.server == nil {
		session.Link = "Blocked"
		linkColor = colors.Gray
	}

	remoteAccess.RPC.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.RPC.status.TextSize = scaleFont(22)
	remoteAccess.RPC.status.TextStyle = fyne.TextStyle{Bold: true}

	serverStatus := canvas.NewText("APPLICATION  CONNECTIONS", colors.Gray)
	serverStatus.TextSize = scaleFont(12)
	serverStatus.Alignment = fyne.TextAlignCenter
	serverStatus.TextStyle = fyne.TextStyle{Bold: true}

	linkCenter := container.NewCenter(
		remoteAccess.RPC.status,
	)

	remoteAccess.RPC.userText = widget.NewEntry()
	remoteAccess.RPC.userText.PlaceHolder = "Username"
	remoteAccess.RPC.userText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.user = s
		}
	}

	remoteAccess.RPC.passText = widget.NewEntry()
	remoteAccess.RPC.passText.Password = true
	remoteAccess.RPC.passText.PlaceHolder = "Password"
	remoteAccess.RPC.passText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.pass = s
		}
	}

	remoteAccess.RPC.portText = widget.NewEntry()
	remoteAccess.RPC.portText.PlaceHolder = "0.0.0.0:10103"
	remoteAccess.RPC.portText.Validator = func(s string) (err error) {
		regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
		test := regexp.MustCompile(regex)
		if test.MatchString(s) {
			remoteAccess.RPC.portText.SetValidationError(nil)
		} else {
			err = errors.New("invalid host name")
			remoteAccess.RPC.portText.SetValidationError(err)
		}
		return
	}
	remoteAccess.RPC.portText.SetText(getRemoteAccess("RPC"))

	linkColor = colors.Green

	if remoteAccess.WS.server == nil {
		session.Link = "Blocked"
		linkColor = colors.Gray
	}

	remoteAccess.WS.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.WS.status.TextSize = scaleFont(22)
	remoteAccess.WS.status.TextStyle = fyne.TextStyle{Bold: true}

	deckChoice := widget.NewSelect([]string{"Web Sockets (WS)", "Remote Procedure Calls (RPC)"}, nil)

	remoteAccess.RPC.toggle = widget.NewButton("Turn On", nil)
	remoteAccess.RPC.toggle.OnTapped = func() {
		switch session.Network {
		case NETWORK_TESTNET:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_TESTNET_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		case NETWORK_SIMULATOR:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_SIMULATOR_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		default:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		}

		toggleRPCServer(remoteAccess.RPC.port)
		if remoteAccess.RPC.server != nil {
			setRemoteAccess(remoteAccess.RPC.port, "RPC")
			deckChoice.Disable()
			remoteAccess.RPC.portText.Disable()
		} else {
			deckChoice.Enable()
			remoteAccess.RPC.portText.Enable()
		}
	}

	if remoteAccess.WS.portText == nil {
		remoteAccess.WS.portText = widget.NewEntry()
		remoteAccess.WS.portText.PlaceHolder = "0.0.0.0:44326"
		remoteAccess.WS.portText.Validator = func(s string) (err error) {
			regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
			test := regexp.MustCompile(regex)
			if test.MatchString(s) {
				remoteAccess.WS.portText.SetValidationError(nil)
			} else {
				err = errors.New("invalid host name")
				remoteAccess.WS.portText.SetValidationError(err)
			}
			return
		}
	}

	remoteAccess.WS.toggle = widget.NewButton("Turn On", nil)
	remoteAccess.WS.toggle.OnTapped = func() {
		if remoteAccess.WS.portText.Validate() != nil {
			remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
			remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
		} else {
			_, err := net.ResolveTCPAddr("tcp", remoteAccess.WS.port)
			if err != nil {
				logger.Errorf("[Remote Access] XSWD port: %s\n", err)
				remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
				remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
			} else {
				remoteAccess.WS.port = remoteAccess.WS.portText.Text
			}
		}

		remoteAccess.EPOCH.err = nil
		toggleXSWD(remoteAccess.WS.port)
		if remoteAccess.WS.server != nil {
			setRemoteAccessDual(remoteAccess.WS.port, "WS") // Use dual storage for consistency
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
			if remoteAccess.EPOCH.enabled {
				err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
				if err != nil {
					logger.Errorf("[EPOCH] Connecting: %s\n", err)
					remoteAccess.EPOCH.err = err
				} else {
					remoteAccess.EPOCH.err = nil
					setRemoteAccess(epoch.GetPort(), "EPOCH")
				}
			}
		} else {
			stopEPOCH()
			remoteAccess.WS.portText.Enable()
			deckChoice.Enable()
		}
	}

	if session.Offline {
		remoteAccess.RPC.toggle.Text = "Disabled in Offline Mode"
		remoteAccess.RPC.toggle.Disable()
		remoteAccess.RPC.portText.Disable()
		remoteAccess.WS.toggle.Text = "Disabled in Offline Mode"
		remoteAccess.WS.toggle.Disable()
		remoteAccess.WS.portText.Disable()
	} else {
		if remoteAccess.RPC.server != nil {
			remoteAccess.RPC.status.Text = "Allowed"
			remoteAccess.RPC.status.Color = colors.Green
			remoteAccess.RPC.toggle.Text = "Turn Off"
			remoteAccess.RPC.userText.Disable()
			remoteAccess.RPC.passText.Disable()
			remoteAccess.RPC.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.RPC.status.Text = "Blocked"
			remoteAccess.RPC.status.Color = colors.Gray
			remoteAccess.RPC.toggle.Text = "Turn On"
			remoteAccess.RPC.userText.Enable()
			remoteAccess.RPC.passText.Enable()
			remoteAccess.RPC.portText.Enable()
		}

		if remoteAccess.WS.server != nil {
			remoteAccess.WS.status.Text = "Allowed"
			remoteAccess.WS.status.Color = colors.Green
			remoteAccess.WS.toggle.Text = "Turn Off"
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.WS.status.Text = "Blocked"
			remoteAccess.WS.status.Color = colors.Gray
			remoteAccess.WS.toggle.Text = "Turn On"
			remoteAccess.WS.portText.Enable()
		}
	}

	remoteAccess.RPC.userText.SetText(remoteAccess.RPC.user)
	remoteAccess.RPC.passText.SetText(remoteAccess.RPC.pass)

	linkCopy := widget.NewHyperlinkWithStyle("Copy Credentials", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopy.OnTapped = func() {
		a.Clipboard().SetContent(remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass)
	}

	linkPermissions := widget.NewHyperlinkWithStyle("Advanced", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkPermissions.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDPermissions())
		removeOverlays()
	}

	remoteAccess.WS.list = widget.NewList(
		func() int {
			return len(remoteAccess.WS.apps)
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabel(""),
			)
		},
		func(li widget.ListItemID, co fyne.CanvasObject) {
			app := remoteAccess.WS.apps[li]
			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*widget.Label).SetText(app.Name)
			})
		},
	)

	remoteAccess.WS.list.OnSelected = func(id widget.ListItemID) {
		remoteAccess.WS.list.UnselectAll()
		remoteAccess.WS.list.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDAppManager(&remoteAccess.WS.apps[id]))
		removeOverlays()
	}

	xswdForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			wsLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					container.NewCenter(
						remoteAccess.WS.status,
					),
					rectSpacer,
					serverStatus,
					wSpacer,
					wrapMobileButton(remoteAccess.WS.toggle),
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkPermissions,
						layout.NewSpacer(),
					),
				),
			),
		),
		container.NewStack(
			rectWidth90,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					labelConnections,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rect,
						remoteAccess.WS.list,
					),
				),
			),
		),
		layout.NewSpacer(),
	)

	rpcForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			rpcLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					linkCenter,
					rectSpacer,
					serverStatus,
					wSpacer,
					wrapMobileButton(remoteAccess.RPC.toggle),
					wSpacer,
					remoteAccess.RPC.portText,
					rectSpacer,
					remoteAccess.RPC.userText,
					rectSpacer,
					remoteAccess.RPC.passText,
					wSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkCopy,
						layout.NewSpacer(),
					),
				),
			),
			layout.NewSpacer(),
		),
	)

	deckFeatures := container.NewStack()
	if remoteAccess.RPC.server != nil {
		deckFeatures.Add(rpcForm)
		deckChoice.SetSelectedIndex(1)
	} else {
		deckFeatures.Add(xswdForm)
		deckChoice.SetSelectedIndex(0)
	}

	deckChoice.OnChanged = func(s string) {
		if s == "Remote Procedure Calls (RPC)" {
			deckFeatures.Objects[0] = rpcForm
		} else {
			deckFeatures.Objects[0] = xswdForm
		}
	}

	remoteAccessContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				title,
			),
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewStack(
				rectWidth90,
				deckChoice,
			),
		),
		container.NewBorder(
			deckFeatures,
			nil,
			nil,
			nil,
		),
	)

	// TELA Tab Content
	telaTitle := canvas.NewText("T E L A", colors.Gray)
	telaTitle.TextStyle = fyne.TextStyle{Bold: true}
	telaTitle.TextSize = scaleFont(16)

	// Port Start entry
	entryPortStart := NewMobileEntry()
	entryPortStart.SetPlaceHolder(strconv.Itoa(tela.DEFAULT_PORT_START))
	// Load Port Start setting from dual storage
	if portStart, found := getTELADual("Port Start"); found {
		entryPortStart.SetText(portStart)
		logger.Printf("[Engram] TELA Port Start loaded from storage: %s", portStart)
	} else {
		logger.Printf("[Engram] TELA Port Start not found in storage, using default")
	}
	entryPortStart.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid port")
		}
		return tela.SetPortStart(i)
	}
	entryPortStart.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Port Start", []byte(s))
		}
	}

	// Min Likes entry
	entryMinLikes := NewMobileEntry()
	entryMinLikes.SetPlaceHolder("30")
	if storedMinLikes, found := getTELADual("Min Likes"); found {
		entryMinLikes.SetText(storedMinLikes)
	} else {
		entryMinLikes.SetText("30")
	}
	entryMinLikes.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid percent")
		}
		if i < 0 || i > 100 {
			return fmt.Errorf("must be 0 to 100")
		}
		return nil
	}
	entryMinLikes.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Min Likes", []byte(s))
		}
	}

	// Exclusions entry
	entryExclusions := NewMobileEntry()
	entryExclusions.SetPlaceHolder("dURL Exclusions (exclude1,exclude2)")
	if storedExclusions, found := getTELADual("Exclusions"); found {
		entryExclusions.SetText(storedExclusions)
	}
	entryExclusions.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Exclusions", []byte(s))
		} else {
			deleteTELADual("Exclusions")
		}
	}

	// Restrictive Mode checkbox
	wRestrictiveMode := widget.NewCheck("Restrictive Mode", nil)
	// Load Restrictive Mode setting from dual storage
	restrictiveModeEnabled := false // Default to OFF (unrestrictive mode)
	if restrictiveMode, found := getTELADual("Restrictive Mode"); found {
		if restrictiveMode == "false" {
			restrictiveModeEnabled = false
			logger.Printf("[Engram] TELA Restrictive Mode loaded from storage: Disabled")
		} else {
			restrictiveModeEnabled = true
			logger.Printf("[Engram] TELA Restrictive Mode loaded from storage: Enabled")
		}
	} else {
		// Also check the old "Mode" key for backward compatibility
		if storedTelaMode, found := getTELADual("Mode"); found {
			if storedTelaMode == "Unrestrictive" {
				restrictiveModeEnabled = false
				logger.Printf("[Engram] TELA Restrictive Mode loaded from legacy Mode key: Disabled")
			} else {
				restrictiveModeEnabled = true
				logger.Printf("[Engram] TELA Restrictive Mode loaded from legacy Mode key: Enabled")
			}
		} else {
			// Default to unrestricted mode
			restrictiveModeEnabled = false
			logger.Printf("[Engram] TELA Restrictive Mode using default: Disabled")
		}
	}
	wRestrictiveMode.SetChecked(restrictiveModeEnabled)
	wRestrictiveMode.OnChanged = func(b bool) {
		if b {
			// For restrictive mode, save the key
			setTELADual("Restrictive Mode", []byte("true"))
			logger.Printf("[Engram] TELA Restrictive Mode enabled (saved true)")
		} else {
			// For unrestricted mode, delete both keys since unrestrictive is the default
			deleteTELADual("Restrictive Mode")
			deleteTELADual("Mode")
			logger.Printf("[Engram] TELA Restrictive Mode disabled (deleted keys)")
		}
	}

	// Allow Content Updates dropdown
	wAllowUpdates := widget.NewSelect([]string{xswd.Deny.String(), xswd.Allow.String()}, nil)
	// Load Allow Updates setting from dual storage
	if allowUpdates, found := getTELADual("Allow Updates"); found {
		if allowUpdates == "Allow" {
			wAllowUpdates.SetSelectedIndex(1)
			logger.Printf("[Engram] TELA Allow Updates loaded from storage: Allow")
		} else {
			wAllowUpdates.SetSelectedIndex(0)
			logger.Printf("[Engram] TELA Allow Updates loaded from storage: Deny")
		}
	} else {
		// Default to Allow when no stored value exists
		wAllowUpdates.SetSelectedIndex(1)
		tela.AllowUpdates(true)
		setTELADual("Allow Updates", []byte("Allow"))
		logger.Printf("[Engram] TELA Allow Updates defaulting to: Allow")
	}
	wAllowUpdates.OnChanged = func(s string) {
		if s == xswd.Allow.String() {
			tela.AllowUpdates(true)
			setTELADual("Allow Updates", []byte("Allow"))
			logger.Printf("[Engram] TELA Allow Updates set to Allow")
		} else {
			tela.AllowUpdates(false)
			setTELADual("Allow Updates", []byte("Deny"))
			logger.Printf("[Engram] TELA Allow Updates set to Deny")
		}
	}

	// Rescan Recheck dropdown
	wRescanRecheck := widget.NewSelect([]string{"No", "Yes"}, nil)
	if storedRescanRecheck, found := getTELADual("Rescan Recheck"); found {
		if storedRescanRecheck == "Yes" {
			wRescanRecheck.SetSelectedIndex(1)
		} else {
			wRescanRecheck.SetSelectedIndex(0)
		}
	} else {
		wRescanRecheck.SetSelectedIndex(0)
	}
	wRescanRecheck.OnChanged = func(s string) {
		setTELADual("Rescan Recheck", []byte(s))
	}

	// Sort By dropdown
	sortByOptions := []string{"Ratings", "A-Z", "Z-A"}
	wSortBy := widget.NewSelect(sortByOptions, nil)
	if storedSortBy, found := getTELADual("Sort By"); found {
		wSortBy.SetSelected(storedSortBy)
	} else {
		wSortBy.SetSelected(sortByOptions[0])
	}
	wSortBy.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Sort By", []byte(s))
		}
	}

	// Reset Defaults button
	btnResetDefaults := widget.NewButton("Reset Default Settings", func() {
		wRestrictiveMode.SetChecked(false)
		wAllowUpdates.SetSelectedIndex(0)
		wRescanRecheck.SetSelectedIndex(0)
		wSortBy.SetSelectedIndex(0)
		entryPortStart.SetText(strconv.Itoa(tela.DEFAULT_PORT_START))
		entryMinLikes.SetText("30")
		entryExclusions.SetText("")
	})

	// Delete Search Data button
	btnDeleteSearchData := widget.NewButton("Delete Search Data", func() {
		verificationOverlay(
			false,
			"TELA BROWSER",
			"Delete stored search data?",
			"Confirm",
			func(b bool) {
				if b {
					DeleteKey("TELA Search", []byte("SCIDs"))
					DeleteKey("TELA Search", []byte("Searched SCIDs"))
					DeleteKey("TELA Search", []byte("Last Scan"))
					DeleteKey("TELA Search", []byte("Last Indexed Height"))
					DeleteKey("TELA Search", []byte("CandidateCache"))
					DeleteKey("TELA Search", []byte("NegativeCache"))
					DeleteKey("TELA Search", []byte("IndexCache"))
					DeleteKey("TELA Search", []byte("DisplayCache"))
				}
			},
		)
	})

	// Shutdown TELA button
	btnShutdownTela := widget.NewButton("Shutdown TELA", func() {
		verificationOverlay(
			false,
			"TELA BROWSER",
			"Shutdown all active TELA servers?",
			"Confirm",
			func(b bool) {
				if b {
					tela.ShutdownTELA()
				}
			},
		)
	})

	// Clear History button
	btnClearHistory := widget.NewButton("Clear History", func() {
		verificationOverlay(
			false,
			"TELA BROWSER",
			"Clear browsing history?",
			"Confirm",
			func(b bool) {
				if b {
					shard, err := GetShard()
					if err != nil {
						return
					}

					store, err := graviton.NewDiskStore(shard)
					if err != nil {
						return
					}

					ss, err := store.LoadSnapshot(0)
					if err != nil {
						return
					}

					tree, err := ss.GetTree("TELA History")
					if err != nil {
						return
					}

					c := tree.Cursor()

					for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
						DeleteKey(tree.GetName(), k)
					}
				}
			},
		)
	})

	telaContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				telaTitle,
			),
		),
		rectSpacer,
		wrapMobileButton(btnShutdownTela),
		rectSpacer,
		rectSpacer,
		container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### Restrictive Mode"), nil, wRestrictiveMode),
		rectSpacer,
		container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### Allow Content Updates"), wAllowUpdates, nil),
		rectSpacer,
		container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### Rescan Recheck"), wRescanRecheck, nil),
		rectSpacer,
		container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### Sort By"), wSortBy, nil),
		rectSpacer,
		widget.NewRichTextFromMarkdown("### Start Port Range"),
		entryPortStart,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### Search Min Likes %"),
		entryMinLikes,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### Search Exclusions"),
		entryExclusions,
		rectSpacer,
		rectSpacer,
		wrapMobileButton(btnResetDefaults),
		rectSpacer,
		wrapMobileButton(btnDeleteSearchData),
		rectSpacer,
		wrapMobileButton(btnClearHistory),
	)

	// Advanced Tab Content
	advancedTitle := canvas.NewText("A D V A N C E D", colors.Gray)
	advancedTitle.TextStyle = fyne.TextStyle{Bold: true}
	advancedTitle.TextSize = scaleFont(16)

	// GNOMON Section
	gnomonTitle := canvas.NewText("GNOMON", colors.Gray)
	gnomonTitle.TextSize = scaleFont(11)
	gnomonTitle.Alignment = fyne.TextAlignCenter
	gnomonTitle.TextStyle = fyne.TextStyle{Bold: true}

	gnomonDescription := widget.NewRichTextFromMarkdown("Gnomon scans and indexes blockchain data in order to unlock more features, like native asset tracking.")
	gnomonDescription.Wrapping = fyne.TextWrapWord

	checkGnomon := widget.NewCheck("Enable Gnomon", func(b bool) {
		if b {
			StoreValue("settings", []byte("gnomon"), []byte("1"))
			gnomon.Active = 1
		} else {
			StoreValue("settings", []byte("gnomon"), []byte("0"))
			gnomon.Active = 0
		}
	})

	gmn, err := GetValue("settings", []byte("gnomon"))
	if err != nil || string(gmn) == "1" {
		gnomon.Active = 1
		checkGnomon.SetChecked(true)
		if err != nil {
			StoreValue("settings", []byte("gnomon"), []byte("1"))
		}
	} else {
		gnomon.Active = 0
		checkGnomon.SetChecked(false)
	}

	// EPOCH STATISTICS Section
	epochTitle := canvas.NewText("EPOCH STATISTICS", colors.Gray)
	epochTitle.TextSize = scaleFont(11)
	epochTitle.Alignment = fyne.TextAlignCenter
	epochTitle.TextStyle = fyne.TextStyle{Bold: true}

	spacerEpoch := canvas.NewRectangle(color.Transparent)
	spacerEpoch.SetMinSize(fyne.NewSize(140, 0))

	wEpoch := widget.NewSelect([]string{"Session", "Total"}, nil)
	wEpoch.SetSelected("Session")

	epochSession, _ := epoch.GetSession(time.Second * 4)

	labelEpochHashes := widget.NewRichTextFromMarkdown("### Hashes")
	labelEpochHashes.Wrapping = fyne.TextWrapWord

	epochHashes := fmt.Sprintf("%.1fK", float64(epochSession.Hashes)/1000)
	textEpochHashes := widget.NewRichTextFromMarkdown(epochHashes)
	textEpochHashes.Wrapping = fyne.TextWrapWord

	labelEpochBlocks := widget.NewRichTextFromMarkdown("### Miniblocks")
	labelEpochBlocks.Wrapping = fyne.TextWrapWord

	epochBlocks := fmt.Sprintf("%d", epochSession.MiniBlocks)
	textEpochBlocks := widget.NewRichTextFromMarkdown(epochBlocks)
	textEpochBlocks.Wrapping = fyne.TextWrapWord

	wEpoch.OnChanged = func(s string) {
		epochSession, _ := epoch.GetSession(time.Second * 4)
		if s == "Total" {
			total := epoch.GetSessionEPOCH_Result{
				Hashes:     remoteAccess.EPOCH.total.Hashes,
				MiniBlocks: remoteAccess.EPOCH.total.MiniBlocks,
			}

			if epoch.IsActive() {
				total.Hashes += epochSession.Hashes
				total.MiniBlocks += epochSession.MiniBlocks
			}

			textEpochHashes.ParseMarkdown(epoch.HashesToString(total.Hashes))
			textEpochBlocks.ParseMarkdown(fmt.Sprintf("%d", total.MiniBlocks))

			return
		}

		textEpochHashes.ParseMarkdown(epoch.HashesToString(epochSession.Hashes))
		textEpochBlocks.ParseMarkdown(fmt.Sprintf("%d", epochSession.MiniBlocks))
	}

	// SCANNING Section
	scanningTitle := canvas.NewText("SCANNING", colors.Gray)
	scanningTitle.TextSize = scaleFont(11)
	scanningTitle.Alignment = fyne.TextAlignCenter
	scanningTitle.TextStyle = fyne.TextStyle{Bold: true}

	scanningDescription := widget.NewRichTextFromMarkdown("Enter the number of past blocks that the wallet should scan:")
	scanningDescription.Wrapping = fyne.TextWrapWord

	entryTrackBlocks := NewMobileEntry()
	entryTrackBlocks.SetPlaceHolder("# of Latest Blocks (Optional)")
	entryTrackBlocks.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		_, parseErr := strconv.ParseInt(s, 10, 64)
		return parseErr
	}
	entryTrackBlocks.OnChanged = func(s string) {
		if s == "" {
			session.TrackRecentBlocks = 0
			return
		}
		if blocks, err := strconv.ParseInt(s, 10, 64); err == nil && blocks > 0 {
			session.TrackRecentBlocks = blocks
		} else {
			session.TrackRecentBlocks = 0
		}
	}

	if session.TrackRecentBlocks > 0 {
		blocks := strconv.FormatInt(session.TrackRecentBlocks, 10)
		entryTrackBlocks.SetText(blocks)
	}

	// MAINTENANCE Section
	maintenanceTitle := canvas.NewText("MAINTENANCE", colors.Gray)
	maintenanceTitle.TextSize = scaleFont(11)
	maintenanceTitle.Alignment = fyne.TextAlignCenter
	maintenanceTitle.TextStyle = fyne.TextStyle{Bold: true}

	btnClearLocalData := widget.NewButton("Clear Local Data", func() {
		verificationOverlay(
			false,
			"ADVANCED",
			fmt.Sprintf("Delete all local %s data?", strings.ToLower(session.Network)),
			"Confirm",
			func(b bool) {
				if b {
					err := cleanGnomonData()
					if err != nil {
						if parseError, ok := err.(*os.PathError); !ok {
							err = fmt.Errorf("error clearing local %s data", session.Network)
						} else {
							err = parseError.Err
						}

						errorDialog := dialog.NewError(err, session.Window)
						errorDialog.SetOnClosed(func() {})
						errorDialog.Show()
						return
					}

					successDialog := dialog.NewInformation("Success", fmt.Sprintf("Gnomon %s data successfully deleted.", strings.ToLower(session.Network)), session.Window)
					successDialog.SetOnClosed(func() {})
					successDialog.Show()
				}
			},
		)
	})

	btnRestoreDefaults := widget.NewButton("Restore Defaults", func() {
		verificationOverlay(
			false,
			"ADVANCED",
			"Reset all settings to defaults?",
			"Confirm",
			func(b bool) {
				if b {
					setNetwork(NETWORK_MAINNET)
					setDaemon(DEFAULT_REMOTE_DAEMON)
					setAuthMode("true")
					setGnomon("1")
					remoteAccess.RPC.user = "username"
					remoteAccess.RPC.pass = "password"
					remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
					setRemoteAccess(remoteAccess.RPC.port, "RPC")

					successDialog := dialog.NewInformation("Success", "All settings have been restored to defaults.", session.Window)
					successDialog.SetOnClosed(func() {})
					successDialog.Show()
				}
			},
		)
	})

	btnExportDebugLog := widget.NewButton("Export Debug Log", func() {
		debugLogPath := getDebugLogPath()
		data, err := os.ReadFile(debugLogPath)
		if err != nil {
			if os.IsNotExist(err) {
				dialog.ShowInformation("Debug Log", "No debug log file found yet.", session.Window)
				return
			}

			logger.Errorf("[Engram] Could not read debug log %s: %s\n", debugLogPath, err)
			dialog.ShowError(fmt.Errorf("could not read debug log"), session.Window)
			return
		}

		dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			if err != nil {
				logger.Errorf("[Engram] File dialog: %s\n", err)
				dialog.ShowError(fmt.Errorf("could not export debug log"), session.Window)
				return
			}

			if uri == nil {
				return
			}

			if _, err = writeToURI(data, uri); err != nil {
				logger.Errorf("[Engram] Exporting debug log %s: %s\n", debugLogPath, err)
				dialog.ShowError(fmt.Errorf("could not export debug log"), session.Window)
				return
			}

			dialog.ShowInformation("Debug Log", "Debug log exported successfully.", session.Window)
		}, session.Window)

		if !a.Driver().Device().IsMobile() {
			uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
			if err == nil {
				dialogFileSave.SetLocation(uri)
			}
		}

		dialogFileSave.SetView(dialog.ListView)
		dialogFileSave.SetFileName(debugLogFileName)
		dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
		dialogFileSave.Show()
	})

	// DATASHARD Section components
	labelDatashard := canvas.NewText("DATASHARD", colors.Gray)
	labelDatashard.TextSize = scaleFont(11)
	labelDatashard.Alignment = fyne.TextAlignCenter
	labelDatashard.TextStyle = fyne.TextStyle{Bold: true}

	headerDatashard := canvas.NewText("DATASHARD  ID", colors.Gray)
	headerDatashard.TextSize = scaleFont(16)
	headerDatashard.Alignment = fyne.TextAlignCenter
	headerDatashard.TextStyle = fyne.TextStyle{Bold: true}

	address := engram.Disk.GetAddress().String()
	shardID := fmt.Sprintf("%x", sha1.Sum([]byte(address)))

	textDatashard := widget.NewRichTextFromMarkdown("### " + shardID)
	textDatashard.Wrapping = fyne.TextWrapWord

	textDatashardDesc := widget.NewRichTextFromMarkdown("Datashards hold encrypted data and stores it locally on your device. Each datashard is unique and can only be decrypted by the account it is associated with. Examples of data stored include:")
	textDatashardDesc.Wrapping = fyne.TextWrapWord

	textDatashardDesc2 := widget.NewRichTextFromMarkdown("* Datapad entries\n* Saved search history\n* Asset scan results\n* Account settings")
	textDatashardDesc2.Wrapping = fyne.TextWrapWord

	btnClearDatashard := widget.NewButton("Delete Datashard", nil)
	btnClearDatashard.OnTapped = func() {
		header := canvas.NewText("DATASHARD  DELETION  REQUESTED", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Are you sure?", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkClose := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkClose.OnTapped = func() {
			session.Datapad = ""
			session.DatapadChanged = false
			removeOverlays()
		}

		btnSubmit := widget.NewButton("Delete Datashard", nil)

		btnSubmit.OnTapped = func() {
			err := cleanWalletData()
			removeOverlays()
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("failed to delete datashard: %v", err), session.Window)
				} else {
					dialog.ShowInformation("Success", "Datashard deleted successfully!\n\nAll local data has been cleared.\nTELA scan will perform a fresh scan on next open.", session.Window)
				}
			})
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay := session.Window.Canvas().Overlays()

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						rectSpacer,
						rectSpacer,
						subHeader,
						widget.NewLabel(""),
						wrapMobileButton(btnSubmit),
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkClose,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	}

	// Create EPOCH statistics section (conditionally hidden when offline)
	epochSection := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			epochTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						wEpoch,
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochHashes,
							),
							container.NewStack(
								spacerEpoch,
								textEpochHashes,
							),
						),
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochBlocks,
							),
							container.NewStack(
								spacerEpoch,
								textEpochBlocks,
							),
						),
					),
				),
				layout.NewSpacer(),
			),
		),
	)

	if session.Offline {
		epochSection.Hide()
	}

	advancedContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				advancedTitle,
			),
		),
		rectSpacer,
		rectSpacer,

		// GNOMON Section
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			gnomonTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		gnomonDescription,
		rectSpacer,
		checkGnomon,

		// EPOCH STATISTICS Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			epochTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						wEpoch,
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochHashes,
							),
							container.NewStack(
								spacerEpoch,
								textEpochHashes,
							),
						),
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochBlocks,
							),
							container.NewStack(
								spacerEpoch,
								textEpochBlocks,
							),
						),
					),
				),
				layout.NewSpacer(),
			),
		),

		// SCANNING Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			scanningTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		scanningDescription,
		rectSpacer,
		entryTrackBlocks,

		// DATASHARD Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			labelDatashard,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						container.NewCenter(headerDatashard),
						rectSpacer,
						textDatashard,
						rectSpacer,
						textDatashardDesc,
						rectSpacer,
						textDatashardDesc2,
						rectSpacer,
						wrapMobileButton(btnClearDatashard),
					),
				),
				layout.NewSpacer(),
			),
		),

		// MAINTENANCE Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			maintenanceTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		wrapMobileButton(btnClearLocalData),
		rectSpacer,
		wrapMobileButton(btnRestoreDefaults),
		rectSpacer,
		wrapMobileButton(btnExportDebugLog),
		rectSpacer,
	)

	// Create the tab container with width constraint
	tabs := container.NewAppTabs(
		container.NewTabItem("Remote", remoteAccessContent),
		container.NewTabItem("TELA", telaContent),
		container.NewTabItem("Advanced", advancedContent),
	)

	// Select default tab based on how we navigated here
	if previousDomain == "app.tela.settings" {
		tabs.SelectIndex(1) // TELA tab
	} else {
		tabs.SelectIndex(0) // Default to Remote Access
	}

	// Wrap tabs in a container with fixed width
	tabsContainer := container.NewStack(
		rectWidth,
		tabs,
	)

	// Back button to return to previous screen (dashboard or TELA)
	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())

		// Use the tracked caller domain to handle returning from sub-pages properly
		targetDomain := previousDomain
		if targetDomain == "app.remoteaccess.manager" || targetDomain == "app.remoteaccess.permissions" {
			targetDomain = settingsCallerDomain
		}

		// Return to TELA if user came from there, otherwise dashboard
		if targetDomain == "app.tela" || targetDomain == "app.tela.settings" {
			session.Window.SetContent(layoutTELA())
		} else if targetDomain == "app.tela.manager" || targetDomain == "app.tela.manager.settings" {
			if cachedTelaManagerContent != nil {
				session.Domain = "app.tela.manager"
				session.Window.SetContent(cachedTelaManagerContent)
			} else {
				session.Window.SetContent(layoutDashboard())
			}
		} else {
			session.Window.SetContent(layoutDashboard())
		}
		removeOverlays()
	})

	// Main content area matching layoutSettings pattern
	formSettings := container.NewVBox(
		rectSpacer,
		rectSpacer,
		tabsContainer,
	)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				formSettings,
			),
			layout.NewSpacer(),
		),
	)

	top := container.NewVBox(
		rectSpacer,
		heading,
		rectSpacer,
	)

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	// Register with navigation stack (app settings allows back navigation)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutMessages() fyne.CanvasObject {
	session.Domain = "app.messages"

	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}

	if !walletapi.Connected {
		session.Window.SetContent(layoutSettings())
	}

	title := canvas.NewText("M Y    C O N T A C T S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	// Move definitions up
	contactInput := widget.NewEntry()
	contactInput.MultiLine = false
	contactInput.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	contactInput.PlaceHolder = "Search username or address"
	contactInput.SetIcon(theme.SearchIcon())

	btnSend := widget.NewButton("New Message", func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutPM())
		removeOverlays()
	})
	btnSend.Disable()

	rebuildBtn := widget.NewButton("Rebuild Message History", func() {
		rebuildMessageHistory()
	})

	checkLimit := widget.NewCheck(" Show only recent messages", nil)
	checkLimit.OnChanged = func(b bool) {
		if b {
			session.LimitMessages = true
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else {
			session.LimitMessages = false
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		}
	}

	if session.LimitMessages {
		checkLimit.Checked = true
	}

	top := container.NewVBox(
		canvas.NewRectangle(color.Transparent),
		container.NewCenter(
			title,
		),
		canvas.NewRectangle(color.Transparent),
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					contactInput,
					canvas.NewRectangle(color.Transparent),
					wrapMobileButton(btnSend),
					canvas.NewRectangle(color.Transparent),
					wrapMobileButton(rebuildBtn),
					canvas.NewRectangle(color.Transparent),
					container.NewCenter(checkLimit),
				),
			),
			layout.NewSpacer(),
		),
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes
	for _, obj := range top.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}
	// Also set sizes for spacers inside the nested VBox
	if hbox, ok := top.Objects[2].(*fyne.Container); ok {
		if stack, ok := hbox.Objects[1].(*fyne.Container); ok {
			if vbox, ok := stack.Objects[1].(*fyne.Container); ok {
				for _, obj := range vbox.Objects {
					if r, ok := obj.(*canvas.Rectangle); ok {
						r.SetMinSize(standardSpacerSize())
					}
				}
			}
		}
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	frame := &iframe{}
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	listMinFrac := float32(0.43)
	if !isMobile() {
		listMinFrac = 0.36
	}
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*listMinFrac))

	messages.Data = nil

	var height uint64

	if session.LimitMessages {
		height = engram.Disk.Get_Height() - 1000000
	} else {
		height = 0
	}

	threadSummaries := getMessageThreadSnapshot()
	data := []string{}
	if len(threadSummaries) > 0 {
		for _, thread := range threadSummaries {
			label := thread.Label
			if label == "" {
				label = resolveAddressDisplay(thread.ContactKey)
			}
			if label == "" && thread.ContactKey == "" {
				continue
			}
			data = append(data, thread.ContactKey+"~~~"+label)
		}
	}
	if len(data) == 0 {
		data = getMessages(height)
	}
	temp := data

	list := binding.BindStringList(&data)

	msgbox.List = widget.NewListWithData(list,
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}
			dataItem := strings.SplitN(str, "~~~", 4)
			if len(dataItem) < 2 {
				return
			}
			short := dataItem[0]
			address := short
			if len(short) > DEFAULT_USERADDR_SHORTEN_LENGTH {
				address = short[len(short)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
			}
			username := dataItem[1]
			if username == "" {
				username = resolveAddressDisplay(dataItem[0])
			}
			// If a username is longer than what *would* be a 'short' address of ...xyzxyzxyzx (e.g. 13), then shorten as well to be similar sizing
			if len(username) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
				username = "..." + username[len(username)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
			}

			label := co.(*widget.Label)
			if username == "" {
				label.SetText("..." + address)
			} else {
				label.SetText(username)
			}
			label.Wrapping = fyne.TextWrapWord
			label.TextStyle.Bold = false
			label.Alignment = fyne.TextAlignLeading
		})

	msgbox.List.OnSelected = func(id widget.ListItemID) {
		msgbox.List.UnselectAll()
		split := strings.Split(data[id], "~~~")
		if len(split) < 2 {
			return
		}
		if split[1] == "" {
			messages.Contact = split[0]
		} else {
			messages.Contact = split[1]
		}

		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutPM())
		removeOverlays()
	}

	validateContactInput := func(value string) bool {
		value = strings.TrimSpace(value)
		if value == "" {
			return false
		}

		if _, err := globals.ParseValidateAddress(value); err == nil {
			return true
		}

		_, err := checkUsername(value, -1)
		return err == nil
	}

	filterContacts := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		searchList := []string{}
		if query == "" {
			data = temp
			list.Reload()
			return
		}

		for _, d := range temp {
			tempd := strings.ToLower(d)
			split := strings.SplitN(tempd, "~~~", 4)
			if len(split) < 2 {
				continue
			}

			if strings.Contains(split[0], query) || strings.Contains(split[1], query) {
				searchList = append(searchList, d)
			}
		}

		data = searchList
		list.Reload()
	}

	contactInput.OnChanged = func(s string) {
		filterContacts(s)
		messages.Contact = strings.TrimSpace(s)
		if validateContactInput(s) {
			btnSend.Enable()
		} else {
			btnSend.Disable()
		}
	}

	features := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			msgbox.List,
		),
		layout.NewSpacer(),
	)

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.messages" {
			return
		}

		if k.Name == fyne.KeyUp {
			session.Dashboard = "main"

			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		} else if k.Name == fyne.KeyF5 {
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		}
	})

	subContainer := container.NewStack(
		container.NewVBox(
			canvas.NewRectangle(color.Transparent),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			canvas.NewRectangle(color.Transparent),
		),
	)

	// Set spacer sizes for subContainer
	if vbox, ok := subContainer.Objects[0].(*fyne.Container); ok {
		for _, obj := range vbox.Objects {
			if r, ok := obj.(*canvas.Rectangle); ok {
				r.SetMinSize(standardSpacerSize())
			}
		}
	}

	c := container.NewBorder(
		top,
		subContainer,
		nil,
		nil,
		features,
	)

	mainLayout := container.NewStack(
		frame,
		c,
	)

	return mainLayout
}

func layoutPM() fyne.CanvasObject {
	session.Domain = "app.messages.contact"

	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}

	if !walletapi.Connected {
		session.Window.SetContent(layoutSettings())
	}

	getPrimaryUsername()

	selectedKey, selectedLabel := resolveMessageContact(messages.Contact, -1)
	contactAddress := messages.Contact
	if selectedLabel != "" {
		contactAddress = selectedLabel
	} else if display := resolveAddressDisplay(selectedKey); display != "" {
		contactAddress = display
	} else if display := resolveAddressDisplay(strings.TrimSpace(messages.Contact)); display != "" {
		contactAddress = display
	}

	if len(contactAddress) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
		short := contactAddress[len(contactAddress)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
		contactAddress = "..." + short
	}

	heading := canvas.NewText(contactAddress, colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	lastActive := canvas.NewText("", colors.Gray)
	lastActive.TextSize = scaleFont(12)
	lastActive.Alignment = fyne.TextAlignCenter
	lastActive.TextStyle = fyne.TextStyle{Bold: false}

	backFromThread := func() {
		prev := session.LastDomain
		prevDom := session.PreviousDomain
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if prevDom != "" && prev != nil {
			session.Window.SetContent(prev)
			session.Domain = prevDom
			session.PreviousDomain = ""
		} else {
			session.Window.SetContent(layoutMessages())
		}
		removeOverlays()
	}
	btnBack := newSizedIconButton(theme.NavigateBackIcon(), backFromThread)

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(statusDotSize())
	frame := &iframe{}
	subframe := canvas.NewRectangle(color.Transparent)
	if isMobile() {
		subframe.SetMinSize(fyne.NewSize(ui.Width, scaleSize(48)))
	} else {
		subframe.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.36))
	}
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width*0.42, 30))
	rectOutbound := canvas.NewRectangle(color.Transparent)
	rectOutbound.SetMinSize(fyne.NewSize(ui.Width*0.166, 30))

	messages.Data = nil

	chats := container.NewVBox()

	chatFrame := container.NewStack(
		rectListBox,
		container.NewStack(
			chats,
		),
	)

	chatbox := container.NewVScroll(
		container.NewStack(
			chatFrame,
		),
	)

	var e *fyne.Container
	var height uint64

	if session.LimitMessages {
		height = engram.Disk.Get_Height() - 1000000
	} else {
		height = 0
	}

	messageRecords := getCachedThreadMessages(messages.Contact, height)
	if len(messageRecords) == 0 {
		records := getMessageCacheSnapshot()
		if len(records) == 0 {
			records = scanMessageTransfers(height)
		}
		for _, message := range records {
			if height > 0 && message.Entry.Height < height {
				continue
			}
			if messageMatchesContact(message, messages.Contact) {
				messageRecords = append(messageRecords, message)
			}
		}
	}
	originalMessageRecords := make([]MessageRecord, len(messageRecords))
	copy(originalMessageRecords, messageRecords)
	renderThread := func(filtered []MessageRecord) {
		messages.Data = nil
		chats.Objects = nil
		if len(filtered) == 0 {
			empty := widget.NewLabel("No messages found.")
			empty.Alignment = fyne.TextAlignCenter
			chats.Add(empty)
			chats.Refresh()
			chatbox.Refresh()
			return
		}

		renderedMessages := make([]RenderedThreadMessage, 0, len(filtered))
		useCachedRender := len(filtered) == len(originalMessageRecords)
		if useCachedRender {
			if cached, ok := getRenderedThreadCache(messages.Contact, height); ok {
				renderedMessages = cached
			} else {
				useCachedRender = false
			}
		}

		if !useCachedRender {
			for d := range filtered {
				if filtered[d].Entry.Incoming {
					replyback := messageReplyback(filtered[d].Entry)
					if replyback != "" {
						t := filtered[d].Entry.Time
						time := string(t.Format(time.RFC822))
						comment := filtered[d].Comment
						links := getTextURL(comment)

						for i := range links {
							if comment == links[i] {
								if len(links[i]) > 25 {
									comment = `[ ` + links[i][0:25] + "..." + ` ](` + links[i] + `)`
								} else {
									comment = `[ ` + links[i] + ` ](` + links[i] + `)`
								}
							} else {
								linkText := ""
								split := strings.Split(comment, links[i])
								if len(links[i]) > 25 {
									linkText = links[i][0:25] + "..."
								} else {
									linkText = links[i]
								}
								comment = `` + split[0] + `[link]` + split[1] + "\n\n›" + `[ ` + linkText + ` ](` + links[i] + `)`
							}
						}
						renderedMessages = append(renderedMessages, RenderedThreadMessage{Sender: replyback, Comment: comment, Timestamp: time, IsIncoming: true})
					}
				} else {
					t := filtered[d].Entry.Time
					time := string(t.Format(time.RFC822))
					comment := filtered[d].Comment
					links := getTextURL(comment)

					for i := range links {
						if comment == links[i] {
							if len(links[i]) > 25 {
								comment = `[ ` + links[i][0:25] + "..." + ` ](` + links[i] + `)`
							} else {
								comment = `[ ` + links[i] + ` ](` + links[i] + `)`
							}
						} else {
							linkText := ""
							split := strings.Split(comment, links[i])
							if len(links[i]) > 25 {
								linkText = links[i][0:25] + "..."
							} else {
								linkText = links[i]
							}
							comment = `` + split[0] + `[link]` + split[1] + "\n\n›" + `[ ` + linkText + ` ](` + links[i] + `)`
						}
					}
					renderedMessages = append(renderedMessages, RenderedThreadMessage{Sender: engram.Disk.GetAddress().String(), Comment: comment, Timestamp: time, IsIncoming: false})
				}
			}
			if len(filtered) == len(originalMessageRecords) {
				setRenderedThreadCache(messages.Contact, height, renderedMessages)
			}
		}

		if len(renderedMessages) > 0 {
			newObjects := make([]fyne.CanvasObject, 0, len(renderedMessages))
			newData := make([]string, 0, len(renderedMessages))
			for _, rendered := range renderedMessages {
				mdata := widget.NewRichTextFromMarkdown("")
				mdata.Wrapping = fyne.TextWrapWord
				datetime := canvas.NewText("", colors.Green)
				datetime.TextSize = scaleFont(11)
				boxColor := colors.Flint
				rect := canvas.NewRectangle(boxColor)
				rect.SetMinSize(fyne.NewSize(ui.Width*0.80, 30))
				rect.CornerRadius = scaleSize(5)
				rect5 := canvas.NewRectangle(color.Transparent)
				rect5.SetMinSize(smallSpacerSize())

				if !rendered.IsIncoming {
					rect.FillColor = colors.DarkGreen
					mdata.ParseMarkdown(rendered.Comment)
					datetime.Text = rendered.Timestamp
					e = container.NewBorder(
						nil,
						container.NewVBox(
							container.NewHBox(
								layout.NewSpacer(),
								datetime,
								rect5,
							),
							rect5,
						),
						rectOutbound,
						container.NewStack(
							rect,
							container.NewVBox(
								mdata,
							),
						),
					)
				} else {
					rect.FillColor = colors.Flint
					mdata.ParseMarkdown(rendered.Comment)
					datetime.Text = rendered.Timestamp
					e = container.NewBorder(
						nil,
						container.NewVBox(
							container.NewHBox(
								rect5,
								datetime,
								layout.NewSpacer(),
							),
							rect5,
						),
						container.NewStack(
							rect,
							container.NewVBox(
								mdata,
							),
						),
						rectOutbound,
					)
				}

				newData = append(newData, rendered.Sender+";;;;"+rendered.Comment+";;;;"+rendered.Timestamp)
				newObjects = append(newObjects, e)
			}
			messages.Data = newData
			chats.Objects = newObjects
			lastActive.Text = "Last Updated:  " + time.Now().Format(time.RFC822)
			lastActive.Refresh()
			chats.Refresh()
			chatbox.Refresh()
			chatbox.ScrollToBottom()
		}
	}

	renderThread(messageRecords)
	threadSearch := widget.NewEntry()
	threadSearch.PlaceHolder = "Search within this thread"
	threadSearch.OnChanged = func(s string) {
		query := strings.ToLower(strings.TrimSpace(s))
		if query == "" {
			renderThread(originalMessageRecords)
			return
		}

		filtered := make([]MessageRecord, 0)
		for _, message := range originalMessageRecords {
			if strings.Contains(strings.ToLower(message.Comment), query) {
				filtered = append(filtered, message)
			}
		}

		renderThread(filtered)
	}

	btnSend := widget.NewButton("Send", nil)
	btnSend.Disable()
	labelLimit := canvas.NewText("", colors.Gray)
	labelLimit.TextSize = scaleFont(11)
	labelLimit.Alignment = fyne.TextAlignLeading
	updateMessageLimit := func(message string, sender string) {
		if sender == "" && engram.Disk != nil {
			sender = engram.Disk.GetAddress().String()
		}

		args := rpc.Arguments{
			{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
			{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: uint64(1)},
			{Name: rpc.RPC_EXPIRY, DataType: rpc.DataTime, Value: time.Now().Add(time.Hour).UTC()},
			{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: message},
			{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: sender},
		}

		packed, err := args.MarshalBinary()
		if err != nil {
			labelLimit.Text = fmt.Sprintf("%d chars", len(message))
			labelLimit.Refresh()
			return
		}

		remaining := transaction.PAYLOAD0_LIMIT - len(packed)
		if remaining < 0 {
			remaining = 0
		}

		labelLimit.Text = fmt.Sprintf("%d chars, ~%d bytes left", len(message), remaining)
		labelLimit.Refresh()
	}

	var entry *mobileEntry
	entry = NewMobileEntry()
	entry.MultiLine = false
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.SetPlaceHolder("Message")
	updateMessageLimit("", session.Username)
	entry.OnChanged = func(s string) {
		messages.Message = s
		contact := messages.Contact
		//check, err := engram.Disk.NameToAddress(messages.Contact)
		check, err := checkUsername(messages.Contact, -1)
		if err == nil {
			contact = check
		}

		_, err = globals.ParseValidateAddress(contact)
		if err != nil {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
			return
		}
		updateMessageLimit(messages.Message, session.Username)

		err = checkMessagePack(messages.Message, session.Username, contact)
		if err != nil {
			btnSend.Text = "Message too long..."
			btnSend.Disable()
			btnSend.Refresh()
			return
		} else {
			if messages.Message == "" {
				btnSend.Text = "Send"
				btnSend.Disable()
				btnSend.Refresh()
			} else {
				btnSend.Text = "Send"
				btnSend.Enable()
				btnSend.Refresh()
			}
		}
	}

	btnSend.OnTapped = func() {
		if messages.Message == "" {
			return
		}
		contact := ""
		_, err := globals.ParseValidateAddress(messages.Contact)
		if err != nil {
			//check, err := engram.Disk.NameToAddress(messages.Contact)
			check, err := checkUsername(messages.Contact, -1)
			if err != nil {
				logger.Errorf("[Message] Failed to send: %s\n", err)
				btnSend.Text = "Failed to verify address..."
				btnSend.Disable()
				btnSend.Refresh()
				return
			}
			contact = check
		} else {
			contact = messages.Contact
		}

		btnSend.Text = "Setting up transfer..."
		btnSend.Disable()
		btnSend.Refresh()

		txid, err := sendMessage(messages.Message, session.Username, contact)
		if err != nil {
			logger.Errorf("[Message] Failed to send: %s\n", err)
			btnSend.Text = "Failed to send message..."
			btnSend.Disable()
			btnSend.Refresh()
			return
		}

		logger.Printf("[Message] Dispatched transaction successfully to: %s\n", messages.Contact)
		btnSend.Text = "Confirming..."
		btnSend.Disable()
		btnSend.Refresh()
		sentMessage := messages.Message
		messages.Message = ""
		entry.Text = ""
		entry.Refresh()
		updateMessageLimit("", session.Username)

		go func() {
			generation := currentWalletGeneration()
			walletapi.WaitNewHeightBlock()
			sHeight := walletapi.Get_Daemon_Height()
			var success bool
			for session.Domain == "app.messages.contact" {
				if !isWalletGenerationActive(generation) {
					return
				}

				var zeroscid crypto.Hash
				_, result := engram.Disk.Get_Payments_TXID(zeroscid, txid.String())

				if result.TXID != txid.String() {
					time.Sleep(time.Second * 1)
				} else {
					success = true
				}

				// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
				if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
					btnSend.Text = "Failed to send message..."
					btnSend.Disable()
					btnSend.Refresh()
					break
				}

				// If daemon height has incremented, print retry counters into button space
				if walletapi.Get_Daemon_Height()-sHeight > 0 {
					btnSend.Text = fmt.Sprintf("Confirming... (%d/%d)", walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
					btnSend.Refresh()
				}

				// If success, reload page w/ latest content. Otherwise retain the Failure message for UX relay
				if success {
					refreshMessageHistoryAsync(false)
					uiDo(func() {
						if !isWalletGenerationActive(generation) {
							return
						}
						messageRecords = append(messageRecords, MessageRecord{
							Entry: rpc.Entry{
								TXID:     txid.String(),
								Time:     time.Now(),
								Incoming: false,
							},
							ContactKey: messages.Contact,
							Label:      messages.Contact,
							Comment:    sentMessage,
						})
						originalMessageRecords = append(originalMessageRecords, messageRecords[len(messageRecords)-1])
						renderThread(messageRecords)
						btnSend.Text = "Send"
						btnSend.Disable()
						btnSend.Refresh()
					})
					break
				} else {
					time.Sleep(time.Second * 1)
				}
			}
		}()
	}

	top := container.NewVBox(
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		container.NewCenter(
			heading,
		),
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes for top
	for _, obj := range top.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	topContent := container.NewVBox(
		lastActive,
		canvas.NewRectangle(color.Transparent),
		threadSearch,
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes for topContent
	for _, obj := range topContent.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	middle := container.NewStack(subframe, chatbox)

	composerItems := []fyne.CanvasObject{
		canvas.NewRectangle(color.Transparent),
		labelLimit,
		canvas.NewRectangle(color.Transparent),
		entry,
		canvas.NewRectangle(color.Transparent),
		wrapMobileButton(btnSend),
		canvas.NewRectangle(color.Transparent),
	}

	for _, obj := range composerItems {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	bottomBlock := container.NewVBox(composerItems...)

	bottom := container.NewStack(
		container.NewVBox(
			canvas.NewRectangle(color.Transparent),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			canvas.NewRectangle(color.Transparent),
		),
	)

	// Set spacer sizes for bottom
	if vbox, ok := bottom.Objects[0].(*fyne.Container); ok {
		for _, obj := range vbox.Objects {
			if r, ok := obj.(*canvas.Rectangle); ok {
				r.SetMinSize(standardSpacerSize())
			}
		}
	}

	center := container.NewBorder(topContent, bottomBlock, nil, nil, middle)

	var gridItem1 *fyne.Container
	if isMobile() {
		pmScroll := container.NewVScroll(center)
		SetCurrentScrollBox(pmScroll)
		entry.OnFocusGained = func() {
			showVirtualKeyboard(entry)
		}
		gridItem1 = container.NewMax(pmScroll)
	} else {
		SetCurrentScrollBox(nil)
		gridItem1 = container.NewMax(center)
	}

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.messages.contact" {
			return
		}

		if k.Name == fyne.KeyUp {
			session.Dashboard = "app.messages"
			messages.Contact = ""
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else if k.Name == fyne.KeyEscape {
			session.Dashboard = "app.messages"
			messages.Contact = ""
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else if k.Name == fyne.KeyF5 {
			session.Window.SetContent(layoutPM())
		}
	})

	// Center slot receives all space between window edges and bottom bar; do not put
	// main content in Border "top" — top height is only MinSize() (see Fyne borderLayout).
	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		gridItem1,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return layout
}

func layoutRemoteAccess() fyne.CanvasObject {
	session.Domain = "app.remoteaccess"

	go refreshXSWDList()

	wSpacer := widget.NewLabel(" ")

	title := canvas.NewText("R E M O T E   A C C E S S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.20))

	frame := &iframe{}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	rpcLabel := canvas.NewText("      C O N F I G U R A T I O N      ", colors.Gray)
	rpcLabel.TextSize = scaleFont(11)
	rpcLabel.Alignment = fyne.TextAlignCenter
	rpcLabel.TextStyle = fyne.TextStyle{Bold: true}

	wsLabel := canvas.NewText("      C O N F I G U R A T I O N      ", colors.Gray)
	wsLabel.TextSize = scaleFont(11)
	wsLabel.Alignment = fyne.TextAlignCenter
	wsLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelConnections := canvas.NewText("  C O N N E C T I O N S  ", colors.Gray)
	labelConnections.TextSize = scaleFont(11)
	labelConnections.Alignment = fyne.TextAlignCenter
	labelConnections.TextStyle = fyne.TextStyle{Bold: true}

	sep1 := canvas.NewRectangle(colors.Gray)
	sep1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep1,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Window.SetContent(layoutTransition())
		if session.LastDomain != nil {
			session.Window.SetContent(session.LastDomain)
		} else {
			session.Window.SetContent(layoutDashboard())
		}
		removeOverlays()
	})

	shortShard := canvas.NewText("APPLICATION  CONNECTIONS", colors.Gray)
	shortShard.TextStyle = fyne.TextStyle{Bold: true}
	shortShard.TextSize = scaleFont(12)

	linkColor := colors.Green

	if remoteAccess.RPC.server == nil {
		session.Link = "Blocked"
		linkColor = colors.Gray
	}

	remoteAccess.RPC.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.RPC.status.TextSize = scaleFont(22)
	remoteAccess.RPC.status.TextStyle = fyne.TextStyle{Bold: true}

	serverStatus := canvas.NewText("APPLICATION  CONNECTIONS", colors.Gray)
	serverStatus.TextSize = scaleFont(12)
	serverStatus.Alignment = fyne.TextAlignCenter
	serverStatus.TextStyle = fyne.TextStyle{Bold: true}

	linkCenter := container.NewCenter(
		remoteAccess.RPC.status,
	)

	remoteAccess.RPC.userText = widget.NewEntry()
	remoteAccess.RPC.userText.PlaceHolder = "Username"
	remoteAccess.RPC.userText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.user = s
		}
	}

	remoteAccess.RPC.passText = widget.NewEntry()
	remoteAccess.RPC.passText.Password = true
	remoteAccess.RPC.passText.PlaceHolder = "Password"
	remoteAccess.RPC.passText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.pass = s
			StoreValue("settings", []byte("rpc_pass"), []byte(s))
		}
	}

	remoteAccess.RPC.portText = widget.NewEntry()
	remoteAccess.RPC.portText.PlaceHolder = "0.0.0.0:10103"
	remoteAccess.RPC.portText.Validator = func(s string) (err error) {
		regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
		test := regexp.MustCompile(regex)
		if test.MatchString(s) {
			remoteAccess.RPC.portText.SetValidationError(nil)
		} else {
			err = errors.New("invalid host name")
			remoteAccess.RPC.portText.SetValidationError(err)
		}

		return
	}
	remoteAccess.RPC.portText.SetText(getRemoteAccess("RPC"))

	linkColor = colors.Green

	if remoteAccess.WS.server == nil {
		session.Link = "Blocked"
		linkColor = colors.Gray
	}

	remoteAccess.WS.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.WS.status.TextSize = scaleFont(22)
	remoteAccess.WS.status.TextStyle = fyne.TextStyle{Bold: true}

	deckChoice := widget.NewSelect([]string{"Web Sockets (WS)", "Remote Procedure Calls (RPC)"}, nil)

	remoteAccess.RPC.toggle = widget.NewButton("Turn On", nil)
	remoteAccess.RPC.toggle.OnTapped = func() {
		switch session.Network {
		case NETWORK_TESTNET:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_TESTNET_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		case NETWORK_SIMULATOR:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_SIMULATOR_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		default:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		}

		toggleRPCServer(remoteAccess.RPC.port)
		if remoteAccess.RPC.server != nil {
			setRemoteAccess(remoteAccess.RPC.port, "RPC")
			deckChoice.Disable()
			remoteAccess.RPC.portText.Disable()
		} else {
			deckChoice.Enable()
			remoteAccess.RPC.portText.Enable()
		}
	}

	if remoteAccess.WS.portText == nil {
		remoteAccess.WS.portText = widget.NewEntry()
		remoteAccess.WS.portText.PlaceHolder = "0.0.0.0:44326"
		remoteAccess.WS.portText.Validator = func(s string) (err error) {
			regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
			test := regexp.MustCompile(regex)
			if test.MatchString(s) {
				remoteAccess.WS.portText.SetValidationError(nil)
			} else {
				err = errors.New("invalid host name")
				remoteAccess.WS.portText.SetValidationError(err)
			}

			return
		}

		remoteAccess.WS.portText.OnChanged = func(s string) {
			if remoteAccess.WS.portText.Validate() == nil {
				remoteAccess.WS.port = s
				setRemoteAccessDual(s, "WS") // Use dual storage instead of setRemoteAccess()

				// CRITICAL FIX: Save WebSocket enabled state to storage
				remoteAccess.WS.global.enabled = true
				setPermissions()
			}
		}
	}

	remoteAccess.WS.toggle = widget.NewButton("Turn On", nil)
	remoteAccess.WS.toggle.OnTapped = func() {
		if remoteAccess.WS.portText.Validate() != nil {
			remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
			remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
		} else {
			_, err := net.ResolveTCPAddr("tcp", remoteAccess.WS.port)
			if err != nil {
				logger.Errorf("[Remote Access] XSWD port: %s\n", err)
				remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
				remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
			} else {
				remoteAccess.WS.port = remoteAccess.WS.portText.Text
			}
		}

		remoteAccess.EPOCH.err = nil
		toggleXSWD(remoteAccess.WS.port)
		if remoteAccess.WS.server != nil {
			setRemoteAccessDual(remoteAccess.WS.port, "WS") // Use dual storage for consistency
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
			if remoteAccess.EPOCH.enabled {
				/*
					if remoteAccess.EPOCH.allowWithAddress {
						// If address is defined by dApp, GetWork will be started and stopped upon each WS call
						logger.Printf("[EPOCH] dApp addresses are enabled\n")
						return
					}
				*/

				err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
				if err != nil {
					logger.Errorf("[EPOCH] Connecting: %s\n", err)
					remoteAccess.EPOCH.err = err
				} else {
					remoteAccess.EPOCH.err = nil
					setRemoteAccess(epoch.GetPort(), "EPOCH")
				}
			}
		} else {
			stopEPOCH()
			remoteAccess.WS.portText.Enable()
			deckChoice.Enable()
		}
	}

	if session.Offline {
		remoteAccess.RPC.toggle.Text = "Disabled in Offline Mode"
		remoteAccess.RPC.toggle.Disable()
		remoteAccess.RPC.portText.Disable()
		remoteAccess.WS.toggle.Text = "Disabled in Offline Mode"
		remoteAccess.WS.toggle.Disable()
		remoteAccess.WS.portText.Disable()
	} else {
		if remoteAccess.RPC.server != nil {
			remoteAccess.RPC.status.Text = "Allowed"
			remoteAccess.RPC.status.Color = colors.Green
			remoteAccess.RPC.toggle.Text = "Turn Off"
			remoteAccess.RPC.userText.Disable()
			remoteAccess.RPC.passText.Disable()
			remoteAccess.RPC.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.RPC.status.Text = "Blocked"
			remoteAccess.RPC.status.Color = colors.Gray
			remoteAccess.RPC.toggle.Text = "Turn On"
			remoteAccess.RPC.userText.Enable()
			remoteAccess.RPC.passText.Enable()
			remoteAccess.RPC.portText.Enable()
		}

		if remoteAccess.WS.server != nil {
			remoteAccess.WS.status.Text = "Allowed"
			remoteAccess.WS.status.Color = colors.Green
			remoteAccess.WS.toggle.Text = "Turn Off"
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.WS.status.Text = "Blocked"
			remoteAccess.WS.status.Color = colors.Gray
			remoteAccess.WS.toggle.Text = "Turn On"
			remoteAccess.WS.portText.Enable()
		}
	}

	remoteAccess.RPC.userText.SetText(remoteAccess.RPC.user)
	remoteAccess.RPC.passText.SetText(remoteAccess.RPC.pass)

	linkCopy := widget.NewHyperlinkWithStyle("Copy Credentials", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopy.OnTapped = func() {
		a.Clipboard().SetContent(remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass)
	}

	linkPermissions := widget.NewHyperlinkWithStyle("Settings", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkPermissions.OnTapped = func() {
		//if remoteAccess.WS.server != nil {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDPermissions())
		removeOverlays()
		//}
	}

	/*
		linkApps := widget.NewHyperlinkWithStyle("View Connections", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkApps.OnTapped = func() {
			if remoteAccess.WS.server != nil {
				session.LastDomain = session.Window.Content()
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutXSWDConnections())
				removeOverlays()
			}
		}
	*/

	remoteAccess.WS.list = widget.NewList(
		func() int {
			return len(remoteAccess.WS.apps)
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabel(""),
				//widget.NewLabel(""),
			)
		},
		func(li widget.ListItemID, co fyne.CanvasObject) {
			app := remoteAccess.WS.apps[li]

			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*widget.Label).SetText(app.Name)
				//co.(*fyne.Container).Objects[1].(*widget.Label).SetText(app.Id)
			})
		},
	)

	remoteAccess.WS.list.OnSelected = func(id widget.ListItemID) {
		remoteAccess.WS.list.UnselectAll()
		remoteAccess.WS.list.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDAppManager(&remoteAccess.WS.apps[id]))
		removeOverlays()
	}

	xswdForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			wsLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					container.NewCenter(
						remoteAccess.WS.status,
					),
					rectSpacer,
					serverStatus,
					wSpacer,
					remoteAccess.WS.toggle,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkPermissions,
						layout.NewSpacer(),
					),
				),
			),
		),
		container.NewStack(
			rectWidth90,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					labelConnections,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rect,
						remoteAccess.WS.list,
					),
				),
			),
		),
		layout.NewSpacer(),
	)

	rpcForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			rpcLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					linkCenter,
					rectSpacer,
					serverStatus,
					wSpacer,
					remoteAccess.RPC.toggle,
					wSpacer,
					remoteAccess.RPC.portText,
					rectSpacer,
					remoteAccess.RPC.userText,
					rectSpacer,
					remoteAccess.RPC.passText,
					wSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkCopy,
						layout.NewSpacer(),
					),
				),
			),
			layout.NewSpacer(),
		),
	)

	deckFeatures := container.NewStack()
	if remoteAccess.RPC.server != nil {
		deckFeatures.Add(rpcForm)
		deckChoice.SetSelectedIndex(1)
	} else {
		deckFeatures.Add(xswdForm)
		deckChoice.SetSelectedIndex(0)
	}

	deckChoice.OnChanged = func(s string) {
		if s == "Remote Procedure Calls (RPC)" {
			deckFeatures.Objects[0] = rpcForm
		} else {
			deckFeatures.Objects[0] = xswdForm
		}
	}

	deckForm := container.NewVScroll(
		container.NewStack(
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewVBox(
						title,
					),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rectWidth90,
						deckChoice,
					),
				),
				container.NewBorder(
					deckFeatures,
					nil,
					nil,
					nil,
				),
			),
		),
	)

	deckForm.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyLeft {
			session.Dashboard = "main"
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		}
	})

	subContainer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		deckForm,
		subContainer,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

// Layout details of an app connected through web socket
func layoutXSWDAppManager(ad *xswd.ApplicationData) fyne.CanvasObject {
	session.Domain = "app.remoteaccess.manager"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelName := widget.NewRichText(&widget.TextSegment{
		Text: ad.Name,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		}})
	labelName.Wrapping = fyne.TextWrapWord

	labelDesc := widget.NewRichText(&widget.TextSegment{
		Text: ad.Description,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			TextStyle: fyne.TextStyle{Bold: false},
		}})
	labelDesc.Wrapping = fyne.TextWrapWord

	labelID := canvas.NewText("   APP  ID", colors.Gray)
	labelID.TextSize = scaleFont(14)
	labelID.Alignment = fyne.TextAlignLeading
	labelID.TextStyle = fyne.TextStyle{Bold: true}

	textID := widget.NewRichTextFromMarkdown(ad.Id)
	textID.Wrapping = fyne.TextWrapWord

	labelSignature := canvas.NewText("   SIGNATURE", colors.Gray)
	labelSignature.TextSize = scaleFont(14)
	labelSignature.Alignment = fyne.TextAlignLeading
	labelSignature.TextStyle = fyne.TextStyle{Bold: true}

	textSignature := widget.NewRichTextFromMarkdown("")
	textSignature.Wrapping = fyne.TextWrapWord

	labelURL := canvas.NewText("   URL", colors.Gray)
	labelURL.TextSize = scaleFont(14)
	labelURL.Alignment = fyne.TextAlignLeading
	labelURL.TextStyle = fyne.TextStyle{Bold: true}

	textURL := widget.NewRichTextFromMarkdown(ad.Url)
	textURL.Wrapping = fyne.TextWrapWord

	labelPermissions := canvas.NewText("   PERMISSIONS", colors.Gray)
	labelPermissions.TextSize = scaleFont(14)
	labelPermissions.Alignment = fyne.TextAlignLeading
	labelPermissions.TextStyle = fyne.TextStyle{Bold: true}

	labelEvents := canvas.NewText("   EVENTS", colors.Gray)
	labelEvents.TextSize = scaleFont(14)
	labelEvents.Alignment = fyne.TextAlignLeading
	labelEvents.TextStyle = fyne.TextStyle{Bold: true}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")
	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")
	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")
	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")
	labelSeparator5 := widget.NewRichTextFromMarkdown("")
	labelSeparator5.Wrapping = fyne.TextWrapOff
	labelSeparator5.ParseMarkdown("---")
	labelSeparator6 := widget.NewRichTextFromMarkdown("")
	labelSeparator6.Wrapping = fyne.TextWrapOff
	labelSeparator6.ParseMarkdown("---")

	signatureItems := container.NewVBox(
		labelSeparator2,
		rectSpacer,
		rectSpacer,
		labelSignature,
		textSignature,
		rectSpacer,
		rectSpacer,
	)

	// Show signature result if one exists
	signatureItems.Hide()
	if len(ad.Signature) > 0 {
		signatureItems.Show()
		_, message, err := engram.Disk.CheckSignature(ad.Signature)
		if err != nil {
			textSignature.ParseMarkdown(err.Error())
		} else {
			textSignature.ParseMarkdown(strings.TrimSpace(string(message)))
		}
	}

	// Find Permissions for connected app and build UI object
	var methods []string
	for k := range ad.Permissions {
		methods = append(methods, k)
	}

	permissionItems := container.NewVBox()

	permissions := []string{
		xswd.Ask.String(),
		xswd.Allow.String(),
		xswd.Deny.String(),
		xswd.AlwaysAllow.String(),
		xswd.AlwaysDeny.String(),
	}

	if len(methods) > 0 {
		sort.Strings(methods)
		for _, name := range methods {
			permission := widget.NewSelect(permissions, nil)
			permission.SetSelected(ad.Permissions[name].String())
			permission.Disable()
			permissionItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### "+name), permission))
		}
	} else {
		permissionItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("No Permissions"), nil))
	}

	// Find RegisteredEvents for connected app and build UI object
	var events []rpc.EventType
	for k := range ad.RegisteredEvents {
		events = append(events, k)
	}

	eventItems := container.NewVBox()

	if len(events) > 0 {
		sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
		for _, name := range events {
			event := widget.NewSelect([]string{"false", "true"}, nil)
			event.SetSelected(strconv.FormatBool(ad.RegisteredEvents[name]))
			event.Disable()
			eventItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown(fmt.Sprintf("### %s", name)), event))
		}
	} else {
		eventItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("No Events"), nil))
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
	})

	image := canvas.NewImageFromResource(resourceWebsocketPng)
	image.SetMinSize(fyne.NewSize(ui.Width*0.25, ui.Width*0.25))
	image.FillMode = canvas.ImageFillContain

	// Check if the application is TELA
	telaURL := "http://localhost"
	if strings.HasPrefix(ad.Url, telaURL) {
		for _, serv := range getTelaActiveServers() {
			if strings.HasPrefix(ad.Url, telaURL+serv.Address) {
				name, _, icon, _, _ := getContractHeader(crypto.HashHexToHash(serv.SCID))
				if icon != "" {
					if img, err := handleImageURL(name, icon, fyne.NewSize(ui.Width*0.25, ui.Width*0.25)); err == nil {
						image = img
					} else {
						logger.Errorf("[Engram] Could not validate icon image: %s\n", err)
					}
				}

				break
			}
		}
	}

	linkURL := widget.NewHyperlinkWithStyle("Open in browser", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkURL.OnTapped = func() {
		link, err := url.Parse(ad.Url)
		if err != nil {
			logger.Errorf("[Engram] Error parsing XSWD application URL: %s\n", err)
			return
		}
		_ = fyne.CurrentApp().OpenURL(link)
	}

	btnRemove := widget.NewButton("Remove", nil)
	btnRemove.OnTapped = func() {
		if remoteAccess.WS.server != nil && len(remoteAccess.WS.apps) > 0 {
			remoteAccess.WS.server.RemoveApplication(ad)
			removeOverlays()
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutRemoteAccess())
		}
	}

	center := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewStack(
				rectWidth90,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						container.NewHBox(
							layout.NewSpacer(),
							image,
							layout.NewSpacer(),
						),
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelName,
							),
							layout.NewSpacer(),
						),
						labelDesc,
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelID,
						textID,
						rectSpacer,
						rectSpacer,
						signatureItems,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelURL,
						rectSpacer,
						textURL,
						container.NewHBox(
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkURL,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelPermissions,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
						),
						permissionItems,
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelEvents,
						rectSpacer,
						eventItems,
						container.NewStack(
							rectWidth90,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator6,
						rectSpacer,
						rectSpacer,
						wrapMobileButton(btnRemove),
						rectSpacer,
						rectSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
		rectSpacer,
		rectSpacer,
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

// Layout XSWD permissions settings
func layoutXSWDPermissions() fyne.CanvasObject {
	session.Domain = "app.remoteaccess.permissions"

	wSpacer := widget.NewLabel(" ")

	frame := &iframe{}

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	title := canvas.NewText("G L O B A L   P E R M I S S I O N S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	xswdLabel := canvas.NewText("W E B   S O C K E T S", colors.Gray)
	xswdLabel.TextSize = scaleFont(11)
	xswdLabel.Alignment = fyne.TextAlignCenter
	xswdLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelMethods := canvas.NewText("  METHODS", colors.Gray)
	labelMethods.TextSize = scaleFont(14)
	labelMethods.Alignment = fyne.TextAlignLeading
	labelMethods.TextStyle = fyne.TextStyle{Bold: true}

	labelConnection := canvas.NewText("  CONNECTIONS", colors.Gray)
	labelConnection.TextSize = scaleFont(14)
	labelConnection.Alignment = fyne.TextAlignLeading
	labelConnection.TextStyle = fyne.TextStyle{Bold: true}

	labelEpoch := canvas.NewText("  EPOCH", colors.Gray)
	labelEpoch.TextSize = scaleFont(14)
	labelEpoch.Alignment = fyne.TextAlignLeading
	labelEpoch.TextStyle = fyne.TextStyle{Bold: true}

	permissionInfo := canvas.NewText("APPLY ON CONNECTION", colors.Gray)
	permissionInfo.TextSize = scaleFont(12)
	permissionInfo.Alignment = fyne.TextAlignCenter
	permissionInfo.TextStyle = fyne.TextStyle{Bold: true}

	btnDefaults := widget.NewButton("Restore Defaults", nil)

	wMode := widget.NewCheck("Restrictive Mode", nil)

	// Simple/Advanced Mode Toggle
	wSimpleMode := widget.NewCheck("Simple Mode (Recommended)", nil)
	wSimpleMode.Checked = IsSimpleMode()

	wConnection := widget.NewSelect([]string{xswd.Ask.String(), xswd.Allow.String()}, nil)

	wGlobalPermissions := widget.NewSelect([]string{"Off", "Apply"}, nil)

	wEpoch := widget.NewSelect([]string{xswd.Deny.String(), xswd.Allow.String()}, nil)

	wEpochAddress := widget.NewSelect([]string{"My Address", "dApp Chooses"}, nil)

	/*
		if remoteAccess.EPOCH.enabled {
			wEpoch.SetSelectedIndex(1)
		} else {
			wEpoch.SetSelectedIndex(0)
			wEpochAddress.Disable()
		}

		if remoteAccess.EPOCH.allowWithAddress {
			wEpochAddress.SetSelectedIndex(1)
		} else {
			wEpochAddress.SetSelectedIndex(0)
		}

		wEpoch.OnChanged = func(s string) {
			if s == xswd.Allow.String() {
				remoteAccess.EPOCH.enabled = true
				wEpochAddress.Enable()
				return
			}

			remoteAccess.EPOCH.enabled = false
			wEpochAddress.SetSelectedIndex(0)
			wEpochAddress.Disable()
		}

		wEpochAddress.OnChanged = func(s string) {
			if s == "dApp Chooses" {
				remoteAccess.EPOCH.allowWithAddress = true
				return
			}

			remoteAccess.EPOCH.allowWithAddress = false
		}
	*/

	spacerEpoch := canvas.NewRectangle(color.Transparent)
	spacerEpoch.SetMinSize(fyne.NewSize(140, 0))

	entryEpochWork := widget.NewEntry()
	entryEpochWork.SetPlaceHolder(":10100")
	entryEpochWork.SetText(epoch.GetPort())
	entryEpochWork.Validator = func(s string) (err error) {
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid port")
		}

		return epoch.SetPort(i)
	}

	entryEpochHash := widget.NewEntry()
	entryEpochHash.SetPlaceHolder("Max hashes")
	entryEpochHash.SetText(strconv.Itoa(epoch.GetMaxHashes()))
	entryEpochHash.Validator = func(s string) (err error) {
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid hash value")
		}

		return epoch.SetMaxHashes(i)
	}

	wEpochPower := widget.NewSelect([]string{"Less", "More"}, nil)
	wEpochPower.SetSelectedIndex(0)
	if epoch.GetMaxThreads() > 2 {
		wEpochPower.SetSelectedIndex(1)
	}

	wEpochPower.OnChanged = func(s string) {
		if s == "More" {
			half := runtime.NumCPU() / 2
			if half > epoch.DEFAULT_MAX_THREADS {
				epoch.SetMaxThreads(half)
			}

			return
		}

		epoch.SetMaxThreads(epoch.DEFAULT_MAX_THREADS)
	}

	if session.Offline {
		wMode.Disable()
		wEpoch.Disable()
		wEpochAddress.Disable()
		entryEpochWork.Disable()
		entryEpochHash.Disable()
		wEpochPower.Disable()
	} else if remoteAccess.WS.server != nil {
		wEpoch.Disable()
		wEpochAddress.Disable()
		entryEpochWork.Disable()
		entryEpochHash.Disable()
		wEpochPower.Disable()
	}

	if remoteAccess.WS.advanced {
		wMode.SetChecked(false)
		if remoteAccess.WS.global.enabled {
			wGlobalPermissions.SetSelectedIndex(1)
			if remoteAccess.WS.global.connect {
				wConnection.SetSelectedIndex(1)
			} else {
				wConnection.SetSelectedIndex(0)
			}
		} else {
			wGlobalPermissions.SetSelectedIndex(0)
			wConnection.SetSelectedIndex(0)
			wConnection.Disable()
			btnDefaults.Disable()
		}
	} else {
		wMode.SetChecked(false)
		wConnection.SetSelectedIndex(0)
		wConnection.Disable()
		wGlobalPermissions.SetSelectedIndex(0)
		wGlobalPermissions.Disable()
		btnDefaults.Disable()
	}

	wMode.OnChanged = func(b bool) {
		remoteAccess.WS.advanced = !b // inverse as check box is for restrictive mode on/off
		if remoteAccess.WS.advanced {
			wGlobalPermissions.Enable()
		} else {
			wGlobalPermissions.SetSelectedIndex(0) // calling this here resets and disables wConnection
			wGlobalPermissions.Disable()
		}
	}

	wConnection.OnChanged = func(s string) {
		if s == xswd.Allow.String() {
			remoteAccess.WS.global.connect = true
		} else {
			remoteAccess.WS.global.connect = false
		}
	}

	formItems := container.NewVBox()

	// Permission options for select widgets
	permissions := []string{
		xswd.Ask.String(),
		xswd.AlwaysAllow.String(),
		xswd.AlwaysDeny.String(),
	}

	noStorePermissions := []string{
		xswd.Ask.String(),
		xswd.AlwaysDeny.String(),
	}

	// onChanged handler for Advanced Mode individual permissions
	onChanged := func(n string) func(s string) {
		return func(s string) {
			remoteAccess.WS.Lock()
			defer remoteAccess.WS.Unlock()

			switch s {
			case xswd.Ask.String():
				remoteAccess.WS.global.permissions[n] = xswd.Ask
			case xswd.AlwaysAllow.String():
				remoteAccess.WS.global.permissions[n] = xswd.AlwaysAllow
			case xswd.AlwaysDeny.String():
				remoteAccess.WS.global.permissions[n] = xswd.AlwaysDeny
			default:
				remoteAccess.WS.global.permissions[n] = xswd.Ask
			}

			// Save updated permissions to storage
			setPermissions()
		}
	}

	// Build Simple Mode UI (6 grouped permissions)
	buildSimpleUI := func() {
		formItems.Objects = []fyne.CanvasObject{}

		for _, group := range permissionGroups {
			if !group.SimpleMode {
				continue // Skip hidden groups
			}

			// Group header with description
			header := widget.NewRichTextFromMarkdown("### " + group.Name)
			desc := canvas.NewText(group.Description, colors.Gray)
			desc.TextSize = scaleFont(11)

			// Permission selector
			permSelect := widget.NewSelect(permissions, nil)

			// Set current value from storage
			currentPerm := GetGroupPermission(group.Name)
			permSelect.SetSelected(currentPerm.String())

			// Disable if WebSocket is not enabled
			if !remoteAccess.WS.global.enabled {
				permSelect.SetSelectedIndex(0)
				permSelect.Disable()
			}

			// OnChanged handler
			permSelect.OnChanged = func(g string) func(s string) {
				return func(s string) {
					var perm xswd.Permission
					switch s {
					case xswd.AlwaysAllow.String():
						perm = xswd.AlwaysAllow
					case xswd.AlwaysDeny.String():
						perm = xswd.AlwaysDeny
					default:
						perm = xswd.Ask
					}
					SetGroupPermission(g, perm)
					logger.Printf("[Engram] Set group '%s' permission to %s", g, s)
				}
			}(group.Name)

			// Add to form
			groupContainer := container.NewVBox(
				header,
				desc,
				permSelect,
				rectSpacer,
			)
			formItems.Add(groupContainer)
		}
	}

	// Build Advanced Mode UI (all individual permissions)
	buildAdvancedUI := func() {
		formItems.Objects = []fyne.CanvasObject{}

		stored, methods := getPermissions()
		for _, name := range methods {
			n := name
			permission := widget.NewSelect([]string{}, nil)
			if engramCanStoreMethod(n) {
				permission.SetOptions(permissions)
			} else {
				permission.SetOptions(noStorePermissions)
			}

			if remoteAccess.WS.global.enabled {
				permission.SetSelected(stored[n].String())
				permission.OnChanged = onChanged(n)
			} else {
				permission.SetSelectedIndex(0)
				permission.Disable()
			}
			formItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### "+n), permission))
		}
	}

	// Simple/Advanced Mode Toggle Handler
	wSimpleMode.OnChanged = func(checked bool) {
		SetSimpleMode(checked)
		if checked {
			buildSimpleUI()
		} else {
			buildAdvancedUI()
		}
		formItems.Refresh()
		logger.Printf("[Engram] Switched to %s mode", map[bool]string{true: "Simple", false: "Advanced"}[checked])
	}

	// Build initial UI based on current mode
	if IsSimpleMode() {
		buildSimpleUI()
	} else {
		buildAdvancedUI()
	}

	statusText := "Disabled"
	statusColor := colors.Gray
	if remoteAccess.WS.global.enabled {
		statusText = "Enabled"
		statusColor = colors.Green
	}

	remoteAccess.WS.global.status = canvas.NewText(statusText, statusColor)
	remoteAccess.WS.global.status.TextSize = scaleFont(22)
	remoteAccess.WS.global.status.TextStyle = fyne.TextStyle{Bold: true}

	btnDefaults.OnTapped = func() {
		if !remoteAccess.WS.global.enabled {
			return
		}

		header := canvas.NewText("RESTORE  DEFAULT  PERMISSIONS", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Are you sure?", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkCancel := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkCancel.OnTapped = func() {
			removeOverlays()
		}

		btnSubmit := widget.NewButton("Restore Defaults", nil)
		btnSubmit.OnTapped = func() {
			wConnection.SetSelectedIndex(0)

			if IsSimpleMode() {
				// Restore Simple Mode defaults
				for _, group := range permissionGroups {
					if !group.SimpleMode {
						continue
					}
					defaultPerm := getSimpleDefault(group.Category)
					SetGroupPermission(group.Name, defaultPerm)
				}
				// Rebuild UI to show defaults
				buildSimpleUI()
			} else {
				// Restore Advanced Mode defaults
				remoteAccess.WS.Lock()
				remoteAccess.WS.global.permissions = SetDefaultPermissions()
				remoteAccess.WS.Unlock()
				setPermissions()
				// Rebuild UI
				buildAdvancedUI()
			}

			formItems.Refresh()
			removeOverlays()
			logger.Printf("[Engram] Restored default permissions in %s mode", map[bool]string{true: "Simple", false: "Advanced"}[IsSimpleMode()])
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay := session.Window.Canvas().Overlays()

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						rectSpacer,
						rectSpacer,
						subHeader,
						widget.NewLabel(""),
						wrapMobileButton(btnSubmit),
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkCancel,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	}

	wGlobalPermissions.OnChanged = func(s string) {
		if s != "Apply" {
			setPermissions()
			btnDefaults.Disable()
			remoteAccess.WS.global.status.Text = "Disabled"
			remoteAccess.WS.global.status.Color = colors.Gray
			remoteAccess.WS.global.status.Refresh()
			remoteAccess.WS.global.enabled = false
			wConnection.SetSelectedIndex(0)
			wConnection.Disable()

			// Disable all select widgets in formItems (works for both Simple and Advanced modes)
			for _, obj := range formItems.Objects {
				if container, ok := obj.(*fyne.Container); ok {
					for _, child := range container.Objects {
						if selectWidget, ok := child.(*widget.Select); ok {
							selectWidget.OnChanged = nil
							selectWidget.SetSelectedIndex(0)
							selectWidget.Disable()
						}
					}
				}
			}
		} else {
			remoteAccess.WS.global.status.Text = "Enabled"
			remoteAccess.WS.global.status.Color = colors.Green
			remoteAccess.WS.global.status.Refresh()
			remoteAccess.WS.global.enabled = true
			wConnection.Enable()
			btnDefaults.Enable()

			go func() {
				if IsSimpleMode() {
					// Rebuild Simple Mode UI with enabled selects
					fyne.Do(func() {
						buildSimpleUI()
						formItems.Refresh()
					})
				} else {
					// Rebuild Advanced Mode UI with enabled selects
					fyne.Do(func() {
						buildAdvancedUI()
						formItems.Refresh()
					})
				}
			}()
		}
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		setPermissions()
		removeOverlays()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
	})

	// Initialized in layoutRemoteAccess()
	remoteAccess.WS.portText.SetText(getRemoteAccess("WS"))

	center := container.NewVScroll(
		container.NewStack(
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewVBox(
						title,
						rectSpacer,
					),
				),
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					xswdLabel,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				container.NewCenter(
					container.NewVBox(
						rectWidth90,
						rectSpacer,
						container.NewCenter(
							remoteAccess.WS.global.status,
						),
						rectSpacer,
						container.NewCenter(
							permissionInfo,
						),
					),
				),
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewCenter(
						container.NewVBox(
							container.NewBorder(
								nil,
								nil,
								nil,
								nil,
								container.NewCenter(wMode),
							),
							rectSpacer,
							remoteAccess.WS.portText,
							rectSpacer,
							labelConnection,
							rectSpacer,
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Type"),
								wConnection,
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Global Permissions"),
								wGlobalPermissions,
							),
							wSpacer,
							labelEpoch,
							rectSpacer,
							/*
								container.NewBorder(
									nil,
									nil,
									widget.NewRichTextFromMarkdown("### Preference"),
									wEpoch,
								),
								container.NewBorder(
									nil,
									nil,
									widget.NewRichTextFromMarkdown("### Reward Address"),
									wEpochAddress,
								),
							*/
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Get Work"),
								container.NewHBox(
									layout.NewSpacer(),
									container.NewStack(
										spacerEpoch,
										entryEpochWork,
									),
								),
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Max Hashes"),
								container.NewHBox(
									layout.NewSpacer(),
									container.NewStack(
										spacerEpoch,
										entryEpochHash,
									),
								),
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Power"),
								wEpochPower,
							),
							wSpacer,
							labelMethods,
							rectSpacer,
							container.NewCenter(wSimpleMode),
							rectSpacer,
							container.NewCenter(
								formItems,
							),
							wSpacer,
						),
					),
					layout.NewSpacer(),
				),
				container.NewCenter(
					container.NewVBox(
						wrapMobileButton(btnDefaults),
						rectWidth90,
					),
				),
				wSpacer,
			),
		),
	)
	center.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			nil,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

func layoutIdentity() fyne.CanvasObject {
	session.Domain = "app.Identity"
	title := canvas.NewText("I D E N T I T Y", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	heading := canvas.NewText("My Contacts", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	frame := &iframe{}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.44))

	shortShard := canvas.NewText("PRIMARY  USERNAME", colors.Gray)
	shortShard.TextStyle = fyne.TextStyle{Bold: true}
	shortShard.TextSize = scaleFont(12)

	idCenter := container.NewCenter(
		shortShard,
	)

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAccount())
		removeOverlays()
	})

	//entryReg := NewMobileEntry()
	entryReg := widget.NewEntry()
	entryReg.MultiLine = false
	entryReg.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	userData, err := queryUsernames(engram.Disk.GetAddress().String())
	if err != nil {
		userData, err = getUsernames()
		if err != nil {
			userData = nil
		}
	}

	userList := binding.BindStringList(&userData)

	btnReg := widget.NewButton(" Register ", nil)
	btnReg.Disable()
	btnReg.OnTapped = func() {
		if len(session.NewUser) > 5 {
			valid, _ := checkUsername(session.NewUser, -1)
			if valid == "" {
				btnReg.Text = "Confirming..."
				btnReg.Disable()
				btnReg.Refresh()
				entryReg.Disable()
				storage, err := registerUsername(session.NewUser)
				if err != nil {
					if strings.Contains(err.Error(), "somehow the tx could not be built") {
						btnReg.Text = fmt.Sprintf("Insufficient Balance: Need %v", globals.FormatMoney(storage))
					} else {
						btnReg.Text = "Unable to register..."
					}
					btnReg.Refresh()
					logger.Errorf("[Username] %s\n", err)
				} else {
					go func() {
						generation := currentWalletGeneration()
						uiDo(func() {
							if !isWalletGenerationActive(generation) {
								return
							}
							entryReg.Text = ""
							entryReg.Refresh()
						})

						walletapi.WaitNewHeightBlock()
						sHeight := walletapi.Get_Daemon_Height()

						for {
							if !isWalletGenerationActive(generation) {
								return
							}

							if session.Domain == "app.Identity" {
								//vars, _, _, err := gnomon.Index.RPC.GetSCVariables("0000000000000000000000000000000000000000000000000000000000000001", engram.Disk.Get_Daemon_TopoHeight(), nil, []string{session.NewUser}, nil, false)
								usernames, err := queryUsernames(engram.Disk.GetAddress().String())
								if err != nil {
									logger.Errorf("[Username] Error querying usernames: %s\n", err)

									uiDo(func() {
										if !isWalletGenerationActive(generation) {
											return
										}
										btnReg.Text = "Error querying usernames"
										btnReg.Refresh()
									})

									return
								}

								for u := range usernames {
									if usernames[u] == session.NewUser {
										logger.Printf("[Username] Successfully registered username: %s\n", session.NewUser)
										_ = tx

										uiDo(func() {
											if !isWalletGenerationActive(generation) {
												return
											}
											btnReg.Text = "Registration successful!"
											btnReg.Refresh()
											session.NewUser = ""
											session.Window.SetContent(layoutIdentity())
										})

										return
									}
								}

								// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
								if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
									uiDo(func() {
										if !isWalletGenerationActive(generation) {
											return
										}
										btnReg.Text = "Unable to register..."
										btnReg.Refresh()
									})

									break
								}

								// If daemon height has incremented, print retry counters into button space
								if walletapi.Get_Daemon_Height()-sHeight > 0 {
									uiDo(func() {
										if !isWalletGenerationActive(generation) {
											return
										}
										btnReg.Text = fmt.Sprintf("Confirming... (%d/%d)", walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
										btnReg.Refresh()
									})
								}
							} else {
								break
							}

							time.Sleep(time.Second * 1)
						}
					}()
				}
			}
		}
	}

	entryReg.PlaceHolder = "New Username"
	entryReg.Validator = func(s string) error {
		btnReg.Text = " Register "
		btnReg.Enable()
		btnReg.Refresh()
		session.NewUser = s
		// Name Service SCID Logic
		//	15  IF STRLEN(name) >= 64 THEN GOTO 50 // skip names misuse
		//	20  IF STRLEN(name) >= 6 THEN GOTO 40
		if len(s) > 5 && len(s) < 64 {
			valid, _ := checkUsername(s, -1)
			if valid == "" {
				btnReg.Enable()
				btnReg.Refresh()
			} else {
				btnReg.Disable()
				err := errors.New("username already exists")
				entryReg.SetValidationError(err)
				btnReg.Refresh()
				return err
			}
		} else {
			btnReg.Disable()
			err := errors.New("username too short need a minimum of six characters")
			entryReg.SetValidationError(err)
			btnReg.Refresh()
			return err
		}

		return nil
	}

	userBox := widget.NewListWithData(userList,
		func() fyne.CanvasObject {
			c := container.NewVBox(
				widget.NewLabel(""),
			)
			return c
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			if len(str) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
				str = "..." + str[len(str)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
			}

			co.(*fyne.Container).Objects[0].(*widget.Label).SetText(str)
			co.(*fyne.Container).Objects[0].(*widget.Label).Wrapping = fyne.TextWrapWord
			co.(*fyne.Container).Objects[0].(*widget.Label).TextStyle.Bold = false
			co.(*fyne.Container).Objects[0].(*widget.Label).Alignment = fyne.TextAlignLeading
		})

	err = getPrimaryUsername()
	if err != nil {
		session.Username = ""
	}

	dispUsername := session.Username
	if len(session.Username) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
		dispUsername = "..." + dispUsername[len(dispUsername)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
	}

	textUsername := canvas.NewText(dispUsername, colors.Green)
	textUsername.TextStyle = fyne.TextStyle{Bold: true}
	textUsername.TextSize = scaleFont(22)

	if session.Username == "" {
		textUsername.Text = "---"
		textUsername.Refresh()
	} /* else {
		for u := range userData {
			if userData[u] == session.Username {
				userBox.Select(u)
				userBox.ScrollTo(u)
			}
		}
	}*/

	userBox.OnSelected = func(id widget.ListItemID) {
		overlay := session.Window.Canvas().Overlays()
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)
		overlay.Add(layoutIdentityDetail(userData[id]))
		userBox.UnselectAll()
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			title,
		),
		rectSpacer,
		rectSpacer,
	)

	shardForm := container.NewVBox(
		idCenter,
		rectSpacer,
		container.NewStack(
			container.NewCenter(
				textUsername,
			),
		),
		rectSpacer,
		rectSpacer,
		entryReg,
		rectSpacer,
		container.NewStack(
			rectListBox,
			userBox,
		),
		rectSpacer,
		wrapMobileButton(btnReg),
		rectSpacer,
	)

	gridItem1 := container.NewCenter(
		shardForm,
	)

	features := container.NewCenter(
		layout.NewSpacer(),
		gridItem1,
		layout.NewSpacer(),
	)

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyRight {
			session.Dashboard = "main"

			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		} else if k.Name == fyne.KeyF5 {
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutIdentity())
			removeOverlays()
		}
	})

	subContainer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		subContainer,
		nil,
		nil,
		features,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutIdentityDetail(username string) fyne.CanvasObject {
	var address string

	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("C O N T A C T    I N F O", colors.Gray)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	top := container.NewVBox(
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		container.NewCenter(
			heading,
		),
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
	)

	for _, obj := range top.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	rectSpacerCompact := canvas.NewRectangle(color.Transparent)
	rectSpacerCompact.SetMinSize(compactSpacerSize())

	labelUsername := canvas.NewText("REGISTERED  USERNAME", colors.Gray)
	labelUsername.TextSize = scaleFont(11)
	labelUsername.Alignment = fyne.TextAlignCenter
	labelUsername.TextStyle = fyne.TextStyle{Bold: true}

	labelTransfer := canvas.NewText("  T R A N S F E R  ", colors.Gray)
	labelTransfer.TextSize = scaleFont(11)
	labelTransfer.Alignment = fyne.TextAlignCenter
	labelTransfer.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
	})

	valueUsername := canvas.NewText(username, colors.Green)
	valueUsername.TextSize = scaleFont(22)
	valueUsername.TextStyle = fyne.TextStyle{Bold: true}
	valueUsername.Alignment = fyne.TextAlignCenter

	btnSetPrimary := widget.NewButton("Set Primary Username", nil)
	btnSetPrimary.OnTapped = func() {
		setPrimaryUsername(username)
		session.Username = username
		//session.Window.SetContent(layoutIdentity())
		removeOverlays()
	}

	btnSend := widget.NewButton("Transfer Username", nil)

	inputAddress := widget.NewEntry()
	inputAddress.PlaceHolder = "Receiver Username or Address"
	inputAddress.Validator = func(s string) error {
		btnSend.Text = "Transfer Username"
		btnSend.Enable()
		btnSend.Refresh()
		address, _ = checkUsername(s, -1)
		if address == "" {
			_, err := globals.ParseValidateAddress(s)
			if err != nil {
				btnSend.Disable()
				btnSend.Refresh()
				err := errors.New("address does not exist")
				inputAddress.SetValidationError(err)
				inputAddress.Refresh()
				return err
			} else {
				btnSend.Enable()
				btnSend.Refresh()
				address = s
			}
		} else {
			btnSend.Enable()
			btnSend.Refresh()
		}

		return nil
	}

	btnSend.OnTapped = func() {
		if address != "" && address != engram.Disk.GetAddress().String() {
			btnSend.Text = "Setting up transfer..."
			btnSend.Disable()
			btnSend.Refresh()
			inputAddress.Disable()
			inputAddress.Refresh()
			btnSetPrimary.Disable()
			storage, err := transferUsername(username, address)
			if err != nil {
				address = ""
				if strings.Contains(err.Error(), "somehow the tx could not be built") {
					btnSend.Text = fmt.Sprintf("Insufficient Balance: Need %v", globals.FormatMoney(storage))
				} else {
					btnSend.Text = "Transfer failed..."
				}
				btnSend.Disable()
				btnSend.Refresh()
				inputAddress.Enable()
				inputAddress.Refresh()
				btnSetPrimary.Enable()
			} else {
				btnSend.Text = "Confirming..."
				btnSend.Refresh()
				go func() {
					generation := currentWalletGeneration()
					walletapi.WaitNewHeightBlock()
					sHeight := walletapi.Get_Daemon_Height()

					for {
						if !isWalletGenerationActive(generation) {
							return
						}

						found := false
						if session.Domain == "app.Identity" {
							usernames, err := queryUsernames(engram.Disk.GetAddress().String())
							if err != nil {
								logger.Errorf("[Username] Error querying usernames: %s\n", err)
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = "Error querying usernames"
									btnSend.Refresh()
									btnSetPrimary.Enable()
								})

								return
							}

							for u := range usernames {
								if usernames[u] == username {
									found = true
								}
							}

							if !found {
								logger.Printf("[TransferOwnership] %s was successfully transferred to: %s\n", username, address)
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									session.Window.SetContent(layoutTransition())
									session.Window.SetContent(layoutIdentity())
									removeOverlays()
								})

								break
							}

							// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
							if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
								logger.Errorf("[TransferOwnership] %s was unsuccessful in transferring to: %s\n", username, address)
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = "Unable to transfer..."
									btnSend.Refresh()
									btnSetPrimary.Enable()
								})

								break
							}

							// If daemon height has incremented, print retry counters into button space
							if walletapi.Get_Daemon_Height()-sHeight > 0 {
								uiDo(func() {
									if !isWalletGenerationActive(generation) {
										return
									}
									btnSend.Text = fmt.Sprintf("Confirming... (%d/%d)", walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
									btnSend.Refresh()
								})
							}
						} else {
							break
						}

						time.Sleep(time.Second * 1)
					}
				}()
			}
		}
	}

	center := container.NewStack(
		container.NewVScroll(
			container.NewStack(
				rectWidth,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						canvas.NewRectangle(color.Transparent),
						valueUsername,
						canvas.NewRectangle(color.Transparent),
						labelUsername,
						wSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								wrapMobileButton(btnSetPrimary),
							),
							layout.NewSpacer(),
						),
						wSpacer,
						container.NewStack(
							rectWidth,
							container.NewHBox(
								layout.NewSpacer(),
								line1,
								layout.NewSpacer(),
								labelTransfer,
								layout.NewSpacer(),
								line2,
								layout.NewSpacer(),
							),
						),
						wSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								inputAddress,
							),
							layout.NewSpacer(),
						),
						canvas.NewRectangle(color.Transparent),
						canvas.NewRectangle(color.Transparent),
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								wrapMobileButton(btnSend),
							),
							layout.NewSpacer(),
						),
					),
					layout.NewSpacer(),
				),
			),
		),
	)

	// Set spacer sizes for the inner VBox in center
	if vscroll, ok := center.Objects[0].(*container.Scroll); ok {
		if stack, ok := vscroll.Content.(*fyne.Container); ok {
			if hbox, ok := stack.Objects[1].(*fyne.Container); ok {
				if vbox, ok := hbox.Objects[1].(*fyne.Container); ok {
					for _, obj := range vbox.Objects {
						if r, ok := obj.(*canvas.Rectangle); ok {
							r.SetMinSize(standardSpacerSize())
						}
					}
				}
			}
		}
	}

	bottom := container.NewStack(
		container.NewVBox(
			canvas.NewRectangle(color.Transparent),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			canvas.NewRectangle(color.Transparent),
		),
	)

	if vbox, ok := bottom.Objects[0].(*fyne.Container); ok {
		for _, obj := range vbox.Objects {
			if r, ok := obj.(*canvas.Rectangle); ok {
				r.SetMinSize(standardSpacerSize())
			}
		}
	}

	mainLayout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return mainLayout
}

func layoutWaiting(title *canvas.Text, heading *canvas.Text, sub *canvas.Text, link *widget.Hyperlink) fyne.CanvasObject {
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.6, ui.Height*0.35))
	rect2 := canvas.NewRectangle(color.Transparent)
	rect2.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))
	frame := canvas.NewRectangle(color.Transparent)
	frame.SetMinSize(fyne.NewSize(ui.Width, ui.Height))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	label := canvas.NewText("PROOF-OF-WORK", colors.Gray)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.TextSize = scaleFont(12)
	hashes := canvas.NewText(fmt.Sprintf("%d", session.RegHashes), colors.Account)
	hashes.TextSize = scaleFont(18)

	go func() {
		for engram.Disk != nil {
			fyne.Do(func() {
				hashes.Text = fmt.Sprintf("%d", session.RegHashes)
				hashes.Refresh()
			})
			time.Sleep(500 * time.Millisecond)
		}
	}()

	session.Gif, _ = x.NewAnimatedGifFromResource(resourceAnimation2Gif)
	session.Gif.SetMinSize(rect.MinSize())
	session.Gif.Resize(rect.MinSize())
	session.Gif.Start()

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			title,
		),
		rectSpacer,
		rectSpacer,
	)

	waitForm := container.NewVBox(
		widget.NewLabel(""),
		rect2,
		heading,
		rectSpacer,
		sub,
		widget.NewLabel(""),
		container.NewStack(
			session.Gif,
		),
		widget.NewLabel(""),
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				container.NewCenter(
					rect2,
					hashes,
				),
				rectSpacer,
				container.NewCenter(
					rect2,
					label,
				),
			),
			layout.NewSpacer(),
		),
	)

	grid := container.NewHBox(
		layout.NewSpacer(),
		waitForm,
		layout.NewSpacer(),
	)

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), link),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		grid,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutAlert(t int) fyne.CanvasObject {
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.6, ui.Width*0.35))
	frame := &iframe{}
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	wSpacer := widget.NewLabel(" ")

	title := canvas.NewText("", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)
	title.Alignment = fyne.TextAlignCenter

	heading := canvas.NewText("", colors.Red)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter

	sub := widget.NewRichTextFromMarkdown("")
	sub.Wrapping = fyne.TextWrapWord

	labelSettings := widget.NewHyperlinkWithStyle("Review Settings", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	if t == 1 {
		title.Text = "E  R  R  O  R"
		heading.Text = "Connection Failure"
		sub.ParseMarkdown("Connection to " + session.Daemon + " has failed. Please review your settings and try again.")
		labelSettings.Text = "Review Settings"
		labelSettings.OnTapped = func() {
			session.Window.SetContent(layoutSettings())
		}
	} else if t == 2 {
		title.Text = "E  R  R  O  R"
		heading.Text = "Write Failure"
		sub.ParseMarkdown("Could not write data to disk, please check to make sure Engram has the proper permissions and/or you have unzipped the contents.")
		labelSettings.Text = "Review Settings"
		labelSettings.OnTapped = func() {
			session.Window.SetContent(layoutMain())
		}
	} else {
		title.Text = "E R R O R"
		heading.Text = "ID-10T Error Protocol"
		sub.ParseMarkdown("System malfunction... Please... Find... Help...")
		labelSettings.Text = "Review Settings"
		labelSettings.OnTapped = func() {
			session.Window.SetContent(layoutSettings())
		}
	}

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	session.Gif, _ = x.NewAnimatedGifFromResource(resourceAnimation2Gif)
	session.Gif.SetMinSize(rect.MinSize())
	session.Gif.Start()

	alertForm := container.NewVBox(
		wSpacer,
		wSpacer,
		rectHeader,
		container.NewStack(
			rect,
			res.red_alert,
		),
		heading,
		rectSpacer,
		sub,
		widget.NewLabel(""),
	)

	footer := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			labelSettings,
			layout.NewSpacer(),
		),
		wSpacer,
	)

	features := container.NewCenter(
		layout.NewSpacer(),
		alertForm,
		layout.NewSpacer(),
	)

	c := container.NewBorder(
		features,
		footer,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutHistory() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	var data []string
	var listData binding.StringList
	var listBox *widget.List
	var cachedTransfers []rpc.Entry
	var historyNormalRows []string
	var historyCoinbaseRows []string
	var historyMessageRows []string

	view := ""

	header := canvas.NewText("  Transaction History", colors.Green)
	header.TextSize = scaleFont(22)
	header.TextStyle = fyne.TextStyle{Bold: true}

	details_header := canvas.NewText("     Transaction Detail", colors.Green)
	details_header.TextSize = scaleFont(22)
	details_header.TextStyle = fyne.TextStyle{Bold: true}

	frame := &iframe{}
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	heading := canvas.NewText("H I S T O R Y", colors.Gray)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.3, 35))

	rectMid := canvas.NewRectangle(color.Transparent)
	rectMid.SetMinSize(fyne.NewSize(ui.Width*0.35, 35))

	results := canvas.NewText("", colors.Green)
	results.TextSize = scaleFont(13)

	listData = binding.BindStringList(&data)
	listBox = widget.NewListWithData(listData,
		func() fyne.CanvasObject {
			return container.NewHBox(
				container.NewStack(
					rect,
					widget.NewLabel(""),
				),
				container.NewStack(
					rectMid,
					widget.NewLabel(""),
				),
				container.NewStack(
					rect,
					widget.NewLabel(""),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, ";;;")

			if split[0] == "HEADER" {
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
				co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText("")
				co.(*fyne.Container).Objects[2].(*fyne.Container).Objects[1].(*widget.Label).SetText("")
				return
			}

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{} // Reset
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{} // Reset
			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
			co.(*fyne.Container).Objects[2].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[3])
		})

	rectSpacer = canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.60))

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	label := canvas.NewText(view, colors.Account)
	label.TextSize = scaleFont(15)
	label.TextStyle = fyne.TextStyle{Bold: true}
	findCachedTransfer := func(txid string) rpc.Entry {
		for i := range cachedTransfers {
			if cachedTransfers[i].TXID == txid {
				return cachedTransfers[i]
			}
		}
		return rpc.Entry{}
	}

	ensureHistoryRows := func() {
		cachedTransfers, historyNormalRows, historyCoinbaseRows, historyMessageRows = syncHistoryRows()
	}

	// Function to load Normal transactions
	loadNormal := func() {
		view = "Normal"
		listBox.UnselectAll()
		results.Text = "  Scanning..."
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyNormalRows...)

			results.Text = fmt.Sprintf("  Results:  %d", len(data))

			_ = listData.Set(data)

			listBox.OnSelected = func(id widget.ListItemID) {
				split := strings.Split(data[id], ";;;")
				if split[0] == "HEADER" {
					listBox.UnselectAll()
					return
				}
				result := findCachedTransfer(split[4])

				if result.TXID == "" {
					label.Text = "---"
				} else {
					label.Text = result.TXID
				}

				fyne.Do(func() {
					label.Refresh()
				})

				overlay := session.Window.Canvas().Overlays()
				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)
				overlay.Add(layoutHistoryDetail(split[4], result))
				listBox.UnselectAll()
			}

			fyne.Do(func() {
				results.Refresh()
				listBox.Refresh()
			})
		}()
	}

	// Function to load Coinbase transactions
	loadCoinbase := func() {
		view = "Coinbase"
		listBox.UnselectAll()
		results.Text = "  Scanning..."
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyCoinbaseRows...)

			results.Text = fmt.Sprintf("  Results:  %d", len(data))

			_ = listData.Set(data)

			listBox.OnSelected = func(id widget.ListItemID) {
				split := strings.Split(data[id], ";;;")
				if split[0] == "HEADER" {
					listBox.UnselectAll()
					return
				}
				result := findCachedTransfer(split[4])

				if result.TXID == "" {
					label.Text = "---"
				} else {
					label.Text = result.TXID
				}

				fyne.Do(func() {
					label.Refresh()
				})

				overlay := session.Window.Canvas().Overlays()
				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)
				overlay.Add(layoutHistoryDetail(split[4], result))
				listBox.UnselectAll()
			}

			fyne.Do(func() {
				results.Refresh()
				listBox.Refresh()
			})
		}()
	}

	// Function to load Messages
	loadMessages := func() {
		view = "Messages"
		listBox.UnselectAll()
		results.Text = "  Scanning..."
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyMessageRows...)

			results.Text = fmt.Sprintf("  Results:  %d", len(data))

			_ = listData.Set(data)

			listBox.OnSelected = func(id widget.ListItemID) {
				split := strings.Split(data[id], ";;;")
				if split[0] == "HEADER" {
					listBox.UnselectAll()
					return
				}
				result := findCachedTransfer(split[4])
				overlay := session.Window.Canvas().Overlays()
				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)
				overlay.Add(layoutHistoryDetail(split[4], result))
				listBox.UnselectAll()
				listBox.Refresh()
			}

			fyne.Do(func() {
				results.Refresh()
				listBox.Refresh()

			})
		}()
	}

	startIcon := theme.MenuDropDownIcon()
	if historySortOrder == "Ascending" {
		startIcon = theme.MenuDropUpIcon()
	}
	btnSortOrder := widget.NewButtonWithIcon("", startIcon, nil)
	btnSortOrder.Importance = widget.LowImportance
	btnSortOrder.Refresh()

	sortSize := canvas.NewRectangle(color.Transparent)
	sortSize.SetMinSize(fyne.NewSize(scaleSize(35), scaleSize(35)))
	btnSortOrderRow := container.NewStack(sortSize, btnSortOrder)

	btnSortOrder.OnTapped = func() {
		if historySortOrder == "Descending" {
			historySortOrder = "Ascending"
			btnSortOrder.SetIcon(theme.MenuDropUpIcon())
		} else {
			historySortOrder = "Descending"
			btnSortOrder.SetIcon(theme.MenuDropDownIcon())
		}
		refreshHistoryAsync(true)
		if view == "Normal" {
			loadNormal()
		} else if view == "Coinbase" {
			loadCoinbase()
		} else if view == "Messages" {
			loadMessages()
		}
	}

	// Create tab content containers (needed for proper tab rendering)
	normalTabContent := container.NewVBox()
	coinbaseTabContent := container.NewVBox()
	messagesTabContent := container.NewVBox()

	// Create tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("Normal", normalTabContent),
		container.NewTabItem("Coinbase", coinbaseTabContent),
		container.NewTabItem("Messages", messagesTabContent),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Handle tab changes
	tabs.OnChanged = func(tab *container.TabItem) {
		switch tab.Text {
		case "Normal":
			loadNormal()
		case "Coinbase":
			loadCoinbase()
		case "Messages":
			loadMessages()
		}
	}

	// Load Normal by default
	loadNormal()

	headerContent := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			container.NewVBox(
				container.NewBorder(nil, nil, btnSortOrderRow, nil, tabs),
				rectSpacer,
				container.NewCenter(results),
			),
		),
		layout.NewSpacer(),
	)

	top = container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		headerContent,
		rectSpacer,
	)

	center := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			listBox,
		),
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

func layoutHistoryDetail(txid string, transfer rpc.Entry) fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("T R A N S A C T I O N    D E T A I L", colors.Gray)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	labelTXID := canvas.NewText("   TRANSACTION  ID", colors.Gray)
	labelTXID.TextSize = scaleFont(14)
	labelTXID.Alignment = fyne.TextAlignLeading
	labelTXID.TextStyle = fyne.TextStyle{Bold: true}

	labelAmount := canvas.NewText("   AMOUNT", colors.Gray)
	labelAmount.TextSize = scaleFont(14)
	labelAmount.Alignment = fyne.TextAlignLeading
	labelAmount.TextStyle = fyne.TextStyle{Bold: true}

	labelDirection := canvas.NewText("   PAYMENT  DIRECTION", colors.Gray)
	labelDirection.TextSize = scaleFont(14)
	labelDirection.Alignment = fyne.TextAlignLeading
	labelDirection.TextStyle = fyne.TextStyle{Bold: true}

	labelMember := canvas.NewText("", colors.Gray)
	labelMember.TextSize = scaleFont(14)
	labelMember.Alignment = fyne.TextAlignLeading
	labelMember.TextStyle = fyne.TextStyle{Bold: true}

	labeliMember := canvas.NewText("", colors.Gray)
	labeliMember.TextSize = scaleFont(14)
	labeliMember.Alignment = fyne.TextAlignLeading
	labeliMember.TextStyle = fyne.TextStyle{Bold: true}

	labelProof := canvas.NewText("   TRANSACTION  PROOF", colors.Gray)
	labelProof.TextSize = scaleFont(14)
	labelProof.Alignment = fyne.TextAlignLeading
	labelProof.TextStyle = fyne.TextStyle{Bold: true}

	labelDestPort := canvas.NewText("   DESTINATION  PORT", colors.Gray)
	labelDestPort.TextSize = scaleFont(14)
	labelDestPort.TextStyle = fyne.TextStyle{Bold: true}

	labelSourcePort := canvas.NewText("   SOURCE  PORT", colors.Gray)
	labelSourcePort.TextSize = scaleFont(14)
	labelSourcePort.TextStyle = fyne.TextStyle{Bold: true}

	labelFees := canvas.NewText("   TRANSACTION  FEES", colors.Gray)
	labelFees.TextSize = scaleFont(14)
	labelFees.TextStyle = fyne.TextStyle{Bold: true}

	labelPayload := canvas.NewText("   PAYLOAD", colors.Gray)
	labelPayload.TextSize = scaleFont(14)
	labelPayload.TextStyle = fyne.TextStyle{Bold: true}

	labelHeight := canvas.NewText("   BLOCK  HEIGHT", colors.Gray)
	labelHeight.TextSize = scaleFont(14)
	labelHeight.TextStyle = fyne.TextStyle{Bold: true}

	labelReply := canvas.NewText("   REPLY  ADDRESS", colors.Gray)
	labelReply.TextSize = scaleFont(14)
	labelReply.TextStyle = fyne.TextStyle{Bold: true}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")

	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")

	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")

	labelSeparator5 := widget.NewRichTextFromMarkdown("")
	labelSeparator5.Wrapping = fyne.TextWrapOff
	labelSeparator5.ParseMarkdown("---")

	labelSeparator6 := widget.NewRichTextFromMarkdown("")
	labelSeparator6.Wrapping = fyne.TextWrapOff
	labelSeparator6.ParseMarkdown("---")

	labelSeparator7 := widget.NewRichTextFromMarkdown("")
	labelSeparator7.Wrapping = fyne.TextWrapOff
	labelSeparator7.ParseMarkdown("---")

	labelSeparator8 := widget.NewRichTextFromMarkdown("")
	labelSeparator8.Wrapping = fyne.TextWrapOff
	labelSeparator8.ParseMarkdown("---")

	labelSeparator9 := widget.NewRichTextFromMarkdown("")
	labelSeparator9.Wrapping = fyne.TextWrapOff
	labelSeparator9.ParseMarkdown("---")

	labelSeparator10 := widget.NewRichTextFromMarkdown("")
	labelSeparator10.Wrapping = fyne.TextWrapOff
	labelSeparator10.ParseMarkdown("---")

	labelSeparator11 := widget.NewRichTextFromMarkdown("")
	labelSeparator11.Wrapping = fyne.TextWrapOff
	labelSeparator11.ParseMarkdown("---")

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	var zeroscid crypto.Hash
	_, details := engram.Disk.Get_Payments_TXID(zeroscid, txid)
	details = enrichMessageEntry(details)

	transferTime := transfer.Time
	if transferTime.IsZero() {
		transferTime = details.Time
	}
	stamp := string(transferTime.Format(time.RFC822))
	height := strconv.FormatUint(details.Height, 10)

	valueMember := widget.NewRichTextFromMarkdown(" ")
	valueMember.Wrapping = fyne.TextWrapBreak

	valueiMember := widget.NewRichTextFromMarkdown("--")
	valueiMember.Wrapping = fyne.TextWrapBreak

	valueReply := widget.NewRichTextFromMarkdown("--")
	valueReply.Wrapping = fyne.TextWrapBreak

	if details.Payload_RPC.HasValue(rpc.RPC_REPLYBACK_ADDRESS, rpc.DataAddress) {
		address := details.Payload_RPC.Value(rpc.RPC_REPLYBACK_ADDRESS, rpc.DataAddress).(rpc.Address)
		valueReply.ParseMarkdown("" + address.String())
	} else if details.Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) && details.DestinationPort == 1337 {
		address := details.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string)
		valueReply.ParseMarkdown("" + address)
	}

	valuePayload := widget.NewRichTextFromMarkdown("--")
	valuePayload.Wrapping = fyne.TextWrapBreak

	if comment := messageComment(details); comment != "" {
		valuePayload.ParseMarkdown("" + comment)
	}

	valueAmount := canvas.NewText("", colors.Account)
	valueAmount.TextSize = scaleFont(22)
	valueAmount.TextStyle = fyne.TextStyle{Bold: true}

	valueDirection := canvas.NewText("", colors.Account)
	valueDirection.TextSize = scaleFont(22)
	valueDirection.TextStyle = fyne.TextStyle{Bold: true}
	if details.Coinbase {
		valueDirection.Text = "  Received"
		labelMember.Text = "  SOURCE"
		valueMember.ParseMarkdown("  Network (Mining Reward)")
		valueAmount.Color = colors.Green
		amount := details.Amount
		valueAmount.Text = "  + " + globals.FormatMoney(amount)
	} else if details.Incoming {
		valueDirection.Text = "  Received"
		labelMember.Text = "  SENDER  ADDRESS"
		if details.Sender == "" || details.Sender == engram.Disk.GetAddress().String() {
			valueMember.ParseMarkdown("--")
		} else {
			valueMember.ParseMarkdown("" + details.Sender)
		}

		if details.Amount == 0 {
			valueAmount.Color = colors.Account
			valueAmount.Text = "  0.00000"
		} else {
			valueAmount.Color = colors.Green
			valueAmount.Text = "  + " + globals.FormatMoney(details.Amount)
		}
	} else {
		valueDirection.Text = "  Sent"
		labelMember.Text = "  RECEIVER  ADDRESS"
		valueMember.ParseMarkdown("" + details.Destination)

		if details.Amount == 0 {
			valueAmount.Color = colors.Account
			valueAmount.Text = "  0.00000"
		} else {
			valueAmount.Color = colors.Account
			valueAmount.Text = "  - " + globals.FormatMoney(details.Amount)
		}
	}

	labeliMember.Text = "  INTEGRATED  ADDRESS"
	var idest string
	if details.Destination == "" {
		// We are the recipient
		idest = engram.Disk.GetAddress().String()
	} else {
		idest = details.Destination
	}
	iaddr, _ := rpc.NewAddress(idest)
	if iaddr != nil {
		var iargs rpc.Arguments
		for _, v := range details.Payload_RPC {
			if !iargs.HasValue(v.Name, v.DataType) {
				// Skip the reply back addr that was injected, but 'reverse' this to be what the original payload was which requests the reply addr
				if v.Name == rpc.RPC_REPLYBACK_ADDRESS {
					iargs = append(iargs, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataUint64, Value: uint64(1)})
				} else {
					iargs = append(iargs, rpc.Argument{Name: v.Name, DataType: v.DataType, Value: v.Value})
				}
			}
		}

		// If value transfer 'V' doesn't exist, we add it here.
		if !iargs.HasValue(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
			iargs = append(iargs, rpc.Argument{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: details.Amount})
		}

		iaddr.Arguments = iargs

		// Check to see if integrated addr creation makes an actual integrated addr
		if iaddr.String() != details.Destination && iaddr.IsIntegratedAddress() {
			valueiMember.ParseMarkdown("" + iaddr.String())
		}
	}

	valueTime := canvas.NewText(stamp, colors.Account)
	valueTime.TextSize = scaleFont(14)
	valueTime.TextStyle = fyne.TextStyle{Bold: true}

	valueFees := canvas.NewText("  "+globals.FormatMoney(details.Fees), colors.Account)
	valueFees.TextSize = scaleFont(22)
	valueFees.TextStyle = fyne.TextStyle{Bold: true}

	valueHeight := canvas.NewText("  "+height, colors.Account)
	valueHeight.TextSize = scaleFont(22)
	valueHeight.TextStyle = fyne.TextStyle{Bold: true}

	valueTXID := widget.NewRichTextFromMarkdown("")
	valueTXID.Wrapping = fyne.TextWrapBreak
	valueTXID.ParseMarkdown("" + txid)

	valuePort := canvas.NewText("", colors.Account)
	valuePort.TextSize = scaleFont(22)
	valuePort.TextStyle = fyne.TextStyle{Bold: true}
	valuePort.Text = "  " + strconv.FormatUint(details.DestinationPort, 10)

	valueSourcePort := canvas.NewText("", colors.Account)
	valueSourcePort.TextSize = scaleFont(22)
	valueSourcePort.TextStyle = fyne.TextStyle{Bold: true}
	valueSourcePort.Text = "  " + strconv.FormatUint(details.SourcePort, 10)

	btnView := newSizedTextButton("View in Explorer", func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + txid)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + txid)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	})

	linkBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		overlay := session.Window.Canvas().Overlays()
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	linkAddress := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkAddress.OnTapped = func() {
		a.Clipboard().SetContent(valueMember.String())
	}

	linkiAddress := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkiAddress.OnTapped = func() {
		a.Clipboard().SetContent(valueiMember.String())
	}

	linkReplyAddress := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkReplyAddress.OnTapped = func() {
		if replyAddress, ok := details.Payload_RPC.Value(rpc.RPC_REPLYBACK_ADDRESS, rpc.DataAddress).(rpc.Address); ok {
			a.Clipboard().SetContent(replyAddress.String())
		}
	}

	linkTXID := widget.NewHyperlinkWithStyle("Copy Transaction ID", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkTXID.OnTapped = func() {
		a.Clipboard().SetContent(txid)
	}

	linkProof := widget.NewHyperlinkWithStyle("Copy Transaction Proof", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkProof.OnTapped = func() {
		a.Clipboard().SetContent(details.Proof)
	}

	linkPayload := widget.NewHyperlinkWithStyle("Copy Payload", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkPayload.OnTapped = func() {
		if _, ok := details.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string); ok {
			a.Clipboard().SetContent(details.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string))
		}
	}

	top = container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		container.NewCenter(
			valueTime,
		),
		rectSpacer,
		rectSpacer,
	)

	center := container.NewStack(
		container.NewVScroll(
			container.NewStack(
				rectWidth,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						rectSpacer,
						labelDirection,
						rectSpacer,
						valueDirection,
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelAmount,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueAmount,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						labelTXID,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueTXID,
						),
						container.NewVBox(
							container.NewHBox(
								linkTXID,
								layout.NewSpacer(),
							),
							container.NewHBox(
								linkProof,
								layout.NewSpacer(),
							),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelMember,
						rectSpacer,
						valueMember,
						container.NewHBox(
							linkAddress,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labeliMember,
						rectSpacer,
						valueiMember,
						container.NewHBox(
							linkiAddress,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelReply,
						rectSpacer,
						valueReply,
						container.NewHBox(
							linkReplyAddress,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator6,
						rectSpacer,
						rectSpacer,
						labelHeight,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueHeight,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator7,
						rectSpacer,
						rectSpacer,
						labelFees,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueFees,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator8,
						rectSpacer,
						rectSpacer,
						labelPayload,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valuePayload,
						),
						container.NewVBox(
							container.NewHBox(
								linkPayload,
								layout.NewSpacer(),
							),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator9,
						rectSpacer,
						rectSpacer,
						labelDestPort,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valuePort,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator10,
						rectSpacer,
						rectSpacer,
						labelSourcePort,
						rectSpacer,
						container.NewStack(
							rectWidth90,
							valueSourcePort,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator11,
						rectSpacer,
						rectSpacer,
						btnView,
						wSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), linkBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return layout
}

func layoutDatapad() fyne.CanvasObject {
	session.Domain = "app.datapad"

	title := canvas.NewText("S E C U R E   N O T E S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	entryNewPad := widget.NewEntry()
	entryNewPad.MultiLine = false
	entryNewPad.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	btnAdd := widget.NewButton(" Create ", nil)
	btnAdd.Disable()

	top := container.NewVBox(
		rectSpacer,
		container.NewCenter(
			title,
		),
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					entryNewPad,
					rectSpacer,
					wrapMobileButton(btnAdd),
				),
			),
			layout.NewSpacer(),
		),
		rectSpacer,
	)

	btnAdd.OnTapped = func() {
		err := StoreEncryptedValue("Datapads", []byte(entryNewPad.Text), []byte(""))
		if err != nil {
			btnAdd.Text = "Error creating new Datapad"
			btnAdd.Disable()
			btnAdd.Refresh()
		} else {
			session.Datapad = entryNewPad.Text
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDatapad())
			removeOverlays()
		}
	}

	entryNewPad.PlaceHolder = "Note Name"
	entryNewPad.SetIcon(theme.SearchIcon())
	entryNewPad.Validator = func(s string) error {
		session.Datapad = s
		if len(s) > 0 {
			_, err := GetEncryptedValue("Datapads", []byte(s))
			if err == nil {
				btnAdd.Text = "Datapad already exists"
				btnAdd.Disable()
				btnAdd.Refresh()
				err := errors.New("username already exists")
				entryNewPad.SetValidationError(err)
				return err
			} else {
				btnAdd.Text = "Create"
				btnAdd.Enable()
				btnAdd.Refresh()
				return nil
			}
		} else {
			btnAdd.Text = "Create"
			btnAdd.Disable()
			err := errors.New("please enter a datapad name")
			entryNewPad.SetValidationError(err)
			btnAdd.Refresh()
			return err
		}
	}
	entryNewPad.OnChanged = func(s string) {
		entryNewPad.Validate()
	}
	entryNewPad.OnSubmitted = func(_ string) {
		if entryNewPad.Validate() == nil {
			btnAdd.OnTapped()
		}
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	frame := &iframe{}

	rectSpacer = canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 4))
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	var padData []string

	shard, err := GetShard()
	if err != nil {
		padData = []string{}
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		padData = []string{}
	}

	ss, err := store.LoadSnapshot(0)

	if err != nil {
		padData = []string{}
	}

	tree, err := ss.GetTree("Datapads")
	if err != nil {
		padData = []string{}
	}

	cursor := tree.Cursor()

	for k, _, err := cursor.First(); err == nil; k, _, err = cursor.Next() {
		if string(k) != "" {
			padData = append(padData, string(k))
		}
	}

	// Remove artificial height cap to allow list to expand in NewBorder center
	_ = rectListBox

	padList := binding.BindStringList(&padData)

	padBox := widget.NewListWithData(padList,
		func() fyne.CanvasObject {
			c := container.NewVBox(
				widget.NewLabel(""),
			)
			return c
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			co.(*fyne.Container).Objects[0].(*widget.Label).SetText(str)
			co.(*fyne.Container).Objects[0].(*widget.Label).Wrapping = fyne.TextWrapWord
			co.(*fyne.Container).Objects[0].(*widget.Label).TextStyle.Bold = false
			co.(*fyne.Container).Objects[0].(*widget.Label).Alignment = fyne.TextAlignLeading
		})

	padBox.OnSelected = func(id widget.ListItemID) {
		session.Datapad = padData[id]
		overlay := session.Window.Canvas().Overlays()
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)
		overlay.Add(
			container.NewStack(
				&iframe{},
				layoutPad(),
			),
		)
		overlay.Top().Show()
		padBox.UnselectAll()
		padBox.Refresh()
	}

	features := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			padBox,
		),
		layout.NewSpacer(),
	)

	subContainer := container.NewVBox(
		container.NewCenter(
			btnBack,
		),
		rectSpacer,
	)

	c := container.NewBorder(
		top,
		subContainer,
		nil,
		nil,
		features,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return layout
}

func layoutPad() fyne.CanvasObject {
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectEntry := canvas.NewRectangle(color.Transparent)
	rectEntry.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.52))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	heading := canvas.NewText(session.Datapad, colors.Gray)
	heading.TextStyle = fyne.TextStyle{Bold: true}
	heading.TextSize = scaleFont(16)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	selectOptions := widget.NewSelect([]string{"Clear", "Export (Plaintext)", "Import From File", "Delete"}, nil)
	selectOptions.PlaceHolder = "Select an Option ..."

	data, err := GetEncryptedValue("Datapads", []byte(session.Datapad))
	if err != nil {
		data = nil
	}

	overlay := session.Window.Canvas().Overlays()

	btnSave := widget.NewButton("Save", nil)

	entryPad := widget.NewEntry()
	entryPad.Wrapping = fyne.TextWrapWord

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	selectOptions.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()

		if s == "Clear" {
			header := canvas.NewText("SECURE NOTES  RESET  REQUESTED", colors.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText("Clear Datapad?", colors.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkClose := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkClose.OnTapped = func() {
				overlay := session.Window.Canvas().Overlays()
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
				selectOptions.Selected = "Select an Option ..."
				selectOptions.Refresh()
			}

			btnSubmit := widget.NewButton("Clear", nil)

			btnSubmit.OnTapped = func() {
				if session.Datapad != "" {
					err := StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(""))
					if err != nil {
						logger.Errorf("[Datapad] Err: %s\n", err)
						selectOptions.Selected = "Select an Option ..."
						selectOptions.Refresh()
						return
					}

					selectOptions.Selected = "Select an Option ..."
					selectOptions.Refresh()
					entryPad.Text = ""
					entryPad.Refresh()
				}

				errorText.Text = "datapad cleared"
				errorText.Color = colors.Green
				errorText.Refresh()

				overlay := session.Window.Canvas().Overlays()
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
				selectOptions.Selected = "Select an Option ..."
				selectOptions.Refresh()
			}

			span := canvas.NewRectangle(color.Transparent)
			span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)

			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewCenter(
						container.NewVBox(
							span,
							container.NewCenter(
								header,
							),
							rectSpacer,
							rectSpacer,
							subHeader,
							widget.NewLabel(""),
							wrapMobileButton(btnSubmit),
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								linkClose,
								layout.NewSpacer(),
							),
							rectSpacer,
							rectSpacer,
						),
					),
				),
			)
		} else if s == "Export (Plaintext)" {
			selectOptions.Selected = "Select an Option ..."
			selectOptions.Refresh()

			dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not export datapad"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if uri == nil {
					return // Canceled
				}

				data := []byte(entryPad.Text)
				_, err = writeToURI(data, uri)
				if err != nil {
					logger.Errorf("[Engram] Exporting datapad %s: %s\n", session.Datapad, err)
					errorText.Text = "error exporting datapad"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				errorText.Text = "exported datapad successfully"
				errorText.Color = colors.Green
				errorText.Refresh()

			}, session.Window)

			if !a.Driver().Device().IsMobile() {
				// Open file browser in current directory
				uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
				if err == nil {
					dialogFileSave.SetLocation(uri)
				} else {
					logger.Errorf("[Engram] Could not open current directory %s\n", err)
				}
			}

			// dialogFileSave.SetFilter(storage.NewMimeTypeFileFilter([]string{"text/*"}))
			dialogFileSave.SetView(dialog.ListView)
			dialogFileSave.SetFileName(fmt.Sprintf("%s.txt", session.Datapad))
			dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
			dialogFileSave.Show()
		} else if s == "Import From File" {
			selectOptions.Selected = "Select an Option ..."
			selectOptions.Refresh()

			dialogFileImport := dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not import file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if uri == nil {
					return // Canceled
				}

				fileName := uri.URI().String()
				if !strings.Contains(uri.URI().MimeType(), "text/") {
					logger.Errorf("[Engram] Cannot import file %s\n", fileName)
					errorText.Text = "cannot import file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if a.Driver().Device().IsMobile() {
					fileName = uri.URI().Name()
				} else {
					fileName = filepath.Base(strings.Replace(fileName, "file://", "", -1))
				}

				filedata, err := readFromURI(uri)
				if err != nil {
					logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", fileName, err)
					errorText.Text = "cannot read file data"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if !isASCII(string(filedata)) {
					errorText.Text = "invalid file data"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if entryPad.Text == "" {
					entryPad.SetText(string(filedata))
				} else {
					entryPad.SetText(fmt.Sprintf("%s\n\n%s", entryPad.Text, string(filedata)))
				}

				errorText.Text = "file data imported successfully"
				errorText.Color = colors.Green
				errorText.Refresh()

			}, session.Window)

			if !a.Driver().Device().IsMobile() {
				// Open file browser in current directory
				uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
				if err == nil {
					dialogFileImport.SetLocation(uri)
				} else {
					logger.Errorf("[Engram] Could not open current directory %s\n", err)
				}
			}

			// dialogFileSave.SetFilter(storage.NewMimeTypeFileFilter([]string{"text/*"}))
			dialogFileImport.SetView(dialog.ListView)
			dialogFileImport.Resize(fyne.NewSize(ui.Width, ui.Height))
			dialogFileImport.Show()
		} else if s == "Delete" {
			header := canvas.NewText("SECURE NOTES  DELETION  REQUESTED", colors.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText("Delete Datapad?", colors.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkClose := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkClose.OnTapped = func() {
				overlay := session.Window.Canvas().Overlays()
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
				selectOptions.Selected = "Select an Option ..."
				selectOptions.Refresh()
			}

			btnSubmit := widget.NewButton("Delete", nil)

			btnSubmit.OnTapped = func() {
				if session.Datapad != "" {
					err := DeleteKey("Datapads", []byte(session.Datapad))
					if err != nil {
						selectOptions.Selected = "Select an Option ..."
						selectOptions.Refresh()
						logger.Errorf("[Datapad] Error deleting %s: %s\n", session.Datapad, err)
					} else {
						session.Datapad = ""
						session.DatapadChanged = false
						removeOverlays()
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutDatapad())
					}
				}
			}

			span := canvas.NewRectangle(color.Transparent)
			span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)

			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewCenter(
						container.NewVBox(
							span,
							container.NewCenter(
								header,
							),
							rectSpacer,
							rectSpacer,
							subHeader,
							widget.NewLabel(""),
							wrapMobileButton(btnSubmit),
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								linkClose,
								layout.NewSpacer(),
							),
							rectSpacer,
							rectSpacer,
						),
					),
				),
			)
		} else {
			session.Datapad = ""
			session.DatapadChanged = false
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
			selectOptions.Selected = "Select an Option ..."
			selectOptions.Refresh()
		}
	}

	btnSave.OnTapped = func() {
		err = StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(entryPad.Text))
		if err != nil {
			btnSave.Disable()
			errorText.Text = "-  FAILED  -"
			errorText.Color = colors.Red
			errorText.Refresh()
		} else {
			session.DatapadChanged = false
			btnSave.Disable()
			heading.Text = session.Datapad
			heading.Refresh()
			errorText.Text = "-  SAVED  -"
			errorText.Color = colors.Green
			errorText.Refresh()
		}
	}

	session.DatapadChanged = false

	btnSave.Disable()

	entryPad.MultiLine = true
	entryPad.Text = string(data)
	entryPad.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		session.DatapadChanged = true
		heading.Text = session.Datapad + "*"
		heading.Refresh()
		btnSave.Enable()
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	linkBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		if session.DatapadChanged {
			header := canvas.NewText("SECURE NOTES  CHANGE  DETECTED", colors.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText("Save Datapad?", colors.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkClose := widget.NewHyperlinkWithStyle("Discard Changes", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkClose.OnTapped = func() {
				session.Datapad = ""
				session.DatapadChanged = false
				removeOverlays()
			}

			btnSubmit := widget.NewButton("Save", nil)

			btnSubmit.OnTapped = func() {
				err = StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(entryPad.Text))
				if err != nil {
					btnSave.Disable()
					errorText.Text = "error saving datapad"
					errorText.Color = colors.Red
					errorText.Refresh()
					overlay.Remove(overlay.Top())
					overlay.Remove(overlay.Top())
				} else {
					session.Datapad = ""
					session.DatapadChanged = false
					removeOverlays()
				}
			}

			span := canvas.NewRectangle(color.Transparent)
			span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)

			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewCenter(
						container.NewVBox(
							span,
							container.NewCenter(
								header,
							),
							rectSpacer,
							rectSpacer,
							subHeader,
							widget.NewLabel(""),
							wrapMobileButton(btnSubmit),
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								linkClose,
								layout.NewSpacer(),
							),
							rectSpacer,
							rectSpacer,
						),
					),
				),
			)
		} else {
			session.Datapad = ""
			session.DatapadChanged = false
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}
	})

	top = container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		container.NewCenter(
			container.NewHBox(
				wrapMobileButton(widget.NewButton("Clear", func() { selectOptions.OnChanged("Clear") })),
				wrapMobileButton(widget.NewButton("Export", func() { selectOptions.OnChanged("Export (Plaintext)") })),
				wrapMobileButton(widget.NewButton("Import", func() { selectOptions.OnChanged("Import From File") })),
				wrapMobileButton(widget.NewButton("Delete", func() { selectOptions.OnChanged("Delete") })),
			),
		),
		rectSpacer,
	)

	center := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			container.NewVScroll(
				container.NewVBox(
					container.NewStack(
						rectEntry,
						entryPad,
					),
					rectSpacer,
					errorText,
					rectSpacer,
					wrapMobileButton(btnSave),
					rectSpacer,
				),
			),
		),
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), linkBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		center,
	)

	return layout
}

func layoutAccount() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))
	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	title := canvas.NewText("M Y   A C C O U N T", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			title,
		),
		rectSpacer,
		rectSpacer,
	)

	addressStr := engram.Disk.GetAddress().String()
	heading := canvas.NewText("", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	var addressToggleBtn *widget.Button
	addressToggleBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		session.AddressHidden = !session.AddressHidden
		if session.AddressHidden {
			heading.Text = "dE...••••••••"
			addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
			StoreEncryptedValue("settings", []byte("AddressHidden"), []byte("true"))
		} else {
			heading.Text = addressStr[0:5] + "..." + addressStr[len(addressStr)-10:]
			addressToggleBtn.SetIcon(theme.VisibilityIcon())
			StoreEncryptedValue("settings", []byte("AddressHidden"), []byte("false"))
		}
		heading.Refresh()
	})
	addressToggleBtn.Importance = widget.LowImportance

	addressCopyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(engram.Disk.GetAddress().String())
	})
	addressCopyBtn.Importance = widget.LowImportance

	if session.AddressHidden {
		heading.Text = "dE...••••••••"
		addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
	} else {
		heading.Text = addressStr[0:5] + "..." + addressStr[len(addressStr)-10:]
		addressToggleBtn.SetIcon(theme.VisibilityIcon())
	}

	labelPassword := canvas.NewText("N E W    P A S S W O R D", colors.Gray)
	labelPassword.TextStyle = fyne.TextStyle{Bold: true}
	labelPassword.TextSize = scaleFont(11)
	labelPassword.Alignment = fyne.TextAlignCenter

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	linkIdentity := widget.NewHyperlinkWithStyle("Identity Settings", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkIdentity.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutIdentity())
		removeOverlays()
	}

	linkServiceAddress := widget.NewHyperlinkWithStyle("Payment Request", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkServiceAddress.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutServiceAddress())
		removeOverlays()
	}

	btnIdentity := newSmallIconButton("Identity", theme.AccountIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutIdentity())
		removeOverlays()
	})

	btnPayment := newSmallIconButton("Payment", theme.ComputerIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutServiceAddress())
		removeOverlays()
	})

	buttonsRow := container.NewHBox(
		layout.NewSpacer(),
		btnPayment,
		rectSpacer,
		btnIdentity,
		layout.NewSpacer(),
	)

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	// Recovery Words Link
	linkRecoveryWords := widget.NewHyperlinkWithStyle("Recovery Words", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRecoveryWords.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		overlay := session.Window.Canvas().Overlays()

		header := canvas.NewText("ACCOUNT  VERIFICATION  REQUIRED", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Confirm Password", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnConfirm := widget.NewButton("Submit", nil)
		btnConfirm.Disable()

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = "Password"
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnConfirm.Text = "Submit"
				btnConfirm.Disable()
				btnConfirm.Refresh()
			} else {
				btnConfirm.Text = "Submit"
				btnConfirm.Enable()
				btnConfirm.Refresh()
			}
		}

		btnConfirm.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)
				overlay.Add(layoutRecovery())
			} else {
				btnConfirm.Text = "Invalid Password..."
				btnConfirm.Disable()
				btnConfirm.Refresh()
			}
		}

		entryPassword.OnReturn = btnConfirm.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		top := container.NewVBox(
			rectSpacer,
			rectSpacer,
			container.NewCenter(header),
			rectSpacer,
			rectSpacer,
		)

		center := container.NewCenter(
			container.NewVBox(
				subHeader,
				widget.NewLabel(""),
				container.NewCenter(
					container.NewStack(
						span,
						entryPassword,
					),
				),
				rectSpacer,
				rectSpacer,
				wrapMobileButton(btnConfirm),
			),
		)

		bottom := container.NewStack(
			container.NewVBox(
				rectSpacer,
				container.NewCenter(
					container.New(layout.NewGridLayoutWithColumns(1), btnBack),
				),
				rectSpacer,
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewBorder(
					top,
					bottom,
					nil,
					nil,
					center,
				),
			),
		)

		safeCanvasFocus(entryPassword)
	}

	// Recovery Hex Keys Link
	linkRecoveryHex := widget.NewHyperlinkWithStyle("Recovery Hex Keys", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRecoveryHex.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		overlay := session.Window.Canvas().Overlays()

		header := canvas.NewText("ACCOUNT  VERIFICATION  REQUIRED", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Confirm Password", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnConfirm := widget.NewButton("Submit", nil)
		btnConfirm.Disable()

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = "Password"
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnConfirm.Text = "Submit"
				btnConfirm.Disable()
				btnConfirm.Refresh()
			} else {
				btnConfirm.Text = "Submit"
				btnConfirm.Enable()
				btnConfirm.Refresh()
			}
		}

		btnConfirm.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)
				overlay.Add(layoutRecoveryHex())
			} else {
				btnConfirm.Text = "Invalid Password..."
				btnConfirm.Disable()
				btnConfirm.Refresh()
			}
		}

		entryPassword.OnReturn = btnConfirm.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		top := container.NewVBox(
			rectSpacer,
			rectSpacer,
			container.NewCenter(header),
			rectSpacer,
			rectSpacer,
		)

		center := container.NewCenter(
			container.NewVBox(
				subHeader,
				widget.NewLabel(""),
				container.NewCenter(
					container.NewStack(
						span,
						entryPassword,
					),
				),
				rectSpacer,
				rectSpacer,
				wrapMobileButton(btnConfirm),
			),
		)

		bottom := container.NewStack(
			container.NewVBox(
				rectSpacer,
				container.NewCenter(
					container.New(layout.NewGridLayoutWithColumns(1), btnBack),
				),
				rectSpacer,
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewBorder(
					top,
					bottom,
					nil,
					nil,
					center,
				),
			),
		)

		safeCanvasFocus(entryPassword)
	}

	// Change Password Link
	linkChangePassword := widget.NewHyperlinkWithStyle("Change Password", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkChangePassword.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		overlay := session.Window.Canvas().Overlays()

		header := canvas.NewText("ACCOUNT  AUTHORIZATION  REQUEST", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Change Password", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnChange := widget.NewButton("Submit", nil)
		btnChange.Disable()

		curPass := widget.NewEntry()
		curPass.Password = true
		curPass.PlaceHolder = "Current Password"
		curPass.OnChanged = func(s string) {
			btnChange.Text = "Submit"
			btnChange.Enable()
			btnChange.Refresh()
		}

		newPass := widget.NewEntry()
		newPass.Password = true
		newPass.PlaceHolder = "New Password"
		newPass.OnChanged = func(s string) {
			btnChange.Text = "Submit"
			btnChange.Enable()
			btnChange.Refresh()
		}

		confirm := widget.NewEntry()
		confirm.Password = true
		confirm.PlaceHolder = "Confirm Password"
		confirm.OnChanged = func(s string) {
			btnChange.Text = "Submit"
			btnChange.Enable()
			btnChange.Refresh()
		}

		btnChange.OnTapped = func() {
			if engram.Disk.Check_Password(curPass.Text) {
				if newPass.Text == confirm.Text && newPass.Text != "" {
					err := engram.Disk.Set_Encrypted_Wallet_Password(newPass.Text)
					if err != nil {
						btnChange.Text = "Error changing password"
						btnChange.Disable()
						btnChange.Refresh()
					} else {
						curPass.Text = ""
						curPass.Refresh()
						newPass.Text = ""
						newPass.Refresh()
						confirm.Text = ""
						confirm.Refresh()
						btnChange.Text = "Password Updated"
						btnChange.Disable()
						btnChange.Refresh()
						if err := engram.Disk.Save_Wallet(); err != nil {
							logger.Errorf("[Settings] Failed to save wallet after password change: %s\n", err)
						}
					}
				} else {
					btnChange.Text = "Passwords do not match"
					btnChange.Disable()
					btnChange.Refresh()
				}
			} else {
				btnChange.Text = "Incorrect password entered"
				btnChange.Disable()
				btnChange.Refresh()
			}
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		top := container.NewVBox(
			rectSpacer,
			rectSpacer,
			container.NewCenter(header),
			rectSpacer,
			rectSpacer,
		)

		center := container.NewCenter(
			container.NewVBox(
				subHeader,
				widget.NewLabel(""),
				container.NewCenter(
					container.NewStack(
						span,
						curPass,
					),
				),
				widget.NewLabel(""),
				widget.NewSeparator(),
				widget.NewLabel(""),
				newPass,
				rectSpacer,
				confirm,
				rectSpacer,
				rectSpacer,
				wrapMobileButton(btnChange),
			),
		)

		bottom := container.NewStack(
			container.NewVBox(
				rectSpacer,
				container.NewCenter(
					container.New(layout.NewGridLayoutWithColumns(1), btnBack),
				),
				rectSpacer,
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewBorder(
					top,
					bottom,
					nil,
					nil,
					center,
				),
			),
		)
	}

	// Export Wallet Link
	linkExportWallet := widget.NewHyperlinkWithStyle("Export Wallet", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkExportWallet.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		verificationOverlay(
			true,
			"",
			"",
			"",
			func(b bool) {
				if b {
					go func() {
						dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
							if err != nil {
								logger.Errorf("[Engram] File dialog: %s\n", err)
								fyne.Do(func() {
									errorText.Text = "could not export wallet file"
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								return
							}

							if uri == nil {
								return // Canceled
							}

							data, err := os.ReadFile(session.Path)
							if err != nil {
								logger.Errorf("[Engram] Reading wallet file %s: %s\n", session.Path, err)
								fyne.Do(func() {
									errorText.Text = "error reading wallet file"
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								return
							}

							_, err = writeToURI(data, uri)
							if err != nil {
								logger.Errorf("[Engram] Exporting %s: %s\n", session.Path, err)
								fyne.Do(func() {
									errorText.Text = "error exporting wallet file"
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								return
							}

							fyne.Do(func() {
								errorText.Text = "exported wallet file successfully"
								errorText.Color = colors.Green
								errorText.Refresh()
							})
						}, session.Window)

						if !a.Driver().Device().IsMobile() {
							// Open file browser in current directory
							uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
							if err == nil {
								dialogFileSave.SetLocation(uri)
							} else {
								logger.Errorf("[Engram] Could not open current directory %s\n", err)
							}
						}

						fyne.Do(func() {
							dialogFileSave.SetFilter(storage.NewExtensionFileFilter([]string{".db"}))
							dialogFileSave.SetView(dialog.ListView)
							dialogFileSave.SetFileName(filepath.Base(session.Path))
							dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
							dialogFileSave.Show()
						})
					}()
				}
			},
		)
	}

	var imageQR *canvas.Image

	qr, err := qrcode.New(engram.Disk.GetAddress().String(), qrcode.Highest)
	if err != nil {

	} else {
		qr.BackgroundColor = colors.DarkMatter
		qr.ForegroundColor = colors.Green
	}

	imageQR = canvas.NewImageFromImage(qr.Image(int(ui.Width * 0.65)))
	imageQR.SetMinSize(fyne.NewSize(ui.Width*0.65, ui.Width*0.65))

	features := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewVBox(
				rectSpacer,
				rectSpacer,

				rectSpacer,
				container.NewCenter(
					container.NewHBox(
						heading,
						addressToggleBtn,
						addressCopyBtn,
					),
				),
				rectSpacer,
				buttonsRow,
				rectSpacer,
				container.NewStack(
					container.NewCenter(
						imageQR,
					),
				),
				container.NewStack(
					container.NewHBox(
						layout.NewSpacer(),
						container.NewVBox(
							linkRecoveryWords,
							linkRecoveryHex,
							linkChangePassword,
							linkExportWallet,
							errorText,
						),
						layout.NewSpacer(),
					),
				),
			),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		features,
	)

	return layout
}

func layoutRecovery() fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")
	heading := canvas.NewText("Recovery Words", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	btnCancel := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
	})

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	grid := container.NewVBox()
	grid.Objects = nil

	header := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
		rectSpacer,
		rectSpacer,
	)

	footer := container.NewVBox(
		wSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnCancel,
			layout.NewSpacer(),
		),
		wSpacer,
	)

	body := widget.NewLabel("Please save the following 25 recovery words in a safe place. Never share them with anyone.")
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	btnCopySeed := widget.NewButton("Copy Recovery Words", nil)

	form := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectHeader,
				body,
			),
			layout.NewSpacer(),
		),
		wSpacer,
		container.NewCenter(grid),
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectHeader,
				wrapMobileButton(btnCopySeed),
			),
			layout.NewSpacer(),
		),
		rectSpacer,
	)

	scrollBox := container.NewVScroll(
		container.NewStack(
			form,
		),
	)
	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	formatted := strings.Split(engram.Disk.GetSeed(), " ")

	rect := canvas.NewRectangle(color.RGBA{19, 25, 34, 255})
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(25)))

	for i := 0; i < len(formatted); i++ {
		pos := fmt.Sprintf("%d", i+1)
		word := strings.ReplaceAll(formatted[i], " ", "")
		grid.Add(container.NewStack(
			rect,
			container.NewHBox(
				widget.NewLabel(" "),
				widget.NewLabelWithStyle(pos, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				layout.NewSpacer(),
				widget.NewLabelWithStyle(word, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabel(" "),
			),
		),
		)
	}

	btnCopySeed.OnTapped = func() {
		a.Clipboard().SetContent(engram.Disk.GetSeed())
	}

	layout := container.NewStack(
		&iframe{},
		container.NewVBox(
			header,
			scrollBox,
			footer,
		),
	)

	return layout
}

func layoutRecoveryHex() fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")
	heading := canvas.NewText("Recovery Hex Keys", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	btnCancel := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
	})

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	grid := container.NewVBox()
	grid.Objects = nil

	header := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
		rectSpacer,
		rectSpacer,
	)

	footer := container.NewVBox(
		wSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnCancel,
			layout.NewSpacer(),
		),
		wSpacer,
	)

	body := widget.NewLabel("Please save the following hex secret key in a safe place. Never share your secret key with anyone.")
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	form := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectHeader,
				body,
			),
			layout.NewSpacer(),
		),
		wSpacer,
		container.NewCenter(grid),
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectHeader,
			),
			layout.NewSpacer(),
		),
		rectSpacer,
	)

	scrollBox := container.NewVScroll(
		container.NewStack(
			form,
		),
	)
	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	keys := engram.Disk.Get_Keys()
	key := fmt.Sprintf("0000000000000000000000000000000000000000000000%s", keys.Secret.Text(16))
	secret := key[len(key)-64:]
	public := keys.Public.StringHex()

	textSecret := widget.NewRichTextFromMarkdown(secret)
	textSecret.Wrapping = fyne.TextWrapWord

	linkCopySecret := widget.NewHyperlinkWithStyle("Copy Secret Key", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	textPublic := widget.NewRichTextFromMarkdown(public)
	textPublic.Wrapping = fyne.TextWrapWord

	linkCopyPublic := widget.NewHyperlinkWithStyle("Copy Public Key", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	labelSecret := canvas.NewText("   SECRET  KEY", colors.Gray)
	labelSecret.TextSize = scaleFont(14)
	labelSecret.Alignment = fyne.TextAlignLeading
	labelSecret.TextStyle = fyne.TextStyle{Bold: true}

	labelPublic := canvas.NewText("   PUBLIC  KEY", colors.Gray)
	labelPublic.TextSize = scaleFont(14)
	labelPublic.Alignment = fyne.TextAlignLeading
	labelPublic.TextStyle = fyne.TextStyle{Bold: true}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	grid.Add(container.NewVBox(
		labelSecret,
		rectSpacer,
		textSecret,
		rectSpacer,
		container.NewHBox(
			linkCopySecret,
		),
		rectSpacer,
		rectSpacer,
		labelSeparator,
		rectSpacer,
		rectSpacer,
		rectSpacer,
	))

	grid.Add(container.NewVBox(
		labelPublic,
		rectSpacer,
		textPublic,
		rectSpacer,
		container.NewHBox(
			linkCopyPublic,
		),
	))

	linkCopySecret.OnTapped = func() {
		a.Clipboard().SetContent(secret)
	}

	linkCopyPublic.OnTapped = func() {
		a.Clipboard().SetContent(public)
	}

	layout := container.NewStack(
		&iframe{},
		container.NewVBox(
			header,
			scrollBox,
			footer,
		),
	)

	return layout
}

func layoutFrame() fyne.CanvasObject {
	return layoutFrameWithWallet("")
}

func layoutFrameWithWallet(singleWalletName string) fyne.CanvasObject {
	entry := widget.NewEntry()
	layout := container.NewStack(entry)

	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	session.Window.SetContent(layout)
	session.Window.SetFixedSize(false)

	go func() {
		time.Sleep(time.Second * 2)
		removeOverlays()

		ui.MaxWidth = entry.Size().Width
		ui.MaxHeight = entry.Size().Height
		lastOrientation := a.Driver().Device().Orientation()
		initialOrientationVertical := fyne.IsVertical(lastOrientation)

		ui.Width = ui.MaxWidth * 0.9
		ui.Height = ui.MaxHeight
		ui.Padding = ui.MaxWidth * 0.05
		if fyne.IsHorizontal(lastOrientation) {
			ui.MaxWidth = ui.MaxWidth * 0.7
			ui.Width = ui.MaxWidth * 0.7
			ui.Padding = ui.MaxWidth * 0.15
		}

		resizeWindow(ui.MaxWidth, ui.MaxHeight)
		session.Window.SetContent(layoutTransition())

		session.Window.SetContent(layoutMain())

		frameWidth := ui.MaxWidth
		frameHeight := ui.MaxHeight

		for a.Driver() != nil {
			currentOrientation := a.Driver().Device().Orientation()
			if lastOrientation != currentOrientation {
				if initialOrientationVertical {
					if fyne.IsVertical(lastOrientation) && !fyne.IsVertical(currentOrientation) {
						ui.MaxWidth = frameHeight
						ui.MaxHeight = frameWidth
					} else {
						ui.MaxWidth = frameWidth
						ui.MaxHeight = frameHeight
					}
				} else {
					if fyne.IsHorizontal(lastOrientation) && !fyne.IsHorizontal(currentOrientation) {
						ui.MaxWidth = frameHeight
						ui.MaxHeight = frameWidth
					} else {
						ui.MaxWidth = frameWidth
						ui.MaxHeight = frameHeight
					}
				}

				ui.Width = ui.MaxWidth * 0.9
				ui.Height = ui.MaxHeight
				ui.Padding = ui.MaxWidth * 0.05
				if fyne.IsHorizontal(currentOrientation) {
					ui.MaxWidth = ui.MaxWidth * 0.7
					ui.Width = ui.MaxWidth * 0.7
					ui.Padding = ui.MaxWidth * 0.15
				}

				lastOrientation = currentOrientation
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
			}
			time.Sleep(time.Second)
		}
	}()

	overlays := session.Window.Canvas().Overlays()
	overlays.Add(
		container.NewStack(
			canvas.NewRectangle(colors.DarkMatter),
		),
	)

	return container.NewVScroll(layout)
}

func layoutFileManager() fyne.CanvasObject {
	session.Domain = "app.sign"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.34))
	rectWidth100 := canvas.NewRectangle(color.Transparent)
	rectWidth100.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.9, 10))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	heading := canvas.NewText("F I L E    M A N A G E R", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelResults := canvas.NewText("   RESULTS", colors.Gray)
	labelResults.TextSize = scaleFont(14)
	labelResults.Alignment = fyne.TextAlignLeading
	labelResults.TextStyle = fyne.TextStyle{Bold: true}

	signedResults := []string{}
	signedData := binding.BindStringList(&signedResults)
	signedList := widget.NewListWithData(signedData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectWidth90,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, "/")
			pos := len(split) - 1
			name := strings.Split(split[pos], ";;;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(name[0])
		},
	)

	verifiedResults := []string{}
	verifiedData := binding.BindStringList(&verifiedResults)
	verifiedList := widget.NewListWithData(verifiedData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectWidth90,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, "/")
			pos := len(split) - 1
			name := strings.Split(split[pos], ";;;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(name[0])
		},
	)

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	dialogBrowse := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			logger.Errorf("[Engram] Open file dialog: %s\n", err)
			errorText.Text = "could not open file"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		if uc == nil {
			return
		}

		if session.Domain == "app.sign" {
			inputFileName := uc.URI().Name()
			outputFileName := inputFileName + ".signed"

			go func() {
				dialogFileSign := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
					if err != nil {
						logger.Errorf("[Engram] Save file dialog: %s\n", err)
						fyne.Do(func() {
							errorText.Text = "could not open signed file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})

						return
					}

					if uri == nil {
						return // Canceled
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						fyne.Do(func() {
							errorText.Text = "could not read file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})

						return
					}

					_, err = writeToURI(engram.Disk.SignData(filedata), uri)
					if err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						fyne.Do(func() {
							errorText.Text = "could not write signed file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})

						return
					}

					outputFile := uri.URI().Name()
					if a.Driver().Device().IsMobile() {
						// Mobile uses content access name on save dialog
						outputFile = outputFileName
					}

					logger.Printf("[Engram] Successfully signed file: %s\n", outputFile)

					fyne.Do(func() {
						errorText.Text = "signed file successfully"
						errorText.Color = colors.Green
						errorText.Refresh()

						signedResults = append(signedResults, outputFile)
						signedData.Set(signedResults)
						signedList.Refresh()

						signedLen := len(signedResults)
						labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", signedLen, signedLen)
						labelResults.Refresh()
					})

				}, session.Window)

				if !a.Driver().Device().IsMobile() {
					// Open file browser in current directory
					uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
					if err == nil {
						dialogFileSign.SetLocation(uri)
					} else {
						logger.Errorf("[Engram] Could not open current directory %s\n", err)
					}
				}

				fyne.Do(func() {
					dialogFileSign.SetFilter(storage.NewExtensionFileFilter([]string{".signed"}))
					dialogFileSign.SetView(dialog.ListView)
					dialogFileSign.SetFileName(outputFileName)
					dialogFileSign.Resize(fyne.NewSize(ui.Width, ui.Height))
					dialogFileSign.SetConfirmText("Save Sign")
					dialogFileSign.Show()
				})
			}()
		} else {
			fileName := uc.URI().Name()
			if !strings.HasSuffix(fileName, ".signed") {
				errorText.Text = "verifying requires a .signed file"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", fileName, err)
				errorText.Text = "could not read file"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			// Trim off .signed from file because engram.Disk.CheckFileSignature() adds it back on anyways - https://github.com/deroproject/derohe/blob/main/walletapi/wallet.go#L709
			fileName = strings.TrimSuffix(fileName, ".signed")
			signer, message, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
				errorText.Text = "signature verification failed"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())
			if isASCII(string(message)) {
				fmt.Println(string(message))
			}

			errorText.Text = "verified file successfully"
			errorText.Color = colors.Green
			errorText.Refresh()

			verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", verifiedLen, verifiedLen)
			labelResults.Refresh()
		}
	}, session.Window)

	if !a.Driver().Device().IsMobile() {
		// Open file browser in current directory
		uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
		if err == nil {
			dialogBrowse.SetLocation(uri)
		} else {
			logger.Errorf("[Engram] Could not open current directory %s\n", err)
		}
	}

	dialogBrowse.Resize(fyne.NewSize(ui.Width, ui.Height))
	dialogBrowse.SetView(dialog.ListView)

	signedList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
	}

	verifiedList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()

		if session.Domain == "app.verify" {
			split := strings.Split(verifiedResults[id], ";;;")
			filepath := strings.Split(split[0], "/")
			filename := filepath[len(filepath)-1]
			filename = strings.Replace(filename, ".signed", "", -1)

			rectSpan := canvas.NewRectangle(color.Transparent)
			rectSpan.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

			header := canvas.NewText("S I G N A T U R E    D E T A I L", colors.Gray)
			header.TextSize = scaleFont(16)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			labelStatus := canvas.NewText("   VERIFICATION   STATUS", colors.Gray)
			labelStatus.TextSize = scaleFont(12)
			labelStatus.TextStyle = fyne.TextStyle{Bold: true}
			labelStatus.Alignment = fyne.TextAlignCenter

			valueStatus := canvas.NewText("   Verified", colors.Green)
			valueStatus.TextSize = scaleFont(22)
			valueStatus.TextStyle = fyne.TextStyle{Bold: true}
			valueStatus.Alignment = fyne.TextAlignCenter

			labelFilename := canvas.NewText("   FILENAME", colors.Gray)
			labelFilename.TextSize = scaleFont(14)
			labelFilename.TextStyle = fyne.TextStyle{Bold: true}

			valueFilename := widget.NewRichTextFromMarkdown(filename)
			valueFilename.Wrapping = fyne.TextWrapBreak

			labelSigner := canvas.NewText("   SIGNER   ADDRESS", colors.Gray)
			labelSigner.TextSize = scaleFont(14)
			labelSigner.TextStyle = fyne.TextStyle{Bold: true}

			valueSigner := widget.NewRichTextFromMarkdown(split[1])
			valueSigner.Wrapping = fyne.TextWrapBreak

			labelSeparator := widget.NewRichTextFromMarkdown("")
			labelSeparator.Wrapping = fyne.TextWrapOff
			labelSeparator.ParseMarkdown("---")

			labelSeparator2 := widget.NewRichTextFromMarkdown("")
			labelSeparator2.Wrapping = fyne.TextWrapOff
			labelSeparator2.ParseMarkdown("---")

			labelSeparator3 := widget.NewRichTextFromMarkdown("")
			labelSeparator3.Wrapping = fyne.TextWrapOff
			labelSeparator3.ParseMarkdown("---")

			linkBack := widget.NewHyperlinkWithStyle("Hide Details", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkBack.OnTapped = func() {
				removeOverlays()
			}

			overlay := session.Window.Canvas().Overlays()
			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)
			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewHBox(
						layout.NewSpacer(),
						container.NewVBox(
							rectSpan,
							rectSpacer,
							header,
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								container.NewVBox(
									valueStatus,
									rectSpacer,
									labelStatus,
								),
								layout.NewSpacer(),
							),
							rectSpacer,
							rectSpacer,
							labelSeparator,
							rectSpacer,
							rectSpacer,
							labelFilename,
							rectSpacer,
							valueFilename,
							rectSpacer,
							rectSpacer,
							labelSeparator2,
							rectSpacer,
							rectSpacer,
							labelSigner,
							rectSpacer,
							valueSigner,
							rectSpacer,
							rectSpacer,
							labelSeparator3,
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								linkBack,
								layout.NewSpacer(),
							),
						),
						layout.NewSpacer(),
					),
				),
			)
			overlay.Top().Show()

			verifiedList.UnselectAll()
		}
	}

	btnBrowse := widget.NewButton("Browse Files", nil)
	btnBrowse.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		if session.Domain == "app.sign" {
			dialogBrowse.SetFilter(nil)
			dialogBrowse.SetConfirmText("Open")
		} else {
			dialogBrowse.SetFilter(storage.NewExtensionFileFilter([]string{".signed"}))
			dialogBrowse.SetConfirmText("Verify")
		}

		dialogBrowse.Show()
	}

	labelAction := canvas.NewText("( DRAG-AND-DROP ENABLED )", colors.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

	entryAddress := widget.NewEntry()
	entryAddress.PlaceHolder = "Username or Address"

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")

	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")

	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		session.Domain = "app.wallet"
		session.LastDomain = capture
	})

	selectType := widget.NewSelect([]string{"Sign Files", "Verify Signed Files"}, nil)
	selectType.SetSelected("Sign Files")

	// Handle drag & drop files for file signing/verifying
	session.Window.SetOnDropped(func(p fyne.Position, files []fyne.URI) {
		errorText.Text = ""
		errorText.Refresh()

		if session.Domain == "app.sign" {
			if a.Driver().Device().IsMobile() {
				if len(files) > 1 {
					errorText.Text = "single file only"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				inputFileName := files[0].Name()

				dialogFileSign := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
					if err != nil {
						logger.Errorf("[Engram] File dialog: %s\n", err)
						uiDo(func() {
							errorText.Text = "could not open signed file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})
						return
					}

					if uri == nil {
						return // Canceled
					}

					uc, err := storage.Reader(files[0])
					if err != nil {
						logger.Errorf("[Engram] Cannot create reader for %s: %s\n", inputFileName, err)
						uiDo(func() {
							errorText.Text = "could not access file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})
						return
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						uiDo(func() {
							errorText.Text = "could not read file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})
						return
					}

					_, err = writeToURI(engram.Disk.SignData(filedata), uri)
					if err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						uiDo(func() {
							errorText.Text = "could not write signed file"
							errorText.Color = colors.Red
							errorText.Refresh()
						})
						return
					}

					// Mobile uses content access name on save dialog
					outputFile := inputFileName + ".signed"

					logger.Printf("[Engram] Successfully signed file: %s\n", outputFile)

					uiDo(func() {
						errorText.Text = "signed file successfully"
						errorText.Color = colors.Green
						errorText.Refresh()

						signedResults = append(signedResults, outputFile)
						_ = signedData.Set(signedResults)
						signedList.Refresh()

						signedLen := len(signedResults)
						labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", signedLen, signedLen)
						labelResults.Refresh()
					})

				}, session.Window)

				dialogFileSign.SetFilter(storage.NewExtensionFileFilter([]string{".signed"}))
				dialogFileSign.SetView(dialog.ListView)
				dialogFileSign.SetFileName(inputFileName)
				dialogFileSign.Resize(fyne.NewSize(ui.Width, ui.Height))
				dialogFileSign.SetConfirmText("Save Sign")
				dialogFileSign.Show()
			} else {
				singedLen := len(signedResults)
				count := 1 + singedLen

				for i, f := range files {
					inputFileName := f.Name()

					uc, err := storage.Reader(f)
					if err != nil {
						logger.Errorf("[Engram] Cannot create reader for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not access file %d", i)
						errorText.Color = colors.Red
						errorText.Refresh()
						continue
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not read file %d", i)
						errorText.Color = colors.Red
						errorText.Refresh()
						continue
					}

					outputfile := inputFileName + ".signed"

					if err := os.WriteFile(outputfile, engram.Disk.SignData(filedata), 0600); err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("cannot sign file %d", i)
						errorText.Color = colors.Red
						errorText.Refresh()
					} else {
						logger.Printf("[Engram] Successfully signed file: %s\n", outputfile)
						labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", count, len(files)+singedLen)
						labelResults.Refresh()
						signedResults = append(signedResults, outputfile)
						count += 1
					}
				}

				signedData.Set(signedResults)
				signedList.Refresh()
			}
		} else if session.Domain == "app.verify" {
			if a.Driver().Device().IsMobile() {
				dialogVerify := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
					errorText.Text = ""
					if uc != nil {
						fileName := uc.URI().Name()
						if filepath.Ext(fileName) != ".signed" {
							errorText.Text = "requires a .signed file"
							errorText.Color = colors.Red
							errorText.Refresh()
							return
						}

						filedata, err := readFromURI(uc)
						if err != nil {
							logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", fileName, err)
							errorText.Text = "cannot read file data"
							errorText.Color = colors.Red
							errorText.Refresh()
							return
						}

						signer, message, err := engram.Disk.CheckSignature(filedata)
						if err != nil {
							logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
							errorText.Text = "signature verification failed"
							errorText.Color = colors.Red
							errorText.Refresh()
							return
						}

						logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())
						if isASCII(string(message)) {
							fmt.Println(string(message))
						}

						errorText.Text = "verified file successfully"
						errorText.Color = colors.Green
						errorText.Refresh()

						verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())
						_ = verifiedData.Set(verifiedResults)
						verifiedList.Refresh()

						verifiedLen := len(verifiedResults)
						labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", verifiedLen, verifiedLen)
						labelResults.Refresh()
					}
				}, session.Window)

				dialogVerify.Resize(fyne.NewSize(ui.Width, ui.Height))
				dialogVerify.SetView(dialog.ListView)
				dialogVerify.Show()
			} else {
				verifiedLen := len(verifiedResults)
				count := 1 + verifiedLen

				for i, f := range files {
					inputFileName := f.Name()

					uc, err := storage.Reader(f)
					if err != nil {
						logger.Errorf("[Engram] Cannot create reader for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not access file %d", i)
						errorText.Color = colors.Red
						errorText.Refresh()
						continue
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not read file %d", i)
						errorText.Color = colors.Red
						errorText.Refresh()
						continue
					}

					outputfile := strings.TrimSuffix(inputFileName, ".signed")

					if signer, message, err := engram.Disk.CheckSignature(filedata); err != nil {
						logger.Errorf("[Engram] Signature verification failed for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("signature verification %d failed", i)
						errorText.Color = colors.Red
						errorText.Refresh()
					} else {
						logger.Printf("[Engram] Signed by: %s\n", signer.String())

						if isASCII(string(message)) {
							logger.Printf("[Engram] Message for %s: %s\n", inputFileName, signer.String())
						}

						if err := os.WriteFile(outputfile, message, 0600); err != nil {
							logger.Errorf("[Engram] Cannot write output file for %s: %s\n", outputfile, err)
							continue
						}

						logger.Printf("[Engram] Successfully wrote message to file: %s\n", outputfile)

						labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", count, len(files)+verifiedLen)
						labelResults.Refresh()
						verifiedResults = append(verifiedResults, inputFileName+";;;"+signer.String())
						count += 1
					}
				}

				verifiedData.Set(verifiedResults)
				verifiedList.Refresh()
			}
		}
	})

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
	)

	center := container.NewStack(
		rectWidth100,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					rectSpacer,
					rectSpacer,
					selectType,
					rectSpacer,
					rectSpacer,
					btnBrowse,
					rectSpacer,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						labelAction,
						layout.NewSpacer(),
					),
					rectSpacer,
					errorText,
					rectSpacer,
					labelSeparator,
					rectSpacer,
					rectSpacer,
					labelResults,
					rectSpacer,
					rectSpacer,
					container.NewStack(
						rectBox,
						signedList,
					),
					rectSpacer,
				),
			),
			layout.NewSpacer(),
		),
	)

	selectType.OnChanged = func(s string) {
		if s == "Sign Files" {
			session.Domain = "app.sign"
			signedList.UnselectAll()
			center.Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[18].(*fyne.Container).Objects[1] = signedList
			signedData.Set(signedResults)
			signedList.Refresh()
			signedLen := len(signedResults)
			labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", signedLen, signedLen)
			labelResults.Refresh()
		} else {
			session.Domain = "app.verify"
			center.Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[18].(*fyne.Container).Objects[1] = verifiedList
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()
			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", verifiedLen, verifiedLen)
			labelResults.Refresh()
		}

		errorText.Text = ""
		errorText.Refresh()
	}

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	body := container.NewVBox(
		top,
		center,
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			body,
			bottom,
			nil,
			nil,
		),
	)

	return NewVScroll(layout)
}

func layoutContractBuilder(promptText string) fyne.CanvasObject {
	session.Domain = "app.sc.builder"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.35))

	rectWidth100 := canvas.NewRectangle(color.Transparent)
	rectWidth100.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.9, 10))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	heading := canvas.NewText("C O N T R A C T    B U I L D E R", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	errorText := canvas.NewText(promptText, colors.Red)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	// Open .bas SC from file browser
	dialogBrowse := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		errorText.Text = ""
		if uc != nil {
			filename := uc.URI().Name()
			if uc.URI().MimeType() != "text/plain" {
				logger.Errorf("[Engram] Cannot open file %s in contract builder\n", filename)
				errorText.Text = "cannot open file"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if filepath.Ext(filename) != ".bas" {
				errorText.Text = "requires a .bas file"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", filename, err)
				errorText.Text = "cannot read file data"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if !isASCII(string(filedata)) {
				errorText.Text = "invalid file data"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			removeOverlays()
			capture := session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutContractEditor(strings.TrimSuffix(filename, ".bas"), string(filedata)))
			session.LastDomain = capture
		}
	}, session.Window)

	if !a.Driver().Device().IsMobile() {
		// Open file browser in current directory
		uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
		if err == nil {
			dialogBrowse.SetLocation(uri)
		} else {
			logger.Errorf("[Engram] Could not open current directory %s\n", err)
		}
	}

	// Resize browser to app size and add SC file filter
	dialogBrowse.Resize(fyne.NewSize(ui.Width, ui.Height))
	dialogBrowse.SetFilter(storage.NewExtensionFileFilter([]string{".bas"}))
	dialogBrowse.SetView(dialog.ListView)

	btnBrowse := widget.NewButton("Browse Files", nil)
	btnBrowse.OnTapped = func() {
		dialogBrowse.Show()
	}

	btnEditor := widget.NewButton("Open Editor", nil)
	btnEditor.OnTapped = func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutContractEditor("", ""))
		session.LastDomain = capture
	}

	labelAction := canvas.NewText("( DRAG-AND-DROP ENABLED )", colors.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		session.LastDomain = capture
	})

	// Handle drag & drop files for smart contracts
	session.Window.SetOnDropped(func(p fyne.Position, files []fyne.URI) {
		if session.Domain == "app.sc.builder" {
			errorText.Text = ""
			errorText.Refresh()

			if len(files) > 1 {
				errorText.Text = "single .bas file only"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			} else {
				uri, err := storage.Reader(files[0])
				if err != nil {
					errorText.Text = "could not read dropped file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				filename := files[0].Name()
				if filepath.Ext(filename) != ".bas" {
					errorText.Text = "requires a .bas file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				filedata, err := readFromURI(uri)
				if err != nil {
					logger.Errorf("[Engram] Cannot read file data for %s: %s\n", filename, err)
					errorText.Text = "cannot read file data"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				go func() {
					fyne.Do(func() {
						removeOverlays()
						capture := session.Window.Content()
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutContractEditor(strings.TrimSuffix(filepath.Base(filename), ".bas"), string(filedata)))
						session.LastDomain = capture
					})
				}()
			}
		}
	})

	entryClone := widget.NewEntry()
	entryClone.SetPlaceHolder("Clone SCID")
	if session.Offline {
		entryClone.Disable()
		entryClone.SetText("Cloning disabled in offline mode")
	}

	entryClone.OnChanged = func(s string) {
		if len(s) == 64 {
			removeOverlays()
			capture := session.Window.Content()
			session.Window.SetContent(layoutTransition())

			code, err := getContractCode(s)
			if err != nil {
				logger.Errorf("[Engram] Clone SC: %s\n", err)
				errorText.Text = "cannot get contract for clone"
				errorText.Color = colors.Red
				errorText.Refresh()
				session.Window.SetContent(layoutContractBuilder(errorText.Text))
				return
			}

			if code == "" {
				errorText.Text = "contract does not exists"
				errorText.Color = colors.Red
				errorText.Refresh()
				session.Window.SetContent(layoutContractBuilder(errorText.Text))
				return
			}

			session.Window.SetContent(layoutContractEditor("", code))
			session.LastDomain = capture
		} else {
			if s == "" {
				errorText.Text = ""
				errorText.Refresh()
			} else {
				errorText.Text = "not a valid scid"
				errorText.Color = colors.Red
				errorText.Refresh()
			}
		}
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
	)

	center := container.NewStack(
		rectWidth100,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					rectSpacer,
					rectSpacer,
					entryClone,
					errorText,
					rectSpacer,
					btnBrowse,
					rectSpacer,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						labelAction,
						layout.NewSpacer(),
					),
					rectSpacer,
					rectSpacer,
					btnEditor,
					rectSpacer,
					labelSeparator,
					rectSpacer,
					rectBox,
				),
			),
			layout.NewSpacer(),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	body := container.NewVBox(
		top,
		center,
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			body,
			bottom,
			nil,
			nil,
		),
	)

	return NewVScroll(layout)
}

func layoutFilesAndContracts() fyne.CanvasObject {
	previousDomain := session.Domain
	session.Domain = "app.filescontracts"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.35))

	rectWidth100 := canvas.NewRectangle(color.Transparent)
	rectWidth100.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.9, 10))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	header := canvas.NewText("F I L E S  &  C O N T R A C T S", colors.Gray)
	header.TextSize = scaleFont(16)
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	// Back button to return to dashboard or previous screen
	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		if previousDomain == "app.tela.manager.files" && cachedTelaManagerContent != nil {
			session.Window.SetContent(layoutTransition())
			session.Domain = "app.tela.manager"
			session.Window.SetContent(cachedTelaManagerContent)
		} else {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
		}
		removeOverlays()
	})

	// ==================== TAB 1: BROWSE FILES (File Manager) ====================
	labelResults := canvas.NewText("   RESULTS", colors.Gray)
	labelResults.TextSize = scaleFont(14)
	labelResults.Alignment = fyne.TextAlignLeading
	labelResults.TextStyle = fyne.TextStyle{Bold: true}

	signedResults := []string{}
	signedData := binding.BindStringList(&signedResults)
	signedList := widget.NewListWithData(signedData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectWidth90,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, "/")
			pos := len(split) - 1
			name := strings.Split(split[pos], ";;;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(name[0])
		},
	)

	verifiedResults := []string{}
	verifiedData := binding.BindStringList(&verifiedResults)
	verifiedList := widget.NewListWithData(verifiedData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectWidth90,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, "/")
			pos := len(split) - 1
			name := strings.Split(split[pos], ";;;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(name[0])
		},
	)

	browseErrorText := canvas.NewText(" ", colors.Green)
	browseErrorText.TextSize = scaleFont(12)
	browseErrorText.Alignment = fyne.TextAlignCenter

	dialogBrowseFiles := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			logger.Errorf("[Engram] Open file dialog: %s\n", err)
			browseErrorText.Text = "could not open file"
			browseErrorText.Color = colors.Red
			browseErrorText.Refresh()
			return
		}

		if uc == nil {
			return
		}

		if session.Domain == "app.filescontracts" {
			inputFileName := uc.URI().Name()
			outputFileName := inputFileName + ".signed"

			dialogFileSign := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] Save file dialog: %s\n", err)
					uiDo(func() {
						browseErrorText.Text = "could not open signed file"
						browseErrorText.Color = colors.Red
						browseErrorText.Refresh()
					})

					return
				}

				if uri == nil {
					return // Canceled
				}

				filedata, err := readFromURI(uc)
				if err != nil {
					logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
					uiDo(func() {
						browseErrorText.Text = "could not read file"
						browseErrorText.Color = colors.Red
						browseErrorText.Refresh()
					})

					return
				}

				_, err = writeToURI(engram.Disk.SignData(filedata), uri)
				if err != nil {
					logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
					uiDo(func() {
						browseErrorText.Text = "could not write signed file"
						browseErrorText.Color = colors.Red
						browseErrorText.Refresh()
					})

					return
				}

				outputFile := uri.URI().Name()
				if a.Driver().Device().IsMobile() {
					outputFile = outputFileName
				}

				logger.Printf("[Engram] Successfully signed file: %s\n", outputFile)

				uiDo(func() {
					browseErrorText.Text = "signed file successfully"
					browseErrorText.Color = colors.Green
					browseErrorText.Refresh()

					signedResults = append(signedResults, outputFile)
					_ = signedData.Set(signedResults)
					signedList.Refresh()

					signedLen := len(signedResults)
					labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", signedLen, signedLen)
					labelResults.Refresh()
				})

			}, session.Window)

			if !a.Driver().Device().IsMobile() {
				uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
				if err == nil {
					dialogFileSign.SetLocation(uri)
				} else {
					logger.Errorf("[Engram] Could not open current directory %s\n", err)
				}
			}

			uiDo(func() {
				dialogFileSign.SetFilter(storage.NewExtensionFileFilter([]string{".signed"}))
				dialogFileSign.SetView(dialog.ListView)
				dialogFileSign.SetFileName(outputFileName)
				dialogFileSign.Resize(fyne.NewSize(ui.Width, ui.Height))
				dialogFileSign.SetConfirmText("Save Sign")
				dialogFileSign.Show()
			})
		}
	}, session.Window)

	if !a.Driver().Device().IsMobile() {
		uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
		if err == nil {
			dialogBrowseFiles.SetLocation(uri)
		} else {
			logger.Errorf("[Engram] Could not open current directory %s\n", err)
		}
	}

	dialogBrowseFiles.Resize(fyne.NewSize(ui.Width, ui.Height))
	dialogBrowseFiles.SetView(dialog.ListView)

	signedList.OnSelected = func(id widget.ListItemID) {
		browseErrorText.Text = ""
		browseErrorText.Refresh()
	}

	verifiedList.OnSelected = func(id widget.ListItemID) {
		browseErrorText.Text = ""
		browseErrorText.Refresh()

		split := strings.Split(verifiedResults[id], ";;;")
		filepath := strings.Split(split[0], "/")
		filename := filepath[len(filepath)-1]
		filename = strings.Replace(filename, ".signed", "", -1)

		rectSpan := canvas.NewRectangle(color.Transparent)
		rectSpan.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

		detailHeader := canvas.NewText("S I G N A T U R E    D E T A I L", colors.Gray)
		detailHeader.TextSize = scaleFont(16)
		detailHeader.Alignment = fyne.TextAlignCenter
		detailHeader.TextStyle = fyne.TextStyle{Bold: true}

		labelStatus := canvas.NewText("   VERIFICATION   STATUS", colors.Gray)
		labelStatus.TextSize = scaleFont(12)
		labelStatus.TextStyle = fyne.TextStyle{Bold: true}
		labelStatus.Alignment = fyne.TextAlignCenter

		valueStatus := canvas.NewText("   Verified", colors.Green)
		valueStatus.TextSize = scaleFont(22)
		valueStatus.TextStyle = fyne.TextStyle{Bold: true}
		valueStatus.Alignment = fyne.TextAlignCenter

		labelFilename := canvas.NewText("   FILENAME", colors.Gray)
		labelFilename.TextSize = scaleFont(14)
		labelFilename.TextStyle = fyne.TextStyle{Bold: true}

		valueFilename := widget.NewRichTextFromMarkdown(filename)
		valueFilename.Wrapping = fyne.TextWrapBreak

		labelSigner := canvas.NewText("   SIGNER   ADDRESS", colors.Gray)
		labelSigner.TextSize = scaleFont(14)
		labelSigner.TextStyle = fyne.TextStyle{Bold: true}

		valueSigner := widget.NewRichTextFromMarkdown(split[1])
		valueSigner.Wrapping = fyne.TextWrapBreak

		labelSeparator := widget.NewRichTextFromMarkdown("")
		labelSeparator.Wrapping = fyne.TextWrapOff
		labelSeparator.ParseMarkdown("---")

		labelSeparator2 := widget.NewRichTextFromMarkdown("")
		labelSeparator2.Wrapping = fyne.TextWrapOff
		labelSeparator2.ParseMarkdown("---")

		labelSeparator3 := widget.NewRichTextFromMarkdown("")
		labelSeparator3.Wrapping = fyne.TextWrapOff
		labelSeparator3.ParseMarkdown("---")

		linkHide := widget.NewHyperlinkWithStyle("Hide Details", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkHide.OnTapped = func() {
			removeOverlays()
		}

		overlay := session.Window.Canvas().Overlays()
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)
		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						rectSpan,
						rectSpacer,
						detailHeader,
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewVBox(
								valueStatus,
								rectSpacer,
								labelStatus,
							),
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelFilename,
						rectSpacer,
						valueFilename,
						rectSpacer,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						labelSigner,
						rectSpacer,
						valueSigner,
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkHide,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
					layout.NewSpacer(),
				),
			),
		)
	}

	btnSignFile := widget.NewButton("Sign File", nil)
	btnSignFile.OnTapped = func() {
		dialogBrowseFiles.Show()
	}

	btnVerifyFile := widget.NewButton("Verify Signature", nil)
	btnVerifyFile.OnTapped = func() {
		dialogVerify := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil {
				logger.Errorf("[Engram] Open file dialog: %s\n", err)
				browseErrorText.Text = "could not open file"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			if uc == nil {
				return
			}

			fileName := uc.URI().Name()
			if !strings.HasSuffix(fileName, ".signed") {
				browseErrorText.Text = "verifying requires a .signed file"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", fileName, err)
				browseErrorText.Text = "could not read file"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			fileName = strings.TrimSuffix(fileName, ".signed")
			signer, message, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
				browseErrorText.Text = "signature verification failed"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())
			if isASCII(string(message)) {
				fmt.Println(string(message))
			}

			browseErrorText.Text = "verified file successfully"
			browseErrorText.Color = colors.Green
			browseErrorText.Refresh()

			verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", verifiedLen, verifiedLen)
			labelResults.Refresh()
		}, session.Window)

		if !a.Driver().Device().IsMobile() {
			uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
			if err == nil {
				dialogVerify.SetLocation(uri)
			} else {
				logger.Errorf("[Engram] Could not open current directory %s\n", err)
			}
		}

		dialogVerify.Resize(fyne.NewSize(ui.Width, ui.Height))
		dialogVerify.SetView(dialog.ListView)
		dialogVerify.Show()
	}

	browseTabContent := container.NewVBox(
		rectSpacer,
		wrapMobileButton(btnSignFile),
		rectSpacer,
		wrapMobileButton(btnVerifyFile),
		rectSpacer,
		browseErrorText,
		rectSpacer,
		rectSpacer,
		labelResults,
		rectSpacer,
		container.NewStack(
			rectBox,
			container.NewVBox(
				widget.NewLabel("   Signed Files:"),
				signedList,
				widget.NewLabel("   Verified Files:"),
				verifiedList,
			),
		),
	)

	// ==================== TAB 2: SMART CONTRACTS (Contract Builder) ====================
	contractErrorText := canvas.NewText("", colors.Red)
	contractErrorText.TextSize = scaleFont(12)
	contractErrorText.Alignment = fyne.TextAlignCenter

	dialogBrowseSC := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		contractErrorText.Text = ""
		if uc != nil {
			filename := uc.URI().Name()
			if uc.URI().MimeType() != "text/plain" {
				logger.Errorf("[Engram] Cannot open file %s in contract builder\n", filename)
				contractErrorText.Text = "cannot open file"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				return
			}

			if filepath.Ext(filename) != ".bas" {
				contractErrorText.Text = "requires a .bas file"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", filename, err)
				contractErrorText.Text = "cannot read file data"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				return
			}

			if !isASCII(string(filedata)) {
				contractErrorText.Text = "invalid file data"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				return
			}

			removeOverlays()
			capture := session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutContractEditor(strings.TrimSuffix(filename, ".bas"), string(filedata)))
			session.LastDomain = capture
		}
	}, session.Window)

	if !a.Driver().Device().IsMobile() {
		uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
		if err == nil {
			dialogBrowseSC.SetLocation(uri)
		} else {
			logger.Errorf("[Engram] Could not open current directory %s\n", err)
		}
	}

	dialogBrowseSC.Resize(fyne.NewSize(ui.Width, ui.Height))
	dialogBrowseSC.SetFilter(storage.NewExtensionFileFilter([]string{".bas"}))
	dialogBrowseSC.SetView(dialog.ListView)

	btnBrowseSC := widget.NewButton("Browse .bas Files", nil)
	btnBrowseSC.OnTapped = func() {
		dialogBrowseSC.Show()
	}

	btnEditor := widget.NewButton("Open Editor", nil)
	btnEditor.OnTapped = func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutContractEditor("", ""))
		session.LastDomain = capture
	}

	labelAction := canvas.NewText("( DRAG-AND-DROP ENABLED )", colors.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

	entryClone := widget.NewEntry()
	entryClone.SetPlaceHolder("Clone SCID (64 characters)")
	if session.Offline {
		entryClone.Disable()
		entryClone.SetText("Cloning disabled in offline mode")
	}

	entryClone.OnChanged = func(s string) {
		if len(s) == 64 {
			removeOverlays()
			capture := session.Window.Content()
			session.Window.SetContent(layoutTransition())

			code, err := getContractCode(s)
			if err != nil {
				logger.Errorf("[Engram] Clone SC: %s\n", err)
				contractErrorText.Text = "cannot get contract for clone"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				session.Window.SetContent(layoutFilesAndContracts())
				return
			}

			if code == "" {
				contractErrorText.Text = "contract does not exist"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
				session.Window.SetContent(layoutFilesAndContracts())
				return
			}

			session.Window.SetContent(layoutContractEditor("", code))
			session.LastDomain = capture
		} else {
			if s == "" {
				contractErrorText.Text = ""
				contractErrorText.Refresh()
			} else {
				contractErrorText.Text = "not a valid scid"
				contractErrorText.Color = colors.Red
				contractErrorText.Refresh()
			}
		}
	}

	contractsTabContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		entryClone,
		contractErrorText,
		rectSpacer,
		wrapMobileButton(btnBrowseSC),
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			labelAction,
			layout.NewSpacer(),
		),
		rectSpacer,
		rectSpacer,
		wrapMobileButton(btnEditor),
	)

	// ==================== TAB 3: ASSETS (Asset Explorer) ====================
	assetTabContent := createAssetExplorerTabContent()

	assetTab := container.NewTabItem("Assets", assetTabContent)
	tabs := container.NewAppTabs(
		assetTab,
		container.NewTabItem("Browse", browseTabContent),
		container.NewTabItem("SCIDs", contractsTabContent),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Handle drag & drop for both tabs
	session.Window.SetOnDropped(func(p fyne.Position, files []fyne.URI) {
		if session.Domain != "app.filescontracts" {
			return
		}

		if len(files) > 1 {
			browseErrorText.Text = "single file only"
			browseErrorText.Color = colors.Red
			browseErrorText.Refresh()
			return
		}

		uri, err := storage.Reader(files[0])
		if err != nil {
			browseErrorText.Text = "could not read dropped file"
			browseErrorText.Color = colors.Red
			browseErrorText.Refresh()
			return
		}

		filename := files[0].Name()
		ext := filepath.Ext(filename)

		if ext == ".signed" {
			// Verify signed file
			filedata, err := readFromURI(uri)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", filename, err)
				browseErrorText.Text = "cannot read file data"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			filename = strings.TrimSuffix(filename, ".signed")
			signer, message, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", filename, err)
				browseErrorText.Text = "signature verification failed"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			logger.Printf("[Engram] %s signed by: %s\n", filename, signer.String())
			if isASCII(string(message)) {
				fmt.Println(string(message))
			}

			browseErrorText.Text = "verified file successfully"
			browseErrorText.Color = colors.Green
			browseErrorText.Refresh()

			verifiedResults = append(verifiedResults, filename+";;;"+signer.String())
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   RESULTS  (%d / %d)", verifiedLen, verifiedLen)
			labelResults.Refresh()

			// Switch to Browse Files tab
			tabs.SelectIndex(0)

		} else if ext == ".bas" {
			// Open contract editor
			filedata, err := readFromURI(uri)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", filename, err)
				browseErrorText.Text = "cannot read file data"
				browseErrorText.Color = colors.Red
				browseErrorText.Refresh()
				return
			}

			uiDo(func() {
				removeOverlays()
				capture := session.Window.Content()
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutContractEditor(strings.TrimSuffix(filepath.Base(filename), ".bas"), string(filedata)))
				session.LastDomain = capture
			})
		} else {
			browseErrorText.Text = "unsupported file type"
			browseErrorText.Color = colors.Red
			browseErrorText.Refresh()
		}
	})

	// Main layout
	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		header,
	)

	center := container.NewStack(
		rectWidth100,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					tabs,
				),
			),
			layout.NewSpacer(),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

func createAssetExplorerTabContent() fyne.CanvasObject {
	var data []string
	var listData binding.StringList
	var listBox *widget.List

	rectLeft := canvas.NewRectangle(color.Transparent)
	rectLeft.SetMinSize(fyne.NewSize(ui.Width*0.40, 35))
	rectRight := canvas.NewRectangle(color.Transparent)
	rectRight.SetMinSize(fyne.NewSize(ui.Width*0.58, 35))
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.45))
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	results := canvas.NewText("", colors.Green)
	results.TextSize = scaleFont(14)

	listData = binding.BindStringList(&data)
	listBox = widget.NewListWithData(listData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectLeft,
						widget.NewLabel(""),
					),
					container.NewStack(
						rectRight,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, ";;")

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
		})

	entrySCID := widget.NewEntry()
	entrySCID.PlaceHolder = "Search by SCID"
	entrySCID.SetIcon(theme.SearchIcon())
	entrySCID.Disable()

	linkClearHistory := widget.NewHyperlinkWithStyle("Clear All", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: false})
	linkClearHistory.OnTapped = func() {
		shard, err := GetShard()
		if err != nil {
			return
		}

		store, err := graviton.NewDiskStore(shard)
		if err != nil {
			return
		}

		ss, err := store.LoadSnapshot(0)

		if err != nil {
			return
		}

		tree, err := ss.GetTree("Explorer History")
		if err != nil {
			return
		}

		c := tree.Cursor()

		for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
			DeleteKey(tree.GetName(), k)
		}

		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAssetExplorer())
	}

	btnMyAssets := widget.NewButton("My Assets", func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMyAssets())
	})

	listing := container.NewStack(
		rectWidth,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				rectSpacer,
				container.NewHBox(
					results,
					layout.NewSpacer(),
					linkClearHistory,
				),
				rectSpacer,
				rectSpacer,
				entrySCID,
				rectSpacer,
				rectSpacer,
				container.NewStack(
					rectList,
					listBox,
				),
				rectSpacer,
				rectSpacer,
				wrapMobileButton(btnMyAssets),
			),
			layout.NewSpacer(),
		),
	)

	var assetData []string

	found := 0
	assetData = nil

	results.Text = fmt.Sprintf("  Results:  %d", found)
	results.Color = colors.Green
	results.Refresh()

	listData.Set(nil)

	if session.Offline {
		results.Text = "  Disabled in offline mode."
		results.Color = colors.Gray
		results.Refresh()
	} else if gnomon.Index == nil {
		results.Text = "  Gnomon is inactive."
		results.Color = colors.Gray
		results.Refresh()
	} else {
		entrySCID.Enable()
	}

	entrySCID.OnChanged = func(s string) {
		if entrySCID.Text != "" && len(s) == 64 {
			showLoadingOverlay()

			var result []*structures.SCIDVariable
			switch gnomon.Index.DBType {
			case "gravdb":
				result = gnomon.Index.GravDBBackend.GetSCIDVariableDetailsAtTopoheight(s, engram.Disk.Get_Daemon_TopoHeight())
			case "boltdb":
				result = gnomon.Index.BBSBackend.GetSCIDVariableDetailsAtTopoheight(s, engram.Disk.Get_Daemon_TopoHeight())
			}

			if len(result) == 0 {
				_, err := getTxData(s)
				if err != nil {
					return
				}
			}

			err := StoreEncryptedValue("Explorer History", []byte(s), []byte(""))
			if err != nil {
				logger.Errorf("[Asset Explorer] Error saving search result: %s\n", err)
				return
			}

			scid := crypto.HashHexToHash(s)

			bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(scid, -1, engram.Disk.GetAddress().String())
			if err != nil {
				bal = 0
			}

			title, desc, _, _, _ := getContractHeader(scid)

			if title == "" {
				title = scid.String()
			}

			if len(title) > 18 {
				title = title[0:18] + "..."
			}

			if desc == "" {
				desc = "N/A"
			}

			if len(desc) > 40 {
				desc = desc[0:40] + "..."
			}

			assetData = append(data, globals.FormatMoney(bal)+";;;"+title+";;;"+desc+";;;;;;"+scid.String())
			listData.Set(assetData)
			found += 1

			entrySCID.SetText("")
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutAssetManager(s))
			removeOverlays()
		}
	}

	go func() {
		if engram.Disk != nil && gnomon.Index != nil {
			for gnomon.Index.LastIndexedHeight < int64(engram.Disk.Get_Daemon_Height()) {
				if session.Domain != "app.explorer" && session.Domain != "app.filescontracts" {
					break
				}
				entrySCID.Disable()
				results.Text = "  Gnomon is syncing..."
				results.Color = colors.Yellow

				fyne.Do(func() {
					results.Refresh()
				})

				time.Sleep(time.Second * 1)
			}

			fyne.Do(func() {
				entrySCID.Enable()
				results.Text = "  Loading previous scan history..."
				results.Color = colors.Yellow
				results.Refresh()
			})

			shard, err := GetShard()
			if err != nil {
				return
			}

			store, err := graviton.NewDiskStore(shard)
			if err != nil {
				return
			}

			ss, err := store.LoadSnapshot(0)

			if err != nil {
				return
			}

			tree, err := ss.GetTree("Explorer History")
			if err != nil {
				return
			}

			c := tree.Cursor()

			for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
				scid := crypto.HashHexToHash(string(k))

				bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(scid, -1, engram.Disk.GetAddress().String())
				if err != nil {
					bal = 0
				}

				title, desc, _, _, _ := getContractHeader(scid)

				if title == "" {
					title = scid.String()
				}

				if len(title) > 18 {
					title = title[0:18] + "..."
				}

				if desc == "" {
					desc = "N/A"
				}

				if len(desc) > 40 {
					desc = desc[0:40] + "..."
				}

				assetData = append(data, globals.FormatMoney(bal)+";;;"+title+";;;"+desc+";;;;;;"+scid.String())
				listData.Set(assetData)
				found += 1
			}
		}

		listData.Set(assetData)

		listBox.OnSelected = func(id widget.ListItemID) {
			split := strings.Split(assetData[id], ";;;")
			listBox.UnselectAll()
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutAssetManager(split[4]))
		}

		fyne.Do(func() {
			results.Text = fmt.Sprintf("  Search History:  %d", found)
			results.Color = colors.Green
			results.Refresh()
			listBox.Refresh()
		})
	}()

	return container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			listing,
		),
	)
}

func layoutContractEditor(filename, filedata string) fyne.CanvasObject {
	session.Domain = "app.sc.editor"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.35))

	rectWidth100 := canvas.NewRectangle(color.Transparent)
	rectWidth100.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.9, 10))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	rectCode := canvas.NewRectangle(color.Transparent)
	rectCode.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.35))

	heading := canvas.NewText("C O N T R A C T    E D I T O R", colors.Green)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelHeaders := canvas.NewText("   HEADERS", colors.Gray)
	labelHeaders.TextSize = scaleFont(14)
	labelHeaders.Alignment = fyne.TextAlignLeading
	labelHeaders.TextStyle = fyne.TextStyle{Bold: true}

	labelCode := canvas.NewText("   CODE (DVM-BASIC)", colors.Gray)
	labelCode.TextSize = scaleFont(14)
	labelCode.Alignment = fyne.TextAlignLeading
	labelCode.TextStyle = fyne.TextStyle{Bold: true}

	labelCodeSize := canvas.NewText("(0.0KB) ", colors.Green)
	labelCodeSize.TextSize = scaleFont(12)
	labelCodeSize.Alignment = fyne.TextAlignTrailing

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	var nameHdr, iconURLHdr, descrHdr string
	nameHdr = filename

	// Get headers from contract code initialize func
	if filedata != "" {
		contract, _, err := dvm.ParseSmartContract(strings.ReplaceAll(filedata, "\x00", ""))
		if err == nil {
			for n, f := range contract.Functions {
				if n == "InitializePrivate" || n == "Initialize" {
					for _, line := range f.Lines {
						if len(line) < 6 {
							// Line is to short to be a STORE
							continue
						}

						for i, parts := range line {
							if parts == "STORE" {
								// Find if code is storing headers
								header := tela.Header(line[i+2])
								if header == tela.HEADER_NAME || header == tela.HEADER_NAME_V2 {
									nameHdr = strings.Trim(line[i+4], `"`)
								} else if header == tela.HEADER_ICON_URL || header == tela.HEADER_ICON_URL_V2 {
									iconURLHdr = strings.Trim(line[i+4], `"`)
								} else if header == tela.HEADER_DESCRIPTION || header == tela.HEADER_DESCRIPTION_V2 {
									descrHdr = strings.Trim(line[i+4], `"`)
								}
							}
						}
					}
				}
			}
		}
	}

	entryName := widget.NewEntry()
	entryName.SetText(nameHdr)
	entryName.SetPlaceHolder("Name")
	entryName.Validator = func(s string) (err error) {
		if s == "" {
			err = fmt.Errorf("enter a name")
			errorText.Text = err.Error()
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		errorText.Text = ""
		errorText.Refresh()

		return nil
	}

	entryIcon := widget.NewEntry()
	entryIcon.SetPlaceHolder("Icon")
	entryIcon.SetText(iconURLHdr)
	entryIcon.Validator = func(s string) (err error) {
		if s == "" {
			err = fmt.Errorf("enter icon URL")
			errorText.Text = err.Error()
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		errorText.Text = ""
		errorText.Refresh()

		return nil
	}

	var entryUpdated bool
	entryDescription := widget.NewEntry()
	entryDescription.SetPlaceHolder("Description")
	entryDescription.SetText(descrHdr)
	entryDescription.Validator = func(s string) (err error) {
		if s == "" && entryUpdated {
			err = fmt.Errorf("enter description")
			errorText.Text = err.Error()
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		entryUpdated = true

		errorText.Text = ""
		errorText.Refresh()

		return nil
	}

	var unsavedChanges bool
	entryCode := widget.NewMultiLineEntry()
	entryCode.SetPlaceHolder("Code")
	entryCode.Wrapping = fyne.TextWrapWord
	entryCode.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()

		size := tela.GetCodeSizeInKB(s)

		labelCodeSize.Text = fmt.Sprintf("(%.2fKB) ", size)
		if size > 20 {
			labelCodeSize.Color = colors.Red
			errorText.Text = "contract size is to large"
			errorText.Color = colors.Red
			errorText.Refresh()
		} else if size > 18.5 {
			labelCodeSize.Color = colors.Yellow
		} else {
			labelCodeSize.Color = colors.Green
		}
		labelCodeSize.Refresh()

		if s != filedata {
			unsavedChanges = true
		} else {
			unsavedChanges = false
		}
	}

	entryCode.SetText(filedata)

	options := []string{"Initialize", "Set Headers", "New Function", "Parse", "Format", "Clear", "Export"}
	if !session.Offline {
		splice := append([]string{"Import Function"}, options[3:]...)
		options = append(options[:3], splice...)
		options = append(options, "Install")
	}

	selectEditor := widget.NewSelect(options, nil)

	entryForm := container.NewVBox(
		rectSpacer,
		selectEditor,
		rectSpacer,
		container.NewBorder(
			nil,
			nil,
			labelCode,
			labelCodeSize,
			nil,
		),
		container.NewStack(
			rectCode,
			entryCode,
		),
		errorText,
		rectSpacer,
		labelHeaders,
		rectSpacer,
		entryName,
		entryIcon,
		entryDescription,
	)

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		if unsavedChanges {
			verificationOverlay(
				false,
				"CONTRACT  EDITOR",
				"Leave with unsaved changes",
				"Confirm",
				func(b bool) {
					if b {
						capture := session.Window.Content()
						session.Window.SetContent(layoutTransition())
						session.Window.SetContent(layoutContractBuilder(""))
						session.LastDomain = capture
					}
				},
			)
		} else {
			removeOverlays()
			capture := session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutContractBuilder(""))
			session.LastDomain = capture
		}
	})

	selectEditor.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()

		switch s {
		case "Initialize": // Set entry text with new starter initialize func
			if entryCode.Text == "" {
				entryCode.SetText(dvmInitFuncExample())
				errorText.Text = "new initialize function created"
				errorText.Color = colors.Green
				errorText.Refresh()
				return
			}

			verificationOverlay(
				false,
				"CONTRACT  EDITOR",
				"Reset to default initialize function",
				"Confirm",
				func(b bool) {
					if b {
						entryCode.SetText(dvmInitFuncExample())
						errorText.Text = "new initialize function created"
						errorText.Color = colors.Green
						errorText.Refresh()
					}
				},
			)
		case "New Function": // Add a new starter initialize func to code entry
			increment := 1
			var hasInitFunc bool
			fn := tela.GetSmartContractFuncNames(entryCode.Text)
			for _, n := range fn {
				// Increment function number if new() already esists
				if strings.TrimRight(n, "0123456789") == "new" {
					increment++
				}

				if n == "InitializePrivate" || n == "Initialize" {
					hasInitFunc = true
				}
			}

			if !hasInitFunc {
				errorText.Text = "no initialize function"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if strings.HasSuffix(entryCode.Text, "\n") {
				entryCode.SetText(entryCode.Text + "\n" + dvmFuncExample(increment))
			} else {
				entryCode.SetText(entryCode.Text + "\n\n" + dvmFuncExample(increment))
			}

			errorText.Text = "new function added"
			errorText.Color = colors.Green
			errorText.Refresh()
		case "Import Function": // Import a function from an on-chain scid
			var hasInitFunc bool
			fn := tela.GetSmartContractFuncNames(entryCode.Text)
			for _, n := range fn {
				if n == "InitializePrivate" || n == "Initialize" {
					hasInitFunc = true
					break
				}
			}

			entryEntrypoint := widget.NewEntry()
			entryEntrypoint.SetPlaceHolder("Function name")
			entryEntrypoint.Validator = func(s string) (err error) {
				if s == "" || (len(s) > 0 && !unicode.IsLetter(rune(s[0]))) {
					return fmt.Errorf("invalid function name")
				}

				return nil
			}

			entrySCID := widget.NewEntry()
			entrySCID.SetPlaceHolder("SCID")
			entrySCID.Validator = func(s string) (err error) {
				if len(s) != 64 {
					return fmt.Errorf("not a valid scid")
				}

				return nil
			}

			overlay := session.Window.Canvas().Overlays()

			header := canvas.NewText("CONTRACT  EDITOR", colors.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText("Import an existing function", colors.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkCancel := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkCancel.OnTapped = func() {
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
			}

			span := canvas.NewRectangle(color.Transparent)
			span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

			overlay.Add(
				container.NewStack(
					&iframe{},
					canvas.NewRectangle(colors.DarkMatter),
				),
			)

			paramsContainer := container.NewVBox(entrySCID, entryEntrypoint)

			btnImport := widget.NewButton("Import", nil)
			btnImport.OnTapped = func() {
				if entrySCID.Validate() != nil {
					entrySCID.FocusGained()
					entrySCID.FocusLost()
					return
				}

				if entryEntrypoint.Validate() != nil {
					entryEntrypoint.FocusGained()
					entryEntrypoint.FocusLost()
					return
				}

				defer removeOverlays()

				if !hasInitFunc {
					if entryEntrypoint.Text != "InitializePrivate" && entryEntrypoint.Text != "Initialize" {
						errorText.Text = "need initializing function first"
						errorText.Color = colors.Red
						errorText.Refresh()
						return
					}
				}

				code, err := getContractCode(entrySCID.Text)
				if err != nil {
					logger.Errorf("[Engram] Editor import function error: %s\n", err)
					errorText.Text = "cannot get contract for function import"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if code == "" {
					errorText.Text = "contract does not exists"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				entrypoint := entryEntrypoint.Text
				contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(code, "\x00", ""))
				if err != nil {
					logger.Errorf("[Engram] Editor import parsing error: %s %s\n", err, pos)
					errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				var tempSC dvm.SmartContract
				tempSC.Functions = make(map[string]dvm.Function)

				for name, f := range contract.Functions {
					if name == entrypoint {
						tempSC.Functions[name] = f
						break
					}
				}

				if tempSC.Functions[entrypoint].LineNumbers == nil {
					errorText.Text = "function not found on scid"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				formatted, err := tela.FormatSmartContract(tempSC, fmt.Sprintf("Function %s", entrypoint))
				if err != nil {
					logger.Errorf("[Engram] Editor import formatting error: %s\n", err)
					errorText.Text = "could not parse dvm to string"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if entryCode.Text == "" {
					entryCode.SetText(formatted)
				} else if strings.HasSuffix(entryCode.Text, "\n") {
					entryCode.SetText(entryCode.Text + "\n" + formatted)
				} else {
					entryCode.SetText(entryCode.Text + "\n\n" + formatted)
				}

				errorText.Text = "imported function successfully"
				errorText.Color = colors.Green
				errorText.Refresh()
			}

			overlay.Add(
				container.NewStack(
					&iframe{},
					container.NewCenter(
						container.NewVBox(
							span,
							container.NewCenter(
								header,
							),
							rectSpacer,
							rectSpacer,
							container.NewCenter(
								subHeader,
							),
							widget.NewLabel(""),
							rectSpacer,
							rectSpacer,
							paramsContainer,
							rectSpacer,
							rectSpacer,
							btnImport,
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								linkCancel,
								layout.NewSpacer(),
							),
							rectSpacer,
							rectSpacer,
						),
					),
				),
			)
		case "Clear": // Clears SC code entry
			verificationOverlay(
				false,
				"CONTRACT  EDITOR",
				"Clear code entry",
				"Confirm",
				func(b bool) {
					if b {
						entryCode.SetText("")
						errorText.Text = "contract code cleared"
						errorText.Color = colors.Green
						errorText.Refresh()
					}
				},
			)
		case "Parse": // Parse SC for errors
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			_, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = colors.Red
				errorText.Refresh()
				logger.Errorf("[Engram] Parse SC: %s %s\n", err, pos)
				return
			}

			errorText.Text = "contract parsed successfully"
			errorText.Color = colors.Green
			errorText.Refresh()
		case "Set Headers": // Set Artificer standard headers into initialize func
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = colors.Red
				errorText.Refresh()
				logger.Errorf("[Engram] Set SC Headers: %s %s\n", err, pos)
				return
			}

			if entryName.Validate() == nil && entryIcon.Validate() == nil && entryDescription.Validate() == nil {
				// Create add header func to use later in confirmations
				addFunction := func() {
					var haveHeader [uint64(3)]bool
					for name, function := range contract.Functions {
						// Find initialize func
						if name == "Initialize" || name == "InitializePrivate" {
							for _, line := range function.Lines {
								if len(line) < 6 {
									// Line is to short to be a STORE
									continue
								}

								for i, parts := range line {
									if parts == "STORE" {
										// Find if code is storing headers and update vars with header entry value
										header := tela.Header(line[i+2])
										if header == tela.HEADER_NAME || header == tela.HEADER_NAME_V2 {
											haveHeader[0] = true
											line[i+4] = fmt.Sprintf(`"%s"`, entryName.Text)
										} else if header == tela.HEADER_ICON_URL || header == tela.HEADER_ICON_URL_V2 {
											haveHeader[1] = true
											line[i+4] = fmt.Sprintf(`"%s"`, entryIcon.Text)
										} else if header == tela.HEADER_DESCRIPTION || header == tela.HEADER_DESCRIPTION_V2 {
											haveHeader[2] = true
											line[i+4] = fmt.Sprintf(`"%s"`, entryDescription.Text)
										}
									}
								}
							}
						}
					}

					// Check if any headers are missing
					var needToAdd, hasInitFunc bool
					for _, hh := range haveHeader {
						if !hh {
							needToAdd = true
							break
						}
					}

					// SC has all headers already, update the code entry
					if !needToAdd {
						code, err := tela.FormatSmartContract(contract, entryCode.Text)
						if err != nil {
							logger.Errorf("[Engram] Format code error: %s\n", err)
							err = errors.New("could not parse dvm to string")
							errorText.Text = err.Error()
							errorText.Color = colors.Red
							errorText.Refresh()
							return
						}

						entryCode.SetText(code)

						errorText.Text = "headers updated"
						errorText.Color = colors.Green
						errorText.Refresh()
						return
					}

					// SC is missing one or more headers so they will be added into initialize func
					for name, function := range contract.Functions {
						if name == "Initialize" || name == "InitializePrivate" {
							hasInitFunc = true

							lineLen := len(function.LineNumbers)
							indexEnd := lineLen - 1

							// Starting from the last line number loop upwards
							for i := 0; i < lineLen; i++ {
								index := indexEnd - i
								if index < 0 {
									break
								}

								line := function.Lines[function.LineNumbers[index]]
								if len(line) < 1 {
									continue
								}

								// If line is RETURN 0 will inject headers here and push RETURN 0 line down if there is room
								if line[0] == "RETURN" && line[1] == "0" {
									if index-1 < 0 {
										err = errors.New("no room for header lines")
										errorText.Text = err.Error()
										errorText.Color = colors.Red
										errorText.Refresh()
										return
									} else if i > 0 && function.LineNumbers[index+1] < function.LineNumbers[index]+4 {
										err = fmt.Errorf("no room for header lines below %d", function.LineNumbers[index])
										errorText.Text = err.Error()
										errorText.Color = colors.Red
										errorText.Refresh()
										return
									} else {
										var addedLines, skipedLines uint64
										for u := uint64(1); u < 5; u++ {
											addLineNum := function.LineNumbers[index] + (u - 1) - skipedLines
											switch u {
											case 1: // nameHdr
												if !haveHeader[0] {
													function.Lines[addLineNum] = []string{"STORE", "(", `"var_header_name"`, ",", fmt.Sprintf(`"%s"`, entryName.Text), ")"}
													addedLines++
												} else {
													// Count skip if we have already to subtract to line number
													skipedLines++
													continue
												}
											case 2: // iconURLHdr
												if !haveHeader[1] {
													function.Lines[addLineNum] = []string{"STORE", "(", `"var_header_icon"`, ",", fmt.Sprintf(`"%s"`, entryIcon.Text), ")"}
													if skipedLines != 1 {
														function.LineNumbers = append(function.LineNumbers, addLineNum)
													}
													addedLines++
												} else {
													skipedLines++
													continue
												}
											case 3: // descrHdr
												if !haveHeader[2] {
													function.Lines[addLineNum] = []string{"STORE", "(", `"var_header_description"`, ",", fmt.Sprintf(`"%s"`, entryDescription.Text), ")"}
													if skipedLines != 2 {
														function.LineNumbers = append(function.LineNumbers, addLineNum)
													}
													addedLines++
												}
											case 4:
												function.Lines[addLineNum] = []string{"RETURN", "0"}
												function.LineNumbers = append(function.LineNumbers, addLineNum)
											}
										}

										// If changes were made sort line numbers and add them to index
										if addedLines > 0 {
											sort.Slice(function.LineNumbers, func(i, j int) bool {
												return function.LineNumbers[i] < function.LineNumbers[j]
											})

											for u, ln := range function.LineNumbers {
												function.LinesNumberIndex[ln] = uint64(u)
											}

											contract.Functions[name] = function
										}

										// fmt.Println("Lines", contract.Functions[name].Lines)
										// fmt.Println("LineNumbers", contract.Functions[name].LineNumbers)
										// fmt.Println("LineNumberIndex", contract.Functions[name].LinesNumberIndex)

										break
									}
								}
							}
						}
					}

					if !hasInitFunc {
						err = errors.New("no initialize function")
						errorText.Text = err.Error()
						errorText.Color = colors.Red
						errorText.Refresh()
						return
					}

					code, err := tela.FormatSmartContract(contract, entryCode.Text)
					if err != nil {
						logger.Errorf("[Engram] Format code error: %s\n", err)
						err = errors.New("could not parse dvm to string")
						errorText.Text = err.Error()
						errorText.Color = colors.Red
						errorText.Refresh()
						return
					}

					if code == entryCode.Text {
						errorText.Text = "did not change headers"
						errorText.Color = colors.Red
						errorText.Refresh()
						return
					}

					entryCode.SetText(code)

					errorText.Text = "contract headers added successfully"
					errorText.Color = colors.Green
					errorText.Refresh()
				}

				codeCheck, err := tela.FormatSmartContract(contract, entryCode.Text)
				if err != nil {
					logger.Errorf("[Engram] Format code error: %s\n", err)
					err = errors.New("could not parse dvm to string")
					errorText.Text = err.Error()
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				// Warn user that code will be formatted if headers are added
				if codeCheck != entryCode.Text {
					verificationOverlay(
						false,
						"CONTRACT  EDITOR",
						"Setting headers formats your code",
						"Confirm",
						func(b bool) {
							if b {
								addFunction()
							}
						},
					)
				} else {
					addFunction()
				}
			}
		case "Format": // Format SC code
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = colors.Red
				errorText.Refresh()
				logger.Errorf("[Engram] Format: %s %s\n", err, pos)
				return
			}

			code, err := tela.FormatSmartContract(contract, entryCode.Text)
			if err != nil {
				logger.Errorf("[Engram] Format code error: %s\n", err)
				err = errors.New("could not parse dvm to string")
				errorText.Text = err.Error()
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if code == entryCode.Text {
				errorText.Text = "contract code is formatted"
				errorText.Color = colors.Green
				errorText.Refresh()
				return
			}

			verificationOverlay(
				false,
				"CONTRACT  EDITOR",
				"Remove whitespace and comments",
				"Confirm",
				func(b bool) {
					if b {
						entryCode.SetText(code)

						errorText.Text = "contract code formatted successfully"
						errorText.Color = colors.Green
						errorText.Refresh()
					}
				},
			)
		case "Export": // Export SC to file
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			exportFileName := fmt.Sprintf("%s.bas", entryName.Text)

			data := []byte(entryCode.Text)
			dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not export contract file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				if uri == nil {
					return // Canceled
				}

				_, err = writeToURI(data, uri)
				if err != nil {
					logger.Errorf("[Engram] Exporting %s: %s\n", exportFileName, err)
					errorText.Text = "error exporting contract file"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				unsavedChanges = false
				filedata = entryCode.Text
				errorText.Text = "exported contract file successfully"
				errorText.Color = colors.Green
				errorText.Refresh()

			}, session.Window)

			if !a.Driver().Device().IsMobile() {
				// Open file browser in current directory
				uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
				if err == nil {
					dialogFileSave.SetLocation(uri)
				} else {
					logger.Errorf("[Engram] Could not open current directory %s\n", err)
				}
			}

			dialogFileSave.SetFilter(storage.NewExtensionFileFilter([]string{".bas"}))
			dialogFileSave.SetView(dialog.ListView)
			dialogFileSave.SetFileName(exportFileName)
			dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
			dialogFileSave.Show()
		case "Install": // Install SC
			code := entryCode.Text
			if code == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(code, "\x00", ""))
			if err != nil {
				logger.Errorf("[Engram] Install SC: %s %s\n", err, pos)
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			var entrypoint string
			var args []rpc.Argument
			for name, function := range contract.Functions {
				if name == "InitializePrivate" || name == "Initialize" {
					entrypoint = name
					for _, v := range function.Params {
						switch v.Type {
						case 0x4:
							args = append(args, rpc.Argument{Name: v.Name, DataType: rpc.DataUint64, Value: v.ValueUint64})
						case 0x5:
							args = append(args, rpc.Argument{Name: v.Name, DataType: rpc.DataString, Value: v.ValueString})
						}
					}
				}
			}

			if entrypoint == "" {
				errorText.Text = "missing initializing entrypoint"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			function := contract.Functions[entrypoint]

			var paramList []fyne.Widget
			if len(function.Params) > 0 {
				params := function.Params
				for i := range params {
					p := i
					entry := widget.NewEntry()
					entry.PlaceHolder = params[p].Name
					if params[p].Type == 0x4 {
						entry.PlaceHolder = params[p].Name + " (Numbers Only)"
					}

					entry.Validator = func(s string) error {
						switch params[p].Type {
						case 0x5:
							return nil
						case 0x4:
							if params[p].Name+" (Numbers Only)" == entry.PlaceHolder {
								amount, err := globals.ParseAmount(s)
								if err != nil {
									logger.Debugf("[%s] Param error: %s\n", params[p].Name, err)
									return err
								} else {
									logger.Debugf("[%s] Amount: %d\n", params[p].Name, amount)
								}
							}
						}

						return nil
					}

					paramList = append(paramList, entry)
				}

				overlay := session.Window.Canvas().Overlays()

				header := canvas.NewText("INSTALL  SMART  CONTRACT", colors.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				subHeader := canvas.NewText(fmt.Sprintf("%s params", entrypoint), colors.Account)
				subHeader.TextSize = scaleFont(22)
				subHeader.Alignment = fyne.TextAlignCenter
				subHeader.TextStyle = fyne.TextStyle{Bold: true}

				linkClose := widget.NewHyperlinkWithStyle("Close", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
				linkClose.OnTapped = func() {
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
					overlay.Remove(overlay.Top())
				}

				span := canvas.NewRectangle(color.Transparent)
				span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

				overlay.Add(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(colors.DarkMatter),
					),
				)

				paramsContainer := container.NewVBox()

				btnInstall := widget.NewButton("Install", nil)

				overlay.Add(
					container.NewStack(
						&iframe{},
						container.NewCenter(
							container.NewVBox(
								span,
								container.NewCenter(
									header,
								),
								rectSpacer,
								rectSpacer,
								container.NewCenter(
									subHeader,
								),
								widget.NewLabel(""),
								//selectRingMembers,
								rectSpacer,
								rectSpacer,
								paramsContainer,
								rectSpacer,
								rectSpacer,
								btnInstall,
								rectSpacer,
								rectSpacer,
								container.NewHBox(
									layout.NewSpacer(),
									linkClose,
									layout.NewSpacer(),
								),
								rectSpacer,
								rectSpacer,
							),
						),
					),
				)

				for _, w := range paramList {
					c := container.NewStack(
						span,
						w,
					)

					paramsContainer.Add(c)
					paramsContainer.Refresh()
				}

				btnInstall.OnTapped = func() {
					validated := true
					for _, w := range paramList {
						entry, ok := w.(*widget.Entry)
						if !ok {
							continue
						}

						if entry.Validate() != nil {
							entry.FocusGained()
							entry.FocusLost()
							validated = false
							break
						}
					}

					if !validated {
						return
					}

					btnInstall.Text = "Installing..."
					btnInstall.Disable()
					btnInstall.Refresh()

					verificationOverlay(
						true,
						"CONTRACT  EDITOR",
						"",
						"",
						func(b bool) {
							if b {
								_, err := installSC(code, args)
								if err != nil {
									errorText.Text = err.Error()
									errorText.Color = colors.Red
									errorText.Refresh()
									return
								}

								unsavedChanges = false
								errorText.Text = "contract installed successfully"
								errorText.Color = colors.Green
								errorText.Refresh()
							}

							overlay.Top().Hide()
							overlay.Remove(overlay.Top())
							overlay.Remove(overlay.Top())
						},
					)
				}

				paramsContainer.Refresh()
				overlay.Top().Show()
			} else {
				verificationOverlay(
					true,
					"CONTRACT  EDITOR",
					"",
					"",
					func(b bool) {
						if b {
							_, err := installSC(code, args)
							if err != nil {
								errorText.Text = err.Error()
								errorText.Color = colors.Red
								errorText.Refresh()
								return
							}

							unsavedChanges = false
							errorText.Text = "contract installed successfully"
							errorText.Color = colors.Green
							errorText.Refresh()
						}
					},
				)
			}
		}
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		heading,
	)

	center := container.NewStack(
		rectWidth100,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					rectSpacer,
					container.NewStack(
						rectBox,
						entryForm,
					),
					rectSpacer,
				),
			),
			layout.NewSpacer(),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	body := container.NewVBox(
		top,
		center,
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			body,
			bottom,
			nil,
			nil,
		),
	)

	return NewVScroll(layout)
}

func layoutTELA() fyne.CanvasObject {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[TELA-LAYOUT] PANIC recovered in layoutTELA(): %v\n", r)
			session.Domain = "app.wallet"
			if session.Window != nil {
				session.Window.SetContent(layoutDashboard())
			}
		}
	}()

	logger.Printf("[TELA-LAYOUT] layoutTELA() starting...\n")
	session.Domain = "app.tela"

	var history []string
	var historyData binding.StringList
	var historyList *widget.List

	var searching []string
	var searchData binding.StringList
	var searchList *widget.List

	var serving []string
	var servingData binding.StringList
	var servingList *widget.List

	var favorites []string
	var favoritesData binding.StringList
	var favoritesList *widget.List
	var refreshFavoritesList func()
	var refreshAppsList func()
	var refreshServerList func()
	var scheduleTelaWarmup func()
	var maybeStartTelaWork func(bool)
	var startTelaInitialLoad func()
	var resetTelaProgress func()
	var hasTelaCache func() bool
	var telaWarmupScheduled atomic.Bool
	var telaWorkActive atomic.Bool
	var telaLaunchPending atomic.Bool
	var activeRowUpdaters sync.Map   // fyne.CanvasObject -> scid
	var activeRatingFetches sync.Map // scid -> bool
	var telaNetworkPaused atomic.Bool

	frame := &iframe{}
	rectLeft := canvas.NewRectangle(color.Transparent)
	rectLeft.SetMinSize(fyne.NewSize(ui.Width*0.40, scaleSize(35)))

	rectRight := canvas.NewRectangle(color.Transparent)
	rectRight.SetMinSize(fyne.NewSize(ui.Width*0.58, scaleSize(35)))

	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, 100))

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	isMobileLayout := ui.Width <= 360
	if isMobileLayout {
		rectSpacer.SetMinSize(fyne.NewSize(scaleSize(6), scaleSize(2)))
	}

	heading := canvas.NewText("T E L A    B R O W S E R", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	results := canvas.NewText("", colors.Green)
	results.TextSize = scaleFont(13)

	telaStatus := canvas.NewText("", color.Transparent)
	telaStatus.TextSize = scaleFont(12)

	telaProgress := NewSlimProgressBar()
	telaProgress.Hide()

	labelLastScan := canvas.NewText("", colors.Green)
	labelLastScan.TextSize = scaleFont(13)

	errorText := canvas.NewText("", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter
	errorText.Hide()

	statusBox := container.NewVBox()
	refreshTelaStatusBox := func() {
		objs := []fyne.CanvasObject{}
		if !errorText.Hidden && strings.TrimSpace(errorText.Text) != "" {
			objs = append(objs, errorText)
		}
		if !results.Hidden && strings.TrimSpace(results.Text) != "" {
			objs = append(objs, results)
		}
		if !telaStatus.Hidden && strings.TrimSpace(telaStatus.Text) != "" {
			objs = append(objs, telaStatus)
		}
		if !telaProgress.Hidden {
			objs = append(objs, telaProgress)
		}
		statusBox.Objects = objs
		statusBox.Refresh()
	}

	var telaSearch []INDEXwithRatings
	var searchMu sync.RWMutex
	var sortBy string
	var sortDescending bool = true
	searchData = binding.BindStringList(&searching)
	var refreshTimer *time.Timer
	var refreshMu sync.Mutex
	refreshSearch := func() {
		refreshMu.Lock()
		defer refreshMu.Unlock()
		if refreshTimer != nil {
			refreshTimer.Stop()
		}
		refreshTimer = time.AfterFunc(1500*time.Millisecond, func() {
			fyne.Do(func() {
				if sortBy != "Ratings" {
					return
				}
				searchMu.Lock()
				newList := telaSearchDisplayAll(telaSearch, sortBy, sortDescending)

				// Compare with current 'searching' list to see if anything actually moved
				if len(newList) == len(searching) {
					changed := false
					for i := range newList {
						if newList[i] != searching[i] {
							changed = true
							break
						}
					}
					if !changed {
						searchMu.Unlock()
						return
					}
				}

				searching = newList
				searchMu.Unlock()
				searchData.Set(searching)
				if searchList != nil {
					searchList.Refresh()
				}
			})
		})
	}

	findTelaSearchEntry := func(scid string) (INDEXwithRatings, bool) {
		searchMu.RLock()
		defer searchMu.RUnlock()
		for _, entry := range telaSearch {
			if entry.SCID == scid {
				return entry, true
			}
		}
		return INDEXwithRatings{}, false
	}

	updateTelaSearchEntry := func(scid string, update func(*INDEXwithRatings)) {
		searchMu.Lock()
		updated := false
		for i := range telaSearch {
			if telaSearch[i].SCID == scid {
				update(&telaSearch[i])
				updated = true
				break
			}
		}
		searchMu.Unlock()
		if updated {
			refreshSearch()
		}
	}

	warmRatings := func() {
		searchMu.RLock()
		if len(telaSearch) == 0 {
			searchMu.RUnlock()
			return
		}
		var missing []string
		for _, entry := range telaSearch {
			if entry.ratings.Average == 0 {
				missing = append(missing, entry.SCID)
			}
		}
		searchMu.RUnlock()

		if len(missing) == 0 {
			return
		}

		go func() {
			// Process in batches of 50 to maximize RPC efficiency
			for i := 0; i < len(missing); i += 50 {
				end := i + 50
				if end > len(missing) {
					end = len(missing)
				}
				batch := missing[i:end]

				// Use background context for async warmup
				_, ratingsMap, _, err := batchFetchINDEXes(context.Background(), batch, 50)
				if err != nil {
					logger.Printf("[TELA] warmRatings batch fetch error: %v\n", err)
					continue
				}

				for scid, ratings := range ratingsMap {
					if ratings.Average > 0 || ratings.Likes > 0 || ratings.Dislikes > 0 {
						updateTelaSearchEntry(scid, func(e *INDEXwithRatings) {
							e.ratings = ratings
						})
					}
				}
			}
		}()
	}

	setTelaStatus := func(text string, clr color.Color) {
		if text == "" || clr == color.Transparent {
			telaStatus.Text = ""
			telaStatus.Color = clr
			fyne.Do(func() {
				telaStatus.Hide()
				telaStatus.Refresh()
				refreshTelaStatusBox()
			})
			return
		}
		if telaStatus.Text == text && telaStatus.Color == clr {
			return
		}
		telaStatus.Text = text
		telaStatus.Color = clr
		fyne.Do(func() {
			telaStatus.Show()
			telaStatus.Refresh()
			refreshTelaStatusBox()
		})
	}

	setResultsText := func(format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		const maxStatusLen = 50
		if len(s) > maxStatusLen {
			s = s[:maxStatusLen-3] + "..."
		}
		results.Text = s
	}

	setTelaProgress := func(value float64) {
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		fyne.Do(func() {
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			telaProgress.SetValue(value)
			refreshTelaStatusBox()
		})
	}

	var displayedTelaProgress float64

	showInfiniteTelaProgress := func() {
		fyne.Do(func() {
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			next := displayedTelaProgress + 0.04
			if next < 0.12 {
				next = 0.12
			}
			if next > 0.72 {
				next = 0.72
			}
			displayedTelaProgress = next
			telaProgress.SetValue(next)
			refreshTelaStatusBox()
		})
	}

	updateTelaProgress := func(value float64) {
		if value < displayedTelaProgress {
			value = displayedTelaProgress
		}
		if value > 0.99 {
			value = 0.99
		}
		displayedTelaProgress = value
		setTelaProgress(value)
	}

	showActiveTelaProgress := func(status string, value float64, initial bool) {
		telaViewActive.Store(true)
		if initial {
			results.Hide()
			telaStatus.Text = status
			telaStatus.Color = colors.Yellow
			telaStatus.Refresh()
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			telaProgress.SetValue(value)
			refreshTelaStatusBox()
			return
		}
		fyne.Do(func() {
			results.Hide()
			refreshTelaStatusBox()
		})
		setTelaStatus(status, colors.Yellow)
		updateTelaProgress(value)
	}

	resetTelaProgress = func() { displayedTelaProgress = 0 }

	completeTelaScanProgress := func() {
		displayedTelaProgress = 1
		setTelaProgress(1)
	}

	hideTelaProgress := func() {
		fyne.Do(func() {
			telaProgress.Hide()
			refreshTelaStatusBox()
		})
	}

	newTelaListItem := func() fyne.CanvasObject {
		heartBtn := widget.NewButtonWithIcon("", resourceHeartOutlineSvg, nil)
		heartBtn.Importance = widget.LowImportance

		activeBg := canvas.NewRectangle(color.Transparent)
		activeBg.SetMinSize(fyne.NewSize(0, scaleSize(39)))

		nameLabel := widget.NewLabel("")
		nameLabel.Alignment = fyne.TextAlignLeading
		nameLabel.Truncation = fyne.TextTruncateEllipsis
		nameLabel.Wrapping = fyne.TextWrapOff

		startCloseBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), nil)
		startCloseBtn.Importance = widget.LowImportance

		launchProgress := NewSlimProgressBar()
		launchProgress.SetBarMinSize(fyne.NewSize(0, scaleSize(10)))
		launchProgress.Hide()

		launchStatus := canvas.NewText("", colors.Yellow)
		launchStatus.TextSize = scaleFont(10)
		launchStatus.Alignment = fyne.TextAlignCenter
		launchStatus.Hide()

		ratingLabel := canvas.NewText("0.0", colors.Account)
		ratingLabel.TextSize = scaleFont(10)
		ratingLabel.TextStyle = fyne.TextStyle{Bold: true}

		ratingHex := canvas.NewImageFromResource(resourceTelaHexagonGray)
		ratingHex.SetMinSize(fyne.NewSize(scaleSize(24), scaleSize(28)))
		ratingHex.FillMode = canvas.ImageFillContain

		ratingContainer := container.NewStack(
			ratingHex,
			container.NewCenter(ratingLabel),
		)

		bottomSpacer := canvas.NewRectangle(color.Transparent)
		bottomSpacer.SetMinSize(fyne.NewSize(0, 1))

		appIcon := canvas.NewImageFromResource(resourceTelaIcon)
		appIcon.SetMinSize(fyne.NewSize(scaleSize(26), scaleSize(26)))
		appIcon.FillMode = canvas.ImageFillContain

		topRow := container.NewBorder(
			nil, bottomSpacer,
			container.NewHBox(appIcon, heartBtn, ratingContainer),
			container.NewPadded(startCloseBtn),
			container.NewPadded(nameLabel),
		)

		row := container.NewStack(
			activeBg,
			container.NewBorder(
				topRow,
				container.NewVBox(
					launchStatus,
					launchProgress,
				),
				nil, nil, nil,
			),
		)
		return row
	}

	updateTelaFavoriteButton := func(btn *widget.Button, scid string) {
		if engram.Disk == nil {
			btn.SetIcon(resourceHeartOutlineMutedSvg)
			btn.Disable()
			btn.OnTapped = nil
			return
		}

		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, scid) {
			btn.SetIcon(resourceFavsPng)
		} else {
			btn.SetIcon(resourceHeartOutlineSvg)
		}
		btn.Enable()
	}

	toggleTelaFavorite := func(scid string) {
		if engram.Disk == nil {
			errorText.Text = "No wallet connected"
			errorText.Color = colors.Gray
			errorText.Refresh()
			return
		}

		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, scid) {
			if err := RemoveTELAFavorite(walletAddress, scid); err != nil {
				errorText.Text = "Error removing favorite"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			errorText.Text = "Removed from favorites"
			errorText.Color = colors.Green
		} else {
			entry, ok := findTelaSearchEntry(scid)
			if !ok {
				errorText.Text = "Could not load TELA metadata"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if err := AddTELAFavorite(walletAddress, scid, entry.NameHdr, entry.DescrHdr, entry.IconHdr, entry.ratings.Average); err != nil {
				errorText.Text = "Error adding favorite"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			errorText.Text = "Added to favorites"
			errorText.Color = colors.Green
		}

		errorText.Refresh()
		if refreshFavoritesList != nil {
			refreshFavoritesList()
		}
		searchList.Refresh()
		favoritesList.Refresh()
	}

	configureTelaListRow := func(raw string, co fyne.CanvasObject) {
		row, ok := co.(*fyne.Container)
		if !ok || len(row.Objects) < 2 {
			return
		}

		activeBg, ok := row.Objects[0].(*canvas.Rectangle)
		if !ok {
			return
		}

		var heartBtn *widget.Button
		var nameLabel *widget.Label
		var startCloseBtn *widget.Button
		var launchProgress *slimProgressBar
		var launchStatus *canvas.Text
		var appIcon *canvas.Image
		var ratingLabel *canvas.Text
		var ratingHex *canvas.Image

		var walk func(fyne.CanvasObject)
		walk = func(obj fyne.CanvasObject) {
			switch v := obj.(type) {
			case *widget.Button:
				if heartBtn == nil {
					heartBtn = v
				} else if startCloseBtn == nil {
					startCloseBtn = v
				}
			case *widget.Label:
				if nameLabel == nil {
					nameLabel = v
				}
			case *slimProgressBar:
				launchProgress = v
			case *canvas.Text:
				if v.Color == colors.Yellow {
					launchStatus = v
				} else if v.Color == colors.Account {
					ratingLabel = v
				}
			case *canvas.Image:
				if appIcon == nil {
					appIcon = v
				} else if ratingHex == nil {
					ratingHex = v
				}
			case *fyne.Container:
				for _, child := range v.Objects {
					walk(child)
				}
			}
		}

		walk(row.Objects[1])
		if heartBtn == nil || nameLabel == nil || startCloseBtn == nil {
			return
		}

		name, scid := parseTelaListEntry(raw)
		nameLabel.SetText(name)

		if appIcon != nil {
			appIcon.Resource = resourceTelaIcon
			if entry, ok := findTelaSearchEntry(scid); ok && entry.IconHdr != "" {
				go func(currentSCID, iconURL, nameHdr string, imgObj *canvas.Image) {
					if img, err := handleImageURL(nameHdr, iconURL, fyne.NewSize(scaleSize(26), scaleSize(26))); err == nil {
						uiDo(func() {
							_, checkSCID := parseTelaListEntry(raw)
							if checkSCID == currentSCID {
								imgObj.Resource = img.Resource
								imgObj.Refresh()
							}
						})
					}
				}(scid, entry.IconHdr, entry.NameHdr, appIcon)
			}
			appIcon.Refresh()
		}

		if ratingLabel != nil && ratingHex != nil {
			ratingLabel.Text = "0.0"
			ratingHex.Resource = resourceTelaHexagonGray

			if entry, ok := findTelaSearchEntry(scid); ok {
				if entry.ratings.Average > 0 {
					ratingLabel.Text = fmt.Sprintf("%.1f", entry.ratings.Average)
					ratingHex.Resource = telaHexagonColor(entry.ratings)
				} else {
					go func(currentSCID string, label *canvas.Text, hex *canvas.Image) {
						if _, loading := activeRatingFetches.LoadOrStore(currentSCID, true); loading {
							return
						}
						defer activeRatingFetches.Delete(currentSCID)

						ratings, err := tela.GetRating(currentSCID, session.Daemon, 0)
						if err == nil && (ratings.Average > 0 || ratings.Dislikes > ratings.Likes) {
							uiDo(func() {
								_, checkSCID := parseTelaListEntry(raw)
								if checkSCID == currentSCID {
									label.Text = fmt.Sprintf("%.1f", ratings.Average)
									hex.Resource = telaHexagonColor(ratings)
									label.Refresh()
									hex.Refresh()
								}
								updateTelaSearchEntry(currentSCID, func(e *INDEXwithRatings) {
									e.ratings = ratings
								})
							})
						}
					}(scid, ratingLabel, ratingHex)
				}
			}
			ratingLabel.Refresh()
			ratingHex.Refresh()
		}

		telaLaunchingSCIDsGlobal.Lock()
		isLaunching := telaLaunchingSCIDsGlobal.m[scid]
		telaLaunchingSCIDsGlobal.Unlock()

		telaStoppingSCIDsGlobal.Lock()
		isStopping := telaStoppingSCIDsGlobal.m[scid]
		telaStoppingSCIDsGlobal.Unlock()

		if isLaunching {
			if launchProgress != nil {
				launchProgress.Show()
			}
			if launchStatus != nil {
				if isStopping {
					launchStatus.Text = "Stopping..."
				} else {
					launchStatus.Text = "Starting..."
				}
				launchStatus.Show()
			}
			startCloseBtn.SetText("Cancel")
			startCloseBtn.SetIcon(theme.CancelIcon())
			startCloseBtn.Enable()

			// Sync UI with existing launch progress
			if _, loaded := activeRowUpdaters.LoadOrStore(co, scid); !loaded {
				go func(targetRow fyne.CanvasObject, rowSCID string) {
					defer activeRowUpdaters.Delete(targetRow)

					telaLaunchStartTimesGlobal.Lock()
					startTime, ok := telaLaunchStartTimesGlobal.m[rowSCID]
					telaLaunchStartTimesGlobal.Unlock()
					if !ok {
						return
					}

					const cap = 0.95
					const tau = 10.0
					for {
						// Check if this row is still assigned to the same SCID
						if current, ok := activeRowUpdaters.Load(targetRow); !ok || current != rowSCID {
							return
						}

						telaLaunchingSCIDsGlobal.Lock()
						stillLaunching := telaLaunchingSCIDsGlobal.m[rowSCID]
						telaLaunchingSCIDsGlobal.Unlock()
						if !stillLaunching {
							return
						}

						elapsed := time.Since(startTime).Seconds()
						val := cap * (1.0 - math.Exp(-elapsed/tau))
						if val > cap {
							val = cap
						}

						uiDo(func() {
							if launchProgress != nil && !launchProgress.Hidden {
								launchProgress.SetValue(val)
							}
							if launchStatus != nil && !launchStatus.Hidden {
								telaStoppingSCIDsGlobal.Lock()
								isStopping := telaStoppingSCIDsGlobal.m[rowSCID]
								telaStoppingSCIDsGlobal.Unlock()
								if isStopping {
									launchStatus.Text = "Stopping..."
								} else {
									if val < 0.30 {
										launchStatus.Text = "Connecting to node..."
									} else if val < 0.60 {
										launchStatus.Text = "Fetching content..."
									} else if val < 0.85 {
										launchStatus.Text = "Preparing app..."
									} else {
										launchStatus.Text = "Almost ready..."
									}
								}
							}
						})
						time.Sleep(200 * time.Millisecond)
					}
				}(co, scid)
			}
		} else if isTelaActive(scid) {
			if launchProgress != nil {
				launchProgress.Hide()
			}
			if launchStatus != nil {
				launchStatus.Hide()
			}
			activeBg.FillColor = color.NRGBA{R: 20, G: 120, B: 70, A: 48}
			startCloseBtn.SetText("Close")
			startCloseBtn.SetIcon(theme.MediaStopIcon())
			startCloseBtn.Enable()
		} else {
			if launchProgress != nil {
				launchProgress.Hide()
			}
			if launchStatus != nil {
				launchStatus.Hide()
			}
			activeBg.FillColor = color.Transparent
			startCloseBtn.SetText("Start")
			startCloseBtn.SetIcon(theme.MediaPlayIcon())
			startCloseBtn.Enable()
		}
		activeBg.Refresh()
		updateTelaFavoriteButton(heartBtn, scid)
		heartBtn.OnTapped = func() {
			toggleTelaFavorite(scid)
		}
		startCloseBtn.OnTapped = func() {
			telaLaunchingSCIDsGlobal.Lock()
			isLaunching := telaLaunchingSCIDsGlobal.m[scid]
			telaLaunchingSCIDsGlobal.Unlock()

			if isLaunching {
				telaStoppingSCIDsGlobal.Lock()
				telaStoppingSCIDsGlobal.m[scid] = true
				telaStoppingSCIDsGlobal.Unlock()

				telaLaunchCancelChansGlobal.Lock()
				if cancelChan, ok := telaLaunchCancelChansGlobal.m[scid]; ok {
					close(cancelChan)
					delete(telaLaunchCancelChansGlobal.m, scid)
				}
				telaLaunchCancelChansGlobal.Unlock()
				if launchStatus != nil {
					launchStatus.Text = "Stopping..."
					launchStatus.Refresh()
				}
				startCloseBtn.SetIcon(theme.ContentCutIcon())
				startCloseBtn.Refresh()
			} else if isTelaActive(scid) {
				entry, ok := findTelaSearchEntry(scid)
				if ok {
					go func() {
						tela.ShutdownServer(entry.DURL)
						if refreshServerList != nil {
							refreshServerList()
						}
						uiDo(func() {
							searchList.Refresh()
							favoritesList.Refresh()
						})
					}()
				}
			} else {
				if engram.Disk == nil {
					errorText.Text = "No wallet connected"
					errorText.Color = colors.Gray
					errorText.Refresh()
					return
				}

				telaLaunchingSCIDsGlobal.Lock()
				if telaLaunchingSCIDsGlobal.m[scid] {
					telaLaunchingSCIDsGlobal.Unlock()
					return
				}
				telaLaunchingSCIDsGlobal.m[scid] = true
				telaLaunchingSCIDsGlobal.Unlock()

				cancelChan := make(chan struct{})
				telaLaunchCancelChansGlobal.Lock()
				telaLaunchCancelChansGlobal.m[scid] = cancelChan
				telaLaunchCancelChansGlobal.Unlock()

				telaLaunchStartTimesGlobal.Lock()
				telaLaunchStartTimesGlobal.m[scid] = time.Now()
				telaLaunchStartTimesGlobal.Unlock()

				if launchStatus != nil {
					launchStatus.Text = "Starting..."
					launchStatus.Show()
				}
				activeBg.Refresh()
				startCloseBtn.SetText("Cancel")
				startCloseBtn.SetIcon(theme.CancelIcon())
				searchList.Refresh()
				favoritesList.Refresh()

				progressDone := make(chan struct{})
				var cancelled atomic.Bool
				// Progress updates are now handled by configureTelaListRow's sync goroutine
				// which is triggered by the searchesList.Refresh() below.

				cleanupLaunch := func(failed, cancelledLaunch bool) {
					close(progressDone)
					telaLaunchingSCIDsGlobal.Lock()
					delete(telaLaunchingSCIDsGlobal.m, scid)
					telaLaunchingSCIDsGlobal.Unlock()
					telaLaunchCancelChansGlobal.Lock()
					delete(telaLaunchCancelChansGlobal.m, scid)
					telaLaunchCancelChansGlobal.Unlock()
					telaStoppingSCIDsGlobal.Lock()
					delete(telaStoppingSCIDsGlobal.m, scid)
					telaStoppingSCIDsGlobal.Unlock()
					telaLaunchStartTimesGlobal.Lock()
					delete(telaLaunchStartTimesGlobal.m, scid)
					telaLaunchStartTimesGlobal.Unlock()
					uiDo(func() {
						if launchProgress != nil {
							if failed || cancelledLaunch {
								launchProgress.SetValue(launchProgress.value)
								launchProgress.Refresh()
							} else {
								launchProgress.SetValue(1.0)
								launchProgress.Refresh()
							}
						}
						if launchStatus != nil {
							if cancelledLaunch {
								launchStatus.Text = "Cancelled"
								launchStatus.Color = colors.Gray
							} else if failed {
								launchStatus.Text = "Launch Error"
								launchStatus.Color = colors.Red
							} else {
								launchStatus.Text = "Done!"
								launchStatus.Color = colors.Green
							}
							launchStatus.Refresh()
						}
						if launchProgress != nil {
							launchProgress.Hide()
						}
						if launchStatus != nil {
							launchStatus.Hide()
						}
						activeBg.SetMinSize(fyne.NewSize(0, scaleSize(40)))
						activeBg.Refresh()
					})

					if refreshServerList != nil {
						refreshServerList()
					}

					uiDo(func() {
						searchList.Refresh()
						favoritesList.Refresh()
					})
				}

				errorText.Text = ""
				errorText.Refresh()
				go func() {
					select {
					case <-progressDone:
						return
					case <-cancelChan:
						cancelled.Store(true)
						return
					}
				}()

				go func() {
					openURLAfterDelay := func(link string) {
						if a.Driver().Device().IsMobile() {
							time.Sleep(2 * time.Second)
						}
						if u, err := url.Parse(link); err == nil {
							fyne.CurrentApp().OpenURL(u)
						}
					}

					link, err := serveTELAWithStaleRecovery(scid, session.Daemon, &cancelled)
					if cancelled.Load() {
						if err == nil {
							tela.ShutdownServer(scid)
						}
						cleanupLaunch(false, true)
						return
					}

					if err == nil {
						pushTELANavigation(scid)
						go openURLAfterDelay(link)
						if err := StoreEncryptedValue("TELA History", []byte(scid), []byte("")); err != nil {
							logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
						}
						cleanupLaunch(false, false)
					} else {
						if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
							telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", scid)}
							linkPermission, permErr := AskPermissionForRequestE("Allow Updated Content", telaLink)
							if permErr != nil {
								logger.Errorf("[Engram] Open TELA link: %s\n", permErr)
								fyne.Do(func() {
									errorText.Text = "error could not open TELA"
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							if linkPermission != xswd.Allow {
								cleanupLaunch(true, false)
								return
							}

							link, updateErr := serveTELAUpdates(scid)
							if updateErr != nil {
								logger.Errorf("[Engram] Error serving TELA: %s\n", updateErr)
								fyne.Do(func() {
									errorText.Text = telaErrorToString(updateErr)
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							pushTELANavigation(scid)
							go openURLAfterDelay(link)
							cleanupLaunch(false, false)
						} else {
							logger.Printf("[TELA] ServeTELA failed for SCID %s: %v", scid, err)
							fyne.Do(func() {
								errorText.Text = "error starting TELA app"
								errorText.Color = colors.Red
								errorText.Refresh()
							})
							cleanupLaunch(true, false)
						}
					}
				}()
			}
		}
	}

	historyData = binding.BindStringList(&history)
	historyList = widget.NewListWithData(historyData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	searchList = widget.NewListWithData(searchData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	servingData = binding.BindStringList(&serving)
	servingList = widget.NewListWithData(servingData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectLeft,
						widget.NewLabel(""),
					),
					container.NewStack(
						rectRight,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, ";;;")
			if len(split) < 2 {
				return
			}

			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			})
		},
	)

	favoritesData = binding.BindStringList(&favorites)
	favoritesList = widget.NewListWithData(favoritesData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	entryHistory := widget.NewEntry()
	entryHistory.PlaceHolder = "Search History"
	entryHistory.SetIcon(theme.SearchIcon())
	entryHistory.Disable()

	entryServeSCID := widget.NewEntry()
	entryServeSCID.PlaceHolder = "Serve by SCID"

	entryAddSCID := widget.NewEntry()
	entryAddSCID.PlaceHolder = "Add SCID"
	entryAddSCID.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		if len(s) == 64 {
			if gnomon.Index != nil {
				if gnomon.GetAllSCIDVariableDetails(s) != nil {
					errorText.Text = "scid already exists"
					errorText.Color = colors.Yellow
					errorText.Refresh()
					return
				}

				code, err := getContractCode(s)
				if err != nil || code == "" {
					errorText.Text = "could not get scid"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				err = gnomon.AddSCIDToIndex(s)
				if err != nil {
					errorText.Text = "error adding scid"
					errorText.Color = colors.Red
					errorText.Refresh()
					return
				}

				entryAddSCID.SetText("")
				errorText.Text = "scid added"
				errorText.Color = colors.Green
				errorText.Refresh()
			}
		}
	}

	entrySearchCompletions := []string{"author:", "durl:", "name:", "my:"}
	entrySearch := x.NewCompletionEntry(entrySearchCompletions)
	entrySearch.PlaceHolder = "Search TELA"
	entrySearch.SetIcon(theme.SearchIcon())
	entrySearch.Disable()

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	var isSearching bool

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Domain = "app.wallet" // break any loops now
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	btnSettingsTela := newSizedIconButton(theme.SettingsIcon(), func() {
		session.Domain = "app.tela.settings" // Mark as coming from TELA
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	})

	linkClearHistory := widget.NewHyperlinkWithStyle("Clear All", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: false})
	linkClearHistory.OnTapped = func() {
		verificationOverlay(
			false,
			"TELA BROWSER",
			"Clear history?",
			"Confirm",
			func(b bool) {
				if b {
					if gnomon.Index == nil || session.Offline {
						return
					}

					shard, err := GetShard()
					if err != nil {
						return
					}

					store, err := graviton.NewDiskStore(shard)
					if err != nil {
						return
					}

					ss, err := store.LoadSnapshot(0)
					if err != nil {
						return
					}

					tree, err := ss.GetTree("TELA History")
					if err != nil {
						return
					}

					c := tree.Cursor()

					for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
						DeleteKey(tree.GetName(), k)
					}

					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutTELA())
				}
			},
		)
	}

	wSelect := widget.NewSelect([]string{"Search", "Favorites", "History"}, nil)
	wSelect.SetSelectedIndex(0)

	btnRescanTela := newSizedIconButton(theme.ViewRefreshIcon(), func() {
		rescanLabel := widget.NewLabel("Force Full Rescan?\n\nThis will clear all cached results, reset the Gnomon index, and perform a complete fresh scan. This usually takes less than a minute.")
		rescanLabel.Wrapping = fyne.TextWrapWord

		dlg := dialog.NewCustomWithoutButtons("TELA BROWSER", rescanLabel, session.Window)

		btnConfirm := widget.NewButtonWithIcon("Rescan", theme.ViewRefreshIcon(), func() {
			dlg.Hide()
			clearAllTELACache()
			forceFreshScan = true
			searchMu.Lock()
			searching = []string{}
			telaSearch = []INDEXwithRatings{}
			searchMu.Unlock()
			searchData.Set(searching)

			generation := currentWalletGeneration()

			results.Text = "  Resetting Gnomon index..."
			results.Color = colors.Yellow
			uiDo(func() {
				if !isWalletGenerationActive(generation) {
					return
				}
				results.Refresh()
			})

			go func() {
				if err := resetGnomonIndex(); err != nil {
					logger.Errorf("[TELA] Could not reset gnomon index: %s\n", err)
					return
				}

				for i := 0; i < 60; i++ {
					if !isWalletGenerationActive(generation) {
						return
					}
					time.Sleep(time.Second)
					if gnomon.Index != nil {
						break
					}
				}

				uiDo(func() {
					if !isWalletGenerationActive(generation) {
						return
					}
					wSelect.SetSelected("Search")
				})
			}()
		})

		btnCancel := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
			dlg.Hide()
		})

		dlg.SetButtons([]fyne.CanvasObject{wrapMobileButton(btnConfirm), btnCancel})
		dlg.Show()
	})

	activateTelaSearch := func() {}

	btnSortOrder := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), nil)
	if !sortDescending {
		btnSortOrder.SetIcon(theme.MenuDropUpIcon())
	}
	btnSortOrder.Importance = widget.LowImportance
	btnSortOrder.OnTapped = func() {
		sortDescending = !sortDescending
		if sortDescending {
			btnSortOrder.SetIcon(theme.MenuDropDownIcon())
			setTELADual("Sort Order", []byte("Descending"))
		} else {
			btnSortOrder.SetIcon(theme.MenuDropUpIcon())
			setTELADual("Sort Order", []byte("Ascending"))
		}

		if wSelect.Selected == "Search" {
			searchMu.RLock()
			searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
			searchMu.RUnlock()
			_ = searchData.Set(searching)
			searchList.Refresh()
		}
	}

	btnTela := widget.NewButtonWithIcon("Apps", resourceBrowserGlobeSvg, func() {
		if wSelect.Selected == "Search" {
			activateTelaSearch()
			return
		}
		wSelect.SetSelected("Search")
	})
	btnTela.Importance = widget.LowImportance

	favoritesLabel := "Favorites"

	btnFavorites := widget.NewButtonWithIcon(favoritesLabel, resourceFavsPng, func() {
		wSelect.SetSelected("Favorites")
	})
	btnFavorites.Importance = widget.LowImportance

	btnHistory := widget.NewButtonWithIcon("History", theme.HistoryIcon(), func() {
		wSelect.SetSelected("History")
	})
	btnHistory.Importance = widget.LowImportance

	// Horizontal button row (like dashboard)
	var tabButtons fyne.CanvasObject
	if isMobileDevice() {
		// Use HBox instead of Grid on mobile to allow wide buttons (with text) to fit correctly.
		// We use a narrower size enforcer for the sort button to save space.
		sortSize := canvas.NewRectangle(color.Transparent)
		sortSize.SetMinSize(scalePoint(40, 48))
		btnSortOrderMobile := container.NewStack(sortSize, btnSortOrder)

		tabButtons = container.NewHBox(
			btnSortOrderMobile,
			wrapMobileButton(btnTela),
			wrapMobileButton(btnFavorites),
			wrapMobileButton(btnHistory),
		)
	} else {
		tabButtons = container.NewHBox(
			btnSortOrder,
			btnTela,
			btnFavorites,
			btnHistory,
		)
	}

	btnShutdown := widget.NewButton("Shutdown TELA", nil)

	var restrictiveMode, rescanRecheck bool
	var lastScan, searchExclusions string
	var minLikes float64
	var telaSCIDs []string
	var sAll = map[string]bool{}
	// Initialize TELA settings from storage
	if storedMinLikes, found := getTELADual("Min Likes"); found {
		if f, err := strconv.ParseFloat(storedMinLikes, 64); err == nil {
			minLikes = f
		}
	} else {
		minLikes = 30
	}

	if storedExclusions, found := getTELADual("Exclusions"); found {
		searchExclusions = storedExclusions
	}

	if storedRescanRecheck, found := getTELADual("Rescan Recheck"); found {
		if storedRescanRecheck == "Yes" {
			rescanRecheck = true
		}
	}

	sortByOptions := []string{"Ratings", "A-Z", "Z-A"}
	if storedSortBy, found := getTELADual("Sort By"); found {
		sortBy = storedSortBy
	} else {
		sortBy = sortByOptions[0]
	}

	if storedSortOrder, found := getTELADual("Sort Order"); found {
		if storedSortOrder == "Ascending" {
			sortDescending = false
		}
	}

	restrictiveMode = false // Default OFF (unrestrictive)
	// First check new "Restrictive Mode" key (set by Settings page)
	if restrictiveModeValue, found := getTELADual("Restrictive Mode"); found {
		if restrictiveModeValue == "true" {
			restrictiveMode = true
		}
	} else {
		// Fallback to legacy "Mode" key for backward compatibility
		if storedTelaMode, found := getTELADual("Mode"); found {
			if storedTelaMode == "Restrictive" {
				restrictiveMode = true
			}
		}
	}

	var getSearchResults func()
	hasTelaCache = func() bool {
		if raw, err := GetEncryptedValue("TELA Search", []byte("DisplayCache")); err == nil && len(raw) > 0 {
			var cachedDisplay telaDisplayCache
			if json.Unmarshal(raw, &cachedDisplay) == nil && len(cachedDisplay) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("SCIDs")); err == nil && len(raw) > 0 {
			var cachedSCIDs []string
			if json.Unmarshal(raw, &cachedSCIDs) == nil && len(cachedSCIDs) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("IndexCache")); err == nil && len(raw) > 0 {
			var cachedINDEXes indexCache
			if json.Unmarshal(raw, &cachedINDEXes) == nil && len(cachedINDEXes) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("CandidateCache")); err == nil && len(raw) > 0 {
			var cachedCandidates telaCandidateCache
			if json.Unmarshal(raw, &cachedCandidates) == nil {
				for _, meta := range cachedCandidates {
					if meta.Result == telaCandidateValidIndex {
						return true
					}
				}
			}
		}
		return false
	}
	getSearchResults = func() {
		if !telaWorkActive.CompareAndSwap(false, true) {
			return
		}
		defer func() {
			telaLaunchPending.Store(false)
			telaWorkActive.Store(false)
			// Clear network-paused flag if the wallet session has changed (closed/switched).
			// The paused-retry goroutine checks the flag itself, but this is a safety net.
			if !isWalletGenerationActive(currentWalletGeneration()) {
				telaNetworkPaused.Store(false)
			}
			if r := recover(); r != nil {
				logger.Errorf("[TELA-SEARCH] getSearchResults PANIC recovered: %v\n", r)
				isSearching = false
				scheduleTelaWarmup()
			}
		}()
		logger.Printf("[TELA-SEARCH] getSearchResults() starting...\n")
		scanStart := time.Now()
		scanCtx, scanCancel := context.WithCancel(context.Background())
		defer scanCancel()
		var syncWaitSeconds int
		var storedSCIDsCount int
		var allCandidates int
		var scannedCandidates int64
		var versionHits int64
		var indexInfoCalls int64
		var retryCount int64
		var filteredNonDisplayable int64
		var filteredByExclusion int64
		var filteredByMinLikes int64
		var preDispatchSkips int64
		var negCacheSkips int64
		var prefilterPassed int64
		var prefilterDropped int64
		var uiRefreshCount int64
		var progressWriteCount int64
		var interruptReason string
		var currentHeight int64
		var phasePrefilterMs int64
		var phaseScanMs int64
		var phaseFinalizeMs int64
		cacheHitMode := "full"
		fullScanReason := "cold_start"
		cacheIntegrity := "ok"
		keepProgressVisible := true
		var heightDelta int64
		var storedIndexedHeight int64

		var gnomonSyncStartTime time.Time
		var estimatedTelaFallback = 30 * time.Second

		currentDaemonHeight := func() int64 {
			if engram.Disk == nil {
				return 0
			}

			return int64(engram.Disk.Get_Daemon_Height())
		}

		isGnomonCaughtUp := func() bool {
			if gnomon.Index == nil {
				return false
			}

			daemonHeight := currentDaemonHeight()
			if daemonHeight <= 0 {
				return gnomon.Index.LastIndexedHeight > 0
			}

			return gnomon.Index.LastIndexedHeight >= daemonHeight
		}

		allowTelaIndexMutations := isGnomonCaughtUp()

		deviceClass := "desktop"
		workerPoolSize := runtime.NumCPU()
		uiRefreshInterval := 250 * time.Millisecond
		progressCheckpointInterval := 2 * time.Second
		if a.Driver().Device().IsMobile() {
			deviceClass = "mobile"
			workerPoolSize = runtime.NumCPU() / 2
			if workerPoolSize < 6 {
				workerPoolSize = 6
			}
			if workerPoolSize > 12 {
				workerPoolSize = 12
			}
			uiRefreshInterval = 500 * time.Millisecond
			progressCheckpointInterval = 4 * time.Second
		} else {
			workerPoolSize = runtime.NumCPU() * 2
			if workerPoolSize < 16 {
				workerPoolSize = 16
			}
			if workerPoolSize > 64 {
				workerPoolSize = 64
			}
		}

		saveProgress := func(position, total int, scid, state string) {
			saveScanProgress(position, total, scid, state)
			atomic.AddInt64(&progressWriteCount, 1)
		}

		fyne.Do(func() {
			entrySearch.Disable()
			entryAddSCID.Disable()
		})
		if isSearching {
			return
		}

		isSearching = true

		// Handle force fresh scan - clear all caches and proceed
		if forceFreshScan {
			logger.Printf("[TELA] Force fresh scan requested - clearing all caches\n")
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			telaSCIDs = []string{}
			searchMu.Unlock()
			sAll = map[string]bool{}
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))
			forceFreshScan = false
			clearScanProgress()
			fullScanReason = "force_fresh_scan"
		}

		// On re-visit telaSearch is empty because it's a local variable in layoutTELA().
		// Load cached display results so we can show them immediately.
		searchMu.Lock()
		if len(telaSearch) == 0 {
			cachedDisplay := loadTelaDisplayCache()
			if len(cachedDisplay) > 0 {
				for _, entry := range cachedDisplay {
					if !isDisplayableTelaApp(entry.INDEX) {
						continue
					}
					telaSearch = append(telaSearch, entry)
				}
				telaSearch = deduplicateTelaSearch(telaSearch)
				logger.Printf("[TELA] Loaded %d apps from display cache into telaSearch\n", len(telaSearch))
			}
		}
		searchMu.Unlock()
		warmRatings()

		// Check for existing progress and handle resume scenarios
		progress := loadScanProgress()
		resumePosition := 0

		if progress.State == "completed" && !isScanProgressStale(progress, 24) {
			// Use cached results - progress is valid, already scanned
		} else if progress.State == "interrupted" && !isScanProgressStale(progress, 24) {
			// Resume from interrupted scan
			resumePosition = progress.Position
			setResultsText("  Resuming scan from position %d...", resumePosition)
			results.Color = colors.Yellow
			fyne.Do(func() {
				results.Refresh()
			})
		} else if progress.State == "interrupted" && isScanProgressStale(progress, 24) {
			// Clear stale interrupted progress
			clearScanProgress()
		}

		if gnomon.Index != nil {
			if storedHeightRaw, err := GetEncryptedValue("TELA Search", []byte("Last Indexed Height")); err == nil {
				if parsedHeight, parseErr := strconv.ParseInt(string(storedHeightRaw), 10, 64); parseErr == nil {
					storedIndexedHeight = parsedHeight
					heightDelta = gnomon.Index.LastIndexedHeight - storedIndexedHeight
					if heightDelta < 0 {
						heightDelta = 0
					}
				} else {
					cacheIntegrity = "missing_height"
				}
			} else {
				cacheIntegrity = "missing_height"
			}
		}
		if rescanRecheck {
			fullScanReason = "rescan_recheck"
		} else if heightDelta > 0 {
			cacheHitMode = "delta"
			fullScanReason = "height_delta"
		}

		// Already scanned - only skip if no updates are expected
		searchMu.RLock()
		hasCached := len(telaSearch) > 0
		searchMu.RUnlock()
		if hasCached && heightDelta == 0 && !rescanRecheck {
			keepProgressVisible = false
			fyne.Do(func() {
				searchMu.Lock()
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				searchMu.Unlock()
				searchData.Set(searching)
				searchList.Refresh()
				searchMu.RLock()
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(telaSearch))
				searchMu.RUnlock()
				results.Color = colors.Green
				entrySearch.Enable()
				entryAddSCID.Enable()
			})

			labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
			labelLastScan.Color = colors.Green
			isSearching = false

			fyne.Do(func() {
				results.Refresh()
				labelLastScan.Refresh()
				hideTelaProgress()
			})

			return
		}

		if !keepProgressVisible && heightDelta == 0 && !rescanRecheck {
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			searchMu.Unlock()
			searchData.Set(nil)
		}
		labelLastScan.Text = ""

		fyne.Do(func() {
			btnShutdown.Disable()
			labelLastScan.Refresh()
		})

		defer func() {
			isSearching = false
			if !keepProgressVisible {
				setTelaStatus("", color.Transparent)
				hideTelaProgress()
			}
			fyne.Do(func() {
				entrySearch.Enable()
				entryAddSCID.Enable()
			})
			if !session.Offline && gnomon.Index != nil {
				if btnShutdown.Disabled() {
					fyne.Do(func() {
						btnShutdown.Enable()
					})
				}
			}
		}()

		// hasValidTelaJSONCache checks for a recent (< 24h) plain JSON cache.
		// If present, we can skip the Gnomon sync wait entirely — we already
		// know which SCIDs are TELA candidates from a previous prefilter run.
		hasValidTelaJSONCache := func() bool {
			cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
			raw, err := os.ReadFile(cachePath)
			if err != nil || len(raw) == 0 {
				return false
			}
			var cache struct {
				SCIDs     []string `json:"scids"`
				Timestamp int64    `json:"timestamp"`
			}
			if err := json.Unmarshal(raw, &cache); err != nil || len(cache.SCIDs) == 0 {
				return false
			}
			if time.Now().Unix()-cache.Timestamp >= 86400 {
				return false
			}
			return true
		}

		gnomonReadyForTela := func() bool {
			// Embedded TELA SCIDs are always available — skip Gnomon sync wait entirely.
			// This makes the first TELA click fast even on a fresh install.
			if len(embeddedTelaSCIDs) > 0 {
				return true
			}
			// If we have a recent JSON cache, Gnomon doesn't need to be fully synced.
			// We already know which SCIDs are TELA candidates — skip the sync wait.
			if hasValidTelaJSONCache() {
				return true
			}
			if hasTelaCache() || len(telaSearch) > 0 {
				return true
			}
			if !gnomon.telaBootstrapReady() {
				return false
			}
			if gnomon.Index == nil {
				return false
			}
			if gnomon.Index.LastIndexedHeight <= 0 {
				return false
			}
			return isGnomonCaughtUp()
		}

		gnomonSyncStarted := false
		for !gnomonReadyForTela() {
			if !gnomonSyncStarted {
				gnomonSyncStartTime = time.Now()
				gnomonSyncStarted = true
			}
			syncWaitSeconds++
			// Check if user navigated away
			if !strings.Contains(session.Domain, ".tela") {
				interruptReason = "navigated_away"
				saveProgress(0, 0, "", "interrupted")
				return
			}

			// Check if Gnomon index became nil (stopped unexpectedly)
			if gnomon.Index == nil {
				interruptReason = "gnomon_nil_while_syncing"
				results.Text = "  Gnomon stopped unexpectedly"
				results.Color = colors.Red
				fyne.Do(func() {
					results.Refresh()
				})
				saveProgress(0, 0, "", "interrupted")
				return
			}

			// Check connection health - wait for reconnect if disconnected
			if !walletapi.Connected {
				interruptReason = "connection_lost_syncing"
				results.Text = "  Connection lost, waiting for reconnect..."
				results.Color = colors.Yellow
				fyne.Do(func() {
					results.Refresh()
				})

				// Wait for connection to restore (up to 30 seconds)
				reconnectAttempts := 0
				for !walletapi.Connected && reconnectAttempts < 30 {
					time.Sleep(time.Second)
					reconnectAttempts++

					// Check if user navigated away while waiting
					if !strings.Contains(session.Domain, ".tela") {
						saveProgress(0, 0, "", "interrupted")
						return
					}
				}

				// If still disconnected after 30 seconds, mark as interrupted
				if !walletapi.Connected {
					interruptReason = "connection_timeout_syncing"
					keepProgressVisible = true
					results.Text = "  Connection timeout"
					results.Color = colors.Red
					fyne.Do(func() {
						results.Refresh()
					})
					saveProgress(0, 0, "", "interrupted")
					return
				}

				// Connection restored - continue syncing
				results.Text = "  Connection restored, resuming sync..."
				results.Color = colors.Yellow
			}

			// Show time-based progress during sync wait
			if gnomon.Index != nil && engram.Disk != nil {
				daemonHeight := int64(engram.Disk.Get_Daemon_Height())
				indexedHeight := gnomon.Index.LastIndexedHeight
				elapsed := time.Since(gnomonSyncStartTime)
				remainingBlocks := daemonHeight - indexedHeight
				processedBlocks := indexedHeight
				var estimatedGnomon time.Duration
				if processedBlocks > 0 && remainingBlocks > 0 {
					timePerBlock := float64(elapsed) / float64(processedBlocks)
					remainingTime := time.Duration(timePerBlock * float64(remainingBlocks))
					estimatedGnomon = elapsed + remainingTime
				} else if processedBlocks > 0 {
					estimatedGnomon = elapsed
				} else {
					estimatedGnomon = elapsed + estimatedTelaFallback
				}
				provisionalTotal := estimatedGnomon + estimatedTelaFallback
				if provisionalTotal > 0 {
					syncProgress := float64(elapsed) / float64(provisionalTotal)
					if daemonHeight > 0 && indexedHeight > 0 {
						heightProgress := float64(indexedHeight) / float64(daemonHeight)
						// Ensure we start at a low value even if synced, moving towards 100%
						if heightProgress > 0.99 && syncProgress < 0.1 {
							syncProgress = 0.05 + (syncProgress * 0.5) // Start around 5% if Gnomon synced
						} else {
							syncProgress = (syncProgress * 0.4) + (heightProgress * 0.6)
						}
					}
					// Cap sync progress at 15% — real work (prefilter/scan) hasn't started yet
					if syncProgress > 0.15 {
						syncProgress = 0.15
					}
					updateTelaProgress(syncProgress)
				}
				setTelaStatus(fmt.Sprintf("Synching gnomon index... [%d / %d]", indexedHeight, daemonHeight), colors.Yellow)
				fyne.Do(func() {
					results.Refresh()
				})
			}

			fyne.Do(func() {
				entrySearch.Disable()
				entryAddSCID.Disable()
			})
			time.Sleep(1 * time.Second)
		}

		// Re-evaluate after the sync wait: the value captured at the start of getSearchResults
		// is stale if we blocked until Gnomon caught up, otherwise "defer cached only" can skip
		// the full owner/SCID scan incorrectly.
		if forceFreshScan {
			logger.Printf("[TELA] Force fresh scan observed after sync wait - clearing state\n")
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			telaSCIDs = []string{}
			sAll = map[string]bool{}
			searchMu.Unlock()
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))
			forceFreshScan = false
			clearScanProgress()
			fullScanReason = "force_fresh_scan"
		}
		allowTelaIndexMutations = isGnomonCaughtUp()

		// Gnomon sync complete - record duration and initialize TELA timing
		indexCacheStore := loadTelaIndexCache()
		ratingsCache := make(map[string]tela.Rating_Result)
		candidateCache := loadTelaCandidateCache()
		currentScanHeight := storedIndexedHeight
		var candidateCacheMu sync.RWMutex
		var indexMu sync.Mutex
		var scidsToIndex []string
		if gnomon.Index != nil {
			currentScanHeight = gnomon.Index.LastIndexedHeight
		}
		if !restrictiveMode {
			// Merge negative SCIDs from both in-memory candidate cache and persistent storage
			// for maximum cache hit rate across sessions.
			sAll = candidateCache.negativeSet()
			persistedNegatives := loadStringSetFromEncryptedStorage("TELA Search", "NegativeCache")
			for scid := range persistedNegatives {
				sAll[scid] = true
			}
			if len(sAll) > 0 {
				logger.Printf("[TELA] Loaded %d negative SCIDs from cache (%d from storage)\n", len(sAll), len(persistedNegatives))
			}
		}

		setCandidateCache := func(scid, result string) {
			candidateCacheMu.Lock()
			candidateCache.set(scid, result, currentScanHeight)
			candidateCacheMu.Unlock()
		}
		isNegativeSCID := func(scid string) bool {
			candidateCacheMu.RLock()
			defer candidateCacheMu.RUnlock()
			return sAll[scid]
		}
		setNegativeSCID := func(scid string, negative bool) {
			candidateCacheMu.Lock()
			if negative {
				sAll[scid] = true
			} else {
				delete(sAll, scid)
			}
			candidateCacheMu.Unlock()
		}

		cachedDisplay := loadTelaDisplayCache()
		if len(cachedDisplay) > 0 {
			for _, entry := range cachedDisplay {
				if !isDisplayableTelaApp(entry.INDEX) {
					continue
				}
				searchMu.Lock()
				telaSearch = append(telaSearch, entry)
				telaSCIDs = append(telaSCIDs, entry.SCID)
				searchMu.Unlock()
				indexCacheStore[entry.SCID] = entry.INDEX
			}
		}
		searchMu.Lock()
		telaSearch = deduplicateTelaSearch(telaSearch)
		searchMu.Unlock()

		storedSCIDs, err := GetEncryptedValue("TELA Search", []byte("SCIDs"))
		if err != nil {
			// Nothing stored, scan for SCIDs
			if len(telaSCIDs) == 0 {
				telaSCIDs = candidateCache.validSCIDs()
			}
			cacheIntegrity = "missing_scids"
			fullScanReason = "no_scid_cache"
			logger.Debugf("[Engram] Could not get stored TELA SCIDs: %s\n", err)
		} else {
			// Have stored SCIDs
			var unmarshaledSCIDs []string
			if err := json.Unmarshal(storedSCIDs, &unmarshaledSCIDs); err == nil {
				scidMap := make(map[string]bool)
				for _, sc := range telaSCIDs {
					scidMap[sc] = true
				}
				for _, sc := range unmarshaledSCIDs {
					if !scidMap[sc] {
						telaSCIDs = append(telaSCIDs, sc)
					}
				}
			}

			fyne.Do(func() {
				results.Refresh()
			})

			// Batch-fetch INDEX data for cached SCIDs missing from indexCacheStore
			// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket
			searchMu.RLock()
			var cacheMissed []string
			for _, sc := range telaSCIDs {
				if _, ok := indexCacheStore[sc]; !ok {
					cacheMissed = append(cacheMissed, sc)
				}
			}
			searchMu.RUnlock()
			if len(cacheMissed) > 0 {
				setResultsText("  Fetching INDEX data... (%d SCIDs)", len(cacheMissed))
				results.Color = colors.Yellow
				fyne.Do(func() {
					results.Refresh()
				})

				fetched, ratingsFetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, cacheMissed, 50)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for cached SCIDs: %v\n", fetchErr)
				}
				for scid, index := range fetched {
					indexCacheStore[scid] = index
					setCandidateCache(scid, telaCandidateValidIndex)
					setNegativeSCID(scid, false)
					if r, ok := ratingsFetched[scid]; ok {
						ratingsCache[scid] = r
					}
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(cacheMissed)))
			}

			searchMu.RLock()
			var searchMap = make(map[string]bool)
			for _, entry := range telaSearch {
				searchMap[entry.SCID] = true
			}
			searchMu.RUnlock()
			var missingSCIDs []string
			for _, sc := range telaSCIDs {
				if !searchMap[sc] {
					missingSCIDs = append(missingSCIDs, sc)
				}
			}

			if len(missingSCIDs) > 0 {
				cachedAdded := int64(0)
				cachedWorkers := workerPoolSize / 2
				if cachedWorkers < 8 {
					cachedWorkers = 8
				}
				if cachedWorkers > 24 {
					cachedWorkers = 24
				}
				cachedSlots := make(chan struct{}, cachedWorkers)
				var cachedWg sync.WaitGroup

				for i, sc := range missingSCIDs {
					if !walletapi.Connected {
						break
					}

					cachedSlots <- struct{}{}
					cachedWg.Add(1)
					go func(idx int, scid string) {
						defer func() {
							<-cachedSlots
							cachedWg.Done()
						}()

						var index tela.INDEX
						if cached, ok := indexCacheStore[scid]; ok {
							index = cached
						} else {
							return
						}

						if !isDisplayableTelaApp(index) {
							setCandidateCache(scid, telaCandidateNoDocs)
							setNegativeSCID(scid, true)
							return
						}

						if allowTelaIndexMutations && gnomon.GetAllSCIDVariableDetails(scid) == nil {
							if atomic.AddInt64(&cachedAdded, 1)%8 == 0 {
								setResultsText("  Adding... (%d / %d)", idx+1, len(missingSCIDs))
								results.Color = colors.Yellow
								fyne.Do(func() {
									results.Refresh()
								})
							}
							gnomon.AddSCIDToIndex(scid)
						}

						if restrictiveMode {
							setCandidateCache(scid, telaCandidateValidIndex)
							return
						}

						_, ratings, err := getLikesRatioCached(scid, index.DURL, searchExclusions, minLikes, ratingsCache)
						if err != nil {
							if strings.Contains(err.Error(), "found search exclusion") {
								atomic.AddInt64(&filteredByExclusion, 1)
							} else if strings.Contains(err.Error(), "below min rating setting") {
								atomic.AddInt64(&filteredByMinLikes, 1)
							}
							setCandidateCache(scid, telaCandidateExcludedByURL)
							return
						}

						setCandidateCache(scid, telaCandidateValidIndex)
						setNegativeSCID(scid, false)

						searchMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						searchMu.Unlock()
					}(i, sc)
				}

				cachedWg.Wait()
			}
			storedSCIDsCount = len(telaSCIDs)

			// Only defer the full scan when we have cached rows to show; otherwise continue
			// into GetAllOwnersAndSCIDs so an initial or empty-cache run still enumerates.
			searchMu.RLock()
			hasSearch := len(telaSearch) > 0
			hasSCIDs := len(telaSCIDs) > 0
			searchMu.RUnlock()
			if !allowTelaIndexMutations && (hasSearch || hasSCIDs) {
				cacheHitMode = "cached_syncing"
				fullScanReason = ""
				if !hasSearch && hasSCIDs {
					searchMu.RLock()
					localSCIDs := make([]string, len(telaSCIDs))
					copy(localSCIDs, telaSCIDs)
					searchMu.RUnlock()
					for _, scid := range localSCIDs {
						if index, ok := indexCacheStore[scid]; ok {
							if !isDisplayableTelaApp(index) {
								continue
							}
							searchMu.Lock()
							telaSearch = append(telaSearch, INDEXwithRatings{INDEX: index})
							searchMu.Unlock()
						}
					}
					warmRatings()
				}
				fyne.Do(func() {
					searchMu.Lock()
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					searchMu.Unlock()
					searchData.Set(searching)
					searchList.Refresh()
					searchMu.RLock()
					if len(telaSearch) > 0 {
						results.Text = fmt.Sprintf("  TELA cache loaded while Gnomon syncs: %d", len(telaSearch))
					} else {
						results.Text = "  Loading cached TELA data..."
					}
					searchMu.RUnlock()
					results.Color = colors.Yellow
					entrySearch.Enable()
					entryAddSCID.Enable()
				})

				if last, err := GetEncryptedValue("TELA Search", []byte("Last Scan")); err == nil {
					lastScan = string(last)
					labelLastScan.Text = fmt.Sprintf("  %s (syncing)", lastScan)
					labelLastScan.Color = colors.Yellow
				}

				fyne.Do(func() {
					results.Refresh()
					labelLastScan.Refresh()
				})

				keepProgressVisible = false
				completeTelaScanProgress()
				logger.Printf("[TELA] Deferring full scan until Gnomon catches up; showing cached results only\n")
				return
			}

			if !rescanRecheck && (len(telaSearch) > 0 || len(telaSCIDs) > 0) && heightDelta == 0 {
				cacheHitMode = "cached_only"
				fullScanReason = ""
				keepProgressVisible = false
				completeTelaScanProgress()
				fyne.Do(func() {
					searchMu.Lock()
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					searchMu.Unlock()
					searchData.Set(searching)
					searchList.Refresh()
					searchMu.RLock()
					results.Text = fmt.Sprintf("  TELA Apps:  %d", len(telaSearch))
					searchMu.RUnlock()
					results.Color = colors.Green
					entrySearch.Enable()
					entryAddSCID.Enable()
				})

				if last, err := GetEncryptedValue("TELA Search", []byte("Last Scan")); err == nil {
					lastScan = string(last)
					labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
					labelLastScan.Color = colors.Green
				}

				if restrictiveMode && len(telaSearch) < 1 {
					errorText.Text = "TELA is in restrictive mode"
					errorText.Color = colors.Yellow
				}

				fyne.Do(func() {
					results.Refresh()
					labelLastScan.Refresh()
					errorText.Refresh()
				})

				logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), versionHits, indexInfoCalls, retryCount, len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)

				return
			}
			if heightDelta > 0 {
				fullScanReason = "height_delta"
			}
		}

		var wg sync.WaitGroup

		hasCachedTelaData := hasTelaCache()
		var all = map[string]string{}
		usedPrecomputedCandidates := false
		if restrictiveMode {
			for _, sc := range telaSCIDs {
				all[sc] = ""
			}
		} else {
			if gnomon.Index == nil ||
				(gnomon.Index.DBType == "gravdb" && gnomon.Index.GravDBBackend == nil) ||
				(gnomon.Index.DBType == "boltdb" && gnomon.Index.BBSBackend == nil) {
				keepProgressVisible = true
				setTelaStatus("Waiting for Gnomon backend...", colors.Yellow)
				showInfiniteTelaProgress()
				scheduleTelaWarmup()
				return
			}
			// Fast path: use pre-computed TELA candidates if available
			candidates := gnomon.GetTelaCandidates()
			if len(candidates) > 0 {
				usedPrecomputedCandidates = true
				logger.Printf("[TELA-SEARCH] Using %d pre-computed TELA candidates from Gnomon\n", len(candidates))
				for _, scid := range candidates {
					all[scid] = ""
				}
				logger.Printf("[TELA-SEARCH] Candidate pool ready: %d total (backfillActive=%v backfillFailed=%v lastHeight=%d gnomonHeight=%d)\n",
					len(all), telaBackfillActive.Load(), telaBackfillFailed.Load(), lastBackfillHeight, currentHeight)
			} else {
				// Fallback 1: plain JSON file cache (no encryption, no Graviton, survives abrupt kills)
				cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
				if raw, err := os.ReadFile(cachePath); err == nil && len(raw) > 0 {
					var cache struct {
						SCIDs     []string `json:"scids"`
						Timestamp int64    `json:"timestamp"`
						Daemon    string   `json:"daemon"`
					}
					if err := json.Unmarshal(raw, &cache); err == nil && len(cache.SCIDs) > 0 {
						if time.Now().Unix()-cache.Timestamp < 86400 {
							usedPrecomputedCandidates = true
							logger.Printf("[TELA-SEARCH] Using %d validated TELA SCIDs from JSON cache (age=%dh, cached_daemon=%s, current_daemon=%s)\n", len(cache.SCIDs), (time.Now().Unix()-cache.Timestamp)/3600, cache.Daemon, session.Daemon)
							for _, scid := range cache.SCIDs {
								all[scid] = ""
							}
						} else {
							logger.Printf("[TELA-SEARCH] JSON cache stale (age=%dh, max=24h)\n", (time.Now().Unix()-cache.Timestamp)/3600)
						}
					} else {
						logger.Printf("[TELA-SEARCH] JSON cache unmarshal failed or empty: %v\n", err)
					}
				} else {
					logger.Printf("[TELA-SEARCH] JSON cache not found or unreadable: %v\n", err)
				}

				// Fallback 2: encrypted Graviton cache (legacy, often fails silently)
				if !usedPrecomputedCandidates {
					if raw, err := GetEncryptedValue("TELA Search", []byte("ValidatedSCIDs")); err == nil && len(raw) > 0 {
						var validated []string
						if err := json.Unmarshal(raw, &validated); err == nil && len(validated) > 0 {
							if tsRaw, err := GetEncryptedValue("TELA Search", []byte("ValidatedSCIDsTimestamp")); err == nil {
								var ts int64
								if err := json.Unmarshal(tsRaw, &ts); err == nil {
									if time.Now().Unix()-ts < 86400 {
										usedPrecomputedCandidates = true
										logger.Printf("[TELA-SEARCH] Using %d validated TELA SCIDs from encrypted cache (age=%dh)\n", len(validated), (time.Now().Unix()-ts)/3600)
										for _, scid := range validated {
											all[scid] = ""
										}
									}
								}
							}
						}
					}
				}

				if !usedPrecomputedCandidates {
					logger.Printf("[TELA-SEARCH] Fetching all indexed owners and SCIDs...\n")
					all = gnomon.GetAllOwnersAndSCIDs()
					logger.Printf("[TELA-SEARCH] Found %d total indexed SCIDs\n", len(all))
				}
			}
		}

		if !restrictiveMode && !hasCachedTelaData && len(all) <= 1 {
			keepProgressVisible = true
			setTelaStatus("Gnomon indexing in progress...", colors.Yellow)
			showInfiniteTelaProgress()
			scheduleTelaWarmup()
			return
		}

		allSCIDs := make([]string, 0, len(all))
		for sc := range all {
			allSCIDs = append(allSCIDs, sc)
		}
		sort.Strings(allSCIDs)

		// Delta scan block removed as it drops valid SCIDs with no interaction heights.

		// Create set of known TELA SCIDs for O(1) lookup
		knownTelaSCIDs := make(map[string]bool, len(telaSCIDs))
		for _, sc := range telaSCIDs {
			knownTelaSCIDs[sc] = true
		}

		prefilterAllowed := map[string]bool{}
		if !restrictiveMode {
			if usedPrecomputedCandidates {
				// Candidates from GetTelaCandidates() are already verified to have telaVersion.
				// Skip the expensive RPC prefilter entirely.
				logger.Printf("[TELA-SEARCH] Skipping prefilter for %d pre-computed candidates\n", len(allSCIDs))
				for _, sc := range allSCIDs {
					if !rescanRecheck && isNegativeSCID(sc) {
						prefilterAllowed[sc] = false
					} else {
						prefilterAllowed[sc] = true
					}
				}
				updateTelaProgress(0.60)
			} else {
				candidates := make([]string, 0, len(allSCIDs))
				for _, sc := range allSCIDs {
					if !rescanRecheck && isNegativeSCID(sc) {
						prefilterAllowed[sc] = false
						continue
					}
					// Skip prefilter for SCIDs with cached INDEX data
					if _, hasIndexData := indexCacheStore[sc]; hasIndexData {
						prefilterAllowed[sc] = true
						continue
					}
					// Skip prefilter for known TELA SCIDs from storage
					if knownTelaSCIDs[sc] {
						prefilterAllowed[sc] = true
						continue
					}
					candidates = append(candidates, sc)
				}

				setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d total)", len(candidates)), colors.Yellow)
				displayedTelaProgress = 0.10
				setTelaProgress(0.10)
				uiDo(func() {
					results.Refresh()
				})

				prefilterStart := time.Now()

				// Create a dedicated RPC pool for prefilter.
				poolSize := 6
				if !a.Driver().Device().IsMobile() {
					poolSize = 8
				}
				batchSize := 200
				if !a.Driver().Device().IsMobile() {
					batchSize = 500
				}
				// Reduce concurrency for remote daemons to avoid connection overwhelm.
				daemonLower := strings.ToLower(session.Daemon)
				if !strings.Contains(daemonLower, "127.0.0.1") && !strings.Contains(daemonLower, "localhost") && !strings.HasPrefix(daemonLower, ":") {
					if poolSize > 4 {
						poolSize = 4
					}
					if batchSize > 200 {
						batchSize = 200
					}
				}
				pool, poolCleanup, poolErr := dialRPCPool(session.Daemon, poolSize)
				if poolErr != nil {
					logger.Printf("[TELA] Failed to create RPC pool (%d connections): %v\n", poolSize, poolErr)
					if gnomon.Index != nil && gnomon.Index.RPC != nil && gnomon.Index.RPC.RPC != nil {
						pool = []*jrpc2.Client{gnomon.Index.RPC.RPC}
						poolCleanup = func() {}
					} else {
						pool = nil
						poolCleanup = func() {}
					}
				}

				var passed map[string]bool
				var batchStats batchPrefilterStats
				var batchErr error
				if len(pool) > 0 {
					passed, batchStats, batchErr = batchPrefilterTelaVersions(scanCtx, candidates, batchSize, 3, pool, func(completed, total int) {
						results.Color = colors.Yellow
						var progress float64
						if total > 0 {
							progress = 0.15 + 0.45*float64(completed)/float64(total)
						}
						updateTelaProgress(progress)
						setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d / %d)", completed, total), colors.Yellow)
						uiDo(func() {
							results.Refresh()
						})
					})
					logger.Printf("[TELA] Prefilter returned, cleaning up %d pool connections...\n", len(pool))
					poolCleanup()
					logger.Printf("[TELA] Pool cleanup done\n")
				} else {
					batchErr = fmt.Errorf("no RPC connections available")
				}
				phasePrefilterMs = time.Since(prefilterStart).Milliseconds()
				logger.Printf("[TELA] Prefilter phase took %dms, passed=%d err=%v\n", phasePrefilterMs, len(passed), batchErr)
				if batchErr != nil {
					logger.Printf("[TELA] Batch prefilter error: %v\n", batchErr)
					if len(telaSearch) > 0 {
						keepProgressVisible = false
						logger.Printf("[TELA] Prefilter failed but %d cached results available, showing them\n", len(telaSearch))
						fyne.Do(func() {
							searchMu.Lock()
							searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
							searchMu.Unlock()
							searchData.Set(searching)
							searchList.Refresh()
							searchMu.RLock()
							results.Text = fmt.Sprintf("  TELA Apps:  %d (prefilter error)", len(telaSearch))
							searchMu.RUnlock()
							results.Color = colors.Yellow
							results.Refresh()
						})
						completeTelaScanProgress()
						return
					}
					keepProgressVisible = true
					setTelaStatus("Network error during prefilter, retrying...", colors.Yellow)
					showInfiniteTelaProgress()
					scheduleTelaWarmup()
					return
				}

				for sc := range passed {
					prefilterAllowed[sc] = true
					atomic.AddInt64(&prefilterPassed, 1)
				}
				atomic.AddInt64(&prefilterDropped, int64(len(candidates)-len(passed)))
				atomic.AddInt64(&versionHits, batchStats.VersionHits)
				logger.Printf("[TELA] Prefilter: passed=%d dropped=%d version_hits=%d\n", len(passed), len(candidates)-len(passed), batchStats.VersionHits)

				// Persist validated TELA SCIDs to plain JSON cache for fast-path fallback on next startup.
				if len(passed) > 0 {
					validatedSCIDs := make([]string, 0, len(passed))
					for scid := range passed {
						validatedSCIDs = append(validatedSCIDs, scid)
					}
					cache := struct {
						SCIDs     []string `json:"scids"`
						Timestamp int64    `json:"timestamp"`
						Daemon    string   `json:"daemon"`
					}{
						SCIDs:     validatedSCIDs,
						Timestamp: time.Now().Unix(),
						Daemon:    session.Daemon,
					}
					if raw, err := json.MarshalIndent(cache, "", "  "); err == nil {
						cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
						if writeErr := os.WriteFile(cachePath, raw, 0600); writeErr != nil {
							logger.Printf("[TELA] Failed to write JSON cache: %v\n", writeErr)
						} else {
							logger.Printf("[TELA] Persisted %d validated SCIDs to JSON cache\n", len(validatedSCIDs))
						}
					}

					// Also try legacy encrypted cache (may help same-session reuse)
					if raw, err := json.Marshal(validatedSCIDs); err == nil {
						if encErr := StoreEncryptedValue("TELA Search", []byte("ValidatedSCIDs"), raw); encErr != nil {
							logger.Printf("[TELA] Failed to write encrypted SCID cache: %v\n", encErr)
						}
						if tsRaw, err := json.Marshal(time.Now().Unix()); err == nil {
							if encErr := StoreEncryptedValue("TELA Search", []byte("ValidatedSCIDsTimestamp"), tsRaw); encErr != nil {
								logger.Printf("[TELA] Failed to write encrypted timestamp cache: %v\n", encErr)
							}
						}
					}
				}
			}

			// Always start a background backfill to discover NEW TELA apps published
			// since the embedded list was compiled. This runs regardless of whether
			// we used embedded SCIDs or ran the prefilter — new apps need discovery.
			// Also re-trigger when Gnomon has indexed new blocks since the last backfill.
			currentHeight = 0
			if gnomon.Index != nil {
				currentHeight = gnomon.Index.LastIndexedHeight
			}
			heightGrew := currentHeight > lastBackfillHeight && lastBackfillHeight > 0
			if !restrictiveMode && !telaBackfillActive.Load() && (!telaBackfillFailed.Load() || heightGrew) {
				if heightGrew {
					logger.Printf("[TELA] Gnomon height grew %d -> %d; resetting backfill failure state\n", lastBackfillHeight, currentHeight)
					telaBackfillFailed.Store(false)
				}
				workers := 8
				if a.Driver().Device().IsMobile() {
					workers = 4
				}
				telaBackfillActive.Store(true)
				go func() {
					defer telaBackfillActive.Store(false)
					defer func() {
						if r := recover(); r != nil {
							logger.Printf("[TELA] Backfill panic: %v\n", r)
							telaBackfillFailed.Store(true)
						}
					}()
					err := gnomon.Index.BackfillTelaCandidates(workers)
					if err != nil {
						logger.Printf("[TELA] Backfill failed: %v\n", err)
						telaBackfillFailed.Store(true)
					} else {
						lastBackfillHeight = currentHeight
						logger.Printf("[TELA] Backfill completed at height %d\n", currentHeight)
					}
				}()
			}
		}

		// Batch-fetch INDEX data for prefilter-passed SCIDs not yet in indexCacheStore.
		// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket.
		indexFetchFailed := make(map[string]bool) // Track SCIDs whose INDEX fetch failed due to network errors
		networkErrorDuringFetch := false          // Track if there was a network error during batch fetch
		indexFetchRecoverableFailure := false
		if !restrictiveMode {
			var indexNeeded []string
			for scid, allowed := range prefilterAllowed {
				if allowed {
					if _, ok := indexCacheStore[scid]; !ok {
						indexNeeded = append(indexNeeded, scid)
					}
				}
			}
			if len(indexNeeded) > 0 {
				logger.Printf("[TELA] Batch INDEX fetch starting for %d SCIDs...\n", len(indexNeeded))
				setResultsText("  Fetching INDEX data... (%d SCIDs)", len(indexNeeded))
				results.Color = colors.Yellow
				uiDo(func() {
					results.Refresh()
				})

				fetched, ratingsFetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, indexNeeded, 50)
				logger.Printf("[TELA] Batch INDEX fetch done: fetched=%d err=%v\n", len(fetched), fetchErr)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for scan: %v\n", fetchErr)
					networkErrorDuringFetch = true
					if len(indexNeeded) > 0 && len(fetched) == 0 {
						indexFetchRecoverableFailure = true
					}
					// Mark SCIDs as failed due to network error - these should NOT be marked as negative
					// They will be retried on the next scan
					for _, scid := range indexNeeded {
						if _, ok := fetched[scid]; !ok {
							indexFetchFailed[scid] = true
						}
					}
				}
				for scid, index := range fetched {
					indexCacheStore[scid] = index
					setCandidateCache(scid, telaCandidateValidIndex)
					setNegativeSCID(scid, false)
					if r, ok := ratingsFetched[scid]; ok {
						ratingsCache[scid] = r
					}
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(indexNeeded)))
			}
		}

		if indexFetchRecoverableFailure && len(telaSearch) == 0 {
			keepProgressVisible = true
			interruptReason = "index_fetch_retrying"
			results.Hide()
			setTelaStatus("Retrying TELA fetch...", colors.Yellow)
			showInfiniteTelaProgress()
			phaseFinalizeMs = 0
			logger.Printf("[TELA] Search metrics: outcome=interrupted reason=index_fetch_retrying elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
			retryGeneration := currentWalletGeneration()
			go func() {
				time.Sleep(3 * time.Second)
				if !strings.Contains(session.Domain, ".tela") || !isWalletGenerationActive(retryGeneration) || globals.Exit_In_Progress {
					return
				}
				maybeStartTelaWork(true)
			}()
			return
		}

		allLen := len(allSCIDs)
		allCandidates = allLen
		resumeTarget := resumePosition
		scanned := int64(resumePosition) // Progress counter, starts from resume position
		scannedCandidates = scanned
		workers := make(chan struct{}, workerPoolSize)
		interrupted := false
		var scanMu sync.Mutex
		lastUIRefresh := time.Now().Add(-uiRefreshInterval)
		lastProgressSave := time.Now()
		seenSCIDs := make(map[string]bool, len(telaSCIDs))
		for _, scid := range telaSCIDs {
			seenSCIDs[scid] = true
		}

		scanPhaseStart := time.Now()

		for i := resumeTarget; i < allLen; i++ {
			sc := allSCIDs[i]

			// Check for interrupted conditions
			if gnomon.Index == nil || !strings.Contains(session.Domain, ".tela") {
				if gnomon.Index == nil {
					interruptReason = "gnomon_nil_during_scan"
				} else {
					interruptReason = "navigated_away"
				}
				interrupted = true
				break
			}

			// Check connection during scan
			if !walletapi.Connected {
				interruptReason = "connection_lost_during_scan"
				results.Text = "  Connection lost during scan"
				results.Color = colors.Red
				uiDo(func() {
					results.Refresh()
				})
				interrupted = true
				break
			}

			scanMu.Lock()
			alreadySeen := seenSCIDs[sc]
			scanMu.Unlock()
			if !restrictiveMode && !rescanRecheck && isNegativeSCID(sc) {
				atomic.AddInt64(&negCacheSkips, 1)
			}
			if !restrictiveMode && !rescanRecheck && (isNegativeSCID(sc) || alreadySeen || !prefilterAllowed[sc]) {
				atomic.AddInt64(&preDispatchSkips, 1)
				scanned = atomic.AddInt64(&scannedCandidates, 1)
				continue
			}

			scanned = atomic.AddInt64(&scannedCandidates, 1)
			now := time.Now()
			if now.Sub(lastUIRefresh) >= uiRefreshInterval || scanned >= int64(allLen) {
				lastUIRefresh = now
				setResultsText("  Scanning... (%d / %d)", scanned, allLen)
				results.Color = colors.Yellow
				// Phase-based progress: scan is 60% -> 90%
				if allLen > 0 {
					updateTelaProgress(0.60 + 0.30*float64(scanned)/float64(allLen))
				}
				uiDo(func() {
					results.Refresh()
				})
				atomic.AddInt64(&uiRefreshCount, 1)
			}

			if now.Sub(lastProgressSave) >= progressCheckpointInterval {
				saveProgress(int(scanned), allLen, sc, "scanning")
				lastProgressSave = now

				scanMu.Lock()
				scidsSnapshot := make([]string, len(telaSCIDs))
				copy(scidsSnapshot, telaSCIDs)
				scanMu.Unlock()

				if storeSCIDs, err := json.Marshal(scidsSnapshot); err == nil {
					if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
						logger.Printf("[TELA] Failed storing checkpoint SCIDs: %v\n", err)
					} else {
						logger.Printf("[TELA] Checkpoint saved %d SCIDs\n", len(scidsSnapshot))
					}
				}

				if err := saveTelaIndexCache(indexCacheStore); err != nil {
					logger.Printf("[TELA] Failed storing checkpoint INDEX cache: %v\n", err)
				}
			}

			workers <- struct{}{}
			wg.Add(1)
			go func(scid string) {
				defer func() {
					<-workers
					wg.Done()
				}()

				// Check if Gnomon was stopped
				if gnomon.Index == nil {
					return
				}

				if restrictiveMode || prefilterAllowed[scid] {
					// Skip SCIDs whose INDEX fetch failed due to network errors - don't mark as negative
					// They will be retried on the next scan
					if indexFetchFailed[scid] {
						return
					}

					var index tela.INDEX
					if cached, ok := indexCacheStore[scid]; ok {
						index = cached
					} else {
						// INDEX not in cache - this SCID passed prefilter but wasn't fetched
						// This could happen if batch fetch was skipped or failed silently
						// Don't mark as negative, just skip for now
						return
					}

					if isDisplayableTelaApp(index) {

						if allowTelaIndexMutations && gnomon.GetAllSCIDVariableDetails(scid) == nil {
							indexMu.Lock()
							scidsToIndex = append(scidsToIndex, scid)
							indexMu.Unlock()
						}

						// In restrictive mode, the list is initialzed from telaSCIDs
						scanMu.Lock()
						if !restrictiveMode {
							if !seenSCIDs[scid] {
								seenSCIDs[scid] = true
								telaSCIDs = append(telaSCIDs, scid)
							}
						}
						scanMu.Unlock()

						_, ratings, err := getLikesRatioCached(scid, index.DURL, searchExclusions, minLikes, ratingsCache)
						if err != nil {
							if strings.Contains(err.Error(), "found search exclusion") {
								atomic.AddInt64(&filteredByExclusion, 1)
							} else if strings.Contains(err.Error(), "below min rating setting") {
								atomic.AddInt64(&filteredByMinLikes, 1)
							}
							setCandidateCache(scid, telaCandidateExcludedByURL)
							return
						}

						setCandidateCache(scid, telaCandidateValidIndex)
						setNegativeSCID(scid, false)

						searchMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						searchMu.Unlock()
					} else {
						atomic.AddInt64(&filteredNonDisplayable, 1)
						setCandidateCache(scid, telaCandidateNoDocs)
						setNegativeSCID(scid, true)
					}
				}
			}(sc)
		}

		if !strings.Contains(session.Domain, ".tela") {
			interrupted = true
		}

		wg.Wait()
		phaseScanMs = time.Since(scanPhaseStart).Milliseconds()

		if len(scidsToIndex) > 0 {
			batch := make(map[string]*structures.FastSyncImport, len(scidsToIndex))
			for _, scid := range scidsToIndex {
				batch[scid] = &structures.FastSyncImport{}
			}
			if err := gnomon.Index.AddSCIDToIndex(batch, false, true); err != nil {
				logger.Printf("[TELA] Batch index error: %v\n", err)
			} else {
				logger.Printf("[TELA] Batch indexed %d SCIDs\n", len(scidsToIndex))
			}
		}

		if interrupted {
			scanMu.Lock()
			scidsSnapshot := make([]string, len(telaSCIDs))
			copy(scidsSnapshot, telaSCIDs)
			scanMu.Unlock()

			if storeSCIDs, err := json.Marshal(scidsSnapshot); err == nil {
				if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
					logger.Printf("[TELA] Failed storing interrupted SCIDs: %v\n", err)
				} else {
					logger.Printf("[TELA] Saved %d SCIDs before interruption\n", len(scidsSnapshot))
				}
			}

			if err := saveTelaIndexCache(indexCacheStore); err != nil {
				logger.Printf("[TELA] Failed storing interrupted INDEX cache: %v\n", err)
			}

			candidateCacheMu.RLock()
			candidateCacheSnapshot := make(telaCandidateCache, len(candidateCache))
			for scid, meta := range candidateCache {
				candidateCacheSnapshot[scid] = meta
			}
			candidateCacheMu.RUnlock()
			if err := saveTelaCandidateCache(candidateCacheSnapshot); err != nil {
				logger.Printf("[TELA] Failed storing interrupted candidate cache: %v\n", err)
			}

			if !restrictiveMode {
				if err := saveStringSetToEncryptedStorage("TELA Search", "NegativeCache", sAll); err != nil {
					logger.Printf("[TELA] Failed storing interrupted negative cache: %v\n", err)
				}
			}

			saveProgress(int(atomic.LoadInt64(&scannedCandidates)), allLen, "", "interrupted")
			results.Text = "  Scan interrupted"
			results.Color = colors.Yellow
			fyne.Do(func() {
				results.Refresh()
				entrySearch.Enable()
				entryAddSCID.Enable()
			})
			logger.Printf("[TELA] Search metrics: outcome=interrupted reason=%s elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", interruptReason, time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
			return
		}

		finalizeStart := time.Now()

		fyne.Do(func() {
			searchMu.Lock()
			searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
			searchMu.Unlock()
			searchData.Set(searching)
			searchList.Refresh()
			results.Show()
			if networkErrorDuringFetch {
				searchMu.RLock()
				results.Text = fmt.Sprintf("  TELA Apps:  %d (some apps may be missing - network error during fetch)", len(telaSearch))
				searchMu.RUnlock()
				results.Color = colors.Yellow
			} else {
				searchMu.RLock()
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(telaSearch))
				searchMu.RUnlock()
				results.Color = colors.Green
			}
			results.Refresh()
		})
		warmRatings()

		timeNow := time.Now().Format(time.RFC822)
		StoreEncryptedValue("TELA Search", []byte("Last Scan"), []byte(timeNow))
		if gnomon.Index != nil {
			if err := StoreEncryptedValue("TELA Search", []byte("Last Indexed Height"), []byte(strconv.FormatInt(gnomon.Index.LastIndexedHeight, 10))); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing Last Indexed Height: %v\n", err)
			}
		}
		if allLen > 0 && atomic.LoadInt64(&scannedCandidates) >= int64(allLen) {
			saveProgress(allLen, allLen, "", "completed")
			completeTelaScanProgress()
		} else {
			saveProgress(int(atomic.LoadInt64(&scannedCandidates)), allLen, "", "interrupted")
			logger.Printf("[Gnomon] Scan ended before completion: %d/%d\n", atomic.LoadInt64(&scannedCandidates), allLen)
		}

		if storeSCIDs, err := json.Marshal(telaSCIDs); err == nil {
			if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing SCIDs cache: count=%d bytes=%d err=%v\n", len(telaSCIDs), len(storeSCIDs), err)
			}
		} else {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed marshaling SCIDs cache: count=%d err=%v\n", len(telaSCIDs), err)
		}

		if !restrictiveMode {
			if err := saveStringSetToEncryptedStorage("TELA Search", "NegativeCache", sAll); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing negative cache: entries=%d err=%v\n", len(sAll), err)
			}
		}

		if err := saveTelaIndexCache(indexCacheStore); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing INDEX cache: entries=%d err=%v\n", len(indexCacheStore), err)
		}

		searchMu.Lock()
		telaSearch = deduplicateTelaSearch(telaSearch)
		if err := saveTelaDisplayCache(telaDisplayCache(telaSearch)); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing display cache: entries=%d err=%v\n", len(telaSearch), err)
		}
		searchMu.Unlock()

		candidateCacheMu.RLock()
		candidateCacheSnapshot := make(telaCandidateCache, len(candidateCache))
		for scid, meta := range candidateCache {
			candidateCacheSnapshot[scid] = meta
		}
		candidateCacheMu.RUnlock()
		if err := saveTelaCandidateCache(candidateCacheSnapshot); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing candidate cache: entries=%d err=%v\n", len(candidateCacheSnapshot), err)
		}

		if restrictiveMode && len(searching) < 1 {
			errorText.Text = "TELA is in restrictive mode"
			errorText.Color = colors.Yellow
			errorText.Refresh()
		}

		lastScan = timeNow
		labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
		labelLastScan.Color = colors.Green

		fyne.Do(func() {
			labelLastScan.Refresh()
			entrySearch.Enable()
			entryAddSCID.Enable()
		})
		phaseFinalizeMs = time.Since(finalizeStart).Milliseconds()

		searchMu.RLock()
		displayedSCIDs := make([]string, 0, len(telaSearch))
		seenDisplayed := make(map[string]struct{}, len(telaSearch))
		for _, entry := range telaSearch {
			if entry.SCID == "" {
				continue
			}
			if _, exists := seenDisplayed[entry.SCID]; exists {
				continue
			}
			seenDisplayed[entry.SCID] = struct{}{}
			displayedSCIDs = append(displayedSCIDs, entry.SCID)
		}
		searchMu.RUnlock()
		if !restrictiveMode && len(displayedSCIDs) > 0 {
			telaSCIDs = displayedSCIDs
		}

		logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
		logger.Printf("[TELA] Discovery state: backfillActive=%v backfillFailed=%v lastBackfillHeight=%d gnomonHeight=%d displayed=%d\n",
			telaBackfillActive.Load(), telaBackfillFailed.Load(), lastBackfillHeight, currentHeight, len(displayedSCIDs))
		keepProgressVisible = false

		// Start background pre-warm for TELA apps on mobile to reduce launch latency.
		// Pre-warm up to 3 most recently used apps from history.
		if a.Driver().Device().IsMobile() && len(telaSearch) > 0 {
			go func() {
				time.Sleep(3 * time.Second) // Wait for UI to settle
				if !strings.Contains(session.Domain, ".tela") || globals.Exit_In_Progress {
					return
				}

				// Get recently used SCIDs from history
				var recentSCIDs []string
				historyRaw, err := GetEncryptedValue("TELA History", []byte("RecentSCIDs"))
				if err == nil && len(historyRaw) > 0 {
					json.Unmarshal(historyRaw, &recentSCIDs)
				}

				// Pre-warm up to 3 apps that are in our search results
				preWarmCount := 0
				for _, scid := range recentSCIDs {
					if preWarmCount >= 3 {
						break
					}
					// Check if this SCID is in our current results
					found := false
					for _, entry := range telaSearch {
						if entry.SCID == scid {
							found = true
							break
						}
					}
					if !found {
						continue
					}

					// Check if already served
					alreadyServed := false
					for _, s := range getTelaActiveServers() {
						if s.SCID == scid {
							alreadyServed = true
							break
						}
					}
					if alreadyServed {
						continue
					}

					logger.Printf("[TELA-PREWARM] Pre-warming SCID %s (%d/3)\n", scid, preWarmCount+1)
					if _, err := serveTELACollisionRecovery(scid, session.Daemon); err != nil {
						logger.Printf("[TELA-PREWARM] Failed to pre-warm %s: %v\n", scid, err)
					} else {
						preWarmCount++
					}
				}
				if preWarmCount > 0 {
					logger.Printf("[TELA-PREWARM] Pre-warmed %d apps\n", preWarmCount)
				}
			}()
		}
	}

	scheduleTelaWarmup = func() {
		if !telaWarmupScheduled.CompareAndSwap(false, true) {
			return
		}
		generation := currentWalletGeneration()
		go func() {
			defer telaWarmupScheduled.Store(false)
			time.Sleep(2 * time.Second)
			if globals.Exit_In_Progress || !isWalletGenerationActive(generation) {
				return
			}
			if !strings.Contains(session.Domain, ".tela") {
				return
			}
			if telaNetworkPaused.Load() || telaWorkActive.Load() || telaLaunchPending.Load() {
				return
			}
			maybeStartTelaWork(true)
		}()
	}

	maybeStartTelaWork = func(force bool) {
		if !strings.Contains(session.Domain, ".tela") || globals.Exit_In_Progress {
			return
		}
		// Do not launch while waiting for network to restore.
		if telaNetworkPaused.Load() {
			return
		}
		if telaWorkActive.Load() {
			return
		}
		if telaLaunchPending.Load() {
			return
		}
		if !telaLaunchPending.CompareAndSwap(false, true) {
			return
		}
		go getSearchResults()
	}

	startTelaInitialLoad = func() {
		if len(searching) > 0 || len(telaSearch) > 0 {
			return
		}
		resetTelaProgress()
		showActiveTelaProgress("Connecting to Gnomon...", 0.02, true)
		if refreshAppsList != nil {
			refreshAppsList()
		}
		if gnomon.Index == nil {
			if engram.Disk != nil {
				generation := currentWalletGeneration()
				enableGnomon, _ := getGnomon()
				if enableGnomon == "1" && isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
					go startGnomon()
				}
			}
			maybeStartTelaWork(true)
			return
		}
		maybeStartTelaWork(true)
	}

	entrySearch.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		normalizedInput := normalizeTelaSearch(s)

		if s == "" {
			if wSelect.Selected == "Favorites" {
				refreshFavoritesList()
				favoritesList.Refresh()
				if engram.Disk == nil {
					results.Text = "  No wallet connected."
					results.Color = colors.Gray
				} else if len(favorites) == 0 {
					results.Text = "  No favorites yet."
					results.Color = colors.Gray
				} else {
					results.Text = fmt.Sprintf("  Favorites:  %d", len(favorites))
					results.Color = colors.Green
				}
				results.Refresh()
			} else {
				if len(telaSearch) > 0 {
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					_ = searchData.Set(searching)
					if refreshAppsList != nil {
						refreshAppsList()
					}
				} else {
					results.Text = "  No TELA apps loaded."
					results.Color = colors.Gray
					results.Refresh()
				}
			}
			if !a.Driver().Device().IsMobile() {
				entrySearch.HideCompletion()
			}

			return
		}

		if !a.Driver().Device().IsMobile() {
			if len(s) < 3 {
				entrySearch.SetOptions(append([]string{s}, entrySearchCompletions...))
				entrySearch.ShowCompletion()
			} else {
				entrySearch.HideCompletion()
			}
		}

		if wSelect.Selected == "Favorites" {
			var queryResult []string
			for _, data := range favorites {
				for _, split := range strings.Split(data, ";;;") {
					if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
						queryResult = append(queryResult, data)
						break
					}
				}
			}

			sort.Strings(queryResult)
			favoritesData.Set(queryResult)
			favoritesList.Refresh()
			results.Text = fmt.Sprintf("  Favorites:  %d", len(queryResult))
			results.Color = colors.Green
			results.Refresh()
			entrySearch.Enable()

			return
		}

		var queryResult []INDEXwithRatings
		query := strings.Split(s, ":")
		if len(query) < 2 {
			if len(s) == 64 {
				// Search scid
				for _, ind := range telaSearch {
					_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
					if err != nil {
						continue
					}

					if ind.SCID == s {
						queryResult = append(queryResult, ind)
						break
					}
				}
			} else {
				// Search all
				for _, ind := range telaSearch {
					_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
					if err != nil {
						continue
					}

					data := []string{
						ind.NameHdr,
						ind.DescrHdr,
						ind.DURL,
						ind.SCID,
					}

					for _, split := range data {
						if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
							queryResult = append(queryResult, ind)
							break
						}
					}
				}
			}

			searching = telaSearchDisplayAll(queryResult, sortBy, sortDescending)
			searchData.Set(searching)
			searchList.Refresh()

			results.Text = fmt.Sprintf("  TELA Apps:  %d", len(queryResult))
			results.Color = colors.Green
			results.Refresh()
			entrySearch.Enable()

			return
		}

		switch normalizeTelaSearch(query[0]) {
		case "name":
			searchMu.RLock()
			snapshot := make([]INDEXwithRatings, len(telaSearch))
			copy(snapshot, telaSearch)
			searchMu.RUnlock()
			for _, ind := range snapshot {
				_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
				if err != nil {
					continue
				}

				if strings.Contains(normalizeTelaSearch(ind.NameHdr), normalizeTelaSearch(query[1])) {
					queryResult = append(queryResult, ind)
				}
			}
		case "durl":
			searchMu.RLock()
			snapshot := make([]INDEXwithRatings, len(telaSearch))
			copy(snapshot, telaSearch)
			searchMu.RUnlock()
			for _, ind := range snapshot {
				_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
				if err != nil {
					continue
				}

				if strings.Contains(normalizeTelaSearch(ind.DURL), normalizeTelaSearch(query[1])) {
					queryResult = append(queryResult, ind)
				}
			}
		case "my":
			if engram.Disk != nil {
				walletAddr := engram.Disk.GetAddress().String()
				for _, ind := range telaSearch {
					if ind.Author == walletAddr {
						queryResult = append(queryResult, ind)
					}
				}
			}
		case "author":
			if len(query[1]) != 66 {
				return
			}

			_, err := globals.ParseValidateAddress(query[1])
			if err != nil {
				return
			}

			for _, ind := range telaSearch {
				_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
				if err != nil {
					continue
				}

				if ind.Author == query[1] {
					queryResult = append(queryResult, ind)
				}
			}
		default:
			errorText.Text = "unknown search prefix"
			errorText.Color = colors.Red
			errorText.Refresh()

			return
		}

		searching = telaSearchDisplayAll(queryResult, sortBy, sortDescending)
		searchData.Set(searching)
		searchList.Refresh()

		results.Text = fmt.Sprintf("  TELA Apps:  %d", len(queryResult))
		results.Color = colors.Green
		results.Refresh()
		entrySearch.Enable()
	}

	entryAddSCID.OnChanged = func(s string) {
		if len(s) == 64 {
			defer entryAddSCID.SetText("")
			bootstrapIndex, err := tela.GetINDEXInfo(s, session.Daemon)
			if err != nil {
				logger.Errorf("[GetINDEXInfo] Bootstrap: %s\n", err)
				errorText.Text = "could not get bootstrap SCID"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			if !strings.HasSuffix(bootstrapIndex.DURL, tela.TAG_BOOTSTRAP) {
				logger.Errorf("[Engram] SCID %s is not a TELA bootstrap INDEX\n", s)
				errorText.Text = "invalid bootstrap SCID"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			storeSCIDs, err := json.Marshal(bootstrapIndex.DOCs)
			if err != nil {
				logger.Errorf("[Engram] Could not marshal bootstrap: %s\n", err)
				errorText.Text = "error initializing bootstrap"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}

			err = StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs)
			if err != nil {
				logger.Errorf("[Engram] Could store bootstrap: %s\n", err)
				errorText.Text = "error storing bootstrap"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
			_ = DeleteKey("TELA Search", []byte("NegativeCache"))
			_ = DeleteKey("TELA Search", []byte("IndexCache"))
			_ = DeleteKey("TELA Search", []byte("CandidateCache"))
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))

			telaSCIDs = bootstrapIndex.DOCs
			errorText.Text = "bootstrap initialized"
			errorText.Color = colors.Green
			errorText.Refresh()

			maybeStartTelaWork(true)
		}
	}

	// Refresh the active server list
	refreshServerList = func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[Engram] refreshServerList panic recovered: %v\n", r)
			}
		}()
		time.Sleep(time.Second * 2)
		var serversRunning []string
		for _, serv := range getTelaActiveServers() {
			serversRunning = append(serversRunning, serv.Name+";;;"+serv.Address+";;;;;;"+serv.SCID)
		}

		sort.Strings(serversRunning)
		fyne.Do(func() {
			servingData.Set(serversRunning)
			servingList.Refresh()
			if refreshAppsList != nil {
				refreshAppsList()
			}
			if !isSearching && wSelect.Selected == "Search" && len(serversRunning) > 0 {
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(searching))
				results.Color = colors.Green
				results.Refresh()
			}
		})
	}

	refreshFavoritesList = func() {
		if engram.Disk != nil {
			walletAddress := engram.Disk.GetAddress().String()
			favs, err := GetTELAFavorites(walletAddress)
			if err != nil || len(favs) == 0 {
				favorites = []string{}
				fyne.Do(func() {
					favoritesData.Set(favorites)
				})
			} else {
				favorites = []string{}
				for scid, favData := range favs {
					favorites = append(favorites, favData.Name+";;;"+scid)
				}
				sort.Strings(favorites)
				fyne.Do(func() {
					favoritesData.Set(favorites)
				})
			}
		}
	}

	refreshAppsList = func() {
		searchMu.RLock()
		if len(telaSearch) == 0 {
			searchMu.RUnlock()
			return
		}

		updated := telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
		searchMu.RUnlock()
		fyne.Do(func() {
			searching = updated
			searchData.Set(searching)
			searchList.Refresh()
			if !isSearching && wSelect.Selected == "Search" {
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(searching))
				results.Color = colors.Green
				results.Refresh()
			}
		})
	}

	refreshTELA := func() {
		go refreshServerList()
		refreshFavoritesList()
		refreshAppsList()
	}

	btnShutdown.OnTapped = func() {
		switch btnShutdown.Text {
		case "Rescan Blockchain":
			verificationOverlay(
				false,
				"TELA BROWSER",
				"Rescan blockchain? This will clear cached results and rescan all TELA apps.",
				"Confirm",
				func(b bool) {
					if b {
						if isSearching {
							return
						}

						clearAllTELACache()
						telaSearch = []INDEXwithRatings{}
						telaSCIDs = []string{}
						sAll = map[string]bool{}
						forceFreshScan = true
						// Reset backfill failure so a rescan can trigger fresh discovery
						telaBackfillFailed.Store(false)
						lastBackfillHeight = 0
						logger.Printf("[TELA] Rescan triggered: reset backfill state\n")
						errorText.Text = ""
						errorText.Refresh()
						go getSearchResults()
					}
				},
			)
		default:
			verificationOverlay(
				false,
				"TELA BROWSER",
				"Shutdown all active TELA servers?",
				"Confirm",
				func(b bool) {
					if b {
						tela.ShutdownTELA()
						servingData.Set(nil)
						errorText.Text = ""
						errorText.Refresh()
					}
				},
			)
		}

		go refreshServerList()
	}

	historyBox := container.NewStack(
		rectList,
		historyList,
	)

	searchBox := container.NewStack(
		rectList,
		searchList,
	)

	servingBox := container.NewStack(
		rectList,
		servingList,
	)

	favoritesBox := container.NewStack(
		rectList,
		favoritesList,
	)

	layoutBrowser := container.NewBorder(
		container.NewVBox(
			entryHistory,
			entrySearch,
			entryServeSCID,
			tabButtons,
			statusBox,
		),
		nil,
		nil,
		nil,
		container.NewStack(
			favoritesBox,
			historyBox,
			searchBox,
			servingBox,
		),
	)

	// Hide all alternative views initially
	entrySearch.Hide()
	entryServeSCID.Hide()
	favoritesBox.Hide()
	historyBox.Hide()
	searchBox.Hide()
	servingBox.Hide()

	results.Show()
	results.Refresh()

	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		if favs, _ := GetTELAFavorites(walletAddress); favs != nil && len(favs) > 0 {
			go preIndexFavorites(favs)
		}
	}

	go func() {
		fyne.Do(func() {
			wSelect.SetSelected("Search")
			startTelaInitialLoad()
		})
	}()

	var historyResults []string
	var historyMu sync.Mutex
	var historyLoading bool

	getHistoryResults := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[Engram] getHistoryResults panic recovered: %v\n", r)
			}
		}()
		historyMu.Lock()
		if historyLoading {
			historyMu.Unlock()
			return
		}
		historyLoading = true
		historyMu.Unlock()

		historyResults = nil
		historyData.Set(nil)
		defer func() {
			historyMu.Lock()
			historyLoading = false
			historyMu.Unlock()
		}()

		disk := engram.Disk
		idx := gnomon.Index
		if disk != nil && idx != nil {
			for {
				if !strings.Contains(session.Domain, ".tela") {
					return
				}

				disk = engram.Disk
				idx = gnomon.Index
				if disk == nil || idx == nil {
					return
				}

				if idx.LastIndexedHeight >= int64(disk.Get_Daemon_Height()) {
					break
				}

				fyne.Do(func() {
					entryHistory.Disable()
					results.Refresh()
				})

				time.Sleep(time.Second)
			}

			results.Text = "  Loading launched apps..."
			results.Color = colors.Yellow

			fyne.Do(func() {
				entryHistory.Enable()
				results.Refresh()
			})

			shard, err := GetShard()
			if err != nil {
				return
			}

			store, err := graviton.NewDiskStore(shard)
			if err != nil {
				return
			}

			ss, err := store.LoadSnapshot(0)

			if err != nil {
				return
			}

			tree, err := ss.GetTree("TELA History")
			if err != nil {
				return
			}

			c := tree.Cursor()

			for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
				scid := crypto.HashHexToHash(string(k))

				title, desc, _, _, _ := getContractHeader(scid)

				if title == "" {
					title = scid.String()
				}

				if desc == "" {
					desc = "N/A"
				}

				historyResults = append(historyResults, title+";;;"+scid.String()+";;;"+desc)
			}

			sort.Strings(historyResults)
			history = historyResults
			historyData.Set(history)

			results.Text = fmt.Sprintf("  Launched Apps:  %d", len(history))
			results.Color = colors.Green

			fyne.Do(func() {
				historyList.Refresh()
				results.Refresh()
				btnShutdown.Enable()
			})
		}
	}

	entryHistory.OnChanged = func(s string) {
		if s == "" {
			go getHistoryResults()
			return
		}

		normalizedInput := normalizeTelaSearch(s)

		var queryResult []string
		for _, data := range history {
			for _, split := range strings.Split(data, ";;;") {
				if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
					queryResult = append(queryResult, data)
					break
				}
			}
		}

		sort.Strings(queryResult)
		history = queryResult
		historyData.Set(history)

		results.Text = fmt.Sprintf("  Search History:  %d", len(queryResult))
		results.Color = colors.Green
		entryHistory.Enable()

		fyne.Do(func() {
			historyList.Refresh()
			results.Refresh()
		})
	}

	activateTelaSearch = func() {
		errorText.Text = ""
		errorText.Refresh()

		entryHistory.Hide()
		entrySearch.Show()
		entryServeSCID.Hide()
		favoritesBox.Hide()
		historyBox.Hide()
		searchBox.Show()
		servingBox.Hide()
		results.Show()

		telaViewActive.Store(false)
		setTelaStatus("", color.Transparent)
		hideTelaProgress()

		if len(searching) > 0 || len(telaSearch) > 0 {
			if len(searching) == 0 && len(telaSearch) > 0 {
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				_ = searchData.Set(searching)
			}
			if refreshAppsList != nil {
				refreshAppsList()
			} else {
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(searching))
				results.Color = colors.Green
				results.Refresh()
			}
			maybeStartTelaWork(true)
			return
		}

		// On re-visit telaSearch is empty because it's a local variable.
		// Load cached display results immediately so apps don't disappear.
		cachedDisplay := loadTelaDisplayCache()
		if len(cachedDisplay) > 0 {
			for _, entry := range cachedDisplay {
				if !isDisplayableTelaApp(entry.INDEX) {
					continue
				}
				telaSearch = append(telaSearch, entry)
			}
			telaSearch = deduplicateTelaSearch(telaSearch)
			if len(telaSearch) > 0 {
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				_ = searchData.Set(searching)
				searchList.Refresh()
				results.Text = fmt.Sprintf("  TELA Apps:  %d", len(telaSearch))
				results.Color = colors.Green
				results.Refresh()
				maybeStartTelaWork(true)
				return
			}
		}

		entrySearch.SetPlaceHolder("Search Apps")
		results.Text = "  No scanned TELA apps yet."
		if forceFreshScan {
			results.Text = "  Resetting TELA results..."
		}
		results.Color = colors.Gray
		results.Refresh()

		searchList.Refresh()
		maybeStartTelaWork(true)
	}

	wSelect.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()

		// Hide all first
		entryHistory.Hide()
		entrySearch.Hide()
		entryServeSCID.Hide()
		favoritesBox.Hide()
		historyBox.Hide()
		searchBox.Hide()
		servingBox.Hide()

		switch s {
		case "Favorites":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			entrySearch.Show()
			entrySearch.SetPlaceHolder("Search favorites")
			refreshFavoritesList()
			if engram.Disk == nil {
				results.Text = "  No wallet connected."
				results.Color = colors.Gray
			} else if len(favorites) == 0 {
				results.Text = "  No favorites yet."
				results.Color = colors.Gray
			} else {
				results.Text = fmt.Sprintf("  Favorites:  %d", len(favorites))
				results.Color = colors.Green
			}
			results.Refresh()
			favoritesBox.Show()
			favoritesList.Refresh()
		case "History":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			if gnomon.Index == nil {
				results.Text = "  Index is unavailable."
				results.Color = colors.Gray
				results.Show()
				results.Refresh()
				refreshTelaStatusBox()
			}

			generation := currentWalletGeneration()
			if isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
				go getHistoryResults()
			}

			entryHistory.Show()
			historyBox.Show()
			historyList.Refresh()
			servingList.UnselectAll()
		case "Search":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			activateTelaSearch()
		}
	}

	if session.Offline {
		results.Text = "  Disabled in offline mode."
		results.Color = colors.Gray
		results.Refresh()
		entryServeSCID.Disable()
		entryAddSCID.Disable()
		btnShutdown.Disable()
	} else if gnomon.Index == nil {
		results.Text = "  Index is unavailable."
		results.Color = colors.Gray
		results.Refresh()
		entryAddSCID.Disable()
	}

	// Note: activateTelaSearch() is called via wSelect.SetSelected("Search") above
	// We don't call it again here to avoid double execution which causes race conditions on Android

	entryServeSCID.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		if len(s) == 64 {
			go func() {
				// Create a TELALink to parse and get its ratings for user to verifiy before serving the content
				telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", s)}
				linkPermission, err := AskPermissionForRequestE("Open TELA Link", telaLink)
				if err != nil {
					logger.Errorf("[Engram] Open TELA link: %s\n", err)
					errorText.Text = "error could not open TELA"
					errorText.Color = colors.Red

					fyne.Do(func() {
						errorText.Refresh()
					})

					return
				}

				if linkPermission != xswd.Allow {
					entryServeSCID.SetText("")
					return
				}

				showLoadingOverlay()
				defer func() {
					go refreshServerList()
				}()

				var index tela.INDEX

				// If serving without Gnomon, scid will not end up in history
				if gnomon.Index != nil {
					result := gnomon.GetAllSCIDVariableDetails(s)
					if len(result) == 0 {
						_, err := getTxData(s)
						if err != nil {
							return
						}
					}

					index.NameHdr, index.DescrHdr, _, _, _ = getContractHeader(crypto.HashHexToHash(s))

					if index.NameHdr == "" {
						index.NameHdr = s
					}

					if len(index.NameHdr) > 36 {
						index.NameHdr = index.NameHdr[0:36] + "..."
					}

					if index.DescrHdr == "" {
						index.DescrHdr = "N/A"
					}

					if len(index.DescrHdr) > 40 {
						index.DescrHdr = index.DescrHdr[0:40] + "..."
					}
				}

				entryServeSCID.SetText("")

				if link, err := serveTELAWithStaleRecovery(s, session.Daemon); err == nil {
					url, err := url.Parse(link)
					if err != nil {
						logger.Errorf("[Engram] TELA URL parse: %s\n", err)
						errorText.Text = "error could parse URL"
						errorText.Color = colors.Red

						fyne.Do(func() {
							errorText.Refresh()
						})

						return // If url is not valid, scid won't be saved in history
					} else {
						pushTELANavigation(s)

						if a.Driver().Device().IsMobile() {
							go func() {
								time.Sleep(2 * time.Second)
								err = fyne.CurrentApp().OpenURL(url)
								if err != nil {
									logger.Errorf("[Engram] TELA OpenURL error: %s\n", err)
									errorText.Text = "error could not open browser"
									errorText.Color = colors.Red

									if isMobileDevice() {
										fyne.Do(func() {
											dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
										})
									}
								} else if isMobileDevice() {
									logger.Printf("[Engram] TELA: Opened in mobile browser %s\n", s)
								}
							}()
						} else {
							err = fyne.CurrentApp().OpenURL(url)
							if err != nil {
								logger.Errorf("[Engram] TELA OpenURL error: %s\n", err)
								errorText.Text = "error could not open browser"
								errorText.Color = colors.Red

								if isMobileDevice() {
									fyne.Do(func() {
										dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
									})
								}
							} else if isMobileDevice() {
								logger.Printf("[Engram] TELA: Opened in mobile browser %s\n", s)
							}
						}
					}

					if gnomon.Index != nil {
						historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
						sort.Strings(historyResults)
						history = historyResults
						historyData.Set(history)

						results.Text = ""

						err = StoreEncryptedValue("TELA History", []byte(s), []byte(""))
						if err != nil {
							logger.Errorf("[Engram] Error saving TELA search result: %s\n", err)
						}
					}
				} else {
					if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
						removeOverlays()

						// Create a TELALink to parse and get its ratings for user to verifiy before serving updated content
						telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", s)}
						linkPermission, err := AskPermissionForRequestE("Allow Updated Content", telaLink)
						if err != nil {
							logger.Errorf("[Engram] Open TELA link: %s\n", err)
							errorText.Text = "error could not open TELA"
							errorText.Color = colors.Red

							fyne.Do(func() {
								errorText.Refresh()
							})

							return
						}

						if linkPermission != xswd.Allow {
							entryServeSCID.SetText("")
							return
						}

						link, err := serveTELAUpdates(s)
						if err != nil {
							logger.Errorf("[Engram] Error serving TELA: %s\n", err)
							errorText.Text = telaErrorToString(err)
							errorText.Color = colors.Red

							fyne.Do(func() {
								errorText.Refresh()
							})

							return
						}

						url, err := url.Parse(link)
						if err != nil {
							logger.Errorf("[Engram] TELA URL parse: %s\n", err)
							errorText.Text = "error could parse URL"
							errorText.Color = colors.Red

							fyne.Do(func() {
								errorText.Refresh()
							})

							return
						} else {
							pushTELANavigation(s)

							if a.Driver().Device().IsMobile() {
								go func() {
									time.Sleep(2 * time.Second)
									err = fyne.CurrentApp().OpenURL(url)
									if err != nil {
										errorText.Text = "error could not open browser"
										errorText.Color = colors.Red

										if isMobileDevice() {
											fyne.Do(func() {
												dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
											})
										}
									}
								}()
							} else {
								err = fyne.CurrentApp().OpenURL(url)
								if err != nil {
									errorText.Text = "error could not open browser"
									errorText.Color = colors.Red

									if isMobileDevice() {
										fyne.Do(func() {
											dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
										})
									}
								}
							}
						}

						if gnomon.Index != nil {
							historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
							sort.Strings(historyResults)
							history = historyResults
							historyData.Set(history)
							fyne.Do(func() {
								historyList.Refresh()
							})

							results.Text = ""

							err = StoreEncryptedValue("TELA History", []byte(s), []byte(""))
							if err != nil {
								logger.Errorf("[Engram] Error saving TELA search result: %s\n", err)
							}
						}

						return
					}

					logger.Errorf("[Engram] Error serving TELA: %s\n", err)
					errorText.Text = telaErrorToString(err)
					errorText.Color = colors.Red
				}

				fyne.Do(func() {
					historyList.Refresh()
					errorText.Refresh()
					results.Refresh()
				})

				removeOverlays()
			}()
		}
	}

	historyList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(history[id], ";;;")
		if len(split) < 4 || len(split[3]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		scid := split[3]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		historyList.UnselectAll()
		historyList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	searchList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(searching[id], ";;;")
		if len(split) < 2 || len(split[1]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		scid := split[1]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		searchList.UnselectAll()
		searchList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	servingList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(serving[id], ";;;")
		if len(split) < 4 || len(split[3]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		scid := split[3]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		servingList.UnselectAll()
		servingList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	favoritesList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()

		if id < 0 || id >= len(favorites) {
			return
		}

		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(favorites[id], ";;;")
		if len(split) < 2 || len(split[1]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID from favorites\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		scid := split[1]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo from favorites: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		favoritesList.UnselectAll()
		favoritesList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshServerList))
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(3),
					btnRescanTela,
					btnBack,
					btnSettingsTela,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			layoutBrowser,
		),
	)

	// Create TELA background with semi-transparent overlay using DarkMatter theme
	bgOverlay := canvas.NewRectangle(color.RGBA{21, 23, 30, 255}) // Full opacity DarkMatter
	bgOverlay.SetMinSize(fyne.NewSize(ui.Width, ui.Height))

	layoutWithBg := container.NewStack(
		// res.telaBg, // Background image - temporarily disabled for color testing
		bgOverlay, // Background color only
		layout,
	)

	return NewVScroll(layoutWithBg)
}

// Layout details of a TELA INDEX
func layoutTELAManager(index tela.INDEX, callback func(), autoLaunch ...bool) fyne.CanvasObject {
	shouldLaunch := false
	if len(autoLaunch) > 0 {
		shouldLaunch = autoLaunch[0]
	}

	session.Domain = "app.tela.manager"
	originalCallerDomain := session.LastDomain // Safely capture the original TELA browser content

	var cachedData *TELAFavoriteData
	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		cachedData, _ = GetTELAFavoriteData(walletAddress, index.SCID)
	}

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(scalePoint(320, 1))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelName := widget.NewRichText(&widget.TextSegment{
		Text: index.NameHdr,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		}})
	labelName.Wrapping = fyne.TextWrapWord

	labelDesc := widget.NewRichText(&widget.TextSegment{
		Text: index.DescrHdr,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			TextStyle: fyne.TextStyle{Bold: false},
		}})
	labelDesc.Wrapping = fyne.TextWrapWord

	labelDURL := canvas.NewText("DURL", colors.Gray)
	labelDURL.TextSize = scaleFont(14)
	labelDURL.Alignment = fyne.TextAlignCenter
	labelDURL.TextStyle = fyne.TextStyle{Bold: true}

	textDURL := widget.NewRichTextFromMarkdown(index.DURL)
	textDURL.Wrapping = fyne.TextWrapWord

	labelSCID := canvas.NewText("SMART  CONTRACT  ID", colors.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignCenter
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	textSCID := widget.NewRichTextFromMarkdown(index.SCID)
	textSCID.Wrapping = fyne.TextWrapWord

	btnViewExplorer := widget.NewButtonWithIcon("", resourceBrowserGlobeSvg, func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	})

	btnCopySCID := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(index.SCID)
	})

	labelAuthor := canvas.NewText("SMART  CONTRACT  AUTHOR", colors.Gray)
	labelAuthor.TextSize = scaleFont(14)
	labelAuthor.Alignment = fyne.TextAlignCenter
	labelAuthor.TextStyle = fyne.TextStyle{Bold: true}

	author := index.Author
	if author == "anon" {
		author = "--"
	}
	textAuthor := widget.NewRichTextFromMarkdown(author)
	textAuthor.Wrapping = fyne.TextWrapWord

	btnMessageAuthor := widget.NewButtonWithIcon("", theme.MailComposeIcon(), func() {
		if index.Author != "" {
			messages.Contact = index.Author
			session.PreviousDomain = session.Domain
			session.LastDomain = session.Window.Content()
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	})

	btnCopyAuthor := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(index.Author)
	})

	labelStatus := canvas.NewText("APPLICATION  STATUS", colors.Gray)
	labelStatus.TextSize = scaleFont(14)
	labelStatus.Alignment = fyne.TextAlignCenter
	labelStatus.TextStyle = fyne.TextStyle{Bold: true}

	textStatus := canvas.NewText("Offline", colors.Gray)
	textStatus.TextSize = scaleFont(22)
	textStatus.Alignment = fyne.TextAlignCenter
	textStatus.TextStyle = fyne.TextStyle{Bold: true}

	sepWidth := ui.Width * 0.9

	labelSeparator := canvas.NewRectangle(colors.Gray)
	labelSeparator.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator2 := canvas.NewRectangle(colors.Gray)
	labelSeparator2.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator3 := canvas.NewRectangle(colors.Gray)
	labelSeparator3.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator4 := canvas.NewRectangle(colors.Gray)
	labelSeparator4.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator5 := canvas.NewRectangle(colors.Gray)
	labelSeparator5.SetMinSize(fyne.NewSize(sepWidth, 1))

	linkBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if originalCallerDomain != nil {
			session.Window.SetContent(originalCallerDomain)
		} else {
			session.Window.SetContent(session.LastDomain)
		}
		session.Domain = "app.tela"
		session.LastDomain = capture
		go callback()
	})

	btnFilesContracts := newSizedIconButton(theme.FolderIcon(), func() {
		session.Domain = "app.tela.manager.files" // Mark as coming from TELA Manager
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutFilesAndContracts())
		removeOverlays()
	})

	btnSettingsTela := newSizedIconButton(theme.SettingsIcon(), func() {
		session.Domain = "app.tela.manager.settings" // Mark as coming from TELA Manager
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	})

	image := canvas.NewImageFromResource(resourceTelaIcon)
	image.SetMinSize(fyne.NewSize(ui.Width*0.3, ui.Width*0.3))
	image.FillMode = canvas.ImageFillContain

	go func() {
		var iconURL string
		if cachedData != nil && cachedData.IconURL != "" && time.Now().Unix()-cachedData.LastUpdated < 3600 {
			iconURL = cachedData.IconURL
		} else {
			_, _, iconURLHdr, _, _ := getContractHeader(crypto.HashHexToHash(index.SCID))
			if iconURLHdr == "" && index.IconHdr != "" {
				iconURLHdr = index.IconHdr
			}
			iconURL = iconURLHdr
		}

		if iconURL != "" {
			if img, err := handleImageURL(index.NameHdr, iconURL, fyne.NewSize(ui.Width*0.3, ui.Width*0.3)); err == nil {
				fyne.Do(func() {
					image.Resource = img.Resource
					image.Refresh()
				})
			} else {
				logger.Errorf("[Engram] Could not validate icon image: %s\n", err)
			}
		}
	}()

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	launchProgress := NewSlimProgressBar()
	launchProgress.Hide()

	launchStatus := canvas.NewText("", colors.Yellow)
	launchStatus.TextSize = scaleFont(12)
	launchStatus.Alignment = fyne.TextAlignCenter
	launchStatus.Hide()

	spacerStatus := canvas.NewRectangle(color.Transparent)
	spacerStatus.SetMinSize(fyne.NewSize(0, 34))

	linkOpenInBrowser := widget.NewHyperlinkWithStyle("Open in Browser", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkOpenInBrowser.Hide()
	linkOpenInBrowser.OnTapped = func() {
		params := fmt.Sprintf("tela://open/%s", index.SCID)
		var toggledUpdates bool
		if !tela.UpdatesAllowed() {
			// user has accepted updated content when serving, call AllowUpdates because OpenTELALink returns error on any updated content
			tela.AllowUpdates(true)
			toggledUpdates = true
		}

		link, err := tela.OpenTELALink(params, session.Daemon)
		if toggledUpdates {
			tela.AllowUpdates(false)
		}
		if err != nil {
			logger.Errorf("[Engram] handling TELA link: %s\n", err)
			errorText.Text = "error handling TELA link"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		url, err := url.Parse(link)
		if err != nil {
			logger.Errorf("[Engram] TELA URL parse: %s\n", err)
			errorText.Text = "error could parse URL"
			errorText.Color = colors.Red
			errorText.Refresh()
		} else {
			pushTELANavigation(index.SCID)

			if a.Driver().Device().IsMobile() {
				go func() {
					time.Sleep(2 * time.Second)
					openErr := fyne.CurrentApp().OpenURL(url)
					if openErr != nil {
						logger.Errorf("[Engram] TELA OpenURL error: %s\n", openErr)
						fyne.Do(func() {
							errorText.Text = "error could not open browser"
							errorText.Color = colors.Red
							errorText.Refresh()
						})
					} else if isMobileDevice() {
						logger.Printf("[Engram] TELA Manager: Opened in mobile browser %s\n", index.SCID)
					}
				}()
			} else {
				err = fyne.CurrentApp().OpenURL(url)
				if err != nil {
					logger.Errorf("[Engram] TELA OpenURL error: %s\n", err)
					errorText.Text = "error could not open browser"
					errorText.Color = colors.Red

					if isMobileDevice() {
						fyne.Do(func() {
							dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
						})
					}
					errorText.Refresh()
				} else if isMobileDevice() {
					logger.Printf("[Engram] TELA Manager: Opened in mobile browser %s\n", index.SCID)
				}
			}
		}
	}

	btnServer := widget.NewButton("Start Application", nil)

	// Check if app is actually running via both SCID and DURL
	appActuallyRunning := false
	for _, s := range getTelaActiveServers() {
		if s.SCID == index.SCID || s.Name == index.DURL {
			appActuallyRunning = true
			break
		}
	}

	// Clean up stale launch state if app is actually running
	// This handles the case where user launches from TELA browser page and then
	// inspects the app while it's loading - we need to show current state, not stale launch state
	if appActuallyRunning {
		telaLaunchingSCIDsGlobal.Lock()
		delete(telaLaunchingSCIDsGlobal.m, index.SCID)
		telaLaunchingSCIDsGlobal.Unlock()

		telaStoppingSCIDsGlobal.Lock()
		delete(telaStoppingSCIDsGlobal.m, index.SCID)
		telaStoppingSCIDsGlobal.Unlock()

		telaLaunchCancelChansGlobal.Lock()
		delete(telaLaunchCancelChansGlobal.m, index.SCID)
		telaLaunchCancelChansGlobal.Unlock()

		telaLaunchStartTimesGlobal.Lock()
		delete(telaLaunchStartTimesGlobal.m, index.SCID)
		telaLaunchStartTimesGlobal.Unlock()
	}

	telaLaunchingSCIDsGlobal.Lock()
	isLaunchingGlobal := telaLaunchingSCIDsGlobal.m[index.SCID]
	telaLaunchingSCIDsGlobal.Unlock()

	telaStoppingSCIDsGlobal.Lock()
	isStoppingGlobal := telaStoppingSCIDsGlobal.m[index.SCID]
	telaStoppingSCIDsGlobal.Unlock()

	if isLaunchingGlobal {
		launchProgress.Show()
		if isStoppingGlobal {
			launchStatus.Text = "Stopping..."
		} else {
			launchStatus.Text = "Starting TELA app..."
		}
		launchStatus.Show()
		btnServer.Text = "Cancel"
		btnServer.SetIcon(theme.CancelIcon())

		// Sync UI with existing launch progress
		go func() {
			telaLaunchStartTimesGlobal.Lock()
			startTime, ok := telaLaunchStartTimesGlobal.m[index.SCID]
			telaLaunchStartTimesGlobal.Unlock()
			if !ok {
				return
			}

			const cap = 0.95
			const tau = 10.0
			for {
				telaLaunchingSCIDsGlobal.Lock()
				stillLaunching := telaLaunchingSCIDsGlobal.m[index.SCID]
				telaLaunchingSCIDsGlobal.Unlock()
				if !stillLaunching {
					break
				}

				elapsed := time.Since(startTime).Seconds()
				val := cap * (1.0 - math.Exp(-elapsed/tau))
				if val > cap {
					val = cap
				}

				uiDo(func() {
					if launchProgress != nil && !launchProgress.Hidden {
						launchProgress.SetValue(val)
					}
					if launchStatus != nil && !launchStatus.Hidden {
						telaStoppingSCIDsGlobal.Lock()
						isStopping := telaStoppingSCIDsGlobal.m[index.SCID]
						telaStoppingSCIDsGlobal.Unlock()
						if isStopping {
							launchStatus.Text = "Stopping..."
						} else {
							if val < 0.30 {
								launchStatus.Text = "Connecting to node..."
							} else if val < 0.60 {
								launchStatus.Text = "Fetching content..."
							} else if val < 0.85 {
								launchStatus.Text = "Preparing app..."
							} else {
								launchStatus.Text = "Almost ready..."
							}
						}
					}
				})
				time.Sleep(200 * time.Millisecond)
			}

			uiDo(func() {
				if tela.HasServer(index.DURL) {
					textStatus.Text = "Running"
					textStatus.Color = colors.Green
					textStatus.Refresh()
					btnServer.Text = "Shutdown Application"
					btnServer.SetIcon(theme.MediaStopIcon())
					btnServer.Refresh()
					launchProgress.Hide()
					launchStatus.Hide()
					linkOpenInBrowser.Show()
				} else {
					if launchProgress != nil {
						launchProgress.SetValue(launchProgress.value)
						launchProgress.Refresh()
					}
					if launchStatus != nil {
						launchStatus.Text = "Launch Error"
						launchStatus.Color = colors.Red
						launchStatus.Refresh()
					}
					if btnServer != nil {
						btnServer.Text = "Start Application"
						btnServer.SetIcon(theme.MediaPlayIcon())
						btnServer.Refresh()
					}
				}
			})
		}()
	} else if tela.HasServer(index.DURL) {
		textStatus.Text = "Running"
		textStatus.Color = colors.Green
		textStatus.Refresh()
		btnServer.Text = "Shutdown Application"
		btnServer.Refresh()
		linkOpenInBrowser.Show()
	}

	btnServer.OnTapped = func() {
		telaLaunchingSCIDsGlobal.Lock()
		isLaunching := telaLaunchingSCIDsGlobal.m[index.SCID]
		telaLaunchingSCIDsGlobal.Unlock()

		if isLaunching {
			telaStoppingSCIDsGlobal.Lock()
			telaStoppingSCIDsGlobal.m[index.SCID] = true
			telaStoppingSCIDsGlobal.Unlock()

			telaLaunchCancelChansGlobal.Lock()
			if cancelChan, ok := telaLaunchCancelChansGlobal.m[index.SCID]; ok {
				close(cancelChan)
				delete(telaLaunchCancelChansGlobal.m, index.SCID)
			}
			telaLaunchCancelChansGlobal.Unlock()
			if launchStatus != nil {
				launchStatus.Text = "Stopping..."
				launchStatus.Refresh()
			}
			btnServer.SetIcon(theme.ContentCutIcon())
			btnServer.Refresh()
		} else if btnServer.Text == "Shutdown Application" {
			tela.ShutdownServer(index.DURL)
			errorText.Text = ""
			errorText.Refresh()
			textStatus.Text = "Offline"
			textStatus.Color = colors.Gray
			textStatus.Refresh()
			btnServer.Text = "Start Application"
			btnServer.Refresh()
			linkOpenInBrowser.Hide()
			if callback != nil {
				callback()
			}
		} else {
			telaLaunchingSCIDsGlobal.Lock()
			if telaLaunchingSCIDsGlobal.m[index.SCID] {
				telaLaunchingSCIDsGlobal.Unlock()
				return
			}
			telaLaunchingSCIDsGlobal.m[index.SCID] = true
			telaLaunchingSCIDsGlobal.Unlock()

			cancelChan := make(chan struct{})
			telaLaunchCancelChansGlobal.Lock()
			telaLaunchCancelChansGlobal.m[index.SCID] = cancelChan
			telaLaunchCancelChansGlobal.Unlock()

			telaLaunchStartTimesGlobal.Lock()
			telaLaunchStartTimesGlobal.m[index.SCID] = time.Now()
			telaLaunchStartTimesGlobal.Unlock()

			launchProgress.Show()
			launchProgress.SetValue(0)
			launchStatus.Text = "Starting TELA app..."
			launchStatus.Show()
			btnServer.SetText("Cancel")
			btnServer.SetIcon(theme.CancelIcon())
			btnServer.Refresh()
			launchProgress.Refresh()
			launchStatus.Refresh()

			progressDone := make(chan struct{})
			progressStart := time.Now()
			var cancelled atomic.Bool
			go func() {
				const cap = 0.95
				const tau = 10.0
				for {
					select {
					case <-progressDone:
						return
					case <-cancelChan:
						cancelled.Store(true)
						return
					case <-time.After(200 * time.Millisecond):
						elapsed := time.Since(progressStart).Seconds()
						val := cap * (1.0 - math.Exp(-elapsed/tau))
						if val > cap {
							val = cap
						}
						uiDo(func() {
							if launchProgress != nil && !launchProgress.Hidden {
								launchProgress.SetValue(val)
							}
							if launchStatus != nil && !launchStatus.Hidden {
								telaStoppingSCIDsGlobal.Lock()
								isStopping := telaStoppingSCIDsGlobal.m[index.SCID]
								telaStoppingSCIDsGlobal.Unlock()
								if isStopping {
									launchStatus.Text = "Stopping..."
								} else {
									if val < 0.30 {
										launchStatus.Text = "Connecting to node..."
									} else if val < 0.60 {
										launchStatus.Text = "Fetching content..."
									} else if val < 0.85 {
										launchStatus.Text = "Preparing app..."
									} else {
										launchStatus.Text = "Almost ready..."
									}
								}
							}
						})
					}
				}
			}()

			cleanupLaunch := func(failed, cancelledLaunch bool) {
				close(progressDone)
				telaLaunchingSCIDsGlobal.Lock()
				delete(telaLaunchingSCIDsGlobal.m, index.SCID)
				telaLaunchingSCIDsGlobal.Unlock()
				telaLaunchCancelChansGlobal.Lock()
				delete(telaLaunchCancelChansGlobal.m, index.SCID)
				telaLaunchCancelChansGlobal.Unlock()
				telaStoppingSCIDsGlobal.Lock()
				delete(telaStoppingSCIDsGlobal.m, index.SCID)
				telaStoppingSCIDsGlobal.Unlock()
				telaLaunchStartTimesGlobal.Lock()
				delete(telaLaunchStartTimesGlobal.m, index.SCID)
				telaLaunchStartTimesGlobal.Unlock()
				uiDo(func() {
					if launchProgress != nil {
						if failed || cancelledLaunch {
							launchProgress.SetValue(launchProgress.value)
							launchProgress.Refresh()
						} else {
							launchProgress.SetValue(1.0)
							launchProgress.Refresh()
						}
					}
					if launchStatus != nil {
						if cancelledLaunch {
							launchStatus.Text = "Cancelled"
							launchStatus.Color = colors.Gray
						} else if failed {
							launchStatus.Text = "Failed"
							launchStatus.Color = colors.Red
						} else {
							launchStatus.Text = "Done!"
							launchStatus.Color = colors.Green
						}
						launchStatus.Refresh()
					}
					time.AfterFunc(400*time.Millisecond, func() {
						uiDo(func() {
							if launchProgress != nil {
								launchProgress.Hide()
							}
							if launchStatus != nil {
								launchStatus.Hide()
							}

							if failed || cancelledLaunch {
								btnServer.Text = "Start Application"
								btnServer.SetIcon(theme.MediaPlayIcon())
								btnServer.Refresh()
							}
						})
					})
				})
			}

			go func() {
				openURLAfterDelay := func(link string) {
					if a.Driver().Device().IsMobile() {
						time.Sleep(2 * time.Second)
					}
					if u, perr := url.Parse(link); perr == nil {
						fyne.CurrentApp().OpenURL(u)
					}
				}

				link, err := serveTELAWithStaleRecovery(index.SCID, session.Daemon, &cancelled)
				if cancelled.Load() {
					if err == nil {
						tela.ShutdownServer(index.SCID)
					}
					cleanupLaunch(false, true)
					return
				}

				if err == nil {
					pushTELANavigation(index.SCID)

					if err := StoreEncryptedValue("TELA History", []byte(index.SCID), []byte("")); err != nil {
						logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
					}

					go openURLAfterDelay(link)

					uiDo(func() {
						textStatus.Text = "   Running"
						textStatus.Color = colors.Green
						textStatus.Refresh()
						btnServer.Text = "Shutdown Application"
						btnServer.SetIcon(theme.MediaStopIcon())
						btnServer.Refresh()
						linkOpenInBrowser.Show()
					})

					telaActiveServersGlobal.Lock()
					telaActiveServersGlobal.active[index.SCID] = true
					telaActiveServersGlobal.Unlock()

					cleanupLaunch(false, false)
				} else {
					if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
						generation := currentWalletGeneration()
						go func() {
							if !isWalletGenerationActive(generation) {
								cleanupLaunch(true, false)
								return
							}

							telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", index.SCID)}
							linkPermission, permErr := AskPermissionForRequestE("Allow Updated Content", telaLink)
							if permErr != nil {
								logger.Errorf("[Engram] Open TELA link: %s\n", permErr)
								uiDo(func() {
									errorText.Text = "error could not open TELA"
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							if linkPermission != xswd.Allow {
								cleanupLaunch(true, false)
								return
							}

							servedLink, serveErr := serveTELAUpdates(index.SCID)
							if serveErr != nil {
								logger.Errorf("[Engram] Error serving TELA: %s\n", serveErr)
								uiDo(func() {
									errorText.Text = telaErrorToString(serveErr)
									errorText.Color = colors.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							parsedURL, parseErr := url.Parse(servedLink)
							if parseErr != nil {
								logger.Errorf("[Engram] TELA URL parse: %s\n", parseErr)
								errorText.Text = "error could parse URL"
								errorText.Color = colors.Red
								errorText.Refresh()
							} else {
								pushTELANavigation(index.SCID)

								if a.Driver().Device().IsMobile() {
									go func() {
										time.Sleep(2 * time.Second)
										openErr := fyne.CurrentApp().OpenURL(parsedURL)
										if openErr != nil {
											logger.Errorf("[Engram] TELA OpenURL error: %s\n", openErr)
											fyne.Do(func() {
												errorText.Text = "error could not open browser"
												errorText.Color = colors.Red
												errorText.Refresh()
											})
										} else {
											logger.Printf("[Engram] TELA Server: Opened in mobile browser %s\n", index.SCID)
										}
									}()
								} else {
									openErr := fyne.CurrentApp().OpenURL(parsedURL)
									if openErr != nil {
										logger.Errorf("[Engram] TELA OpenURL error: %s\n", openErr)
										errorText.Text = "error could not open browser"
										errorText.Color = colors.Red

										if isMobileDevice() {
											fyne.Do(func() {
												dialog.ShowInformation("Browser Error", "Could not open browser. Please ensure you have a browser installed.", session.Window)
											})
										}
									} else {
										logger.Printf("[Engram] TELA Server: Opened in mobile browser %s\n", index.SCID)
									}
								}
							}

							uiDo(func() {
								textStatus.Text = "   Running"
								textStatus.Color = colors.Green
								textStatus.Refresh()
								btnServer.Text = "Shutdown Application"
								btnServer.SetIcon(theme.MediaStopIcon())
								btnServer.Refresh()
								linkOpenInBrowser.Show()
							})

							telaActiveServersGlobal.Lock()
							telaActiveServersGlobal.active[index.SCID] = true
							telaActiveServersGlobal.Unlock()

							if saveErr := StoreEncryptedValue("TELA History", []byte(index.SCID), []byte("")); saveErr != nil {
								logger.Errorf("[Engram] Error saving TELA search result: %s\n", saveErr)
							}

							cleanupLaunch(false, false)
						}()
					} else {
						fyne.Do(func() {
							logger.Errorf("[Engram] Error serving TELA: %s\n", err)
							errorText.Text = telaErrorToString(err)
							errorText.Color = colors.Red
							errorText.Refresh()
						})
						cleanupLaunch(true, false)
					}
				}
			}()
		}
	}

	var ratings tela.Rating_Result
	if cachedData != nil && cachedData.Rating > 0 {
		ratings.Average = cachedData.Rating
	}

	labelRatingAverage := canvas.NewText(fmt.Sprintf("%.1f", ratings.Average), colors.Account)
	labelRatingAverage.TextSize = scaleFont(24)
	labelRatingAverage.Alignment = fyne.TextAlignCenter
	labelRatingAverage.TextStyle = fyne.TextStyle{Bold: true}

	hexagonImg := canvas.NewImageFromResource(telaHexagonColor(ratings))
	hexagonImg.SetMinSize(fyne.NewSize(80, 86))

	go func() {
		freshRatings, err := tela.GetRating(index.SCID, session.Daemon, 0)
		if err != nil {
			logger.Errorf("[Engram] GetRating: %s\n", err)
			return
		}

		fyne.Do(func() {
			ratings = freshRatings
			labelRatingAverage.Text = fmt.Sprintf("%.1f", ratings.Average)
			labelRatingAverage.Refresh()
			hexagonImg.Resource = telaHexagonColor(ratings)
			hexagonImg.Refresh()
		})

		if engram.Disk != nil && cachedData != nil {
			walletAddr := engram.Disk.GetAddress().String()
			AddTELAFavorite(walletAddr, index.SCID, cachedData.Name, cachedData.Description, cachedData.IconURL, freshRatings.Average)
		}
	}()

	linkTelaRatings := widget.NewHyperlinkWithStyle("View All Ratings", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkTelaRatings.OnTapped = func() {
		showLoadingOverlay()
		go func() {
			err := viewTELARatingsOverlay(index.NameHdr, index.SCID)
			if err != nil {
				removeOverlays()
				uiDo(func() {
					errorText.Text = err.Error()
					errorText.Color = colors.Red
					errorText.Refresh()
				})
			}
		}()
	}

	var favContainer *fyne.Container
	var favCenter *fyne.Container
	var btnFavorite *widget.Button

	btnFavoriteIcon := resourceHeartOutlineSvg
	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, index.SCID) {
			btnFavoriteIcon = resourceFavsPng
		}
	}

	btnFavorite = widget.NewButtonWithIcon("", btnFavoriteIcon, func() {
		if engram.Disk == nil {
			errorText.Text = "No wallet connected"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		walletAddress := engram.Disk.GetAddress().String()

		if IsTELAFavorite(walletAddress, index.SCID) {
			err := RemoveTELAFavorite(walletAddress, index.SCID)
			if err != nil {
				errorText.Text = "Error removing favorite"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
			btnFavorite.SetIcon(resourceHeartOutlineSvg)
			errorText.Text = "Removed from favorites"
			errorText.Color = colors.Green
		} else {
			err := AddTELAFavorite(walletAddress, index.SCID, index.NameHdr, index.DescrHdr, index.IconHdr, ratings.Average)
			if err != nil {
				errorText.Text = "Error adding favorite"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
			btnFavorite.SetIcon(resourceFavsPng)
			errorText.Text = "Added to favorites"
			errorText.Color = colors.Green
		}
		errorText.Refresh()

		if favContainer != nil {
			favContainer.Refresh()
		}
		if favCenter != nil {
			favCenter.Refresh()
		}

		if callback != nil {
			callback()
		}
	})

	favContainer = container.NewHBox(
		btnFavorite,
	)

	favCenter = container.NewCenter(
		favContainer,
	)

	center := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewStack(
				rectWidth90,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						favCenter,
						rectSpacer,
						container.NewCenter(
							image,
						),
						rectSpacer,
						labelName,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelDesc,
							),
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelStatus,
						rectSpacer,
						container.NewCenter(
							container.NewStack(
								rectWidth90,
								wrapMobileButton(btnServer),
							),
						),
						rectSpacer,
						launchStatus,
						launchProgress,
						rectSpacer,
						textStatus,
						rectSpacer,
						linkOpenInBrowser,
						rectSpacer,
						errorText,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						container.NewStack(
							container.NewHBox(
								layout.NewSpacer(),
								container.NewStack(
									hexagonImg,
									container.NewCenter(
										labelRatingAverage,
									),
								),
								layout.NewSpacer(),
							),
						),
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						container.NewCenter(
							container.NewStack(
								rectWidth90,
								wrapMobileButton(widget.NewButton("Rate", func() {
									rateTELAOverlay(index.NameHdr, index.SCID)
								})),
							),
						),
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkTelaRatings,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelDURL,
						textDURL,
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelAuthor,
						textAuthor,
						container.NewCenter(
							container.NewHBox(
								btnCopyAuthor,
								btnMessageAuthor,
							),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelSCID,
						textSCID,
						container.NewCenter(
							container.NewHBox(
								btnViewExplorer,
								btnCopySCID,
							),
						),
						rectSpacer,
						rectSpacer,
						container.NewStack(
							rectWidth90,
						),
						rectSpacer,
						rectSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
		rectSpacer,
		rectSpacer,
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(3),
					btnFilesContracts,
					linkBack,
					btnSettingsTela,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	go func() {
		time.Sleep(500 * time.Millisecond)

		servers := getTelaActiveServers()

		tempMap := make(map[string]bool)
		for _, s := range servers {
			tempMap[s.SCID] = true
			tempMap[s.Name] = true
		}

		appRunningNow := tempMap[index.SCID] || tempMap[index.DURL]

		currentButtonText := btnServer.Text

		if appRunningNow && currentButtonText != "Shutdown Application" {
			uiDo(func() {
				launchProgress.Hide()
				launchStatus.Hide()
				textStatus.Text = "Running"
				textStatus.Color = colors.Green
				textStatus.Refresh()
				btnServer.Text = "Shutdown Application"
				btnServer.SetIcon(theme.MediaStopIcon())
				btnServer.Refresh()
				linkOpenInBrowser.Show()
			})
		} else if !appRunningNow && currentButtonText == "Shutdown Application" {
			uiDo(func() {
				textStatus.Text = "Offline"
				textStatus.Color = colors.Gray
				textStatus.Refresh()
				btnServer.Text = "Start Application"
				btnServer.SetIcon(theme.MediaPlayIcon())
				btnServer.Refresh()
				linkOpenInBrowser.Hide()
			})
		}
	}()

	vScroll := NewVScroll(layout)
	cachedTelaManagerContent = vScroll

	if shouldLaunch && btnServer.Text == "Start Application" {
		go func() {
			time.Sleep(100 * time.Millisecond)
			fyne.Do(btnServer.OnTapped)
		}()
	}

	return vScroll
}
