// Copyright 2023-2024 DERO Foundation. All rights reserved.
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
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var appDriver fyne.Device

type returnEntry struct {
	widget.Entry
	OnReturn      func()
	OnFocusGained func()
}

// NewReturnEntry creates a new single line entry widget that executes a function when the
// return key is pressed
func NewReturnEntry() *returnEntry {
	entry := &returnEntry{}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *returnEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn:
		e.OnReturn()
	case fyne.KeyEnter:
		e.OnReturn()
	default:
		e.Entry.TypedKey(key)
	}
}

func (e *returnEntry) FocusGained() {
	e.Entry.FocusGained()
	if e.OnFocusGained != nil {
		e.OnFocusGained()
	}
}

// NewImageButton creates a button that displays an image with proper button styling
func NewImageButton(res fyne.Resource, onTap func(), width ...float32) *fyne.Container {
	btn := widget.NewButton("", onTap)
	btn.Importance = widget.MediumImportance

	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain

	imgWidth := scaleSize(90)
	imgHeight := scaleSize(35)

	imgSizer := container.NewGridWrap(fyne.NewSize(imgWidth, imgHeight), img)

	sizeEnforcer := canvas.NewRectangle(color.Transparent)
	w := float32(100)
	if len(width) > 0 && width[0] > 0 {
		w = width[0]
	}
	h := float32(40)
	if isMobile() {
		h = 48
	}
	sizeEnforcer.SetMinSize(scalePoint(w, h))

	return container.NewStack(sizeEnforcer, btn, container.NewCenter(imgSizer))
}

var _ fyne.Draggable = (*iframe)(nil)

type iframe struct {
	widget.BaseWidget
}

func (o *iframe) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.99))
	o.ExtendBaseWidget(o)
	return &iframeRenderer{
		rect: rect,
	}
}

func (o *iframe) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	return o.BaseWidget.MinSize()
}

func (o *iframe) Tapped(e *fyne.PointEvent) {

}

func (o *iframe) TappedSecondary(e *fyne.PointEvent) {

}

func (o *iframe) Dragged(e *fyne.DragEvent) {
	if engram.Disk != nil {
		if nav.PosX == 0 && nav.PosY == 0 {
			nav.PosX = e.Position.X
			nav.PosY = e.Position.Y
		}
		nav.CurX = e.Position.X
		nav.CurY = e.Position.Y
	}
}

func (o *iframe) DragEnd() {
	/*
		if engram.Disk != nil {
			if nav.CurX > nav.PosX+30 {
				if session.Domain == "app.wallet" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutIdentity())
				} else if session.Domain == "app.remoteaccess" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutDashboard())
				}
			} else if nav.CurX < nav.PosX-30 {
				if session.Domain == "app.wallet" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutRemoteAccess())
				} else if session.Domain == "app.Identity" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutDashboard())
				}
			} else if nav.CurY > nav.PosY+30 {
				if session.Domain == "app.wallet" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutTransfers())
				} else if session.Domain == "app.messages" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutDashboard())
				} else if session.Domain == "app.messages.contact" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutMessages())
				}
			} else if nav.CurY < nav.PosY-30 {
				if session.Domain == "app.wallet" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutMessages())
				} else if session.Domain == "app.transfers" {
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutDashboard())
				}
			}

			nav.PosX = 0
			nav.PosY = 0
		}
	*/
	if engram.Disk != nil {
		if nav.CurY > nav.PosY+30 {
			if session.Domain == "app.messages" {
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMessages())
			} else if session.Domain == "app.messages.contact" {
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutPM())
			} else if session.Domain == "app.Identity" {
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutIdentity())
			}
		}
	}
}

var _ fyne.WidgetRenderer = (*iframeRenderer)(nil)

type iframeRenderer struct {
	rect *canvas.Rectangle
}

func (o *iframeRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func (o *iframeRenderer) Destroy() {
}

func (o *iframeRenderer) Layout(size fyne.Size) {
	o.rect.Resize(size)
}

func (o *iframeRenderer) MinSize() fyne.Size {
	return o.rect.MinSize()
}

func (o *iframeRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{o.rect}
}

func (o *iframeRenderer) Refresh() {
	uiDo(func() {
		o.rect.Refresh()
	})
}

type mobileEntry struct {
	widget.Entry
	OnReturn      func()
	OnFocusLost   func()
	OnFocusGained func()
	scrollBox     *container.Scroll
}

// NewMobileEntry creates a new single line entry widget with more options for mobile devices
func NewMobileEntry() *mobileEntry {
	entry := &mobileEntry{}
	entry.ExtendBaseWidget(entry)
	return entry
}

// NewMobileEntryWithScroll creates a new single line entry widget with mobile-focused scrolling behavior
func NewMobileEntryWithScroll(scrollBox *container.Scroll) *mobileEntry {
	entry := &mobileEntry{scrollBox: scrollBox}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (o *mobileEntry) FocusGained() {
	o.Entry.FocusGained()
	if o.scrollBox != nil {
		scrollToFieldOnMobile(o, o.scrollBox)
	} else if currentScrollBox != nil {
		scrollToFieldOnMobile(o, currentScrollBox)
	}
	if o.OnFocusGained != nil {
		o.OnFocusGained()
	}
}

func (o *mobileEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if o.OnReturn != nil {
			o.OnReturn()
		}
	default:
		o.Entry.TypedKey(key)
	}
}

type contextMenuButton struct {
	widget.Button
	menu *fyne.Menu
}

func (o *contextMenuButton) Tapped(e *fyne.PointEvent) {
	widget.ShowPopUpMenuAtPosition(o.menu, fyne.CurrentApp().Driver().CanvasForObject(o), e.AbsolutePosition)
}

// NewContextMenuButton creates a new button widget with a dropdown menu
func NewContextMenuButton(label string, image fyne.Resource, menu *fyne.Menu) *contextMenuButton {
	o := &contextMenuButton{menu: menu}
	o.Text = label
	o.SetIcon(image)
	o.ExtendBaseWidget(o)
	return o
}

// NewVScroll places content in a VScroll container for mobile orientations and scrolling
func NewVScroll(content *fyne.Container) *container.Scroll {
	return container.NewVScroll(container.NewCenter(content, widget.NewLabel("")))
}

// SetCurrentScrollBox sets the global scroll container for mobile input focus scrolling
func SetCurrentScrollBox(sb *container.Scroll) {
	currentScrollBox = sb
}

func newSizedIconButton(icon fyne.Resource, onTap func(), width ...float32) *fyne.Container {
	btn := widget.NewButtonWithIcon("", icon, onTap)
	btn.Importance = widget.MediumImportance
	sizeEnforcer := canvas.NewRectangle(color.Transparent)
	h := float32(40)
	if isMobile() {
		h = 48
	}
	w := float32(100)
	if len(width) > 0 && width[0] > 0 {
		w = width[0]
	}
	sizeEnforcer.SetMinSize(scalePoint(w, h))
	return container.NewStack(sizeEnforcer, btn)
}

func newSmallIconLink(label string, icon fyne.Resource, onTap func()) *fyne.Container {
	link := widget.NewHyperlinkWithStyle(label, nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	link.OnTapped = onTap
	iconImg := canvas.NewImageFromResource(icon)
	iconImg.FillMode = canvas.ImageFillContain
	h := float32(14)
	if isMobile() {
		h = 16
	}
	iconImg.SetMinSize(scalePoint(14, h))
	sizeEnforcer := canvas.NewRectangle(color.Transparent)
	sizeEnforcer.SetMinSize(scalePoint(80, 20))
	linkContainer := container.NewHBox(iconImg, link)
	return container.NewStack(sizeEnforcer, linkContainer)
}

func newSmallIconButton(label string, icon fyne.Resource, onTap func()) *fyne.Container {
	btn := widget.NewButtonWithIcon(label, icon, onTap)
	btn.Importance = widget.LowImportance
	return container.NewGridWrap(scalePoint(110, 40), btn)
}

type slimProgressBar struct {
	widget.BaseWidget
	value     float64
	height    float32
	minWidth  float32
	minHeight float32
	label     *canvas.Text
}

func NewSlimProgressBar() *slimProgressBar {
	bar := &slimProgressBar{height: scaleSize(6)}
	bar.label = canvas.NewText("0%", colors.Gray)
	bar.label.TextSize = scaleFont(11)
	bar.label.Alignment = fyne.TextAlignTrailing
	bar.ExtendBaseWidget(bar)
	return bar
}

func (s *slimProgressBar) SetBarMinSize(size fyne.Size) {
	s.minWidth = size.Width
	s.minHeight = size.Height
	s.Refresh()
}

func (s *slimProgressBar) SetValue(value float64) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	if s.value == value {
		return
	}
	s.value = value
	s.Refresh()
}

func (s *slimProgressBar) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(color.NRGBA{R: 36, G: 38, B: 44, A: 0xff})
	track.CornerRadius = s.height / 2
	fill := canvas.NewRectangle(colors.Green)
	fill.CornerRadius = s.height / 2
	barWrap := container.NewStack(track, fill)
	objects := []fyne.CanvasObject{barWrap, s.label}
	return &slimProgressBarRenderer{bar: s, track: track, fill: fill, label: s.label, barWrap: barWrap, objects: objects}
}

func (s *slimProgressBar) MinSize() fyne.Size {
	w := s.minWidth
	h := s.minHeight
	if w <= 0 {
		w = scaleSize(160)
	}
	if h <= 0 {
		h = scaleSize(16)
	}
	return fyne.NewSize(w, h)
}

type slimProgressBarRenderer struct {
	bar     *slimProgressBar
	track   *canvas.Rectangle
	fill    *canvas.Rectangle
	label   *canvas.Text
	barWrap *fyne.Container
	objects []fyne.CanvasObject
}

func (r *slimProgressBarRenderer) Layout(size fyne.Size) {
	labelWidth := scaleSize(34)
	barWidth := size.Width - labelWidth
	if barWidth < 0 {
		barWidth = 0
	}
	barY := (size.Height - r.bar.height) / 2
	r.barWrap.Move(fyne.NewPos(0, barY))
	r.barWrap.Resize(fyne.NewSize(barWidth, r.bar.height))
	fillWidth := barWidth * float32(r.bar.value)
	if r.bar.value > 0 && fillWidth < r.bar.height {
		fillWidth = r.bar.height
	}
	if fillWidth > barWidth {
		fillWidth = barWidth
	}
	r.track.Move(fyne.NewPos(0, 0))
	r.track.Resize(fyne.NewSize(barWidth, r.bar.height))
	r.fill.Move(fyne.NewPos(0, 0))
	r.fill.Resize(fyne.NewSize(fillWidth, r.bar.height))
	labelY := (size.Height - r.bar.label.TextSize) / 2
	r.label.Move(fyne.NewPos(barWidth+scaleSize(4), labelY-scaleSize(1)))
	r.label.Resize(fyne.NewSize(labelWidth-scaleSize(4), r.bar.label.TextSize))
}

func (r *slimProgressBarRenderer) MinSize() fyne.Size {
	return r.bar.MinSize()
}

func (r *slimProgressBarRenderer) Refresh() {
	r.track.FillColor = color.NRGBA{R: 36, G: 38, B: 44, A: 0xff}
	r.fill.FillColor = colors.Green
	r.track.CornerRadius = r.bar.height / 2
	r.fill.CornerRadius = r.bar.height / 2
	r.bar.label.Text = fmt.Sprintf("%d%%", int(r.bar.value*100+0.5))
	r.bar.label.Color = colors.Gray
	r.bar.label.Refresh()
	r.Layout(r.bar.Size())
	canvas.Refresh(r.track)
	canvas.Refresh(r.fill)
}

func (r *slimProgressBarRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *slimProgressBarRenderer) Destroy() {}

func (r *slimProgressBarRenderer) BackgroundColor() color.Color {
	return theme.Color(theme.ColorNameBackground)
}

type spacer struct {
	widget.BaseWidget
	width  float32
	height float32
}

type spacerRenderer struct {
	spacer *spacer
}

func NewSpacer(width, height float32) *spacer {
	s := &spacer{
		width:  width,
		height: height,
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *spacer) CreateRenderer() fyne.WidgetRenderer {
	return &spacerRenderer{spacer: s}
}

func (s *spacer) MinSize() fyne.Size {
	return fyne.NewSize(s.width, s.height)
}

func (r *spacerRenderer) Layout(size fyne.Size) {}

func (r *spacerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.spacer.width, r.spacer.height)
}

func (r *spacerRenderer) Refresh() {}

func (r *spacerRenderer) Objects() []fyne.CanvasObject {
	return nil
}

func (r *spacerRenderer) Destroy() {}

// scrollToFieldOnMobile scrolls the scroll container to ensure a field is visible
// above the keyboard on mobile devices
func scrollToFieldOnMobile(field fyne.CanvasObject, scrollBox *container.Scroll) {
	if !isMobile() {
		return
	}

	if field == nil || scrollBox == nil {
		return
	}

	go func() {
		time.Sleep(300 * time.Millisecond)

		fyne.Do(func() {
			if appExiting {
				return
			}

			canvas := fyne.CurrentApp().Driver().CanvasForObject(field)
			if canvas == nil {
				return
			}

			fieldPosOnScreen := fyne.CurrentApp().Driver().AbsolutePositionForObject(field)
			if fieldPosOnScreen.Y <= 0 {
				return
			}

			desiredY := scaleSize(160)
			scrollAmount := fieldPosOnScreen.Y - desiredY
			if scrollAmount <= 0 {
				return
			}

			newOffset := scrollBox.Offset.Y + scrollAmount
			if newOffset < 0 {
				newOffset = 0
			}

			scrollBox.Offset = fyne.NewPos(0, newOffset)
			scrollBox.Refresh()
		})
	}()
}

func showVirtualKeyboard(obj fyne.Focusable) {
	if obj == nil || appDriver == nil {
		return
	}
	mobileDriver, ok := appDriver.(interface{ ShowVirtualKeyboard() })
	if !ok {
		return
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		fyne.Do(func() {
			mobileDriver.ShowVirtualKeyboard()
		})
	}()
}

func hideVirtualKeyboard() {
	if appDriver == nil {
		return
	}
	mobileDriver, ok := appDriver.(interface{ HideVirtualKeyboard() })
	if !ok {
		return
	}
	mobileDriver.HideVirtualKeyboard()
}
