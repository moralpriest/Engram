package apptheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type TintTheme struct {
	fyne.Theme
	iconColor color.Color
}

func NewTintTheme(theme fyne.Theme, iconColor color.Color) *TintTheme {
	return &TintTheme{Theme: theme, iconColor: iconColor}
}

func (t *TintTheme) IconColor() color.Color {
	return t.iconColor
}

func (t *TintTheme) SetIconColor(c color.Color) {
	t.iconColor = c
}

func (t *TintTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameForeground {
		return t.iconColor
	}
	return t.Theme.Color(name, variant)
}
