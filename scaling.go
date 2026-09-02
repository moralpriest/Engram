// Copyright 2023-2026 DERO Foundation. All rights reserved.
package main

import (
	"runtime"

	"fyne.io/fyne/v2"
)

const (
	ReferenceWidth  float32 = 324.0
	ReferenceHeight float32 = 680.0
	MinScale        float32 = 0.75
	MaxScale        float32 = 1.5
)

func isMobile() bool {
	return runtime.GOOS == "android" || runtime.GOOS == "ios"
}

func scaleByWidth() float32 {
	if ui.Width <= 0 {
		return 1.0
	}

	factor := ui.Width / ReferenceWidth

	if factor < MinScale {
		factor = MinScale
	}
	if factor > MaxScale {
		factor = MaxScale
	}

	return factor
}

func scaleByHeight() float32 {
	if ui.Height <= 0 {
		return 1.0
	}

	factor := ui.Height / ReferenceHeight

	if factor < MinScale {
		factor = MinScale
	}
	if factor > MaxScale {
		factor = MaxScale
	}

	return factor
}

func scale() float32 {
	if !isMobile() {
		return 1.0
	}

	w := scaleByWidth()
	h := scaleByHeight()
	if h < w {
		return h
	}
	return w
}

func isSmallScreen() bool {
	if !isMobile() {
		return false
	}
	return ui.Height < 700 || scale() < 0.9
}

func compactSpacing() float32 {
	if isSmallScreen() {
		return 0.5
	}
	return 1.0
}

func scaleSize(baseSize float32) float32 {
	return baseSize * scale()
}

func scalePoint(baseWidth, baseHeight float32) fyne.Size {
	return fyne.NewSize(
		scaleSize(baseWidth),
		scaleSize(baseHeight),
	)
}

func scaleFont(baseSize float32) float32 {
	factor := scale()
	if factor > 1.2 {
		factor = 1.2 + (factor-1.2)*0.5
	}
	return baseSize * factor
}

func statusDotSize() fyne.Size {
	return scalePoint(10, 10)
}

func smallSpacerSize() fyne.Size {
	return scalePoint(5, 5)
}

func standardSpacerSize() fyne.Size {
	return scalePoint(10, 5)
}

func compactSpacerSize() fyne.Size {
	return scalePoint(6, 5)
}

func buttonIconSize() fyne.Size {
	w := scaleSize(20)
	if w > 24 {
		w = 24
	}
	return fyne.NewSize(w, w)
}

func navIconSize() fyne.Size {
	s := scaleSize(16)
	if s > 20 {
		s = 20
	}
	return fyne.NewSize(s, s)
}
