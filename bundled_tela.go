package main

import (
	"bytes"
	_ "embed"

	"fyne.io/fyne/v2"

	apptheme "github.com/DEROFDN/engram/internal/theme"
)

//go:embed assets/text-stage1.svg
var telaTextSvgData []byte

var telaBaseColor = []byte("#13CA69")

const (
	telaMainEngram      = "#13CA69"
	telaMainDerotopia   = "#38B6FF"
	telaMainElDorado    = "#DAA520"
	telaMainCrystallina = "#7C5CBF"
	telaMainAtlantis    = "#34A2B5"
)

func telaTextIconHoverResource() fyne.Resource {
	main := "#ffffff"
	if apptheme.ThemeMode == apptheme.ThemeCrystallina {
		main = "#38384a"
	}
	data := bytes.ReplaceAll(telaTextSvgData, telaBaseColor, []byte(main))
	return &fyne.StaticResource{StaticName: "text-stage1-hover.svg", StaticContent: data}
}

func telaTextIconResource() fyne.Resource {
	main := telaMainEngram
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		main = telaMainDerotopia
	case apptheme.ThemeElDorado:
		main = telaMainElDorado
	case apptheme.ThemeCrystallina:
		main = telaMainCrystallina
	case apptheme.ThemeAtlantis:
		main = telaMainAtlantis
	}
	data := bytes.ReplaceAll(telaTextSvgData, telaBaseColor, []byte(main))
	return &fyne.StaticResource{StaticName: "text-stage1.svg", StaticContent: data}
}
