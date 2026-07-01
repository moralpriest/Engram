package main

import (
	"bytes"
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	apptheme "github.com/DEROFDN/engram/internal/theme"
)

//go:embed assets/DaemonON.svg
var daemonSvgData []byte

//go:embed assets/MinerON.svg
var minerSvgData []byte

//go:embed assets/MinerOFF.svg
var minerOffSvgData []byte

//go:embed assets/MinerOFF.png
var minerOffPngData []byte

//go:embed assets/green-cyberdeck.svg
var cyberdeckSvgData []byte

// Login-page palette — main matches C.Green, dark matches C.DarkGreen.
const (
	cyberdeckMainEngram      = "#13ca69"
	cyberdeckDarkEngram      = "#015a2c"
	cyberdeckMainDerotopia   = "#8a2be2"
	cyberdeckDarkDerotopia   = "#6214b4"
	cyberdeckMainElDorado    = "#ffd700"
	cyberdeckDarkElDorado    = "#b8860b"
	cyberdeckMainCrystallina = "#7c5cbf"
	cyberdeckDarkCrystallina = "#5c3c9f"
	cyberdeckMainAtlantis    = "#34a2b5"
	cyberdeckDarkAtlantis    = "#136c7a"
)

// Dashboard palette — main matches cDaemonMiner, dark matches C.DarkGreen.
const (
	cyberdeckDashMainEngram      = "#13ca69"
	cyberdeckDashDarkEngram      = "#015a2c"
	cyberdeckDashMainDerotopia   = "#38b6ff"
	cyberdeckDashDarkDerotopia   = "#1a6d99"
	cyberdeckDashMainElDorado    = "#daa520"
	cyberdeckDashDarkElDorado    = "#b8860b"
	cyberdeckDashMainCrystallina = "#7c5cbf"
	cyberdeckDashDarkCrystallina = "#5c3c9f"
	cyberdeckDashMainAtlantis    = "#34a2b5"
	cyberdeckDashDarkAtlantis    = "#136c7a"
)

var (
	cyberdeckBaseMain = []byte("#13ca69")
	cyberdeckBaseDark = []byte("#015a2c")
)

func daemonIconResource() fyne.Resource {
	return theme.NewThemedResource(&fyne.StaticResource{StaticName: "DaemonON.svg", StaticContent: daemonSvgData})
}

func minerIconResource() fyne.Resource {
	return theme.NewThemedResource(&fyne.StaticResource{StaticName: "MinerON.svg", StaticContent: minerSvgData})
}

func minerOffIconResource() fyne.Resource {
	return &fyne.StaticResource{StaticName: "MinerOFF.png", StaticContent: minerOffPngData}
}

// cyberdeckIconResource returns the cyberdeck icon with the login-page palette
// (C.Green main + C.DarkGreen inner).
func cyberdeckIconResource() fyne.Resource {
	main, dark := cyberdeckMainEngram, cyberdeckDarkEngram
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		main, dark = cyberdeckMainDerotopia, cyberdeckDarkDerotopia
	case apptheme.ThemeElDorado:
		main, dark = cyberdeckMainElDorado, cyberdeckDarkElDorado
	case apptheme.ThemeCrystallina:
		main, dark = cyberdeckMainCrystallina, cyberdeckDarkCrystallina
	case apptheme.ThemeAtlantis:
		main, dark = cyberdeckMainAtlantis, cyberdeckDarkAtlantis
	}
	data := bytes.ReplaceAll(cyberdeckSvgData, cyberdeckBaseMain, []byte(main))
	data = bytes.ReplaceAll(data, cyberdeckBaseDark, []byte(dark))
	return &fyne.StaticResource{StaticName: "green-cyberdeck.svg", StaticContent: data}
}

// cyberdeck hover constants — outer/main becomes highlight, inner/dark keeps normal.
const (
	cyberdeckHoverMain      = "#ffffff"
	cyberdeckHoverMainCryst = "#38384a"
)

// cyberdeckDashboardIconResource returns the cyberdeck icon with the dashboard
// palette (cDaemonMiner main + C.DarkGreen inner).
func cyberdeckDashboardIconResource() fyne.Resource {
	main, dark := cyberdeckDashMainEngram, cyberdeckDashDarkEngram
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		main, dark = cyberdeckDashMainDerotopia, cyberdeckDashDarkDerotopia
	case apptheme.ThemeElDorado:
		main, dark = cyberdeckDashMainElDorado, cyberdeckDashDarkElDorado
	case apptheme.ThemeCrystallina:
		main, dark = cyberdeckDashMainCrystallina, cyberdeckDashDarkCrystallina
	case apptheme.ThemeAtlantis:
		main, dark = cyberdeckDashMainAtlantis, cyberdeckDashDarkAtlantis
	}
	data := bytes.ReplaceAll(cyberdeckSvgData, cyberdeckBaseMain, []byte(main))
	data = bytes.ReplaceAll(data, cyberdeckBaseDark, []byte(dark))
	return &fyne.StaticResource{StaticName: "green-cyberdeck.svg", StaticContent: data}
}

// cyberdeckDashboardIconHoverResource returns the cyberdeck icon for the
// dashboard hover state.  The outer/main body becomes the highlight colour
// (white on dark themes, dark slate on Crystallina) while the inner/dark
// shade keeps its normal dashboard value — see Theme.md §XIII.
func cyberdeckDashboardIconHoverResource() fyne.Resource {
	var main string
	switch apptheme.ThemeMode {
	case apptheme.ThemeCrystallina:
		main = cyberdeckHoverMainCryst
	default:
		main = cyberdeckHoverMain
	}

	dark := cyberdeckDashDarkEngram
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		dark = cyberdeckDashDarkDerotopia
	case apptheme.ThemeElDorado:
		dark = cyberdeckDashDarkElDorado
	case apptheme.ThemeCrystallina:
		dark = cyberdeckDashDarkCrystallina
	case apptheme.ThemeAtlantis:
		dark = cyberdeckDashDarkAtlantis
	}

	data := bytes.ReplaceAll(cyberdeckSvgData, cyberdeckBaseMain, []byte(main))
	data = bytes.ReplaceAll(data, cyberdeckBaseDark, []byte(dark))
	return &fyne.StaticResource{StaticName: "green-cyberdeck.svg", StaticContent: data}
}

