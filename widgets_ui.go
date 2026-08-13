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

// walletBtn is a custom tappable button for wallet selection with colored background
type walletBtn struct {
	widget.BaseWidget
	BgColor  color.Color
	TxtColor color.Color
	Label    string
	onTapped func()
	bg       *canvas.Rectangle
	lbl      *canvas.Text
}

func newWalletBtn(label string, onTap func()) *walletBtn {
	w := &walletBtn{
		BgColor:  apptheme.C.DarkMatter,
		TxtColor: apptheme.C.Gray,
		Label:    label,
		onTapped: onTap,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *walletBtn) CreateRenderer() fyne.WidgetRenderer {
	w.bg = canvas.NewRectangle(w.BgColor)
	w.bg.CornerRadius = scaleSize(12)
	w.lbl = canvas.NewText(w.Label, w.TxtColor)
	w.lbl.Alignment = fyne.TextAlignCenter
	w.lbl.TextStyle = fyne.TextStyle{Bold: true}
	return &walletBtnRenderer{btn: w}
}

func (w *walletBtn) Tapped(_ *fyne.PointEvent) {
	if w.onTapped != nil {
		w.onTapped()
	}
}

func (w *walletBtn) SetColors(bg, txt color.Color) {
	w.BgColor = bg
	w.TxtColor = txt
	if w.bg != nil {
		w.bg.FillColor = bg
		w.bg.Refresh()
	}
	if w.lbl != nil {
		w.lbl.Color = txt
		w.lbl.Refresh()
	}
}

type walletBtnRenderer struct {
	btn *walletBtn
}

func (r *walletBtnRenderer) Layout(s fyne.Size) {
	r.btn.bg.Resize(s)
	r.btn.lbl.Resize(s)
}

func (r *walletBtnRenderer) MinSize() fyne.Size {
	return fyne.NewSize(scaleSize(100), scaleSize(36))
}

func (r *walletBtnRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.btn.bg, r.btn.lbl}
}

func (r *walletBtnRenderer) Refresh() {
	r.btn.bg.FillColor = r.btn.BgColor
	r.btn.bg.Refresh()
	r.btn.lbl.Text = r.btn.Label
	r.btn.lbl.Color = r.btn.TxtColor
	r.btn.lbl.Refresh()
}

func (r *walletBtnRenderer) Destroy() {}

func newAdaptiveButton(label string, icon fyne.Resource, tapped func()) fyne.CanvasObject {
	return wrapMobileButton(widget.NewButtonWithIcon(label, icon, tapped))
}

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
