package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/civilware/tela/logger"

	"github.com/DEROFDN/engram/pkg/engine"
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
// miner gates startup on. The engine runs the same KAT before any mining
// starts and refuses to run on a broken pipeline.
const dirtybirdKatHash = "54e2324ddacc3f0383501a9e5760f85d63e9bc6705e9124ca7aef89016ab81ea"

var (
	// dirtybirdEngine is the running embeddable engine, or nil when stopped.
	// Guarded by dirtybirdMu.
	dirtybirdMu      sync.Mutex
	dirtybirdEngine  *engine.Engine
	dirtybirdStarted time.Time // when the current session began
	dirtybirdReward  time.Time // when the last mini block was found
	dirtybirdMini    uint64    // last observed MiniBlocks, for LastRewardTime
)

// startDirtybirdMiner launches the Dirtybird in-process miner. It mirrors
// startEmbeddedMiner() in miner_embed.go: the same wallet, daemon, and thread
// resolution feed the embeddable engine, which owns the getwork client, the
// workers, and the hashrate window.
func startDirtybirdMiner() {
	dirtybirdMu.Lock()
	if dirtybirdEngine != nil {
		dirtybirdMu.Unlock()
		logger.Printf("[Miner] Dirtybird already running")
		return
	}
	dirtybirdMu.Unlock()

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

	// engine.Start derives its own cancellable context and tears every
	// goroutine down on Stop, so the caller passes a bare background context.
	eng, err := engine.Start(context.Background(), engine.Config{
		Endpoint: daemonAddr,
		Wallet:   walletAddr,
		Threads:  threads,
		Backend:  engine.DefaultBackendName,
		Logf: func(level, format string, args ...interface{}) {
			logger.Printf("[Miner] "+format, args...)
		},
	})
	if err != nil {
		logger.Printf("[Miner] Dirtybird engine failed to start: %v", err)
		dmState.minerState = dmStateError
		uiDo(syncToggleStates)
		return
	}

	dirtybirdMu.Lock()
	dirtybirdEngine = eng
	dirtybirdStarted = time.Now()
	dirtybirdReward = time.Time{}
	dirtybirdMini = 0
	dirtybirdMu.Unlock()

	logger.Printf("[Miner] Starting Dirtybird miner: %d threads, wallet=%s, daemon=%s", threads, walletAddr, daemonAddr)
	dmState.minerState = dmStateRunning
	uiDo(syncToggleStates)
}

// stopDirtybirdMiner stops the Dirtybird in-process miner. Stop blocks until
// every engine goroutine has exited.
func stopDirtybirdMiner() {
	dirtybirdMu.Lock()
	eng := dirtybirdEngine
	dirtybirdEngine = nil
	dirtybirdStarted = time.Time{}
	dirtybirdReward = time.Time{}
	dirtybirdMu.Unlock()

	if eng == nil {
		return
	}
	eng.Stop()

	dmState.minerState = dmStateStopped
	logger.Printf("[Miner] Dirtybird stopped")
	uiDo(syncToggleStates)
}

// dirtybirdIsActive reports whether the Dirtybird engine is running.
func dirtybirdIsActive() bool {
	dirtybirdMu.Lock()
	defer dirtybirdMu.Unlock()
	return dirtybirdEngine != nil
}

// getDirtybirdMiningStats builds the shared MiningStats projection directly
// from engine.Stats(), which is the source of truth for the Dirtybird engine.
// Only the engine's own 1Hz sampler advances the hashrate window, so reading
// it here at UI cadence cannot perturb the figure.
func getDirtybirdMiningStats() (MiningStats, bool) {
	dirtybirdMu.Lock()
	eng := dirtybirdEngine
	started := dirtybirdStarted
	reward := dirtybirdReward
	mini := dirtybirdMini
	dirtybirdMu.Unlock()
	if eng == nil {
		return MiningStats{}, false
	}

	st := eng.Stats()
	if !st.Running {
		return MiningStats{}, false
	}

	// Record the reward time the first time MiniBlocks grows past what we saw.
	if st.MiniBlocks > mini {
		dirtybirdMu.Lock()
		if st.MiniBlocks > dirtybirdMini {
			dirtybirdMini = st.MiniBlocks
			dirtybirdReward = time.Now()
			reward = dirtybirdReward
		}
		dirtybirdMu.Unlock()
	}

	stats := MiningStats{
		StartTime:       started,
		MiniBlocks:      int64(st.MiniBlocks),
		Rejected:        int64(st.Rejected),
		CurrentHashrate: st.Hashrate,
		SpeedStr:        formatHashrate(st.Hashrate),
		LastRewardTime:  reward,
	}

	// Network hashrate from job difficulty, as the DEROHE path does.
	if st.Difficulty > 0 {
		netHash := float64(st.Difficulty) / 1.8
		if netHash > 0 {
			stats.NetHashrate = netHash
			stats.NetHashStr = formatHashrate(netHash)
		}
	}

	return stats, true
}