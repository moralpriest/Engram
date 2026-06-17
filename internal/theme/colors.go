// Theme colors for the Engram wallet application.
package apptheme

import "image/color"

type Colors struct {
	Network    color.Color
	Account    color.Color
	Blue       color.Color
	Red        color.Color
	DarkGreen  color.Color
	Green      color.Color
	Gray       color.Color
	Yellow     color.Color
	DarkMatter color.Color
	Cold       color.Color
	Flint      color.Color
	Purple     color.Color
	LightBlue  color.Color
	SoftRed    color.Color
}

var engramColors = Colors{
	Network:    color.RGBA{R: 67, G: 239, B: 67, A: 255},
	Account:    color.RGBA{R: 233, G: 228, B: 233, A: 0xff},
	DarkMatter: color.RGBA{21, 23, 30, 255},
	Red:        color.RGBA{R: 214, B: 74, G: 70, A: 255},
	DarkGreen:  color.RGBA{17, 127, 78, 0xff},
	Green:      color.RGBA{19, 202, 105, 0xff},
	Blue:       color.RGBA{R: 27, B: 249, G: 127, A: 255},
	Gray:       color.RGBA{R: 99, B: 110, G: 99, A: 0xff},
	Yellow:     color.RGBA{244, 208, 11, 255},
	Cold:       color.RGBA{60, 73, 92, 255},
	Flint:      color.RGBA{44, 44, 52, 0xff},
	Purple:     color.RGBA{191, 64, 191, 0xff},
	LightBlue:  color.RGBA{56, 182, 255, 255},
	SoftRed:    color.RGBA{R: 240, G: 110, B: 110, A: 255},
}

var derotopiaColors = Colors{
	Network:    color.RGBA{R: 138, G: 43, B: 226, A: 255},
	Account:    color.RGBA{R: 233, G: 228, B: 233, A: 0xff},
	DarkMatter: color.RGBA{21, 23, 30, 255},
	Red:        color.RGBA{R: 214, B: 74, G: 70, A: 255},
	DarkGreen:  color.RGBA{98, 20, 180, 0xff},
	Green:      color.RGBA{138, 43, 226, 0xff},
	Blue:       color.RGBA{R: 106, G: 13, B: 173, A: 255},
	Gray:       color.RGBA{R: 99, B: 110, G: 99, A: 0xff},
	Yellow:     color.RGBA{244, 208, 11, 255},
	Cold:       color.RGBA{60, 73, 92, 255},
	Flint:      color.RGBA{44, 44, 52, 0xff},
	Purple:     color.RGBA{19, 202, 105, 0xff},
	LightBlue:  color.RGBA{255, 105, 180, 255}, // candy pink for marquee
	SoftRed:    color.RGBA{R: 240, G: 110, B: 110, A: 255},
}

// C is the active color palette. It points to the current theme's color set
// and is swapped when Activate is called.
var C = &engramColors

const (
	ThemeEngram    = "engram"
	ThemeDerotopia = "derotopia"
)

func Activate(name string) {
	switch name {
	case ThemeDerotopia:
		C = &derotopiaColors
		ThemeMode = ThemeDerotopia
	default:
		C = &engramColors
		ThemeMode = ThemeEngram
	}
}
