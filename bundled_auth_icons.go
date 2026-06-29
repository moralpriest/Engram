package main

import (
	"bytes"
	_ "embed"

	"fyne.io/fyne/v2"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

//go:embed assets/green-dero.svg
var greenDeroSvgData []byte

//go:embed assets/green-wallet.svg
var greenWalletSvgData []byte

// Color replacement constants for green-dero.svg
const (
	deroOuterEngram      = "#13ca69"
	deroInnerEngram      = "#39e281"
	deroOuterDerotopia   = "#8a2be2"
	deroInnerDerotopia   = "#b98aff"
	deroOuterElDorado    = "#ffd700"
	deroInnerElDorado    = "#ffe873"
	deroOuterCrystallina = "#7c5cbf"
	deroInnerCrystallina = "#c896ff"
	deroOuterAtlantis    = "#34a2b5"
	deroInnerAtlantis    = "#6fd0e0"
	deroHoverOuter       = "#ffffff"
	deroHoverOuterCryst  = "#38384a"
)

// Color replacement constants for green-wallet.svg
const (
	walletMainEngram      = "#13ca69"
	walletDarkEngram      = "#016531"
	walletMainDerotopia   = "#8a2be2"
	walletDarkDerotopia   = "#6214b4"
	walletMainElDorado    = "#ffd700"
	walletDarkElDorado    = "#b8860b"
	walletMainCrystallina = "#7c5cbf"
	walletDarkCrystallina = "#5c3c9f"
	walletMainAtlantis    = "#34a2b5"
	walletDarkAtlantis    = "#136c7a"
	walletHoverMain       = "#ffffff"
	walletHoverMainCryst  = "#38384a"
)

func replaceHex(data []byte, old, new string) []byte {
	return bytes.ReplaceAll(data, []byte(old), []byte(new))
}

// ---- green-dero.svg dispatch ----

func newAccountIconResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroOuterDerotopia)
		out = replaceHex(out, deroInnerEngram, deroInnerDerotopia)
		return &fyne.StaticResource{StaticName: "green_dero_derotopia.svg", StaticContent: out}
	case apptheme.ThemeElDorado:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroOuterElDorado)
		out = replaceHex(out, deroInnerEngram, deroInnerElDorado)
		return &fyne.StaticResource{StaticName: "green_dero_eldorado.svg", StaticContent: out}
	case apptheme.ThemeCrystallina:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroOuterCrystallina)
		out = replaceHex(out, deroInnerEngram, deroInnerCrystallina)
		return &fyne.StaticResource{StaticName: "green_dero_crystallina.svg", StaticContent: out}
	case apptheme.ThemeAtlantis:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroOuterAtlantis)
		out = replaceHex(out, deroInnerEngram, deroInnerAtlantis)
		return &fyne.StaticResource{StaticName: "green_dero_atlantis.svg", StaticContent: out}
	default:
		return &fyne.StaticResource{StaticName: "green_dero_engram.svg", StaticContent: greenDeroSvgData}
	}
}

func newAccountIconHoverResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroHoverOuter)
		out = replaceHex(out, deroInnerEngram, deroInnerDerotopia)
		return &fyne.StaticResource{StaticName: "green_dero_derotopia_hover.svg", StaticContent: out}
	case apptheme.ThemeElDorado:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroHoverOuter)
		out = replaceHex(out, deroInnerEngram, deroInnerElDorado)
		return &fyne.StaticResource{StaticName: "green_dero_eldorado_hover.svg", StaticContent: out}
	case apptheme.ThemeCrystallina:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroHoverOuterCryst)
		out = replaceHex(out, deroInnerEngram, deroInnerCrystallina)
		return &fyne.StaticResource{StaticName: "green_dero_crystallina_hover.svg", StaticContent: out}
	case apptheme.ThemeAtlantis:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroHoverOuter)
		out = replaceHex(out, deroInnerEngram, deroInnerAtlantis)
		return &fyne.StaticResource{StaticName: "green_dero_atlantis_hover.svg", StaticContent: out}
	default:
		out := replaceHex(greenDeroSvgData, deroOuterEngram, deroHoverOuter)
		return &fyne.StaticResource{StaticName: "green_dero_engram_hover.svg", StaticContent: out}
	}
}

// ---- green-wallet.svg dispatch ----

func recoverAccountIconResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletMainDerotopia)
		out = replaceHex(out, walletDarkEngram, walletDarkDerotopia)
		return &fyne.StaticResource{StaticName: "green_wallet_derotopia.svg", StaticContent: out}
	case apptheme.ThemeElDorado:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletMainElDorado)
		out = replaceHex(out, walletDarkEngram, walletDarkElDorado)
		return &fyne.StaticResource{StaticName: "green_wallet_eldorado.svg", StaticContent: out}
	case apptheme.ThemeCrystallina:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletMainCrystallina)
		out = replaceHex(out, walletDarkEngram, walletDarkCrystallina)
		return &fyne.StaticResource{StaticName: "green_wallet_crystallina.svg", StaticContent: out}
	case apptheme.ThemeAtlantis:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletMainAtlantis)
		out = replaceHex(out, walletDarkEngram, walletDarkAtlantis)
		return &fyne.StaticResource{StaticName: "green_wallet_atlantis.svg", StaticContent: out}
	default:
		return &fyne.StaticResource{StaticName: "green_wallet_engram.svg", StaticContent: greenWalletSvgData}
	}
}

func recoverAccountIconHoverResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletHoverMain)
		out = replaceHex(out, walletDarkEngram, walletDarkDerotopia)
		return &fyne.StaticResource{StaticName: "green_wallet_derotopia_hover.svg", StaticContent: out}
	case apptheme.ThemeElDorado:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletHoverMain)
		out = replaceHex(out, walletDarkEngram, walletDarkElDorado)
		return &fyne.StaticResource{StaticName: "green_wallet_eldorado_hover.svg", StaticContent: out}
	case apptheme.ThemeCrystallina:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletHoverMainCryst)
		out = replaceHex(out, walletDarkEngram, walletDarkCrystallina)
		return &fyne.StaticResource{StaticName: "green_wallet_crystallina_hover.svg", StaticContent: out}
	case apptheme.ThemeAtlantis:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletHoverMain)
		out = replaceHex(out, walletDarkEngram, walletDarkAtlantis)
		return &fyne.StaticResource{StaticName: "green_wallet_atlantis_hover.svg", StaticContent: out}
	default:
		out := replaceHex(greenWalletSvgData, walletMainEngram, walletHoverMain)
		return &fyne.StaticResource{StaticName: "green_wallet_engram_hover.svg", StaticContent: out}
	}
}
