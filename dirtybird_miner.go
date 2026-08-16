package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/civilware/tela/logger"

	"github.com/DEROFDN/engram/internal/dirtybird/astrobwt"
	"github.com/DEROFDN/engram/internal/dirtybird/getwork"
	"github.com/DEROFDN/engram/internal/dirtybird/miner"
)

// MinerEngine selects which in-process mining engine runs.
type MinerEngine int

const (
	// MinerDEROHE is the existing embedded DEROHE astrobwt miner.
	MinerDEROHE MinerEngine = iota
	// MinerDirtybird is the Dirtybird pure-Go AstroBWTv3 miner.
	MinerDirtybird
)

// engineName returns a stable key used for persistence and radio selection.
func (e MinerEngine) String() string {
	if e == MinerDirtybird {
		return "dirtybird"
	}
	return "derohe"
}

// engineFromString maps a persisted key back to a MinerEngine.
func engineFromString(s string) MinerEngine {
	if s == MinerDirtybird.String() {
		return MinerDirtybird
	}
	return MinerDEROHE
}

// dirtybirdKatHash is the AstroBWTv3("a") known-answer value the Dirtybird
// miner gates startup on. The hash core is verified against it (and a real
// 48-byte miniblock vector) before any mining starts.
const dirtybirdKatHash = "54e2324ddacc3f0383501a9e5760f85d63e9bc6705e9124ca7aef89016ab81ea"

const (
	dirtybirdStopped int64 = iota
	dirtybirdActive
)

var (
	dirtybirdMiner struct {
		mission int64 // atomic: dirtybirdActive or dirtybirdStopped
		ctx     context.Context
		cancel  context.CancelFunc
		state   *miner.State
		submits chan getwork.Submit
		client  *getwork.Client
		backend astrobwt.Backend
	}
)

// startDirtybirdMiner launches the Dirtybird in-process miner. It mirrors
// startEmbeddedMiner() in miner_embed.go: the same wallet, daemon, and thread
// resolution feed the Dirtybird worker orchestration.
func startDirtybirdMiner() {
	if atomic.LoadInt64(&dirtybirdMiner.mission) == dirtybirdActive {
		logger.Printf("[Miner] Dirtybird already running")
		return
	}

	walletAddr := minerWalletAddr
	if walletAddr == "" && engram.Disk != nil {
		walletAddr = engram.Disk.GetAddress().String()
	}
	if walletAddr == "" {
		logger.Printf("[Miner] No wallet address configured")
		dmState.minerState = dmStateError
		uiDo(syncToggleStates)
		return
	}

	daemonAddr := minerDaemonAddr
	if daemonAddr == "" {
		daemonAddr = fmt.Sprintf("127.0.0.1:%d", daemonWorkPort())
	}

	threads := minerThreads
	if threads < 1 {
		threads = 1
	}

	// Verify the copied hash core before mining (Dirtybird's startup KAT).
	backend := astrobwt.BackendV114 // structure-aware SA, ~2x faster, SAIS fallback
	kat := fmt.Sprintf("%x", astrobwt.NewWithBackend(backend).Hash([]byte("a")))
	if kat != dirtybirdKatHash {
		logger.Printf("[Miner] Dirtybird KAT failed (pow(\"a\") = %s); refusing to mine", kat)
		dmState.minerState = dmStateError
		uiDo(syncToggleStates)
		return
	}

	// Reset shared counters and stats.
	minerCounter = 0
	minerDifficulty = 0
	minerHeight = 0
	blockCounter = 0
	miniBlockCounter = 0
	rejectedCounter = 0
	miningStats.mu.Lock()
	miningStats.StartTime = time.Now()
	miningStats.MiniBlocks = 0
	miningStats.Rejected = 0
	miningStats.CurrentHashrate = 0
	miningStats.NetHashrate = 0
	miningStats.LastRewardTime = time.Time{}
	miningStats.SpeedStr = ""
	miningStats.NetHashStr = ""
	miningStats.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	st := &miner.State{}
	submits := make(chan getwork.Submit, 16)

	client := &getwork.Client{
		Endpoint:     daemonAddr,
		Wallet:       walletAddr,
		Submits:      submits,
		OnDisconnect: st.Invalidate,
		SubmitValid: func(s getwork.Submit) bool {
			return st.Active() && st.Epoch() == s.Epoch
		},
		Logf: func(format string, args ...interface{}) {
			logger.Printf("[Miner] "+format, args...)
		},
		Errorf: func(format string, args ...interface{}) {
			logger.Printf("[Miner] "+format, args...)
		},
	}
	client.OnJob = func(j getwork.Job) bool {
		if _, err := st.SetJob(j); err != nil {
			logger.Printf("[Miner] Bad job: %v", err)
			return false
		}
		return true
	}

	dirtybirdMiner.ctx = ctx
	dirtybirdMiner.cancel = cancel
	dirtybirdMiner.state = st
	dirtybirdMiner.submits = submits
	dirtybirdMiner.client = client
	dirtybirdMiner.backend = backend
	atomic.StoreInt64(&dirtybirdMiner.mission, dirtybirdActive)

	logger.Printf("[Miner] Starting Dirtybird miner: %d threads, wallet=%s, daemon=%s", threads, walletAddr, daemonAddr)
	dmState.minerState = dmStateRunning

	for t := 0; t < threads; t++ {
		go miner.Run(ctx, t, st, submits, nil, backend, false)
	}
	go client.Run(ctx)
	go dirtybirdStatsLoop()

	uiDo(syncToggleStates)
}

// stopDirtybirdMiner stops the Dirtybird in-process miner and resets stats.
func stopDirtybirdMiner() {
	if atomic.LoadInt64(&dirtybirdMiner.mission) != dirtybirdActive {
		return
	}
	dirtybirdMiner.cancel()
	atomic.StoreInt64(&dirtybirdMiner.mission, dirtybirdStopped)
	dmState.minerState = dmStateStopped

	miningStats.mu.Lock()
	miningStats.StartTime = time.Time{}
	miningStats.CurrentHashrate = 0
	miningStats.SpeedStr = ""
	miningStats.LastRewardTime = time.Time{}
	miningStats.NetHashrate = 0
	miningStats.NetHashStr = ""
	miningStats.MiniBlocks = 0
	miningStats.Rejected = 0
	miningStats.mu.Unlock()

	logger.Printf("[Miner] Dirtybird stopped")
	uiDo(syncToggleStates)
}

// dirtybirdIsActive reports whether the Dirtybird engine is running.
func dirtybirdIsActive() bool {
	return atomic.LoadInt64(&dirtybirdMiner.mission) == dirtybirdActive
}

// dirtybirdStatsLoop mirrors minerStatsLoop in miner_embed.go: it samples the
// Dirtybird State atomics and feeds the shared MiningStats used by the live
// stats panel.
func dirtybirdStatsLoop() {
	st := dirtybirdMiner.state
	if st == nil {
		return
	}

	var lastCounter uint64
	var lastCounterTime = time.Now()

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for atomic.LoadInt64(&dirtybirdMiner.mission) == dirtybirdActive {
		select {
		case <-dirtybirdMiner.ctx.Done():
			return
		case <-tick.C:
		}

		currentCounter := st.TotalHashes.Load()
		now := time.Now()

		// Mirror the daemon job counters into the shared counters.
		blockCounter = st.Blocks.Load()
		miniBlockCounter = st.MiniBlocks.Load()
		rejectedCounter = st.Rejected.Load()
		minerHeight = int64(st.Height.Load())
		minerDifficulty = st.Diff.Load()

		miningStats.mu.Lock()

		if currentCounter > lastCounter {
			elapsed := now.Sub(lastCounterTime).Seconds()
			if elapsed > 0 {
				speed := float64(currentCounter-lastCounter) / elapsed
				miningStats.CurrentHashrate = speed
				miningStats.SpeedStr = formatHashrate(speed)
			}
		}

		// Network hashrate from job difficulty (job.Difficulty).
		if minerDifficulty > 0 {
			netHash := float64(minerDifficulty) / 1.8
			if netHash > 0 {
				miningStats.NetHashrate = netHash
				miningStats.NetHashStr = formatHashrate(netHash)
			}
		}

		miningStats.Rejected = int64(st.Rejected.Load())
		miningStats.MiniBlocks = int64(st.MiniBlocks.Load())
		if miningStats.LastRewardTime.IsZero() && st.MiniBlocks.Load() > 0 {
			miningStats.LastRewardTime = now
		}

		miningStats.mu.Unlock()

		lastCounter = currentCounter
		lastCounterTime = now

		uiDo(func() {
			updateMinerStatsUI()
		})
	}
}
