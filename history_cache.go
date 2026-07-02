package main

import (
	"sync"
	"time"

	"github.com/deroproject/derohe/cryptography/crypto"
)

// historyCache holds the formatted history rows (newest first) for the wallet
// that is currently open, keyed per view ("Normal"/"Coinbase"/"Messages"). It
// is a paint accelerator only: every history visit still runs a full fetch and
// replaces the cached rows, so chain reorgs self-heal on the next refresh.
// RAM only; dropped on wallet close. Row slices are treated as immutable by
// all callers — never mutate a slice handed to put or returned by get.
type historyCache struct {
	sync.Mutex
	addr  string              // wallet address the cached views belong to
	views map[string][]string // view name -> rows
}

var histCache historyCache

// get returns the cached rows for addr's view. A different address is always
// a miss, so one wallet can never paint another wallet's history.
func (c *historyCache) get(addr, view string) ([]string, bool) {
	c.Lock()
	defer c.Unlock()
	if addr == "" || addr != c.addr {
		return nil, false
	}
	rows, ok := c.views[view]
	return rows, ok
}

// put stores rows for addr's view. A new address evicts every view of the
// previous wallet. nil rows are a valid entry: an empty history still counts
// as a hit, so empty wallets paint instantly too.
func (c *historyCache) put(addr, view string, rows []string) {
	if addr == "" {
		return
	}
	c.Lock()
	defer c.Unlock()
	if addr != c.addr || c.views == nil {
		c.addr = addr
		c.views = make(map[string][]string)
	}
	c.views[view] = rows
}

// clear drops all cached rows (privacy + memory on wallet close).
func (c *historyCache) clear() {
	c.Lock()
	defer c.Unlock()
	c.addr = ""
	c.views = nil
}

// warmHistoryCache pre-builds the Normal-view rows shortly after login so the
// first History visit paints from cache. It runs one full Show_Transfers scan
// (which takes the wallet's exclusive lock), deferred a few seconds to stay
// clear of the login-time sync/save burst. Only the default view is warmed;
// Coinbase/Messages fill on their first visit. The "Normal" literal must match
// the label layoutHistory passes to load (see menu.OnChanged in layouts.go).
func warmHistoryCache() {
	time.Sleep(3 * time.Second)
	w := engram.Disk
	if w == nil {
		return
	}
	addr := w.GetAddress().String()
	var zeroscid crypto.Hash
	entries := w.Show_Transfers(zeroscid, false, true, true, 0, w.Get_Height(), "", "", 0, 0)
	rows := streamHistoryRows(entries, historyRowNormal, 0, nil)
	if engram.Disk != w { // wallet closed or switched while we scanned
		return
	}
	histCache.put(addr, "Normal", rows)
}
