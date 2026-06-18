package main

import (
	"fyne.io/fyne/v2"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

// ---- Filled star icons (theme accent colour) ----

var resourceFavsEngramSvg = &fyne.StaticResource{
	StaticName:    "favs_engram.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="#13CA69" stroke="none"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsDerotopiaSvg = &fyne.StaticResource{
	StaticName:    "favs_derotopia.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="#8A2BE2" stroke="none"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsElDoradoSvg = &fyne.StaticResource{
	StaticName:    "favs_eldorado.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="#FFD700" stroke="none"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsCrystallinaSvg = &fyne.StaticResource{
	StaticName:    "favs_crystallina.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="#7C5CBF" stroke="none"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

// ---- Outline star icons (theme accent colour) ----

var resourceFavsOutlineEngramSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_engram.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#13CA69" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsOutlineDerotopiaSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_derotopia.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#8A2BE2" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsOutlineElDoradoSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_eldorado.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#FFD700" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsOutlineCrystallinaSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_crystallina.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#7C5CBF" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

// ---- Muted outline star icons ----

var resourceFavsOutlineMutedSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_muted.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#7E8591" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

var resourceFavsOutlineCrystallinaMutedSvg = &fyne.StaticResource{
	StaticName:    "favs_outline_crystallina_muted.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#A898C8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12,2 15,9 22,9 16.5,14 18.5,21 12,17 5.5,21 7.5,14 2,9 9,9"/></svg>`),
}

// ---- Dispatch functions ----

func favsResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		return resourceFavsDerotopiaSvg
	case apptheme.ThemeElDorado:
		return resourceFavsElDoradoSvg
	case apptheme.ThemeCrystallina:
		return resourceFavsCrystallinaSvg
	default:
		return resourceFavsEngramSvg
	}
}

func favsOutlineResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		return resourceFavsOutlineDerotopiaSvg
	case apptheme.ThemeElDorado:
		return resourceFavsOutlineElDoradoSvg
	case apptheme.ThemeCrystallina:
		return resourceFavsOutlineCrystallinaSvg
	default:
		return resourceFavsOutlineEngramSvg
	}
}

func favsOutlineMutedResource() fyne.Resource {
	if apptheme.ThemeMode == apptheme.ThemeCrystallina {
		return resourceFavsOutlineCrystallinaMutedSvg
	}
	return resourceFavsOutlineMutedSvg
}
