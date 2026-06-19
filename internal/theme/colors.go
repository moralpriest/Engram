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

var atlantisColors = Colors{
	Network:    color.RGBA{R: 52, G: 162, B: 181, A: 255},  // cyan-teal
	Account:    color.RGBA{R: 184, G: 212, B: 208, A: 255}, // pale seafoam
	DarkMatter: color.RGBA{4, 18, 21, 255},                 // hadal zone background
	Red:        color.RGBA{R: 214, B: 74, G: 70, A: 255},
	DarkGreen:  color.RGBA{19, 108, 122, 255},             // darker cyan
	Green:      color.RGBA{R: 52, G: 162, B: 181, A: 255}, // primary cyan-teal
	Blue:       color.RGBA{R: 26, G: 99, B: 120, A: 255},  // deep ocean blue
	Gray:       color.RGBA{R: 90, B: 122, G: 122, A: 255}, // muted seafoam
	Yellow:     color.RGBA{122, 154, 74, 255},             // phosphorescent green
	Cold:       color.RGBA{28, 63, 69, 255},               // inactive deep water
	Flint:      color.RGBA{18, 42, 46, 255},               // card surface
	Purple:     color.RGBA{107, 91, 138, 255},             // bioluminescent purple
	LightBlue:  color.RGBA{R: 232, G: 184, B: 75, A: 255}, // ancient amber marquee
	SoftRed:    color.RGBA{R: 240, G: 110, B: 110, A: 255},
}

var eldoradoColors = Colors{
	Network:    color.RGBA{R: 255, G: 215, B: 0, A: 255},
	Account:    color.RGBA{R: 233, G: 228, B: 233, A: 0xff},
	DarkMatter: color.RGBA{30, 20, 10, 255}, // dark bronze
	Red:        color.RGBA{R: 214, B: 74, G: 70, A: 255},
	DarkGreen:  color.RGBA{184, 134, 11, 0xff},
	Green:      color.RGBA{R: 255, G: 215, B: 0, A: 255},
	Blue:       color.RGBA{R: 218, G: 165, B: 32, A: 255},
	Gray:       color.RGBA{R: 99, B: 110, G: 99, A: 0xff},
	Yellow:     color.RGBA{255, 191, 0, 255},
	Cold:       color.RGBA{60, 73, 92, 255},
	Flint:      color.RGBA{44, 44, 52, 0xff},
	Purple:     color.RGBA{205, 127, 50, 0xff},
	LightBlue:  color.RGBA{80, 200, 120, 255},
	SoftRed:    color.RGBA{R: 240, G: 110, B: 110, A: 255},
}

var crystallinaColors = Colors{
	Network:    color.RGBA{100, 200, 210, 255}, // aquamarine
	Account:    color.RGBA{56, 56, 74, 255},    // dark slate text on white
	DarkMatter: color.RGBA{240, 242, 245, 255}, // off-white background
	Red:        color.RGBA{214, 74, 70, 255},
	DarkGreen:  color.RGBA{92, 60, 159, 255},  // dark amethyst
	Green:      color.RGBA{124, 92, 191, 255}, // amethyst primary
	Blue:       color.RGBA{92, 180, 240, 255}, // ice blue
	Gray:       color.RGBA{142, 142, 160, 255},
	Yellow:     color.RGBA{245, 215, 68, 255},
	Cold:       color.RGBA{160, 170, 190, 255},
	Flint:      color.RGBA{216, 218, 230, 255}, // light card surfaces
	Purple:     color.RGBA{200, 150, 255, 255}, // light lilac
	LightBlue:  color.RGBA{56, 182, 255, 255},  // sky blue marquee
	SoftRed:    color.RGBA{240, 110, 110, 255},
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
	Network:    color.RGBA{R: 138, G: 43, B: 226, A: 255}, // purple
	Account:    color.RGBA{R: 233, G: 228, B: 233, A: 0xff},
	DarkMatter: color.RGBA{18, 12, 28, 255}, // dark eggplant
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
	ThemeEngram      = "engram"
	ThemeDerotopia   = "derotopia"
	ThemeElDorado    = "eldorado"
	ThemeCrystallina = "crystallina"
	ThemeAtlantis    = "atlantis"
)

// StatusTextColor returns an appropriate color for loading/status text,
// chosen for readability against the current theme's background.
// Returns dark teal for Crystallina, yellow for El Dorado, and the
// theme's LightBlue (marquee color) for Engram Classic and Derotopia.
func StatusTextColor() color.Color {
	switch ThemeMode {
	case ThemeCrystallina:
		return color.RGBA{0, 130, 150, 255} // dark teal
	case ThemeDerotopia:
		return C.LightBlue // candy pink
	case ThemeEngram, ThemeElDorado:
		return C.LightBlue // sky blue / emerald green
	case ThemeAtlantis:
		return color.RGBA{52, 162, 181, 255} // cyan for TELA status/loading
	default:
		return C.Yellow // any fallback
	}
}

// BalanceColor returns the color for the DERO balance amount text.
// Returns sky blue for Derotopia (matching dashboard icons), green for all others.
func BalanceColor() color.Color {
	if ThemeMode == ThemeDerotopia {
		return color.RGBA{56, 182, 255, 255} // sky blue, matching message icon and pulse
	}
	return C.Green
}

// IsLightTheme returns true if the current theme uses a light background.
// Used to skip dark-theme-only styling (e.g. HighImportance buttons).
func IsLightTheme() bool {
	switch ThemeMode {
	case ThemeCrystallina:
		return true
	default:
		return false // Atlantis is dark
	}
}

func Activate(name string) {
	switch name {
	case ThemeDerotopia:
		C = &derotopiaColors
		ThemeMode = ThemeDerotopia
	case ThemeElDorado:
		C = &eldoradoColors
		ThemeMode = ThemeElDorado
	case ThemeCrystallina:
		C = &crystallinaColors
		ThemeMode = ThemeCrystallina
	case ThemeAtlantis:
		C = &atlantisColors
		ThemeMode = ThemeAtlantis
	default:
		C = &engramColors
		ThemeMode = ThemeEngram
	}
}
