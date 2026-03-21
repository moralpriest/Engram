package main

import "fyne.io/fyne/v2"

var resourceHeartOutlineSvg = &fyne.StaticResource{
	StaticName:    "heart_outline.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19.84 4.61a5.5 5.5 0 0 0-7.78 0L12 4.67l-.06-.06a5.5 5.5 0 0 0-7.78 7.78l.06.06L12 20.23l7.78-7.78.06-.06a5.5 5.5 0 0 0 0-7.78z"/></svg>`),
}

var resourceHeartOutlineMutedSvg = &fyne.StaticResource{
	StaticName:    "heart_outline_muted.svg",
	StaticContent: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#7E8591" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19.84 4.61a5.5 5.5 0 0 0-7.78 0L12 4.67l-.06-.06a5.5 5.5 0 0 0-7.78 7.78l.06.06L12 20.23l7.78-7.78.06-.06a5.5 5.5 0 0 0 0-7.78z"/></svg>`),
}
