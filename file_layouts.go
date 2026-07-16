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
	"sort"
	"strings"
	"unicode"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/graviton"
)

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

	heading := canvas.NewText(i18n.T("files.heading"), apptheme.C.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelResults := canvas.NewText("   "+i18n.T("files.results"), apptheme.C.Gray)
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

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	dialogBrowse := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			logger.Errorf("[Engram] Open file dialog: %s\n", err)
			errorText.Text = "could not open file"
			errorText.Color = apptheme.C.Red
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
							errorText.Color = apptheme.C.Red
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
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
						})

						return
					}

					_, err = writeToURI(engram.Disk.SignData(filedata), uri)
					if err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						fyne.Do(func() {
							errorText.Text = "could not write signed file"
							errorText.Color = apptheme.C.Red
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
						errorText.Color = apptheme.C.Green
						errorText.Refresh()

						signedResults = append(signedResults, outputFile)
						signedData.Set(signedResults)
						signedList.Refresh()

						signedLen := len(signedResults)
						labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), signedLen, signedLen)
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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", fileName, err)
				errorText.Text = "could not read file"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			// Trim off .signed from file because engram.Disk.CheckFileSignature() adds it back on anyways - https://github.com/deroproject/derohe/blob/main/walletapi/wallet.go#L709
			fileName = strings.TrimSuffix(fileName, ".signed")
			signer, _, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
				errorText.Text = "signature verification failed"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
			logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())

			errorText.Text = "verified file successfully"
			errorText.Color = apptheme.C.Green
			errorText.Refresh()

			verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())

			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), verifiedLen, verifiedLen)
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

			header := canvas.NewText(i18n.T("files.signature_detail"), apptheme.C.Gray)
			header.TextSize = scaleFont(16)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			labelStatus := canvas.NewText(i18n.T("files.verification_status"), apptheme.C.Gray)
			labelStatus.TextSize = scaleFont(12)
			labelStatus.TextStyle = fyne.TextStyle{Bold: true}
			labelStatus.Alignment = fyne.TextAlignCenter

			valueStatus := canvas.NewText(i18n.T("files.verified"), apptheme.C.Green)
			valueStatus.TextSize = scaleFont(22)
			valueStatus.TextStyle = fyne.TextStyle{Bold: true}
			valueStatus.Alignment = fyne.TextAlignCenter

			labelFilename := canvas.NewText(i18n.T("files.filename"), apptheme.C.Gray)
			labelFilename.TextSize = scaleFont(14)
			labelFilename.TextStyle = fyne.TextStyle{Bold: true}

			valueFilename := widget.NewRichTextFromMarkdown(filename)
			valueFilename.Wrapping = fyne.TextWrapBreak

			labelSigner := canvas.NewText(i18n.T("files.signer_address"), apptheme.C.Gray)
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

			linkBack := widget.NewHyperlinkWithStyle(i18n.T("files.hide_details"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			linkBack.OnTapped = func() {
				removeOverlays()
			}

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

	btnBrowse := widget.NewButton(i18n.T("files.browse_files"), nil)
	btnBrowse.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		if session.Domain == "app.sign" {
			dialogBrowse.SetFilter(nil)
			dialogBrowse.SetConfirmText(i18n.T("files.open"))
		} else {
			dialogBrowse.SetFilter(storage.NewExtensionFileFilter([]string{".signed"}))
			dialogBrowse.SetConfirmText(i18n.T("files.verify"))
		}

		dialogBrowse.Show()
	}

	labelAction := canvas.NewText(i18n.T("files.drag_drop"), apptheme.C.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

	entryAddress := widget.NewEntry()
	entryAddress.PlaceHolder = "Username or Address"

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
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				inputFileName := files[0].Name()

				dialogFileSign := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
					if err != nil {
						logger.Errorf("[Engram] File dialog: %s\n", err)
						uiDo(func() {
							errorText.Text = "could not open signed file"
							errorText.Color = apptheme.C.Red
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
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
						})
						return
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						uiDo(func() {
							errorText.Text = "could not read file"
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
						})
						return
					}

					_, err = writeToURI(engram.Disk.SignData(filedata), uri)
					if err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						uiDo(func() {
							errorText.Text = "could not write signed file"
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
						})
						return
					}

					// Mobile uses content access name on save dialog
					outputFile := inputFileName + ".signed"

					logger.Printf("[Engram] Successfully signed file: %s\n", outputFile)

					uiDo(func() {
						errorText.Text = "signed file successfully"
						errorText.Color = apptheme.C.Green
						errorText.Refresh()

						signedResults = append(signedResults, outputFile)
						_ = signedData.Set(signedResults)
						signedList.Refresh()

						signedLen := len(signedResults)
						labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), signedLen, signedLen)
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
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						continue
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not read file %d", i)
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						continue
					}

					outputfile := inputFileName + ".signed"

					if err := os.WriteFile(outputfile, engram.Disk.SignData(filedata), 0600); err != nil {
						logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("cannot sign file %d", i)
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
					} else {
						logger.Printf("[Engram] Successfully signed file: %s\n", outputfile)
						labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), count, len(files)+singedLen)
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
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
							return
						}

						filedata, err := readFromURI(uc)
						if err != nil {
							logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", fileName, err)
							errorText.Text = "cannot read file data"
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
							return
						}

						signer, _, err := engram.Disk.CheckSignature(filedata)
						if err != nil {
							logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
							errorText.Text = "signature verification failed"
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
							return
						}

						logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())

						errorText.Text = "verified file successfully"
						errorText.Color = apptheme.C.Green
						errorText.Refresh()

						verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())
						_ = verifiedData.Set(verifiedResults)
						verifiedList.Refresh()

						verifiedLen := len(verifiedResults)
						labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), verifiedLen, verifiedLen)
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
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						continue
					}

					filedata, err := readFromURI(uc)
					if err != nil {
						logger.Errorf("[Engram] Cannot read file data for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("could not read file %d", i)
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						continue
					}

					outputfile := strings.TrimSuffix(inputFileName, ".signed")

					if signer, message, err := engram.Disk.CheckSignature(filedata); err != nil {
						logger.Errorf("[Engram] Signature verification failed for %s: %s\n", inputFileName, err)
						errorText.Text = fmt.Sprintf("signature verification %d failed", i)
						errorText.Color = apptheme.C.Red
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

						labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), count, len(files)+verifiedLen)
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
			labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), signedLen, signedLen)
			labelResults.Refresh()
		} else {
			session.Domain = "app.verify"
			center.Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*fyne.Container).Objects[18].(*fyne.Container).Objects[1] = verifiedList
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()
			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), verifiedLen, verifiedLen)
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

	heading := canvas.NewText(i18n.T("files.contract_builder"), apptheme.C.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	errorText := canvas.NewText(promptText, apptheme.C.Red)
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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			if filepath.Ext(filename) != ".bas" {
				errorText.Text = "requires a .bas file"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", filename, err)
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

	btnBrowse := widget.NewButton(i18n.T("files.browse_files"), nil)
	btnBrowse.OnTapped = func() {
		dialogBrowse.Show()
	}

	btnEditor := widget.NewButton(i18n.T("files.open_editor"), nil)
	btnEditor.OnTapped = func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutContractEditor("", ""))
		session.LastDomain = capture
	}

	labelAction := canvas.NewText(i18n.T("files.drag_drop"), apptheme.C.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			} else {
				uri, err := storage.Reader(files[0])
				if err != nil {
					errorText.Text = "could not read dropped file"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				filename := files[0].Name()
				if filepath.Ext(filename) != ".bas" {
					errorText.Text = "requires a .bas file"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				filedata, err := readFromURI(uri)
				if err != nil {
					logger.Errorf("[Engram] Cannot read file data for %s: %s\n", filename, err)
					errorText.Text = "cannot read file data"
					errorText.Color = apptheme.C.Red
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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				session.Window.SetContent(layoutContractBuilder(errorText.Text))
				return
			}

			if code == "" {
				errorText.Text = "contract does not exists"
				errorText.Color = apptheme.C.Red
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
				errorText.Color = apptheme.C.Red
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
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.9, ui.MaxHeight*0.12))

	rectWidth100 := canvas.NewRectangle(color.Transparent)
	rectWidth100.SetMinSize(fyne.NewSize(ui.Width*0.99, 10))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.9, 10))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	header := canvas.NewText(i18n.T("files.heading"), apptheme.C.Gray)
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
	labelResults := canvas.NewText("   "+i18n.T("files.results"), apptheme.C.Gray)
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

	browseErrorText := canvas.NewText(" ", apptheme.C.Green)
	browseErrorText.TextSize = scaleFont(12)
	browseErrorText.Alignment = fyne.TextAlignCenter

	dialogBrowseFiles := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			logger.Errorf("[Engram] Open file dialog: %s\n", err)
			browseErrorText.Text = "could not open file"
			browseErrorText.Color = apptheme.C.Red
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
						browseErrorText.Color = apptheme.C.Red
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
						browseErrorText.Color = apptheme.C.Red
						browseErrorText.Refresh()
					})

					return
				}

				_, err = writeToURI(engram.Disk.SignData(filedata), uri)
				if err != nil {
					logger.Errorf("[Engram] Cannot sign %s: %s\n", inputFileName, err)
					uiDo(func() {
						browseErrorText.Text = "could not write signed file"
						browseErrorText.Color = apptheme.C.Red
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
					browseErrorText.Color = apptheme.C.Green
					browseErrorText.Refresh()

					signedResults = append(signedResults, outputFile)
					_ = signedData.Set(signedResults)
					signedList.Refresh()

					signedLen := len(signedResults)
					labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), signedLen, signedLen)
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

		detailHeader := canvas.NewText(i18n.T("files.signature_detail"), apptheme.C.Gray)
		detailHeader.TextSize = scaleFont(16)
		detailHeader.Alignment = fyne.TextAlignCenter
		detailHeader.TextStyle = fyne.TextStyle{Bold: true}

		labelStatus := canvas.NewText(i18n.T("files.verification_status"), apptheme.C.Gray)
		labelStatus.TextSize = scaleFont(12)
		labelStatus.TextStyle = fyne.TextStyle{Bold: true}
		labelStatus.Alignment = fyne.TextAlignCenter

		valueStatus := canvas.NewText(i18n.T("files.verified"), apptheme.C.Green)
		valueStatus.TextSize = scaleFont(22)
		valueStatus.TextStyle = fyne.TextStyle{Bold: true}
		valueStatus.Alignment = fyne.TextAlignCenter

		labelFilename := canvas.NewText(i18n.T("files.filename"), apptheme.C.Gray)
		labelFilename.TextSize = scaleFont(14)
		labelFilename.TextStyle = fyne.TextStyle{Bold: true}

		valueFilename := widget.NewRichTextFromMarkdown(filename)
		valueFilename.Wrapping = fyne.TextWrapBreak

		labelSigner := canvas.NewText(i18n.T("files.signer_address"), apptheme.C.Gray)
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

		linkHide := widget.NewHyperlinkWithStyle(i18n.T("files.hide_details"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkHide.OnTapped = func() {
			removeOverlays()
		}

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

	btnSignFile := widget.NewButton(i18n.T("files.sign_file"), nil)
	btnSignFile.OnTapped = func() {
		dialogBrowseFiles.Show()
	}

	btnVerifyFile := widget.NewButton(i18n.T("files.verify_signature"), nil)
	btnVerifyFile.OnTapped = func() {
		dialogVerify := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil {
				logger.Errorf("[Engram] Open file dialog: %s\n", err)
				browseErrorText.Text = "could not open file"
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}

			if uc == nil {
				return
			}

			fileName := uc.URI().Name()
			if !strings.HasSuffix(fileName, ".signed") {
				browseErrorText.Text = "verifying requires a .signed file"
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", fileName, err)
				browseErrorText.Text = "could not read file"
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}

			fileName = strings.TrimSuffix(fileName, ".signed")
			signer, _, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", fileName, err)
				browseErrorText.Text = "signature verification failed"
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}
			logger.Printf("[Engram] %s signed by: %s\n", fileName, signer.String())

			browseErrorText.Text = "verified file successfully"
			browseErrorText.Color = apptheme.C.Green
			browseErrorText.Refresh()

			verifiedResults = append(verifiedResults, fileName+";;;"+signer.String())
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), verifiedLen, verifiedLen)
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

	// ==================== NOTES TAB: Inline notes entry & list ====================
	notesEntry := widget.NewEntry()
	notesEntry.MultiLine = false
	notesEntry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	notesEntry.PlaceHolder = i18n.T("datapad.note_name")
	notesEntry.SetIcon(theme.SearchIcon())

	btnAddNote := widget.NewButton(i18n.T("datapad.create"), nil)
	btnAddNote.Disable()

	notesEntry.Validator = func(s string) error {
		session.Datapad = s
		if len(s) > 0 {
			_, err := GetEncryptedValue("Datapads", []byte(s))
			if err == nil {
				btnAddNote.Text = i18n.T("datapad.err_exists")
				btnAddNote.Disable()
				btnAddNote.Refresh()
				err2 := errors.New("datapad already exists")
				notesEntry.SetValidationError(err2)
				return err2
			} else {
				btnAddNote.Text = i18n.T("datapad.create")
				btnAddNote.Enable()
				btnAddNote.Refresh()
				return nil
			}
		} else {
			btnAddNote.Text = i18n.T("datapad.create")
			btnAddNote.Disable()
			err2 := errors.New("datapad name required")
			notesEntry.SetValidationError(err2)
			btnAddNote.Refresh()
			return err2
		}
	}
	notesEntry.OnChanged = func(s string) {
		notesEntry.Validate()
	}
	notesEntry.OnSubmitted = func(_ string) {
		if notesEntry.Validate() == nil {
			btnAddNote.OnTapped()
		}
	}

	var padData []string

	shard, err := GetShard()
	if err == nil {
		store, err2 := graviton.NewDiskStore(shard)
		if err2 == nil {
			ss, err3 := store.LoadSnapshot(0)
			if err3 == nil {
				tree, err4 := ss.GetTree("Datapads")
				if err4 == nil {
					cursor := tree.Cursor()
					for k, _, err5 := cursor.First(); err5 == nil; k, _, err5 = cursor.Next() {
						if string(k) != "" {
							padData = append(padData, string(k))
						}
					}
				}
			}
		}
	}

	padList := binding.BindStringList(&padData)

	padBox := widget.NewListWithData(padList,
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabel(""),
			)
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

	btnAddNote.OnTapped = func() {
		err := StoreEncryptedValue("Datapads", []byte(notesEntry.Text), []byte(""))
		if err != nil {
			btnAddNote.Text = i18n.T("datapad.err_create")
			btnAddNote.Disable()
			btnAddNote.Refresh()
		} else {
			padData = append(padData, notesEntry.Text)
			padList.Set(padData)
			notesEntry.SetText("")
			padBox.Refresh()
		}
	}

	// Build combined tab content: Notes entry/list + Browse sign/verify
	notesTop := container.NewVBox(
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				newWidth90Rect(),
				container.NewVBox(
					notesEntry,
					rectSpacer,
					wrapMobileButton(btnAddNote),
				),
			),
			layout.NewSpacer(),
		),
		rectSpacer,
	)

	// Ensure the notes list has visible height even with few items
	notesRect := canvas.NewRectangle(color.Transparent)
	notesRect.SetMinSize(fyne.NewSize(ui.Width*0.9, ui.MaxHeight*0.1))

	notesListContent := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			notesRect,
			padBox,
		),
		layout.NewSpacer(),
	)

	separator := widget.NewRichTextFromMarkdown("")
	separator.Wrapping = fyne.TextWrapOff
	separator.ParseMarkdown("---")

	// Merge notes and browse into one combined view
	combinedNotesBrowseContent := container.NewVBox(
		notesTop,
		notesListContent,
		rectSpacer,
		separator,
		rectSpacer,
		wrapMobileButton(btnSignFile),
		rectSpacer,
		wrapMobileButton(btnVerifyFile),
		rectSpacer,
		browseErrorText,
		rectSpacer,
		labelResults,
		rectSpacer,
		container.NewStack(
			rectBox,
			container.NewVBox(
				widget.NewLabel("   "+i18n.T("files.signed_files")),
				signedList,
				widget.NewLabel("   "+i18n.T("files.verified_files")),
				verifiedList,
			),
		),
	)

	// ==================== TAB 2: SMART CONTRACTS (Contract Builder) ====================
	contractErrorText := canvas.NewText("", apptheme.C.Red)
	contractErrorText.TextSize = scaleFont(12)
	contractErrorText.Alignment = fyne.TextAlignCenter

	dialogBrowseSC := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		contractErrorText.Text = ""
		if uc != nil {
			filename := uc.URI().Name()
			if uc.URI().MimeType() != "text/plain" {
				logger.Errorf("[Engram] Cannot open file %s in contract builder\n", filename)
				contractErrorText.Text = "cannot open file"
				contractErrorText.Color = apptheme.C.Red
				contractErrorText.Refresh()
				return
			}

			if filepath.Ext(filename) != ".bas" {
				contractErrorText.Text = "requires a .bas file"
				contractErrorText.Color = apptheme.C.Red
				contractErrorText.Refresh()
				return
			}

			filedata, err := readFromURI(uc)
			if err != nil {
				logger.Errorf("[Engram] Cannot read URI file data for %s: %s\n", filename, err)
				contractErrorText.Text = "cannot read file data"
				contractErrorText.Color = apptheme.C.Red
				contractErrorText.Refresh()
				return
			}

			if !isASCII(string(filedata)) {
				contractErrorText.Text = "invalid file data"
				contractErrorText.Color = apptheme.C.Red
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

	btnBrowseSC := widget.NewButton(i18n.T("files.browse_bas"), nil)
	btnBrowseSC.OnTapped = func() {
		dialogBrowseSC.Show()
	}

	btnEditor := widget.NewButton(i18n.T("files.open_editor"), nil)
	btnEditor.OnTapped = func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutContractEditor("", ""))
		session.LastDomain = capture
	}

	labelAction := canvas.NewText(i18n.T("files.drag_drop"), apptheme.C.Gray)
	labelAction.TextSize = scaleFont(12)
	labelAction.Alignment = fyne.TextAlignLeading
	labelAction.TextStyle = fyne.TextStyle{Bold: true}

	entryClone := widget.NewEntry()
	entryClone.SetPlaceHolder(i18n.T("files.clone_scid"))
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
				contractErrorText.Color = apptheme.C.Red
				contractErrorText.Refresh()
				session.Window.SetContent(layoutFilesAndContracts())
				return
			}

			if code == "" {
				contractErrorText.Text = "contract does not exist"
				contractErrorText.Color = apptheme.C.Red
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
				contractErrorText.Color = apptheme.C.Red
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

	assetTab := container.NewTabItem(i18n.T("files.tab_assets"), assetTabContent)
	tabs := container.NewAppTabs(
		assetTab,
		container.NewTabItem(i18n.T("files.tab_notes"), combinedNotesBrowseContent),
		container.NewTabItem(i18n.T("files.tab_scids"), contractsTabContent),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Restore previously selected tab (e.g. Datapad tab after delete)
	if session.FilesContractTab > 0 {
		tabs.SelectIndex(session.FilesContractTab)
		session.FilesContractTab = 0
	}

	// Handle drag & drop for both tabs
	session.Window.SetOnDropped(func(p fyne.Position, files []fyne.URI) {
		if session.Domain != "app.filescontracts" {
			return
		}

		if len(files) > 1 {
			browseErrorText.Text = "single file only"
			browseErrorText.Color = apptheme.C.Red
			browseErrorText.Refresh()
			return
		}

		uri, err := storage.Reader(files[0])
		if err != nil {
			browseErrorText.Text = "could not read dropped file"
			browseErrorText.Color = apptheme.C.Red
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
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}

			filename = strings.TrimSuffix(filename, ".signed")
			signer, _, err := engram.Disk.CheckSignature(filedata)
			if err != nil {
				logger.Errorf("[Engram] Signature verification failed for %s: %s\n", filename, err)
				browseErrorText.Text = "signature verification failed"
				browseErrorText.Color = apptheme.C.Red
				browseErrorText.Refresh()
				return
			}
			logger.Printf("[Engram] %s signed by: %s\n", filename, signer.String())

			browseErrorText.Text = "verified file successfully"
			browseErrorText.Color = apptheme.C.Green
			browseErrorText.Refresh()

			verifiedResults = append(verifiedResults, filename+";;;"+signer.String())
			verifiedData.Set(verifiedResults)
			verifiedList.Refresh()

			verifiedLen := len(verifiedResults)
			labelResults.Text = fmt.Sprintf("   "+i18n.T("files.results_count"), verifiedLen, verifiedLen)
			labelResults.Refresh()

			// Switch to Notes tab (index 1)
			tabs.SelectIndex(1)

		} else if ext == ".bas" {
			// Open contract editor
			filedata, err := readFromURI(uri)
			if err != nil {
				logger.Errorf("[Engram] Cannot read file data for %s: %s\n", filename, err)
				browseErrorText.Text = "cannot read file data"
				browseErrorText.Color = apptheme.C.Red
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
			browseErrorText.Color = apptheme.C.Red
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

	heading := canvas.NewText(i18n.T("files.contract_editor"), apptheme.C.Green)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelHeaders := canvas.NewText(i18n.T("files.headers"), apptheme.C.Gray)
	labelHeaders.TextSize = scaleFont(14)
	labelHeaders.Alignment = fyne.TextAlignLeading
	labelHeaders.TextStyle = fyne.TextStyle{Bold: true}

	labelCode := canvas.NewText(i18n.T("files.code"), apptheme.C.Gray)
	labelCode.TextSize = scaleFont(14)
	labelCode.Alignment = fyne.TextAlignLeading
	labelCode.TextStyle = fyne.TextStyle{Bold: true}

	labelCodeSize := canvas.NewText("(0.0KB) ", apptheme.C.Green)
	labelCodeSize.TextSize = scaleFont(12)
	labelCodeSize.Alignment = fyne.TextAlignTrailing

	errorText := canvas.NewText(" ", apptheme.C.Green)
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
			errorText.Color = apptheme.C.Red
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
			errorText.Color = apptheme.C.Red
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
			errorText.Color = apptheme.C.Red
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
			labelCodeSize.Color = apptheme.C.Red
			errorText.Text = "contract size is to large"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
		} else if size > 18.5 {
			labelCodeSize.Color = apptheme.C.Yellow
		} else {
			labelCodeSize.Color = apptheme.C.Green
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
				errorText.Color = apptheme.C.Green
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
						errorText.Color = apptheme.C.Green
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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			if strings.HasSuffix(entryCode.Text, "\n") {
				entryCode.SetText(entryCode.Text + "\n" + dvmFuncExample(increment))
			} else {
				entryCode.SetText(entryCode.Text + "\n\n" + dvmFuncExample(increment))
			}

			errorText.Text = "new function added"
			errorText.Color = apptheme.C.Green
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

			header := canvas.NewText(i18n.T("files.contract_editor"), apptheme.C.Gray)
			header.TextSize = scaleFont(14)
			header.Alignment = fyne.TextAlignCenter
			header.TextStyle = fyne.TextStyle{Bold: true}

			subHeader := canvas.NewText(i18n.T("files.import_function"), apptheme.C.Account)
			subHeader.TextSize = scaleFont(22)
			subHeader.Alignment = fyne.TextAlignCenter
			subHeader.TextStyle = fyne.TextStyle{Bold: true}

			linkCancel := widget.NewHyperlinkWithStyle(i18n.T("common.cancel"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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
					canvas.NewRectangle(apptheme.C.DarkMatter),
				),
			)

			paramsContainer := container.NewVBox(entrySCID, entryEntrypoint)

			btnImport := widget.NewButton(i18n.T("common.import"), nil)
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
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						return
					}
				}

				code, err := getContractCode(entrySCID.Text)
				if err != nil {
					logger.Errorf("[Engram] Editor import function error: %s\n", err)
					errorText.Text = "cannot get contract for function import"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				if code == "" {
					errorText.Text = "contract does not exists"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				entrypoint := entryEntrypoint.Text
				contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(code, "\x00", ""))
				if err != nil {
					logger.Errorf("[Engram] Editor import parsing error: %s %s\n", err, pos)
					errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
					errorText.Color = apptheme.C.Red
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
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				formatted, err := tela.FormatSmartContract(tempSC, fmt.Sprintf("Function %s", entrypoint))
				if err != nil {
					logger.Errorf("[Engram] Editor import formatting error: %s\n", err)
					errorText.Text = "could not parse dvm to string"
					errorText.Color = apptheme.C.Red
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
				errorText.Color = apptheme.C.Green
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
						errorText.Color = apptheme.C.Green
						errorText.Refresh()
					}
				},
			)
		case "Parse": // Parse SC for errors
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			_, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				logger.Errorf("[Engram] Parse SC: %s %s\n", err, pos)
				return
			}

			errorText.Text = "contract parsed successfully"
			errorText.Color = apptheme.C.Green
			errorText.Refresh()
		case "Set Headers": // Set Artificer standard headers into initialize func
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = apptheme.C.Red
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
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
							return
						}

						entryCode.SetText(code)

						errorText.Text = "headers updated"
						errorText.Color = apptheme.C.Green
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
										errorText.Color = apptheme.C.Red
										errorText.Refresh()
										return
									} else if i > 0 && function.LineNumbers[index+1] < function.LineNumbers[index]+4 {
										err = fmt.Errorf("no room for header lines below %d", function.LineNumbers[index])
										errorText.Text = err.Error()
										errorText.Color = apptheme.C.Red
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
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						return
					}

					code, err := tela.FormatSmartContract(contract, entryCode.Text)
					if err != nil {
						logger.Errorf("[Engram] Format code error: %s\n", err)
						err = errors.New("could not parse dvm to string")
						errorText.Text = err.Error()
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						return
					}

					if code == entryCode.Text {
						errorText.Text = "did not change headers"
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
						return
					}

					entryCode.SetText(code)

					errorText.Text = "contract headers added successfully"
					errorText.Color = apptheme.C.Green
					errorText.Refresh()
				}

				codeCheck, err := tela.FormatSmartContract(contract, entryCode.Text)
				if err != nil {
					logger.Errorf("[Engram] Format code error: %s\n", err)
					err = errors.New("could not parse dvm to string")
					errorText.Text = err.Error()
					errorText.Color = apptheme.C.Red
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
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(entryCode.Text, "\x00", ""))
			if err != nil {
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				logger.Errorf("[Engram] Format: %s %s\n", err, pos)
				return
			}

			code, err := tela.FormatSmartContract(contract, entryCode.Text)
			if err != nil {
				logger.Errorf("[Engram] Format code error: %s\n", err)
				err = errors.New("could not parse dvm to string")
				errorText.Text = err.Error()
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			if code == entryCode.Text {
				errorText.Text = "contract code is formatted"
				errorText.Color = apptheme.C.Green
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
						errorText.Color = apptheme.C.Green
						errorText.Refresh()
					}
				},
			)
		case "Export": // Export SC to file
			if entryCode.Text == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			exportFileName := fmt.Sprintf("%s.bas", entryName.Text)

			data := []byte(entryCode.Text)
			dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
				if err != nil {
					logger.Errorf("[Engram] File dialog: %s\n", err)
					errorText.Text = "could not export contract file"
					errorText.Color = apptheme.C.Red
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
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				unsavedChanges = false
				filedata = entryCode.Text
				errorText.Text = "exported contract file successfully"
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

			dialogFileSave.SetFilter(storage.NewExtensionFileFilter([]string{".bas"}))
			dialogFileSave.SetView(dialog.ListView)
			dialogFileSave.SetFileName(exportFileName)
			dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
			dialogFileSave.Show()
		case "Install": // Install SC
			code := entryCode.Text
			if code == "" {
				errorText.Text = "contract code is empty"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			contract, pos, err := dvm.ParseSmartContract(strings.ReplaceAll(code, "\x00", ""))
			if err != nil {
				logger.Errorf("[Engram] Install SC: %s %s\n", err, pos)
				errorText.Text = fmt.Sprintf("error parsing contract %s", pos)
				errorText.Color = apptheme.C.Red
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
				errorText.Color = apptheme.C.Red
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

				header := canvas.NewText(i18n.T("files.install_contract"), apptheme.C.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				subHeader := canvas.NewText(fmt.Sprintf(i18n.T("files.params_fmt"), entrypoint), apptheme.C.Account)
				subHeader.TextSize = scaleFont(22)
				subHeader.Alignment = fyne.TextAlignCenter
				subHeader.TextStyle = fyne.TextStyle{Bold: true}

				linkClose := widget.NewHyperlinkWithStyle(i18n.T("common.close"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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
						canvas.NewRectangle(apptheme.C.DarkMatter),
					),
				)

				paramsContainer := container.NewVBox()

				btnInstall := widget.NewButton(i18n.T("files.install"), nil)

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
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
									return
								}

								unsavedChanges = false
								errorText.Text = "contract installed successfully"
								errorText.Color = apptheme.C.Green
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
								errorText.Color = apptheme.C.Red
								errorText.Refresh()
								return
							}

							unsavedChanges = false
							errorText.Text = "contract installed successfully"
							errorText.Color = apptheme.C.Green
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
