package main

import (
	"runtime"

	"fyne.io/fyne/v2"
)

const (
	ReferenceWidth float32 = 324.0
	MinScale       float32 = 0.8
	MaxScale       float32 = 1.5
)

func isMobile() bool {
	return runtime.GOOS == "android" || runtime.GOOS == "ios"
}

func isDesktop() bool {
	return !isMobile()
}

func scale() float32 {
	if !isMobile() {
		return 1.0
	}

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
