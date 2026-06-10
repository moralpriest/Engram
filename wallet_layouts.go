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
	"errors"
	"fmt"
	"image/color"
	"log"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/DEROFDN/engram/i18n"
	"github.com/DEROFDN/engram/internal/camera"
	"github.com/civilware/epoch"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/xswd"
	qrcode "github.com/skip2/go-qrcode"
)

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

	topButtonWidth := (ui.Width*0.9 - scaleSize(10)) / 2

	gramSend := newLargeIconButton(i18n.T("dashboard.send"), theme.UploadIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutSend())
		removeOverlays()
	}, topButtonWidth)

	heading := canvas.NewText(i18n.T("dashboard.balance"), colors.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	sendDesc := canvas.NewText(i18n.T("dashboard.transfer_details"), colors.Gray)
	sendDesc.TextSize = scaleFont(18)
	sendDesc.Alignment = fyne.TextAlignCenter
	sendDesc.TextStyle = fyne.TextStyle{Bold: true}

	sendHeading := canvas.NewText(i18n.T("dashboard.send_money"), colors.Green)
	sendHeading.TextSize = scaleFont(22)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	headerLabel := canvas.NewText("  "+network+"  ", colors.Gray)
	headerLabel.TextSize = scaleFont(11)
	headerLabel.Alignment = fyne.TextAlignCenter
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}

	statusLabel := canvas.NewText(i18n.T("dashboard.status"), colors.Gray)
	statusLabel.TextSize = scaleFont(11)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	daemonLabel := canvas.NewText(i18n.T("dashboard.offline"), colors.Gray)
	daemonLabel.TextSize = scaleFont(12)
	daemonLabel.Alignment = fyne.TextAlignCenter
	daemonLabel.TextStyle = fyne.TextStyle{Bold: false}

	remoteAccessText := i18n.T("dashboard.remote_access")
	if remoteAccess.WS.server != nil {
		remoteAccessText = i18n.T("dashboard.remote_ws")
	} else if remoteAccess.RPC.server != nil {
		remoteAccessText = i18n.T("dashboard.remote_rpc")
	} else {
		status.RemoteAccess.FillColor = colors.Gray
		status.RemoteAccess.Refresh()
	}

	remoteAccessLabel := canvas.NewText(remoteAccessText, colors.Gray)
	remoteAccessLabel.TextSize = scaleFont(12)
	remoteAccessLabel.Alignment = fyne.TextAlignTrailing
	remoteAccessLabel.TextStyle = fyne.TextStyle{Bold: false}

	gnomonLabel := canvas.NewText(i18n.T("dashboard.gnomon"), colors.Gray)
	gnomonLabel.TextSize = scaleFont(12)
	gnomonLabel.Alignment = fyne.TextAlignCenter
	gnomonLabel.TextStyle = fyne.TextStyle{Bold: false}

	epochLabel := canvas.NewText(i18n.T("dashboard.epoch"), colors.Gray)
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

	telaLabel := canvas.NewText(i18n.T("dashboard.tela"), colors.Gray)
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

	session.DaemonLabel = daemonLabel

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

	sep3 := canvas.NewRectangle(colors.Gray)
	sep3.SetMinSize(fyne.NewSize(ui.Width*0.2, scaleSize(2)))
	statusLine1 := container.NewVBox(
		layout.NewSpacer(),
		sep3,
		layout.NewSpacer(),
	)
	sep4 := canvas.NewRectangle(colors.Gray)
	sep4.SetMinSize(fyne.NewSize(ui.Width*0.2, scaleSize(2)))
	statusLine2 := container.NewVBox(
		layout.NewSpacer(),
		sep4,
		layout.NewSpacer(),
	)

	buttonWidth := ui.Width * 0.9 / 3

	btnExit := newIconLabelButtonWithColor(i18n.T("dashboard.exit"), theme.LogoutIcon(), colors.SoftRed, color.Black, func() {
		if session.Navigating {
			return
		}
		session.Navigating = true
		defer func() { session.Navigating = false }()
		closeWallet()
	}, buttonWidth)

	btnSettings := newIconLabelButton(i18n.T("dashboard.settings"), theme.SettingsIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	}, buttonWidth)

	btnNotes := newIconLabelButton(i18n.T("dashboard.notes"), theme.DocumentIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDatapad())
		removeOverlays()
	}, buttonWidth)

	btnMessages := newIconLabelButton(i18n.T("dashboard.messages"), theme.MailComposeIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		removeOverlays()
		session.Window.SetContent(layoutMessages())
	}, buttonWidth)

	btnContracts := newIconLabelButton(i18n.T("dashboard.contracts"), theme.FolderIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutFilesAndContracts())
		removeOverlays()
	}, buttonWidth)

	linkHistory := newBorderedButtonWithIcon(i18n.T("dashboard.history"), theme.HistoryIcon(), color.White, func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutHistory())
		removeOverlays()
	}, topButtonWidth)

	linkMyAccount := newBorderedButtonWithIcon(i18n.T("dashboard.my_account"), theme.AccountIcon(), color.White, func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAccount())
		removeOverlays()
	}, topButtonWidth)

	btnReceive := newLargeIconButton(i18n.T("dashboard.receive"), theme.DownloadIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutReceive())
		removeOverlays()
	}, topButtonWidth)

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

	deroFormBody := container.NewVBox(
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
	)

	var statusSection *fyne.Container

	statusBtn := widget.NewButton("", func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutNetwork())
		removeOverlays()
	})
	statusBtn.Importance = widget.LowImportance

	rectHoverSpacer := canvas.NewRectangle(color.Transparent)
	rectHoverSpacer.SetMinSize(scalePoint(1, 2))

	statusContent := container.NewVBox(
		rectHoverSpacer,
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
	)

	statusContentStack := container.NewStack(statusContent, statusBtn)

	statusSection = container.NewVBox(
		container.NewHBox(
			statusLine1,
			layout.NewSpacer(),
			statusLabel,
			layout.NewSpacer(),
			statusLine2,
		),
		rectSpacer,
		statusContentStack,
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

	bottomRowWidth := ui.Width * 0.9

	btnTELAWeb := newTELAButton(func() {
		if session.Navigating {
			return
		}

		session.Navigating = true

		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[TELA-BUTTON] Panic: %v\n%s\n", r, debug.Stack())
				session.Navigating = false
				session.Domain = "app.wallet"

				fyne.Do(func() {
					if session.Window != nil {
						session.Window.SetContent(layoutDashboard())
					}
				})
			}
		}()

		defer func() {
			session.Navigating = false
		}()

		if gnomon.Index == nil {
			showLoadingOverlay()
			fyne.Do(func() {
				errLabel := canvas.NewText(i18n.T("wallet.gnomon_initializing"), colors.Yellow)
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

		if !isDaemonConnected() {
			showLoadingOverlay()
			fyne.Do(func() {
				errLabel := canvas.NewText(i18n.T("wallet.waiting_connection"), colors.Yellow)
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
				if isDaemonConnected() && gnomon.Index != nil {
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
			return
		}

		if !session.WalletOpen {
			return
		}

		if engram.Disk == nil {
			return
		}

		asked := hasAskedXSWD()
		alreadyEnabled := remoteAccess.WS.global.enabled
		if !asked && !alreadyEnabled {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			go func() {
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

		currentContent := session.Window.Content()
		session.LastDomain = currentContent

		session.Window.SetContent(layoutTransition())

		telaLayout := layoutTELA()

		if telaLayout == nil {
			session.Window.SetContent(layoutDashboard())
			return
		}

		session.Window.SetContent(telaLayout)

		removeOverlays()
	}, buttonWidth)

	buttonsSection := container.NewStack(
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

	prioritiseStatus := getPrioritiseStatus()

	var gridContent fyne.CanvasObject
	if prioritiseStatus {
		gridContent = container.NewVBox(deroFormBody, statusSection)
	} else {
		gridContent = container.NewVBox(deroFormBody, buttonsSection)
	}

	grid := container.NewCenter(gridContent)
	top := container.NewCenter(
		layout.NewSpacer(),
		grid,
		layout.NewSpacer(),
	)

	var bottom fyne.CanvasObject
	if prioritiseStatus {
		bottom = buttonsSection
	} else {
		widthRect := canvas.NewRectangle(color.Transparent)
		widthRect.SetMinSize(fyne.NewSize(ui.Width, 0))
		bottom = container.NewStack(
			container.NewVBox(
				container.NewCenter(
					container.NewStack(widthRect, statusSection),
				),
				rectSpacer,
			),
		)
	}

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

	btnSend := widget.NewButtonWithIcon(i18n.T("send.save"), theme.DocumentSaveIcon(), nil)
	btnSend.Disable()

	btnSendNow := widget.NewButtonWithIcon(i18n.T("send.send"), theme.UploadIcon(), nil)
	btnSendNow.Disable()

	wAmount := widget.NewEntry()
	wAmount.SetPlaceHolder(i18n.T("send.amount"))

	wMessage := widget.NewEntry()
	wMessage.SetValidationError(nil)
	wMessage.SetPlaceHolder(i18n.T("send.comment"))
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
	wPaymentID.SetPlaceHolder(i18n.T("send.payment_id"))

	options := []string{i18n.T("send.ring_2"), i18n.T("send.ring_4"), i18n.T("send.ring_8"), i18n.T("send.ring_16"), i18n.T("send.ring_32"), i18n.T("send.ring_64"), i18n.T("send.ring_128")}
	wRings := widget.NewSelect(options, nil)
	wRings.SetSelected(i18n.T("send.ring_16"))

	wReceiver := widget.NewEntry()
	wReceiver.SetPlaceHolder(i18n.T("send.receiver"))
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
					wRings.SetSelected(i18n.T("send.ring_16"))
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

	wRings.PlaceHolder = i18n.T("send.select_ring")
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
			wRings.SetSelected(i18n.T("send.ring_16"))
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

	btnTransfers := widget.NewButtonWithIcon(i18n.T("send.transfers"), theme.ListIcon(), nil)
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
				sendAmount := tx.Amount
				sendDest := tx.Address.String()
				err = addTransfer()
				if err == nil {
					btnSendNow.Disable()
					btnSendNow.Text = i18n.T("send.sending")
					btnSendNow.Refresh()
					go func() {
						showLoadingOverlay()
						txid, sendErr := sendTransfers()
						if sendErr != nil {
							log.Printf("[Send] Error: %v\n", sendErr)
							uiDo(func() {
								removeOverlays()
								btnSendNow.Text = i18n.T("send.send")
								btnSendNow.Enable()
								btnSendNow.Refresh()
								dialog.ShowError(sendErr, session.Window)
							})
							return
						}
						log.Printf("[Send] Transaction sent: %s\n", txid)
						uiDo(func() {
							removeOverlays()
							session.LastDomain = session.Window.Content()
							session.Window.SetContent(layoutTransition())
							session.Window.SetContent(layoutDashboard())
							removeOverlays()
						})
						if getNotificationsEnabled() {
							fyne.CurrentApp().SendNotification(fyne.NewNotification("Engram", i18n.T("notification.send_success")))
						}
						confirmSendAsync(txid, sendAmount, sendDest)
					}()
				}
			} else {
				wReceiver.SetValidationError(errors.New("invalid address"))
				wReceiver.Refresh()
			}
		}
	}

	sendHeading := canvas.NewText(i18n.T("send.heading"), colors.Gray)
	sendHeading.TextSize = scaleFont(16)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	optionalLabel := canvas.NewText(i18n.T("send.optional"), colors.Gray)
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
		wrapMobileButton(widget.NewButtonWithIcon(i18n.T("send.scan_qr"), theme.MediaPhotoIcon(), func() {
			s := camera.NewScanner(session.Window, func(code string) {
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

	heading := canvas.NewText(i18n.T("receive.heading"), colors.DarkGreen)
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

	btnCreate := widget.NewButton(i18n.T("payment.create"), nil)

	wPaymentID := widget.NewEntry()

	wReceiver := widget.NewEntry()
	wReceiver.Text = engram.Disk.GetAddress().String()
	wReceiver.Disable()

	tx.Address, _ = globals.ParseValidateAddress(engram.Disk.GetAddress().String())

	wReceiver.SetPlaceHolder(i18n.T("send.receiver"))
	wReceiver.SetValidationError(nil)

	wAmount := widget.NewEntry()
	wAmount.SetPlaceHolder(i18n.T("send.amount"))

	wMessage := widget.NewEntry()
	wMessage.SetPlaceHolder(i18n.T("send.comment"))
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
	wPaymentID.SetPlaceHolder(i18n.T("send.payment_id"))

	sendHeading := canvas.NewText(i18n.T("payment.heading"), colors.Gray)
	sendHeading.TextSize = scaleFont(16)
	sendHeading.Alignment = fyne.TextAlignCenter
	sendHeading.TextStyle = fyne.TextStyle{Bold: true}

	optionalLabel := canvas.NewText(i18n.T("payment.optional"), colors.Gray)
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
				header := canvas.NewText(i18n.T("payment.heading"), colors.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				subHeader := canvas.NewText(i18n.T("payment.created"), colors.Account)
				subHeader.TextSize = scaleFont(22)
				subHeader.Alignment = fyne.TextAlignCenter
				subHeader.TextStyle = fyne.TextStyle{Bold: true}

				labelAddress := canvas.NewText(i18n.T("payment.integrated_address"), colors.Gray)
				labelAddress.TextSize = scaleFont(12)
				labelAddress.Alignment = fyne.TextAlignCenter
				labelAddress.TextStyle = fyne.TextStyle{Bold: true}

				btnCopy := widget.NewButton(i18n.T("payment.copy"), nil)

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
					subHeader.Text = i18n.T("payment.error")
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

func layoutTransfers() fyne.CanvasObject {
	session.Domain = "app.transfers"

	sendHeading := canvas.NewText(i18n.T("transfers.heading"), colors.Gray)
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

	btnSend := widget.NewButtonWithIcon(i18n.T("transfers.send_transfers"), theme.UploadIcon(), nil)

	btnClear := widget.NewButtonWithIcon(i18n.T("transfers.clear"), theme.WindowCloseIcon(), func() {
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
		btnSend.Text = i18n.T("transfers.disabled_offline")
		btnSend.Disable()
	}

	btnSend.OnTapped = func() {
		overlay := session.Window.Canvas().Overlays()

		header := canvas.NewText(i18n.T("transfers.verification_heading"), colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("transfers.confirm_password"), colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkClose := widget.NewHyperlinkWithStyle(i18n.T("common.cancel"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkClose.OnTapped = func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}

		btnSubmit := widget.NewButton(i18n.T("transfers.submit"), nil)

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = i18n.T("transfers.password_placeholder")
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnSubmit.Text = i18n.T("transfers.submit")
				btnSubmit.Disable()
				btnSubmit.Refresh()
			} else {
				btnSubmit.Text = i18n.T("transfers.submit")
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
					btnSend.Text = i18n.T("transfers.setting_up")
					btnSend.Disable()
					btnSend.Refresh()
					txid, err := sendTransfers()
					if err != nil {
						btnSend.Text = i18n.T("transfers.send_transfers")
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
							btnSend.Text = i18n.T("transfers.confirming")
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
									btnSend.Text = i18n.T("transfers.successful")
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
									btnSend.Text = i18n.T("transfers.failed")
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
									btnSend.Text = fmt.Sprintf(i18n.T("transfers.confirming_progress"), walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
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
					btnSubmit.Text = i18n.T("transfers.invalid_password")
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

	heading := canvas.NewText(i18n.T("transfers_detail.heading"), colors.Gray)
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

	labelDestination := canvas.NewText(i18n.T("transfers_detail.receiver_address"), colors.Gray)
	labelDestination.TextSize = scaleFont(14)
	labelDestination.Alignment = fyne.TextAlignLeading
	labelDestination.TextStyle = fyne.TextStyle{Bold: true}

	labelAmount := canvas.NewText(i18n.T("transfers_detail.amount"), colors.Gray)
	labelAmount.TextSize = scaleFont(14)
	labelAmount.Alignment = fyne.TextAlignLeading
	labelAmount.TextStyle = fyne.TextStyle{Bold: true}

	labelService := canvas.NewText(i18n.T("transfers_detail.payment_request"), colors.Gray)
	labelService.TextSize = scaleFont(14)
	labelService.Alignment = fyne.TextAlignLeading
	labelService.TextStyle = fyne.TextStyle{Bold: true}

	labelDestPort := canvas.NewText(i18n.T("transfers_detail.dest_port"), colors.Gray)
	labelDestPort.TextSize = scaleFont(14)
	labelDestPort.TextStyle = fyne.TextStyle{Bold: true}

	labelSourcePort := canvas.NewText(i18n.T("transfers_detail.source_port"), colors.Gray)
	labelSourcePort.TextSize = scaleFont(14)
	labelSourcePort.TextStyle = fyne.TextStyle{Bold: true}

	labelFees := canvas.NewText(i18n.T("transfers_detail.fees"), colors.Gray)
	labelFees.TextSize = scaleFont(14)
	labelFees.TextStyle = fyne.TextStyle{Bold: true}

	labelPayload := canvas.NewText(i18n.T("transfers_detail.payload"), colors.Gray)
	labelPayload.TextSize = scaleFont(14)
	labelPayload.TextStyle = fyne.TextStyle{Bold: true}

	labelReply := canvas.NewText(i18n.T("transfers_detail.reply_address"), colors.Gray)
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
			valueType.ParseMarkdown(i18n.T("transfers_detail.service"))
		} else {
			valueDestination.ParseMarkdown(details.Destination)
			valueType.ParseMarkdown(i18n.T("transfers_detail.normal"))
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

	btnDelete := widget.NewButton(i18n.T("transfers_detail.cancel"), nil)
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

	header := canvas.NewText(i18n.T("history.title"), colors.Green)
	header.TextSize = scaleFont(22)
	header.TextStyle = fyne.TextStyle{Bold: true}

	details_header := canvas.NewText(i18n.T("history.detail_title"), colors.Green)
	details_header.TextSize = scaleFont(22)
	details_header.TextStyle = fyne.TextStyle{Bold: true}

	frame := &iframe{}
	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	heading := canvas.NewText(i18n.T("history.heading"), colors.Gray)
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
		cachedTransfers, historyNormalRows, historyCoinbaseRows, historyMessageRows = waitForHistoryRefreshAndSync()
	}

	// Function to load Normal transactions
	loadNormal := func() {
		view = i18n.T("history.normal")
		listBox.UnselectAll()
		results.Text = i18n.T("history.scanning")
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyNormalRows...)

			// If empty, poll cache for up to 30s waiting for pulse loop to populate it
			if len(data) == 0 {
				for i := 0; i < 30; i++ {
					time.Sleep(1 * time.Second)
					_, historyNormalRows, _, _, _, _, _, ok := getHistoryRowCache()
					if ok && len(historyNormalRows) > 0 {
						data = append([]string(nil), historyNormalRows...)
						break
					}
				}
			}

			results.Text = fmt.Sprintf(i18n.T("history.results"), len(data))

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
		view = i18n.T("history.coinbase")
		listBox.UnselectAll()
		results.Text = i18n.T("history.scanning")
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyCoinbaseRows...)

			// If empty, poll cache for up to 30s waiting for pulse loop to populate it
			if len(data) == 0 {
				for i := 0; i < 30; i++ {
					time.Sleep(1 * time.Second)
					_, _, historyCoinbaseRows, _, _, _, _, ok := getHistoryRowCache()
					if ok && len(historyCoinbaseRows) > 0 {
						data = append([]string(nil), historyCoinbaseRows...)
						break
					}
				}
			}

			results.Text = fmt.Sprintf(i18n.T("history.results"), len(data))

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
		view = i18n.T("history.messages")
		listBox.UnselectAll()
		results.Text = i18n.T("history.scanning")
		results.Refresh()
		data = nil
		_ = listData.Set(nil)

		go func() {
			ensureHistoryRows()
			data = append([]string(nil), historyMessageRows...)

			// If empty, poll cache for up to 30s waiting for pulse loop to populate it
			if len(data) == 0 {
				for i := 0; i < 30; i++ {
					time.Sleep(1 * time.Second)
					_, _, _, historyMessageRows, _, _, _, ok := getHistoryRowCache()
					if ok && len(historyMessageRows) > 0 {
						data = append([]string(nil), historyMessageRows...)
						break
					}
				}
			}

			results.Text = fmt.Sprintf(i18n.T("history.results"), len(data))

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
		if view == i18n.T("history.normal") {
			loadNormal()
		} else if view == i18n.T("history.coinbase") {
			loadCoinbase()
		} else if view == i18n.T("history.messages") {
			loadMessages()
		}
	}

	// Create tab content containers (needed for proper tab rendering)
	normalTabContent := container.NewVBox()
	coinbaseTabContent := container.NewVBox()
	messagesTabContent := container.NewVBox()

	// Create tabs
	tabs := container.NewAppTabs(
		container.NewTabItem(i18n.T("history.normal"), normalTabContent),
		container.NewTabItem(i18n.T("history.coinbase"), coinbaseTabContent),
		container.NewTabItem(i18n.T("history.messages"), messagesTabContent),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Handle tab changes
	tabs.OnChanged = func(tab *container.TabItem) {
		switch tab.Text {
		case i18n.T("history.normal"):
			loadNormal()
		case i18n.T("history.coinbase"):
			loadCoinbase()
		case i18n.T("history.messages"):
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

	heading := canvas.NewText(i18n.T("detail.heading"), colors.Gray)
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

	labelTXID := canvas.NewText(i18n.T("detail.txid"), colors.Gray)
	labelTXID.TextSize = scaleFont(14)
	labelTXID.Alignment = fyne.TextAlignLeading
	labelTXID.TextStyle = fyne.TextStyle{Bold: true}

	labelAmount := canvas.NewText(i18n.T("detail.amount"), colors.Gray)
	labelAmount.TextSize = scaleFont(14)
	labelAmount.Alignment = fyne.TextAlignLeading
	labelAmount.TextStyle = fyne.TextStyle{Bold: true}

	labelDirection := canvas.NewText(i18n.T("detail.direction"), colors.Gray)
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

	labelProof := canvas.NewText(i18n.T("detail.proof"), colors.Gray)
	labelProof.TextSize = scaleFont(14)
	labelProof.Alignment = fyne.TextAlignLeading
	labelProof.TextStyle = fyne.TextStyle{Bold: true}

	labelDestPort := canvas.NewText(i18n.T("detail.dest_port"), colors.Gray)
	labelDestPort.TextSize = scaleFont(14)
	labelDestPort.TextStyle = fyne.TextStyle{Bold: true}

	labelSourcePort := canvas.NewText(i18n.T("detail.source_port"), colors.Gray)
	labelSourcePort.TextSize = scaleFont(14)
	labelSourcePort.TextStyle = fyne.TextStyle{Bold: true}

	labelFees := canvas.NewText(i18n.T("detail.fees"), colors.Gray)
	labelFees.TextSize = scaleFont(14)
	labelFees.TextStyle = fyne.TextStyle{Bold: true}

	labelPayload := canvas.NewText(i18n.T("detail.payload"), colors.Gray)
	labelPayload.TextSize = scaleFont(14)
	labelPayload.TextStyle = fyne.TextStyle{Bold: true}

	labelHeight := canvas.NewText(i18n.T("detail.height"), colors.Gray)
	labelHeight.TextSize = scaleFont(14)
	labelHeight.TextStyle = fyne.TextStyle{Bold: true}

	labelReply := canvas.NewText(i18n.T("detail.reply_address"), colors.Gray)
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
		valueDirection.Text = i18n.T("detail.received")
		labelMember.Text = i18n.T("detail.source")
		valueMember.ParseMarkdown(i18n.T("detail.mining_reward"))
		valueAmount.Color = colors.Green
		amount := details.Amount
		valueAmount.Text = "  + " + globals.FormatMoney(amount)
	} else if details.Incoming {
		valueDirection.Text = i18n.T("detail.received")
		labelMember.Text = i18n.T("detail.sender")
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
		valueDirection.Text = i18n.T("detail.sent")
		labelMember.Text = i18n.T("detail.receiver_addr")
		valueMember.ParseMarkdown("" + details.Destination)

		if details.Amount == 0 {
			valueAmount.Color = colors.Account
			valueAmount.Text = "  0.00000"
		} else {
			valueAmount.Color = colors.Account
			valueAmount.Text = "  - " + globals.FormatMoney(details.Amount)
		}
	}

	labeliMember.Text = i18n.T("detail.integrated_addr")
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

	btnView := newSizedTextButton(i18n.T("detail.view_explorer"), func() {
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

	linkAddress := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_address"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkAddress.OnTapped = func() {
		a.Clipboard().SetContent(valueMember.String())
	}

	linkiAddress := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_address"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkiAddress.OnTapped = func() {
		a.Clipboard().SetContent(valueiMember.String())
	}

	linkReplyAddress := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_address"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkReplyAddress.OnTapped = func() {
		if replyAddress, ok := details.Payload_RPC.Value(rpc.RPC_REPLYBACK_ADDRESS, rpc.DataAddress).(rpc.Address); ok {
			a.Clipboard().SetContent(replyAddress.String())
		}
	}

	linkTXID := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_txid"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkTXID.OnTapped = func() {
		a.Clipboard().SetContent(txid)
	}

	linkProof := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_proof"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkProof.OnTapped = func() {
		a.Clipboard().SetContent(details.Proof)
	}

	linkPayload := widget.NewHyperlinkWithStyle(i18n.T("detail.copy_payload"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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
