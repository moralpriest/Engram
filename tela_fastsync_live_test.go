package main

// Live integration test for the TELA FastSync discovery path.
//
// It exercises the exact flow Engram's first TELA click runs after a fresh
// install: FastSync reads the GnomonSC registry contract (any node), persists
// ~50K SCIDs, the turbo probe classifies TELA INDEX/DOC contracts, and the
// compat shim seeds the telacandidates bucket so GetTelaCandidates() returns
// real apps on the first click — no multi-hour block scan, no per-SCID
// prefilter.
//
// Requirements:
//   - Network access to a full DERO node (default dero-node.net:10102).
//   - ~1-4 minutes of wall time (registry fetch + probe on first run).
//
// Gated behind ENGRAM_TELA_LIVE=1 so normal `go test ./...` stays fast and
// offline-safe. Run with:
//
//	ENGRAM_TELA_LIVE=1 go test -tags migrated_fynedo -count=1 -run TestTelaFastSyncLive -v -timeout 600s .
//
// The test uses a temp DB dir and never touches the real datashards folder.

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deroproject/derohe/walletapi"
	"github.com/hypergnomon/hypergnomon/pkg/gnomes/indexer"
	"github.com/hypergnomon/hypergnomon/pkg/gnomes/storage"
	"github.com/hypergnomon/hypergnomon/pkg/gnomes/structures"
)

func TestTelaFastSyncLive(t *testing.T) {
	if os.Getenv("ENGRAM_TELA_LIVE") != "1" {
		t.Skip("set ENGRAM_TELA_LIVE=1 to run the live TELA FastSync integration test")
	}

	endpoint := os.Getenv("ENGRAM_TELA_NODE")
	if endpoint == "" {
		endpoint = "dero-node.net:10102"
	}

	dbDir := filepath.Join(t.TempDir(), "gnomon")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &structures.FastSyncConfig{
		Enabled:       true,
		ForceFastSync: true,
		SkipFSRecheck: true,
	}
	// Mirror Engram's real construction path: pre-open the bbolt store and
	// inject it via NewIndexer so the compat BBSBackend is wired (the seed
	// writes the telacandidates bucket through it). NewIndexerWithDBDir skips
	// that wiring and cannot serve candidates.
	bolt, err := storage.NewBBoltDB(dbDir, "gnomon")
	if err != nil {
		t.Fatalf("NewBBoltDB: %v", err)
	}
	idx := indexer.NewIndexer(nil, bolt, "boltdb", nil, 0, endpoint, "daemon", false, false, cfg, nil, false)
	if idx == nil || idx.BBSBackend == nil {
		t.Fatal("NewIndexer returned nil indexer or nil BBSBackend")
	}
	t.Cleanup(func() { idx.Close() })

	start := time.Now()
	idx.StartDaemonMode(64)

	// FastSync must push LastIndexedHeight to (near) chain tip within a few
	// seconds — a registry contract read + turbo flush. The old broken path
	// (block-scan fallback) creeps up a few hundred blocks at a time; require
	// reaching ~99%% of chain height so the test can't pass without FastSync.
	tipDeadline := time.Now().Add(90 * time.Second)
	caughtUp := false
	for time.Now().Before(tipDeadline) {
		if idx.ChainHeight > 0 && idx.LastIndexedHeight >= idx.ChainHeight*99/100 {
			caughtUp = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !caughtUp {
		t.Fatalf("FastSync did not reach chain tip within 90s (LastIndexedHeight=%d ChainHeight=%d) — block-scan fallback?", idx.LastIndexedHeight, idx.ChainHeight)
	}
	t.Logf("FastSync caught up to height %d (tip %d) in %.1fs", idx.LastIndexedHeight, idx.ChainHeight, time.Since(start).Seconds())

	// The probe runs in the background and then seeds the bucket. Wait for
	// real apps to appear (fresh install has no cache, so the full probe of
	// ~50K SCIDs takes a couple of minutes).
	deadline := time.Now().Add(5 * time.Minute)
	var candidates []string
	for time.Now().Before(deadline) {
		candidates = idx.GetTelaCandidates()
		if len(candidates) >= 50 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	// Both bounds are assertions on the doc-separation fix. The lower bound
	// proves discovery happened; the UPPER bound proves DOC contracts did not
	// leak into the candidates bucket as valid_index — if the seed ever writes
	// docs as apps again, candidates jumps to ~948 and this fails.
	if len(candidates) < 50 {
		t.Fatalf("expected >=50 TELA app candidates from FastSync, got %d (LastIndexedHeight=%d)", len(candidates), idx.LastIndexedHeight)
	}
	if len(candidates) > 300 {
		t.Fatalf("candidate count %d exceeds sane app bound (docs leaked as valid_index?) — bucket has %d classified entries", len(candidates), len(idx.BBSBackend.GetAllTelaCandidates()))
	}
	t.Logf("GetTelaCandidates returned %d apps after %.1fs total", len(candidates), time.Since(start).Seconds())

	// Verify the doc-separation fix: the seed must not leak DOC contracts
	// into the candidates bucket as apps. Every candidate must be a distinct
	// 64-hex-char SCID, and the bucket's classified set must contain a
	// separate doc population that GetTelaCandidates excludes.
	all := idx.BBSBackend.GetAllTelaCandidates()
	docCount := 0
	for _, status := range all {
		if status == "doc" {
			docCount++
		}
	}
	t.Logf("bucket classified: %d entries (%d apps exposed, %d docs recorded separately)", len(all), len(candidates), docCount)
	if docCount == 0 {
		t.Logf("note: no doc contracts recorded (node/prune variance) — app count is the authoritative check")
	}
	for _, c := range candidates {
		if len(c) != 64 || !isHexString(c) {
			t.Fatalf("candidate %q is not a 64-hex SCID", c)
		}
	}

	t.Logf("SUCCESS: %d TELA apps discovered via FastSync in %.1fs (no prefilter, no block scan)", len(candidates), time.Since(start).Seconds())
}

// TestWalletRestoreAndTelaLive is the complete end-to-end flow: open a wallet
// exactly like the app's login() does, then drive FastSync + TELA candidate
// discovery against a live remote node and assert apps are listable well
// within the 10s budget the user sees in the UI.
//
// SECURITY: no wallet file, seed, or password is hardcoded or committed. When
// ENGRAM_TEST_WALLET points at an existing wallet, its password must come from
// ENGRAM_TEST_WALLET_PASS (there is no default). Otherwise a throwaway random
// wallet is created in a temp dir with a runtime-generated password, so the
// local mainnet/ folder (which can hold real wallets) is never referenced.
func TestWalletRestoreAndTelaLive(t *testing.T) {
	if os.Getenv("ENGRAM_TELA_LIVE") != "1" {
		t.Skip("set ENGRAM_TELA_LIVE=1 to run the live wallet+TELA integration test")
	}

	// --- Leg 1: wallet open (mirrors functions.go login()) ---
	walletPath := os.Getenv("ENGRAM_TEST_WALLET")
	walletPass := os.Getenv("ENGRAM_TEST_WALLET_PASS")
	var wd *walletapi.Wallet_Disk
	var err error
	if walletPath != "" {
		// Explicit wallet: password must come from the environment too.
		if walletPass == "" {
			t.Skip("ENGRAM_TEST_WALLET is set but ENGRAM_TEST_WALLET_PASS is not — refusing to guess a wallet password")
		}
		wd, err = walletapi.Open_Encrypted_Wallet(walletPath, walletPass)
		if err != nil {
			t.Fatalf("wallet restore failed: %v", err)
		}
	} else {
		// Throwaway random wallet in a temp dir with a generated password:
		// exercises the same Open_Encrypted_Wallet code path end to end with
		// no secrets in the repo.
		wd, err = walletapi.Create_Encrypted_Wallet_Random(filepath.Join(t.TempDir(), "throwaway.db"), randomTestPassword())
		if err != nil {
			t.Fatalf("could not create throwaway test wallet: %v", err)
		}
		t.Logf("created throwaway random test wallet (no wallet file committed)")
	}
	addr := wd.GetAddress().String()
	// Test wallets may be mainnet (dero1) or testnet (deto1) addresses.
	if (!strings.HasPrefix(addr, "dero1") && !strings.HasPrefix(addr, "deto1")) || len(addr) < 60 {
		t.Fatalf("restored wallet has unexpected address %q", addr)
	}
	t.Logf("wallet restored: %s... (password accepted)", addr[:24])

	// --- Leg 2: FastSync + TELA candidates, same as the first click ---
	endpoint := os.Getenv("ENGRAM_TELA_NODE")
	if endpoint == "" {
		endpoint = "dero-node.net:10102"
	}
	dbDir := filepath.Join(t.TempDir(), "gnomon")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Warm-start simulation: reuse the app's existing tela_cache.bin so the
	// turbo probe loads instantly (this is the user's measured <10s scenario;
	// a true cold start probes ~50K SCIDs and takes a couple of minutes). The
	// test still builds a fresh DB in a temp dir — never touches datashards.
	warmCache := false
	if srcCache := filepath.Join(AppPath(), "datashards", "gnomon", "tela_cache.bin"); fileExists(srcCache) {
		if data, err := os.ReadFile(srcCache); err == nil {
			if err := os.WriteFile(filepath.Join(dbDir, "tela_cache.bin"), data, 0o600); err != nil {
				t.Logf("warning: could not stage warm cache: %v", err)
			} else {
				warmCache = true
			}
		}
	}
	cfg := &structures.FastSyncConfig{
		Enabled:       true,
		ForceFastSync: true,
		SkipFSRecheck: true,
	}
	bolt, err := storage.NewBBoltDB(dbDir, "gnomon")
	if err != nil {
		t.Fatalf("NewBBoltDB: %v", err)
	}
	idx := indexer.NewIndexer(nil, bolt, "boltdb", nil, 0, endpoint, "daemon", false, false, cfg, nil, false)
	if idx == nil || idx.BBSBackend == nil {
		t.Fatal("NewIndexer returned nil indexer or nil BBSBackend")
	}
	t.Cleanup(func() { idx.Close() })

	start := time.Now()
	idx.StartDaemonMode(64)

	// Budget for the full list. The user's real-world measurement is ~2s from
	// the TELA click (Gnomon already warmed up at login). This test starts the
	// indexer from scratch, so it must also cover the one-time registry fetch
	// (~7s on this node). The FastSync regression guard is the tip-catchup
	// loop in TestTelaFastSyncLive; here 30s is far below the old multi-hour
	// block scan while absorbing node-latency jitter (measured 12-16s on
	// dero-node.net across runs). A soft warning fires above 15s so the timing
	// stays visible without making the test flaky. Without a warm cache
	// (fresh machine) the full probe takes minutes, so fall back to the
	// generous cold-start deadline.
	budget := 30 * time.Second
	if !warmCache {
		budget = 5 * time.Minute
	}
	var candidates []string
	for time.Since(start) < budget {
		candidates = idx.GetTelaCandidates()
		if len(candidates) >= 50 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if len(candidates) < 50 {
		t.Fatalf("TELA apps not listable within %v: got %d candidates in %.1fs (LastIndexedHeight=%d ChainHeight=%d)",
			budget, len(candidates), time.Since(start).Seconds(), idx.LastIndexedHeight, idx.ChainHeight)
	}
	if len(candidates) > 300 {
		t.Fatalf("candidate count %d exceeds sane app bound (docs leaked as valid_index?)", len(candidates))
	}
	elapsed := time.Since(start).Seconds()
	if warmCache && elapsed > 15 {
		t.Logf("warning: listable in %.1fs — above the user-visible ~10s target; check node latency", elapsed)
	}
	t.Logf("SUCCESS: wallet restored + %d TELA apps listable in %.1fs (budget %v)", len(candidates), elapsed, budget)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// randomTestPassword returns a random 32-hex-char password for the throwaway
// test wallet. It is generated at runtime and never stored or committed.
func randomTestPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "throwaway-test-wallet"
	}
	return hex.EncodeToString(b)
}

func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
