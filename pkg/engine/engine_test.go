// Copyright 2017-2026 DERO Project. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DEROFDN/engram/pkg/fakegetwork"
)

// logSink captures engine log lines and signals when one containing want
// arrives. The engine calls Logf from the getwork client's reader and writer
// goroutines as well as the caller's, so the mutex is load-bearing under -race.
type logSink struct {
	mu   sync.Mutex
	msgs []string
	want string
	hit  chan struct{}
	done bool
}

func newLogSink(want string) *logSink {
	return &logSink{want: want, hit: make(chan struct{})}
}

func (l *logSink) logf(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, level+" "+msg)
	if !l.done && strings.Contains(msg, l.want) {
		l.done = true
		close(l.hit)
	}
}

func (l *logSink) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.msgs...)
}

func (l *logSink) count(substr string) int {
	n := 0
	for _, m := range l.lines() {
		if strings.Contains(m, substr) {
			n++
		}
	}
	return n
}

// stoppedWorkerResidual is comfortably more than the pending hashes a worker
// can flush on its way out of the grind loop (under counterFlush = 16 each),
// so growth past it means the worker is still mining rather than winding
// down. It is sized for the single-threaded engines the tests below start; a
// test with more than four workers would need to scale it.
const stoppedWorkerResidual = 64

// waitForStats polls until ok reports true, and fails the test if it never does.
func waitForStats(t *testing.T, e *Engine, within time.Duration, what string, ok func(Stats) bool) Stats {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if s := e.Stats(); ok(s) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s; last stats = %+v", within, what, e.Stats())
	return Stats{}
}

func TestStartRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"no endpoint", Config{Wallet: "dero1qytest"}},
		{"no wallet", Config{Endpoint: "localhost:10100"}},
		{"neither", Config{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := Start(context.Background(), c.cfg)
			if err == nil {
				e.Stop()
				t.Fatal("Start succeeded; want ErrInvalidConfig")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestStartRejectsUnknownBackend(t *testing.T) {
	_, err := Start(context.Background(), Config{
		Endpoint: "localhost:10100",
		Wallet:   "dero1qytest",
		Backend:  "bogus",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("err = %v, want unknown-backend error", err)
	}
}

func TestStartBrokenHashRefusesToMine(t *testing.T) {
	// Both real backends must pass the KAT; a wrong constant simulates a
	// broken pipeline.
	old := katHash
	katHash = "0000000000000000000000000000000000000000000000000000000000000000"
	defer func() { katHash = old }()
	_, err := Start(context.Background(), Config{
		Endpoint: "localhost:10100",
		Wallet:   "dero1qytest",
		Threads:  1,
	})
	if err == nil || !errors.Is(err, ErrBrokenHash) {
		t.Fatalf("err = %v, want ErrBrokenHash", err)
	}
}

func TestNormalizeThreads(t *testing.T) {
	if got := NormalizeThreads(0); got < 1 || got > MaxThreads {
		t.Fatalf("NormalizeThreads(0) = %d, want in [1,%d]", got, MaxThreads)
	}
	if got := NormalizeThreads(MaxThreads + 100); got != MaxThreads {
		t.Fatalf("NormalizeThreads(over) = %d, want %d", got, MaxThreads)
	}
	if got := NormalizeThreads(-5); got < 1 {
		t.Fatalf("NormalizeThreads(-5) = %d, want >= 1", got)
	}
}

// TestStartStopConnectsAndStats is the happy path against a real getwork
// websocket server: the client dials, receives a valid job, workers mine it,
// and Stats reports connected/height/hashrate before Stop tears everything
// down.
func TestStartStopConnectsAndStats(t *testing.T) {
	srv := fakegetwork.Start(fakegetwork.Config{
		Jobs: []fakegetwork.Job{
			fakegetwork.ValidJob("job-0", 1000),
			fakegetwork.ValidJob("job-1", 1001),
			fakegetwork.ValidJob("job-2", 1002),
		},
		PushInterval: 50 * time.Millisecond,
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	e, err := Start(ctx, Config{
		Endpoint: srv.URL(),
		Wallet:   "dero1qytest",
		Threads:  1,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop() // an early t.Fatal must not leak a live client

	// Wait until a job is installed and hashing starts.
	st := waitForStats(t, e, 10*time.Second, "a connected engine hashing job-0",
		func(s Stats) bool { return s.Connected && s.Height >= 1000 && s.Hashes > 0 })
	if !st.Connected {
		t.Fatal("client never connected")
	}
	if st.Height < 1000 || st.Height > 1002 {
		t.Fatalf("Height = %d, want in [1000,1002]", st.Height)
	}
	if st.Hashes == 0 {
		t.Fatal("no hashes counted while mining")
	}
	if st.Running != true {
		t.Fatal("Running = false, want true")
	}
	if st.Threads != 1 {
		t.Fatalf("Threads = %d, want 1", st.Threads)
	}
	if !strings.Contains(st.Endpoint, srv.Addr()) {
		t.Fatalf("Endpoint = %q, want host %q", st.Endpoint, srv.Addr())
	}
	if st.Wallet != "dero1qytest" {
		t.Fatalf("Wallet = %q", st.Wallet)
	}

	done := make(chan struct{})
	go func() { e.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return within 5s")
	}

	// Running is derived from the goroutines, not hardcoded.
	after := e.Stats()
	if after.Running {
		t.Fatal("Running = true after Stop, want false")
	}
	// Nothing advances the window once the sampler is gone, so a nonzero
	// figure here is a stale average decaying as 1/t, not a measurement.
	if after.Hashrate != 0 {
		t.Fatalf("Hashrate = %v after Stop, want 0", after.Hashrate)
	}

	// Stop is idempotent.
	e.Stop()
}

// TestStartWithDeadContextIsNotRunning covers the other way Running can lie:
// Start on an already-cancelled context returns a non-nil engine and a nil
// error while every goroutine exits at once.
func TestStartWithDeadContextIsNotRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e, err := Start(ctx, Config{
		Endpoint: "localhost:10100",
		Wallet:   "dero1qytest",
		Threads:  1,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop()
	waitForStats(t, e, 5*time.Second, "Running to report false",
		func(s Stats) bool { return !s.Running })
}

// TestStatsPollingDoesNotDisturbHashrate is the regression test for Stats
// mutating the window it reads. A host polling at render cadence used to
// overwrite every ring slot with near-identical timestamps, after which the
// readout collapsed to 0 -- or spiked absurdly when a worker flush landed
// between two microsecond-apart slots.
func TestStatsPollingDoesNotDisturbHashrate(t *testing.T) {
	srv := fakegetwork.Start(fakegetwork.Config{})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e, err := Start(ctx, Config{
		Endpoint: srv.URL(),
		Wallet:   "dero1qytest",
		Threads:  1,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop()

	base := waitForStats(t, e, 20*time.Second, "a nonzero hashrate",
		func(s Stats) bool { return s.Hashrate > 0 }).Hashrate

	// Hammer the readout the way a 60fps UI would, and compare the burst
	// readings against each other rather than against base: the burst spans
	// well under a millisecond at any miner speed, so a window nothing
	// perturbs cannot drift across it. Spread here is the polling itself.
	var lo, hi float64
	for i := 0; i < 200; i++ {
		got := e.Stats().Hashrate
		if got <= 0 {
			t.Fatalf("Hashrate = %v on poll %d (baseline %v); polling collapsed the window", got, i, base)
		}
		if i == 0 || got < lo {
			lo = got
		}
		if i == 0 || got > hi {
			hi = got
		}
	}
	if hi > lo*2 {
		t.Fatalf("Hashrate ranged over [%v, %v] across 200 polls (baseline %v); polling perturbed the window", lo, hi, base)
	}
}

// TestPairHonorsAnExplicitOverride pins the tri-state: nil takes the platform
// default, and a non-nil false really disables pairing. A plain bool could not
// express that on arm64, so a host could not reproduce the CLI's -pair=false.
func TestPairHonorsAnExplicitOverride(t *testing.T) {
	off, on := false, true
	if got := (&Engine{cfg: Config{Pair: &off}}).pair(); got {
		t.Fatal("pair() = true for Pair=&false, want false")
	}
	if got := (&Engine{cfg: Config{Pair: &on}}).pair(); !got {
		t.Fatal("pair() = false for Pair=&true, want true")
	}
	if got := (&Engine{cfg: Config{}}).pair(); got != DefaultPair() {
		t.Fatalf("pair() = %v for Pair=nil, want DefaultPair() = %v", got, DefaultPair())
	}
}

// TestThrottledSuppressesRepeats covers the helper the rejection logging
// leans on. The CLI's twin is tested the same way in main_test.go; the copy
// in this package had no test of its own, so it could drift unnoticed.
func TestThrottledSuppressesRepeats(t *testing.T) {
	var got []string
	sink := func(level, format string, args ...interface{}) {
		got = append(got, fmt.Sprintf(format, args...))
	}
	th := throttled(time.Minute, sink)

	th("WARN", "same %d", 1)
	th("WARN", "same %d", 1)
	th("WARN", "same %d", 1)
	if len(got) != 1 {
		t.Fatalf("emitted %d lines for three identical messages, want 1: %v", len(got), got)
	}
	if got[0] != "same 1" {
		t.Fatalf("first line = %q, want %q", got[0], "same 1")
	}

	// A changed message goes out at once, carrying what was swallowed.
	th("WARN", "different")
	if len(got) != 2 {
		t.Fatalf("a changed message was suppressed: %v", got)
	}
	if !strings.Contains(got[1], "2 repeats suppressed") {
		t.Fatalf("second line = %q, want the suppressed count", got[1])
	}

	// A zero interval throttles nothing.
	got = nil
	nth := throttled(0, sink)
	nth("WARN", "x")
	nth("WARN", "x")
	if len(got) != 2 {
		t.Fatalf("interval 0 suppressed a message: %v", got)
	}
}

// TestRejectedPushesAreThrottled is the engine-level half: a daemon that keeps
// pushing keepalive frames must not put one line per frame into the host's log
// sink. Without the throttle this test sees three.
func TestRejectedPushesAreThrottled(t *testing.T) {
	srv := fakegetwork.Start(fakegetwork.Config{
		Jobs: []fakegetwork.Job{
			fakegetwork.ValidJob("job-0", 1000),
			{JobID: "ka"},
			{JobID: "ka"},
			{JobID: "ka"},
			fakegetwork.ValidJob("job-1", 1001),
		},
		PushInterval: 50 * time.Millisecond,
	})
	defer srv.Close()

	sink := newLogSink("ignored non-job frame")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e, err := Start(ctx, Config{
		Endpoint: srv.URL(),
		Wallet:   "dero1qytest",
		Threads:  1,
		Logf:     sink.logf,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop()

	// Reaching job-1 proves all three keepalives were consumed.
	waitForStats(t, e, 20*time.Second, "the job behind the keepalives",
		func(s Stats) bool { return s.Height == 1001 })

	if n := sink.count("ignored non-job frame"); n != 1 {
		t.Fatalf("logged %d rejection lines for 3 identical keepalives, want 1: %v", n, sink.lines())
	}
}

// badVersionJob is a well-formed 48-byte frame whose miniblock version nibble
// is not 1. That is the one rejection meaning "this miner cannot mine the
// current chain" rather than "that was a keepalive", so it takes the other
// branch of onJob.
func badVersionJob(id string, height uint64) fakegetwork.Job {
	blob := []byte(fakegetwork.ValidBlob())
	blob[1] = '2' // low nibble of byte 0: version 1 -> 2
	return fakegetwork.Job{
		JobID:            id,
		BlockhashingBlob: string(blob),
		Difficulty:       1000,
		Height:           height,
	}
}

// TestBadVersionPushesAreThrottled covers the sibling branch. The CLI hands
// one throttled logger to handleRejectedPush and so throttles both messages;
// throttling only the keepalive warning would leave a version-bumped daemon
// free to flood the error path at job cadence.
func TestBadVersionPushesAreThrottled(t *testing.T) {
	srv := fakegetwork.Start(fakegetwork.Config{
		Jobs: []fakegetwork.Job{
			badVersionJob("bad-0", 900),
			badVersionJob("bad-1", 901),
			badVersionJob("bad-2", 902),
			fakegetwork.ValidJob("job-1", 1001),
		},
		PushInterval: 50 * time.Millisecond,
	})
	defer srv.Close()

	sink := newLogSink("rejected job push")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e, err := Start(ctx, Config{
		Endpoint: srv.URL(),
		Wallet:   "dero1qytest",
		Threads:  1,
		Logf:     sink.logf,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop()

	// Reaching the valid job behind them proves all three were consumed.
	waitForStats(t, e, 20*time.Second, "the job behind the bad-version pushes",
		func(s Stats) bool { return s.Height == 1001 })

	if n := sink.count("rejected job push"); n != 1 {
		t.Fatalf("logged %d error lines for 3 identical bad-version pushes, want 1: %v", n, sink.lines())
	}
}

// TestKeepaliveFrameDoesNotKillMining pins the CLI rejection policy: a
// non-version rejection (short blob / empty jobid) is a keepalive/status
// frame and must NOT invalidate the last good job.
//
// The keepalive is the LAST frame on purpose. With a valid job behind it, a
// regression that invalidated on every rejection would be masked -- the next
// job simply revives the state and hashing resumes. Here nothing follows, so
// only the surviving job-0 can advance the counter.
func TestKeepaliveFrameDoesNotKillMining(t *testing.T) {
	srv := fakegetwork.Start(fakegetwork.Config{
		Jobs: []fakegetwork.Job{
			fakegetwork.ValidJob("job-0", 1000),
			{JobID: "ka"}, // keepalive frame with no blob
		},
		PushInterval: 100 * time.Millisecond,
	})
	defer srv.Close()

	sink := newLogSink("ignored non-job frame")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e, err := Start(ctx, Config{
		Endpoint: srv.URL(),
		Wallet:   "dero1qytest",
		Threads:  1,
		Logf:     sink.logf,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer e.Stop()

	// Budget: the ceilings below must sum to less than the client's 20s read
	// timeout, after which it redials and the server pushes job-0 again --
	// which would satisfy the growth check without job-0 having survived.
	waitForStats(t, e, 8*time.Second, "job-0 to be mining",
		func(s Stats) bool { return s.Connected && s.Height == 1000 && s.Hashes > 0 })

	// The old version asserted before the keepalive was ever sent; wait for
	// onJob to have actually rejected it.
	select {
	case <-sink.hit:
	case <-time.After(5 * time.Second):
		t.Fatalf("keepalive frame never reached onJob; log = %v", sink.lines())
	}

	// A single tick of growth is not enough: a worker that has just been
	// stopped still flushes its handful of pending hashes, which is indist-
	// inguishable from mining if the bar is "the counter moved at all".
	// Demand two rounds of growth well past that residual.
	before := e.Stats().Hashes
	mid := waitForStats(t, e, 3*time.Second, "hashing to continue past the keepalive",
		func(s Stats) bool { return s.Hashes > before+stoppedWorkerResidual })
	waitForStats(t, e, 3*time.Second, "hashing to still be going a round later",
		func(s Stats) bool { return s.Hashes > mid.Hashes+stoppedWorkerResidual })
}