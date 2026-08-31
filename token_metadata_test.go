package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// testRPCRecorder substitutes for the real DERO.GetSC fetch. It counts how many
// RPCs run and tracks the max number of concurrently-executing RPCs so tests
// can assert the single-flight dedup and the concurrency cap in
// fetchTokenMetadata.
type testRPCRecorder struct {
	mu        sync.Mutex
	calls     int
	active    int
	maxActive int
	delay     time.Duration
}

func (r *testRPCRecorder) getSC(ctx context.Context, info TokenInfo) (tokenGetSCResult, bool) {
	r.mu.Lock()
	r.calls++
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()

	time.Sleep(r.delay) // simulate network round-trip

	r.mu.Lock()
	r.active--
	r.mu.Unlock()

	return tokenGetSCResult{code: "", vals: map[string]string{"name": "MyToken", "symbol": "MTK"}}, true
}

// resetTokenFetchTestState clears the global single-flight map + semaphore and
// wipes the in-memory token name cache so a test starts from a cold state.
func resetTokenFetchTestState() {
	tokenFetchMu.Lock()
	tokenFetchInFly = map[string]*tokenFetchCall{}
	tokenFetchMu.Unlock()
	tokenFetchSema = make(chan struct{}, tokenFetchConcurrency)
	tokenNameCache.Range(func(k, _ interface{}) bool {
		tokenNameCache.Delete(k)
		return true
	})
}

func withFakeRPC(t *testing.T, rec *testRPCRecorder) {
	t.Helper()
	old := tokenGetSC
	tokenGetSC = rec.getSC
	t.Cleanup(func() { tokenGetSC = old })
}

func TestTokenFetchSingleFlight(t *testing.T) {
	resetTokenFetchTestState()
	rec := &testRPCRecorder{delay: 20 * time.Millisecond}
	withFakeRPC(t, rec)

	const scid = "0000000000000000000000000000000000000000000000000000000000000001"

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := fetchTokenMetadata(context.Background(), scid)
			if info.Name == "" {
				t.Error("expected resolved name back from the shared in-flight call")
			}
		}()
	}
	wg.Wait()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.calls != 1 {
		t.Fatalf("single-flight: expected exactly 1 RPC for 20 concurrent same-SCID requests, got %d", rec.calls)
	}
}

func TestTokenFetchConcurrencyCap(t *testing.T) {
	resetTokenFetchTestState()
	rec := &testRPCRecorder{delay: 30 * time.Millisecond}
	withFakeRPC(t, rec)

	const n = 12 // distinct SCIDs > cap
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		scid := fmt.Sprintf("%064x", i+1)
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			fetchTokenMetadata(context.Background(), s)
		}(scid)
	}
	wg.Wait()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.calls != n {
		t.Fatalf("expected %d RPCs for %d distinct SCIDs, got %d", n, n, rec.calls)
	}
	if rec.maxActive > tokenFetchConcurrency {
		t.Fatalf("concurrency cap exceeded: maxActive=%d cap=%d", rec.maxActive, tokenFetchConcurrency)
	}
}

func TestTokenFetchConcurrentResultsStored(t *testing.T) {
	resetTokenFetchTestState()
	rec := &testRPCRecorder{delay: 15 * time.Millisecond}
	withFakeRPC(t, rec)

	// Ensure a raced duplicate returns the same metadata the leader resolved,
	// even for the caller that ends up waiting on the shared call.
	scid := "abcdef000000000000000000000000000000000000000000000000000000000001"
	var wg sync.WaitGroup
	var mu sync.Mutex
	foundName := ""
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := fetchTokenMetadata(context.Background(), scid)
			mu.Lock()
			if info.Name != "" {
				foundName = info.Name
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if foundName == "" {
		t.Fatal("expected metadata from the shared in-flight call to be visible to all concurrent callers")
	}
	if v, ok := tokenNameCache.Load(scid); !ok {
		t.Fatal("expected the resolved metadata to be stored in the name cache")
	} else if v.(TokenInfo).Name == "" {
		t.Fatal("cached metadata should have a non-empty name")
	}
}
