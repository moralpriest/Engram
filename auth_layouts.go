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
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/mnemonics"
	qrcode "github.com/skip2/go-qrcode"
)

func layoutMain() fyne.CanvasObject {
	// Set theme
	a.Settings().SetTheme(apptheme.Main)
	UpdateThemeLogo()
	session.Domain = "app.main"
	session.Path = ""
	session.Password = ""

	// Define objects

	btnLogin := widget.NewButtonWithIcon(i18n.T("main.connect"), theme.LoginIcon(), nil)

	if session.Error != "" {
		btnLogin.Text = session.Error
		btnLogin.Disable()
		btnLogin.Refresh()
		session.Error = ""
	}

	btnLogin.OnTapped = func() {
		if session.Path == "" {
			btnLogin.Text = i18n.T("main.no_account")
			btnLogin.Disable()
			btnLogin.Refresh()
		} else if session.Password == "" {
			btnLogin.Text = i18n.T("main.invalid_password")
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			if !session.Offline {
				btnLogin.Text = i18n.T("main.connect")
			} else {
				btnLogin.Text = i18n.T("main.decrypt")
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
					btnLogin.Text = i18n.T("main.no_account")
					btnLogin.Disable()
				} else if session.Password == "" {
					btnLogin.Text = i18n.T("main.invalid_password")
					btnLogin.Disable()
				} else {
					if !session.Offline {
						btnLogin.Text = i18n.T("main.connect")
					} else {
						btnLogin.Text = i18n.T("main.decrypt")
					}
					btnLogin.Enable()
					login()
					btnLogin.Text = i18n.T("main.invalid_password")
					btnLogin.Disable()
					session.Error = ""
				}
			}
		} else {
			return
		}
	})

	// New Account button with icon
	btnNewAccount := newBorderedButtonWithIcon(i18n.T("main.new_account"), theme.ContentAddIcon(), color.White, func() {
		session.Domain = "app.create"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutNewAccount())
		removeOverlays()
	}, ui.Width*0.9)

	// Recover Account button with icon
	btnRecoverAccount := newBorderedButtonWithIcon(i18n.T("main.recover_account"), theme.DocumentIcon(), color.White, func() {
		session.Domain = "app.restore"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutRestore())
		removeOverlays()
	}, ui.Width*0.9)

	// Connection Settings button with icon
	btnConnectionSettings := newGunmetalButtonWithIcon(i18n.T("main.connection_settings"), theme.SettingsIcon(), apptheme.C.Green, func() {
		session.Domain = "app.settings"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSettings())
		removeOverlays()
	}, ui.Width*0.9)

	modeData := binding.BindBool(&session.Offline)
	mode := widget.NewCheckWithData(i18n.T("main.offline_mode"), modeData)
	mode.OnChanged = func(b bool) {
		if b {
			session.Offline = true
			btnLogin.Text = i18n.T("main.decrypt")
			btnLogin.Refresh()
		} else {
			session.Offline = false
			btnLogin.Text = i18n.T("main.connect")
			btnLogin.Refresh()
		}
	}

	wPassword := NewReturnEntry()
	wPassword.OnReturn = btnLogin.OnTapped
	wPassword.Password = true

	loginDebouncer := NewDebouncer(200 * time.Millisecond)

	wPassword.OnChanged = func(s string) {
		session.Error = ""
		session.Password = s

		loginDebouncer.Debounce(func() {
			uiDo(func() {
				if !session.Offline {
					btnLogin.Text = i18n.T("main.connect")
				} else {
					btnLogin.Text = i18n.T("main.decrypt")
				}
				btnLogin.Enable()

				if len(session.Password) < 1 {
					btnLogin.Disable()
				} else if session.Path == "" {
					btnLogin.Disable()
				} else {
					btnLogin.Enable()
				}
			})
		})
	}
	wPassword.SetPlaceHolder(i18n.T("main.password"))

	// Get account databases in app directory
	list, err := GetAccounts()
	if err != nil {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAlert(2))
	}

	// Pulse animation for fresh install
	if len(list) == 0 {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if len(btnNewAccount.Objects) > 1 {
				if bg, ok := btnNewAccount.Objects[1].(*canvas.Rectangle); ok {
					done := make(chan struct{})
					pulseButton(bg, done)
					<-done
				}
			}

			if len(btnRecoverAccount.Objects) > 1 {
				if bg, ok := btnRecoverAccount.Objects[1].(*canvas.Rectangle); ok {
					pulseButton(bg, nil)
				}
			}
		}()
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
			btnLogin.Text = i18n.T("main.connect")
		} else {
			btnLogin.Text = i18n.T("main.decrypt")
		}
		btnLogin.Enable()
		safeCanvasFocus(wPassword)
		lastWalletKey := "last_wallet_" + session.Network
		StoreValue("settings", []byte(lastWalletKey), []byte(walletName))
	}

	unselectButtons := func() {
		for _, b := range walletBtns {
			b.SetColors(apptheme.C.DarkMatter, apptheme.C.Gray)
		}
	}

	// Wallet selection highlight color - theme aware
	var logoGreen color.Color
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		logoGreen = color.RGBA{200, 100, 255, 255} // bright purple for Derotopia
	case apptheme.ThemeElDorado:
		logoGreen = color.RGBA{255, 215, 0, 255} // gold for El Dorado
	case apptheme.ThemeCrystallina:
		logoGreen = color.RGBA{124, 92, 191, 255} // amethyst for Crystallina
	case apptheme.ThemeAtlantis:
		logoGreen = color.RGBA{52, 162, 181, 255} // cyan-teal for Atlantis
	default:
		logoGreen = color.RGBA{R: 70, G: 184, B: 104, A: 0xff} // original green for Engram Classic
	}

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

	// Theme header: transparent spacer (theme logo is overlaid in background stack)
	headerBlock := canvas.NewRectangle(color.Transparent)
	headerBlock.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))

	headerBox := canvas.NewRectangle(color.Transparent)
	headerBox.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	frame := &iframe{}

	newSpacer := func() *canvas.Rectangle {
		s := canvas.NewRectangle(color.Transparent)
		s.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))
		return s
	}

	loginSection := container.NewVBox(
		newSpacer(),
		wPassword,
		newSpacer(),
		mode,
		newSpacer(),
		btnLogin,
	)
	if len(list) == 0 {
		loginSection.Hide()
	}

	status.Connection.FillColor = apptheme.C.Gray
	status.RemoteAccess.FillColor = apptheme.C.Gray
	status.Gnomon.FillColor = apptheme.C.Gray
	status.EPOCH.FillColor = apptheme.C.Gray
	status.Sync.FillColor = apptheme.C.Gray

	// Create uniform button container - each button same size
	buttonGrid := container.NewVBox(
		container.New(layout.NewGridLayout(1),
			btnNewAccount,
		),
		newSpacer(),
		container.New(layout.NewGridLayout(1),
			btnRecoverAccount,
		),
		newSpacer(),
		container.New(layout.NewGridLayout(1),
			btnConnectionSettings,
		),
	)

	// Footer text
	copyrightLabel := canvas.NewText(i18n.T("auth.copyright"), apptheme.C.Gray)
	copyrightLabel.TextSize = scaleFont(10)
	copyrightLabel.Alignment = fyne.TextAlignCenter

	versionLabel := canvas.NewText(fmt.Sprintf("Engram v%s", versionString), apptheme.C.Gray)
	versionLabel.TextSize = scaleFont(10)
	versionLabel.Alignment = fyne.TextAlignCenter

	footer := container.NewVBox(
		copyrightLabel,
		versionLabel,
	)

	headerSpacer := container.NewStack(headerBlock)

	form := container.NewVBox(
		wSpacer,
		headerSpacer,
		newSpacer(),
		newSpacer(),
		walletButtons,
		loginSection,
		newSpacer(),
		buttonGrid,
		footer,
	)

	if isMobile() {
		btnLoginHeight := float32(48)
		btnLoginSizeEnforcer := canvas.NewRectangle(color.Transparent)
		btnLoginSizeEnforcer.SetMinSize(fyne.NewSize(ui.Width*0.9, scaleSize(btnLoginHeight)))
		wrappedBtnLogin := container.NewStack(btnLoginSizeEnforcer, btnLogin)

		mobileLoginSection := container.NewVBox(
			newSpacer(),
			wPassword,
			newSpacer(),
			mode,
			newSpacer(),
			wrappedBtnLogin,
		)
		if len(list) == 0 {
			mobileLoginSection.Hide()
		}

		form = container.NewVBox(
			wSpacer,
			headerSpacer,
			newSpacer(),
			newSpacer(),
			walletButtons,
			mobileLoginSection,
			newSpacer(),
			container.New(layout.NewGridLayout(1),
				btnNewAccount,
			),
			newSpacer(),
			container.New(layout.NewGridLayout(1),
				btnRecoverAccount,
			),
			newSpacer(),
			container.New(layout.NewGridLayout(1),
				btnConnectionSettings,
			),
			footer,
		)
	}

	var stackObjs []fyne.CanvasObject
	stackObjs = append(stackObjs, frame, res.mainBg)
	if apptheme.ThemeMode != apptheme.ThemeEngram && !isMobile() {
		res.gram.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))
		res.gram.FillMode = canvas.ImageFillContain
		logoOffset := scaleSize(30)
		coverPad := canvas.NewRectangle(color.Transparent)
		coverPad.SetMinSize(fyne.NewSize(1, logoOffset))
		coverRect := canvas.NewRectangle(apptheme.C.DarkMatter)
		coverRect.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.3))
		stackObjs = append(stackObjs, container.NewVBox(coverPad, coverRect, layout.NewSpacer()))
		logoPad := canvas.NewRectangle(color.Transparent)
		logoPad.SetMinSize(fyne.NewSize(1, logoOffset))
		stackObjs = append(stackObjs, container.NewVBox(logoPad, res.gram, layout.NewSpacer()))
	}
	stackObjs = append(stackObjs, container.NewCenter(form))
	layout := container.NewStack(stackObjs...)

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
	a.Settings().SetTheme(apptheme.Main)
	UpdateThemeLogo()
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
	lblWalletName := canvas.NewText(strings.TrimSuffix(walletName, ".db"), apptheme.C.Green)
	lblWalletName.TextSize = scaleFont(16)
	lblWalletName.Alignment = fyne.TextAlignCenter
	lblWalletName.TextStyle = fyne.TextStyle{Bold: true}

	// Password entry
	wPassword := NewReturnEntry()
	wPassword.Password = true
	wPassword.SetPlaceHolder(i18n.T("main.password"))

	// Login button
	btnLogin := widget.NewButtonWithIcon(i18n.T("main.connect"), theme.LoginIcon(), nil)
	btnLogin.Disable()

	if session.Error != "" {
		btnLogin.Text = session.Error
		btnLogin.Disable()
		btnLogin.Refresh()
		session.Error = ""
	}

	btnLogin.OnTapped = func() {
		if session.Password == "" {
			btnLogin.Text = i18n.T("main.invalid_password")
			btnLogin.Disable()
			btnLogin.Refresh()
		} else {
			if !session.Offline {
				btnLogin.Text = i18n.T("main.connect")
			} else {
				btnLogin.Text = i18n.T("main.decrypt")
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
			btnLogin.Text = i18n.T("main.connect")
		} else {
			btnLogin.Text = i18n.T("main.decrypt")
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
	mode := widget.NewCheckWithData(i18n.T("main.offline_mode"), modeData)
	mode.OnChanged = func(b bool) {
		if b {
			session.Offline = true
			btnLogin.Text = i18n.T("main.decrypt")
			btnLogin.Refresh()
		} else {
			session.Offline = false
			btnLogin.Text = i18n.T("main.connect")
			btnLogin.Refresh()
		}
	}

	// Switch Account button
	btnSwitchAccount := newGunmetalButtonWithIcon("Switch Account", theme.AccountIcon(), apptheme.C.Green, func() {
		session.Domain = "app.main"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	}, ui.Width*0.9)

	// Connection Settings button
	btnConnectionSettings := newGunmetalButtonWithIcon(i18n.T("main.connection_settings"), theme.SettingsIcon(), apptheme.C.Green, func() {
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
					btnLogin.Text = i18n.T("main.invalid_password")
					btnLogin.Disable()
					btnLogin.Refresh()
				} else {
					if !session.Offline {
						btnLogin.Text = i18n.T("main.connect")
					} else {
						btnLogin.Text = i18n.T("main.decrypt")
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

	// Theme header: transparent spacer (theme logo is overlaid in background stack)
	headerBlock := canvas.NewRectangle(color.Transparent)
	headerBlock.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))

	headerSpacer := container.NewStack(headerBlock)

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
			headerSpacer,
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
			headerSpacer,
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

	var stackObjs []fyne.CanvasObject
	stackObjs = append(stackObjs, frame, res.mainBg)
	if apptheme.ThemeMode != apptheme.ThemeEngram && !isMobile {
		res.gram.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.2))
		res.gram.FillMode = canvas.ImageFillContain
		logoOffset := scaleSize(30)
		coverPad := canvas.NewRectangle(color.Transparent)
		coverPad.SetMinSize(fyne.NewSize(1, logoOffset))
		coverRect := canvas.NewRectangle(apptheme.C.DarkMatter)
		coverRect.SetMinSize(fyne.NewSize(ui.Width, ui.MaxHeight*0.3))
		stackObjs = append(stackObjs, container.NewVBox(coverPad, coverRect, layout.NewSpacer()))
		logoPad := canvas.NewRectangle(color.Transparent)
		logoPad.SetMinSize(fyne.NewSize(1, logoOffset))
		stackObjs = append(stackObjs, container.NewVBox(logoPad, res.gram, layout.NewSpacer()))
	}
	stackObjs = append(stackObjs, container.NewCenter(form))
	layout := container.NewStack(stackObjs...)

	return layout
}

func layoutNewAccount() fyne.CanvasObject {
	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}
	a.Settings().SetTheme(apptheme.Alt)

	session.Domain = "app.register"
	session.Language = -1
	session.Error = ""
	session.Name = ""
	session.Password = ""
	session.PasswordConfirm = ""

	languages := mnemonics.Language_List()
	sort.Strings(languages)

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	btnCreate := widget.NewButton(i18n.T("create.create"), nil)
	btnCreate.Disable()

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Domain = "app.main"
		session.Error = ""
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	})

	btnCopySeed := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), nil)
	btnCopySeed.Importance = widget.LowImportance

	grid := container.NewVBox()

	wordsHidden := true
	var wordLabels []*widget.Label
	var formatted []string

	wordsToggleBtn := widget.NewButtonWithIcon("", theme.VisibilityIcon(), nil)
	wordsToggleBtn.Importance = widget.LowImportance
	wordsToggleBtn.OnTapped = func() {
		wordsHidden = !wordsHidden
		if wordsHidden {
			wordsToggleBtn.SetIcon(theme.VisibilityIcon())
			for _, lbl := range wordLabels {
				if lbl != nil {
					lbl.SetText("••••••••")
				}
			}
		} else {
			wordsToggleBtn.SetIcon(theme.VisibilityOffIcon())
			for i, lbl := range wordLabels {
				if lbl != nil && i < len(formatted) {
					lbl.SetText(strings.ReplaceAll(formatted[i], " ", ""))
				}
			}
		}
		grid.Refresh()
	}

	var addressStr string
	lblAddress := canvas.NewText("", apptheme.C.Green)
	lblAddress.TextSize = scaleFont(22)
	lblAddress.Alignment = fyne.TextAlignCenter
	lblAddress.TextStyle = fyne.TextStyle{Bold: true}

	var addressToggleBtn *widget.Button
	var addressCopyBtn *widget.Button

	addressHidden := true

	addressToggleBtn = widget.NewButtonWithIcon("", theme.VisibilityOffIcon(), func() {
		addressHidden = !addressHidden
		if addressHidden {
			lblAddress.Text = "dE...••••••••"
			addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
		} else {
			if addressStr != "" {
				lblAddress.Text = addressStr[0:5] + "..." + addressStr[len(addressStr)-10:]
			} else {
				lblAddress.Text = ""
			}
			addressToggleBtn.SetIcon(theme.VisibilityIcon())
		}
		lblAddress.Refresh()
	})
	addressToggleBtn.Importance = widget.LowImportance

	addressCopyBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		if addressStr != "" {
			a.Clipboard().SetContent(addressStr)
		}
	})
	addressCopyBtn.Importance = widget.LowImportance

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

	createDebouncer := NewDebouncer(200 * time.Millisecond)

	updateCreateBtn := func() {
		if len(session.Password) > 0 && session.Password == session.PasswordConfirm && !findAccount() && session.Language != -1 {
			btnCreate.Enable()
		} else {
			btnCreate.Disable()
		}
	}

	wPassword.OnChanged = func(s string) {
		session.Error = ""
		errorText.Text = ""
		errorText.Refresh()
		session.Password = s

		createDebouncer.Debounce(func() {
			uiDo(updateCreateBtn)
		})
	}
	wPassword.SetPlaceHolder(i18n.T("main.password"))
	wPassword.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wPasswordConfirm := widget.NewEntry()
	wPasswordConfirm.Password = true
	wPasswordConfirm.OnChanged = func(s string) {
		session.Error = ""
		errorText.Text = ""
		errorText.Refresh()
		session.PasswordConfirm = s

		createDebouncer.Debounce(func() {
			uiDo(updateCreateBtn)
		})
	}
	wPasswordConfirm.SetPlaceHolder(i18n.T("create.confirm_password"))
	wPasswordConfirm.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wAccount := widget.NewEntry()
	wAccount.SetPlaceHolder(i18n.T("create.account_name"))
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
			errorText.Color = apptheme.C.Red
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
	wLanguage.PlaceHolder = i18n.T("create.select_language")

	wSpacer := widget.NewLabel(" ")
	heading := canvas.NewText(i18n.T("main.new_account"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	heading2 := canvas.NewText(i18n.T("restore.recover"), apptheme.C.Green)
	heading2.TextSize = scaleFont(22)
	heading2.Alignment = fyne.TextAlignCenter
	heading2.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

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

	body := widget.NewLabel(i18n.T("create.save_words"))
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	btnEnter := widget.NewButtonWithIcon(i18n.T("create.enter"), theme.NavigateNextIcon(), func() {
		fyne.Do(func() {
			// Call login() to properly initialize wallet (network, daemon, gnomon, etc.)
			// login() checks if engram.Disk is nil and skips wallet opening if already open
			session.IsNewWallet = true
			login()
			// Password will be cleared by login() after successful initialization
		})
	})

	formSuccess := container.NewVBox(
		body,
		wSpacer,
		container.NewCenter(wordsToggleBtn),
		rectSpacer,
		container.NewCenter(grid),
		rectSpacer,
		container.NewCenter(btnCopySeed),
		rectSpacer,
		errorText,
		rectSpacer,
		wrapMobileButton(btnEnter),
		rectSpacer,
		container.NewCenter(
			container.NewHBox(
				lblAddress,
				addressToggleBtn,
				addressCopyBtn,
			),
		),
		rectSpacer,
	)

	formSuccess.Hide()

	keyboardSpacer := func() fyne.CanvasObject {
		if isMobile() {
			return NewSpacer(0, ui.Height*0.4)
		}
		return layout.NewSpacer()
	}()

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				form,
				formSuccess,
				keyboardSpacer,
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
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		} else {
			errorText.Text = ""
			errorText.Refresh()
		}

		showLoadingOverlayWithText(i18n.T("creating_wallet"), i18n.T("wallet_eta"))

		go func() {
			address, seed, err := create()

			fyne.Do(func() {
				removeOverlays()

				if err != nil {
					errorText.Text = session.Error
					errorText.Refresh()
					return
				}

				formatted = strings.Split(seed, " ")
				wordsHidden = true
				wordsToggleBtn.SetIcon(theme.VisibilityIcon())
				wordsToggleBtn.Refresh()

				rect := canvas.NewRectangle(apptheme.C.Flint)
				rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(25)))

				grid.Objects = nil
				wordLabels = make([]*widget.Label, len(formatted))

				for i := 0; i < len(formatted); i++ {
					pos := fmt.Sprintf("%d", i+1)
					wordLabels[i] = widget.NewLabelWithStyle("••••••••", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
					grid.Add(container.NewStack(
						rect,
						container.NewHBox(
							widget.NewLabel(" "),
							widget.NewLabelWithStyle(pos, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
							layout.NewSpacer(),
							wordLabels[i],
							widget.NewLabel(" "),
						),
					),
					)
				}

				btnCopySeed.OnTapped = func() {
					a.Clipboard().SetContent(seed)
				}

				addressStr = address
				if addressHidden {
					lblAddress.Text = "dE...••••••••"
					addressToggleBtn.SetIcon(theme.VisibilityOffIcon())
				} else {
					lblAddress.Text = addressStr[0:5] + "..." + addressStr[len(addressStr)-10:]
					addressToggleBtn.SetIcon(theme.VisibilityIcon())
				}
				lblAddress.Refresh()
				addressToggleBtn.Refresh()
				addressCopyBtn.Refresh()

				form.Hide()
				keyboardSpacer.Hide()
				form.Refresh()
				formSuccess.Show()
				formSuccess.Refresh()
				btnEnter.Refresh()
				grid.Refresh()
				scrollBox.Refresh()
				scrollBox.Offset = fyne.NewPos(0, 0)
				session.Window.Canvas().Content().Refresh()
				session.Window.Canvas().Refresh(session.Window.Content())
			})
		}()
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
	a.Settings().SetTheme(apptheme.Alt)

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

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	// Password strength indicator
	strengthText := canvas.NewText(" ", apptheme.C.Gray)
	strengthText.TextSize = scaleFont(11)
	strengthText.Alignment = fyne.TextAlignCenter

	btnCreate := widget.NewButton(i18n.T("restore.recover"), nil)
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

	btnCopyAddress := widget.NewButtonWithIcon(i18n.T("restore.copy_address"), theme.ContentCopyIcon(), nil)

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
	wPassword.SetPlaceHolder(i18n.T("main.password"))
	wPassword.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	wPasswordConfirm := NewMobileEntry()
	wPasswordConfirm.Password = true
	wPasswordConfirm.OnChanged = func(s string) {
		session.Error = ""
		clearFormText(errorText)
		session.PasswordConfirm = s

		updateRecoveryButtonState()
	}
	wPasswordConfirm.SetPlaceHolder(i18n.T("create.confirm_password"))
	wPasswordConfirm.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	// Card selection for recovery type
	selectedRecoveryType = 0 // 0=Words, 1=Hex, 2=Import

	// Card backgrounds for selection state
	cardBgWords := canvas.NewRectangle(apptheme.C.Green)
	cardBgHex := canvas.NewRectangle(apptheme.C.Gray)
	cardBgImport := canvas.NewRectangle(apptheme.C.Gray)

	cardBgWords.CornerRadius = scaleSize(12)
	cardBgHex.CornerRadius = scaleSize(12)
	cardBgImport.CornerRadius = scaleSize(12)

	// Card labels
	lblWords := canvas.NewText(i18n.T("restore.words"), apptheme.C.DarkMatter)
	lblWords.TextSize = scaleFont(14)
	lblWords.Alignment = fyne.TextAlignCenter
	lblWords.TextStyle = fyne.TextStyle{Bold: true}

	lblHex := canvas.NewText(i18n.T("restore.hex_key"), color.White)
	lblHex.TextSize = scaleFont(14)
	lblHex.Alignment = fyne.TextAlignCenter
	lblHex.TextStyle = fyne.TextStyle{Bold: true}

	lblImport := canvas.NewText(i18n.T("restore.import"), color.White)
	lblImport.TextSize = scaleFont(14)
	lblImport.Alignment = fyne.TextAlignCenter
	lblImport.TextStyle = fyne.TextStyle{Bold: true}

	// Card descriptions
	descWords := canvas.NewText(i18n.T("restore.25_words"), apptheme.C.DarkMatter)
	descWords.TextSize = scaleFont(11)
	descWords.Alignment = fyne.TextAlignCenter

	descHex := canvas.NewText(i18n.T("restore.64_chars"), color.White)
	descHex.TextSize = scaleFont(11)
	descHex.Alignment = fyne.TextAlignCenter

	descImport := canvas.NewText(i18n.T("restore.db_file"), color.White)
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
		// Start with theme-aware bright highlight color
		highlightColor := apptheme.PulseHighlightColor()
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
		cardBgWords.FillColor = apptheme.C.Gray
		cardBgHex.FillColor = apptheme.C.Gray
		cardBgImport.FillColor = apptheme.C.Gray
		lblWords.Color = color.White
		lblHex.Color = color.White
		lblImport.Color = color.White
		descWords.Color = color.White
		descHex.Color = color.White
		descImport.Color = color.White

		// Highlight selected with animation
		switch selectedRecoveryType {
		case 0:
			animateCardPulse(cardBgWords, apptheme.C.Green)
			lblWords.Color = apptheme.C.DarkMatter
			descWords.Color = apptheme.C.DarkMatter
		case 1:
			animateCardPulse(cardBgHex, apptheme.C.Green)
			lblHex.Color = apptheme.C.DarkMatter
			descHex.Color = apptheme.C.DarkMatter
		case 2:
			animateCardPulse(cardBgImport, apptheme.C.Green)
			lblImport.Color = apptheme.C.DarkMatter
			descImport.Color = apptheme.C.DarkMatter
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

	seedLanguages := mnemonics.Language_List()
	sort.Strings(seedLanguages)
	wLanguage := widget.NewSelect(seedLanguages, nil)
	wLanguage.OnChanged = func(s string) {
		index := wLanguage.SelectedIndex()
		session.Language = index
		safeCanvasFocus(wAccount)
		clearFormText(errorText)
	}
	wLanguage.PlaceHolder = i18n.T("create.select_language")
	wLanguage.Hide()

	wAccount.SetPlaceHolder(i18n.T("create.account_name"))
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

	heading := canvas.NewText(i18n.T("main.recover_account"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	// Network indicator
	networkName := strings.ToUpper(cachedNetwork)
	networkColor := apptheme.C.Green
	if cachedNetwork != NETWORK_MAINNET {
		networkColor = apptheme.C.Yellow
	}
	networkIndicator := canvas.NewText(networkName, networkColor)
	networkIndicator.TextSize = scaleFont(12)
	networkIndicator.Alignment = fyne.TextAlignCenter

	heading2 := canvas.NewText(i18n.T("restore.success"), apptheme.C.Green)
	heading2.TextSize = scaleFont(22)
	heading2.Alignment = fyne.TextAlignCenter
	heading2.TextStyle = fyne.TextStyle{Bold: true}

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())

	rectHeader := canvas.NewRectangle(color.Transparent)
	rectHeader.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(5)))

	status.Connection.FillColor = apptheme.C.Gray
	status.RemoteAccess.FillColor = apptheme.C.Gray
	status.Gnomon.FillColor = apptheme.C.Gray
	status.Sync.FillColor = apptheme.C.Gray

	grid := container.NewVBox()
	grid.Objects = nil

	seedEntry := NewMobileEntry()
	seedEntry.SetPlaceHolder(i18n.T("restore.seed_placeholder"))
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

	seedInfo := canvas.NewText(" ", apptheme.C.Gray)
	seedInfo.TextSize = scaleFont(11)
	seedInfo.Alignment = fyne.TextAlignCenter

	// Show word count as user types
	seedEntry.OnChanged = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			seedInfo.Text = " "
			seedInfo.Color = apptheme.C.Gray
		} else {
			wordCount := len(strings.Fields(s))
			seedInfo.Text = fmt.Sprintf("%d/25 words", wordCount)
			if wordCount == 24 || wordCount == 25 {
				seedInfo.Color = apptheme.C.Green
			} else if wordCount > 25 {
				seedInfo.Color = apptheme.C.Red
			} else {
				seedInfo.Color = apptheme.C.Gray
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
	hexEntry.SetPlaceHolder(i18n.T("restore.hex_placeholder"))
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

	importFileText := canvas.NewText(" ", apptheme.C.Green)
	importFileText.TextSize = scaleFont(12)
	importFileText.Alignment = fyne.TextAlignCenter

	// Button to open file picker for import - OnTapped set after formSuccess is defined
	btnSelectFile := widget.NewButtonWithIcon(i18n.T("restore.select_file"), theme.FolderOpenIcon(), nil)

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

	body := widget.NewLabel(i18n.T("restore.success_body"))
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

	btnEnter := widget.NewButtonWithIcon(i18n.T("create.enter"), theme.NavigateNextIcon(), func() {
		fyne.Do(func() {
			// Call login() to properly initialize wallet (network, daemon, gnomon, etc.)
			// login() checks if engram.Disk is nil and skips wallet opening if already open
			session.IsRecovery = true
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
		a.Settings().SetTheme(apptheme.Alt)
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

			passHeader := canvas.NewText(i18n.T("restore.enter_password"), apptheme.C.Gray)
			passHeader.TextSize = scaleFont(14)
			passHeader.Alignment = fyne.TextAlignCenter
			passHeader.TextStyle = fyne.TextStyle{Bold: true}

			passSubHeader := canvas.NewText(i18n.T("restore.unlock_wallet"), apptheme.C.Account)
			passSubHeader.TextSize = scaleFont(22)
			passSubHeader.Alignment = fyne.TextAlignCenter
			passSubHeader.TextStyle = fyne.TextStyle{Bold: true}

			btnUnlock := widget.NewButton(i18n.T("restore.unlock"), nil)
			btnUnlock.Disable()

			entryWalletPass := NewReturnEntry()
			entryWalletPass.Password = true
			entryWalletPass.PlaceHolder = "Password"
			entryWalletPass.OnFocusGained = func() {
				showVirtualKeyboard(entryWalletPass)
			}
			entryWalletPass.OnChanged = func(s string) {
				if s == "" {
					btnUnlock.Text = i18n.T("restore.unlock")
					btnUnlock.Disable()
					btnUnlock.Refresh()
				} else {
					btnUnlock.Text = i18n.T("restore.unlock")
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
				btnUnlock.Text = i18n.T("wallet.unlocking")
				btnUnlock.Refresh()

				enteredPassword := entryWalletPass.Text

				temp, err := walletapi.Open_Encrypted_Wallet(filePath, enteredPassword)
				if err != nil {
					logger.Errorf("[Engram] Cannot open imported wallet: %s\n", err)
					btnUnlock.Text = i18n.T("main.invalid_password")
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
					qr.BackgroundColor = apptheme.C.DarkMatter
					qr.ForegroundColor = apptheme.C.Green
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
					canvas.NewRectangle(apptheme.C.DarkMatter),
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

		if findAccount() {
			showFormError(errorText, "account name already exists")
			return
		}
		clearFormText(errorText)

		// Perform input checks before backgrounding to report errors immediately
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
			_, err := hex.DecodeString(hexEntry.Text)
			if err != nil {
				showFormError(errorText, "invalid hex characters")
				return
			}
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
			_, _, err := mnemonics.Words_To_Key(words)
			if err != nil {
				showFormError(errorText, err.Error())
				return
			}
		}

		showLoadingOverlayWithText(i18n.T("wallet.recovering"), i18n.T("wallet_eta"))

		go func() {
			var err error
			var language string
			var temp *walletapi.Wallet_Disk

			if selectedRecoveryType == 1 {
				hexKey, _ := hex.DecodeString(hexEntry.Text)
				temp, err = walletapi.Create_Encrypted_Wallet(session.Path, session.Password, new(crypto.BNRed).SetBytes(hexKey))
				language = "English"
			} else {
				words := strings.TrimSpace(seedEntry.Text)
				language, _, _ = mnemonics.Words_To_Key(words)
				temp, err = walletapi.Create_Encrypted_Wallet_From_Recovery_Words(session.Path, session.Password, words)
			}

			fyne.Do(func() {
				removeOverlays()

				if err != nil {
					showFormError(errorText, err.Error())
					return
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
				qr, errQr := qrcode.New(address, qrcode.Highest)
				var successImage image.Image
				var qrSize float32
				if errQr == nil {
					qrSize = ui.Width * 0.45
					if qrSize > ui.Height*0.3 {
						qrSize = ui.Height * 0.3
					}
					qr.BackgroundColor = apptheme.C.DarkMatter
					qr.ForegroundColor = apptheme.C.Green
					successImage = qr.Image(int(qrSize))
				}

				engram.Disk.Get_Balance_Rescan()
				if err := engram.Disk.Save_Wallet(); err != nil {
					logger.Errorf("[Register] Failed to save wallet after creation: %s\n", err)
				}

				// Wallet remains open for immediate transition via i18n.T("create.enter") button
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

				if errQr == nil {
					successQR.Image = successImage
					successQR.SetMinSize(fyne.NewSize(qrSize, qrSize))
					successQR.Refresh()
				}

				successAddress.SetText(address)

				btnCopyAddress.OnTapped = func() {
					a.Clipboard().SetContent(address)
				}

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
			})
		}()
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
