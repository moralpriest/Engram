// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"sync"

	"fyne.io/fyne/v2"
)

// LayoutFunc is a function that creates a layout
type LayoutFunc func() fyne.CanvasObject

// NavEntry represents a navigation history entry
type NavEntry struct {
	Domain    string
	CanGoBack bool
}

// NavigationStack manages the history of screens for back navigation
type NavigationStack struct {
	mu      sync.RWMutex
	history []NavEntry
	maxSize int
}

// NewNavigationStack creates a new navigation stack
func NewNavigationStack() *NavigationStack {
	return &NavigationStack{
		history: make([]NavEntry, 0, 20),
		maxSize: 20,
	}
}

// Push adds a new entry to the navigation history
func (ns *NavigationStack) Push(domain string, canGoBack bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Don't push duplicate consecutive entries
	if len(ns.history) > 0 {
		last := ns.history[len(ns.history)-1]
		if last.Domain == domain {
			return
		}
	}

	ns.history = append(ns.history, NavEntry{
		Domain:    domain,
		CanGoBack: canGoBack,
	})

	// Trim if exceeds max size
	if len(ns.history) > ns.maxSize {
		ns.history = ns.history[1:]
	}
}

// Pop removes and returns the previous entry (current entry is removed, previous is returned)
func (ns *NavigationStack) Pop() (NavEntry, bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if len(ns.history) < 2 {
		return NavEntry{}, false // Keep at least one entry
	}

	// Check if current screen allows back navigation
	current := ns.history[len(ns.history)-1]
	if !current.CanGoBack {
		return NavEntry{}, false
	}

	// Remove current entry
	ns.history = ns.history[:len(ns.history)-1]

	// Return the new current entry (previous screen)
	return ns.history[len(ns.history)-1], true
}

// CanGoBack checks if navigation back is possible
func (ns *NavigationStack) CanGoBack() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	if len(ns.history) < 2 {
		return false
	}

	// Check if the current screen allows back navigation
	last := ns.history[len(ns.history)-1]
	return last.CanGoBack
}

// Current returns the current navigation entry
func (ns *NavigationStack) Current() (NavEntry, bool) {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	if len(ns.history) == 0 {
		return NavEntry{}, false
	}

	return ns.history[len(ns.history)-1], true
}

// Clear resets the navigation history
func (ns *NavigationStack) Clear() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	ns.history = ns.history[:0]
}

// Size returns the current stack size
func (ns *NavigationStack) Size() int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	return len(ns.history)
}

// layoutRegistry maps domain names to layout functions
// Note: Only layouts that take no parameters can be registered here
var layoutRegistry = map[string]LayoutFunc{
	"app.main":             layoutMain,
	"app.wallet":           layoutDashboard,
	"app.send":             layoutSend,
	"app.service":          layoutServiceAddress,
	"app.create":           layoutNewAccount,
	"app.restore":          layoutRestore,
	"app.explorer":         layoutAssetExplorer,
	"app.myassets":         layoutMyAssets,
	"app.transfers":        layoutTransfers,
	"app.settings":         layoutSettings,
	"app.appsettings":      layoutAppSettings,
	"app.messages":         layoutMessages,
	"app.messages.contact": layoutMessages, // Go back to messages list
	"app.remoteaccess":     layoutRemoteAccess,
	"app.register":         layoutNewAccount,
}

// getLayoutForDomain returns the layout function for a given domain
func getLayoutForDomain(domain string) LayoutFunc {
	if fn, ok := layoutRegistry[domain]; ok {
		return fn
	}
	return nil
}

// RegisterLayout registers a layout function for a domain (for dynamic registrations)
func RegisterLayout(domain string, fn LayoutFunc) {
	layoutRegistry[domain] = fn
}
