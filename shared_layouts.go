// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	x "fyne.io/x/fyne/widget"

	"github.com/DEROFDN/engram/i18n"
)

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

func layoutWaiting(title *canvas.Text, heading *canvas.Text, sub *canvas.Text, link *widget.Hyperlink) fyne.CanvasObject {
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.6, ui.Height*0.35))
	rect2 := canvas.NewRectangle(color.Transparent)
	rect2.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))
	frame := canvas.NewRectangle(color.Transparent)
	frame.SetMinSize(fyne.NewSize(ui.Width, ui.Height))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())
	label := canvas.NewText(i18n.T("register.proof_of_work"), colors.Gray)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.TextSize = scaleFont(12)
	hashes := canvas.NewText(fmt.Sprintf("%d", session.RegHashes), colors.Account)
	hashes.TextSize = scaleFont(18)

	startTime := time.Now()
	timeLabel := canvas.NewText(i18n.T("register.countdown_estimating"), colors.Green)
	timeLabel.TextStyle = fyne.TextStyle{Bold: true}
	timeLabel.TextSize = scaleFont(12)

	go func() {
		for engram.Disk != nil && session.Domain == "app.register" {
			elapsed := time.Since(startTime).Seconds()
			fyne.Do(func() {
				hashes.Text = fmt.Sprintf("%d", session.RegHashes)
				hashes.Refresh()

				if elapsed >= 2.0 && session.RegHashes > 0 {
					hashRate := float64(session.RegHashes) / elapsed
					expectedTotal := 16777216.0 / hashRate
					remaining := expectedTotal - elapsed

					if remaining <= 0 {
						timeLabel.Text = i18n.T("register.countdown_completing")
					} else if remaining > 3600 {
						timeLabel.Text = fmt.Sprintf(i18n.T("register.countdown_fmt_hours"), int(remaining)/3600, (int(remaining)%3600)/60)
					} else if remaining > 60 {
						timeLabel.Text = fmt.Sprintf(i18n.T("register.countdown_fmt_minutes"), int(remaining)/60, int(remaining)%60)
					} else {
						timeLabel.Text = fmt.Sprintf(i18n.T("register.countdown_fmt_seconds"), int(remaining))
					}
				} else {
					timeLabel.Text = i18n.T("register.countdown_estimating")
				}
				timeLabel.Refresh()
			})
			time.Sleep(500 * time.Millisecond)
		}
	}()

	session.Gif, _ = x.NewAnimatedGifFromResource(resourceAnimation2Gif)
	session.Gif.SetMinSize(rect.MinSize())
	session.Gif.Resize(rect.MinSize())
	session.Gif.Start()

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			title,
		),
		rectSpacer,
		rectSpacer,
	)

	waitForm := container.NewVBox(
		widget.NewLabel(""),
		rect2,
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
				rectSpacer,
				container.NewCenter(
					rect2,
					timeLabel,
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

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), link),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		footer,
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
		title.Text = i18n.T("common.error")
		heading.Text = i18n.T("error.connection_failure")
		sub.ParseMarkdown(fmt.Sprintf(i18n.T("error.connection_desc"), session.Daemon))
		labelSettings.Text = i18n.T("error.review_settings")
		labelSettings.OnTapped = func() {
			session.Window.SetContent(layoutSettings())
		}
	} else if t == 2 {
		title.Text = i18n.T("common.error")
		heading.Text = i18n.T("error.write_failure")
		sub.ParseMarkdown(i18n.T("error.write_desc"))
		labelSettings.Text = i18n.T("error.review_settings")
		labelSettings.OnTapped = func() {
			session.Window.SetContent(layoutMain())
		}
	} else {
		title.Text = i18n.T("common.error")
		heading.Text = i18n.T("error.id10t")
		sub.ParseMarkdown(i18n.T("error.system_malfunction"))
		labelSettings.Text = i18n.T("error.review_settings")
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

func layoutFrame() fyne.CanvasObject {
	return layoutFrameWithWallet("")
}

func layoutFrameWithWallet(singleWalletName string) fyne.CanvasObject {
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

		if langData, err := GetValue("settings", []byte("language")); err == nil && len(langData) > 0 {
			i18n.SetLanguage(string(langData))
			session.Window.SetContent(layoutMain())
		} else {
			session.Window.SetContent(layoutLanguageSelector())
		}

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
