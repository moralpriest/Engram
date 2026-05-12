//go:build android || ios

// Copyright 2023-2026 DERO Foundation. All rights reserved.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"image/color"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	apptrixwebview "apptrix.org/components/widget/webview"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	tela "github.com/civilware/tela"
	"github.com/civilware/tela/logger"
)

var (
	activeTELAMobileWebView   fyne.CanvasObject
	activeTELAMobileWebViewMu sync.Mutex
	webViewMinimized          bool
	proxyStarted              bool
	proxyPort                 = "44443"
	proxyTarget               string
	proxyMu                   sync.Mutex
	activeSCID                string
	activeDURL                string
	activeLinksMu             sync.Mutex
	activeLinks               = make(map[string]string) // SCID -> link
)

func generateSelfSignedCert(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Engram TELA Proxy"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return err
	}

	return nil
}

// setProxyTarget updates the backend target for the HTTPS proxy.
// The proxy reads this value on each request, so it's safe to call at any time.
func setProxyTarget(target string) {
	proxyMu.Lock()
	proxyTarget = target
	proxyMu.Unlock()
	logger.Printf("[WebView] Proxy target set to: %s\n", target)
}

// getXSWDBackend returns the WebSocket (XSWD) backend address from the global config.
func getXSWDBackend() string {
	port := remoteAccess.WS.port
	if port == "" {
		return "127.0.0.1:44326"
	}
	host, portNum, err := net.SplitHostPort(port)
	if err != nil {
		return "127.0.0.1:44326"
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, portNum)
}

// handleWebSocketProxy hijacks a WebSocket upgrade request and proxies it to the XSWD backend.
func handleWebSocketProxy(w http.ResponseWriter, r *http.Request) {
	backend := getXSWDBackend()
	logger.Printf("[WebView] Proxying WebSocket to XSWD at %s\n", backend)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		logger.Errorf("[WebView] WebSocket hijack failed: %v\n", err)
		return
	}
	defer clientConn.Close()

	backendConn, err := net.DialTimeout("tcp", backend, 10*time.Second)
	if err != nil {
		logger.Errorf("[WebView] WebSocket backend dial failed: %v\n", err)
		return
	}
	defer backendConn.Close()

	// Rewrite the Host header to match the backend before forwarding
	r.Host = backend
	if err := r.Write(backendConn); err != nil {
		logger.Errorf("[WebView] WebSocket write upgrade request failed: %v\n", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(backendConn, clientConn)
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientConn, backendConn)
	}()
	wg.Wait()
	logger.Printf("[WebView] WebSocket proxy closed\n")
}

// startInternalHTTPSProxy starts a local HTTPS server that proxies to the TELA HTTP server
// and routes WebSocket connections to the XSWD backend.
// This is necessary because Android blocks cleartext (HTTP) to 127.0.0.1 in the WebView
// if not explicitly allowed in the manifest, but we can't easily modify the manifest.
// Uses its own mutex (proxyMu) so callers need not hold any lock.
func startInternalHTTPSProxy() {
	proxyMu.Lock()
	if proxyStarted {
		proxyMu.Unlock()
		return
	}
	proxyStarted = true
	proxyMu.Unlock()

	// Use a Director func so the target can be updated dynamically per-request
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			proxyMu.Lock()
			t := proxyTarget
			proxyMu.Unlock()
			if t == "" {
				return
			}
			targetURL, err := url.Parse(t)
			if err != nil {
				return
			}
			r.URL.Scheme = targetURL.Scheme
			r.URL.Host = targetURL.Host
			r.Host = targetURL.Host
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		ModifyResponse: func(r *http.Response) error {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "text/html") {
				return nil
			}
			if r.Header.Get("Content-Encoding") != "" {
				return nil
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return err
			}
			r.Body.Close()

			injection := `<script>(function(){var OWS=window.WebSocket;window.WebSocket=function(u,p){if(typeof u==='string'&&u.startsWith('ws://127.0.0.1')){u='wss://127.0.0.1:44443'+u.replace(/^ws:\/\/127\.0\.0\.1(:\d+)?/,'');}return new OWS(u,p);};})();</script>`

			bodyStr := string(body)
			newBody := strings.Replace(bodyStr, "</head>", injection+"</head>", 1)
			if newBody == bodyStr {
				newBody = strings.Replace(bodyStr, "</body>", injection+"</body>", 1)
			}
			if newBody == bodyStr {
				newBody = bodyStr + injection
			}

			r.Body = io.NopCloser(strings.NewReader(newBody))
			r.ContentLength = int64(len(newBody))
			r.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
			return nil
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			handleWebSocketProxy(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	go func() {
		storageRoot := fyne.CurrentApp().Storage().RootURI().Path()
		certPath := filepath.Join(storageRoot, "proxy.crt")
		keyPath := filepath.Join(storageRoot, "proxy.key")

		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			logger.Printf("[WebView] Generating self-signed cert for proxy...\n")
			if err := generateSelfSignedCert(certPath, keyPath); err != nil {
				logger.Errorf("[WebView] Failed to generate cert: %v\n", err)
				return
			}
		}

		logger.Printf("[WebView] Starting internal HTTPS proxy on :%s\n", proxyPort)
		err := http.ListenAndServeTLS(":"+proxyPort, certPath, keyPath, handler)
		if err != nil {
			logger.Errorf("[WebView] Proxy error: %v\n", err)
		}
	}()
}

// showTELAMobileWebView displays a full-screen WebView overlay for Android/iOS
func showTELAMobileWebView(scid, urlStr, durl string) {
	activeTELAMobileWebViewMu.Lock()
	defer activeTELAMobileWebViewMu.Unlock()

	if session.Window == nil {
		logger.Warnf("[WebView] session.Window is nil, cannot show WebView\n")
		return
	}

	// If already showing one, remove it first to avoid stacks
	if activeTELAMobileWebView != nil {
		fyne.Do(func() {
			session.Window.Canvas().Overlays().Remove(activeTELAMobileWebView)
			protectedOverlays.Delete(activeTELAMobileWebView)
			apptrixwebview.DestroyNativeWebView()
		})
		activeTELAMobileWebView = nil
	}

	webViewMinimized = false

	wv, errF := apptrixwebview.New(session.Window)
	if errF != nil {
		logger.Errorf("[WebView] Failed to create WebView: %v\n", errF)
		return
	}

	parsedURL, err := url.Parse(urlStr)
	if err == nil {
		wv.Load(parsedURL)
	}

	// Close button — fully shuts down the TELA app, destroys the WebView,
	// and returns to the Fyne UI.
	closeBtn := widget.NewButtonWithIcon("", theme.WindowCloseIcon(), func() {
		closeTELAMobileWebView()
	})
	closeBtn.Importance = widget.LowImportance

	// Minimize button — hides the native WebView window and removes the
	// Fyne overlay, but keeps the TELA server and proxy running.
	// The user can restore via the "Return to TELA App" button on the
	// TELA app inspection page.
	minBtn := widget.NewButtonWithIcon("", theme.WindowMinimizeIcon(), func() {
		minimizeTELAMobileWebView()
	})
	minBtn.Importance = widget.LowImportance

	// Switcher button (Grid) — shows the TELA App Switcher
	switcherBtn := widget.NewButtonWithIcon("", theme.GridIcon(), func() {
		showTELASwitcher()
	})
	switcherBtn.Importance = widget.LowImportance

	titleStr := durl
	if scid == "986fc20fefeda2227e5722af66390c57f3606468a485215f773326aa872697c8" || durl == "villager.tela" || durl == "[HASH]" {
		titleStr = "Villager"
	}

	if titleStr == "" || titleStr == "[HASH]" {
		if scid != "" && !strings.Contains(scid, "[HASH]") {
			titleStr = scid
		} else if parsedURL != nil && !strings.Contains(parsedURL.Host, "127.0.0.1") {
			titleStr = parsedURL.Host
		} else {
			titleStr = "TELA App"
		}
	}

	title := canvas.NewText(titleStr, color.White)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(14)

	header := container.NewHBox(
		closeBtn,
		minBtn,
		layout.NewSpacer(),
		title,
		layout.NewSpacer(),
		switcherBtn,
	)

	// Background rectangle for the header - make it taller and ensure it's on top
	headerHeight := scaleSize(50)
	headerBG := canvas.NewRectangle(color.NRGBA{20, 20, 25, 255})
	headerBG.SetMinSize(fyne.NewSize(0, headerHeight))

	headerStack := container.NewStack(headerBG, header)

	// Main container background
	bg := canvas.NewRectangle(color.Black)

	// Use a Border layout with a top spacer to force the WebView down
	// This ensures the native WebView component doesn't overlap our controls
	content := container.NewBorder(headerStack, nil, nil, nil, wv)

	// iframe forces the overlay to full-screen size (required since Fyne doesn't
	// auto-resize non-OverlayContainer overlays added via Overlays().Add())
	overlay := container.NewStack(&iframe{}, bg, content)

	activeTELAMobileWebView = overlay
	// Protect from mass removal during dashboard transitions
	protectedOverlays.Store(overlay, true)

	session.Window.Canvas().Overlays().Add(overlay)
	logger.Printf("[WebView] Displaying TELA WebView for SCID %s DURL %s at %s\n", scid, durl, urlStr)
}

// minimizeTELAMobileWebView hides the WebView without shutting down the app.
// The native Android WebView is hidden (View.GONE), the Fyne overlay is
// removed, but the reference is kept so the user can restore via
// "Return to TELA App" on the inspection page.
func minimizeTELAMobileWebView() {
	activeTELAMobileWebViewMu.Lock()
	defer activeTELAMobileWebViewMu.Unlock()
	if activeTELAMobileWebView != nil && session.Window != nil {
		apptrixwebview.HideNativeWebView()
		fyne.Do(func() {
			session.Window.Canvas().Overlays().Remove(activeTELAMobileWebView)
			session.Window.Canvas().Refresh(session.Window.Canvas().Content())
		})
		webViewMinimized = true
		logger.Printf("[WebView] TELA WebView minimized\n")
	}
}

// closeTELAMobileWebView destroys the WebView and shuts down the app.
// Both the Fyne overlay and the native Android WebView are removed.
func closeTELAMobileWebView() {
	activeTELAMobileWebViewMu.Lock()
	scid := activeSCID
	if activeTELAMobileWebView != nil && session.Window != nil {
		apptrixwebview.DestroyNativeWebView()
		fyne.Do(func() {
			session.Window.Canvas().Overlays().Remove(activeTELAMobileWebView)
			protectedOverlays.Delete(activeTELAMobileWebView)
			session.Window.Canvas().Refresh(session.Window.Canvas().Content())
		})
		activeTELAMobileWebView = nil
		activeSCID = ""
		webViewMinimized = false
	}
	activeTELAMobileWebViewMu.Unlock()

	activeLinksMu.Lock()
	delete(activeLinks, scid)
	activeLinksMu.Unlock()

	tela.ShutdownServer(scid)
}

// showTELASwitcher displays a 2x2 grid of all currently running TELA apps
func showTELASwitcher() {
	activeTELAMobileWebViewMu.Lock()
	currentSCID := activeSCID
	currentDURL := activeDURL
	activeTELAMobileWebViewMu.Unlock()

	// Hide current WebView if any
	minimizeTELAMobileWebView()

	var overlay fyne.CanvasObject
	uiDo(func() {
		headerText := canvas.NewText("ACTIVE TELA APPS", color.White)
		headerText.TextStyle = fyne.TextStyle{Bold: true}
		headerText.TextSize = scaleFont(18)

		header := container.NewCenter(headerText)
		headerBG := canvas.NewRectangle(color.NRGBA{20, 20, 25, 255})
		headerBG.SetMinSize(fyne.NewSize(0, scaleSize(60)))
		headerStack := container.NewStack(headerBG, header)

		// Get all active servers
		activeServers := getTelaActiveServers()
		grid := container.New(layout.NewGridWrapLayout(fyne.NewSize(ui.Width/2-scaleSize(20), ui.Width/2-scaleSize(20))))

		for _, s := range activeServers {
			scid := s.SCID
			durl := s.Name // Use s.Name as the display URL/name

			activeLinksMu.Lock()
			link := activeLinks[scid]
			activeLinksMu.Unlock()

			// If link is missing (e.g. after reload), use the address/port
			if link == "" && s.Address != "" {
				link = "http://127.0.0.1:" + s.Address
			}

			btnTitle := durl
			if scid == "986fc20fefeda2227e5722af66390c57f3606468a485215f773326aa872697c8" || scid == "villager" {
				btnTitle = "Villager"
			}
			if btnTitle == "" {
				btnTitle = scid
				if len(btnTitle) > 10 {
					btnTitle = btnTitle[:8] + "..."
				}
			}

			// Individual app card
			icon := canvas.NewImageFromResource(theme.ComputerIcon())
			icon.SetMinSize(fyne.NewSize(scaleSize(64), scaleSize(64)))
			icon.FillMode = canvas.ImageFillContain

			nameLabel := canvas.NewText(btnTitle, color.White)
			nameLabel.Alignment = fyne.TextAlignCenter
			nameLabel.TextSize = scaleFont(12)

			cardContent := container.NewVBox(
				layout.NewSpacer(),
				container.NewCenter(icon),
				nameLabel,
				layout.NewSpacer(),
			)

			bg := canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 40, A: 255})
			bg.CornerRadius = scaleSize(8)

			if scid == currentSCID {
				bg.StrokeColor = colors.Green
				bg.StrokeWidth = 2
			}

			appBtn := container.NewStack(bg, cardContent,
				widget.NewButton("", func() {
					session.Window.Canvas().Overlays().Remove(overlay)
					openTELAApp(scid, link, durl)
				}),
			)

			grid.Add(appBtn)
		}

		scroll := container.NewVScroll(grid)

		backBtn := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
			session.Window.Canvas().Overlays().Remove(overlay)
			if currentSCID != "" {
				activeLinksMu.Lock()
				link := activeLinks[currentSCID]
				activeLinksMu.Unlock()
				openTELAApp(currentSCID, link, currentDURL)
			}
		})
		backBtn.Importance = widget.MediumImportance

		bottomBar := container.NewHBox(layout.NewSpacer(), container.NewGridWrap(fyne.NewSize(scaleSize(120), scaleSize(40)), backBtn), layout.NewSpacer())
		bottomBarStack := container.NewStack(
			canvas.NewRectangle(colors.DarkMatter),
			container.NewPadded(bottomBar),
		)

		bg := canvas.NewRectangle(color.Black)
		content := container.NewBorder(headerStack, bottomBarStack, nil, nil, container.NewPadded(scroll))

		overlay = container.NewStack(&iframe{}, bg, content)
		protectedOverlays.Store(overlay, true)
		session.Window.Canvas().Overlays().Add(overlay)
	})
}

// IsTELAMobileWebViewActive returns true if the mobile webview is currently created
func IsTELAMobileWebViewActive() bool {
	activeTELAMobileWebViewMu.Lock()
	defer activeTELAMobileWebViewMu.Unlock()
	return activeTELAMobileWebView != nil
}

// hideTELAWebViewForPermission hides the native WebView so that Fyne overlays
// (permission prompts) are visible. Returns true if the WebView was visible
// and was hidden. The caller should call restoreTELAWebViewAfterPermission
// when the overlay is dismissed.
func hideTELAWebViewForPermission() bool {
	if !isMobile() {
		return false
	}
	activeTELAMobileWebViewMu.Lock()
	defer activeTELAMobileWebViewMu.Unlock()
	if activeTELAMobileWebView == nil {
		return false
	}
	apptrixwebview.HideNativeWebView()
	fyne.Do(func() {
		session.Window.Canvas().Overlays().Remove(activeTELAMobileWebView)
	})
	logger.Printf("[WebView] Hidden for permission overlay\n")
	return !webViewMinimized
}

// restoreTELAWebViewAfterPermission re-shows the native WebView if it was
// explicitly hidden by hideTELAWebViewForPermission. If the WebView was
// already hidden (minimized), it stays hidden.
func restoreTELAWebViewAfterPermission() {
	activeTELAMobileWebViewMu.Lock()
	defer activeTELAMobileWebViewMu.Unlock()
	if activeTELAMobileWebView != nil && !webViewMinimized {
		fyne.Do(func() {
			session.Window.Canvas().Overlays().Add(activeTELAMobileWebView)
			apptrixwebview.ShowNativeWebView()
		})
		logger.Printf("[WebView] Restored after permission overlay\n")
	}
}

// openTELAApp is the unified entry point for launching TELA apps across platforms.
// On mobile, it uses the internal WebView; on desktop, it opens the system browser.
func openTELAApp(scid, link, durl string) {
	link = cleanTELALink(link)
	if durl == "" && scid == "986fc20fefeda2227e5722af66390c57f3606468a485215f773326aa872697c8" {
		durl = "villager.tela"
	}

	activeLinksMu.Lock()
	activeLinks[scid] = link
	activeLinksMu.Unlock()

	if isMobile() {
		activeTELAMobileWebViewMu.Lock()
		if activeTELAMobileWebView != nil {
			if activeSCID == scid {
				if session.Window == nil {
					activeTELAMobileWebViewMu.Unlock()
					logger.Warnf("[TELA] session.Window is nil, cannot restore WebView\n")
					return
				}
				// Restore from standby — re-add the Fyne overlay and re-show the
				// native Android WebView (it was hidden via View.GONE on minimize)
				fyne.Do(func() {
					session.Window.Canvas().Overlays().Add(activeTELAMobileWebView)
					apptrixwebview.ShowNativeWebView()
				})
				webViewMinimized = false
				activeTELAMobileWebViewMu.Unlock()
				logger.Printf("[TELA] Restored TELA WebView from standby for SCID %s\n", scid)
				return
			}
			// Different SCID — destroy old WebView and create a new one
			if session.Window != nil {
				fyne.Do(func() {
					session.Window.Canvas().Overlays().Remove(activeTELAMobileWebView)
					protectedOverlays.Delete(activeTELAMobileWebView)
					apptrixwebview.DestroyNativeWebView()
				})
			}
			activeTELAMobileWebView = nil
			activeSCID = ""
			webViewMinimized = false
		}
		activeTELAMobileWebViewMu.Unlock()

		// Validate the link before showing the WebView
		if link == "" {
			logger.Errorf("[TELA] Empty link for SCID %s, aborting WebView launch\n", scid)
			return
		}

		// Start the HTTPS proxy and set the backend target from the HTTP link
		if parsedLink, err := url.Parse(link); err == nil && parsedLink.Host != "" {
			proxyTargetURL := fmt.Sprintf("http://%s", parsedLink.Host)
			setProxyTarget(proxyTargetURL)
			startInternalHTTPSProxy()
			rewritten := fmt.Sprintf("https://127.0.0.1:%s", proxyPort)
			if parsedLink.Path != "" {
				rewritten += parsedLink.Path
			}
			if parsedLink.RawQuery != "" {
				rewritten += "?" + parsedLink.RawQuery
			}
			link = rewritten
			logger.Printf("[TELA] Rewrote URL to %s (proxy target %s)\n", link, proxyTargetURL)
		} else {
			logger.Errorf("[TELA] Invalid link %q for SCID %s, aborting WebView launch\n", link, scid)
			return
		}

		logger.Printf("[TELA] Mobile launching internal WebView for SCID %s: %s\n", scid, link)
		showTELAMobileWebView(scid, link, durl)
	} else {
		logger.Printf("[TELA] Opening URL in system browser: %s\n", link)
		if u, err := url.Parse(link); err == nil {
			err = fyne.CurrentApp().OpenURL(u)
			if err != nil {
				logger.Errorf("[TELA] OpenURL error: %s\n", err)
			}
		}
	}
}
