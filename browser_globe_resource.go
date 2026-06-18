package main

import (
	"fyne.io/fyne/v2"
	apptheme "github.com/DEROFDN/engram/internal/theme"
)

// ---- Globe icons (TELA Apps tab) ----

var resourceBrowserGlobeEngramSvg = &fyne.StaticResource{
	StaticName:    "browser_globe_engram.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#13CA69" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`),
}

var resourceBrowserGlobeDerotopiaSvg = &fyne.StaticResource{
	StaticName:    "browser_globe_derotopia.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#8A2BE2" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`),
}

var resourceBrowserGlobeElDoradoSvg = &fyne.StaticResource{
	StaticName:    "browser_globe_eldorado.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#FFD700" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`),
}

var resourceBrowserGlobeCrystallinaSvg = &fyne.StaticResource{
	StaticName:    "browser_globe_crystallina.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#7C5CBF" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`),
}

// ---- History icons (TELA History tab, Fyne Material Design path) ----

var resourceBrowserHistoryEngramSvg = &fyne.StaticResource{
	StaticName:    "browser_history_engram.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#13CA69" width="24px" height="24px"><path d="M0 0h24v24H0z" fill="none"/><path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>`),
}

var resourceBrowserHistoryDerotopiaSvg = &fyne.StaticResource{
	StaticName:    "browser_history_derotopia.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#8A2BE2" width="24px" height="24px"><path d="M0 0h24v24H0z" fill="none"/><path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>`),
}

var resourceBrowserHistoryElDoradoSvg = &fyne.StaticResource{
	StaticName:    "browser_history_eldorado.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#FFD700" width="24px" height="24px"><path d="M0 0h24v24H0z" fill="none"/><path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>`),
}

var resourceBrowserHistoryCrystallinaSvg = &fyne.StaticResource{
	StaticName:    "browser_history_crystallina.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#7C5CBF" width="24px" height="24px"><path d="M0 0h24v24H0z" fill="none"/><path d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>`),
}

// ---- Dispatch functions ----

func globeResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		return resourceBrowserGlobeDerotopiaSvg
	case apptheme.ThemeElDorado:
		return resourceBrowserGlobeElDoradoSvg
	case apptheme.ThemeCrystallina:
		return resourceBrowserGlobeCrystallinaSvg
	default:
		return resourceBrowserGlobeEngramSvg
	}
}

func historyResource() fyne.Resource {
	switch apptheme.ThemeMode {
	case apptheme.ThemeDerotopia:
		return resourceBrowserHistoryDerotopiaSvg
	case apptheme.ThemeElDorado:
		return resourceBrowserHistoryElDoradoSvg
	case apptheme.ThemeCrystallina:
		return resourceBrowserHistoryCrystallinaSvg
	default:
		return resourceBrowserHistoryEngramSvg
	}
}
