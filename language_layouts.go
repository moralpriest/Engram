package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

func layoutLanguageSelector() fyne.CanvasObject {
	session.Domain = "app.language"

	frame := &iframe{}
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	languages := i18n.LanguageOrder()

	type pulseText struct {
		title     string
		subtitle1 string
		subtitle2 string
		name      string
	}

	pulseTexts := make([]pulseText, len(languages))
	savedLang := i18n.GetLanguage()
	startIdx := 0
	for i, code := range languages {
		if code == savedLang {
			startIdx = i
		}
		i18n.SetLanguage(code)
		pulseTexts[i] = pulseText{
			title:     i18n.T("language.title"),
			subtitle1: i18n.T("language.subtitle1"),
			subtitle2: i18n.T("language.subtitle2"),
			name:      i18n.AvailableLanguages()[code],
		}
	}
	i18n.SetLanguage(savedLang)

	title := canvas.NewText(pulseTexts[startIdx].title, apptheme.C.Green)
	title.TextSize = scaleFont(24)
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	sub1 := canvas.NewText(pulseTexts[startIdx].subtitle1, apptheme.C.Gray)
	sub1.TextSize = scaleFont(13)
	sub1.Alignment = fyne.TextAlignCenter

	sub2 := canvas.NewText(pulseTexts[startIdx].subtitle2, apptheme.C.Gray)
	sub2.TextSize = scaleFont(13)
	sub2.Alignment = fyne.TextAlignCenter

	var wLang *widget.Select
	selecting := false
	userSelected := false

	pulseIndex := 0
	updatePulseText := func(idx int, animate bool) {
		t := pulseTexts[idx]

		applyText := func() {
			pulseIndex = idx
			title.Text = t.title
			sub1.Text = t.subtitle1
			sub2.Text = t.subtitle2
			title.Refresh()
			sub1.Refresh()
			sub2.Refresh()

			if !selecting && wLang != nil && wLang.SelectedIndex() < 0 {
				wLang.PlaceHolder = t.name
				wLang.Refresh()
			}
		}

		if !animate {
			applyText()
			title.Color = apptheme.C.Green
			sub1.Color = apptheme.C.Gray
			sub2.Color = apptheme.C.Gray
			title.Refresh()
			sub1.Refresh()
			sub2.Refresh()
			return
		}

		// Fade out to transparent (background color)
		fadeOutTitle := canvas.NewColorRGBAAnimation(apptheme.C.Green, color.Transparent, time.Millisecond*600, func(c color.Color) {
			title.Color = c
			title.Refresh()
		})
		fadeOutSub1 := canvas.NewColorRGBAAnimation(apptheme.C.Gray, color.Transparent, time.Millisecond*600, func(c color.Color) {
			sub1.Color = c
			sub1.Refresh()
		})
		fadeOutSub2 := canvas.NewColorRGBAAnimation(apptheme.C.Gray, color.Transparent, time.Millisecond*600, func(c color.Color) {
			sub2.Color = c
			sub2.Refresh()
		})

		time.AfterFunc(time.Millisecond*600, func() {
			fyne.Do(func() {
				applyText()

				// Fade in from transparent
				fadeInTitle := canvas.NewColorRGBAAnimation(color.Transparent, apptheme.C.Green, time.Millisecond*600, func(c color.Color) {
					title.Color = c
					title.Refresh()
				})
				fadeInSub1 := canvas.NewColorRGBAAnimation(color.Transparent, apptheme.C.Gray, time.Millisecond*600, func(c color.Color) {
					sub1.Color = c
					sub1.Refresh()
				})
				fadeInSub2 := canvas.NewColorRGBAAnimation(color.Transparent, apptheme.C.Gray, time.Millisecond*600, func(c color.Color) {
					sub2.Color = c
					sub2.Refresh()
				})
				fadeInTitle.Start()
				fadeInSub1.Start()
				fadeInSub2.Start()
			})
		})

		fadeOutTitle.Start()
		fadeOutSub1.Start()
		fadeOutSub2.Start()
	}

	transitionToMain := func() {
		session.Domain = "app.main"
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	}

	// Dropdown pulses through languages for preview; user picks and clicks Confirm
	langNames := []string{}
	for _, code := range languages {
		langNames = append(langNames, i18n.AvailableLanguages()[code])
	}

	wLang = widget.NewSelect(langNames, nil)
	wLang.PlaceHolder = "..."
	if startIdx > 0 {
		wLang.SetSelectedIndex(startIdx)
	}

	btnConfirm := widget.NewButtonWithIcon("", theme.LoginIcon(), func() {
		idx := wLang.SelectedIndex()
		if idx < 0 {
			idx = startIdx
		}
		i18n.SetLanguageFromIndex(idx)
		StoreValue("settings", []byte("language"), []byte(languages[idx]))
		updateTrayLanguage()
		startAppForegroundAndroid()
		transitionToMain()
	})

	wLang.OnChanged = func(s string) {
		if selecting {
			return
		}
		selecting = true
		userSelected = true
		idx := 0
		for i, name := range langNames {
			if name == s {
				idx = i
				break
			}
		}
		updatePulseText(idx, false)
		selecting = false
	}

	updatePulseText(startIdx, false)

	dropdownCard := canvas.NewRectangle(color.Transparent)
	dropdownCard.SetMinSize(fyne.NewSize(ui.Width*0.6, 0))

	btnCard := canvas.NewRectangle(color.Transparent)
	btnCard.SetMinSize(fyne.NewSize(ui.Width*0.6, 0))

	form := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(title),
		rectSpacer,
		container.NewCenter(sub1),
		container.NewCenter(sub2),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewStack(dropdownCard, wLang),
		),
		rectSpacer,
		container.NewCenter(
			container.NewStack(btnCard, wrapMobileButton(btnConfirm)),
		),
	)

	layout := container.NewStack(
		frame,
		res.mainBg,
		container.NewCenter(form),
	)

	go func() {
		time.Sleep(2 * time.Second)
		for !appExiting {
			time.Sleep(3 * time.Second)
			if session.Domain != "app.language" || userSelected {
				return
			}
			fyne.Do(func() {
				if session.Domain != "app.language" || userSelected {
					return
				}
				newIdx := pulseIndex + 1
				if newIdx >= len(languages) {
					newIdx = 0
				}
				selecting = true
				wLang.SetSelectedIndex(newIdx)
				selecting = false
				updatePulseText(newIdx, true)
			})
		}
	}()

	return NewVScroll(layout)
}
