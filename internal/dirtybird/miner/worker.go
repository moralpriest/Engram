package miner

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"runtime"
	"time"

	"github.com/DEROFDN/engram/internal/dirtybird/astrobwt"
	"github.com/DEROFDN/engram/internal/dirtybird/getwork"
)

const counterFlush = 16 // hashes between shared-counter flushes

// Run is one mining worker. It locks its OS thread, optionally pins it, and
// grinds nonces on the current job until the epoch changes or ctx ends.
//
// Nonce layout (byte-exact family convention, from derohe mineblock):
// random fill of blob bytes 36..47 per job, big-endian uint32 counter at
// bytes 43..46, thread id at byte 47.
//
// pair grinds two nonces per iteration through HashPair, batching both final
// SHA-256s through the 2-way SHA-NI block (zig miner's 2-workers-per-thread
// design). Measured +5%/hash at 1T but -2% at 20T on the i7-13700HX (the
// shared SHA port saturates and the second scratch doubles the footprint),
// so it is opt-in for low thread counts.
func Run(ctx context.Context, tid int, st *State, submits chan<- getwork.Submit, pinOrder []int, backend astrobwt.Backend, pair bool) {
	runtime.LockOSThread()
	if pinOrder != nil {
		pinCurrentThread(tid, pinOrder)
	}
	h := astrobwt.NewWithBackend(backend)

	for ctx.Err() == nil {
		blob, jobid, target, epoch := st.Job()
		if epoch == 0 || !st.Active() { // no live job yet
			// A bare sleep keeps the outage poll allocation-free (time.After
			// heap-allocates a timer per poll, and GC is off between the
			// hourly forced collections); the cost is up to 50 ms of added
			// cancellation latency. Use a state-change channel if that ever
			// matters.
			time.Sleep(50 * time.Millisecond)
			continue
		}

		workA := blob
		rand.Read(workA[36:48])
		workA[47] = byte(tid)
		workB := workA // second nonce lane (pair mode only)

		submit := func(work *[48]byte) {
			select {
			case submits <- getwork.Submit{JobID: jobid, Blob: hex.EncodeToString(work[:]), Epoch: epoch}:
				st.Submitted.Add(1)
			default: // mailbox full: drop rather than stall the hot loop
				st.Stale.Add(1)
			}
		}

		var nonce uint32
		var local uint64
		for st.Active() && st.Epoch() == epoch && ctx.Err() == nil {
			if pair {
				nonce++
				binary.BigEndian.PutUint32(workA[43:47], nonce)
				nonce++
				binary.BigEndian.PutUint32(workB[43:47], nonce)
				powA, powB := h.HashPair(workA[:], workB[:])
				local += 2
				if MeetsTarget(&powA, &target) && st.Active() && st.Epoch() == epoch {
					submit(&workA)
				}
				if MeetsTarget(&powB, &target) && st.Active() && st.Epoch() == epoch {
					submit(&workB)
				}
			} else {
				nonce++
				binary.BigEndian.PutUint32(workA[43:47], nonce)
				pow := h.Hash(workA[:])
				local++
				if MeetsTarget(&pow, &target) && st.Active() && st.Epoch() == epoch {
					submit(&workA)
				}
			}
			if local >= counterFlush {
				st.TotalHashes.Add(local)
				local = 0
			}
		}
		st.TotalHashes.Add(local)
	}
}
