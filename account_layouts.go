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
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/tela/logger"
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
