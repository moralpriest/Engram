// Copyright 2017-2026 DERO Project. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakegetwork is an in-process DERO getwork server for tests.
//
// It is the importable companion to the standalone tools/fakegetwork binary.
// Where that tool drives a real miner process through a full lifecycle and
// counts what arrives, this package lets other modules (which cannot import
// Engram's internal dirtybird packages directly from outside the module)
// stand up a typed getwork endpoint inside their own go test and drive
// whatever client they are testing.
//
// The protocol traps documented in tools/fakegetwork are encoded here so a
// caller cannot reproduce them by accident:
//
//   - Job frames pass through unvalidated. An empty BlockhashingBlob frame is
//     pushed exactly as given, so keepalive/status frames can be exercised.
//     ValidBlob and ValidJob exist so ordinary tests never build a blob with
//     the wrong miniblock version nibble (blob[0]&0xf must be 1 or a DERO
//     miner silently refuses the job) or a difficulty below 2 (the difficulty-1
//     target overflow makes every hash a winner and floods the submit queue).
//
//   - Submissions are read by a dedicated goroutine with no read deadline.
//     gorilla/websocket allows one reader and one writer per connection, and a
//     short read deadline puts the connection into a permanent failed state
//     before the first submission arrives -- both traps silently zeroed the
//     submit count in the tool that was meant to detect them.
//
// The server listens on plaintext ws:// only. TLS (wss://) is out of scope;
// every DERO miner accepts ws://, and the standalone tool already covers the
// TLS-dialing family.
package fakegetwork

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DEROFDN/engram/internal/dirtybird/getwork"
)

// validBlobHex is a real 48-byte miniblock (version nibble 1), the shape a
// getwork daemon pushes. Every generated job carries it.
const validBlobHex = "419ebb000000001bbdc9bf2200000000635d6e4e24829b4249fe0e67878ad4350000000043f53e5436cf610000086b00"

// Job is the public mirror of getwork.Job. Field names are idiomatic; the
// server converts to the wire frame before writing.
type Job struct {
	JobID            string
	BlockhashingBlob string
	Difficulty       uint64
	Height           uint64
	Blocks           uint64
	MiniBlocks       uint64
	Rejected         uint64
	LastError        string
}

// Submission is a share the client submitted, measured server-side.
type Submission struct {
	JobID string
	Blob  string
}

// Config controls the server. Zero values produce a server that pushes a
// single ValidJob("job-0", 1000) and then holds the connection open.
type Config struct {
	// Jobs are the frames pushed per connection, in order. An empty slice
	// defaults to one ValidJob. Frames pass through unvalidated.
	Jobs []Job
	// PushInterval, when positive, spaces individual pushes within a pass.
	PushInterval time.Duration
	// JobEvery, when positive, loops the pass at this cadence instead of
	// stopping after one pass. A zero value pushes once and holds.
	JobEvery time.Duration
	// OnSubmit, if set, is invoked for every submission received.
	OnSubmit func(Submission)
}

// Server is an in-process getwork daemon. Create one with Start and Close it
// when done.
type Server struct {
	srv          *httptest.Server
	jobs         []Job
	pushInterval time.Duration
	jobEvery     time.Duration
	onSubmit     func(Submission)

	mu          sync.Mutex
	submissions []Submission
	conns       map[*websocket.Conn]struct{}
	closed      bool
	handlers    sync.WaitGroup
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// Start begins serving on an ephemeral loopback address.
func Start(cfg Config) *Server {
	s := &Server{
		jobs:         append([]Job(nil), cfg.Jobs...),
		pushInterval: cfg.PushInterval,
		jobEvery:     cfg.JobEvery,
		onSubmit:     cfg.OnSubmit,
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Addr returns the bare "host:port" listen address.
func (s *Server) Addr() string { return s.srv.Listener.Addr().String() }

// URL returns the "ws://host:port" dial URL.
func (s *Server) URL() string { return "ws://" + s.Addr() }

// Close shuts the server down, closes every upgraded connection, and waits
// for the handlers to finish. httptest forgets hijacked connections, so
// without this a handler could still be pushing jobs -- or invoking OnSubmit
// -- after Close returned, which lands in a test that has already finished.
// Like httptest.Server.Close it blocks until the handlers return, so an
// OnSubmit that never returns will hang it. Safe to call more than once.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	conns := make([]*websocket.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.conns = nil
	s.mu.Unlock()

	for _, c := range conns {
		c.Close()
	}
	s.handlers.Wait()
	s.srv.Close()
}

// track registers an upgraded connection, reporting false if Close already
// ran and the handler should simply hang up.
func (s *Server) track(conn *websocket.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if s.conns == nil {
		s.conns = make(map[*websocket.Conn]struct{})
	}
	s.conns[conn] = struct{}{}
	// Counted under the same lock that guards closed, so Close cannot start
	// waiting between the check and the Add.
	s.handlers.Add(1)
	return true
}

func (s *Server) untrack(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	s.handlers.Done()
}

// Submissions returns every submission received so far, in order.
func (s *Server) Submissions() []Submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Submission(nil), s.submissions...)
}

// ValidBlob returns the canonical 48-byte miniblock hex with version nibble 1,
// the job shape every DERO getwork miner accepts.
func ValidBlob() string { return validBlobHex }

// ValidJob returns a difficulty-1000 job carrying ValidBlob at the given
// height. Difficulty 1000 (>= 2) avoids the difficulty-1 target overflow trap.
func ValidJob(id string, height uint64) Job {
	return Job{
		JobID:            id,
		BlockhashingBlob: validBlobHex,
		Difficulty:       1000,
		Height:           height,
	}
}

func (s *Server) addSubmission(sub Submission) {
	s.mu.Lock()
	s.submissions = append(s.submissions, sub)
	s.mu.Unlock()
	if s.onSubmit != nil {
		s.onSubmit(sub)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	if !s.track(conn) {
		return
	}
	defer s.untrack(conn)

	// Submissions are drained by a dedicated reader with no read deadline; see
	// the package comment for the two traps this arrangement avoids.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub getwork.Submit
			if json.Unmarshal(msg, &sub) != nil {
				continue
			}
			// Any JSON object decodes into a Submit with zeroed fields;
			// counting those would inflate the number this package exists
			// to measure.
			if sub.JobID == "" || sub.Blob == "" {
				continue
			}
			s.addSubmission(Submission{JobID: sub.JobID, Blob: sub.Blob})
		}
	}()

	// The reader calls OnSubmit, so the handler is not done until it is.
	defer func() { conn.Close(); <-readerDone }()

	pass := s.jobs
	if len(pass) == 0 {
		pass = []Job{ValidJob("job-0", 1000)}
	}

	for {
		for i, j := range pass {
			if i > 0 && s.pushInterval > 0 {
				select {
				case <-time.After(s.pushInterval):
				case <-readerDone:
					return
				}
			}
			if err := conn.WriteJSON(wireJob(j)); err != nil {
				return
			}
		}
		if s.jobEvery <= 0 {
			<-readerDone // single pass: hold until the client goes away
			return
		}
		select {
		case <-time.After(s.jobEvery):
		case <-readerDone:
			return
		}
	}
}

// wireJob converts the public mirror to the internal wire frame. Every field
// passes through verbatim, difficulty included: rewriting a zero would hand
// back a job the caller meant the client to reject, which is exactly the
// miner.ErrBadDiff path a test wants to drive.
func wireJob(j Job) getwork.Job {
	return getwork.Job{
		JobID:             j.JobID,
		Blockhashing_blob: j.BlockhashingBlob,
		Difficulty:        strconv.FormatUint(j.Difficulty, 10),
		Difficultyuint64:  j.Difficulty,
		Height:            j.Height,
		Blocks:            j.Blocks,
		MiniBlocks:        j.MiniBlocks,
		Rejected:          j.Rejected,
		LastError:         j.LastError,
	}
}