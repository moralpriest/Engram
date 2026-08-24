// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license that can be found in the LICENSE file.

package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

// showDialogResized shows a file dialog and sizes it to the app window on
// desktop. On mobile the native OS picker is used (full-screen), so resizing
// is skipped entirely: the OS-override path in FileDialog.Show() returns
// without creating a dialog window, and fyne 2.8.0's FileDialog.Resize calls
// MinSize() (which dereferences that nil window) before its own nil check,
// crashing the app the moment any file dialog is opened.
func showDialogResized(d *dialog.FileDialog) {
	d.Show()
	if isMobile() {
		return
	}
	d.Resize(fyne.NewSize(ui.Width, ui.Height))
}

func wrapMobileButton(obj fyne.CanvasObject) fyne.CanvasObject {
	if isMobile() {
		sizeEnforcer := canvas.NewRectangle(color.Transparent)
		sizeEnforcer.SetMinSize(scalePoint(48, 48))
		return container.NewStack(sizeEnforcer, obj)
	}
	return obj
}

func pulseButton(rect *canvas.Rectangle, done chan struct{}) {
	if rect == nil {
		if done != nil {
			close(done)
		}
		return
	}

	originalColor := rect.StrokeColor
	green := apptheme.C.Green

	anim := canvas.NewColorRGBAAnimation(originalColor, green, 1000*time.Millisecond, func(c color.Color) {
		rect.StrokeColor = c
		rect.Refresh()
	})
	anim.AutoReverse = true
	anim.Start()

	time.AfterFunc(2*time.Second, func() {
		if done != nil {
			close(done)
		}
	})
}

// tappableText is a simple text label that responds to taps.
// Used for the "Available" amount on the send page: tap to fill max.
type tappableText struct {
	widget.BaseWidget
	Text     string
	Color    color.Color
	FontSize float32
	onTapped func()
	lbl      *canvas.Text
}

func newTappableText(text string, textColor color.Color, onTap func()) *tappableText {
	t := &tappableText{
		Text:     text,
		Color:    textColor,
		FontSize: scaleFont(14),
		onTapped: onTap,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableText) CreateRenderer() fyne.WidgetRenderer {
	t.lbl = canvas.NewText(t.Text, t.Color)
	t.lbl.TextSize = t.FontSize
	t.lbl.TextStyle = fyne.TextStyle{Bold: true}
	return &tappableTextRenderer{label: t}
}

func (t *tappableText) Tapped(_ *fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *tappableText) SetText(s string) {
	t.Text = s
	if t.lbl != nil {
		t.lbl.Text = s
		t.lbl.Refresh()
	}
}

func (t *tappableText) SetColor(c color.Color) {
	t.Color = c
	if t.lbl != nil {
		t.lbl.Color = c
		t.lbl.Refresh()
	}
}

type tappableTextRenderer struct {
	label *tappableText
}

func (r *tappableTextRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func (r *tappableTextRenderer) Destroy() {
}

func (r *tappableTextRenderer) Layout(size fyne.Size) {
	r.label.lbl.Resize(size)
}

func (r *tappableTextRenderer) MinSize() fyne.Size {
	return r.label.lbl.MinSize()
}

func (r *tappableTextRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.label.lbl}
}

func (r *tappableTextRenderer) Refresh() {
	r.label.lbl.Text = r.label.Text
	r.label.lbl.Color = r.label.Color
	r.label.lbl.Refresh()
}
