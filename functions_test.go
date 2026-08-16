package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/DEROFDN/engram/i18n"
	"github.com/civilware/tela"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
)

func TestDecodeHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "valid lowercase hex", in: "68656c6c6f", want: "hello"},
		{name: "valid mixed hex", in: "48656C6C6F", want: "Hello"},
		{name: "invalid hex returns original", in: "not-hex", want: "not-hex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeHex(tt.in)
			if got != tt.want {
				t.Fatalf("decodeHex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRegistrationPoWSolved guards the raised registration PoW target: Engram
// must search for 28 leading zero bits (not the historical 24), so a winner
// with only three zero bytes but a set high nibble on byte 3 is rejected.
func TestRegistrationPoWSolved(t *testing.T) {
	hash := func(b0, b1, b2, b3 byte) crypto.Hash {
		var h crypto.Hash
		h[0] = b0
		h[1] = b1
		h[2] = b2
		h[3] = b3
		return h
	}

	tests := []struct {
		name string
		h    crypto.Hash
		want bool
	}{
		{name: "28-bit winner", h: hash(0, 0, 0, 0x0F), want: true},
		{name: "all zero", h: hash(0, 0, 0, 0), want: true},
		{name: "only 24 bits (high nibble set)", h: hash(0, 0, 0, 0x10), want: false},
		{name: "third byte set", h: hash(0, 0, 1, 0), want: false},
		{name: "first byte set", h: hash(1, 0, 0, 0), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registrationPoWSolved(tt.h); got != tt.want {
				t.Fatalf("registrationPoWSolved(%x) = %v, want %v", tt.h, got, tt.want)
			}
		})
	}
}

// TestRegistrationETA guards the countdown number shown on the registration
// screen: it is 2x the memoryless mean (2^bits/hashRate), i.e. the upper bound
// of the typical window that ~86%% of runs finish by, and must be zero for a
// non-positive hashrate.
func TestRegistrationETA(t *testing.T) {
	mean := float64(uint64(1)<<registrationPoWLeadingZeroBits) / 1e6

	tests := []struct {
		name     string
		hashRate float64
		want     float64
	}{
		{name: "1 MH/s", hashRate: 1e6, want: 2.0 * mean},
		{name: "double speed halves ETA", hashRate: 2e6, want: 2.0 * mean / 2},
		{name: "zero hashrate", hashRate: 0, want: 0},
		{name: "negative hashrate", hashRate: -5, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registrationETA(tt.hashRate)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Fatalf("registrationETA(%v) = %v, want %v", tt.hashRate, got, tt.want)
			}
		})
	}
}

// TestRateWindowConvergence guards the countdown's hashrate estimator: the
// first seconds of mining are slow while workers spin up, and on a fast
// machine the ETA must converge to the true steady-state rate within a few
// seconds rather than staying dragged down by the warm-up (the old 20s window
// kept the slow start in view for most of a sub-minute run). Simulates a
// machine that ramps from 0.5M to 6M h/s over the first 3s, ticking every
// 0.5s like the UI loop.
func TestRateWindowConvergence(t *testing.T) {
	const (
		windowSeconds = 6.0
		minSpanSeconds = 2.0
	)

	var w rateWindow
	attempts := int64(0)
	elapsed := 0.0
	lastRate := 0.0

	step := func(rate float64) {
		attempts += int64(rate * 0.5)
		elapsed += 0.5
		lastRate = w.rate(attempts, elapsed, windowSeconds, minSpanSeconds)
	}

	// Warm-up: 0.5M h/s for the first 3 seconds.
	for elapsed < 3.0 {
		step(0.5e6)
	}
	// Steady state: 6M h/s for 12 more seconds.
	for i := 0; i < 24; i++ {
		step(6e6)
	}

	// The warm-up must have fully left the 6s window, so the estimate should
	// be back at (or very near) the steady-state rate.
	if lastRate < 5.5e6 {
		t.Fatalf("rate window did not converge to steady-state 6M h/s: got %.0f h/s", lastRate)
	}
}

// TestRateWindowFallback guards the pre-window fallback: before the window
// spans minSpanSeconds it reports the lifetime average (attempts/elapsed), so
// the countdown always has a sane value from the first tick.
func TestRateWindowFallback(t *testing.T) {
	var w rateWindow

	// Two samples 0.5s apart, so the window spans only 0.5s < minSpan 2.0.
	first := w.rate(100, 0.5, 6.0, 2.0)
	if got := w.rate(200, 1.0, 6.0, 2.0); got != 200.0/1.0 {
		t.Fatalf("expected lifetime-average fallback %v, got %v", 200.0/1.0, got)
	}
	if first <= 0 {
		t.Fatalf("first sample rate must be positive, got %v", first)
	}
}

// TestPulseTokenLifecycle guards the pulse-token fix: a freshly started pulse
// must run under a non-zero token. On a fresh login pulseToken starts at 0, and
// isPulseTokenActive rejects 0 — before the fix the pulse loop exited
// immediately and flipped daemonConnected back to false, bouncing the login
// background goroutine back to the auth screen ("logs me out" + frozen yellow
// sync indicator).
func TestPulseTokenLifecycle(t *testing.T) {
	// Save and stub the global wallet-session state the pulse guards read.
	walletSessionMu.Lock()
	prevDisk := engram.Disk
	prevOpen := session.WalletOpen
	prevToken := pulseToken
	prevRunning := pulseRunning
	prevGen := pulseSessionGeneration
	prevWalletGen := walletSessionGeneration
	engram.Disk = &walletapi.Wallet_Disk{}
	session.WalletOpen = true
	pulseToken = 0
	pulseRunning = false
	pulseSessionGeneration = 0
	walletSessionGeneration = 0
	walletSessionMu.Unlock()
	defer func() {
		walletSessionMu.Lock()
		engram.Disk = prevDisk
		session.WalletOpen = prevOpen
		pulseToken = prevToken
		pulseRunning = prevRunning
		pulseSessionGeneration = prevGen
		walletSessionGeneration = prevWalletGen
		walletSessionMu.Unlock()
	}()

	if !startPulseForActiveWallet() {
		t.Fatal("startPulseForActiveWallet should succeed with a fresh pulse")
	}
	tok := currentPulseToken()
	if tok == 0 {
		t.Fatal("pulse token must be non-zero for a freshly started pulse (isPulseTokenActive rejects 0)")
	}
	if !isPulseTokenActive(tok) {
		t.Fatalf("isPulseTokenActive(%d) should be true for the running pulse", tok)
	}

	// A second StartPulse while one is running must be guarded.
	if startPulseForActiveWallet() {
		t.Fatal("second start while running should be rejected")
	}

	// Finishing allows a restart with a fresh, higher token.
	finishPulseForGeneration(tok)
	if pulseRunning {
		t.Fatal("pulseRunning should be false after finishPulseForGeneration")
	}
	if !startPulseForActiveWallet() {
		t.Fatal("restart after finish should succeed")
	}
	tok2 := currentPulseToken()
	if tok2 <= tok {
		t.Fatalf("expected new token %d > old token %d", tok2, tok)
	}

	// bumpPulseGeneration (foreground) must invalidate the running pulse.
	bumpPulseGeneration()
	if isPulseTokenActive(tok2) {
		t.Fatal("old token should be invalidated by bumpPulseGeneration")
	}

	// A stale finish from a previous generation must not clear a newer pulse.
	if !startPulseForActiveWallet() {
		t.Fatal("restart after bump should succeed")
	}
	tok3 := currentPulseToken()
	finishPulseForGeneration(tok2) // stale generation
	if !pulseRunning || pulseSessionGeneration != tok3 {
		t.Fatal("stale finishPulseForGeneration must not clear the newer pulse")
	}
	finishPulseForGeneration(tok3)
	if pulseRunning {
		t.Fatal("finish with the current generation should clear the pulse")
	}

	// beginWalletShutdown must invalidate a running pulse.
	if !startPulseForActiveWallet() {
		t.Fatal("start before shutdown should succeed")
	}
	tok4 := currentPulseToken()
	if !beginWalletShutdown() {
		t.Fatal("beginWalletShutdown should succeed with an open wallet")
	}
	if isPulseTokenActive(tok4) {
		t.Fatal("running pulse should be invalidated by beginWalletShutdown")
	}
}

func TestBatchPrefilterTelaVersionsGuards(t *testing.T) {
	t.Run("empty scid list returns empty result", func(t *testing.T) {
		passed, stats, err := batchPrefilterTelaVersions(context.Background(), nil, 1000, 3, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(passed) != 0 {
			t.Fatalf("expected empty passed map, got %d items", len(passed))
		}
		if stats.VersionHits != 0 || stats.Dropped != 0 || stats.Retries != 0 {
			t.Fatalf("expected zero stats, got %+v", stats)
		}
	})

	t.Run("empty rpc pool errors", func(t *testing.T) {
		_, _, err := batchPrefilterTelaVersions(context.Background(), []string{"abcd"}, 1000, 3, nil, nil)
		if err == nil {
			t.Fatal("expected error for empty rpc pool")
		}
		if err.Error() != "rpc pool is empty" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBatchFetchINDEXesEmpty(t *testing.T) {
	fetched, ratings, invalid, err := batchFetchINDEXes(context.Background(), nil, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fetched) != 0 {
		t.Fatalf("expected empty fetched map, got %d items", len(fetched))
	}
	if len(ratings) != 0 {
		t.Fatalf("expected empty ratings map, got %d items", len(ratings))
	}
	if len(invalid) != 0 {
		t.Fatalf("expected empty invalid map, got %d items", len(invalid))
	}
}

func TestTelaCandidateCacheHelpers(t *testing.T) {
	cache := telaCandidateCache{}
	cache.set("valid-b", telaCandidateValidIndex, 22)
	cache.set("not-tela", telaCandidateNotTela, 22)
	cache.set("invalid", telaCandidateInvalidIndex, 22)
	cache.set("no-docs", telaCandidateNoDocs, 22)
	cache.set("excluded", telaCandidateExcludedByURL, 22)

	valid := cache.validSCIDs()
	if len(valid) != 1 || valid[0] != "valid-b" {
		t.Fatalf("unexpected valid SCIDs: %#v", valid)
	}

	negative := cache.negativeSet()
	for _, scid := range []string{"not-tela", "invalid", "no-docs"} {
		if !negative[scid] {
			t.Fatalf("expected %q in negative set", scid)
		}
	}
	if negative["valid-b"] {
		t.Fatal("did not expect valid candidate in negative set")
	}
	if negative["excluded"] {
		t.Fatal("did not expect settings-dependent exclusion in negative set")
	}
	if meta := cache["valid-b"]; meta.LastCheckedHeight != 22 || meta.Result != telaCandidateValidIndex {
		t.Fatalf("unexpected metadata stored: %+v", meta)
	}
}

func TestBuildINDEXFromVarsErrors(t *testing.T) {
	t.Run("missing C fails", func(t *testing.T) {
		_, err := buildINDEXFromVars("scid", map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error when C is missing")
		}
	})

	t.Run("missing dURL fails", func(t *testing.T) {
		_, err := buildINDEXFromVars("scid", map[string]interface{}{"C": "invalidhex"})
		if err == nil {
			t.Fatal("expected error when dURL is missing")
		}
	})
}

func TestParseTelaListEntry(t *testing.T) {
	tests := []struct {
		input string
		name  string
		scid  string
	}{
		{"Name;;;scid123", "Name", "scid123"},
		{"OnlyName", "OnlyName", ""},
		{";;;scid456", "", "scid456"},
		{"", "", ""},
	}

	for _, tt := range tests {
		n, s := parseTelaListEntry(tt.input)
		if n != tt.name || s != tt.scid {
			t.Errorf("parseTelaListEntry(%q) = %q, %q; want %q, %q", tt.input, n, s, tt.name, tt.scid)
		}
	}
}

func TestNormalizeTelaSearch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello  ", "hello"},
		{"WORLD", "world"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := normalizeTelaSearch(tt.input); got != tt.expected {
			t.Errorf("normalizeTelaSearch(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTelaHexagonColor(t *testing.T) {
	if got := telaHexagonColor(tela.Rating_Result{Average: 7.0}); got != resourceTelaHexagonGreen {
		t.Error("expected green for 7.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 5.0}); got != resourceTelaHexagonYellow {
		t.Error("expected yellow for 5.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 2.0}); got != resourceTelaHexagonRed {
		t.Error("expected red for 2.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 0.0, Likes: 0, Dislikes: 1}); got != resourceTelaHexagonRed {
		t.Error("expected red for unrated app with dislikes")
	}
}

func TestSessionDomainToString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"app.wallet", "Dashboard"},
		{"wallet", "Dashboard"},
		{"app.explorer", "Asset Explorer"},
		{"app.tela", "TELA"},
		{"tela.manager", "TELA"},
		{"app.send", "Send"},
	}

	for _, tt := range tests {
		if got := sessionDomainToString(tt.input); got != tt.expected {
			t.Errorf("sessionDomainToString(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNotificationI18nKeysExist(t *testing.T) {
	keys := []string{
		"notification.send_success",
		"notification.send_failed",
		"notification.incoming",
		"settings.enable_notifications",
		"settings.notifications_desc",
	}
	for _, key := range keys {
		got := i18n.T(key)
		if got == key {
			t.Errorf("i18n key %q not found (returned raw key)", key)
		}
		if got == "" {
			t.Errorf("i18n key %q returned empty string", key)
		}
	}
}

func TestFormatNotificationIncoming(t *testing.T) {
	tmpl := i18n.T("notification.incoming")
	got := fmt.Sprintf(tmpl, "1.50000")
	if got == tmpl {
		t.Error("expected formatted string, got raw template")
	}
	if len(got) == 0 {
		t.Error("expected non-empty formatted string")
	}
}

func TestFormatNotificationSendSuccess(t *testing.T) {
	got := i18n.T("notification.send_success")
	if got == "" || got == "notification.send_success" {
		t.Error("notification.send_success key missing or empty")
	}
}

func TestFormatNotificationSendFailed(t *testing.T) {
	got := i18n.T("notification.send_failed")
	if got == "" || got == "notification.send_failed" {
		t.Error("notification.send_failed key missing or empty")
	}
}

func TestWaitForHistoryRefreshAndSync(t *testing.T) {
	t.Run("returns data when not running", func(t *testing.T) {
		// Reset state
		historyRefreshState.Lock()
		historyRefreshState.running = false
		historyRefreshState.Unlock()

		// Call the function - should not block and should return
		transfers, normalRows, coinbaseRows, messageRows := waitForHistoryRefreshAndSync()
		_ = transfers
		_ = normalRows
		_ = coinbaseRows
		_ = messageRows
		// If we reach here, the function returned (didn't deadlock)
	})

	t.Run("blocks when running and unblocks when done", func(t *testing.T) {
		// Set running to true
		historyRefreshState.Lock()
		historyRefreshState.running = true
		historyRefreshState.Unlock()

		done := make(chan struct{})
		var resultTransfers []rpc.Entry

		go func() {
			transfers, _, _, _ := waitForHistoryRefreshAndSync()
			resultTransfers = transfers
			close(done)
		}()

		// Give goroutine time to start waiting
		time.Sleep(50 * time.Millisecond)

		// Release the lock
		historyRefreshState.Lock()
		historyRefreshState.running = false
		historyRefreshState.Unlock()

		select {
		case <-done:
			// Success - goroutine unblocked and returned
			_ = resultTransfers
		case <-time.After(5 * time.Second):
			t.Fatal("waitForHistoryRefreshAndSync did not unblock after running became false")
		}
	})

	t.Run("concurrent calls serialize correctly", func(t *testing.T) {
		historyRefreshState.Lock()
		historyRefreshState.running = false
		historyRefreshState.Unlock()

		var wg sync.WaitGroup
		const numGoroutines = 10
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				waitForHistoryRefreshAndSync()
			}()
		}

		wg.Wait()
		// All goroutines completed without deadlock
	})
}
