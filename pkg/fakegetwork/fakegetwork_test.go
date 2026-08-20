// Copyright 2017-2026 DERO Project. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package fakegetwork

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DEROFDN/engram/internal/dirtybird/getwork"
)

func TestValidBlobShape(t *testing.T) {
	b, err := hex.DecodeString(validBlobHex)
	if err != nil {
		t.Fatalf("validBlobHex not hex: %v", err)
	}
	if len(b) != 48 {
		t.Fatalf("blob length = %d, want 48", len(b))
	}
	if b[0]&0xf != 1 {
		t.Fatalf("version nibble = %d, want 1 (Go miner refuses other values)", b[0]&0xf)
	}
}

func TestValidJob(t *testing.T) {
	j := ValidJob("abc", 42)
	if j.JobID != "abc" || j.Height != 42 || j.Difficulty < 2 {
		t.Fatalf("ValidJob = %+v", j)
	}
	if j.BlockhashingBlob != validBlobHex {
		t.Fatal("ValidJob does not carry ValidBlob")
	}
}

// TestServerPushesJobsAndCapturesSubmissions drives the server with a raw
// websocket client: it must receive the configured jobs in order and record a
// submission the client sends, including the OnSubmit callback.
func TestServerPushesJobsAndCapturesSubmissions(t *testing.T) {
	onSubmitCh := make(chan Submission, 1)
	srv := Start(Config{
		Jobs: []Job{
			ValidJob("job-0", 1000),
			{JobID: "ka"},
			ValidJob("job-1", 1001),
		},
		PushInterval: 10 * time.Millisecond,
		OnSubmit:     func(s Submission) { onSubmitCh <- s },
	})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	want := []string{"job-0", "ka", "job-1"}
	for i, id := range want {
		var frame getwork.Job
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if frame.JobID != id {
			t.Fatalf("frame %d JobID = %q, want %q", i, frame.JobID, id)
		}
		if id == "ka" && frame.Blockhashing_blob != "" {
			t.Fatalf("keepalive frame carried a blob: %q", frame.Blockhashing_blob)
		}
	}

	if err := conn.WriteJSON(getwork.Submit{JobID: "job-1", Blob: "deadbeef"}); err != nil {
		t.Fatalf("write submit: %v", err)
	}

	select {
	case cb := <-onSubmitCh:
		if cb.JobID != "job-1" || cb.Blob != "deadbeef" {
			t.Fatalf("OnSubmit = %+v, want job-1/deadbeef", cb)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnSubmit never fired")
	}

	subs := srv.Submissions()
	if len(subs) != 1 || subs[0].JobID != "job-1" || subs[0].Blob != "deadbeef" {
		t.Fatalf("Submissions = %+v, want one job-1/deadbeef", subs)
	}
}

// TestEmptyConfigDefaultsToOneJob pins the zero-value behavior.
func TestEmptyConfigDefaultsToOneJob(t *testing.T) {
	srv := Start(Config{})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var frame getwork.Job
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read: %v", err)
	}
	if frame.JobID != "job-0" {
		t.Fatalf("JobID = %q, want job-0", frame.JobID)
	}
	if frame.Difficultyuint64 != 1000 {
		t.Fatalf("Difficultyuint64 = %d, want 1000", frame.Difficultyuint64)
	}
}

func TestServerURLShape(t *testing.T) {
	srv := Start(Config{})
	defer srv.Close()
	if !strings.HasPrefix(srv.URL(), "ws://") {
		t.Fatalf("URL() = %q, want ws:// prefix", srv.URL())
	}
	if srv.Addr() != strings.TrimPrefix(srv.URL(), "ws://") {
		t.Fatalf("URL() and Addr() disagree: %q vs %q", srv.URL(), srv.Addr())
	}
}

// TestZeroDifficultyReachesTheWire pins the "frames pass through unvalidated"
// contract. Rewriting a zero difficulty to 1000 would hand the client a job
// the caller meant it to reject, making the miner.ErrBadDiff path untestable.
func TestZeroDifficultyReachesTheWire(t *testing.T) {
	srv := Start(Config{Jobs: []Job{{JobID: "d0", BlockhashingBlob: validBlobHex}}})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var frame getwork.Job
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read: %v", err)
	}
	if frame.Difficultyuint64 != 0 {
		t.Fatalf("Difficultyuint64 = %d, want 0 (passed through, not defaulted)", frame.Difficultyuint64)
	}
	if frame.Difficulty != "0" {
		t.Fatalf("Difficulty = %q, want \"0\"", frame.Difficulty)
	}
}

// TestNonSubmitFramesAreNotCounted keeps the submission count honest: any JSON
// object decodes into a Submit with zeroed fields, so a stray frame would
// otherwise be recorded as a share.
func TestNonSubmitFramesAreNotCounted(t *testing.T) {
	onSubmitCh := make(chan Submission, 4)
	srv := Start(Config{OnSubmit: func(s Submission) { onSubmitCh <- s }})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var frame getwork.Job
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Neither of these is a share: one has no fields in common with Submit,
	// the other carries a job id but no blob.
	if err := conn.WriteJSON(map[string]string{"status": "keepalive"}); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if err := conn.WriteJSON(getwork.Submit{JobID: "job-0"}); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	// A real share behind them: once it lands, the two above have been read.
	if err := conn.WriteJSON(getwork.Submit{JobID: "job-0", Blob: "deadbeef"}); err != nil {
		t.Fatalf("write submit: %v", err)
	}

	select {
	case got := <-onSubmitCh:
		if got.JobID != "job-0" || got.Blob != "deadbeef" {
			t.Fatalf("first OnSubmit = %+v, want the real share; a non-share was counted", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnSubmit never fired")
	}

	if subs := srv.Submissions(); len(subs) != 1 {
		t.Fatalf("Submissions = %+v, want only the real share", subs)
	}
}

// TestCloseWaitsForHandlers pins the other half of the teardown: Close must
// not return while a handler -- and therefore an OnSubmit callback -- is still
// running, or the callback lands in a test that has already finished.
func TestCloseWaitsForHandlers(t *testing.T) {
	var once sync.Once
	started := make(chan struct{})
	var mu sync.Mutex
	finished := false

	srv := Start(Config{OnSubmit: func(Submission) {
		once.Do(func() { close(started) })
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		finished = true
		mu.Unlock()
	}})
	defer srv.Close() // idempotent; the real Close is below

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var frame getwork.Job
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := conn.WriteJSON(getwork.Submit{JobID: "job-0", Blob: "deadbeef"}); err != nil {
		t.Fatalf("write submit: %v", err)
	}

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("OnSubmit never fired")
	}

	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	if !finished {
		t.Fatal("Close returned while a handler was still running")
	}
}

// TestCloseHangsUpLiveConnections covers the leak: httptest forgets hijacked
// connections, so without explicit teardown the handler keeps pushing jobs at
// a client that has already been "closed", and it plus its reader stay alive.
func TestCloseHangsUpLiveConnections(t *testing.T) {
	srv := Start(Config{JobEvery: 20 * time.Millisecond})
	defer srv.Close() // idempotent; the real Close is below

	conn, _, err := websocket.DefaultDialer.Dial(srv.URL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var frame getwork.Job
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read before close: %v", err)
	}

	srv.Close()

	// The connection must go away on its own. Without the fix the server keeps
	// pushing job frames and this loop runs until the deadline.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if strings.Contains(err.Error(), "timeout") {
				t.Fatal("server kept the connection alive after Close")
			}
			return // hung up, as it should be
		}
	}
}