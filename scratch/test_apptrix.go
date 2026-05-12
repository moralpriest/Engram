//go:build ignore

package main

import (
	"fmt"

	"apptrix.org/components/widget/webview"
)

func main() {
	wv := webview.NewWebView()
	fmt.Printf("WebView: %v\n", wv)
}
