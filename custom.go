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
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type returnEntry struct {
	widget.Entry
	OnReturn func()
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

// ImageButton is a button that displays an image with proper button styling
type ImageButton struct {
	widget.Button
	image    *canvas.Image
	imageRes fyne.Resource
}

func NewImageButton(res fyne.Resource, onTap func()) *ImageButton {
	btn := &ImageButton{
		imageRes: res,
	}
	btn.OnTapped = onTap
	btn.Importance = widget.MediumImportance
	btn.ExtendBaseWidget(btn)
	return btn
}

func (b *ImageButton) CreateRenderer() fyne.WidgetRenderer {
	renderer := b.Button.CreateRenderer()
	b.image = canvas.NewImageFromResource(b.imageRes)
	imgWidth := scaleSize(90)
	imgHeight := scaleSize(35)
	b.image.SetMinSize(fyne.NewSize(imgWidth, imgHeight))
	b.image.FillMode = canvas.ImageFillContain

	return &imageButtonRenderer{
		baseRenderer: renderer,
		image:        b.image,
		button:       b,
	}
}

type imageButtonRenderer struct {
	baseRenderer fyne.WidgetRenderer
	image        *canvas.Image
	button       *ImageButton
}

func (r *imageButtonRenderer) Layout(size fyne.Size) {
	r.baseRenderer.Layout(size)
	imgWidth := scaleSize(90)
	imgHeight := scaleSize(35)
	r.image.Resize(fyne.NewSize(imgWidth, imgHeight))
	r.image.Move(fyne.NewPos((size.Width-imgWidth)/2, (size.Height-imgHeight)/2))
}

func (r *imageButtonRenderer) MinSize() fyne.Size {
	return scalePoint(100, 40)
}

func (r *imageButtonRenderer) Refresh() {
	r.baseRenderer.Refresh()
	r.image.Refresh()
}

func (r *imageButtonRenderer) Objects() []fyne.CanvasObject {
	objects := r.baseRenderer.Objects()
	return append(objects, r.image)
}

func (r *imageButtonRenderer) Destroy() {
	r.baseRenderer.Destroy()
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
	OnFocusLost   func()
	OnFocusGained func()
}

// NewMobileEntry creates a new single line entry widget with more options for mobile devices
func NewMobileEntry() *mobileEntry {
	entry := &mobileEntry{}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (o *mobileEntry) FocusGained() {
	o.Entry.FocusGained()
	if o.OnFocusGained != nil {
		o.OnFocusGained()
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

func newSizedIconButton(icon fyne.Resource, onTap func()) *fyne.Container {
	btn := widget.NewButtonWithIcon("", icon, onTap)
	btn.Importance = widget.MediumImportance
	sizeEnforcer := canvas.NewRectangle(color.Transparent)
	h := float32(40)
	if isMobile() {
		h = 48
	}
	sizeEnforcer.SetMinSize(scalePoint(100, h))
	return container.NewStack(sizeEnforcer, btn)
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
