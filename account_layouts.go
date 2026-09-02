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
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/rpcserver"
	"github.com/deroproject/derohe/walletapi/xswd"
	qrcode "github.com/skip2/go-qrcode"
)

func layoutAccount() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, 10))
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))
	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	title := canvas.NewText(i18n.T("account.heading"), apptheme.C.Gray)
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
	heading := canvas.NewText("", apptheme.C.Green)
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

	labelPassword := canvas.NewText(i18n.T("account.new_password"), apptheme.C.Gray)
	labelPassword.TextStyle = fyne.TextStyle{Bold: true}
	labelPassword.TextSize = scaleFont(11)
	labelPassword.Alignment = fyne.TextAlignCenter

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	linkIdentity := widget.NewHyperlinkWithStyle(i18n.T("account.identity"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkIdentity.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutIdentity())
		removeOverlays()
	}

	linkServiceAddress := widget.NewHyperlinkWithStyle(i18n.T("account.payment_request"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkServiceAddress.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutServiceAddress())
		removeOverlays()
	}

	btnIdentity := newSmallIconButton(i18n.T("account.identity_btn"), theme.AccountIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutIdentity())
		removeOverlays()
	})

	btnPayment := newSmallIconButton(i18n.T("account.payment_btn"), theme.ComputerIcon(), func() {
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

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	// Recovery Words Link
	linkRecoveryWords := widget.NewHyperlinkWithStyle(i18n.T("account.recovery_words"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRecoveryWords.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()

		header := canvas.NewText(i18n.T("account.verification"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("account.confirm_password"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnConfirm := widget.NewButton(i18n.T("account.submit"), nil)
		btnConfirm.Disable()

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = i18n.T("account.password")
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			} else {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Enable()
				btnConfirm.Refresh()
			}
		}

		btnConfirm.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				addFullscreenOverlay(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(apptheme.C.DarkMatter),
					),
				)
				addFullscreenOverlay(layoutRecovery())
			} else {
				btnConfirm.Text = i18n.T("account.invalid_password")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			}
		}

		entryPassword.OnReturn = btnConfirm.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		addFullscreenOverlay(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

		addFullscreenOverlay(
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
	linkRecoveryHex := widget.NewHyperlinkWithStyle(i18n.T("account.recovery_hex"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRecoveryHex.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()

		header := canvas.NewText(i18n.T("account.verification"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("account.confirm_password"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnConfirm := widget.NewButton(i18n.T("account.submit"), nil)
		btnConfirm.Disable()

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = i18n.T("account.password")
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			} else {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Enable()
				btnConfirm.Refresh()
			}
		}

		btnConfirm.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				addFullscreenOverlay(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(apptheme.C.DarkMatter),
					),
				)
				addFullscreenOverlay(layoutRecoveryHex())
			} else {
				btnConfirm.Text = i18n.T("account.invalid_password")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			}
		}

		entryPassword.OnReturn = btnConfirm.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		addFullscreenOverlay(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

		addFullscreenOverlay(
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

	// Rename Wallet Link
	linkRenameWallet := widget.NewHyperlinkWithStyle(i18n.T("account.rename_wallet"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRenameWallet.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()

		header := canvas.NewText(i18n.T("account.verification"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("account.confirm_password"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnConfirm := widget.NewButton(i18n.T("account.submit"), nil)
		btnConfirm.Disable()

		entryPassword := NewReturnEntry()
		entryPassword.Password = true
		entryPassword.PlaceHolder = i18n.T("account.password")
		entryPassword.OnChanged = func(s string) {
			if s == "" {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			} else {
				btnConfirm.Text = i18n.T("account.submit")
				btnConfirm.Enable()
				btnConfirm.Refresh()
			}
		}

		btnConfirm.OnTapped = func() {
			if engram.Disk.Check_Password(entryPassword.Text) {
				addFullscreenOverlay(
					container.NewStack(
						&iframe{},
						canvas.NewRectangle(apptheme.C.DarkMatter),
					),
				)
				addFullscreenOverlay(renameWalletOverlay(entryPassword.Text, errorText))
			} else {
				btnConfirm.Text = i18n.T("account.invalid_password")
				btnConfirm.Disable()
				btnConfirm.Refresh()
			}
		}

		entryPassword.OnReturn = btnConfirm.OnTapped

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		addFullscreenOverlay(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

		addFullscreenOverlay(
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
	linkChangePassword := widget.NewHyperlinkWithStyle(i18n.T("account.change_password"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkChangePassword.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()

		header := canvas.NewText(i18n.T("account.authorization"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("account.change_password"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay := session.Window.Canvas().Overlays()
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		btnChange := widget.NewButton(i18n.T("account.submit"), nil)
		btnChange.Disable()

		curPass := widget.NewEntry()
		curPass.Password = true
		curPass.PlaceHolder = i18n.T("account.current_password")

		changeDebouncer := NewDebouncer(200 * time.Millisecond)
		updateChangeBtn := func() {
			btnChange.Text = i18n.T("account.submit")
			btnChange.Enable()
		}

		curPass.OnChanged = func(s string) {
			changeDebouncer.Debounce(func() {
				uiDo(updateChangeBtn)
			})
		}

		newPass := widget.NewEntry()
		newPass.Password = true
		newPass.PlaceHolder = i18n.T("account.new_password_ph")
		newPass.OnChanged = func(s string) {
			changeDebouncer.Debounce(func() {
				uiDo(updateChangeBtn)
			})
		}

		confirm := widget.NewEntry()
		confirm.Password = true
		confirm.PlaceHolder = i18n.T("account.confirm_ph")
		confirm.OnChanged = func(s string) {
			changeDebouncer.Debounce(func() {
				uiDo(updateChangeBtn)
			})
		}

		btnChange.OnTapped = func() {
			if engram.Disk.Check_Password(curPass.Text) {
				if newPass.Text == confirm.Text && newPass.Text != "" {
					err := engram.Disk.Set_Encrypted_Wallet_Password(newPass.Text)
					if err != nil {
						btnChange.Text = i18n.T("account.error_change")
						btnChange.Disable()
						btnChange.Refresh()
					} else {
						curPass.Text = ""
						curPass.Refresh()
						newPass.Text = ""
						newPass.Refresh()
						confirm.Text = ""
						confirm.Refresh()
						btnChange.Text = i18n.T("account.password_updated")
						btnChange.Disable()
						btnChange.Refresh()
						if err := engram.Disk.Save_Wallet(); err != nil {
							logger.Errorf("[Settings] Failed to save wallet after password change: %s\n", err)
						}
					}
				} else {
					btnChange.Text = i18n.T("account.passwords_no_match")
					btnChange.Disable()
					btnChange.Refresh()
				}
			} else {
				btnChange.Text = i18n.T("account.incorrect_password")
				btnChange.Disable()
				btnChange.Refresh()
			}
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		addFullscreenOverlay(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

		addFullscreenOverlay(
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
	linkExportWallet := widget.NewHyperlinkWithStyle(i18n.T("account.export_wallet"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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
									errorText.Text = i18n.T("account.export_error")
									errorText.Color = apptheme.C.Red
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
									errorText.Text = i18n.T("account.read_error")
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								return
							}

							_, err = writeToURI(data, uri)
							if err != nil {
								logger.Errorf("[Engram] Exporting %s: %s\n", session.Path, err)
								fyne.Do(func() {
									errorText.Text = i18n.T("account.export_error_saving")
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								return
							}

							fyne.Do(func() {
								errorText.Text = i18n.T("account.export_success")
								errorText.Color = apptheme.C.Green
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
							showDialogResized(dialogFileSave)
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
		qr.BackgroundColor = apptheme.C.DarkMatter
		qr.ForegroundColor = apptheme.C.Green
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
							linkRenameWallet,
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
	heading := canvas.NewText("Recovery Words", apptheme.C.Green)
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

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnCancel),
			),
			rectSpacer,
		),
	)

	body := widget.NewLabel(i18n.T("account.save_words"))
	body.Wrapping = fyne.TextWrapWord
	body.Alignment = fyne.TextAlignCenter
	body.TextStyle = fyne.TextStyle{Bold: true}

	btnCopySeed := widget.NewButton(i18n.T("account.copy_words"), nil)

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

	rect := canvas.NewRectangle(apptheme.C.Flint)
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
		container.NewBorder(
			header,
			footer,
			nil,
			nil,
			scrollBox,
		),
	)

	return layout
}

func layoutRecoveryHex() fyne.CanvasObject {
	wSpacer := widget.NewLabel(" ")
	heading := canvas.NewText("Recovery Hex Keys", apptheme.C.Green)
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

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnCancel),
			),
			rectSpacer,
		),
	)

	body := widget.NewLabel(i18n.T("account.save_hex"))
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

	linkCopySecret := widget.NewHyperlinkWithStyle(i18n.T("account.copy_secret"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	textPublic := widget.NewRichTextFromMarkdown(public)
	textPublic.Wrapping = fyne.TextWrapWord

	linkCopyPublic := widget.NewHyperlinkWithStyle(i18n.T("account.copy_public"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	labelSecret := canvas.NewText(i18n.T("account.secret_key"), apptheme.C.Gray)
	labelSecret.TextSize = scaleFont(14)
	labelSecret.Alignment = fyne.TextAlignLeading
	labelSecret.TextStyle = fyne.TextStyle{Bold: true}

	labelPublic := canvas.NewText(i18n.T("account.public_key"), apptheme.C.Gray)
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
		container.NewBorder(
			header,
			footer,
			nil,
			nil,
			scrollBox,
		),
	)

	return layout
}

var (
	errWalletNameInvalid = errors.New("invalid wallet name")
	errWalletNameExists  = errors.New("wallet name already exists")
)

// renameWalletOverlay shows the rename dialog: a name entry prefilled with the
// current wallet name plus confirm/cancel. It needs the wallet password (from
// the verification overlay) so the wallet can be reopened at its new path once
// the db file has been renamed.
func renameWalletOverlay(password string, errorText *canvas.Text) fyne.CanvasObject {
	header := canvas.NewText(i18n.T("account.authorization"), apptheme.C.Gray)
	header.TextSize = scaleFont(14)
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	subHeader := canvas.NewText(i18n.T("account.rename_wallet"), apptheme.C.Account)
	subHeader.TextSize = scaleFont(22)
	subHeader.Alignment = fyne.TextAlignCenter
	subHeader.TextStyle = fyne.TextStyle{Bold: true}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		overlay := session.Window.Canvas().Overlays()
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	currentName := strings.TrimSuffix(filepath.Base(session.Path), ".db")

	btnConfirm := widget.NewButton(i18n.T("account.submit"), nil)
	btnConfirm.Disable()

	entryName := NewReturnEntry()
	entryName.Text = currentName
	entryName.PlaceHolder = i18n.T("account.rename_placeholder")
	entryName.OnChanged = func(s string) {
		name := strings.TrimSuffix(strings.TrimSpace(s), ".db")
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) == currentName {
			btnConfirm.Text = i18n.T("account.submit")
			btnConfirm.Disable()
		} else {
			btnConfirm.Text = i18n.T("account.submit")
			btnConfirm.Enable()
		}
		btnConfirm.Refresh()
	}

	btnConfirm.OnTapped = func() {
		btnConfirm.Disable()
		btnConfirm.Text = i18n.T("account.submit")
		btnConfirm.Refresh()

		err := renameWalletFile(entryName.Text, password)
		if err != nil {
			switch {
			case errors.Is(err, errWalletNameInvalid):
				btnConfirm.Text = i18n.T("account.rename_invalid")
			case errors.Is(err, errWalletNameExists):
				btnConfirm.Text = i18n.T("account.rename_exists")
			default:
				btnConfirm.Text = i18n.T("account.rename_error")
			}
			btnConfirm.Enable()
			btnConfirm.Refresh()
			return
		}

		// Success: dismiss all overlays and report on the account page.
		overlay := session.Window.Canvas().Overlays()
		for overlay.Top() != nil {
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
		}
		errorText.Text = i18n.T("account.rename_success")
		errorText.Color = apptheme.C.Green
		errorText.Refresh()
	}

	entryName.OnReturn = btnConfirm.OnTapped

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

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
					entryName,
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

	return container.NewStack(
		&iframe{},
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)
}

// renameWalletFile renames the wallet db file on disk and reopens the wallet at
// its new path so the app keeps running with the same keys. The wallet is saved
// and closed first so the file can move safely, then reopened with the password
// captured from the verification overlay.
func renameWalletFile(newName, password string) error {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(newName), ".db"))

	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return errWalletNameInvalid
	}

	oldPath := session.Path
	newPath := filepath.Join(filepath.Dir(oldPath), name+".db")

	// Nothing to do when the name did not actually change.
	if filepath.Clean(newPath) == filepath.Clean(oldPath) {
		return nil
	}

	if _, err := os.Stat(newPath); err == nil {
		return errWalletNameExists
	}

	// Save and close the wallet so the db file can be renamed safely.
	if engram.Disk != nil {
		if err := engram.Disk.Save_Wallet(); err != nil {
			logger.Errorf("[Engram] Failed to save wallet before rename: %s\n", err)
		}
		engram.Disk.Close_Encrypted_Wallet()
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		logger.Errorf("[Engram] Renaming wallet %s to %s: %s\n", oldPath, newPath, err)
		restoreWalletSession(oldPath, password)
		return err
	}

	// Keep the backup copy in sync with the new name.
	if _, err := os.Stat(oldPath + ".bak"); err == nil {
		os.Remove(newPath + ".bak")
		if err := os.Rename(oldPath+".bak", newPath+".bak"); err != nil {
			logger.Warnf("[Engram] Could not rename wallet backup %s: %s\n", oldPath+".bak", err)
		}
	}

	// Reopen the wallet at its new path; roll the file back if that fails.
	temp, err := walletapi.Open_Encrypted_Wallet(newPath, password)
	if err != nil {
		_ = os.Rename(newPath, oldPath)
		logger.Errorf("[Engram] Reopening renamed wallet %s: %s\n", newPath, err)
		restoreWalletSession(oldPath, password)
		return err
	}

	engram.Disk = temp
	session.Path = newPath
	session.Name = name

	reapplyWalletSessionSettings()
	restartRemoteAccessServers()

	logger.Printf("[Engram] Wallet renamed: %s -> %s\n", oldPath, newPath)
	return nil
}

// restoreWalletSession reopens the wallet at the given path after a failed
// rename so the running session is not left with a closed wallet.
func restoreWalletSession(path, password string) {
	temp, err := walletapi.Open_Encrypted_Wallet(path, password)
	if err != nil {
		logger.Errorf("[Engram] Failed to restore wallet session at %s: %s\n", path, err)
		return
	}
	engram.Disk = temp
	reapplyWalletSessionSettings()
	restartRemoteAccessServers()
}

// reapplyWalletSessionSettings re-applies network, daemon, and ring-size state
// to a freshly opened wallet (mirrors the setup done in login()).
func reapplyWalletSessionSettings() {
	switch session.Network {
	case NETWORK_TESTNET:
		engram.Disk.SetNetwork(false)
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = false
	case NETWORK_SIMULATOR:
		engram.Disk.SetNetwork(true)
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = true
	default:
		engram.Disk.SetNetwork(true)
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
	}

	if !session.Offline {
		walletapi.SetDaemonAddress(session.Daemon)
		engram.Disk.SetDaemonAddress(session.Daemon)
		if session.TrackRecentBlocks > 0 {
			engram.Disk.SetTrackRecentBlocks(session.TrackRecentBlocks)
		}
	} else {
		engram.Disk.SetOfflineMode()
	}

	setRingSize(engram.Disk, 16)
	beginWalletSession()
}

// restartRemoteAccessServers re-binds the Remote Access RPC and XSWD servers to
// the reopened wallet after a rename, since both cache the wallet pointer.
func restartRemoteAccessServers() {
	// Remote Access RPC server.
	if remoteAccess.RPC.server != nil {
		remoteAccess.RPC.server.RPCServer_Stop()
		remoteAccess.RPC.server = nil
		port := remoteAccess.RPC.port
		if port == "" {
			port = getRemoteAccess("RPC")
		}
		if port != "" {
			globals.Arguments["--rpc-bind"] = port
			if remoteAccess.RPC.user == "" {
				remoteAccess.RPC.user = newRPCUsername()
			}
			if remoteAccess.RPC.pass == "" {
				remoteAccess.RPC.pass = newRPCPassword()
			}
			globals.Arguments["--rpc-login"] = remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass
			srv, err := rpcserver.RPCServer_Start(engram.Disk, "RemoteAccess")
			if err != nil {
				logger.Errorf("[Engram] Failed to restart RPC server after wallet rename: %s\n", err)
			} else {
				remoteAccess.RPC.server = srv
				logger.Printf("[Engram] RPC server restarted after wallet rename\n")
			}
		}
	}

	// XSWD (WebSocket) server.
	if remoteAccess.WS.server != nil {
		remoteAccess.WS.server.Stop()
		xswdStateMu.Lock()
		remoteAccess.WS.server = nil
		remoteAccess.WS.apps = []xswd.ApplicationData{}
		xswdStateMu.Unlock()
		port := remoteAccess.WS.port
		if port == "" {
			port = ":44326"
		}
		go toggleXSWD(port)
	}
}
