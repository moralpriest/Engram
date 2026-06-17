package apptheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type TintTheme struct {
	fyne.Theme
	iconColor    color.Color
	primaryColor color.Color
}

func NewTintTheme(theme fyne.Theme, iconColor color.Color) *TintTheme {
	return &TintTheme{Theme: theme, iconColor: iconColor}
}

func NewTintThemeWithPrimary(theme fyne.Theme, iconColor, primaryColor color.Color) *TintTheme {
	return &TintTheme{Theme: theme, iconColor: iconColor, primaryColor: primaryColor}
}

func (t *TintTheme) IconColor() color.Color {
	return t.iconColor
}

func (t *TintTheme) SetIconColor(c color.Color) {
	t.iconColor = c
}

func (t *TintTheme) PrimaryColor() color.Color {
	return t.primaryColor
}

func (t *TintTheme) SetPrimaryColor(c color.Color) {
	t.primaryColor = c
}

func (t *TintTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameForeground && t.iconColor != nil {
		return t.iconColor
	}
	if name == theme.ColorNamePrimary && t.primaryColor != nil {
		return t.primaryColor
	}
	return t.Theme.Color(name, variant)
}
