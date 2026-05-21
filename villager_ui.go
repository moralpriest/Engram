// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license that can be found in the LICENSE file.

package main

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
)

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

	stopPulse := make(chan struct{})
	var closeOnce sync.Once
	closeMenu := func() {
		closeOnce.Do(func() {
			close(stopPulse)
		})
		overlay.Remove(menu)
	}

	var btnEdit fyne.CanvasObject

	onTapEdit := func() {
		closeMenu()
		showLoadingOverlay()
		go func() {
			EnsureXSWD()

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
	}

	if hasVillager {
		btnEdit = widget.NewButtonWithIcon("Edit Villager", theme.DocumentCreateIcon(), onTapEdit)
	} else {
		borderedBtn := newBorderedButtonWithIcon("Edit Villager", theme.DocumentCreateIcon(), color.White, onTapEdit, 240)
		btnEdit = borderedBtn

		go func() {
			if len(borderedBtn.Objects) > 1 {
				if bg, ok := borderedBtn.Objects[1].(*canvas.Rectangle); ok {
					for {
						select {
						case <-stopPulse:
							return
						default:
							done := make(chan struct{})
							pulseButton(bg, done)
							select {
							case <-done:
							case <-stopPulse:
								return
							}
						}
					}
				}
			}
		}()
	}

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
		closeMenu()
	})
	if !hasVillager {
		btnHide.Disable()
	}

	bgIcon := theme.ColorChromaticIcon()
	if session.VillagerBackground {
		bgIcon = theme.ColorAchromaticIcon()
	}
	btnBgToggle := widget.NewButtonWithIcon("Background Toggle", bgIcon, func() {
		session.VillagerBackground = !session.VillagerBackground
		val := "false"
		if session.VillagerBackground {
			val = "true"
		}
		go setTELADual("VillagerBackground", []byte(val))

		go func() {
			if engram.Disk != nil {
				address := engram.Disk.GetAddress().String()
				pixels := session.VillagerPixels
				if pixels == "" {
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
		closeMenu()
	})
	if !hasVillager {
		btnBgToggle.Disable()
	}

	btnClose := widget.NewButtonWithIcon("Close", theme.CancelIcon(), func() {
		closeMenu()
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

	background := canvas.NewRectangle(color.NRGBA{0, 0, 0, 150})

	menuBg := canvas.NewRectangle(theme.BackgroundColor())
	menuBg.CornerRadius = scaleSize(10)

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
