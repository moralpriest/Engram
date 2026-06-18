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
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/Gnomon/structures"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/graviton"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

func layoutAssetExplorer() fyne.CanvasObject {
	session.Domain = "app.explorer"

	frame := &iframe{}

	heading := canvas.NewText(i18n.T("assets.heading"), apptheme.C.Gray)
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

	heading := canvas.NewText(i18n.T("assets.my_heading"), apptheme.C.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	results := canvas.NewText("", apptheme.C.Green)
	results.TextSize = scaleFont(13)

	labelLastScan := canvas.NewText("", apptheme.C.Green)
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
	entrySCID.PlaceHolder = i18n.T("assets.search_scid")
	entrySCID.SetIcon(theme.SearchIcon())

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
		session.Window.SetContent(layoutAssetExplorer())
		removeOverlays()
	})

	btnRescan := widget.NewButton(i18n.T("assets.rescan"), nil)
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
		results.Color = apptheme.C.Gray
		results.Refresh()
	} else if gnomon.Index == nil {
		results.Text = "  Asset tracking is disabled. Gnomon is inactive."
		results.Color = apptheme.C.Gray
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
			results.Color = apptheme.StatusTextColor()
			fyne.Do(func() {
				results.Refresh()
			})

			for gnomon.Index.LastIndexedHeight < int64(engram.Disk.Get_Daemon_Height()) {
				results.Text = fmt.Sprintf("  Gnomon is syncing... [%d / %d]", gnomon.Index.LastIndexedHeight, int64(engram.Disk.Get_Daemon_Height()))
				results.Color = apptheme.StatusTextColor()

				fyne.Do(func() {
					results.Refresh()
				})

				time.Sleep(time.Second * 1)
			}

			results.Text = "  Loading previous scan results..."
			results.Color = apptheme.StatusTextColor()

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
				results.Color = apptheme.StatusTextColor()

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
						results.Color = apptheme.StatusTextColor()
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
				results.Color = apptheme.StatusTextColor()

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
						results.Color = apptheme.StatusTextColor()

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
				results.Color = apptheme.C.Green

				labelLastScan.Text = fmt.Sprintf("  %s", timeNow)
				labelLastScan.Color = apptheme.C.Green

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

			results.Color = apptheme.C.Green

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
							canvas.NewRectangle(apptheme.C.DarkMatter),
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

	heading := canvas.NewText(i18n.T("assets.manager"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelSigner := canvas.NewText(i18n.T("assets.author"), apptheme.C.Gray)
	labelSigner.TextSize = scaleFont(14)
	labelSigner.Alignment = fyne.TextAlignCenter
	labelSigner.TextStyle = fyne.TextStyle{Bold: true}

	labelOwner := canvas.NewText(i18n.T("assets.owner"), apptheme.C.Gray)
	labelOwner.TextSize = scaleFont(14)
	labelOwner.Alignment = fyne.TextAlignCenter
	labelOwner.TextStyle = fyne.TextStyle{Bold: true}

	labelSCID := canvas.NewText(i18n.T("assets.scid"), apptheme.C.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignCenter
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	labelBalance := canvas.NewText(i18n.T("assets.balance"), apptheme.C.Gray)
	labelBalance.TextSize = scaleFont(14)
	labelBalance.Alignment = fyne.TextAlignCenter
	labelBalance.TextStyle = fyne.TextStyle{Bold: true}

	labelTransfer := canvas.NewText(i18n.T("assets.transfer"), apptheme.C.Gray)
	labelTransfer.TextSize = scaleFont(14)
	labelTransfer.Alignment = fyne.TextAlignCenter
	labelTransfer.TextStyle = fyne.TextStyle{Bold: true}

	labelExecute := canvas.NewText(i18n.T("assets.execute"), apptheme.C.Gray)
	labelExecute.TextSize = scaleFont(14)
	labelExecute.Alignment = fyne.TextAlignCenter
	labelExecute.TextStyle = fyne.TextStyle{Bold: true}

	var ringsize uint64
	var err error

	options := []string{i18n.T("asset_ring.2"), i18n.T("asset_ring.4"), i18n.T("asset_ring.8"), i18n.T("asset_ring.16"), i18n.T("asset_ring.32"), i18n.T("asset_ring.64"), i18n.T("asset_ring.128")}

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

	btnSend := widget.NewButton(i18n.T("assets.send_asset"), nil)

	entryAddress.Validator = func(s string) error {
		btnSend.Text = i18n.T("assets.send_asset")
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
	entryAmount.PlaceHolder = i18n.T("assets.amount")
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

		btnSend.Text = i18n.T("assets.send_asset")
		btnSend.Enable()
		entryAmount.SetValidationError(nil)

		return nil
	}

	var zerobal uint64

	balance := canvas.NewText(fmt.Sprintf("  %d", zerobal), apptheme.C.Green)
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
						btnSend.Text = i18n.T("assets.send_asset")
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
		btnSend.Text = i18n.T("assets.send_asset")
		btnSend.Enable()
	} else {
		btnSend.Text = "You do not own this asset"
		btnSend.Disable()
	}
	btnSend.Refresh()

	linkCopySigner := widget.NewHyperlinkWithStyle(i18n.T("assets.copy_address"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopySigner.OnTapped = func() {
		a.Clipboard().SetContent(signer)
	}

	linkCopyOwner := widget.NewHyperlinkWithStyle(i18n.T("assets.copy_address"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopyOwner.OnTapped = func() {
		a.Clipboard().SetContent(owner)
	}

	linkMessageAuthor := widget.NewHyperlinkWithStyle(i18n.T("assets.message_author"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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

	linkMessageOwner := widget.NewHyperlinkWithStyle(i18n.T("assets.message_owner"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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

	linkCopySCID := widget.NewHyperlinkWithStyle(i18n.T("assets.copy_scid"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkCopySCID.OnTapped = func() {
		a.Clipboard().SetContent(scid)
	}

	linkView := widget.NewHyperlinkWithStyle(i18n.T("assets.view_explorer"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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

		options := []string{i18n.T("asset_ring.2"), i18n.T("asset_ring.4"), i18n.T("asset_ring.8"), i18n.T("asset_ring.16"), i18n.T("asset_ring.32"), i18n.T("asset_ring.64"), i18n.T("asset_ring.128")}

		var ringsize uint64

		signerRequired := false

		selectRingMembers := widget.NewSelect(options, nil)
		selectRingMembers.PlaceHolder = i18n.T("send.select_ring")

		for f := range contract.Functions {
			if contract.Functions[f].Name == s {
				params = contract.Functions[f].Params

				header := canvas.NewText(i18n.T("assets.execute_func"), apptheme.C.Gray)
				header.TextSize = scaleFont(14)
				header.Alignment = fyne.TextAlignCenter
				header.TextStyle = fyne.TextStyle{Bold: true}

				funcName := canvas.NewText(s, apptheme.C.Account)
				funcName.TextSize = scaleFont(22)
				funcName.Alignment = fyne.TextAlignCenter
				funcName.TextStyle = fyne.TextStyle{Bold: true}

				linkClose := widget.NewHyperlinkWithStyle(i18n.T("assets.close"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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
						canvas.NewRectangle(apptheme.C.DarkMatter),
					),
				)

				entryDEROValue := widget.NewEntry()
				entryDEROValue.PlaceHolder = i18n.T("assets.dero_amount")
				entryDEROValue.Validator = func(s string) error {
					dero_amount, err = globals.ParseAmount(s)
					if err != nil {
						entryDEROValue.SetValidationError(err)
						return err
					}

					return nil
				}

				entryAssetValue := widget.NewEntry()
				entryAssetValue.PlaceHolder = i18n.T("assets.amount")
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

				btnExecuteBase := widget.NewButton(i18n.T("assets.execute"), nil)
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

	results := canvas.NewText("", apptheme.C.Green)
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
	entrySCID.PlaceHolder = i18n.T("assets.search_scid")
	entrySCID.SetIcon(theme.SearchIcon())
	entrySCID.Disable()

	linkClearHistory := widget.NewHyperlinkWithStyle(i18n.T("assets.clear_all"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: false})
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

	btnMyAssets := widget.NewButton(i18n.T("assets.my_assets_btn"), func() {
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
	results.Color = apptheme.C.Green
	results.Refresh()

	listData.Set(nil)

	if session.Offline {
		results.Text = "  Disabled in offline mode."
		results.Color = apptheme.C.Gray
		results.Refresh()
	} else if gnomon.Index == nil {
		results.Text = "  Gnomon is inactive."
		results.Color = apptheme.C.Gray
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
				results.Color = apptheme.StatusTextColor()

				fyne.Do(func() {
					results.Refresh()
				})

				time.Sleep(time.Second * 1)
			}

			fyne.Do(func() {
				entrySCID.Enable()
				results.Text = "  Loading previous scan history..."
				results.Color = apptheme.StatusTextColor()
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
			results.Text = fmt.Sprintf("  %s  %d", i18n.T("files.search_history"), found)
			results.Color = apptheme.C.Green
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
