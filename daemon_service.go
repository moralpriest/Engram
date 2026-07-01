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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/civilware/tela/logger"
)

// Priority seed nodes from user's start-derod.sh
var prioritySeedNodes = []string{
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
	daemonCmd *exec.Cmd
	minerCmd  *exec.Cmd
	daemonMu  sync.Mutex
	minerMu   sync.Mutex

	daemonLog = newRingBuffer(200)
	minerLog  = newRingBuffer(200)

	minerThreads    int
	minerWalletAddr string
	minerDaemonAddr string
)

func init() {
	cpus := runtime.NumCPU()
	minerThreads = cpus - 4
	if minerThreads < 1 {
		minerThreads = 1
	}
	minerDaemonAddr = "127.0.0.1:10102"
}

// ringBuffer is a circular buffer for capturing process output.
type ringBuffer struct {
	mu    sync.Mutex
	lines []string
	size  int
	pos   int
	full  bool
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{lines: make([]string, size), size: size}
}

func (rb *ringBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		if line == "" {
			continue
		}
		rb.lines[rb.pos] = line
		rb.pos = (rb.pos + 1) % rb.size
		if rb.pos == 0 {
			rb.full = true
		}
	}
	return len(p), nil
}

func (rb *ringBuffer) Lines() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if !rb.full {
		result := make([]string, rb.pos)
		copy(result, rb.lines[:rb.pos])
		return result
	}
	result := make([]string, rb.size)
	copy(result, rb.lines[rb.pos:])
	copy(result[rb.size-rb.pos:], rb.lines[:rb.pos])
	return result
}

// daemonBinary returns the binary name for the current OS.
func daemonBinary() string {
	if runtime.GOOS == "windows" {
		return "derod.exe"
	}
	return "derod"
}

// minerBinary returns the miner binary name for the current OS.
func minerBinary() string {
	switch runtime.GOOS {
	case "darwin":
		return "dero-miner"
	case "windows":
		return "dirtybird.exe"
	default:
		return "dirtybird"
	}
}

// findBinary locates a binary: first in AppPath()/bin, then system PATH.
func findBinary(name string) string {
	local := filepath.Join(AppPath(), "bin", name)
	if _, err := os.Stat(local); err == nil {
		return local
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}
	return ""
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

// getDaemonArgs builds CLI arguments for derod.
func getDaemonArgs() []string {
	dataDir := daemonDataDir()
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

	args := []string{
		"--data-dir", dataDir,
		"--rpc-bind", fmt.Sprintf("127.0.0.1:%d", rpcPort),
		"--p2p-bind", fmt.Sprintf("0.0.0.0:%d", p2pPort),
		"--work-bind", fmt.Sprintf("0.0.0.0:%d", workPort),
	}

	for _, node := range prioritySeedNodes {
		args = append(args, "--add-priority-node", node)
	}

	return args
}

// startDaemon launches the derod process.
func startDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if daemonCmd != nil && daemonCmd.Process != nil {
		logger.Printf("[Daemon] Already running (pid %d)", daemonCmd.Process.Pid)
		return
	}

	binary := findBinary(daemonBinary())
	if binary == "" {
		logger.Printf("[Daemon] Binary not found: %s", daemonBinary())
		dmState.daemonState = dmStateError
		return
	}

	args := getDaemonArgs()
	dataDir := daemonDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Printf("[Daemon] Failed to create data dir %s: %v", dataDir, err)
		dmState.daemonState = dmStateError
		return
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, daemonLog)
	cmd.Stderr = io.MultiWriter(os.Stderr, daemonLog)

	if err := cmd.Start(); err != nil {
		logger.Printf("[Daemon] Failed to start: %v", err)
		dmState.daemonState = dmStateError
		return
	}

	daemonCmd = cmd
	dmState.daemonState = dmStateRunning
	logger.Printf("[Daemon] Started (pid %d, binary: %s)", cmd.Process.Pid, binary)

	go func() {
		err := cmd.Wait()
		daemonMu.Lock()
		daemonCmd = nil
		daemonMu.Unlock()

		if err != nil {
			logger.Printf("[Daemon] Exited: %v", err)
		} else {
			logger.Printf("[Daemon] Stopped")
		}
		if dmState.daemonState != dmStateStopped {
			dmState.daemonState = dmStateStopped
		}
	}()
}

// stopDaemon gracefully stops the daemon process.
func stopDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if daemonCmd == nil || daemonCmd.Process == nil {
		logger.Printf("[Daemon] Not running")
		return
	}

	pid := daemonCmd.Process.Pid
	logger.Printf("[Daemon] Stopping pid %d", pid)

	if runtime.GOOS != "windows" {
		daemonCmd.Process.Signal(syscall.SIGTERM)
	}

	done := make(chan error, 1)
	go func() {
		done <- daemonCmd.Wait()
	}()

	select {
	case <-done:
		logger.Printf("[Daemon] Stopped gracefully")
	case <-time.After(10 * time.Second):
		logger.Printf("[Daemon] Force killing pid %d", pid)
		daemonCmd.Process.Kill()
		<-done
	}

	daemonCmd = nil
	dmState.daemonState = dmStateStopped
}

// startMiner launches the miner process.
func startMiner() {
	minerMu.Lock()
	defer minerMu.Unlock()

	if minerCmd != nil && minerCmd.Process != nil {
		logger.Printf("[Miner] Already running (pid %d)", minerCmd.Process.Pid)
		return
	}

	binary := findBinary(minerBinary())
	if binary == "" {
		logger.Printf("[Miner] Binary not found: %s", minerBinary())
		dmState.minerState = dmStateError
		return
	}

	if minerWalletAddr == "" && engram.Disk != nil {
		minerWalletAddr = engram.Disk.GetAddress().String()
	}
	if minerWalletAddr == "" {
		logger.Printf("[Miner] No wallet address configured")
		dmState.minerState = dmStateError
		return
	}

	args := getMinerArgs(binary)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, minerLog)
	cmd.Stderr = io.MultiWriter(os.Stderr, minerLog)

	if err := cmd.Start(); err != nil {
		logger.Printf("[Miner] Failed to start: %v", err)
		dmState.minerState = dmStateError
		return
	}

	minerCmd = cmd
	dmState.minerState = dmStateRunning
	logger.Printf("[Miner] Started (pid %d, binary: %s, threads: %d)", cmd.Process.Pid, binary, minerThreads)

	go func() {
		err := cmd.Wait()
		minerMu.Lock()
		minerCmd = nil
		minerMu.Unlock()

		if err != nil {
			logger.Printf("[Miner] Exited: %v", err)
		} else {
			logger.Printf("[Miner] Stopped")
		}
		if dmState.minerState != dmStateStopped {
			dmState.minerState = dmStateStopped
		}
	}()
}

// getMinerArgs builds CLI arguments for the miner.
func getMinerArgs(binary string) []string {
	if runtime.GOOS == "darwin" {
		return []string{
			"--daemon-address", minerDaemonAddr,
			"--wallet-address", minerWalletAddr,
			"--threads", fmt.Sprintf("%d", minerThreads),
			"--mine-on-worker", "true",
		}
	}
	// Dirtybird on Linux / Windows
	args := []string{
		"--daemon-rpc-address", minerDaemonAddr,
		"--wallet-address", minerWalletAddr,
		"--threads", fmt.Sprintf("%d", minerThreads),
		"--mine-on-worker", "true",
	}
	if strings.Contains(binary, "dirtybird") || strings.Contains(binary, "DirtyBird") {
		args = []string{
			"--daemon-rpc-address", minerDaemonAddr,
			"--address", minerWalletAddr,
			"--threads", fmt.Sprintf("%d", minerThreads),
			"--mine-on-worker", "true",
		}
	}
	return args
}

// stopMiner gracefully stops the miner process.
func stopMiner() {
	minerMu.Lock()
	defer minerMu.Unlock()

	if minerCmd == nil || minerCmd.Process == nil {
		logger.Printf("[Miner] Not running")
		return
	}

	pid := minerCmd.Process.Pid
	logger.Printf("[Miner] Stopping pid %d", pid)

	if runtime.GOOS != "windows" {
		minerCmd.Process.Signal(syscall.SIGTERM)
	}

	done := make(chan error, 1)
	go func() {
		done <- minerCmd.Wait()
	}()

	select {
	case <-done:
		logger.Printf("[Miner] Stopped gracefully")
	case <-time.After(5 * time.Second):
		logger.Printf("[Miner] Force killing pid %d", pid)
		minerCmd.Process.Kill()
		<-done
	}

	minerCmd = nil
	dmState.minerState = dmStateStopped
}

// DaemonInfo holds daemon status data fetched via get_info RPC.
type DaemonInfo struct {
	Height     uint64 `json:"height"`
	Topoheight uint64 `json:"topoheight"`
	Difficulty uint64 `json:"difficulty"`
	Version    string `json:"version"`
	Network    string `json:"network"`
	InPeers    int    `json:"incoming_connections_count"`
	OutPeers   int    `json:"outgoing_connections_count"`
	TxPoolSize int    `json:"tx_pool_size"`
	Status     string `json:"status"`
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
		return DaemonInfo{}, fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	var result daemonInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DaemonInfo{}, fmt.Errorf("decode error: %w", err)
	}

	if result.Error != nil {
		return DaemonInfo{}, fmt.Errorf("RPC error (code %d): %s", result.Error.Code, result.Error.Message)
	}

	if result.Result == nil {
		return DaemonInfo{}, fmt.Errorf("empty RPC result")
	}

	daemonInfoMu.Lock()
	cachedDaemonInfo = *result.Result
	daemonInfoMu.Unlock()

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

// updateDaemonStateFromDetection refreshes dmState based on live checks.
// Called periodically and on page load.
func updateDaemonStateFromDetection() {
	localNode := checkLocalNode()
	if localNode {
		if daemonCmd != nil && daemonCmd.Process != nil {
			dmState.daemonState = dmStateRunning
		} else {
			dmState.daemonState = dmStateExternal
		}
	} else if isDaemonConnected() {
		dmState.daemonState = dmStateRunning
	} else if daemonCmd != nil && daemonCmd.Process != nil {
		dmState.daemonState = dmStateRunning
	} else {
		if detectChainCorruption() {
			dmState.daemonState = dmStateError
		} else {
			dmState.daemonState = dmStateStopped
		}
	}
}
