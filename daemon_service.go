// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/civilware/tela/logger"

	// DERO HE embedded daemon
	"github.com/deroproject/derohe/blockchain"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/p2p"
)

// Priority seed nodes from user's start-derod.sh
var embeddedHTTPServer *http.Server

var prioritySeedNodes = []string{
	"78.159.118.236:11011",
	"209.58.186.186:11011",
	"23.81.165.146:11011",
	"85.17.52.28:11011",
	"82.65.143.182:11011",
	"204.12.199.25:11011",
	"154.26.138.136:11011",
	"66.85.74.214:33849",
	"dero-node.mysrv.cloud:11011",
	"dero-node-yashnik-eu.mysrv.cloud:11011",
	"dero-node-va.mysrv.cloud:11011",
	"dero-node-ch4k1pu.mysrv.cloud:11011",
	"dero-node-orionure-sg.mysrv.cloud:11011",
	"dero-node-orionure-us.mysrv.cloud:11011",
	"dero-node-gustavogerman.mysrv.cloud:11011",
	"dero-node-maikze.mysrv.cloud:11011",
	"dero-node-sk.mysrv.cloud:11011",
	"dero-node.net:11011",
	"85.214.253.170:58686",
}

var (
	daemonMu sync.Mutex

	minerThreads            int
	minerWalletAddr         string
	minerDaemonAddr         string
	daemonMode              string                 // "full" or "pruned" - persisted
	daemonFastSync          bool                   // use --fastsync flag on startup - persisted
	daemonIntegratorAddress string                 // optional integrator reward address
	globalChain             *blockchain.Blockchain // embedded daemon instance

	miningStats MiningStats
)

func init() {
	cpus := runtime.NumCPU()
	minerThreads = cpus - 4
	if minerThreads < 1 {
		minerThreads = 1
	}
	// minerDaemonAddr resolved dynamically in startMiner()

	loadDaemonConfig()
}

// daemonDataDir returns the per-network data directory.
func daemonDataDir() string {
	n := strings.ToLower(session.Network)
	if n == "" || n == "mainnet" {
		return filepath.Join(AppPath(), "node", "mainnet")
	}
	return filepath.Join(AppPath(), "node", n)
}

// daemonRPCAddress returns 127.0.0.1:<port> for the current network.
func daemonRPCAddress() string {
	switch session.Network {
	case NETWORK_TESTNET:
		return "127.0.0.1:40402"
	case NETWORK_SIMULATOR:
		return "127.0.0.1:20000"
	default:
		return "127.0.0.1:10102"
	}
}

// startDaemon launches the embedded derod process (compiled from source).
func startDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if globalChain != nil {
		logger.Printf("[Daemon] Already running (embedded)")
		return
	}

	dataDir := daemonDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Printf("[Daemon] Failed to create data dir %s: %v", dataDir, err)
		dmState.daemonState = dmStateError
		return
	}

	// Always use embedded daemon (built from source) on all platforms
	// No external binary download required
	startEmbeddedDaemon(dataDir)
}

// stopDaemon gracefully stops the daemon process.
func stopDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	// Stop embedded daemon if running
	if globalChain != nil {
		logger.Printf("[Daemon] Stopping embedded daemon...")
		// Stop the RPC server first
		if embeddedHTTPServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			embeddedHTTPServer.Shutdown(ctx)
			embeddedHTTPServer = nil
		}
		close(globalChain.Exit_Event)
		globalChain = nil
		dmState.daemonState = dmStateStopped
		uiDo(syncToggleStates)
		return
	}

	logger.Printf("[Daemon] Not running")
}

func loadDaemonConfig() {
	// Load persisted daemon mode
	if val, err := GetEncryptedValue("settings", []byte("daemon_mode")); err == nil && len(val) > 0 {
		daemonMode = string(val)
	}
	if daemonMode == "" {
		daemonMode = "pruned" // default to pruned
	}

	// Load persisted fastsync flag
	if val, err := GetEncryptedValue("settings", []byte("daemon_fastsync")); err == nil && len(val) > 0 {
		daemonFastSync = string(val) == "true"
	} else {
		// Migrate: default fastsync to true when pruned, false for full
		daemonFastSync = daemonMode == "pruned"
	}

	// Load persisted integrator address
	if val, err := GetEncryptedValue("settings", []byte("daemon_integrator")); err == nil && len(val) > 0 {
		daemonIntegratorAddress = string(val)
	}

	// Load force full mode preference
	loadForceFullMode()
}

func saveDaemonMode(mode string) {
	daemonMode = mode
	StoreEncryptedValue("settings", []byte("daemon_mode"), []byte(mode))
}

func saveDaemonFastSync(fastSync bool) {
	daemonFastSync = fastSync
	if fastSync {
		StoreEncryptedValue("settings", []byte("daemon_fastsync"), []byte("true"))
	} else {
		StoreEncryptedValue("settings", []byte("daemon_fastsync"), []byte("false"))
	}
}

func saveIntegratorAddress(addr string) {
	daemonIntegratorAddress = addr
	StoreEncryptedValue("settings", []byte("daemon_integrator"), []byte(addr))
}

var forceFullMode bool

func loadForceFullMode() {
	if val, err := GetEncryptedValue("settings", []byte("force_full_mode")); err == nil && len(val) > 0 {
		forceFullMode = string(val) == "true"
	}
	logger.Printf("[Daemon] Force Full Mode: %v", forceFullMode)
}

func saveForceFullMode(force bool) {
	forceFullMode = force
	if force {
		StoreEncryptedValue("settings", []byte("force_full_mode"), []byte("true"))
	} else {
		StoreEncryptedValue("settings", []byte("force_full_mode"), []byte("false"))
	}
}

// startEmbeddedRPCServer starts a lightweight HTTP JSON-RPC server for the embedded daemon.
// This allows fetchDaemonInfo() to reach the daemon for sync status detection.
// We implement this ourselves because the derohe fork's cmd/derod/rpc package has build errors.
func startEmbeddedRPCServer() {
	// Determine RPC port based on network
	rpcAddr := daemonRPCAddress()

	mux := http.NewServeMux()
	mux.HandleFunc("/json_rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read and parse the JSON-RPC request
		var req struct {
			JsonRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			ID      string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Only handle get_info method
		if req.Method != "get_info" {
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]interface{}{"code": -32601, "message": "Method not found"},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Build daemon info from the embedded blockchain
		chain := globalChain
		info := DaemonInfo{
			Height:       0,
			Topoheight:   0,
			Difficulty:   0,
			Version:      "embedded",
			Network:      "mainnet",
			InPeers:      0,
			OutPeers:     0,
			TxPoolSize:   0,
			Status:       "OK",
			Synchronized: false,
		}

		if chain != nil {
			chain.RLock()
			height := chain.Get_Height()
			topoHeight := chain.Load_TOPO_HEIGHT()
			diff := chain.Get_Difficulty()
			chain.RUnlock()

			info.Height = uint64(height)
			info.Topoheight = uint64(topoHeight)
			info.Difficulty = uint64(diff)

			// Determine if synced: height close to topoheight means caught up
			if height > 0 && topoHeight > 0 && (topoHeight-height) <= 10 {
				info.Synchronized = true
				info.Status = "OK"
			} else if height > 0 {
				info.Status = "SYNCHRONIZING"
			}

			// Get peer counts
			outCount, inCount := p2p.Peer_Direction_Count()
			info.OutPeers = int(outCount)
			info.InPeers = int(inCount)
			logger.Printf("[Daemon RPC] Peer counts: InPeers=%d OutPeers=%d (height=%d topo=%d)", info.InPeers, info.OutPeers, info.Height, info.Topoheight)

			// Get network name
			info.Network = session.Network
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  info,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := &http.Server{
		Addr:    rpcAddr,
		Handler: mux,
	}

	embeddedHTTPServer = server

	logger.Printf("[Daemon] Starting embedded RPC server on %s", rpcAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Printf("[Daemon] RPC server error: %v", err)
	}
}

// startEmbeddedDaemon starts the daemon in-process using the derohe library (mobile/fallback).
func startEmbeddedDaemon(dataDir string) {
	logger.Printf("[Daemon] Starting embedded daemon (data-dir: %s)", dataDir)
	// Verify data directory is writable
	testFile := filepath.Join(dataDir, ".engram_write_test")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		logger.Printf("[Daemon] WARNING: Cannot create data directory %s: %v", dataDir, err)
	} else if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
		logger.Printf("[Daemon] WARNING: Data directory %s is NOT writable: %v", dataDir, err)
	} else {
		os.Remove(testFile)
		logger.Printf("[Daemon] Data directory %s is writable", dataDir)
	}

	// Set globals.Arguments with our configuration (simulates CLI args)
	// This is required by blockchain.Blockchain_Start which reads from globals.Arguments
	if globals.Arguments == nil {
		globals.Arguments = make(map[string]interface{})
	}
	globals.Arguments["--data-dir"] = dataDir

	// Determine network-specific ports for the embedded daemon
	rpcPort := DEFAULT_DAEMON_PORT
	p2pPort := 10101
	workPort := DEFAULT_WORK_PORT
	if session.Network == NETWORK_TESTNET {
		rpcPort = DEFAULT_TESTNET_DAEMON_PORT
		p2pPort = 40401
		workPort = DEFAULT_TESTNET_WORK_PORT
	} else if session.Network == NETWORK_SIMULATOR {
		rpcPort = DEFAULT_SIMULATOR_DAEMON_PORT
		p2pPort = 20001
		workPort = 20000
	}

	// Bind RPC, P2P, and getwork servers so the daemon's RPC is reachable locally
	globals.Arguments["--rpc-bind"] = fmt.Sprintf("127.0.0.1:%d", rpcPort)
	globals.Arguments["--p2p-bind"] = fmt.Sprintf("0.0.0.0:%d", p2pPort)
	globals.Arguments["--getwork-bind"] = fmt.Sprintf("0.0.0.0:%d", workPort)
	globals.Arguments["--max-peers"] = 101

	// Add priority seed nodes for faster peer discovery (collect all into a slice)
	var priorityNodes []string
	for _, node := range prioritySeedNodes {
		priorityNodes = append(priorityNodes, node)
	}
	if len(priorityNodes) > 0 {
		globals.Arguments["--add-priority-node"] = priorityNodes
	}

	// NOTE: --fastsync is intentionally NOT set for embedded mode.
	// In the derohe library, fastsync triggers a snapshot download that
	// stalls at height=0 when started via Blockchain_Start/P2P_Init
	// instead of the standalone derod binary. The chain connects to peers
	// but never advances height/topoheight past 0. Standard block-by-block
	// sync works correctly in embedded mode.

	// Time sync was already verified by checkSystemHealth - tell the daemon
	globals.TimeIsInSync = true
	if len(priorityNodes) > 0 {
		logger.Printf("[Daemon] Using %d priority seed nodes: %v", len(priorityNodes), priorityNodes)
	} else {
		logger.Printf("[Daemon] No priority seed nodes configured")
	}

	// Initialize globals (logging, network mode)
	globals.Initialize()

	// Build params for Blockchain_Start (include RPC/P2P binds so the server starts)
	params := map[string]interface{}{
		"--data-dir":  dataDir,
		"--rpc-bind":  fmt.Sprintf("127.0.0.1:%d", rpcPort),
		"--p2p-bind":  fmt.Sprintf("0.0.0.0:%d", p2pPort),
		"--max-peers": 101,
	}
	if session.Network == NETWORK_TESTNET {
		params["--testnet"] = ""
	}
	if len(priorityNodes) > 0 {
		params["--add-priority-node"] = priorityNodes
	}
	if daemonIntegratorAddress != "" {
		params["--integrator-address"] = daemonIntegratorAddress
	}

	// Prune blockchain if configured
	if daemonMode == "pruned" {
		logger.Printf("[Daemon] Pruning blockchain history...")
		if err := blockchain.Prune_Blockchain(50); err != nil {
			logger.Printf("[Daemon] Pruning completed: %v", err)
		}
	}

	// Start the blockchain
	chain, err := blockchain.Blockchain_Start(params)
	if err != nil {
		logger.Printf("[Daemon] Failed to start blockchain: %v", err)
		dmState.daemonState = dmStateError
		return
	}
	globalChain = chain

	// Initialize P2P networking (connects to peers for sync)
	params["chain"] = chain
	if err := p2p.P2P_Init(params); err != nil {
		logger.Printf("[Daemon] P2P_Init FAILED: %v", err)
	} else {
		logger.Printf("[Daemon] P2P_Init succeeded")
		// Start cron jobs (required! P2P_Init registers syncroniser @every 4s,
		// ping_loop @every 10s, etc. via globals.Cron.AddFunc, but they never
		// fire unless globals.Cron.Start() is called. Without this the chain
		// connects to peers but never requests blocks — height stays at 0.)
		globals.Cron.Start()
	}
	// Start verbose P2P debugging for mobile (logs peer connections periodically)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if globalChain == nil {
					return // Daemon stopped
				}
				outPeers, inPeers := p2p.Peer_Direction_Count()
				globalChain.RLock()
				height := globalChain.Get_Height()
				topoHeight := globalChain.Load_TOPO_HEIGHT()
				globalChain.RUnlock()
				logger.Printf("[Daemon P2P Debug] Peers: Out=%d In=%d Height=%d TopoHeight=%d",
					outPeers, inPeers, height, topoHeight)
			case <-globalChain.Exit_Event:
				return
			}
		}
	}()

	// Start the JSON-RPC server so fetchDaemonInfo() can reach this daemon
	// We use a lightweight HTTP server instead of the broken derohe RPC package
	go startEmbeddedRPCServer()

	dmState.daemonState = dmStateConnecting // Start in connecting state since we need to sync
	logger.Printf("[Daemon] Embedded daemon started successfully")

	// Poll for RPC availability and eagerly populate the UI when the daemon
	// becomes reachable. After this initial catch-up the 5-second background
	// refresh loop (started by startBackgroundDaemonRefresh) takes over.
	go func() {
		for i := 0; i < 30; i++ { // up to ~30 seconds
			time.Sleep(1 * time.Second)
			if globalChain == nil {
				return // daemon stopped while we were waiting
			}
			if info, err := fetchDaemonInfo(); err == nil {
				uiDo(func() {
					updateInfoUILabels(info)
					updateDaemonStateFromDetection()
					syncToggleStates()
				})
				return
			}
		}
	}()

	// Monitor for exit
	go func() {
		<-globalChain.Exit_Event
		logger.Printf("[Daemon] Embedded daemon stopped")
		// Stop the RPC server
		if embeddedHTTPServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			embeddedHTTPServer.Shutdown(ctx)
			embeddedHTTPServer = nil
		}
		globalChain = nil
		dmState.daemonState = dmStateStopped
		uiDo(syncToggleStates)
	}()
}

func resolveMinerDaemonAddr() string {
	// Get the current daemon address the user is connected to
	daemonAddr := session.Daemon
	if daemonAddr == "" {
		daemonAddr = getDaemon()
	}

	// If it's localhost or empty, use default local work port
	if daemonAddr == "" || strings.HasPrefix(daemonAddr, "127.0.0.1") || strings.HasPrefix(daemonAddr, "localhost") {
		return fmt.Sprintf("127.0.0.1:%d", daemonWorkPort())
	}

	// Parse the host from the daemon address
	host := daemonAddr
	if strings.Contains(daemonAddr, ":") {
		var err error
		host, _, err = net.SplitHostPort(daemonAddr)
		if err != nil {
			logger.Printf("[Miner] Failed to parse daemon address %s: %v", daemonAddr, err)
			return fmt.Sprintf("127.0.0.1:%d", daemonWorkPort())
		}
	}

	// Use the work port for the current network
	return fmt.Sprintf("%s:%d", host, daemonWorkPort())
}

// startMiner launches the in-process miner.
func startMiner() {
	if minerWalletAddr == "" && engram.Disk != nil {
		minerWalletAddr = engram.Disk.GetAddress().String()
	}
	if minerWalletAddr == "" {
		logger.Printf("[Miner] No wallet address configured")
		dmState.minerState = dmStateError
		uiDo(syncToggleStates)
		return
	}
	// Resolve the miner daemon address from the connected node (supports remote nodes)
	minerDaemonAddr = resolveMinerDaemonAddr()
	go startEmbeddedMiner()
}

// stopMiner stops the in-process miner.
func stopMiner() {
	stopEmbeddedMiner()
	miningStats.mu.Lock()
	miningStats.StartTime = time.Time{}
	miningStats.CurrentHashrate = 0
	miningStats.SpeedStr = ""
	miningStats.LastRewardTime = time.Time{}
	miningStats.mu.Unlock()
}

// DaemonInfo holds daemon status data fetched via get_info RPC.
type DaemonInfo struct {
	Height       uint64  `json:"height"`
	Topoheight   uint64  `json:"topoheight"`
	Difficulty   uint64  `json:"difficulty"`
	Version      string  `json:"version"`
	Network      string  `json:"network"`
	InPeers      int     `json:"incoming_connections_count"`
	OutPeers     int     `json:"outgoing_connections_count"`
	TxPoolSize   int     `json:"tx_pool_size"`
	Status       string  `json:"status"`
	Hashrate1hr  float64 `json:"hashrate_1hr"`
	Hashrate1d   float64 `json:"hashrate_1d"`
	Hashrate7d   float64 `json:"hashrate_7d"`
	Synchronized bool    `json:"synchronized"`
}

// MiningStats tracks real-time mining statistics.
type MiningStats struct {
	mu sync.Mutex

	StartTime       time.Time // When the current mining session started
	MiniBlocks      int64     // Total mini blocks found this session
	Blocks          int64     // Total blocks found this session
	Rejected        int64     // Total rejected shares
	CurrentHashrate float64   // Local mining speed in H/s
	NetHashrate     float64   // Network hashrate in H/s (from difficulty)
	LastRewardTime  time.Time // When the last mini block was found
	SpeedStr        string    // Formatted speed string (e.g. "20.0 KH/s")
	NetHashStr      string    // Formatted network hashrate (e.g. "12.78 MH/s")
}

// ETA calculates the estimated time until the next mini-block reward.
// Formula: (netHash / myHash) * 1.8 seconds per interval (10 mini-blocks per 18s block)
func (ms *MiningStats) ETA() time.Duration {
	if ms.CurrentHashrate <= 0 || ms.NetHashrate <= 0 {
		return 0
	}
	ratio := ms.NetHashrate / ms.CurrentHashrate
	expectedSeconds := ratio * 1.8
	if expectedSeconds > 86400*7 { // cap at 1 week
		expectedSeconds = 86400 * 7
	}
	return time.Duration(expectedSeconds) * time.Second
}

// SessionDuration returns how long the current mining session has been running.
func (ms *MiningStats) SessionDuration() time.Duration {
	if ms.StartTime.IsZero() {
		return 0
	}
	return time.Since(ms.StartTime)
}

func formatHashrate(h float64) string {
	switch {
	case h >= 1e12:
		return fmt.Sprintf("%.3f TH/s", h/1e12)
	case h >= 1e9:
		return fmt.Sprintf("%.3f GH/s", h/1e9)
	case h >= 1e6:
		return fmt.Sprintf("%.3f MH/s", h/1e6)
	case h >= 1e3:
		return fmt.Sprintf("%.3f KH/s", h/1e3)
	default:
		return fmt.Sprintf("%.0f H/s", h)
	}
}

// GetMiningStats returns a thread-safe copy of the current mining stats.
func GetMiningStats() MiningStats {
	miningStats.mu.Lock()
	defer miningStats.mu.Unlock()
	cp := miningStats
	cp.mu = sync.Mutex{}
	return cp
}

type daemonInfoResponse struct {
	JsonRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Result  *DaemonInfo `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var (
	cachedDaemonInfo DaemonInfo
	daemonInfoMu     sync.Mutex
)

// fetchDaemonInfo calls get_info RPC and caches the result.
func fetchDaemonInfo() (DaemonInfo, error) {
	addr := daemonRPCAddress()
	url := fmt.Sprintf("http://%s/json_rpc", addr)

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "0",
		"method":  "get_info",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return DaemonInfo{}, fmt.Errorf("marshal error: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Printf("[Daemon RPC] fetchDaemonInfo ERROR - RPC call failed: %v", err)
		return DaemonInfo{}, fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	var result daemonInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Printf("[Daemon RPC] fetchDaemonInfo ERROR - decode error: %v", err)
		return DaemonInfo{}, fmt.Errorf("decode error: %w", err)
	}

	if result.Error != nil {
		logger.Printf("[Daemon RPC] fetchDaemonInfo ERROR - RPC error (code %d): %s", result.Error.Code, result.Error.Message)
		return DaemonInfo{}, fmt.Errorf("RPC error (code %d): %s", result.Error.Code, result.Error.Message)
	}

	if result.Result == nil {
		logger.Printf("[Daemon RPC] fetchDaemonInfo ERROR - empty RPC result")
		return DaemonInfo{}, fmt.Errorf("empty RPC result")
	}

	daemonInfoMu.Lock()
	cachedDaemonInfo = *result.Result
	daemonInfoMu.Unlock()

	logger.Printf("[Daemon RPC] fetchDaemonInfo SUCCESS - height=%d topoheight=%d synced=%v",
		result.Result.Height, result.Result.Topoheight, result.Result.Synchronized)
	return *result.Result, nil
}

// getCachedDaemonInfo returns the last fetched daemon info without making an RPC call.
func getCachedDaemonInfo() DaemonInfo {
	daemonInfoMu.Lock()
	defer daemonInfoMu.Unlock()
	return cachedDaemonInfo
}

// detectChainCorruption checks for corrupted chain LMDB files.
// Returns true if corruption is detected.
func detectChainCorruption() bool {
	dataDir := daemonDataDir()
	dataFile := filepath.Join(dataDir, "data.mdb")

	stat, err := os.Stat(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}

	if stat.Size() == 0 {
		return true
	}

	f, err := os.Open(dataFile)
	if err != nil {
		return true
	}
	defer f.Close()

	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return true
	}

	if !bytes.HasPrefix(header, []byte{0x00, 0x00, 0x00, 0x00}) {
		return true
	}

	return false
}

// popBlocksRequest is the RPC payload for pop_blocks.
type popBlocksRequest struct {
	JsonRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		N int `json:"n"`
	} `json:"params"`
}

// popBlocksResponse is the RPC response for pop_blocks.
type popBlocksResponse struct {
	ID      string `json:"id"`
	JsonRPC string `json:"jsonrpc"`
	Result  *struct {
		Status string `json:"status"`
		Height int    `json:"height"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// rewindChain sends pop_blocks RPC to the local daemon.
func rewindChain(n int) error {
	addr := daemonRPCAddress()
	url := fmt.Sprintf("http://%s/json_rpc", addr)

	payload := popBlocksRequest{
		JsonRPC: "2.0",
		ID:      "0",
		Method:  "pop_blocks",
	}
	payload.Params.N = n

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	var result popBlocksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	if result.Error != nil {
		return fmt.Errorf("RPC error (code %d): %s", result.Error.Code, result.Error.Message)
	}

	logger.Printf("[Daemon] Rewound %d blocks, new height: %d", n, result.Result.Height)
	return nil
}

// isDaemonSynced checks if the daemon is fully synced based on available info.
// Returns true if synchronized field is true, or if height is close to topoheight.
func isDaemonSynced(info DaemonInfo) bool {
	if info.Synchronized {
		return true
	}
	// If we have no meaningful height yet, we are definitely not synced
	// This handles the case of a brand new chain (height=0, topoheight=0)
	if info.Height == 0 || info.Topoheight == 0 {
		return false
	}
	// When daemon is still syncing, height can be significantly lower than topoheight
	// A fully synced daemon should have height very close to topoheight (within 10 blocks)
	if info.Height < info.Topoheight && info.Topoheight-info.Height > 10 {
		return false
	}
	return true
}

// updateDaemonStateFromDetection refreshes dmState based on live checks.
// Called periodically and on page load.
func updateDaemonStateFromDetection() {
	// Check for embedded daemon (mobile) first — globalChain is set when running in-process.
	if globalChain != nil {
		// Check embedded daemon sync status (mobile)
		if info, err := fetchDaemonInfo(); err == nil {
			// Log peer counts for debugging P2P issues on mobile
			logger.Printf("[Daemon RPC] State check: height=%d topo=%d synced=%v InPeers=%d OutPeers=%d",
				info.Height, info.Topoheight, info.Synchronized, info.InPeers, info.OutPeers)

			// Determine state based on sync progress and peer connections
			totalPeers := info.InPeers + info.OutPeers

			if totalPeers == 0 && (info.Height == 0 || info.Topoheight == 0) {
				// No peers and no blockchain data - still connecting to network
				dmState.daemonState = dmStateConnecting
				logger.Printf("[Daemon RPC] State detection: CONNECTING (no peers, no data)")
			} else if !isDaemonSynced(info) {
				dmState.daemonState = dmStateSyncing
				logger.Printf("[Daemon RPC] State detection: SYNCING (height=%d topo=%d peers=%d)",
					info.Height, info.Topoheight, totalPeers)
			} else {
				dmState.daemonState = dmStateRunning
				logger.Printf("[Daemon RPC] State detection: RUNNING/SYNCED (height=%d topo=%d peers=%d)",
					info.Height, info.Topoheight, totalPeers)
			}
		} else {
			// RPC failed - daemon might still be starting or unreachable
			dmState.daemonState = dmStateConnecting
			logger.Printf("[Daemon RPC] State detection: CONNECTING (RPC failed: %v)", err)
		}
		if detectChainCorruption() {
			dmState.daemonState = dmStateCorrupt
		}
		return
	}

	// Check if an external daemon is reachable (user configured remote node)
	if info, err := fetchDaemonInfo(); err == nil {
		totalPeers := info.InPeers + info.OutPeers
		if totalPeers == 0 && (info.Height == 0 || info.Topoheight == 0) {
			dmState.daemonState = dmStateConnecting
		} else if !isDaemonSynced(info) {
			dmState.daemonState = dmStateSyncing
		} else {
			dmState.daemonState = dmStateExternal
		}
		if detectChainCorruption() {
			dmState.daemonState = dmStateCorrupt
		}
	} else {
		dmState.daemonState = dmStateStopped
	}
}
