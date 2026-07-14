package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/Icon-white.png
var engramWhitePngData []byte

// resourceEngramWhitePng is the white Engram logo as a PNG resource.
// Used as the main system tray icon for reliable cross-platform rendering.
var resourceEngramWhitePng = &fyne.StaticResource{
	StaticName:    "Icon-white.png",
	StaticContent: engramWhitePngData,
}
