//go:build ignore

package main

import (
	"fmt"

	"fyne.io/fyne/v2/app"
	"fyne.io/x/fyne/widget"
)

func main() {
	a := app.New()
	w := a.NewWindow("Test")
	wv := widget.NewWebView()
	fmt.Printf("WebView type: %T\n", wv)
	w.SetContent(wv)
	// Don't run, just compile check
}
