package apptheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type ETheme struct {
	Regular       fyne.Resource
	Bold          fyne.Resource
	Italic        fyne.Resource
	BoldItalic    fyne.Resource
	Astrolyt      fyne.Resource
	GoNoto        fyne.Resource
	ScaleFontSize func(float32) float32
}

type ETheme2 struct {
	Regular       fyne.Resource
	Bold          fyne.Resource
	Italic        fyne.Resource
	BoldItalic    fyne.Resource
	Astrolyt      fyne.Resource
	GoNoto        fyne.Resource
	ScaleFontSize func(float32) float32
}

var Main *ETheme
var Alt *ETheme2

// ThemeMode tracks the active theme for the Fyne theme implementations.
// Set by Activate() in colors.go.
var ThemeMode = ThemeEngram

func getBaseSize(s fyne.ThemeSizeName) float32 {
	switch s {
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameScrollBar:
		return 16
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameText:
		return 15
	case theme.SizeNameInputBorder:
		return 2
	case theme.SizeNameInputRadius:
		return 10
	case theme.SizeNameHeadingText:
		return 24
	default:
		return theme.DefaultTheme().Size(s)
	}
}

func accentNRGBA(r, g, b, a uint8) color.Color {
	if ThemeMode == ThemeDerotopia {
		return color.NRGBA{R: 138, G: 43, B: 226, A: a}
	}
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

func (t *ETheme) Color(c fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch c {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 21, G: 23, B: 30, A: 0xff}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 235, G: 235, B: 235, A: 0x99}
	case theme.ColorNameButton:
		return accentNRGBA(19, 202, 105, 0x75)
	case theme.ColorNameDisabledButton:
		return accentNRGBA(19, 202, 105, 0x22)
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 164, G: 164, B: 164, A: 0x42}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}
	case theme.ColorNameFocus:
		return accentNRGBA(19, 202, 105, 0x88)
	case theme.ColorNameForeground:
		return color.NRGBA{R: 208, G: 208, B: 208, A: 0xff}
	case theme.ColorNameHover:
		return accentNRGBA(19, 202, 105, 0x99)
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 31, G: 33, B: 40, A: 0xee}
	case theme.ColorNameInputBackground:
		return color.Alpha16{A: 0x0}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0x35}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 208, G: 208, B: 208, A: 0x19}
	case theme.ColorNamePrimary:
		return accentNRGBA(19, 202, 105, 0xff)
	case theme.ColorNameScrollBar:
		return accentNRGBA(19, 202, 105, 0x44)
	case theme.ColorNameShadow:
		return color.Alpha16{0x19}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 31, G: 33, B: 40, A: 0xff}
	default:
		return theme.DefaultTheme().Color(c, v)
	}
}

func (t *ETheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Symbol {
		return t.Astrolyt
	}
	if s.Monospace {
		return t.Regular
	}
	if s.Bold {
		if s.Italic {
			return t.BoldItalic
		}
		return t.Bold
	}
	if s.Italic {
		return t.Italic
	}
	return t.Regular
}

func (t *ETheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *ETheme) Size(s fyne.ThemeSizeName) float32 {
	return t.ScaleFontSize(getBaseSize(s))
}

func (t *ETheme2) Color(c fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch c {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 21, G: 23, B: 30, A: 0xff}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 235, G: 235, B: 235, A: 0x99}
	case theme.ColorNameButton:
		return accentNRGBA(19, 202, 105, 0x75)
	case theme.ColorNameDisabledButton:
		return accentNRGBA(19, 202, 105, 0x22)
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 164, G: 164, B: 164, A: 0x42}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}
	case theme.ColorNameFocus:
		return accentNRGBA(19, 202, 105, 0x88)
	case theme.ColorNameForeground:
		return color.NRGBA{R: 208, G: 208, B: 208, A: 0xff}
	case theme.ColorNameHover:
		return accentNRGBA(19, 202, 105, 0x99)
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 21, G: 23, B: 30, A: 0xee}
	case theme.ColorNameInputBackground:
		return color.Alpha16{A: 0x0}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 208, G: 208, B: 208, A: 0x19}
	case theme.ColorNamePrimary:
		return accentNRGBA(19, 202, 105, 0xff)
	case theme.ColorNameScrollBar:
		return accentNRGBA(19, 202, 105, 0x44)
	case theme.ColorNameShadow:
		return color.Alpha16{0x19}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 31, G: 33, B: 40, A: 0xff}
	default:
		return theme.DefaultTheme().Color(c, v)
	}
}

func (t *ETheme2) Font(s fyne.TextStyle) fyne.Resource {
	if s.Symbol {
		return t.Astrolyt
	}
	if s.Monospace {
		return t.GoNoto
	}
	if s.Bold {
		if s.Italic {
			return t.GoNoto
		}
		return t.Bold
	}
	if s.Italic {
		return t.GoNoto
	}
	return t.GoNoto
}

func (t *ETheme2) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *ETheme2) Size(s fyne.ThemeSizeName) float32 {
	return t.ScaleFontSize(getBaseSize(s))
}
