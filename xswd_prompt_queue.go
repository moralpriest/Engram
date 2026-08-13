package main

import (
	"sync"

	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/deroproject/derohe/walletapi/xswd"
)

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
func queuedAskPermissionForRequest(ad *xswd.ApplicationData, request *jrpc2.Request) xswd.Permission {
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
