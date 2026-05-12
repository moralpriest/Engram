//go:build !android && !ios

// Copyright 2023-2026 DERO Foundation. All rights reserved.
package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"github.com/civilware/tela/logger"
)

// openTELAApp is the unified entry point for launching TELA apps across platforms.
// On desktop, it opens the system browser.
func openTELAApp(scid, link, durl string) {
	link = cleanTELALink(link)
	logger.Printf("[TELA] Opening URL in system browser: %s\n", link)
	if u, err := url.Parse(link); err == nil {
		if err := fyne.CurrentApp().OpenURL(u); err != nil {
			logger.Errorf("[TELA] OpenURL error: %s\n", err)
		}
	}
}

// hideTELAWebViewForPermission is a no-op on desktop (no embedded WebView).
func hideTELAWebViewForPermission() bool {
	return false
}

// restoreTELAWebViewAfterPermission is a no-op on desktop (no embedded WebView).
func restoreTELAWebViewAfterPermission() {
}
