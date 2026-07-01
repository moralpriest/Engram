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
	"path/filepath"
	"strings"

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
	"github.com/deroproject/graviton"
)

func layoutDatapad() fyne.CanvasObject {
	session.Domain = "app.datapad"

	title := canvas.NewText(i18n.T("datapad.heading"), apptheme.C.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	entryNewPad := widget.NewEntry()
	entryNewPad.MultiLine = false
	entryNewPad.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	btnAdd := widget.NewButton(i18n.T("datapad.create"), nil)
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
			btnAdd.Text = i18n.T("datapad.err_create")
			btnAdd.Disable()
			btnAdd.Refresh()
		} else {
			session.Datapad = entryNewPad.Text
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDatapad())
			removeOverlays()
		}
	}

	entryNewPad.PlaceHolder = i18n.T("datapad.note_name")
	entryNewPad.SetIcon(theme.SearchIcon())
	entryNewPad.Validator = func(s string) error {
		session.Datapad = s
		if len(s) > 0 {
			_, err := GetEncryptedValue("Datapads", []byte(s))
			if err == nil {
				btnAdd.Text = i18n.T("datapad.err_exists")
				btnAdd.Disable()
				btnAdd.Refresh()
				err := errors.New("datapad already exists")
				entryNewPad.SetValidationError(err)
				return err
			} else {
				btnAdd.Text = i18n.T("datapad.create")
				btnAdd.Enable()
				btnAdd.Refresh()
				return nil
			}
		} else {
			btnAdd.Text = i18n.T("datapad.create")
			btnAdd.Disable()
			err := errors.New("datapad name required")
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

	sep := canvas.NewRectangle(apptheme.C.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(apptheme.C.Gray)
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
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

func createDatapadTabContent() fyne.CanvasObject {
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	entryNewPad := widget.NewEntry()
	entryNewPad.MultiLine = false
	entryNewPad.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	btnAdd := widget.NewButton(i18n.T("datapad.create"), nil)
	btnAdd.Disable()

	entryNewPad.PlaceHolder = i18n.T("datapad.note_name")
	entryNewPad.SetIcon(theme.SearchIcon())
	entryNewPad.Validator = func(s string) error {
		session.Datapad = s
		if len(s) > 0 {
			_, err := GetEncryptedValue("Datapads", []byte(s))
			if err == nil {
				btnAdd.Text = i18n.T("datapad.err_exists")
				btnAdd.Disable()
				btnAdd.Refresh()
				err := errors.New("datapad already exists")
				entryNewPad.SetValidationError(err)
				return err
			} else {
				btnAdd.Text = i18n.T("datapad.create")
				btnAdd.Enable()
				btnAdd.Refresh()
				return nil
			}
		} else {
			btnAdd.Text = i18n.T("datapad.create")
			btnAdd.Disable()
			err := errors.New("datapad name required")
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
				canvas.NewRectangle(apptheme.C.DarkMatter),
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

	btnAdd.OnTapped = func() {
		err := StoreEncryptedValue("Datapads", []byte(entryNewPad.Text), []byte(""))
		if err != nil {
			btnAdd.Text = i18n.T("datapad.err_create")
			btnAdd.Disable()
			btnAdd.Refresh()
		} else {
			padData = append(padData, entryNewPad.Text)
			padList.Set(padData)
			entryNewPad.SetText("")
			padBox.Refresh()
		}
	}

	top := container.NewVBox(
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

	features := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			padBox,
		),
		layout.NewSpacer(),
	)

	return container.NewBorder(
		top,
		nil,
		nil,
		nil,
		features,
	)
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

	heading := canvas.NewText(session.Datapad, apptheme.C.Gray)
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

	var handleAction func(action string)

	data, err := GetEncryptedValue("Datapads", []byte(session.Datapad))
	if err != nil {
		data = nil
	}

	overlay := session.Window.Canvas().Overlays()

	btnSave := widget.NewButton(i18n.T("datapad.save"), nil)

	entryPad := widget.NewEntry()
	entryPad.Wrapping = fyne.TextWrapWord

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	handleAction = func(action string) {
		errorText.Text = ""
		errorText.Refresh()

		if action == "clear" {
			header := canvas.NewText(i18n.T("datapad.clear_request"), apptheme.C.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText(i18n.T("datapad.clear_prompt"), apptheme.C.Account)
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

			btnSubmit := widget.NewButton(i18n.T("datapad.clear"), nil)

			btnSubmit.OnTapped = func() {
				if session.Datapad != "" {
					err := StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(""))
					if err != nil {
						logger.Errorf("[Datapad] Err: %s\n", err)
						return
					}

					entryPad.Text = ""
					entryPad.Refresh()
				}

				errorText.Text = i18n.T("datapad.status_cleared")
				errorText.Color = apptheme.C.Green
				errorText.Refresh()

				overlay := session.Window.Canvas().Overlays()
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
			}

			span := canvas.NewRectangle(color.Transparent)
			span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

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
		} else if action == "export" {
			dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not export datapad"
					errorText.Color = apptheme.C.Red
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
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				errorText.Text = "exported datapad successfully"
				errorText.Color = apptheme.C.Green
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
		} else if action == "import" {
			dialogFileImport := dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not import file"
					errorText.Color = apptheme.C.Red
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
					errorText.Color = apptheme.C.Red
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
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				if !isASCII(string(filedata)) {
					errorText.Text = "invalid file data"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				if entryPad.Text == "" {
					entryPad.SetText(string(filedata))
				} else {
					entryPad.SetText(fmt.Sprintf("%s\n\n%s", entryPad.Text, string(filedata)))
				}

				errorText.Text = "file data imported successfully"
				errorText.Color = apptheme.C.Green
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
		} else if action == "delete" {
			header := canvas.NewText(i18n.T("datapad.delete_request"), apptheme.C.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText(i18n.T("datapad.delete_prompt"), apptheme.C.Account)
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

			btnSubmit := widget.NewButton(i18n.T("datapad.delete"), nil)

			btnSubmit.OnTapped = func() {
				if session.Datapad != "" {
					err := DeleteKey("Datapads", []byte(session.Datapad))
					if err != nil {
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
					canvas.NewRectangle(apptheme.C.DarkMatter),
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
	}

	btnSave.OnTapped = func() {
		err = StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(entryPad.Text))
		if err != nil {
			btnSave.Disable()
			errorText.Text = "-  FAILED  -"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
		} else {
			session.DatapadChanged = false
			btnSave.Disable()
			heading.Text = session.Datapad
			heading.Refresh()
			errorText.Text = "-  SAVED  -"
			errorText.Color = apptheme.C.Green
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

	sep := canvas.NewRectangle(apptheme.C.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(apptheme.C.Gray)
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
			header := canvas.NewText(i18n.T("datapad.change_detected"), apptheme.C.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText(i18n.T("datapad.save_prompt"), apptheme.C.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkClose := widget.NewHyperlinkWithStyle(i18n.T("datapad.discard"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkClose.OnTapped = func() {
				session.Datapad = ""
				session.DatapadChanged = false
				removeOverlays()
			}

			btnSubmit := widget.NewButton(i18n.T("datapad.save"), nil)

			btnSubmit.OnTapped = func() {
				err = StoreEncryptedValue("Datapads", []byte(session.Datapad), []byte(entryPad.Text))
				if err != nil {
					btnSave.Disable()
					errorText.Text = i18n.T("datapad.err_save")
					errorText.Color = apptheme.C.Red
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
					canvas.NewRectangle(apptheme.C.DarkMatter),
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
				newSizedIconButton(theme.ContentClearIcon(), func() { handleAction("clear") }, 48),
				newSizedIconButton(theme.DocumentSaveIcon(), func() { handleAction("export") }, 48),
				newSizedIconButton(theme.FolderOpenIcon(), func() { handleAction("import") }, 48),
				newSizedIconButton(theme.DeleteIcon(), func() { handleAction("delete") }, 48),
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
