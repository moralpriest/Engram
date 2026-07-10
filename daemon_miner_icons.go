package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	apptheme "github.com/DEROFDN/engram/internal/theme"
)

type coloredIcon struct {
	resource fyne.Resource
	tint     color.Color
}

var (
	daemonColoredMu sync.Mutex
	daemonColored   = make(map[int]*coloredIcon)
	minerColoredMu  sync.Mutex
	minerColored    = make(map[int]*coloredIcon)
)

//go:embed assets/DaemonON.svg
var daemonSvgData []byte

//go:embed assets/MinerON.svg
var minerSvgData []byte

//go:embed assets/MinerOFF.png
var minerOffPngData []byte

//go:embed assets/green-cyberdeck.svg
var cyberdeckSvgData []byte

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

// daemonIconColor returns the tint color for the daemon icon based on state.
// Syncing and connecting use green instead of yellow so the pulse animation
// breathes in the theme's active color rather than the warning color.
func daemonIconColor(state int) color.Color {
	if state == dmStateSyncing || state == dmStateConnecting {
		return apptheme.C.Green
	}
	return stateColorDM(state)
}

func daemonIconForState(state int) fyne.Resource {
	tint := daemonIconColor(state)
	daemonColoredMu.Lock()
	defer daemonColoredMu.Unlock()
	if entry, ok := daemonColored[state]; ok && colorsEqual(entry.tint, tint) {
		return entry.resource
	}
	res := daemonSvgForColor(tint)
	daemonColored[state] = &coloredIcon{resource: res, tint: tint}
	return res
}

func minerIconForState(state int) fyne.Resource {
	tint := stateColorDM(state)
	minerColoredMu.Lock()
	defer minerColoredMu.Unlock()
	if entry, ok := minerColored[state]; ok && colorsEqual(entry.tint, tint) {
		return entry.resource
	}

	src, _, err := image.Decode(bytes.NewReader(minerOffPngData))
	if err != nil {
		return fyne.NewStaticResource("miner.png", minerOffPngData)
	}

	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)

	tr, tg, tb, _ := tint.RGBA()
	tnr := float64(tr) / 65535.0
	tng := float64(tg) / 65535.0
	tnb := float64(tb) / 65535.0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			na := uint8(a / 257)
			if na > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{
					R: uint8(tnr * 255),
					G: uint8(tng * 255),
					B: uint8(tnb * 255),
					A: na,
				})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return fyne.NewStaticResource("miner.png", minerOffPngData)
	}

	res := fyne.NewStaticResource("miner_tinted.png", buf.Bytes())
	minerColored[state] = &coloredIcon{resource: res, tint: tint}
	return res
}

// daemonSvgForColor builds a tinted daemon SVG by replacing "currentColor"
// in the source with the hex representation of tint. This avoids
// canvas.RecolorSVG (which relies on XML marshal/unmarshal of the SVG dom
// and can silently fail) and matches the approach used by cyberdeckIconResource.
func daemonSvgForColor(tint color.Color) fyne.Resource {
	r, g, b, _ := tint.RGBA()
	hexStr := fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
	data := strings.ReplaceAll(string(daemonSvgData), "currentColor", hexStr)
	return &fyne.StaticResource{
		StaticName:    "daemon_tinted.svg",
		StaticContent: []byte(data),
	}
}

func colorsEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// clearDaemonColoredCache empties the daemon icon tint cache so the next
// call to daemonIconForState re-tints the SVG with the current theme colors.
// Must be called when the theme changes.
func clearDaemonColoredCache() {
	daemonColoredMu.Lock()
	daemonColored = make(map[int]*coloredIcon)
	daemonColoredMu.Unlock()
}

// clearMinerColoredCache empties the miner icon tint cache so the next
// call to minerIconForState re-tints with the current theme colors.
func clearMinerColoredCache() {
	minerColoredMu.Lock()
	minerColored = make(map[int]*coloredIcon)
	minerColoredMu.Unlock()
}

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

const (
	cyberdeckHoverMain      = "#ffffff"
	cyberdeckHoverMainCryst = "#38384a"
)

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
