// Copyright 2023-2024 DERO Foundation. All rights reserved.
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

var forceFreshScan bool

func isMobileDevice() bool {
	return isMobile()
}

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
	w.bg.CornerRadius = 4
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

func layoutMain() fyne.CanvasObject {
	// Set theme
	a.Settings().SetTheme(themes.main)
	session.Domain = "app.main"
	session.Path = ""
	session.Password = ""

	// Define objects

	btnLogin := widget.NewButtonWithIcon("Connect", resourceConnectPng, nil)

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
	btnNewAccount := widget.NewButtonWithIcon("New Account", theme.ContentAddIcon(), func() {
		session.Domain = "app.create"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutNewAccount())
		removeOverlays()
	})

	// Recover Account button with icon
	btnRecoverAccount := widget.NewButtonWithIcon("Recover Account", theme.DocumentIcon(), func() {
		session.Domain = "app.restore"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutRestore())
		removeOverlays()
	})

	// Connection Settings button with icon
	btnConnectionSettings := widget.NewButtonWithIcon("Connection Settings", theme.SettingsIcon(), func() {
		session.Domain = "app.settings"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSettings())
		removeOverlays()
	})

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

	walletButtons := container.NewVBox()
	var walletBtns []*walletBtn
	var extraDropdown *widget.Select
	logoGreen := color.RGBA{R: 70, G: 184, B: 104, A: 0xff}

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
		lastWalletKey := "last_wallet_" + session.Network
		if err := StoreValue("settings", []byte(lastWalletKey), []byte(walletName)); err != nil {
			logger.Debugf("[Wallets] Failed storing last selected wallet %q: %v\n", walletName, err)
		}
		safeCanvasFocus(wPassword)
		btnLogin.Refresh()
	}

	unselectButtons := func() {
		for _, b := range walletBtns {
			b.SetColors(colors.DarkMatter, colors.Gray)
		}
	}

	for i, walletName := range list {
		if i >= 3 {
			break
		}
		selectedWallet := walletName
		btn := newWalletBtn(walletName, nil)
		btn.onTapped = func() {
			unselectButtons()
			btn.SetColors(logoGreen, color.Black)
			if extraDropdown != nil {
				extraDropdown.ClearSelected()
			}
			selectWallet(selectedWallet)
		}
		walletBtns = append(walletBtns, btn)
		walletButtons.Add(wrapMobileButton(container.New(layout.NewGridLayout(1), btn)))
	}

	if len(list) > 3 {
		extraList := list[3:]
		extraDropdown = widget.NewSelect(extraList, func(s string) {
			if s == "" {
				return
			}
			unselectButtons()
			selectWallet(s)
		})
		extraDropdown.PlaceHolder = fmt.Sprintf("More wallets (%d)", len(extraList))
		walletButtons.Add(extraDropdown)
	}

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
		selectedTopButton := false
		for i, btn := range walletBtns {
			if i < len(list) && list[i] == autoSelectWallet {
				btn.SetColors(logoGreen, color.Black)
				selectedTopButton = true
				break
			}
		}
		if extraDropdown != nil && !selectedTopButton {
			extraDropdown.SetSelected(autoSelectWallet)
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

	// Separator line between Connect and more options
	separatorLine := canvas.NewRectangle(color.White)
	separatorLine.SetMinSize(fyne.NewSize(ui.Width*0.9, 1))
	separator := container.NewCenter(separatorLine)
	separatorSpacer := canvas.NewRectangle(color.Transparent)
	separatorSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

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
			separator,
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
				separator,
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

	if len(list) >= 1 {
		go func() {
			time.Sleep(100 * time.Millisecond)
			safeCanvasFocus(wPassword)
		}()
	}

	return NewVScroll(layout)
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

	gramSend := widget.NewButton(" Send ", nil)

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
	if len(tela.GetServerInfo()) > 0 {
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

	btnLogout := newSizedIconButton(theme.LogoutIcon(), func() {
		if session.Navigating {
			return
		}
		session.Navigating = true
		defer func() { session.Navigating = false }()
		closeWallet()
	})

	// Settings button with cogwheel icon
	btnSettings := newSizedIconButton(theme.SettingsIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	})

	// Datapad module button with icon
	btnDatapad := newSizedIconButton(theme.DocumentIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDatapad())
		removeOverlays()
	})

	// Messages module button with icon
	btnMessages := newSizedIconButton(theme.MailComposeIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMessages())
		removeOverlays()
	})

	// Files & Contracts module button (merged File Manager + Contract Builder)
	btnFilesContracts := newSizedIconButton(theme.FolderIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutFilesAndContracts())
		removeOverlays()
	})

	// TELA module button with logo - custom image button with proper sizing
	btnTELA := NewImageButton(resourceTelaPng, func() {
		// Log entry immediately for crash diagnosis
		logger.Printf("[TELA-BUTTON] === ENTRY - button callback started ===\n")

		if session.Navigating {
			logger.Printf("[TELA-BUTTON] Already navigating, returning early\n")
			return
		}

		// Set navigating flag with recovery to ensure it's reset on panic
		session.Navigating = true
		logger.Printf("[TELA-BUTTON] Set Navigating=true\n")

		// Primary panic recovery - catches panics in this callback
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[TELA-BUTTON] === PANIC RECOVERED ===\n")
				logger.Errorf("[TELA-BUTTON] Panic value: %v\n", r)
				logger.Errorf("[TELA-BUTTON] Stack: %s\n", debug.Stack())
				session.Navigating = false
				session.Domain = "app.wallet"

				// Safe UI update
				fyne.Do(func() {
					if session.Window != nil {
						session.Window.SetContent(layoutDashboard())
					}
				})
				logger.Printf("[TELA-BUTTON] === PANIC RECOVERY COMPLETE ===\n")
			}
		}()

		// Ensure navigating is reset on any exit
		defer func() {
			session.Navigating = false
			logger.Printf("[TELA-BUTTON] Reset Navigating=false\n")
		}()

		logger.Printf("[TELA-BUTTON] Checking state - gnomon.Index=%v walletapi.Connected=%v engram.Disk=%v session.WalletOpen=%v\n",
			gnomon.Index != nil, walletapi.Connected, engram.Disk != nil, session.WalletOpen)

		// Guard: If Gnomon is still initializing (Index is nil), show warning instead of crashing
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
			// Wait and retry after a delay
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
						}
					})
				}
			}()
			return
		}

		// Verify session state before proceeding
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

		// Safe capture of current content
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
	})

	linkHistory := newSmallIconButton("History", theme.HistoryIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutHistory())
		removeOverlays()
	})

	linkMyAccount := newSmallIconButton("My Account", theme.AccountIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAccount())
		removeOverlays()
	})

	btnTransfers := widget.NewButton("Transfers", nil)
	btnTransfers.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
		removeOverlays()
	}

	btnTransfersWrapper := wrapMobileButton(btnTransfers)
	gramSendWrapper := wrapMobileButton(gramSend)

	res.gram.SetMinSize(fyne.NewSize(ui.Width, scaleSize(150)))

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
		res.gram,
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
		rectSpacer,
		gramSendWrapper,
		rectSpacer,
		btnTransfersWrapper,
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

	gramSend.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSend())
		removeOverlays()
	}

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

	bottom := container.NewStack(
		container.NewVBox(
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(3),
					btnDatapad,
					btnTELA,
					btnMessages,
				),
			),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(3),
					btnFilesContracts,
					btnLogout,
					btnSettings,
				),
			),
			rectSpacer,
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
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

	btnSend := widget.NewButton("Save", nil)

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
		} else {
			balance, _ := engram.Disk.Get_Balance()
			entry, err := globals.ParseAmount(s)
			if err != nil {
				tx.Amount = 0
				wAmount.SetValidationError(errors.New("invalid transaction amount"))
				btnSend.Disable()
				return errors.New("invalid transaction amount")
			}

			if entry == 0 {
				tx.Amount = 0
				wAmount.SetValidationError(errors.New("invalid transaction amount"))
				btnSend.Disable()
				return errors.New("invalid transaction amount")
			}

			if entry <= balance {
				tx.Amount = entry
				wAmount.SetValidationError(nil)
				if wReceiver.Validate() == nil {
					btnSend.Enable()
				}
			} else {
				tx.Amount = 0
				btnSend.Disable()
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

	form := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			rect300,
			sendHeading,
		),
		rectSpacer,
		rectSpacer,
		wRings,
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

	grid := container.NewCenter(
		form,
	)

	top := container.NewCenter(
		layout.NewSpacer(),
		grid,
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rect300,
					wrapMobileButton(btnSend),
				),
				layout.NewSpacer(),
			),
			rectSpacer,
			container.NewHBox(
				layout.NewSpacer(),
				container.NewHBox(
					layout.NewSpacer(),
					linkCancel,
					layout.NewSpacer(),
				),
				layout.NewSpacer(),
			),
			wSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
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
			wAmount.SetValidationError(errors.New("invalid transaction amount"))
			btnCreate.Disable()
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
		return errors.New("invalid transaction amount")
	}

	wAmount.SetValidationError(nil)

	wPaymentID.Validator = func(s string) (err error) {
		tx.PaymentID, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			tx.PaymentID = 0
			btnCreate.Disable()
			wPaymentID.SetValidationError(err)
			return
		} else {
			if wReceiver.Text != "" {
				btnCreate.Enable()
				wPaymentID.SetValidationError(nil)
				return
			} else {
				err = errors.New("empty payment id")
				wPaymentID.SetValidationError(err)
				return
			}
		}
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
		if tx.Address != nil && tx.PaymentID != 0 {
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
				address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: tx.Amount})
				address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.PaymentID})
				address.Arguments = append(address.Arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Comment})

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

				linkClose := widget.NewHyperlinkWithStyle("Go Back", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
				linkClose.OnTapped = func() {
					overlay := session.Window.Canvas().Overlays()
					overlay.Top().Hide()
					overlay.Remove(overlay.Top())
					overlay.Remove(overlay.Top())
				}

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
				wReceiver.SetValidationError(errors.New("invalid address"))
				wReceiver.Refresh()
			}
		}
	}

	form := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			rect300,
			sendHeading,
		),
		rectSpacer,
		rectSpacer,
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

	grid := container.NewCenter(
		form,
	)

	top := container.NewCenter(
		layout.NewSpacer(),
		grid,
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rect300,
					btnCreate,
				),
				layout.NewSpacer(),
			),
			rectSpacer,
			container.NewHBox(
				layout.NewSpacer(),
				container.NewHBox(
					layout.NewSpacer(),
					btnBack,
					layout.NewSpacer(),
				),
				layout.NewSpacer(),
			),
			wSpacer,
		),
	)

	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutNewAccount() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
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

	footer := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			btnBack,
			layout.NewSpacer(),
		),
		wSpacer,
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
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
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

	btnCopyAddress := widget.NewButton("Copy Address", nil)

	wPassword := NewMobileEntry()
	wPassword.OnFocusGained = func() {
		scrollToFieldOnMobile(wPassword, scrollBox)
	}

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
	wPasswordConfirm.OnFocusGained = func() {
		scrollToFieldOnMobile(wPasswordConfirm, scrollBox)
	}

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

	wSpacer := widget.NewLabel(" ")
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
	hexSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(60)))

	hexForm := container.NewVBox(
		rectSpacer,
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
		rectSpacer,
		seedEntry,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnToggleSeed,
			btnPasteSeed,
			layout.NewSpacer(),
		),
		rectSpacer,
		seedInfo,
		rectSpacer,
		errorText,
		rectSpacer,
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
		rectSpacer,
		seedForm,
	)

	importFileText := canvas.NewText(" ", colors.Green)
	importFileText.TextSize = scaleFont(12)
	importFileText.Alignment = fyne.TextAlignCenter

	// Button to open file picker for import - OnTapped set after formSuccess is defined
	btnSelectFile := widget.NewButtonWithIcon("Select Wallet File", theme.FolderOpenIcon(), nil)

	importFileForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(btnSelectFile),
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
			recoveryForm.Objects[8] = hexForm
			safeCanvasFocus(hexEntry)
		case 0: // Recovery Words
			wLanguage.Hide()
			form.Objects[1].(*fyne.Container).Objects[2] = recoveryForm
			recoveryForm.Objects[8] = seedForm
			safeCanvasFocus(seedEntry)
		case 2: // Import File
			btnCreate.Disable()
			clearFormText(importFileText)
			clearFormText(errorText)
			form.Objects[1].(*fyne.Container).Objects[2] = importFileForm
		}
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

				if cachedNetwork == NETWORK_MAINNET {
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
							container.NewCenter(
								container.NewStack(
									passSpan,
									entryWalletPass,
								),
							),
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
		container.NewStack(
			rectHeader,
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
		),
	)

	scrollBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))

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

		if cachedNetwork == NETWORK_MAINNET {
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

	footer := container.NewCenter(
		rect1,
		container.NewVBox(
			wrapMobileButton(btnCreate),
			rectSpacer,
			container.NewHBox(
				layout.NewSpacer(),
				btnBack,
				layout.NewSpacer(),
			),
			wSpacer,
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
		session.Window.SetContent(layoutFilesAndContracts())
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
				btnRescan,
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

	labelSigner := canvas.NewText("   SMART  CONTRACT  AUTHOR", colors.Gray)
	labelSigner.TextSize = scaleFont(14)
	labelSigner.Alignment = fyne.TextAlignLeading
	labelSigner.TextStyle = fyne.TextStyle{Bold: true}

	labelOwner := canvas.NewText("   SMART  CONTRACT  OWNER", colors.Gray)
	labelOwner.TextSize = scaleFont(14)
	labelOwner.Alignment = fyne.TextAlignLeading
	labelOwner.TextStyle = fyne.TextStyle{Bold: true}

	labelSCID := canvas.NewText("   SMART  CONTRACT  ID", colors.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignLeading
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	labelBalance := canvas.NewText("   ASSET  BALANCE", colors.Gray)
	labelBalance.TextSize = scaleFont(14)
	labelBalance.Alignment = fyne.TextAlignLeading
	labelBalance.TextStyle = fyne.TextStyle{Bold: true}

	labelTransfer := canvas.NewText("   TRANSFER  ASSET", colors.Gray)
	labelTransfer.TextSize = scaleFont(14)
	labelTransfer.Alignment = fyne.TextAlignLeading
	labelTransfer.TextStyle = fyne.TextStyle{Bold: true}

	labelExecute := canvas.NewText("   EXECUTE  ACTION", colors.Gray)
	labelExecute.TextSize = scaleFont(14)
	labelExecute.Alignment = fyne.TextAlignLeading
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

	image := canvas.NewImageFromResource(resourceBlankPng)
	image.SetMinSize(fyne.NewSize(ui.Width*0.3, ui.Width*0.3))
	image.FillMode = canvas.ImageFillContain

	if icon != "" {
		if img, err := handleImageURL(name, icon, fyne.NewSize(ui.Width*0.3, ui.Width*0.3)); err == nil {
			image = img
		} else {
			logger.Errorf("[Engram] Could not validate icon image: %s\n", err)
		}
	}

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

	linkBack := widget.NewHyperlinkWithStyle(fmt.Sprintf("Back to %s", sessionDomainToString(captureDomain)), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkBack.OnTapped = func() {
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
	}

	image = canvas.NewImageFromResource(resourceBlankPng)
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
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	}

	linkMessageOwner := widget.NewHyperlinkWithStyle("Message the Owner", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkMessageOwner.OnTapped = func() {
		if owner != "" && owner != "--" {
			messages.Contact = owner
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

	contract, _, err = dvm.ParseSmartContract(code)
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

				btnExecute := widget.NewButton("Execute", nil)

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
								btnExecute,
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

				btnExecute.OnTapped = func() {
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

					btnExecute.Text = "Executing..."
					btnExecute.Disable()
					btnExecute.Refresh()

					storage, err := executeContractFunction(hash, ringsize, dero_amount, asset_amount, funcName.Text, params)
					if err != nil {
						if strings.Contains(err.Error(), "somehow the tx could not be built") {
							btnExecute.Text = fmt.Sprintf("Insufficient Balance: Need %v", globals.FormatMoney(storage))
						} else if strings.Contains(err.Error(), "Discarded knowingly") {
							btnExecute.Text = "Error... discarded knowingly"
						} else if strings.Contains(err.Error(), "Recovered in function") {
							btnExecute.Text = "Error... invalid input"
						} else {
							btnExecute.Text = "Error executing function..."
						}
						btnExecute.Disable()
						btnExecute.Refresh()
					} else {
						btnExecute.Text = "Function executed successfully!"
						btnExecute.Disable()
						btnExecute.Refresh()
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
						btnSend,
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
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
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
			center,
		),
	)

	return NewVScroll(layout)
}

func layoutTransfers() fyne.CanvasObject {
	session.Domain = "app.transfers"

	wSpacer := widget.NewLabel(" ")

	sendTitle := canvas.NewText("T R A N S F E R S", colors.Gray)
	sendTitle.TextStyle = fyne.TextStyle{Bold: true}
	sendTitle.TextSize = scaleFont(16)

	sendDesc := canvas.NewText("", colors.Gray)
	sendDesc.TextSize = scaleFont(18)
	sendDesc.Alignment = fyne.TextAlignCenter
	sendDesc.TextStyle = fyne.TextStyle{Bold: true}

	sendHeading := canvas.NewText("S A V E D    T R A N S F E R S", colors.Gray)
	sendHeading.TextSize = scaleFont(16)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))
	frame := &iframe{}
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(30)))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
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
		session.Window.SetContent(layoutDashboard())
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

	btnSend := widget.NewButton("Send Transfers", nil)

	btnClear := widget.NewButton("Clear", func() {
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
						rectSpacer,
						rectSpacer,
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
						btnSubmit,
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

		safeCanvasFocus(entryPassword)
	}

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.transfers" {
			return
		}

		if k.Name == fyne.KeyDown {
			session.Dashboard = "main"
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		}
	})

	sendForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		sendHeading,
		rectSpacer,
		rectSpacer,
		container.NewStack(
			rectListBox,
			scrollBox,
		),
		wSpacer,
		wrapMobileButton(btnSend),
		rectSpacer,
		wrapMobileButton(btnClear),
		rectSpacer,
		rectSpacer,
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

	c := container.NewBorder(
		features,
		bottom,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

func layoutTransfersDetail(index int) fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("T R A N S F E R    D E T A I L", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

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

	linkBack := widget.NewHyperlinkWithStyle("Back to Transfers", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkBack.OnTapped = func() {
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutTransfers())
	}

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

	top := container.NewVBox(
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
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
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
		restoreLabel := widget.NewLabel("Reset all settings to defaults?")
		restoreLabel.Wrapping = fyne.TextWrapWord
		dialog.ShowCustomConfirm("Restore Defaults", "No", "Yes", restoreLabel, func(confirmed bool) {
			if confirmed {
				return
			}

			setNetwork(NETWORK_MAINNET)
			setDaemon(DEFAULT_REMOTE_DAEMON)
			setAuthMode("true")
			setGnomon("1")

			// Clear saved nodes for all networks
			StoreValue("settings", []byte("mainnet_nodes"), []byte{})
			StoreValue("settings", []byte("testnet_nodes"), []byte{})
			StoreValue("settings", []byte("simulator_nodes"), []byte{})

			// Regenerate RPC credentials
			remoteAccess.RPC.user = newRPCUsername()
			remoteAccess.RPC.pass = newRPCPassword()
			StoreValue("settings", []byte("rpc_user"), []byte(remoteAccess.RPC.user))
			StoreValue("settings", []byte("rpc_pass"), []byte(remoteAccess.RPC.pass))

			resizeWindow(ui.MaxWidth, ui.MaxHeight)
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutSettings())
			removeOverlays()
		}, session.Window)
	}

	statusText := canvas.NewText("", colors.Account)
	statusText.TextSize = scaleFont(12)

	btnDelete.OnTapped = func() {
		clearLabel := widget.NewLabel(fmt.Sprintf("Delete all local %s data?", strings.ToLower(session.Network)))
		clearLabel.Wrapping = fyne.TextWrapWord
		dialog.ShowCustomConfirm("Clear Local Data", "No", "Yes", clearLabel, func(confirmed bool) {
			if confirmed {
				return
			}

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
		}, session.Window)
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

	gridItem1 := container.NewCenter(
		container.NewVBox(
			heading,
			scrollBox,
			rectSpacer,
		),
	)

	features := container.NewCenter(
		layout.NewSpacer(),
		gridItem1,
		layout.NewSpacer(),
	)

	footer := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			btnBack,
			layout.NewSpacer(),
		),
	)

	c := container.NewBorder(
		features,
		footer,
		nil,
		nil,
	)

	return NewVScroll(c)
}

// layoutAppSettings creates the centralized settings page with 3 tabs:
// Remote Access, TELA, and Advanced
func layoutAppSettings() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	previousDomain := session.Domain // Save before overwriting
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
	entryPortStart := widget.NewEntry()
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
	entryMinLikes := widget.NewEntry()
	entryMinLikes.SetPlaceHolder("30")
	if storedMinLikes, err := GetEncryptedValue("TELA Settings", []byte("Min Likes")); err == nil {
		entryMinLikes.SetText(string(storedMinLikes))
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
			StoreEncryptedValue("TELA Settings", []byte("Min Likes"), []byte(s))
		}
	}

	// Exclusions entry
	entryExclusions := widget.NewEntry()
	entryExclusions.SetPlaceHolder("dURL Exclusions (exclude1,exclude2)")
	if storedExclusions, err := GetEncryptedValue("TELA Settings", []byte("Exclusions")); err == nil {
		entryExclusions.SetText(string(storedExclusions))
	}
	entryExclusions.OnChanged = func(s string) {
		if s != "" {
			StoreEncryptedValue("TELA Settings", []byte("Exclusions"), []byte(s))
		} else {
			DeleteKey("TELA Settings", []byte("Exclusions"))
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
		if storedTelaMode, err := GetEncryptedValue("TELA Settings", []byte("Mode")); err == nil {
			if string(storedTelaMode) == "Unrestrictive" {
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
			if engram.Disk != nil {
				DeleteKey("TELA Settings", []byte("Restrictive Mode"))
				DeleteKey("TELA Settings", []byte("Mode"))
			}
			DeleteKey("TELASettingsUnencrypted", []byte("Restrictive Mode"))
			DeleteKey("TELASettingsUnencrypted", []byte("Mode"))
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
	if storedRescanRecheck, err := GetEncryptedValue("TELA Settings", []byte("Rescan Recheck")); err == nil {
		if string(storedRescanRecheck) == "Yes" {
			wRescanRecheck.SetSelectedIndex(1)
		} else {
			wRescanRecheck.SetSelectedIndex(0)
		}
	} else {
		wRescanRecheck.SetSelectedIndex(0)
	}
	wRescanRecheck.OnChanged = func(s string) {
		StoreEncryptedValue("TELA Settings", []byte("Rescan Recheck"), []byte(s))
	}

	// Sort By dropdown
	sortByOptions := []string{"Ratings", "A-Z", "Z-A"}
	wSortBy := widget.NewSelect(sortByOptions, nil)
	if storedSortBy, err := GetEncryptedValue("TELA Settings", []byte("Sort By")); err == nil {
		wSortBy.SetSelected(string(storedSortBy))
	} else {
		wSortBy.SetSelected(sortByOptions[0])
	}
	wSortBy.OnChanged = func(s string) {
		if s != "" {
			StoreEncryptedValue("TELA Settings", []byte("Sort By"), []byte(s))
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

	entryTrackBlocks := widget.NewEntry()
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
		clearLabel := widget.NewLabel(fmt.Sprintf("Delete all local %s data?", strings.ToLower(session.Network)))
		clearLabel.Wrapping = fyne.TextWrapWord
		dialog.ShowCustomConfirm("Clear Local Data", "No", "Yes", clearLabel, func(confirmed bool) {
			if confirmed {
				return
			}

			err := cleanGnomonData()
			if err != nil {
				if parseError, ok := err.(*os.PathError); !ok {
					err = fmt.Errorf("error clearing local %s data", session.Network)
				} else {
					err = parseError.Err
				}

				// Show error dialog
				errorDialog := dialog.NewError(err, session.Window)
				errorDialog.SetOnClosed(func() {})
				errorDialog.Show()
				return
			}

			// Show success notification
			successDialog := dialog.NewInformation("Success", fmt.Sprintf("Gnomon %s data successfully deleted.", strings.ToLower(session.Network)), session.Window)
			successDialog.SetOnClosed(func() {})
			successDialog.Show()
		}, session.Window)
	})

	btnRestoreDefaults := widget.NewButton("Restore Defaults", func() {
		restoreLabel := widget.NewLabel("Reset all settings to defaults?")
		restoreLabel.Wrapping = fyne.TextWrapWord
		dialog.ShowCustomConfirm("Restore Defaults", "No", "Yes", restoreLabel, func(confirmed bool) {
			if confirmed {
				return
			}

			// Reset all settings to defaults
			setNetwork(NETWORK_MAINNET)
			setDaemon(DEFAULT_REMOTE_DAEMON)
			setAuthMode("true")
			setGnomon("1")
			remoteAccess.RPC.user = "username"
			remoteAccess.RPC.pass = "password"
			remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
			setRemoteAccess(remoteAccess.RPC.port, "RPC")

			// Show success notification
			successDialog := dialog.NewInformation("Success", "All settings have been restored to defaults.", session.Window)
			successDialog.SetOnClosed(func() {})
			successDialog.Show()
		}, session.Window)
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
		// Return to TELA if user came from there, otherwise dashboard
		if previousDomain == "app.tela" || previousDomain == "app.tela.manager" || previousDomain == "app.tela.settings" {
			session.Window.SetContent(layoutTELA())
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

	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.Height*0.8))

	gridItem1 := container.NewCenter(
		container.NewVBox(
			rectSpacer,
			heading,
			scrollBox,
			rectSpacer,
			rectSpacer,
		),
	)

	features := container.NewCenter(
		layout.NewSpacer(),
		gridItem1,
		layout.NewSpacer(),
	)

	footer := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			btnBack,
			layout.NewSpacer(),
		),
		widget.NewLabel(" "),
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

	// Register with navigation stack (app settings allows back navigation)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutMessages() fyne.CanvasObject {
	session.Domain = "app.messages"

	if !walletapi.Connected {
		session.Window.SetContent(layoutSettings())
	}

	title := canvas.NewText("M Y    C O N T A C T S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

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
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))
	frame := &iframe{}
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(30)))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	rect.SetMinSize(statusDotSize())
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.43))

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

	rebuildBtn := widget.NewButton("Rebuild Message History", func() {
		rebuildMessageHistory()
	})

	btnSend := widget.NewButton("New Message", func() {
		_, err := globals.ParseValidateAddress(messages.Contact)
		if err != nil {
			//_, err := engram.Disk.NameToAddress(messages.Contact)
			_, err := checkUsername(messages.Contact, -1)
			if err != nil {
				return
			}
		}

		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutPM())
		removeOverlays()
	})
	btnSend.Disable()

	contactInput := widget.NewEntry()
	contactInput.MultiLine = false
	contactInput.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	contactInput.PlaceHolder = "Search username or address"

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

	messageForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			title,
			layout.NewSpacer(),
		),
		rectSpacer,
		rectSpacer,
		contactInput,
		rectSpacer,
		rectSpacer,
		container.NewStack(
			rectListBox,
			msgbox.List,
		),
		rectSpacer,
		btnSend,
		rectSpacer,
		rebuildBtn,
		rectSpacer,
		checkLimit,
	)

	gridItem1 := container.NewCenter(
		messageForm,
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

	c := container.NewBorder(
		features,
		subContainer,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return layout
}

func layoutPM() fyne.CanvasObject {
	session.Domain = "app.messages.contact"

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

	title := canvas.NewText("M E S S A G E S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	heading := canvas.NewText(contactAddress, colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	lastActive := canvas.NewText("", colors.Gray)
	lastActive.TextSize = scaleFont(12)
	lastActive.Alignment = fyne.TextAlignCenter
	lastActive.TextStyle = fyne.TextStyle{Bold: false}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMessages())
		removeOverlays()
	})

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.7, 30))
	frame := &iframe{}
	subframe := canvas.NewRectangle(color.Transparent)
	subframe.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.51))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	rect.SetMinSize(statusDotSize())
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

	entry := widget.NewEntry()
	entry.MultiLine = false
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.PlaceHolder = "Message"
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

	messageForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			heading,
			layout.NewSpacer(),
		),
		rectSpacer,
		lastActive,
		rectSpacer,
		threadSearch,
		rectSpacer,
		rectSpacer,
		container.NewStack(
			subframe,
			chatbox,
		),
		rectSpacer,
		rectSpacer,
		labelLimit,
		rectSpacer,
		entry,
		rectSpacer,
		btnSend,
		rectSpacer,
		rectSpacer,
	)

	gridItem1 := container.NewCenter(
		messageForm,
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

	subContainer := container.NewStack(
		container.NewVBox(
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

	c := container.NewBorder(
		features,
		subContainer,
		nil,
		nil,
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

	linkBack := widget.NewHyperlinkWithStyle("Back to Dashboard", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkBack.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	}

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
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
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
		for _, serv := range tela.GetServerInfo() {
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
			center,
			bottom,
			nil,
			nil,
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

	shardForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				title,
				rectSpacer,
			),
		),
		rectSpacer,
		container.NewStack(
			container.NewCenter(
				textUsername,
			),
		),
		rectSpacer,
		idCenter,
		rectSpacer,
		rectSpacer,
		container.NewStack(
			rectListBox,
			userBox,
		),
		rectSpacer,
		entryReg,
		rectSpacer,
		wrapMobileButton(btnReg),
		rectSpacer,
		rectSpacer,
		rectSpacer,
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

	c := container.NewBorder(
		features,
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

func layoutIdentityDetail(username string) fyne.CanvasObject {
	var address string

	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("I D E N T I T Y    D E T A I L", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

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

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
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
						valueUsername,
						rectSpacer,
						labelUsername,
						wSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								btnSetPrimary,
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
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								btnSend,
							),
							layout.NewSpacer(),
						),
					),
					layout.NewSpacer(),
				),
			),
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
			center,
		),
	)

	return layout
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

	waitForm := container.NewVBox(
		widget.NewLabel(""),
		container.NewHBox(
			layout.NewSpacer(),
			title,
			layout.NewSpacer(),
		),
		widget.NewLabel(""),
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

	footer := container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			link,
			layout.NewSpacer(),
		),
		widget.NewLabel(""),
	)

	c := container.NewBorder(
		grid,
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
	var entries []rpc.Entry
	var zeroscid crypto.Hash
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
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

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

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
			co.(*fyne.Container).Objects[2].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[3])
		})

	rectSpacer := canvas.NewRectangle(color.Transparent)
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
		if transfers, normalRows, coinbaseRows, messageRows, ok := getHistoryRowCache(); ok {
			cachedTransfers = transfers
			historyNormalRows = normalRows
			historyCoinbaseRows = coinbaseRows
			historyMessageRows = messageRows
			return
		}

		entries = engram.Disk.Show_Transfers(zeroscid, true, true, true, 0, engram.Disk.Get_Height(), "", "", 0, 0)
		cachedTransfers = append([]rpc.Entry(nil), entries...)
		messages := scanMessageTransfers(0)
		historyNormalRows, historyCoinbaseRows, historyMessageRows = buildHistoryRows(entries, messages)
		setHistoryRowCache(cachedTransfers, historyNormalRows, historyCoinbaseRows, historyMessageRows)
	}

	// Function to load Normal transactions
	loadNormal := func() {
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
				listBox.ScrollToBottom()
			})
		}()
	}

	// Function to load Coinbase transactions
	loadCoinbase := func() {
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
				listBox.ScrollToBottom()
			})
		}()
	}

	// Function to load Messages
	loadMessages := func() {
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
				listBox.ScrollToBottom()
			})
		}()
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

	center := container.NewStack(
		rectWidth,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				tabs,
				rectSpacer,
				results,
				rectSpacer,
				rectSpacer,
				container.NewStack(
					rectList,
					listBox,
				),
			),
			layout.NewSpacer(),
		),
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			center,
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
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

func layoutHistoryDetail(txid string, transfer rpc.Entry) fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	frame := &iframe{}

	heading := canvas.NewText("T R A N S A C T I O N    D E T A I L", colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

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
		if amount < 0 {
			amount = -amount
		}
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

	btnView := widget.NewButton("View in Explorer", nil)
	btnView.OnTapped = func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + txid)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + txid)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	}

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

	top := container.NewVBox(
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
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
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

	heading := canvas.NewText("", colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	entryNewPad := widget.NewEntry()
	entryNewPad.MultiLine = false
	entryNewPad.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	btnAdd := widget.NewButton(" Create ", nil)
	btnAdd.Disable()
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

	rectSpacer := canvas.NewRectangle(color.Transparent)
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

	if len(padData) > 0 {
		listHeight := float32(len(padData) * 42)
		if listHeight > 280 {
			listHeight = 280
		}
		rectListBox.SetMinSize(fyne.NewSize(ui.Width, listHeight))
	}

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

	shardForm := container.NewVBox(
		rectSpacer,
		container.NewCenter(container.NewVBox(title, rectSpacer)),
		rectSpacer,
		entryNewPad,
		rectSpacer,
		wrapMobileButton(btnAdd),
		rectSpacer,
		container.NewStack(
			rectListBox,
			padBox,
		),
		rectSpacer,
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

	subContainer := container.NewStack(
		container.NewVBox(
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

	c := container.NewBorder(
		features,
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

func layoutPad() fyne.CanvasObject {
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectEntry := canvas.NewRectangle(color.Transparent)
	rectEntry.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.52))

	heading := canvas.NewText(session.Datapad, colors.Green)
	heading.TextSize = scaleFont(20)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

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

	top := container.NewVBox(
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

	center := container.NewStack(
		rectWidth,
		container.NewCenter(
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
				linkBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
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

	return NewVScroll(layout)
}

func layoutAccount() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))
	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 0))

	title := canvas.NewText("M Y    A C C O U N T", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

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

	linkCopyAddress := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopyAddress.OnTapped = func() {
		a.Clipboard().SetContent(engram.Disk.GetAddress().String())
	}

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

	btnIdentity := widget.NewButtonWithIcon("Identity", theme.AccountIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutIdentity())
		removeOverlays()
	})

	btnServiceAddress := widget.NewButtonWithIcon("Payment", theme.ComputerIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutServiceAddress())
		removeOverlays()
	})

	buttonsRow := container.NewHBox(
		layout.NewSpacer(),
		wrapMobileButton(btnServiceAddress),
		rectSpacer,
		wrapMobileButton(btnIdentity),
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

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(header),
						rectSpacer,
						rectSpacer,
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
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							btnBack,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
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

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(header),
						rectSpacer,
						rectSpacer,
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
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							btnBack,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
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

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(header),
						rectSpacer,
						rectSpacer,
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
						btnChange,
						widget.NewLabel(""),
						container.NewHBox(
							layout.NewSpacer(),
							btnBack,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
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
				container.NewCenter(
					container.NewVBox(
						title,
						rectSpacer,
					),
				),
				rectSpacer,
				container.NewCenter(
					container.NewHBox(
						heading,
						addressToggleBtn,
					),
				),
				container.NewHBox(
					layout.NewSpacer(),
					linkCopyAddress,
					layout.NewSpacer(),
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
			container.NewCenter(
				btnBack,
			),
		),
	)

	layout := container.NewBorder(
		features,
		bottom,
		nil,
		nil,
	)

	return NewVScroll(layout)
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

	linkCancel := widget.NewHyperlinkWithStyle("Back to My Account", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCancel.OnTapped = func() {
		removeOverlays()
	}

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
			linkCancel,
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

	linkCancel := widget.NewHyperlinkWithStyle("Back to My Account", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCancel.OnTapped = func() {
		removeOverlays()
	}

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
			linkCancel,
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

	// Back button to return to dashboard
	btnBack := wrapMobileButton(newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	}))

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
		container.NewHBox(
			layout.NewSpacer(),
			btnSignFile,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnVerifyFile,
			layout.NewSpacer(),
		),
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
		container.NewTabItem("Browse", browseTabContent),
		container.NewTabItem("SCIDs", contractsTabContent),
		assetTab,
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
		contract, _, err := dvm.ParseSmartContract(filedata)
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
				contract, pos, err := dvm.ParseSmartContract(code)
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

			_, pos, err := dvm.ParseSmartContract(entryCode.Text)
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

			contract, pos, err := dvm.ParseSmartContract(entryCode.Text)
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

			contract, pos, err := dvm.ParseSmartContract(entryCode.Text)
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

			contract, pos, err := dvm.ParseSmartContract(code)
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

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	var telaSearch []INDEXwithRatings

	telaListHeartWidth := scaleSize(34)
	if isMobileLayout {
		telaListHeartWidth = scaleSize(40)
	}

	telaListButtonWidth := scaleSize(70)
	if isMobileLayout {
		telaListButtonWidth = scaleSize(80)
	}

	parseTelaListEntry := func(raw string) (name, scid string) {
		split := strings.Split(raw, ";;;")
		if len(split) > 0 {
			name = split[0]
		}
		if len(split) > 1 {
			scid = split[1]
		}
		return
	}

	normalizeSearch := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}

	findTelaSearchEntry := func(scid string) *INDEXwithRatings {
		for i := range telaSearch {
			if telaSearch[i].SCID == scid {
				return &telaSearch[i]
			}
		}

		return nil
	}

	isTelaActive := func(scid string) bool {
		for _, serv := range tela.GetServerInfo() {
			if serv.SCID == scid {
				return true
			}
		}

		return false
	}

	isDisplayableTelaApp := func(index tela.INDEX) bool {
		if len(index.DOCs) < 1 {
			return false
		}

		if strings.HasSuffix(index.DURL, tela.TAG_LIBRARY) || strings.HasSuffix(index.DURL, tela.TAG_DOC_SHARDS) || strings.HasSuffix(index.DURL, tela.TAG_BOOTSTRAP) {
			return false
		}

		name := strings.TrimSpace(index.NameHdr)
		descr := strings.TrimSpace(index.DescrHdr)
		if name == "" || descr == "" {
			return false
		}

		return true
	}

	setTelaStatus := func(text string, clr color.Color) {
		if telaStatus.Text == text && telaStatus.Color == clr {
			return
		}
		telaStatus.Text = text
		telaStatus.Color = clr
		fyne.Do(func() {
			telaStatus.Refresh()
		})
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
		})
	}

	hideTelaProgress := func() {
		fyne.Do(func() {
			telaProgress.Hide()
		})
	}

	newTelaListItem := func() fyne.CanvasObject {
		heartBtn := widget.NewButtonWithIcon("", resourceHeartOutlineSvg, nil)
		heartBtn.Importance = widget.LowImportance

		activeBg := canvas.NewRectangle(color.Transparent)
		activeBg.SetMinSize(fyne.NewSize(0, scaleSize(36)))

		heartSpacer := canvas.NewRectangle(color.Transparent)
		heartSpacer.SetMinSize(fyne.NewSize(telaListHeartWidth, scaleSize(34)))
		heartWrap := container.NewCenter(container.NewStack(heartSpacer, container.NewCenter(heartBtn)))

		nameLabel := widget.NewLabel("")
		nameLabel.Alignment = fyne.TextAlignLeading
		nameLabel.Truncation = fyne.TextTruncateEllipsis
		nameLabel.Wrapping = fyne.TextWrapOff

		startCloseBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), nil)
		startCloseBtn.Importance = widget.LowImportance

		btnSpacer := canvas.NewRectangle(color.Transparent)
		btnSpacer.SetMinSize(fyne.NewSize(telaListButtonWidth, scaleSize(34)))
		btnWrap := container.NewCenter(container.NewStack(btnSpacer, container.NewCenter(startCloseBtn)))

		return container.NewStack(
			activeBg,
			container.NewBorder(
				nil,
				nil,
				heartWrap,
				btnWrap,
				container.NewPadded(nameLabel),
			),
		)
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
			entry := findTelaSearchEntry(scid)
			if entry == nil {
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
		if isTelaActive(scid) {
			activeBg.FillColor = color.NRGBA{R: 20, G: 120, B: 70, A: 48}
			startCloseBtn.SetText("Close")
			startCloseBtn.SetIcon(theme.MediaStopIcon())
		} else {
			activeBg.FillColor = color.Transparent
			startCloseBtn.SetText("Start")
			startCloseBtn.SetIcon(theme.MediaPlayIcon())
		}
		activeBg.Refresh()
		updateTelaFavoriteButton(heartBtn, scid)
		heartBtn.OnTapped = func() {
			toggleTelaFavorite(scid)
		}
		startCloseBtn.OnTapped = func() {
			if isTelaActive(scid) {
				entry := findTelaSearchEntry(scid)
				if entry != nil {
					tela.ShutdownServer(entry.DURL)
					if refreshServerList != nil {
						refreshServerList()
					}
					searchList.Refresh()
					favoritesList.Refresh()
				}
			} else {
				if engram.Disk == nil {
					errorText.Text = "No wallet connected"
					errorText.Color = colors.Gray
					errorText.Refresh()
					return
				}
				errorText.Text = ""
				errorText.Refresh()
				go func() {
					if link, err := tela.ServeTELA(scid, session.Daemon); err == nil {
						pushTELANavigation(scid)
						if u, err := url.Parse(link); err == nil {
							fyne.CurrentApp().OpenURL(u)
						}
						if err := StoreEncryptedValue("TELA History", []byte(scid), []byte("")); err != nil {
							logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
						}
						fyne.Do(func() {
							if refreshServerList != nil {
								refreshServerList()
							}
							searchList.Refresh()
							favoritesList.Refresh()
						})
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
								return
							}

							if linkPermission != xswd.Allow {
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
								return
							}

							pushTELANavigation(scid)
							if u, parseErr := url.Parse(link); parseErr == nil {
								fyne.CurrentApp().OpenURL(u)
							}
							fyne.Do(func() {
								if refreshServerList != nil {
									refreshServerList()
								}
								searchList.Refresh()
								favoritesList.Refresh()
							})
						} else {
							logger.Printf("[TELA] ServeTELA failed for SCID %s: %v", scid, err)
							fyne.Do(func() {
								errorText.Text = "error starting TELA app"
								errorText.Color = colors.Red
								errorText.Refresh()
							})
						}
					}
				}()
			}
		}
	}

	historyData = binding.BindStringList(&history)
	historyList = widget.NewListWithData(historyData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewVBox(
					container.NewStack(
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

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*widget.Label).SetText(split[0])
		},
	)

	searchData = binding.BindStringList(&searching)
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
		rescanLabel := widget.NewLabel("Force Full Rescan?\n\nThis will clear all cached results, reset the Gnomon index, and perform a complete fresh scan. This may take several minutes.")
		rescanLabel.Wrapping = fyne.TextWrapWord

		dlg := dialog.NewCustomWithoutButtons("TELA BROWSER", rescanLabel, session.Window)

		btnConfirm := widget.NewButtonWithIcon("Rescan", theme.ViewRefreshIcon(), func() {
			dlg.Hide()
			clearAllTELACache()
			forceFreshScan = true
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

	btnTela := widget.NewButtonWithIcon("Apps", resourceBrowserGlobeSvg, func() {
		if wSelect.Selected == "Search" {
			activateTelaSearch()
			return
		}
		wSelect.SetSelected("Search")
	})
	btnTela.Importance = widget.LowImportance

	favoritesLabel := ""
	if isMobileDevice() {
		favoritesLabel = "Favorites"
	}
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
		tabButtons = container.NewGridWithColumns(3,
			wrapMobileButton(btnTela),
			wrapMobileButton(btnFavorites),
			wrapMobileButton(btnHistory),
		)
	} else {
		tabButtons = container.NewHBox(
			btnTela,
			btnFavorites,
			btnHistory,
		)
	}

	btnShutdown := widget.NewButton("Shutdown TELA", nil)

	var restrictiveMode, rescanRecheck bool
	var lastScan, searchExclusions, sortBy string
	var minLikes float64
	var telaSCIDs []string
	var sAll = map[string]bool{}
	var telaStartupWaitMu sync.Mutex
	telaStartupWaiting := false

	// Initialize TELA settings from storage
	if storedMinLikes, err := GetEncryptedValue("TELA Settings", []byte("Min Likes")); err == nil {
		if f, err := strconv.ParseFloat(string(storedMinLikes), 64); err == nil {
			minLikes = f
		}
	} else {
		minLikes = 30
	}

	if storedExclusions, err := GetEncryptedValue("TELA Settings", []byte("Exclusions")); err == nil {
		searchExclusions = string(storedExclusions)
	}

	if storedRescanRecheck, err := GetEncryptedValue("TELA Settings", []byte("Rescan Recheck")); err == nil {
		if string(storedRescanRecheck) == "Yes" {
			rescanRecheck = true
		}
	}

	sortByOptions := []string{"Ratings", "A-Z", "Z-A"}
	if storedSortBy, err := GetEncryptedValue("TELA Settings", []byte("Sort By")); err == nil {
		sortBy = string(storedSortBy)
	} else {
		sortBy = sortByOptions[0]
	}

	restrictiveMode = false // Default OFF (unrestrictive)
	// First check new "Restrictive Mode" key (set by Settings page)
	if restrictiveModeValue, found := getTELADual("Restrictive Mode"); found {
		if restrictiveModeValue == "true" {
			restrictiveMode = true
		}
	} else {
		// Fallback to legacy "Mode" key for backward compatibility
		if storedTelaMode, err := GetEncryptedValue("TELA Settings", []byte("Mode")); err == nil {
			if string(storedTelaMode) == "Restrictive" {
				restrictiveMode = true
			}
		}
	}

	var getSearchResults func()
	getSearchResults = func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[TELA-SEARCH] getSearchResults PANIC recovered: %v\n", r)
				isSearching = false
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
		var phasePrefilterMs int64
		var phaseScanMs int64
		var phaseFinalizeMs int64
		cacheHitMode := "full"
		fullScanReason := "cold_start"
		cacheIntegrity := "ok"
		var heightDelta int64
		var storedIndexedHeight int64

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
			telaSearch = []INDEXwithRatings{}
			telaSCIDs = []string{}
			sAll = map[string]bool{}
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))
			forceFreshScan = false
			clearScanProgress()
			fullScanReason = "force_fresh_scan"
		}

		// Check for existing progress and handle resume scenarios
		progress := loadScanProgress()
		resumePosition := 0

		if progress.State == "completed" && !isScanProgressStale(progress, 24) {
			// Use cached results - progress is valid, already scanned
		} else if progress.State == "interrupted" && !isScanProgressStale(progress, 24) {
			// Resume from interrupted scan
			resumePosition = progress.Position
			results.Text = fmt.Sprintf("  Resuming scan from position %d...", resumePosition)
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

		// Already scanned (skip if force fresh scan was just triggered)
		if len(telaSearch) > 0 {
			fyne.Do(func() {
				searching = telaSearchDisplayAll(telaSearch, sortBy)
				searchData.Set(searching)
				searchList.Refresh()
				results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(telaSearch))
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
			})

			return
		}

		telaSearch = []INDEXwithRatings{}
		searchData.Set(nil)
		labelLastScan.Text = ""

		fyne.Do(func() {
			btnShutdown.Disable()
			labelLastScan.Refresh()
		})

		defer func() {
			isSearching = false
			setTelaStatus("", color.Transparent)
			hideTelaProgress()
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

		if gnomon.Index == nil {
			results.Text = "  Gnomon is starting..."
			results.Color = colors.Yellow
			setTelaStatus("Starting Gnomon index...", colors.Yellow)
			setTelaProgress(0.05)
			fyne.Do(func() {
				results.Refresh()
			})
			return
		}

		hasTelaCache := func() bool {
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

		gnomonReadyForTela := func() bool {
			if gnomon.Index == nil {
				return false
			}
			// If we already have cached TELA search results, we can proceed immediately
			// without waiting for gnomon to fully sync - use cached data to display
			if hasTelaCache() || len(telaSearch) > 0 {
				return true
			}
			// Double-check after cache check to avoid race
			if gnomon.Index == nil {
				return false
			}
			// Check if backends are initialized
			if (gnomon.Index.DBType == "gravdb" && gnomon.Index.GravDBBackend == nil) ||
				(gnomon.Index.DBType == "boltdb" && gnomon.Index.BBSBackend == nil) {
				return false
			}

			allOwnersAndSCIDs := gnomon.GetAllOwnersAndSCIDs()
			if len(allOwnersAndSCIDs) <= 1 {
				return false
			}

			return isGnomonCaughtUp()
		}

		for !gnomonReadyForTela() {
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

			fyne.Do(func() {
				entrySearch.Disable()
				entryAddSCID.Disable()
			})
			results.Text = "  Gnomon is starting..."
			results.Color = colors.Yellow
			if gnomon.Index == nil || ((gnomon.Index.DBType == "gravdb" && gnomon.Index.GravDBBackend == nil) || (gnomon.Index.DBType == "boltdb" && gnomon.Index.BBSBackend == nil)) {
				setTelaStatus("Starting Gnomon index...", colors.Yellow)
				setTelaProgress(0.1)
			} else {
				daemonHeight := currentDaemonHeight()
				indexedHeight := int64(0)
				if gnomon.Index != nil {
					indexedHeight = gnomon.Index.LastIndexedHeight
				}
				progress := 0.2
				if daemonHeight > 0 {
					progress = math.Max(0.2, math.Min(0.9, float64(indexedHeight)/float64(daemonHeight)))
				}
				setTelaStatus(fmt.Sprintf("Syncing Gnomon index... (%d / %d)", indexedHeight, daemonHeight), colors.Yellow)
				setTelaProgress(progress)
			}

			// Save syncing state for progress tracking
			var daemonHeight int
			if engram.Disk != nil {
				daemonHeight = int(engram.Disk.Get_Daemon_Height())
			}
			if gnomon.Index != nil {
				saveProgress(int(gnomon.Index.LastIndexedHeight), daemonHeight, "", "syncing")
			}

			fyne.Do(func() {
				results.Refresh()
			})

			time.Sleep(time.Second)
		}

		allowTelaIndexMutations := isGnomonCaughtUp()

		if gnomon.Index != nil && gnomon.Index.LastIndexedHeight < currentDaemonHeight() {
			results.Text = "  Gnomon still syncing, using available TELA data..."
			results.Color = colors.Yellow
			daemonHeight := currentDaemonHeight()
			progress := 0.2
			if daemonHeight > 0 {
				progress = math.Max(0.2, math.Min(0.9, float64(gnomon.Index.LastIndexedHeight)/float64(daemonHeight)))
			}
			setTelaStatus(fmt.Sprintf("Syncing Gnomon index... (%d / %d)", gnomon.Index.LastIndexedHeight, daemonHeight), colors.Yellow)
			setTelaProgress(progress)
			fyne.Do(func() {
				results.Refresh()
			})
		}

		indexCacheStore := loadTelaIndexCache()
		candidateCache := loadTelaCandidateCache()
		currentScanHeight := storedIndexedHeight
		var candidateCacheMu sync.RWMutex
		var indexMu sync.Mutex
		var scidsToIndex []string
		if gnomon.Index != nil {
			currentScanHeight = gnomon.Index.LastIndexedHeight
		}
		if !restrictiveMode {
			sAll = candidateCache.negativeSet()
			if len(sAll) == 0 {
				sAll = loadStringSetFromEncryptedStorage("TELA Search", "NegativeCache")
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
		hasDisplayCache := len(cachedDisplay) > 0
		if len(cachedDisplay) > 0 {
			for _, entry := range cachedDisplay {
				if !isDisplayableTelaApp(entry.INDEX) {
					continue
				}
				telaSearch = append(telaSearch, entry)
				telaSCIDs = append(telaSCIDs, entry.SCID)
				indexCacheStore[entry.SCID] = entry.INDEX
			}
		}

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
			if len(telaSCIDs) == 0 {
				json.Unmarshal(storedSCIDs, &telaSCIDs)
			}

			results.Text = "  Scanning..."
			results.Color = colors.Yellow

			fyne.Do(func() {
				results.Refresh()
			})

			// Batch-fetch INDEX data for cached SCIDs missing from indexCacheStore
			// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket
			var cacheMissed []string
			for _, sc := range telaSCIDs {
				if _, ok := indexCacheStore[sc]; !ok {
					cacheMissed = append(cacheMissed, sc)
				}
			}
			if len(cacheMissed) > 0 {
				results.Text = fmt.Sprintf("  Fetching INDEX data... (%d SCIDs)", len(cacheMissed))
				results.Color = colors.Yellow
				fyne.Do(func() {
					results.Refresh()
				})

				fetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, cacheMissed, 50)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for cached SCIDs: %v\n", fetchErr)
				}
				for scid, index := range fetched {
					indexCacheStore[scid] = index
					setCandidateCache(scid, telaCandidateValidIndex)
					setNegativeSCID(scid, false)
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(cacheMissed)))
			}

			if !hasDisplayCache {
				cachedAdded := int64(0)
				var cachedMu sync.Mutex
				cachedWorkers := workerPoolSize / 2
				if cachedWorkers < 8 {
					cachedWorkers = 8
				}
				if cachedWorkers > 24 {
					cachedWorkers = 24
				}
				cachedSlots := make(chan struct{}, cachedWorkers)
				var cachedWg sync.WaitGroup

				for i, sc := range telaSCIDs {
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
								results.Text = fmt.Sprintf("  Adding... (%d / %d)", idx+1, len(telaSCIDs))
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

						_, ratings, err := getLikesRatio(scid, index.DURL, searchExclusions, minLikes)
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

						cachedMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						cachedMu.Unlock()
					}(i, sc)
				}

				cachedWg.Wait()
			}
			storedSCIDsCount = len(telaSCIDs)

			if !allowTelaIndexMutations {
				cacheHitMode = "cached_syncing"
				fullScanReason = ""
				if len(telaSearch) == 0 && len(telaSCIDs) > 0 {
					for _, scid := range telaSCIDs {
						if index, ok := indexCacheStore[scid]; ok {
							if !isDisplayableTelaApp(index) {
								continue
							}
							telaSearch = append(telaSearch, INDEXwithRatings{INDEX: index})
						}
					}
				}
				fyne.Do(func() {
					searching = telaSearchDisplayAll(telaSearch, sortBy)
					searchData.Set(searching)
					searchList.Refresh()
					results.Text = fmt.Sprintf("  TELA cache loaded while Gnomon syncs: %d", len(telaSearch))
					results.Color = colors.Yellow
					entrySearch.Enable()
					entryAddSCID.Enable()
				})

				if last, err := GetEncryptedValue("TELA Search", []byte("Last Scan")); err == nil {
					lastScan = string(last)
					labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
					labelLastScan.Color = colors.Yellow
				}

				fyne.Do(func() {
					results.Refresh()
					labelLastScan.Refresh()
				})

				logger.Printf("[TELA] Deferring full scan until Gnomon catches up; showing cached results only\n")
				return
			}

			if !rescanRecheck && (len(telaSearch) > 0 || len(telaSCIDs) > 0) && heightDelta == 0 {
				cacheHitMode = "cached_only"
				fullScanReason = ""
				fyne.Do(func() {
					searching = telaSearchDisplayAll(telaSearch, sortBy)
					searchData.Set(searching)
					searchList.Refresh()
					results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(telaSearch))
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
		if restrictiveMode {
			for _, sc := range telaSCIDs {
				all[sc] = ""
			}
		} else {
			// Guard against uninitialized backends
			if gnomon.Index == nil ||
				(gnomon.Index.DBType == "gravdb" && gnomon.Index.GravDBBackend == nil) ||
				(gnomon.Index.DBType == "boltdb" && gnomon.Index.BBSBackend == nil) {
				results.Text = "  Gnomon is initializing..."
				results.Color = colors.Yellow
				uiDo(func() {
					results.Refresh()
				})
				go func() {
					time.Sleep(2 * time.Second)
					if strings.Contains(session.Domain, ".tela") {
						uiDo(func() {
							getSearchResults()
						})
					}
				}()
				return
			}
			all = gnomon.GetAllOwnersAndSCIDs()
		}

		if !restrictiveMode && !hasCachedTelaData && len(all) <= 1 {
			results.Text = "  Gnomon is preparing SCIDs..."
			results.Color = colors.Yellow
			uiDo(func() {
				results.Refresh()
			})

			go func() {
				time.Sleep(2 * time.Second)
				if strings.Contains(session.Domain, ".tela") {
					go getSearchResults()
				}
			}()
			return
		}

		allSCIDs := make([]string, 0, len(all))
		for sc := range all {
			allSCIDs = append(allSCIDs, sc)
		}
		sort.Strings(allSCIDs)

		if !restrictiveMode && !rescanRecheck && heightDelta > 0 {
			candidateSet := map[string]bool{}
			// Use known cached SCIDs (telaSCIDs) instead of candidateCache.validSCIDs()
			// because telaSCIDs is the authoritative list of confirmed TELA apps
			for _, sc := range telaSCIDs {
				candidateSet[sc] = true
			}
			skippedNoHeights := 0
			for _, sc := range allSCIDs {
				heights := gnomon.GetSCIDInteractionHeight(sc)
				if len(heights) == 0 {
					// Skip - Gnomon may not have indexed this SCID's interactions yet.
					// SCIDs with missing interaction data will be caught in subsequent scans
					// once Gnomon finishes indexing. New TELA apps are infrequent, so
					// this trade-off is acceptable to avoid scanning ~50k false positives.
					// See TELA_DELTA_SCAN_ISSUE.md for details.
					skippedNoHeights++
					continue
				}
				for _, h := range heights {
					if h > storedIndexedHeight {
						candidateSet[sc] = true
						break
					}
				}
			}
			if skippedNoHeights > 0 {
				logger.Printf("[TELA] Delta scan: skipped %d SCIDs with no interaction heights (Gnomon may not have indexed them yet)\n", skippedNoHeights)
			}
			if len(candidateSet) > 0 {
				allSCIDs = make([]string, 0, len(candidateSet))
				for sc := range candidateSet {
					allSCIDs = append(allSCIDs, sc)
				}
				sort.Strings(allSCIDs)
			}
		}

		// Create set of known TELA SCIDs for O(1) lookup
		knownTelaSCIDs := make(map[string]bool, len(telaSCIDs))
		for _, sc := range telaSCIDs {
			knownTelaSCIDs[sc] = true
		}

		prefilterAllowed := map[string]bool{}
		if !restrictiveMode {
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

			results.Text = fmt.Sprintf("  Prefiltering... (%d candidates)", len(candidates))
			results.Color = colors.Yellow
			setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d total)", len(candidates)), colors.Yellow)
			setTelaProgress(0.92)
			uiDo(func() {
				results.Refresh()
			})

			prefilterStart := time.Now()
			poolSize := 3
			pool, poolCleanup, poolErr := dialRPCPool(session.Daemon, poolSize)
			if poolErr != nil {
				logger.Printf("[TELA] Failed to create RPC pool (%d connections): %v\n", poolSize, poolErr)
				// Fallback: use Gnomon's single connection
				if gnomon.Index != nil && gnomon.Index.RPC != nil && gnomon.Index.RPC.RPC != nil {
					pool = []*jrpc2.Client{gnomon.Index.RPC.RPC}
					poolCleanup = func() {} // Don't close Gnomon's connection
				}
			}

			var passed map[string]bool
			var batchStats batchPrefilterStats
			var batchErr error
			if len(pool) > 0 {
				passed, batchStats, batchErr = batchPrefilterTelaVersions(scanCtx, candidates, 500, 3, pool, func(completed, total int) {
					results.Text = fmt.Sprintf("  Prefiltering... (%d / %d)", completed, total)
					results.Color = colors.Yellow
					progress := 0.92
					if total > 0 {
						progress = 0.92 + (float64(completed)/float64(total))*0.06
					}
					setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d / %d)", completed, total), colors.Yellow)
					setTelaProgress(progress)
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
					results.Text = fmt.Sprintf("  TELA SCIDs:  %d (cached apps shown - network error)", len(telaSearch))
					results.Color = colors.Yellow
					uiDo(func() {
						searching = telaSearchDisplayAll(telaSearch, sortBy)
						searchData.Set(searching)
						searchList.Refresh()
						results.Refresh()
					})
					phaseFinalizeMs = 0
					logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), "cached_network_error", heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
					return
				}

				results.Text = "  TELA network error - no cached apps available"
				results.Color = colors.Red
				uiDo(func() {
					searchData.Set(nil)
					searchList.Refresh()
					results.Refresh()
				})
				phaseFinalizeMs = 0
				logger.Printf("[TELA] Search metrics: outcome=interrupted reason=prefilter_network_error elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
				return
			} else {
				atomic.AddInt64(&retryCount, batchStats.Retries)
				atomic.AddInt64(&prefilterPassed, batchStats.VersionHits)
				atomic.AddInt64(&versionHits, batchStats.VersionHits)
				atomic.AddInt64(&prefilterDropped, batchStats.Dropped)

				// Only mark SCIDs as negative if prefilter had meaningful results.
				// If prefilter passed 0 candidates, it might be due to network issues or
				// daemon problems, not because all SCIDs are invalid TELA apps.
				// Marking all as negative would prevent valid TELA apps from being found later.
				prefilterSuccessRate := float64(len(passed)) / float64(len(candidates)+1)
				shouldMarkNegative := len(passed) > 0 && prefilterSuccessRate > 0.01 // At least 1% passed

				if !shouldMarkNegative && len(candidates) > 0 {
					logger.Printf("[TELA] Prefilter passed 0/%d candidates - skipping negative cache update to allow retry\n", len(candidates))
				}

				for _, sc := range candidates {
					prefilterAllowed[sc] = passed[sc]
					if shouldMarkNegative && !passed[sc] {
						setCandidateCache(sc, telaCandidateNotTela)
						setNegativeSCID(sc, true)
					} else {
						setNegativeSCID(sc, false)
					}
				}
			}
		}

		// Batch-fetch INDEX data for prefilter-passed SCIDs not yet in indexCacheStore.
		// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket.
		indexFetchFailed := make(map[string]bool) // Track SCIDs whose INDEX fetch failed due to network errors
		networkErrorDuringFetch := false          // Track if there was a network error during batch fetch
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
				results.Text = fmt.Sprintf("  Fetching INDEX data... (%d SCIDs)", len(indexNeeded))
				results.Color = colors.Yellow
				uiDo(func() {
					results.Refresh()
				})

				fetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, indexNeeded, 50)
				logger.Printf("[TELA] Batch INDEX fetch done: fetched=%d err=%v\n", len(fetched), fetchErr)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for scan: %v\n", fetchErr)
					networkErrorDuringFetch = true
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
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(indexNeeded)))
			}
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
				results.Text = fmt.Sprintf("  Scanning... (%d / %d)", scanned, allLen)
				results.Color = colors.Yellow
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

						_, ratings, err := getLikesRatio(scid, index.DURL, searchExclusions, minLikes)
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

						scanMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						scanMu.Unlock()
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
			searching = telaSearchDisplayAll(telaSearch, sortBy)
			searchData.Set(searching)
			searchList.Refresh()
			if networkErrorDuringFetch {
				results.Text = fmt.Sprintf("  TELA SCIDs:  %d (some apps may be missing - network error during fetch)", len(telaSearch))
				results.Color = colors.Yellow
			} else {
				results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(telaSearch))
				results.Color = colors.Green
			}
			results.Refresh()
		})

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

		if err := saveTelaDisplayCache(telaDisplayCache(telaSearch)); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing display cache: entries=%d err=%v\n", len(telaSearch), err)
		}

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
		if !restrictiveMode && len(displayedSCIDs) > 0 {
			telaSCIDs = displayedSCIDs
		}

		logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
	}

	entrySearch.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		normalizedInput := normalizeSearch(s)

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
				go getSearchResults()
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
					if strings.Contains(normalizeSearch(split), normalizedInput) {
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
						if strings.Contains(normalizeSearch(split), normalizedInput) {
							queryResult = append(queryResult, ind)
							break
						}
					}
				}
			}

			searching = telaSearchDisplayAll(queryResult, sortBy)
			searchData.Set(searching)
			searchList.Refresh()

			results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(queryResult))
			results.Color = colors.Green
			results.Refresh()
			entrySearch.Enable()

			return
		}

		switch normalizeSearch(query[0]) {
		case "name":
			for _, ind := range telaSearch {
				_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
				if err != nil {
					continue
				}

				if strings.Contains(normalizeSearch(ind.NameHdr), normalizeSearch(query[1])) {
					queryResult = append(queryResult, ind)
				}
			}
		case "durl":
			for _, ind := range telaSearch {
				_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
				if err != nil {
					continue
				}

				if strings.Contains(normalizeSearch(ind.DURL), normalizeSearch(query[1])) {
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

		searching = telaSearchDisplayAll(queryResult, sortBy)
		searchData.Set(searching)
		searchList.Refresh()

		results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(queryResult))
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

			go getSearchResults()
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
		for _, serv := range tela.GetServerInfo() {
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
				results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(searching))
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
		if len(telaSearch) == 0 {
			return
		}

		updated := telaSearchDisplayAll(telaSearch, sortBy)
		fyne.Do(func() {
			searching = updated
			searchData.Set(searching)
			searchList.Refresh()
			if !isSearching && wSelect.Selected == "Search" {
				results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(searching))
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
			errorText,
			tabButtons,
			results,
			telaStatus,
			telaProgress,
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
	results.Text = "  Loading TELA dapps..."
	results.Color = colors.Yellow
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

				results.Text = "  Gnomon is syncing..."
				results.Color = colors.Yellow

				fyne.Do(func() {
					entryHistory.Disable()
					results.Refresh()
				})

				time.Sleep(time.Second)
			}

			results.Text = "  Loading previous search history..."
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

				if len(title) > 36 {
					title = title[0:36] + "..."
				}

				if desc == "" {
					desc = "N/A"
				}

				if len(desc) > 40 {
					desc = desc[0:40] + "..."
				}

				historyResults = append(historyResults, title+";;;"+desc+";;;;;;"+scid.String())
			}

			sort.Strings(historyResults)
			history = historyResults
			historyData.Set(history)

			results.Text = fmt.Sprintf("  Search History:  %d", len(historyResults))
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

		normalizedInput := normalizeSearch(s)

		var queryResult []string
		for _, data := range history {
			for _, split := range strings.Split(data, ";;;") {
				if strings.Contains(normalizeSearch(split), normalizedInput) {
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

		entrySearch.SetPlaceHolder("Search TELA")
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
			results.Text = "  Gnomon is inactive. Waiting..."
			results.Color = colors.Gray
			results.Refresh()

			telaStartupWaitMu.Lock()
			if telaStartupWaiting {
				telaStartupWaitMu.Unlock()
				// If another startup is in progress, check if gnomon is now ready
				if gnomon.Index != nil {
					// Gnomon became ready during another startup, proceed with search
					go getSearchResults()
				} else {
					searchList.Refresh()
				}
				return
			}
			telaStartupWaiting = true
			telaStartupWaitMu.Unlock()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[Engram] TELA startup wait panic recovered: %v\n", r)
					}
				}()
				generation := currentWalletGeneration()
				defer func() {
					telaStartupWaitMu.Lock()
					telaStartupWaiting = false
					telaStartupWaitMu.Unlock()
				}()
				for i := 0; i < 60; i++ {
					if !isWalletGenerationActive(generation) || globals.Exit_In_Progress {
						return
					}
					time.Sleep(time.Second)
					if !strings.Contains(session.Domain, ".tela") {
						return
					}
					if gnomon.Index != nil {
						uiDo(func() {
							if !isWalletGenerationActive(generation) {
								return
							}
							results.Text = "  Starting TELA scan..."
							results.Color = colors.Yellow
							results.Refresh()
						})
						if isWalletGenerationActive(generation) {
							go getSearchResults()
						}
						return
					}
				}
			}()
		} else if len(searching) > 0 {
			results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(searching))
			results.Color = colors.Green
			results.Refresh()
		} else if len(telaSearch) > 0 {
			searching = telaSearchDisplayAll(telaSearch, sortBy)
			_ = searchData.Set(searching)
			results.Text = fmt.Sprintf("  TELA SCIDs:  %d", len(searching))
			results.Color = colors.Green
			results.Refresh()
		} else {
			generation := currentWalletGeneration()
			if isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
				go getSearchResults()
			}
		}

		searchList.Refresh()
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
			entrySearch.Show()
			entrySearch.SetPlaceHolder("Search favorites...")
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
			if gnomon.Index == nil {
				results.Text = "  Gnomon is inactive."
				results.Color = colors.Gray
				results.Refresh()
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
		results.Text = "  Gnomon is inactive."
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

				if link, err := tela.ServeTELA(s, session.Daemon); err == nil {
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

					if gnomon.Index != nil {
						historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
						sort.Strings(historyResults)
						history = historyResults
						historyData.Set(history)

						results.Text = fmt.Sprintf("  Search History:  %d", len(historyResults))
						results.Color = colors.Green

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

						if gnomon.Index != nil {
							historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
							sort.Strings(historyResults)
							history = historyResults
							historyData.Set(history)
							fyne.Do(func() {
								historyList.Refresh()
							})

							results.Text = fmt.Sprintf("  Search History:  %d", len(historyResults))
							results.Color = colors.Green

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

	generation := currentWalletGeneration()
	if isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
		go getHistoryResults()
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
func layoutTELAManager(index tela.INDEX, callback func()) fyne.CanvasObject {
	session.Domain = "app.tela.manager"

	var cachedData *TELAFavoriteData
	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		cachedData, _ = GetTELAFavoriteData(walletAddress, index.SCID)
	}

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

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

	labelDURL := canvas.NewText("   DURL", colors.Gray)
	labelDURL.TextSize = scaleFont(14)
	labelDURL.Alignment = fyne.TextAlignLeading
	labelDURL.TextStyle = fyne.TextStyle{Bold: true}

	textDURL := widget.NewRichTextFromMarkdown(index.DURL)
	textDURL.Wrapping = fyne.TextWrapWord

	labelSCID := canvas.NewText("   SMART  CONTRACT  ID", colors.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignLeading
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	textSCID := widget.NewRichTextFromMarkdown(index.SCID)
	textSCID.Wrapping = fyne.TextWrapWord

	linkViewExplorer := widget.NewHyperlinkWithStyle("View in Explorer", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkViewExplorer.OnTapped = func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	}

	linkCopySCID := widget.NewHyperlinkWithStyle("Copy SCID", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopySCID.OnTapped = func() {
		a.Clipboard().SetContent(index.SCID)
	}

	labelAuthor := canvas.NewText("   SMART  CONTRACT  AUTHOR", colors.Gray)
	labelAuthor.TextSize = scaleFont(14)
	labelAuthor.Alignment = fyne.TextAlignLeading
	labelAuthor.TextStyle = fyne.TextStyle{Bold: true}

	author := index.Author
	if author == "anon" {
		author = "--"
	}
	textAuthor := widget.NewRichTextFromMarkdown(author)
	textAuthor.Wrapping = fyne.TextWrapWord

	linkMessageAuthor := widget.NewHyperlinkWithStyle("Message the Author", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkMessageAuthor.OnTapped = func() {
		if index.Author != "" {
			messages.Contact = index.Author
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	}

	linkCopyAuthor := widget.NewHyperlinkWithStyle("Copy Address", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopyAuthor.OnTapped = func() {
		a.Clipboard().SetContent(index.Author)
	}

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
		session.Window.SetContent(session.LastDomain)
		session.Domain = "app.tela"
		session.LastDomain = capture
		go callback()
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

	btnServer := widget.NewButton("Start Application", nil)

	if tela.HasServer(index.DURL) {
		textStatus.Text = "Running"
		textStatus.Color = colors.Green
		textStatus.Refresh()
		btnServer.Text = "Shutdown Application"
		btnServer.Refresh()
		linkOpenInBrowser.Show()
	}

	btnServer.OnTapped = func() {
		if btnServer.Text != "Start Application" {
			tela.ShutdownServer(index.DURL)
			errorText.Text = ""
			errorText.Refresh()
			textStatus.Text = "Offline"
			textStatus.Color = colors.Gray
			textStatus.Refresh()
			btnServer.Text = "Start Application"
			btnServer.Refresh()
			linkOpenInBrowser.Hide()
		} else {
			showLoadingOverlay()

			go func() {
				// Start the TELA server
				if link, err := tela.ServeTELA(index.SCID, session.Daemon); err == nil {
					// Server started successfully, get the URL
					url, err := url.Parse(link)
					if err != nil {
						logger.Errorf("[Engram] TELA URL parse: %s\n", err)
						errorText.Text = "error could parse URL"
						errorText.Color = colors.Red
						errorText.Refresh()
					} else {
						pushTELANavigation(index.SCID)

						// Save to TELA history
						if err := StoreEncryptedValue("TELA History", []byte(index.SCID), []byte("")); err != nil {
							logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
						}

						// Open the URL in browser
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
						} else {
							logger.Printf("[Engram] TELA Server: Opened in mobile browser %s\n", index.SCID)
						}
					}
				} else {
					// Check if this is an update conflict
					if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
						removeOverlays()

						generation := currentWalletGeneration()
						go func() {
							if !isWalletGenerationActive(generation) {
								return
							}

							// Create a TELALink to parse and get its ratings for user to verifiy before serving updated content
							telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", index.SCID)}
							linkPermission, err := AskPermissionForRequestE("Allow Updated Content", telaLink)
							if err != nil {
								logger.Errorf("[Engram] Open TELA link: %s\n", err)
								errorText.Text = "error could not open TELA"
								errorText.Color = colors.Red
								errorText.Refresh()

								return
							}

							if linkPermission != xswd.Allow {
								removeOverlays()
								return
							}

							// Serve the updated content
							link, err := serveTELAUpdates(index.SCID)
							if err != nil {
								logger.Errorf("[Engram] Error serving TELA: %s\n", err)
								errorText.Text = telaErrorToString(err)
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
								} else {
									logger.Printf("[Engram] TELA Server: Opened in mobile browser %s\n", index.SCID)
								}
							}

							fyne.Do(func() {
								textStatus.Text = "   Online"
								textStatus.Color = colors.Green
								textStatus.Refresh()
								btnServer.Text = "Shutdown Application"
								btnServer.Refresh()
								linkOpenInBrowser.Show()
							})

							err = StoreEncryptedValue("TELA History", []byte(index.SCID), []byte(""))
							if err != nil {
								logger.Errorf("[Engram] Error saving TELA search result: %s\n", err)
							}
						}()
					} else {
						// Other error occurred
						fyne.Do(func() {
							logger.Errorf("[Engram] Error serving TELA: %s\n", err)
							errorText.Text = telaErrorToString(err)
							errorText.Color = colors.Red
							errorText.Refresh()
						})
					}
				}

				// Always remove overlays when done
				uiDo(func() {
					removeOverlays()
				})
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

	hexagonImg := canvas.NewImageFromResource(telaHexagonColor(ratings.Average))
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
			hexagonImg.Resource = telaHexagonColor(ratings.Average)
			hexagonImg.Refresh()
		})

		if engram.Disk != nil && cachedData != nil {
			walletAddr := engram.Disk.GetAddress().String()
			AddTELAFavorite(walletAddr, index.SCID, cachedData.Name, cachedData.Description, cachedData.IconURL, freshRatings.Average)
		}
	}()

	linkTelaRatings := widget.NewHyperlinkWithStyle("View All Ratings", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkTelaRatings.OnTapped = func() {
		err := viewTELARatingsOverlay(index.NameHdr, index.SCID)
		if err != nil {
			errorText.Text = err.Error()
			errorText.Color = colors.Red
			errorText.Refresh()
		}
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
						wrapMobileButton(btnServer),
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
						container.NewHBox(
							linkMessageAuthor,
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkCopyAuthor,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelSCID,
						textSCID,
						container.NewHBox(
							linkViewExplorer,
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkCopySCID,
							layout.NewSpacer(),
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
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
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
			center,
		),
	)

	return NewVScroll(layout)
}
