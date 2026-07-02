package main

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	apptheme "github.com/DEROFDN/engram/internal/theme"
)

type toggleSwitch struct {
	widget.BaseWidget
	checked  bool
	disabled bool
	onChange func(bool)

	animMu  sync.Mutex
	anim    *fyne.Animation
	knobPos float32
}

func newToggleSwitch(checked bool, onChange func(bool)) *toggleSwitch {
	t := &toggleSwitch{
		checked:  checked,
		onChange: onChange,
	}
	if checked {
		t.knobPos = 1
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *toggleSwitch) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(trackColor(t.checked, t.disabled))
	knob := canvas.NewRectangle(knobColor(t.disabled))
	return &toggleSwitchRenderer{t: t, track: track, knob: knob}
}

func (t *toggleSwitch) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(scaleSize(48), scaleSize(28))
}

func (t *toggleSwitch) Tapped(_ *fyne.PointEvent) {
	if t.disabled {
		return
	}
	t.checked = !t.checked
	t.animate()
	if t.onChange != nil {
		t.onChange(t.checked)
	}
}

func (t *toggleSwitch) setChecked(v bool) {
	if t.checked == v {
		return
	}
	t.checked = v
	t.animMu.Lock()
	if t.anim != nil {
		t.anim.Stop()
		t.anim = nil
	}
	t.animMu.Unlock()
	if v {
		t.knobPos = 1
	} else {
		t.knobPos = 0
	}
	t.Refresh()
}

func (t *toggleSwitch) Disable() {
	t.disabled = true
	t.Refresh()
}

func (t *toggleSwitch) Enable() {
	t.disabled = false
	t.Refresh()
}

func (t *toggleSwitch) animate() {
	t.animMu.Lock()
	defer t.animMu.Unlock()
	if t.anim != nil {
		t.anim.Stop()
	}
	start := t.knobPos
	target := float32(0)
	if t.checked {
		target = 1
	}
	t.anim = fyne.NewAnimation(150*time.Millisecond, func(progress float32) {
		t.knobPos = start + (target-start)*progress
		canvas.Refresh(t)
	})
	t.anim.Start()
}

type toggleSwitchRenderer struct {
	t     *toggleSwitch
	track *canvas.Rectangle
	knob  *canvas.Rectangle
}

func (r *toggleSwitchRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)
	r.track.CornerRadius = size.Height / 2

	knobDiam := size.Height - 4
	if knobDiam < 4 {
		knobDiam = 4
	}
	maxX := size.Width - knobDiam - 2
	if maxX < 0 {
		maxX = 0
	}
	knobX := float32(2) + maxX*r.t.knobPos

	r.knob.Move(fyne.NewPos(knobX, 2))
	r.knob.Resize(fyne.NewSize(knobDiam, knobDiam))
	r.knob.CornerRadius = knobDiam / 2
}

func (r *toggleSwitchRenderer) MinSize() fyne.Size {
	return r.t.MinSize()
}

func (r *toggleSwitchRenderer) Refresh() {
	r.track.FillColor = trackColor(r.t.checked, r.t.disabled)
	r.track.CornerRadius = r.t.Size().Height / 2
	r.knob.FillColor = knobColor(r.t.disabled)
	r.Layout(r.t.Size())
	canvas.Refresh(r.track)
	canvas.Refresh(r.knob)
}

func (r *toggleSwitchRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.knob}
}

func (r *toggleSwitchRenderer) Destroy() {}

func (r *toggleSwitchRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func trackColor(checked, disabled bool) color.Color {
	if disabled {
		return color.NRGBA{R: 60, G: 60, B: 70, A: 100}
	}
	if checked {
		return apptheme.C.Green
	}
	return color.NRGBA{R: 80, G: 80, B: 90, A: 200}
}

func knobColor(disabled bool) color.Color {
	if disabled {
		return color.NRGBA{R: 160, G: 160, B: 170, A: 180}
	}
	return color.White
}
