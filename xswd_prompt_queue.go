package main

import (
	"strings"
	"sync"
	"time"

	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/deroproject/derohe/walletapi/xswd"
)

// xswdAppCacheKey returns the identity used for XSWD approval caches.
//
// For browser clients the xswd library validates ad.Url against the HTTP
// Origin header (rejects the handshake when both are present and differ), so
// ad.Url is a spoof-resistant cache key: each TELA app is served from its own
// localhost port, giving every app a distinct origin. ad.Name and ad.Id are
// both self-declared handshake JSON and must never key a cache alone — any
// page open in the user's browser could otherwise declare a trusted app's
// name and silently inherit its recent approvals (the xswd server accepts
// connections from any origin).
//
// Clients that send no Origin header keep whatever Url they declared, so this
// fallback is weaker; it is namespaced so it can never collide with a real
// origin key.
func xswdAppCacheKey(ad *xswd.ApplicationData) string {
	if ad.Url != "" {
		return "origin:" + ad.Url
	}
	return "name:" + ad.Name
}

// isSafeAutoAllowMethod reports whether a method is low-risk read-only and may
// be auto-allowed for a grace period after the user approved the connection.
// Sensitive methods (signing, key access, transactions, transfer history) must
// never appear here; they always get an explicit per-method prompt via
// isSensitiveAutoAllowExcluded.
func isSafeAutoAllowMethod(method string) bool {
	switch method {
	case "GetAddress", "getaddress", "GetAddressEPOCH",
		"GetBalance", "getbalance",
		"GetHeight", "getheight",
		"GetDaemon", "GetPrimaryUsername",
		"MakeIntegratedAddress", "make_integrated_address", "SplitIntegratedAddress", "split_integrated_address",
		"HasMethod", "Echo", "GetSessionEPOCH", "GetMaxHashesEPOCH", "HandleTELALinks", "Subscribe", "Unsubscribe":
		return true
	}
	return false
}

// isSensitiveAutoAllowExcluded reports whether a method must never be
// batch-allowed by a connection approval alone. These always require an
// explicit per-method prompt, even when every other Ask method was
// auto-allowed at connect time. This covers fund movement, signing/key
// access, transfer-history reads, and EPOCH mining triggers (which would
// otherwise let a freshly-connected app start mining without consent).
func isSensitiveAutoAllowExcluded(method string) bool {
	switch method {
	case "Transfer", "transfer", "transfer_split", "scinvoke",
		"SignData", "CheckSignature",
		"QueryKey", "query_key",
		"GetTransfers", "get_transfers", "GetTransferbyTXID", "get_transfer_by_txid", "get-transfer_by_txid",
		"AttemptEPOCH", "AttemptEPOCHWithAddr", "SubmitEPOCH":
		return true
	}
	return false
}

// xswdPromptQueue serializes XSWD connection/permission prompts so they are
// presented strictly one-at-a-time. XSWDPrompt and AskPermissionForRequest each
// push two overlay layers and remove them by position; when several permissions
// fire in quick succession (common on mobile), positional removal can pop the
// *other* dialog's widgets, leaking a dark backdrop that swallows every touch.
// The FIFO queue guarantees one prompt's layers are fully removed before the
// next prompt's layers are added.
var (
	xswdPromptMu    sync.Mutex
	xswdPromptQueue []func()
	xswdPromptBusy  bool
)

// enqueueXSWDPrompt runs fn strictly one-at-a-time in FIFO order. fn runs on a
// background goroutine; it must do its own fyne.Do for UI work. Safe to call
// from any goroutine (the XSWD server calls prompt handlers from its own
// goroutines).
func enqueueXSWDPrompt(fn func()) {
	xswdPromptMu.Lock()
	xswdPromptQueue = append(xswdPromptQueue, fn)
	if xswdPromptBusy {
		xswdPromptMu.Unlock()
		return
	}
	xswdPromptBusy = true
	xswdPromptMu.Unlock()
	go runNextXSWDPrompt()
}

func runNextXSWDPrompt() {
	for {
		xswdPromptMu.Lock()
		if len(xswdPromptQueue) == 0 {
			xswdPromptBusy = false
			xswdPromptMu.Unlock()
			return
		}
		fn := xswdPromptQueue[0]
		xswdPromptQueue = xswdPromptQueue[1:]
		xswdPromptMu.Unlock()

		// A panicking prompt must never wedge the queue: recover so the next
		// prompt still runs (the specific waiter is unblocked by the queued
		// wrapper's own recover below).
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[XSWD] prompt handler panicked: %v", r)
				}
			}()
			fn()
		}()
	}
}

// queuedXSWDPrompt is the connection prompt handler wired into the XSWD server;
// it routes through the FIFO queue so prompts never stack their overlay layers.
func queuedXSWDPrompt(ad *xswd.ApplicationData) (confirmed bool) {
	result := make(chan bool, 1)
	enqueueXSWDPrompt(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Printf("[XSWD] XSWDPrompt panicked: %v", r)
				result <- false
			}
		}()
		result <- XSWDPrompt(ad)
	})
	return <-result
}

// queuedAskPermissionForRequest is the per-method permission prompt handler
// wired into the XSWD server; it routes through the FIFO queue.
// Fast-path: if the method was already Allow-cached (batch-allow at connection
// or prior Allow) or is safe-auto-allowed via recent connection approval,
// return immediately without queuing a dialog. This restores 0.6.9 fluidity
// where Villager's burst appeared "one after other without delays" instead
// of one app-switch per permission on mobile.
func queuedAskPermissionForRequest(ad *xswd.ApplicationData, request *jrpc2.Request) xswd.Permission {
	method := request.Method()
	// Stored AlwaysAllow / AlwaysDeny – no prompt needed.
	if ad.Permissions != nil {
		if p, ok := ad.Permissions[method]; ok && p != xswd.Ask {
			return p
		}
	}
	// Gnomon always-allow – no prompt.
	if strings.HasPrefix(method, "Gnomon.") {
		return xswd.Allow
	}
	// Method Allow cache (10m) from prior Allow tap.
	appKey := xswdAppCacheKey(ad)
	xswdMethodAllowCacheMu.Lock()
	if ts, ok := xswdMethodAllowCache[appKey+"|"+method]; ok && len(method) > 0 {
		if time.Since(ts) < 10*time.Minute {
			xswdMethodAllowCacheMu.Unlock()
			return xswd.Allow
		}
	}
	xswdMethodAllowCacheMu.Unlock()
	// Safe methods auto-allowed for 10m after connection approval (Villager burst).
	if isSafeAutoAllowMethod(method) {
		xswdApprovedCacheMu.Lock()
		if ts, ok := xswdApprovedCache[appKey]; ok && time.Since(ts) < 10*time.Minute {
			xswdApprovedCacheMu.Unlock()
			return xswd.Allow
		}
		xswdApprovedCacheMu.Unlock()
	}

	result := make(chan xswd.Permission, 1)
	enqueueXSWDPrompt(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Printf("[XSWD] AskPermissionForRequest panicked: %v", r)
				result <- xswd.Deny
			}
		}()
		result <- AskPermissionForRequest(ad, request)
	})
	return <-result
}

// signalPromptDone is a non-blocking prompt completion signal. A prompt's
// waiter may already have returned via ad.OnClose, in which case a blocking
// send from the UI goroutine would deadlock; this makes completion idempotent.
func signalPromptDone(done chan struct{}) {
	select {
	case done <- struct{}{}:
	default:
	}
}
