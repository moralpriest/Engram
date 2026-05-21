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
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
)

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

	entryReg := widget.NewEntry()

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
