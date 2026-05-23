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
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	x "fyne.io/x/fyne/widget"
	"github.com/civilware/Gnomon/indexer"
	"github.com/civilware/Gnomon/rwc"
	"github.com/civilware/Gnomon/structures"
	"github.com/civilware/epoch"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	"github.com/gorilla/websocket"
	"mvdan.cc/xurls/v2"

	"github.com/civilware/Gnomon/storage"
	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/globals"

	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/transaction"

	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/mnemonics"
	"github.com/deroproject/derohe/walletapi/rpcserver"
	"github.com/deroproject/derohe/walletapi/xswd"

	"github.com/DEROFDN/engram/i18n"
)

type App struct {
	App    fyne.App
	Window fyne.Window
	Focus  bool
}

type UI struct {
	Padding   float32
	MaxWidth  float32
	Width     float32
	MaxHeight float32
	Height    float32
}

type Colors struct {
	Network    color.Color
	Account    color.Color
	Blue       color.Color
	Red        color.Color
	DarkGreen  color.Color
	Green      color.Color
	Gray       color.Color
	Yellow     color.Color
	DarkMatter color.Color
	Cold       color.Color
	Flint      color.Color
	Purple     color.Color
	LightBlue  color.Color
	SoftRed    color.Color
}

type Navigation struct {
	PosX float32
	PosY float32
	CurX float32
	CurY float32
}

type Session struct {
	Window              fyne.Window
	DesktopMode         bool
	Domain              string
	LastDomain          fyne.CanvasObject
	Network             string
	Offline             bool
	Language            int
	ID                  string
	Link                string
	Type                string
	Daemon              string
	WalletOpen          bool
	Username            string
	Datapad             string
	DatapadChanged      bool
	LastBalance         uint64
	Balance             uint64
	BalanceHidden       bool
	AddressHidden       bool
	BalanceUSD          string
	BalanceText         *canvas.Text
	BalanceUSDText      *canvas.Text
	ModeText            *canvas.Text
	IDText              *canvas.Text
	LinkText            *canvas.Text
	StatusText          *canvas.Text
	ReceivingAddress    string
	Path                string
	Name                string
	Password            string
	PasswordConfirm     string
	DaemonHeight        uint64
	WalletHeight        uint64
	RPCServer           *rpcserver.RPCServer
	Verified            bool
	Dashboard           string
	Error               string
	NewUser             string
	Gif                 *x.AnimatedGif
	RegHashes           int64
	LimitMessages       bool
	TrackRecentBlocks   int64
	NavStack            *NavigationStack
	Navigating          bool
	PreviousDomain      string
	VillagerHidden      bool
	VillagerBackground  bool
	VillagerPopupShown  bool
	MessageWarningShown bool
	VillagerAddress     string
	VillagerPixels      string
	IsRecovery          bool
	IsNewWallet         bool
}

type RemoteAccess struct {
	RPC struct {
		user     string
		pass     string
		port     string
		userText *widget.Entry
		passText *widget.Entry
		portText *widget.Entry
		toggle   *widget.Button
		status   *canvas.Text
		server   *rpcserver.RPCServer
	}
	WS struct {
		sync.RWMutex
		port     string
		portText *widget.Entry
		list     *widget.List
		toggle   *widget.Button
		status   *canvas.Text
		server   *xswd.XSWD
		apps     []xswd.ApplicationData
		advanced bool
		global   struct {
			connect     bool
			enabled     bool
			status      *canvas.Text
			permissions map[string]xswd.Permission
		}
	}
	EPOCH struct {
		enabled          bool
		allowWithAddress bool
		err              error
		total            epoch.GetSessionEPOCH_Result
	}
}

type INDEXwithRatings struct {
	ratings tela.Rating_Result
	tela.INDEX
}

// deduplicateTelaSearch removes duplicate SCIDs, keeping the first occurrence.
func deduplicateTelaSearch(search []INDEXwithRatings) []INDEXwithRatings {
	seen := make(map[string]struct{}, len(search))
	result := make([]INDEXwithRatings, 0, len(search))
	for _, entry := range search {
		if entry.SCID == "" {
			continue
		}
		if _, ok := seen[entry.SCID]; ok {
			continue
		}
		seen[entry.SCID] = struct{}{}
		result = append(result, entry)
	}
	return result
}

type telaDisplayCache []INDEXwithRatings

type Engram struct {
	Disk *walletapi.Wallet_Disk
}

type Theme struct {
	main eTheme
	alt  eTheme2
}

type Gnomon struct {
	Active   int
	Index    *indexer.Indexer
	BBolt    *storage.BboltStore
	Graviton *storage.GravitonStore
	Path     string
	bootMu   sync.RWMutex
	boot     GnomonBootstrapState
}

type GnomonBootstrapState struct {
	Phase     string
	Current   int64
	Total     int64
	Ready     bool
	Active    bool
	Err       string
	StartedAt time.Time
	UpdatedAt time.Time
}

var gnomonMu sync.Mutex

type ProofData struct {
	Receivers []string
	Amounts   []uint64
	Payloads  []string
}

type Status struct {
	Canvas       *canvas.Text
	Message      string
	Network      *canvas.Text
	Connection   *canvas.Circle
	Sync         *canvas.Circle
	RemoteAccess *canvas.Circle
	Gnomon       *canvas.Circle
	EPOCH        *canvas.Circle
}

type Transfers struct {
	Address    *rpc.Address
	PaymentID  uint64
	Amount     uint64
	Comment    string
	GasStorage uint64
	Fees       uint64
	Pending    []rpc.Transfer
	TX         *transaction.Transaction
	TXID       crypto.Hash
	Proof      string
	Ringsize   uint64
	SendAll    bool
	Size       float32
	Status     string
	OfflineTX  bool
	Filename   string
}

type TELAFavoriteData struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IconURL     string  `json:"icon_url"`
	Rating      float64 `json:"rating"`
	LastUpdated int64   `json:"last_updated"`
}

func shouldSkipWalletSave() bool {
	return true
}

type MessageBox struct {
	List *widget.List
	Data binding.ExternalStringList
}

var messageCacheMu sync.RWMutex
var addressDisplayCacheMu sync.RWMutex

var walletSessionMu sync.RWMutex
var walletSessionGeneration uint64
var pulseSessionGeneration uint64
var pulseRunning bool

func currentWalletGeneration() uint64 {
	walletSessionMu.RLock()
	defer walletSessionMu.RUnlock()
	return walletSessionGeneration
}

func isWalletGenerationActive(generation uint64) bool {
	walletSessionMu.RLock()
	defer walletSessionMu.RUnlock()
	return generation != 0 && generation == walletSessionGeneration && engram.Disk != nil && session.WalletOpen
}

func beginWalletSession() uint64 {
	walletSessionMu.Lock()
	defer walletSessionMu.Unlock()
	walletSessionGeneration++
	return walletSessionGeneration
}

func beginWalletShutdown() bool {
	walletSessionMu.Lock()
	defer walletSessionMu.Unlock()
	if !session.WalletOpen && engram.Disk == nil {
		return false
	}
	walletSessionGeneration++
	pulseRunning = false
	pulseSessionGeneration = 0
	return true
}

func finishWalletShutdown() {}

func startPulseForActiveWallet() bool {
	walletSessionMu.Lock()
	defer walletSessionMu.Unlock()
	logger.Printf("[DEBUG] startPulseForActiveWallet - engram.Disk=%v, session.WalletOpen=%v, pulseRunning=%v, pulseSessionGeneration=%d, walletSessionGeneration=%d\n",
		engram.Disk != nil, session.WalletOpen, pulseRunning, pulseSessionGeneration, walletSessionGeneration)
	if engram.Disk == nil || !session.WalletOpen {
		logger.Printf("[DEBUG] startPulseForActiveWallet returning false - Disk or WalletOpen check failed\n")
		return false
	}
	if pulseRunning && pulseSessionGeneration == walletSessionGeneration {
		logger.Printf("[DEBUG] startPulseForActiveWallet returning false - pulse already running for this session\n")
		return false
	}
	pulseRunning = true
	pulseSessionGeneration = walletSessionGeneration
	logger.Printf("[DEBUG] startPulseForActiveWallet returning true - pulse starting\n")
	return true
}

func finishPulseForGeneration(generation uint64) {
	walletSessionMu.Lock()
	defer walletSessionMu.Unlock()
	if pulseSessionGeneration == generation {
		pulseRunning = false
		pulseSessionGeneration = 0
	}
}

var appExitFlag atomic.Bool
var telaViewActive atomic.Bool
var forceFreshScan bool

type Messages struct {
	Contact string
	Address string
	Data    []string
	Height  uint64
	Message string
}

// PermissionGroup represents a grouped set of permission methods for Simple Mode
type PermissionGroup struct {
	Name        string
	Description string
	Methods     []string
	SimpleMode  bool   // Show in simple mode
	Category    string // "transactions", "readonly", "sensitive", "mining", "tela", "utility"
}

// PermissionPresets defines default behaviors for Simple and Advanced modes
type PermissionPresets struct {
	Simple   map[string]xswd.Permission // Simple mode defaults
	Advanced map[string]xswd.Permission // Advanced mode defaults (current behavior)
}

// PermissionGroups defines the 6 user-friendly groups for Simple Mode
var permissionGroups = []PermissionGroup{
	{
		Name:        "Send Transactions",
		Description: "Allow apps to send DERO/assets from your wallet",
		Methods:     []string{"Transfer", "transfer_split", "scinvoke"},
		SimpleMode:  true,
		Category:    "transactions",
	},
	{
		Name:        "View Wallet Info",
		Description: "Allow apps to see your address and balance",
		Methods: []string{
			"GetAddress", "GetAddressEPOCH", "getaddress",
			"GetBalance", "getbalance",
			"GetHeight", "getheight",
			"MakeIntegratedAddress", "make_integrated_address",
			"SplitIntegratedAddress", "split_integrated_address",
		},
		SimpleMode: true,
		Category:   "readonly",
	},
	{
		Name:        "View Transaction History",
		Description: "Allow apps to see your transaction records",
		Methods: []string{
			"GetTransfers", "get_transfers",
			"GetTransferbyTXID", "get_transfer_by_txid",
		},
		SimpleMode: true,
		Category:   "sensitive",
	},
	{
		Name:        "Sign Messages",
		Description: "Allow apps to request cryptographic signatures",
		Methods: []string{
			"SignData", "CheckSignature",
			"QueryKey", "query_key",
		},
		SimpleMode: true,
		Category:   "sensitive",
	},
	{
		Name:        "EPOCH Mining",
		Description: "Allow apps to use your device for mining",
		Methods: []string{
			"AttemptEPOCH", "AttemptEPOCHWithAddr",
			"SubmitEPOCH", "GetSessionEPOCH", "GetMaxHashesEPOCH",
		},
		SimpleMode: true,
		Category:   "mining",
	},
	{
		Name:        "TELA & Links",
		Description: "Allow apps to open TELA links and content",
		Methods: []string{
			"HandleTELALinks", "Subscribe", "Unsubscribe",
		},
		SimpleMode: true,
		Category:   "tela",
	},
	// Hidden groups (SimpleMode: false) - auto-handled
	{
		Name:        "Utility Methods",
		Description: "Read-only utility methods (auto-allowed)",
		Methods:     []string{"Echo", "HasMethod", "GetDaemon", "GetPrimaryUsername"},
		SimpleMode:  false,
		Category:    "utility",
	},
}

type InstallContract struct {
	TXID string
}

type Client struct {
	WS  *websocket.Conn
	RPC *jrpc2.Client
}

type ScanProgress struct {
	Position  int    `json:"position"`
	Total     int    `json:"total"`
	LastSCID  string `json:"last_scid"`
	State     string `json:"state"` // "syncing", "scanning", "completed", "interrupted"
	Timestamp int64  `json:"timestamp"`
}

type indexCache map[string]tela.INDEX

type telaCandidateCache map[string]telaCandidateMeta

type telaCandidateMeta struct {
	LastCheckedHeight int64  `json:"last_checked_height"`
	Result            string `json:"result"`
}

const (
	telaCandidateValidIndex    = "valid_index"
	telaCandidateNotTela       = "not_tela_version"
	telaCandidateInvalidIndex  = "invalid_index"
	telaCandidateNoDocs        = "no_docs"
	telaCandidateExcludedByURL = "excluded_by_durl"
)

type batchPrefilterStats struct {
	VersionHits int64
	Dropped     int64
	Retries     int64
}

func (c telaCandidateCache) set(scid, result string, height int64) {
	if c == nil || scid == "" {
		return
	}
	c[scid] = telaCandidateMeta{LastCheckedHeight: height, Result: result}
}

func (c telaCandidateCache) validSCIDs() []string {
	valid := make([]string, 0, len(c))
	for scid, meta := range c {
		if meta.Result == telaCandidateValidIndex {
			valid = append(valid, scid)
		}
	}
	sort.Strings(valid)
	return valid
}

func (c telaCandidateCache) negativeSet() map[string]bool {
	set := map[string]bool{}
	for scid, meta := range c {
		switch meta.Result {
		case telaCandidateNotTela, telaCandidateInvalidIndex, telaCandidateNoDocs:
			set[scid] = true
		}
	}
	return set
}

func loadStringSetFromEncryptedStorage(bucket, key string) map[string]bool {
	set := map[string]bool{}
	raw, err := GetEncryptedValue(bucket, []byte(key))
	if err != nil || len(raw) == 0 {
		return set
	}

	var entries []string
	if err := json.Unmarshal(raw, &entries); err != nil {
		logger.Printf("[TELA] Failed decoding %s cache: %v\n", key, err)
		return set
	}

	for _, entry := range entries {
		if entry != "" {
			set[entry] = true
		}
	}

	return set
}

func saveStringSetToEncryptedStorage(bucket, key string, set map[string]bool) error {
	entries := make([]string, 0, len(set))
	for k, ok := range set {
		if ok && k != "" {
			entries = append(entries, k)
		}
	}
	sort.Strings(entries)
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	return StoreEncryptedValue(bucket, []byte(key), data)
}

func loadTelaIndexCache() indexCache {
	cache := indexCache{}
	raw, err := GetEncryptedValue("TELA Search", []byte("IndexCache"))
	if err != nil || len(raw) == 0 {
		return cache
	}

	if err := json.Unmarshal(raw, &cache); err != nil {
		logger.Printf("[TELA] Failed decoding INDEX cache: %v\n", err)
		return indexCache{}
	}

	return cache
}

func saveTelaIndexCache(cache indexCache) error {
	if cache == nil {
		cache = indexCache{}
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return StoreEncryptedValue("TELA Search", []byte("IndexCache"), data)
}

func loadTelaDisplayCache() telaDisplayCache {
	cache := telaDisplayCache{}
	raw, err := GetEncryptedValue("TELA Search", []byte("DisplayCache"))
	if err != nil || len(raw) == 0 {
		return cache
	}

	if err := json.Unmarshal(raw, &cache); err != nil {
		logger.Printf("[TELA] Failed decoding display cache: %v\n", err)
		return telaDisplayCache{}
	}

	return cache
}

func saveTelaDisplayCache(cache telaDisplayCache) error {
	if cache == nil {
		cache = telaDisplayCache{}
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return StoreEncryptedValue("TELA Search", []byte("DisplayCache"), data)
}

func loadTelaCandidateCache() telaCandidateCache {
	cache := telaCandidateCache{}
	raw, err := GetEncryptedValue("TELA Search", []byte("CandidateCache"))
	if err != nil || len(raw) == 0 {
		return cache
	}

	if err := json.Unmarshal(raw, &cache); err != nil {
		logger.Printf("[TELA] Failed decoding candidate cache: %v\n", err)
		return telaCandidateCache{}
	}

	return cache
}

func saveTelaCandidateCache(cache telaCandidateCache) error {
	if cache == nil {
		cache = telaCandidateCache{}
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return StoreEncryptedValue("TELA Search", []byte("CandidateCache"), data)
}

// dnsCache stores resolved addresses to avoid repeated DNS lookups.
var dnsCache = struct {
	sync.Mutex
	cache map[string]dnsEntry
}{
	cache: make(map[string]dnsEntry),
}

type dnsEntry struct {
	ip        string
	expiresAt time.Time
}

// resolveWithCache resolves a hostname to an IP address with caching.
// Returns the original endpoint if resolution fails or cache is expired.
func resolveWithCache(endpoint string) string {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint // Not a host:port format
	}

	// Check if host is already an IP address
	if ip := net.ParseIP(host); ip != nil {
		return endpoint
	}

	dnsCache.Lock()
	entry, exists := dnsCache.cache[host]
	if exists && time.Now().Before(entry.expiresAt) {
		resolved := entry.ip + ":" + port
		dnsCache.Unlock()
		return resolved
	}
	dnsCache.Unlock()

	// Resolve and cache
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return endpoint // Return original on failure
	}

	resolved := addrs[0] + ":" + port
	dnsCache.Lock()
	dnsCache.cache[host] = dnsEntry{
		ip:        addrs[0],
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	dnsCache.Unlock()

	logger.Printf("[DNS] Resolved %s -> %s (cached for 5m)\n", host, addrs[0])
	return resolved
}

// dialRPCPool creates n independent jrpc2 websocket connections to the DERO daemon.
// Each connection has its own websocket and jrpc2.Client for true parallel RPC pipelines.
// Caller must close all connections when done via the returned cleanup function.
func dialRPCPool(endpoint string, n int) ([]*jrpc2.Client, func(), error) {
	clients := make([]*jrpc2.Client, 0, n)
	conns := make([]*websocket.Conn, 0, n)

	// Use DNS cache to avoid repeated lookups
	resolvedEndpoint := resolveWithCache(endpoint)
	uri := "ws://" + resolvedEndpoint + "/ws"

	cleanup := func() {
		// Close websockets first to unblock jrpc2's background reader goroutines,
		// then close jrpc2 clients. Reversing this order causes Close() to hang
		// because jrpc2.Client.Close() calls done.Wait() on the reader goroutine
		// which is stuck in websocket.NextReader().
		for i := range conns {
			conns[i].Close()
		}
		for i := range clients {
			clients[i].Close()
		}
	}

	for i := 0; i < n; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(uri, nil)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("dial pool connection %d/%d: %w", i+1, n, err)
		}
		io := rwc.New(ws)
		clients = append(clients, jrpc2.NewClient(channel.RawJSON(io, io), nil))
		conns = append(conns, ws)
	}
	return clients, cleanup, nil
}

func batchPrefilterTelaVersions(ctx context.Context, scids []string, batchSize, maxRetries int, pool []*jrpc2.Client, onProgress func(completed, total int)) (map[string]bool, batchPrefilterStats, error) {
	passed := make(map[string]bool)
	stats := batchPrefilterStats{}

	if len(scids) == 0 {
		return passed, stats, nil
	}
	if batchSize < 1 {
		batchSize = 1000
	}
	if maxRetries < 1 {
		maxRetries = 3
	}

	if len(pool) == 0 {
		return nil, stats, fmt.Errorf("rpc pool is empty")
	}

	chainHeight := gnomon.Index.ChainHeight
	maxConcurrency := len(pool)

	type batchJob struct {
		items []string
	}
	type batchResult struct {
		passed  map[string]bool
		dropped int64
		retries int64
		err     error
	}

	totalBatches := (len(scids) + batchSize - 1) / batchSize
	jobs := make(chan batchJob, totalBatches)
	results := make(chan batchResult, totalBatches)
	var wg sync.WaitGroup

	worker := func(myClient *jrpc2.Client) {
		defer wg.Done()
		for job := range jobs {
			// Check for cancellation before starting work
			select {
			case <-ctx.Done():
				results <- batchResult{passed: make(map[string]bool), err: ctx.Err()}
				return
			default:
			}

			result := batchResult{passed: make(map[string]bool), dropped: int64(len(job.items))}
			var responses []*jrpc2.Response
			var err error

			for attempt := 0; attempt < maxRetries; attempt++ {
				// Check cancellation before each attempt
				select {
				case <-ctx.Done():
					result.err = ctx.Err()
					results <- result
					return
				default:
				}

				specs := make([]jrpc2.Spec, 0, len(job.items))
				for _, scid := range job.items {
					specs = append(specs, jrpc2.Spec{
						Method: "DERO.GetSC",
						Params: rpc.GetSC_Params{
							SCID:       scid,
							Variables:  false,
							TopoHeight: chainHeight,
							KeysString: []string{"telaVersion"},
						},
					})
				}

				batchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				responses, err = myClient.Batch(batchCtx, specs)
				cancel()
				if err == nil {
					break
				}

				result.retries++
				if !isConnectionError(err) || attempt >= maxRetries-1 {
					break
				}
				// Exponential backoff with jitter: base * 2^attempt ± 25%
				baseMs := 1000 << uint(attempt)
				jitterMs := baseMs / 4
				jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(jitterMs*2)))
				sleepMs := int64(baseMs) - int64(jitterMs) + jitter.Int64()
				time.Sleep(time.Duration(sleepMs) * time.Millisecond)
			}

			if err != nil {
				result.err = err
				results <- result
				continue
			}

			for i, resp := range responses {
				if i >= len(job.items) || resp == nil || resp.Error() != nil {
					continue
				}

				var out rpc.GetSC_Result
				if err := resp.UnmarshalResult(&out); err != nil {
					continue
				}

				// With Variables:false and KeysString:["telaVersion"],
				// the daemon returns the value in ValuesString[0].
				// When a key doesn't exist, the daemon returns "NOT AVAILABLE err: ..."
				// so we must exclude that to only pass SCIDs with an actual telaVersion value.
				scid := job.items[i]
				if len(out.ValuesString) > 0 && out.ValuesString[0] != "" && !strings.HasPrefix(out.ValuesString[0], "NOT AVAILABLE") {
					if !result.passed[scid] {
						result.passed[scid] = true
						result.dropped--
					}
				}
			}

			results <- result
		}
	}

	workers := maxConcurrency
	if workers > len(scids) {
		workers = len(scids)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(pool[i])
	}

	go func() {
		for i := 0; i < len(scids); i += batchSize {
			// Check cancellation before sending the next job
			select {
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				close(results)
				return
			default:
			}

			end := i + batchSize
			if end > len(scids) {
				end = len(scids)
			}
			jobs <- batchJob{items: scids[i:end]}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var firstErr error
	completedBatches := 0
	for result := range results {
		completedBatches++
		stats.Retries += result.retries
		stats.Dropped += result.dropped
		for scid := range result.passed {
			if !passed[scid] {
				passed[scid] = true
				stats.VersionHits++
			}
		}

		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}

		// Report progress after each batch
		if onProgress != nil {
			onProgress(completedBatches, totalBatches)
		}
	}

	if stats.Dropped < 0 {
		stats.Dropped = 0
	}

	return passed, stats, firstErr
}

// decodeHex decodes a hex string, returning the original if decoding fails.
func decodeHex(s string) string {
	if decoded, err := hex.DecodeString(s); err == nil {
		return string(decoded)
	}
	return s
}

// getHeaderFromVars tries v2Key first, then v1Key, hex-decoding the result.
func getHeaderFromVars(vars map[string]interface{}, v2Key, v1Key string) string {
	if v, ok := vars[v2Key].(string); ok {
		return decodeHex(v)
	}
	if v, ok := vars[v1Key].(string); ok {
		return decodeHex(v)
	}
	return ""
}

// parseINDEXDOCs extracts DOC SCIDs from a parsed DVM smart contract.
// Reimplements unexported tela.parseINDEXForDOCs.
func parseINDEXDOCs(sc dvm.SmartContract) []string {
	var docKeys []string
	docMap := map[string]string{}
	for name, function := range sc.Functions {
		if name == "InitializePrivate" {
			for _, line := range function.Lines {
				for i, parts := range line {
					if strings.Contains(parts, `"DOC`) {
						if i+2 < len(line) {
							scid := strings.Trim(line[i+2], `"`)
							docKeys = append(docKeys, parts)
							docMap[parts] = scid
						}
					}
				}
			}
		}
	}
	sort.Strings(docKeys)
	scids := make([]string, 0, len(docKeys))
	for _, v := range docKeys {
		scids = append(scids, docMap[v])
	}
	return scids
}

// buildINDEXFromVars constructs a tela.INDEX from pre-fetched SC variables,
// avoiding the per-SCID WebSocket connection that tela.GetINDEXInfo creates.
func buildINDEXFromVars(scid string, vars map[string]interface{}) (tela.INDEX, error) {
	var index tela.INDEX

	// SC code is required
	c, ok := vars["C"].(string)
	if !ok {
		return index, fmt.Errorf("could not get SC code from %s", scid)
	}

	var modTag string
	if storedMods, ok := vars["mods"].(string); ok {
		modTag = decodeHex(storedMods)
	}

	// Decode hex code
	code := decodeHex(c)

	// Validate INDEX version (exported, pure computation)
	sc, version, err := tela.ValidINDEXVersion(code, modTag)
	if err != nil {
		logger.Debugf("[TELA] SCID %s does not parse as TELA-INDEX-1 (code len: %d): %v\n", scid, len(code), err)
		return index, fmt.Errorf("scid does not parse as TELA-INDEX-1: %s", err)
	}

	// dURL is required
	d, ok := vars["dURL"].(string)
	if !ok {
		logger.Debugf("[TELA] SCID %s is missing dURL variable\n", scid)
		return index, fmt.Errorf("could not get dURL from %s", scid)
	}
	dURL := decodeHex(d)

	// Headers with V2 -> V1 fallback
	nameHdr := getHeaderFromVars(vars, "var_header_name", "nameHdr")
	descrHdr := getHeaderFromVars(vars, "var_header_description", "descrHdr")
	iconHdr := getHeaderFromVars(vars, "var_header_icon", "iconURLHdr")

	author := "anon"
	if addr, ok := vars["owner"].(string); ok {
		author = decodeHex(addr)
	}

	// Parse DOCs from SC code (reimplements unexported tela.parseINDEXForDOCs)
	docs := parseINDEXDOCs(sc)

	index = tela.INDEX{
		Mods:      modTag,
		SCID:      scid,
		Author:    author,
		DURL:      dURL,
		DOCs:      docs,
		SCVersion: &version,
		SC:        sc,
		Headers: tela.Headers{
			NameHdr:  nameHdr,
			DescrHdr: descrHdr,
			IconHdr:  iconHdr,
		},
	}

	return index, nil
}

// batchFetchINDEXes fetches SC variables for multiple SCIDs in batched RPC calls
// and constructs tela.INDEX for each, using Gnomon's existing connection.
// This replaces individual tela.GetINDEXInfo() calls which each open a new WebSocket.
func batchFetchINDEXes(ctx context.Context, scids []string, batchSize int) (map[string]tela.INDEX, map[string]tela.Rating_Result, map[string]bool, error) {
	result := make(map[string]tela.INDEX, len(scids))
	ratings := make(map[string]tela.Rating_Result, len(scids))
	invalid := make(map[string]bool)
	var batchErr error
	var resultMu sync.Mutex
	if len(scids) == 0 {
		return result, ratings, invalid, nil
	}
	if batchSize < 1 {
		batchSize = 50
	}

	retryableFetchError := func(err error) bool {
		if err == nil {
			return false
		}
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "closed network connection") ||
			strings.Contains(msg, "use of closed network connection") ||
			strings.Contains(msg, "broken pipe") ||
			strings.Contains(msg, "connection reset by peer") ||
			strings.Contains(msg, "eof") ||
			strings.Contains(msg, "timeout") ||
			strings.Contains(msg, "deadline exceeded")
	}

	// Use multiple RPC connections for parallel batch processing
	workerCount := 4
	if len(scids) < workerCount*batchSize {
		workerCount = 1
	}
	pool, cleanup, err := dialRPCPool(session.Daemon, workerCount)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to dial rpc pool: %v", err)
	}
	defer cleanup()

	// Create work queue of batches
	type batchWork struct {
		offset int
		scids  []string
	}
	var batches []batchWork
	for i := 0; i < len(scids); i += batchSize {
		end := i + batchSize
		if end > len(scids) {
			end = len(scids)
		}
		batches = append(batches, batchWork{offset: i, scids: scids[i:end]})
	}

	// Process batches in parallel using worker pool
	var wg sync.WaitGroup
	batchChan := make(chan batchWork, len(batches))
	for _, b := range batches {
		batchChan <- b
	}
	close(batchChan)

	for w := 0; w < workerCount && w < len(pool); w++ {
		rpcClient := pool[w]
		wg.Add(1)
		go func(client *jrpc2.Client) {
			defer wg.Done()
			for work := range batchChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				batch := work.scids
				specs := make([]jrpc2.Spec, len(batch))
				for j, scid := range batch {
					specs[j] = jrpc2.Spec{
						Method: "DERO.GetSC",
						Params: rpc.GetSC_Params{
							SCID:      scid,
							Variables: true,
							Code:      true,
						},
					}
				}

				var responses []*jrpc2.Response
				var err error
				for attempt := 0; attempt < 3; attempt++ {
					batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
					responses, err = client.Batch(batchCtx, specs)
					cancel()
					if err == nil {
						break
					}
					if !retryableFetchError(err) || attempt == 2 {
						break
					}
					logger.Printf("[TELA] Batch INDEX fetch retry %d for offset %d after rpc error: %v\n", attempt+1, work.offset, err)
				}
				if err != nil {
					logger.Printf("[TELA] Batch INDEX fetch error at offset %d: %v\n", work.offset, err)
					resultMu.Lock()
					if batchErr == nil {
						batchErr = err
					}
					resultMu.Unlock()
					continue
				}

				for j, resp := range responses {
					if j >= len(batch) || resp == nil {
						continue
					}
					if resp.Error() != nil {
						logger.Printf("[TELA] Batch INDEX fetch result error for %s: %v\n", batch[j], resp.Error())
						continue
					}

					var out rpc.GetSC_Result
					if err := resp.UnmarshalResult(&out); err != nil {
						continue
					}

					scid := batch[j]
					index, err := buildINDEXFromVars(scid, out.VariableStringKeys)
					if err != nil {
						resultMu.Lock()
						invalid[scid] = true
						resultMu.Unlock()
						continue
					}

					// Extract likes/dislikes and individual ratings while we have the variables
					var r tela.Rating_Result
					var ratingSum uint64
					var ratingCount uint64

					for k, v := range out.VariableStringKeys {
						switch k {
						case "likes":
							if f, ok := v.(float64); ok {
								r.Likes = uint64(f)
							} else if i, ok := v.(uint64); ok {
								r.Likes = i
							}
						case "dislikes":
							if f, ok := v.(float64); ok {
								r.Dislikes = uint64(f)
							} else if i, ok := v.(uint64); ok {
								r.Dislikes = i
							}
						default:
							// Key might be an address rating (dero... or deto...)
							if len(k) >= 60 && (strings.HasPrefix(k, "dero") || strings.HasPrefix(k, "deto")) {
								if rStr, ok := v.(string); ok {
									decoded := decodeHex(rStr)
									split := strings.Split(decoded, "_")
									if len(split) >= 2 {
										if ratingVal, err := strconv.ParseUint(split[0], 10, 64); err == nil {
											if h, err := strconv.ParseUint(split[1], 10, 64); err == nil {
												ratingSum += ratingVal
												ratingCount++
												r.Ratings = append(r.Ratings, tela.Rating{
													Address: k,
													Rating:  ratingVal,
													Height:  h,
												})
											}
										}
									}
								}
							}
						}
					}

					if ratingCount > 0 {
						r.Average = float64(ratingSum) / (float64(ratingCount) * 10)
						if r.Average <= 0 {
							r.Average = 0.01
						}
					}

					resultMu.Lock()
					result[scid] = index
					ratings[scid] = r
					resultMu.Unlock()
				}
			}
		}(rpcClient)
	}

	wg.Wait()

	if batchErr != nil {
		return result, ratings, invalid, batchErr
	}

	return result, ratings, invalid, nil
}

// Get the Engram settings from the local Graviton tree
func initSettings() {
	logger.Printf("[Engram] initSettings() called - starting initialization")
	getNetwork()
	getMode()
	getDaemon()
	getRPCCredentials()

	// Load and apply TELA settings on startup
	initTELAPreferences()

	// Initialize EPOCH max hashes to 10000 (hard limit) to support dApps like Dero Beats
	// Default from epoch package is 1000, which causes "hashes exceeds maxHashes" errors
	if err := epoch.SetMaxHashes(10000); err != nil {
		logger.Errorf("[Engram] Failed to set EPOCH max hashes: %s\n", err)
	} else {
		logger.Printf("[Engram] EPOCH max hashes set to 10000 (was %d)", 1000)
	}

	logger.Printf("[Engram] initSettings() completed")

	if a.Driver().Device().IsMobile() {
		err := tela.SetShardPath(AppPath())
		if err != nil {
			logger.Errorf("[Engram] Setting TELA shard: %s\n", err)
			return
		}
	}
}

// Store TELA setting with dual storage (encrypted + unencrypted fallback)
func setTELADual(key string, value []byte) {
	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err := StoreEncryptedValue("TELA Settings", []byte(key), value)
		if err != nil {
			logger.Debugf("[Engram] setTELADual encrypted storage failed for %s: %s\n", key, err)
		} else {
			logger.Printf("[Engram] setTELADual: Successfully saved %s to encrypted storage", key)
		}
	}

	// Always save to unencrypted storage as fallback
	err := StoreValue("TELASettingsUnencrypted", []byte(key), value)
	if err != nil {
		logger.Debugf("[Engram] setTELADual unencrypted storage failed for %s: %s\n", key, err)
	} else {
		logger.Printf("[Engram] setTELADual: Successfully saved %s to fallback storage", key)
	}
}

// Get TELA setting with dual storage (try encrypted first, then fallback)
func getTELADual(key string) (value string, found bool) {
	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("TELA Settings", []byte(key))
		if err == nil && stored != nil {
			logger.Printf("[Engram] getTELADual: Successfully loaded %s from encrypted storage", key)
			return string(stored), true
		} else if err != nil {
			logger.Debugf("[Engram] getTELADual encrypted storage failed for %s: %s\n", key, err)
		}
	}

	// Fallback to unencrypted storage
	stored, err := GetValue("TELASettingsUnencrypted", []byte(key))
	if err == nil && stored != nil {
		logger.Printf("[Engram] getTELADual: Successfully loaded %s from fallback storage", key)
		return string(stored), true
	} else if err != nil {
		logger.Debugf("[Engram] getTELADual fallback storage failed for %s: %s\n", key, err)
	}

	logger.Printf("[Engram] getTELADual: No stored value found for %s", key)
	return "", false
}

// Delete TELA setting from dual storage
func deleteTELADual(key string) {
	if engram.Disk != nil {
		DeleteKey("TELA Settings", []byte(key))
	}
	DeleteKey("TELASettingsUnencrypted", []byte(key))
	logger.Printf("[Engram] deleteTELADual: Deleted %s from dual storage", key)
}

// Initialize TELA preferences from dual storage (works with or without wallet)
func initTELAPreferences() {
	logger.Printf("[Engram] initTELAPreferences() called - wallet available: %v", engram.Disk != nil)

	// Load and apply TELA Port Start using dual storage
	if portStart, found := getTELADual("Port Start"); found {
		if p, err := strconv.Atoi(portStart); err == nil {
			if err := tela.SetPortStart(p); err != nil {
				logger.Errorf("[Engram] Failed to set TELA port start: %s\n", err)
			} else {
				logger.Printf("[Engram] TELA Port Start applied: %d", p)
			}
		}
	} else {
		logger.Printf("[Engram] TELA Port Start not found, using default")
	}

	// Load and apply Allow Updates setting using dual storage
	if allowUpdates, found := getTELADual("Allow Updates"); found {
		tela.AllowUpdates(allowUpdates == "Allow")
		logger.Printf("[Engram] TELA Allow Updates applied: %s", allowUpdates)
	} else {
		// Default to Allow when no stored value exists
		tela.AllowUpdates(true)
		logger.Printf("[Engram] TELA Allow Updates not found, defaulting to Allow")
	}

	// Load and apply Restrictive Mode setting using dual storage
	if restrictiveMode, found := getTELADual("Restrictive Mode"); found {
		// TODO: Apply restrictive mode to tela package when function is available
		logger.Printf("[Engram] TELA Restrictive Mode setting loaded: %s", restrictiveMode)
	} else {
		logger.Printf("[Engram] TELA Restrictive Mode not found, using default (Disabled)")
	}

	// Load Villager preferences
	if pref, found := getTELADual("VillagerHidden"); found {
		session.VillagerHidden = pref == "true"
	} else {
		session.VillagerHidden = false
	}

	if bgPref, found := getTELADual("VillagerBackground"); found {
		session.VillagerBackground = bgPref == "true"
	} else {
		session.VillagerBackground = true
	}
}

// Initialize WebSocket state from dual storage (works with or without wallet)
func initWebSocketState() {
	logger.Printf("[Engram] initWebSocketState() called - wallet available: %v", engram.Disk != nil)

	// Load stored WebSocket port using dual storage (works without wallet)
	if wsPort := getRemoteAccessDual("WS"); wsPort != "" {
		remoteAccess.WS.port = wsPort
		if remoteAccess.WS.portText != nil {
			uiDo(func() {
				if remoteAccess.WS.portText != nil {
					remoteAccess.WS.portText.SetText(wsPort)
				}
			})
		}
		logger.Printf("[Engram] WebSocket port loaded from dual storage: %s", wsPort)
	} else {
		logger.Printf("[Engram] No WebSocket port found in storage")
	}

	// Load global permissions (includes WebSocket enabled state)
	getPermissions()

	// Load RPC credentials from regular settings (works without wallet)
	getRPCCredentials()

	// Update UI based on loaded enabled state
	if remoteAccess.WS.toggle != nil && !session.Offline {
		logger.Printf("[Engram] Updating WebSocket UI based on loaded state: enabled=%v", remoteAccess.WS.global.enabled)
		uiDo(func() {
			if remoteAccess.WS.toggle == nil {
				return
			}
			if remoteAccess.WS.global.enabled {
				remoteAccess.WS.toggle.Text = "Turn On"
				logger.Printf("[Engram] Set toggle text to 'Turn On' for enabled=true")
			} else {
				if remoteAccess.WS.status != nil {
					remoteAccess.WS.status.Text = "Blocked"
					remoteAccess.WS.status.Color = colors.Gray
				}
				remoteAccess.WS.toggle.Text = "Turn On"
				logger.Printf("[Engram] Set toggle text to 'Turn On' for enabled=false")
			}
		})
	}

	// If WebSocket was previously enabled, restart it
	if remoteAccess.WS.global.enabled && !session.Offline {
		logger.Printf("[Engram] Attempting to restart WebSocket (was previously enabled)")
		EnsureXSWD()
	} else {
		logger.Printf("[Engram] Not restarting WebSocket - enabled=%v or offline=%v", remoteAccess.WS.global.enabled, session.Offline)
	}
}

func setPulseDisconnectedStatus(refresh bool) {
	uiDo(func() {
		status.Connection.FillColor = colors.Red
		status.Sync.FillColor = colors.Red
		status.Gnomon.FillColor = colors.Red
		status.EPOCH.FillColor = colors.Red
		if refresh {
			status.Connection.Refresh()
			status.Sync.Refresh()
			status.Gnomon.Refresh()
			status.EPOCH.Refresh()
		}
	})
}

func refreshPermissionsAfterConnect() {
	go func() {
		time.Sleep(time.Second * 2)
		uiDo(func() {
			_, _ = getPermissions()
		})
	}()
}

func pulseReconnect(count int) (int, bool) {
	logger.Printf("[Network] Attempting network connection to: %s\n", walletapi.Daemon_Endpoint)
	err := walletapi.Connect(session.Daemon)
	if err != nil {
		if count >= DEFAULT_DAEMON_RECONNECT_TIMEOUT {
			walletapi.Connected = false
			setPulseDisconnectedStatus(true)
			return count, false
		}

		count++
		logger.Errorf("[Network] Failed to connect to: %s (%d / %d)\n", walletapi.Daemon_Endpoint, count, DEFAULT_DAEMON_RECONNECT_TIMEOUT)
		walletapi.Connected = false
		setPulseDisconnectedStatus(false)
		time.Sleep(time.Second)
		return count, true
	}

	time.Sleep(time.Second)
	session.Offline = false
	return 0, false
}

func updatePulseStatusIndicators() {
	if walletapi.IsDaemonOnline() {
		uiDo(func() {
			status.Connection.FillColor = colors.Green
			if session.DaemonHeight > 0 && session.DaemonHeight-session.WalletHeight < 2 {
				status.Connection.FillColor = colors.Green
				status.Sync.FillColor = colors.Green
			} else if session.DaemonHeight == 0 {
				status.Sync.FillColor = colors.Red
			} else {
				status.Sync.FillColor = color.Transparent
			}

			if gnomon.Index != nil {
				if gnomon.Index.Status == "indexed" {
					status.Gnomon.FillColor = colors.Green
				} else if uint64(gnomon.Index.LastIndexedHeight) < session.WalletHeight-15 {
					status.Gnomon.FillColor = colors.Red
				} else {
					status.Gnomon.FillColor = color.Transparent
				}
			} else {
				status.Gnomon.FillColor = colors.Gray
			}

			if epoch.IsActive() {
				if epoch.IsProcessing() {
					status.EPOCH.FillColor = color.Transparent
				} else {
					status.EPOCH.FillColor = colors.Green
				}
			} else if remoteAccess.EPOCH.err != nil {
				status.EPOCH.FillColor = colors.Red
			} else {
				status.EPOCH.FillColor = colors.Gray
			}
		})
		return
	}

	uiDo(func() {
		status.Connection.FillColor = colors.Gray
		status.Sync.FillColor = colors.Gray
		status.RemoteAccess.FillColor = colors.Gray
		status.Gnomon.FillColor = colors.Gray
		status.EPOCH.FillColor = colors.Gray
	})
	logger.Printf("[Network] Offline › Last Height: %d / %d\n", session.WalletHeight, session.DaemonHeight)
}

func refreshPulseWalletState(sentNotifications *bool) {
	if engram.Disk == nil || !session.WalletOpen {
		return
	}

	if session.WalletHeight != engram.Disk.Get_Height() {
		*sentNotifications = false
	}

	session.Balance, _ = engram.Disk.Get_Balance()
	session.WalletHeight = engram.Disk.Get_Height()
	session.DaemonHeight = engram.Disk.Get_Daemon_Height()
	session.LastBalance = session.Balance

	updatePulseStatusIndicators()

	if gnomon.Index == nil && engram.Disk != nil {
		enableGnomon, _ := getGnomon()
		if enableGnomon == "1" {
			startGnomon()
		}
	}

	var zeroscid crypto.Hash
	entries := engram.Disk.Show_Transfers(zeroscid, false, true, false, session.WalletHeight-1, session.WalletHeight-1, "", "", uint64(1337), 0)
	for e := range entries {
		if entries[e].Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) && !*sentNotifications {
			sender := entries[e].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string)
			notification := fyne.NewNotification(sender, "New message was received (Height: "+fmt.Sprintf("%d", entries[e].Height)+")")
			fyne.CurrentApp().SendNotification(notification)
			*sentNotifications = true
		}
	}

	uiDo(func() {
		if engram.Disk == nil || !session.WalletOpen {
			return
		}
		if session.BalanceText != nil && !session.BalanceHidden {
			session.BalanceText.Text = globals.FormatMoney(session.Balance)
			session.BalanceText.Refresh()
		}
		if session.StatusText != nil {
			session.StatusText.Text = fmt.Sprintf("%d", session.WalletHeight)
			session.StatusText.Refresh()
		}
		status.Connection.Refresh()
		status.Sync.Refresh()
		status.RemoteAccess.Refresh()
		status.Gnomon.Refresh()
		status.EPOCH.Refresh()
	})

	// Background update of history and messages to keep cache warm
	refreshMessageHistoryAsync(false)
	refreshHistoryAsync(false)
}

// Go routine to update the latest information from the connected daemon (Online Mode only)

func StartPulse() {
	logger.Printf("[DEBUG] StartPulse() called\n")
	if !startPulseForActiveWallet() {
		logger.Printf("[DEBUG] StartPulse() - startPulseForActiveWallet returned false\n")
		return
	}

	generation := currentWalletGeneration()
	defer finishPulseForGeneration(generation)

	logger.Printf("[DEBUG] StartPulse() - checking connection, walletapi.Connected=%v, engram.Disk=%v\n", walletapi.Connected, engram.Disk != nil)

	if !walletapi.Connected && engram.Disk != nil {
		maxRetries := 3
		var err error
		var connected bool
		for attempt := 1; attempt <= maxRetries; attempt++ {
			logger.Printf("[Network] Attempting network connection to: %s (attempt %d/%d)\n", walletapi.Daemon_Endpoint, attempt, maxRetries)
			err = walletapi.Connect(session.Daemon)
			if err == nil {
				// Verify connection actually works by checking daemon height
				time.Sleep(500 * time.Millisecond)
				height := walletapi.Get_Daemon_Height()
				if height > 0 {
					logger.Printf("[Network] Connection verified - daemon height: %d\n", height)
					connected = true
					break
				}
				// Connection returned nil but daemon height is 0 = not really connected
				logger.Printf("[Network] Connection succeeded but daemon unreachable (height=0), retrying...\n")
				walletapi.Connected = false
				err = fmt.Errorf("daemon height is 0")
			}
			logger.Printf("[Network] Connection attempt %d failed: %v\n", attempt, err)
			if attempt < maxRetries {
				logger.Printf("[Network] Retrying in 2 seconds...\n")
				time.Sleep(2 * time.Second)
			}
		}
		if !connected {
			logger.Errorf("[Network] Failed to connect after %d attempts: %s\n", maxRetries, walletapi.Daemon_Endpoint)
			walletapi.Connected = false
			return
		}

		// Connection successful - set state immediately before goroutine
		walletapi.Connected = true
		engram.Disk.SetOnlineMode()

		// Update UI to show connected status immediately
		uiDo(func() {
			status.Connection.FillColor = colors.Green
			status.Connection.Refresh()
			status.Sync.FillColor = colors.Yellow
			status.Sync.Refresh()
		})

		logger.Printf("[Network] Connection established successfully, starting pulse loop\n")

		// Start Gnomon indexing as soon as daemon is connected
		logger.Printf("[DEBUG] Calling startGnomon() from StartPulse()\n")
		go startGnomon()

		refreshPermissionsAfterConnect()

		sentNotifications := false
		go func() {
			count := 0
			for isWalletGenerationActive(generation) {
				if !session.WalletOpen {
					break
				}

				if walletapi.Get_Daemon_Height() < 1 || !walletapi.Connected {
					var shouldContinue bool
					count, shouldContinue = pulseReconnect(count)
					if shouldContinue {
						continue
					}
					if !walletapi.Connected {
						break
					}
				}

				if !engram.Disk.IsRegistered() {
					if !walletapi.Connected {
						logger.Errorf("[Network] Could not connect to daemon...%d\n", engram.Disk.Get_Daemon_TopoHeight())
						uiDo(func() {
							status.Connection.FillColor = colors.Red
							status.Connection.Refresh()
							status.Sync.FillColor = colors.Red
						})
					}

					time.Sleep(time.Second)
				} else {
					if !isWalletGenerationActive(generation) {
						break
					}

					refreshPulseWalletState(&sentNotifications)

					time.Sleep(time.Second)
				}
			}

			if walletapi.Connected {
				walletapi.Connected = false
			}
		}()
	}
}

// Get Network setting from the local Graviton tree (Ex: Mainnet, Testnet, Simulator)
func getNetwork() (network string) {
	result, err := GetValue("settings", []byte("network"))
	if err != nil {
		network = NETWORK_MAINNET
		session.Network = network
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
		if setErr := setNetwork(network); setErr != nil {
			logger.Errorf("[Settings] Could not store default network: %s\n", setErr)
		}
		return
	} else {
		if string(result) == NETWORK_TESTNET {
			network = NETWORK_TESTNET
			session.Network = network
			globals.Arguments["--testnet"] = true
			globals.Arguments["--simulator"] = false
			return
		} else if string(result) == NETWORK_SIMULATOR {
			network = NETWORK_SIMULATOR
			session.Network = network
			globals.Arguments["--testnet"] = false
			globals.Arguments["--simulator"] = true
			return
		} else {
			network = NETWORK_MAINNET
			session.Network = network
			globals.Arguments["--testnet"] = false
			globals.Arguments["--simulator"] = false
			return
		}
	}
}

// Set Network setting to the local Graviton tree (Ex: Mainnet, Testnet, Simulator)
func setNetwork(network string) (err error) {
	s := ""
	if network == NETWORK_MAINNET {
		s = network
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
	} else if network == NETWORK_SIMULATOR {
		s = network
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = true
	} else {
		s = NETWORK_TESTNET
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = false
	}

	session.Network = s

	if err = StoreValue("settings", []byte("network"), []byte(s)); err != nil {
		logger.Errorf("[Settings] Could not store network setting: %s\n", err)
		return err
	}

	return
}

// Get daemon endpoint setting from the local Graviton tree
func getDaemon() (r string) {
	result, err := GetValue("settings", []byte("endpoint"))
	if err != nil {
		if checkLocalNode() {
			r = "127.0.0.1:10102"
		} else {
			r = DEFAULT_REMOTE_DAEMON
		}
		if err := setDaemon(r); err != nil {
			logger.Errorf("[Settings] Could not store default daemon: %s\n", err)
		}
		session.Daemon = r
		globals.Arguments["--daemon-address"] = r
		return
	}

	r = string(result)
	session.Daemon = r
	globals.Arguments["--daemon-address"] = r
	return
}

func checkLocalNode() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:10102", 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func testNodeConnection(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func testNodeConnectionTimeout(address string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// Set the daemon endpoint setting to local Graviton tree
func setDaemon(s string) (err error) {
	if err = StoreValue("settings", []byte("endpoint"), []byte(s)); err != nil {
		logger.Errorf("[Settings] Could not store daemon endpoint: %s\n", err)
		return err
	}
	globals.Arguments["--daemon-address"] = s
	session.Daemon = s
	return
}

// Get Remote Access endpoint setting from the local Graviton tree
func getRemoteAccess(key string) (r string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		return
	}

	stored, err := GetEncryptedValue("RemoteAccess", []byte(key))
	if err != nil {
		logger.Debugf("[Engram] getRemoteAccess %s: %s\n", key, err)
		return
	}

	return string(stored)
}

// Get Remote Access endpoint setting with dual storage (try encrypted first, then fallback)
func getRemoteAccessDual(key string) (r string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		return
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("RemoteAccess", []byte(key))
		if err == nil && stored != nil {
			logger.Printf("[Engram] getRemoteAccessDual: Successfully loaded %s from encrypted storage", key)
			return string(stored)
		} else if err != nil {
			logger.Debugf("[Engram] getRemoteAccessDual encrypted storage failed: %s\n", err)
		}
	}

	// Fallback to unencrypted storage
	stored, err := GetValue("RemoteAccessUnencrypted", []byte(key))
	if err == nil && stored != nil {
		logger.Printf("[Engram] getRemoteAccessDual: Successfully loaded %s from fallback storage", key)
		return string(stored)
	} else if err != nil {
		logger.Debugf("[Engram] getRemoteAccessDual fallback storage failed: %s\n", err)
	}

	logger.Printf("[Engram] getRemoteAccessDual: No stored value found for %s", key)
	return ""
}

// Set Remote Access endpoint setting to the local Graviton tree
func setRemoteAccess(port, key string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		logger.Debugf("[Engram] setRemoteAccess: invalid key\n")
		return
	}

	err := StoreEncryptedValue("RemoteAccess", []byte(key), []byte(port))
	if err != nil {
		logger.Debugf("[Engram] setRemoteAccess %s: %s\n", key, err)
	}
}

// Set Remote Access endpoint setting with dual storage (encrypted + unencrypted fallback)
func setRemoteAccessDual(port, key string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		logger.Debugf("[Engram] setRemoteAccessDual: invalid key\n")
		return
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err := StoreEncryptedValue("RemoteAccess", []byte(key), []byte(port))
		if err != nil {
			logger.Debugf("[Engram] setRemoteAccessDual encrypted storage failed: %s\n", err)
		} else {
			logger.Printf("[Engram] setRemoteAccessDual: Successfully saved %s to encrypted storage", key)
		}
	}

	// Always save to unencrypted storage as fallback
	err := StoreValue("RemoteAccessUnencrypted", []byte(key), []byte(port))
	if err != nil {
		logger.Debugf("[Engram] setRemoteAccessDual unencrypted storage failed: %s\n", err)
	} else {
		logger.Printf("[Engram] setRemoteAccessDual: Successfully saved %s to fallback storage", key)
	}
}

// Get mode (online, offline) setting from local Graviton tree
func getMode() {

	/*
		if globals.Arguments["--offline"].(bool) == true {
			session.Mode = "Offline"
			return
		}

		s := "mode"
		t := "settings"
		key := []byte(s)
		result, err := GetValue(t, key)
		if err != nil {
			session.Mode = "Online"
			err := setMode("Online")
			globals.Arguments["--offline"] = false
			if err != nil {
				fmt.Printf("[Engram] Error: %s\n", err)
				return
			}
		} else {
			if result == nil {
				session.Mode = "Online"
				err := setMode("Online")
				globals.Arguments["--offline"] = false
				if err != nil {
					fmt.Printf("[Engram] Error: %s\n", err)
					return
				}
			} else {
				if string(result) == "Offline" {
					globals.Arguments["--offline"] = true
					session.Mode = "Offline"
				} else {
					globals.Arguments["--offline"] = false
					session.Mode = "Online"
				}
			}
		}
	*/
}

// Set the default Offline Mode settings to the local Graviton tree
/*
func setMode(s string) (err error) {
	err = StoreValue("settings", []byte("mode"), []byte(s))
	if s == "Offline" {
		globals.Arguments["--offline"] = true
	} else {
		globals.Arguments["--offline"] = false
	}
	return
}
*/

// Get the default Gnomon settings from local Graviton tree
func getGnomon() (r string, err error) {
	v, err := GetValue("settings", []byte("gnomon"))
	if err != nil {
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
		if storeErr := StoreValue("settings", []byte("gnomon"), []byte("1")); storeErr != nil {
			logger.Errorf("[Settings] Could not store default gnomon setting: %s\n", storeErr)
		}
		return "1", nil
	}

	if string(v) == "1" {
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
	} else {
		gnomon.Active = 0
	}

	r = string(v)
	return
}

// Set the default Gnomon settings to the local Graviton tree
func setGnomon(s string) (err error) {
	if s == "1" {
		err = StoreValue("settings", []byte("gnomon"), []byte("1"))
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
	} else {
		err = StoreValue("settings", []byte("gnomon"), []byte("0"))
		gnomon.Active = 0
	}
	return
}

// Get the RPC credentials from local Graviton tree
func getRPCCredentials() {
	if user, err := GetValue("settings", []byte("rpc_user")); err == nil && len(user) > 0 {
		remoteAccess.RPC.user = string(user)
		// Update UI field when settings are loaded at startup
		if remoteAccess.RPC.userText != nil {
			uiDo(func() {
				if remoteAccess.RPC.userText != nil {
					remoteAccess.RPC.userText.SetText(string(user))
				}
			})
		}
	}
	if pass, err := GetValue("settings", []byte("rpc_pass")); err == nil && len(pass) > 0 {
		remoteAccess.RPC.pass = string(pass)
		// Update UI field when settings are loaded at startup
		if remoteAccess.RPC.passText != nil {
			uiDo(func() {
				if remoteAccess.RPC.passText != nil {
					remoteAccess.RPC.passText.SetText(string(pass))
				}
			})
		}
	}
}

var xswdStateMu sync.RWMutex

/*
func getAuthMode() (result string, err error) {
	r, err := GetValue("settings", []byte("auth_mode"))
	if err != nil {
		StoreValue("settings", []byte("auth_mode"), []byte("true"))
		remoteAccess.mode = 1
		result = "true"
	} else {
		result = string(r)
		if string(result) == "true" {
			remoteAccess.mode = 1
			result = "true"
		} else {
			remoteAccess.mode = 0
			result = "false"
		}
	}
	return
}
*/

// Get the auth_mode settings from local Graviton tree
func setAuthMode(s string) {
	if s == "true" {
		if err := StoreValue("settings", []byte("auth_mode"), []byte("true")); err != nil {
			logger.Errorf("[Settings] Could not store auth mode: %s\n", err)
		}
	} else {
		if err := StoreValue("settings", []byte("auth_mode"), []byte("false")); err != nil {
			logger.Errorf("[Settings] Could not store auth mode: %s\n", err)
		}
	}
}

// Check if a URL exists in the string
func getTextURL(s string) (result []string) {
	return xurls.Relaxed().FindAllString(s, -1)
}

// Set the window size from provided height and width
func resizeWindow(width float32, height float32) {
	s := fyne.NewSize(width, height)
	uiDo(func() {
		if session.Window != nil {
			session.Window.Resize(s)
		}
	})
}

func safeCanvasFocus(obj fyne.Focusable) {
	if appExiting || session.Window == nil || obj == nil {
		return
	}

	fyne.Do(func() {
		if appExiting || session.Window == nil {
			return
		}

		canvasObj, ok := obj.(fyne.CanvasObject)
		if !ok || !canvasObj.Visible() {
			return
		}

		canvas := fyne.CurrentApp().Driver().CanvasForObject(canvasObj)
		if canvas == nil || canvas != session.Window.Canvas() {
			return
		}

		canvas.Focus(obj)
	})
}

// Close the active wallet
func closeWallet() {
	logger.Printf("[Engram] closeWallet() called from domain: %s\n", session.Domain)
	if !beginWalletShutdown() {
		return
	}

	showLoadingOverlay()
	defer finishWalletShutdown()

	if engram.Disk != nil {
		logger.Printf("[Engram] Initiating asynchronous wallet shutdown...\n")

		// Capture resources for background cleanup
		disk := engram.Disk
		rpcServer := remoteAccess.RPC.server
		wsServer := remoteAccess.WS.server
		wsClient := rpc_client.WS
		rpcClient := rpc_client.RPC

		// Immediate state reset
		engram.Disk = nil
		session.WalletOpen = false
		introShownThisSession = false
		session.Domain = "app.main"
		session.BalanceUSD = ""
		session.LastBalance = 0
		tx = Transfers{}

		// Clear Villager state
		session.VillagerHidden = false
		session.VillagerBackground = true
		session.VillagerPopupShown = false
		session.MessageWarningShown = false
		session.VillagerAddress = ""
		session.VillagerPixels = ""

		res.villagerMu.Lock()
		res.villager = nil
		res.villagerMu.Unlock()

		remoteAccess.RPC.server = nil
		remoteAccess.WS.server = nil
		rpc_client.WS = nil
		rpc_client.RPC = nil

		go func() {
			// CRITICAL FIX: Stop Gnomon FIRST to release database lock
			if gnomon.Index != nil {
				logger.Printf("[Gnomon] Shutting down indexers (background)...\n")
				stopGnomon()
				// Reduced delay - startGnomon has retry logic if lock is still held
				time.Sleep(1 * time.Second)
			}

			stopEPOCH()

			// Stop network services to release ports as early as possible
			if rpcServer != nil {
				rpcServer.RPCServer_Stop()
				logger.Printf("[Engram] Remote Access RPC closed (background).\n")
			}

			if wsServer != nil {
				wsServer.Stop()
				logger.Printf("[Engram] Remote Access XSWD closed (background).\n")
			}

			if wsClient != nil {
				wsClient.Close()
				logger.Printf("[Engram] Websocket client closed (background).\n")
			}

			if rpcClient != nil {
				rpcClient.Close()
				logger.Printf("[Engram] RPC client closed (background).\n")
			}

			// Save and close wallet disk
			if disk != nil {
				disk.SetOfflineMode()
				walletapi.Connected = false
				globals.Exit_In_Progress = true
				if shouldSkipWalletSave() {
					logger.Printf("[Engram] Skipping wallet save on close (background)")
				} else if err := disk.Save_Wallet(); err != nil {
					logger.Errorf("[Engram] Failed to save wallet on close (background): %s\n", err)
				}
				disk.Close_Encrypted_Wallet()
			}

			tela.ShutdownTELA()
			resetMessageCache()
			logger.Printf("[Engram] Background wallet shutdown completed.\n")
		}()

		session.Path = ""
		session.Name = ""

		uiDo(func() {
			if session.Window != nil {
				session.LastDomain = layoutMain()
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMain())
				removeOverlays()
			}
		})
		if gnomon.Active == 0 {
			gnomon.Active = 1
		}
		logger.Printf("[Engram] Wallet sign-out initiated successfully.\n")
		return
	}
}

// Create a new account and wallet file
func create() (address string, seed string, err error) {
	check := findAccount()

	if session.Path == "" {
		session.Error = "Please enter an account name."
	} else if session.Language == -1 {
		session.Error = "Please select a language."
	} else if session.Password == "" {
		session.Error = "Please enter a password."
	} else if session.PasswordConfirm == "" {
		session.Error = "Please confirm your password."
	} else if session.PasswordConfirm != session.Password {
		session.Error = "Passwords do not match."
	} else if check {
		session.Error = "Account name already exists."
	} else {
		engram.Disk, err = walletapi.Create_Encrypted_Wallet_Random(session.Path, session.Password)

		if err != nil {
			session.Language = -1
			session.Name = ""
			session.Path = ""
			session.Password = ""
			session.PasswordConfirm = ""
			session.Error = "Account could not be created."
		} else {
			switch session.Network {
			case NETWORK_TESTNET:
				engram.Disk.SetNetwork(false)
				globals.Arguments["--testnet"] = true
				globals.Arguments["--simulator"] = false
			case NETWORK_SIMULATOR:
				engram.Disk.SetNetwork(true)
				globals.Arguments["--testnet"] = false
				globals.Arguments["--simulator"] = true
			default:
				engram.Disk.SetNetwork(true)
				globals.Arguments["--testnet"] = false
				globals.Arguments["--simulator"] = false
			}

			languages := mnemonics.Language_List()

			if session.Language < 0 || session.Language > len(languages)-1 {
				session.Language = 0 // English
			}

			engram.Disk.SetSeedLanguage(languages[session.Language])
			address = engram.Disk.GetAddress().String()
			seed = engram.Disk.GetSeed()
			engram.Disk.Close_Encrypted_Wallet()
			engram.Disk = nil
			session.Error = "Account successfully created."
			session.Language = -1
			session.Name = ""
			session.PasswordConfirm = ""
			session.Domain = "app.main"
		}
	}
	return
}

// The main login routine - optimized for fast dashboard display
func login() {
	showLoadingOverlay()

	if engram.Disk == nil {
		temp, err := walletapi.Open_Encrypted_Wallet(session.Path, session.Password)
		if err != nil {
			session.Domain = "app.main"
			session.Error = err.Error()
			if len(session.Error) > 40 {
				session.Error = fmt.Sprintf("%s...", session.Error[0:40])
			}
			removeOverlays()
			return
		}

		engram.Disk = temp
		session.Password = ""
		loadPersistedMessageCache()
		beginWalletSession()

		// Reset exit flag so Gnomon can start in this session
		globals.Exit_In_Progress = false

		// CRITICAL: Set WalletOpen before initSettings so toggleXSWD auto-start succeeds
		session.WalletOpen = true

		logger.Printf("[Engram] Wallet opened - loading encrypted settings...")
		initSettings()
	}

	switch session.Network {
	case NETWORK_TESTNET:
		engram.Disk.SetNetwork(false)
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = false
	case NETWORK_SIMULATOR:
		engram.Disk.SetNetwork(true)
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = true
	default:
		engram.Disk.SetNetwork(true)
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
	}
	session.BalanceUSD = ""
	session.LastBalance = 0

	logger.Printf("[DEBUG] login() - session.Offline=%v, session.Daemon='%s', walletapi.Connected=%v, engram.Disk=%v, session.WalletOpen=%v\n",
		session.Offline, session.Daemon, walletapi.Connected, engram.Disk != nil, session.WalletOpen)

	if !session.Offline {
		walletapi.Connected = false
		walletapi.SetDaemonAddress(session.Daemon)
		engram.Disk.SetDaemonAddress(session.Daemon)

		if session.TrackRecentBlocks > 0 {
			logger.Printf("[Engram] Scan tracking enabled, only scanning the last %d blocks...\n", session.TrackRecentBlocks)
			engram.Disk.SetTrackRecentBlocks(session.TrackRecentBlocks)
		}

		if s, err := strconv.Atoi(getRemoteAccess("EPOCH")); err == nil {
			if err := epoch.SetPort(s); err != nil {
				logger.Errorf("[Engram] Setting EPOCH port: %s\n", err)
			}
		}

		remoteAccess.EPOCH.total.Hashes = 0
		remoteAccess.EPOCH.total.MiniBlocks = 0
		if epochData, err := GetEncryptedValue("RemoteAccess", []byte("EPOCH")); err == nil {
			if err := json.Unmarshal(epochData, &remoteAccess.EPOCH.total); err != nil {
				logger.Errorf("[Engram] Setting EPOCH total: %s\n", err)
			}
		}

		go StartPulse()
	} else {
		engram.Disk.SetOfflineMode()
		status.Connection.FillColor = colors.Gray
		status.Sync.FillColor = colors.Gray
	}

	setRingSize(engram.Disk, 16)
	session.Verified = false

	isRegistered := false
	if engram.Disk != nil {
		address := engram.Disk.GetAddress().String()
		if regVal, err := GetEncryptedValue("settings", []byte("Registration:"+address)); err == nil && string(regVal) == "true" {
			isRegistered = true
		} else if engram.Disk.Get_Registration_TopoHeight() >= 1 {
			isRegistered = true
			go StoreEncryptedValue("settings", []byte("Registration:"+address), []byte("true"))
		}
	}

	if session.Offline || isRegistered {
		if a.Driver().Device().IsMobile() {
			session.Domain = "app.wallet"
			resizeWindow(ui.MaxWidth, ui.MaxHeight)
		}
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
		if session.Offline {
			fyne.Do(func() {
				updateDashboardAfterLogin()
			})
		}
	}

	if !session.Offline {
		status.Connection.FillColor = colors.Yellow
		status.Connection.Refresh()
		status.Sync.FillColor = colors.Yellow
		status.Sync.Refresh()
		session.Balance = 0

		if !isRegistered {
			// CRITICAL: Remove the previous overlay (created by showLoadingOverlay() at the start of login)
			// This resets res.loading to nil, forcing showLoadingOverlayWithText to create a fresh AnimatedGif instance.
			// Without this, the same res.loading pointer exists in two scene graphs simultaneously,
			// causing Fyne to misalign the hexagon animation relative to the text on mobile devices.
			removeOverlays()
			if session.IsRecovery {
				showLoadingOverlayWithText(i18n.T("wallet.init_recovered"), i18n.T("wallet.sync_status"))
			} else if session.IsNewWallet {
				showLoadingOverlayWithText(i18n.T("wallet.init_new"), i18n.T("wallet.sync_status"))
			} else {
				showLoadingOverlayWithText(i18n.T("wallet.syncing"), i18n.T("wallet.registration_status"))
			}
		}

		go func() {
			logger.Printf("[DEBUG] login goroutine starting\n")
			generation := currentWalletGeneration()

			// Initialize WebSocket and Gnomon state in background
			initWebSocketState()
			_, _ = getGnomon()

			// Wait for StartPulse to actually establish connection
			logger.Printf("[DEBUG] waiting for walletapi.Connected to become true...\n")
			connected := waitForConnectionWithTimeout(10 * time.Second)
			logger.Printf("[DEBUG] waitForConnection returned: %v, walletapi.Connected=%v\n", connected, walletapi.Connected)

			if !connected || !isWalletGenerationActive(generation) {
				logger.Printf("[DEBUG] connection timeout or wallet closed\n")
				uiDo(func() {
					if !isWalletGenerationActive(generation) {
						return
					}
					removeOverlays()
					status.Connection.FillColor = colors.Red
					status.Connection.Refresh()
					status.Sync.FillColor = colors.Red
					status.Sync.Refresh()
					session.Domain = "app.main"
					session.Error = "Could not connect to daemon."
					session.Window.SetContent(layoutMain())
				})
				return
			}

			fyne.Do(func() {
				status.Connection.FillColor = colors.Green
				status.Connection.Refresh()
			})

			waitForWalletSync(3 * time.Second)
			logger.Printf("[DEBUG] wallet sync complete\n")
			if !isWalletGenerationActive(generation) {
				return
			}

			logger.Printf("[DEBUG] checking wallet registration...\n")
			needsReg, regDone := checkRegistrationWithTimeout(10 * time.Second)
			logger.Printf("[DEBUG] registration check complete: needsReg=%v, regDone=%v\n", needsReg, regDone)
			if !isWalletGenerationActive(generation) {
				return
			}

			if needsReg {
				fyne.Do(func() {
					if !isWalletGenerationActive(generation) {
						return
					}
					removeOverlays()
					registerAccount()
					session.Verified = true
				})
				logger.Printf("[Registration] Account registration PoW started...\n")
				logger.Printf("[Registration] Registering your account. This can take up to 120 minutes (one time). Please wait...\n")
				return
			}

			if !needsReg && regDone {
				address := engram.Disk.GetAddress().String()
				go StoreEncryptedValue("settings", []byte("Registration:"+address), []byte("true"))
			}

			if regDone && isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
				logger.Printf("[DEBUG] calling startGnomon()\n")
				go startGnomon()
			}

			if !isWalletGenerationActive(generation) {
				return
			}

			refreshMessageHistoryAsync(false)

			fyne.Do(func() {
				if !isRegistered {
					if a.Driver().Device().IsMobile() {
						session.Domain = "app.wallet"
						resizeWindow(ui.MaxWidth, ui.MaxHeight)
					}
					session.Window.SetContent(layoutDashboard())
					removeOverlays()
				}
				session.IsRecovery = false
				session.IsNewWallet = false
				updateDashboardAfterLogin()
			})
		}()
	}

	address := engram.Disk.GetAddress().String()
	shard := fmt.Sprintf("%x", sha1.Sum([]byte(address)))
	session.ID = shard
	session.LimitMessages = true
}

// waitForConnectionWithTimeout waits for daemon connection with a timeout
func waitForConnectionWithTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if walletapi.Connected {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return walletapi.Connected
}

// waitForWalletSync waits briefly for wallet height to catch up
func waitForWalletSync(timeout time.Duration) {
	if engram.Disk == nil {
		return
	}

	deadline := time.Now().Add(timeout)
	daemonHeight := engram.Disk.Get_Daemon_Height()
	for time.Now().Before(deadline) {
		if engram.Disk == nil {
			return
		}

		if engram.Disk.Get_Height() >= daemonHeight {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// checkRegistrationWithTimeout checks if account registration is needed
func checkRegistrationWithTimeout(timeout time.Duration) (needsRegistration bool, canProceed bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if engram.Disk == nil || !session.WalletOpen {
			return false, false
		}

		height := engram.Disk.Get_Registration_TopoHeight()
		if height >= 1 {
			return false, true
		}
		time.Sleep(500 * time.Millisecond)
	}

	if engram.Disk == nil || !session.WalletOpen {
		return false, false
	}

	height := engram.Disk.Get_Registration_TopoHeight()
	if height < 1 {
		return true, false
	}
	return false, true
}

// updateDashboardAfterLogin updates dashboard UI elements after background operations complete
func updateDashboardAfterLogin() {
	session.Balance, _ = engram.Disk.Get_Balance()
	fyne.Do(func() {
		if session.BalanceText != nil {
			if session.BalanceHidden {
				session.BalanceText.Text = "••••••"
			} else {
				session.BalanceText.Text = globals.FormatMoney(session.Balance)
			}
			session.BalanceText.Refresh()
		}
	})

	session.WalletHeight = engram.Disk.Wallet_Memory.Get_Height()
	session.DaemonHeight = engram.Disk.Get_Daemon_Height()
	fyne.Do(func() {
		if session.StatusText != nil {
			session.StatusText.Text = fmt.Sprintf("%d", session.WalletHeight)
			session.StatusText.Refresh()
		}

		if session.WalletHeight == session.DaemonHeight && !session.Offline {
			status.Sync.FillColor = colors.Green
			status.Sync.Refresh()
		}
	})

	go updateVillagerAvatar()
}

// Remove all overlays
func removeOverlays() {
	uiDo(func() {
		if session.Window == nil {
			return
		}

		overlays := session.Window.Canvas().Overlays()
		list := overlays.List()

		for o := range list {
			overlays.Remove(list[o])
		}

		if res.loading != nil {
			res.loading.Hide()
			res.loading.Stop()
			res.loading = nil
		}
	})
}

// Add an overlay with the loading animation
func showLoadingOverlay() {
	uiDo(func() {
		if session.Window == nil {
			return
		}

		frame := &iframe{}

		if res.loading == nil {
			res.loading, _ = x.NewAnimatedGifFromResource(resourceLoadingGif)
			res.loading.SetMinSize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))
		}

		rect := canvas.NewRectangle(colors.DarkMatter)
		rect.SetMinSize(frame.Size())

		background := container.NewStack(
			rect,
			container.NewCenter(
				res.loading,
			),
		)

		res.loading.Start()

		layout := container.NewStack(
			frame,
			background,
		)

		overlays := session.Window.Canvas().Overlays()
		overlays.Add(layout)
	})
}

// Add an overlay with the loading animation, description, and ETA
func showLoadingOverlayWithText(title, eta string) {
	uiDo(func() {
		if session.Window == nil {
			return
		}

		frame := &iframe{}

		// Force destroy any old instances to completely prevent Fyne double-parenting layout glitches
		if res.loading != nil {
			res.loading.Stop()
			res.loading = nil
		}

		res.loading, _ = x.NewAnimatedGifFromResource(resourceLoadingGif)
		gifSize := fyne.NewSize(ui.Width*0.45, ui.Width*0.45)
		if gifSize.Width > 150 {
			gifSize = fyne.NewSize(150, 150) // Cap size for tablets/landscape so it looks great
		}
		res.loading.SetMinSize(gifSize)
		res.loading.Resize(gifSize)

		rect := canvas.NewRectangle(colors.DarkMatter)
		rect.SetMinSize(frame.Size())

		lblTitle := canvas.NewText(title, colors.Green)
		lblTitle.TextSize = scaleFont(16)
		lblTitle.Alignment = fyne.TextAlignCenter
		lblTitle.TextStyle = fyne.TextStyle{Bold: true}

		lblETA := canvas.NewText(eta, colors.Gray)
		lblETA.TextSize = scaleFont(12)
		lblETA.Alignment = fyne.TextAlignCenter

		rectPassSpacer := canvas.NewRectangle(color.Transparent)
		rectPassSpacer.SetMinSize(fyne.NewSize(10, 5))

		background := container.NewStack(
			rect,
			container.NewCenter(
				container.NewVBox(
					container.NewStack(res.loading),
					widget.NewLabel(""),
					lblTitle,
					rectPassSpacer,
					lblETA,
				),
			),
		)

		res.loading.Start()

		layout := container.NewStack(
			frame,
			background,
		)

		overlays := session.Window.Canvas().Overlays()
		overlays.Add(layout)
	})
}

// Load embedded resources
func loadResources() {
	res.bg = canvas.NewImageFromResource(resourceBgPng)
	res.bg.FillMode = canvas.ImageFillContain

	res.bg2 = canvas.NewImageFromResource(resourceBg2Png)
	res.bg2.FillMode = canvas.ImageFillContain

	res.icon = canvas.NewImageFromResource(resourceIconPng)
	res.icon.FillMode = canvas.ImageFillContain

	res.header = canvas.NewImageFromResource(resourceBackground1Png)
	res.header.FillMode = canvas.ImageFillContain

	res.load = canvas.NewImageFromResource(resourceLoadPng)
	res.load.FillMode = canvas.ImageFillStretch

	res.dero = canvas.NewImageFromResource(resourceDeroPng)
	res.dero.FillMode = canvas.ImageFillContain

	res.gram = canvas.NewImageFromResource(resourceDEROLogoPng)
	res.gram.FillMode = canvas.ImageFillContain

	res.red_alert = canvas.NewImageFromResource(resourceRedAlertPng)
	res.red_alert.FillMode = canvas.ImageFillContain

	res.green_alert = canvas.NewImageFromResource(resourceGreenAlertPng)
	res.green_alert.FillMode = canvas.ImageFillContain

	res.mainBg = canvas.NewImageFromResource(resourceEngramMainPng)
	res.mainBg.FillMode = canvas.ImageFillContain

	res.telaBg = canvas.NewImageFromResource(resourceTelaPng)
	res.telaBg.FillMode = canvas.ImageFillContain

}

// Validate if the provided word is a seed word
func checkSeedWord(w string) (check bool) {
	split := strings.Split(w, " ")

	if len(split) > 1 {
		return
	}
	_, _, _, check = mnemonics.Find_indices([]string{w})

	return
}

// Add a DERO transfer to the batch
func addTransfer() error {
	var arguments = rpc.Arguments{}
	var err error

	logger.Printf("[Send] Starting tx...\n")
	if tx.Address.IsIntegratedAddress() {
		if tx.Address.Arguments.Validate_Arguments() != nil {
			logger.Errorf("[Service] Integrated Address arguments could not be validated\n")
			err = errors.New("integrated address arguments could not be validated")
			return err
		}

		logger.Printf("[Send] Not Integrated..\n")
		if !tx.Address.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
			logger.Errorf("[Service] Integrated Address does not contain destination port\n")
			err = errors.New("integrated address does not contain destination port")
			return err
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.Address.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})
		logger.Printf("[Send] Added arguments..\n")

		if tx.Address.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) {

			if tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				logger.Errorf("[Service] This address has expired: %s\n", tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				err = errors.New("this address has expired")
				return err
			} else {
				logger.Warnf("[Service] This address will expire: %s\n", tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
			}
		}

		logger.Printf("[Service] Destination port is integrated in address: %d\n", tx.Address.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if tx.Address.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) {
			logger.Printf("[Service] Integrated Message: %s\n", tx.Address.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Address.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	logger.Printf("[Send] Checking arguments..\n")

	for _, arg := range tx.Address.Arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	logger.Printf("[Send] Checking Amount..\n")

	if tx.Address.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
		logger.Printf("[Service] Transaction amount: %s\n", globals.FormatMoney(tx.Address.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)))
		tx.Amount = tx.Address.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		balance, _ := engram.Disk.Get_Balance()
		logger.Printf("[Send] Balance: %d\n", balance)
		logger.Printf("[Send] Amount: %d\n", tx.Amount)

		if tx.Amount > balance {
			logger.Errorf("[Send] Error: Insufficient funds\n")
			err = errors.New("insufficient funds")
			return err
		} else if tx.Amount == balance {
			tx.SendAll = true
		} else {
			tx.SendAll = false
		}
	}

	logger.Printf("[Send] Checking services..\n")

	if tx.Address.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataUint64) {
		logger.Printf("[Service] Reply Address required, sending: %s\n", engram.Disk.GetAddress().String())
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_REPLYBACK_ADDRESS, DataType: rpc.DataAddress, Value: engram.Disk.GetAddress()})
	}

	logger.Printf("[Send] Checking payment ID/destination port..\n")

	if len(arguments) == 0 {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.PaymentID})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Comment})
	}

	logger.Printf("[Send] Checking Pack..\n")

	if _, err := arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Send] Arguments packing err: %s\n", err)
		return err
	}

	if tx.Ringsize == 0 {
		tx.Ringsize = 2
	} else if tx.Ringsize > 128 {
		tx.Ringsize = 128
	} else if !crypto.IsPowerOf2(int(tx.Ringsize)) {
		tx.Ringsize = 2
		logger.Errorf("[Send] Error: Invalid ringsize - New ringsize = %d\n", tx.Ringsize)
		err = errors.New("invalid ringsize")
		return err
	}

	tx.Status = "Unsent"

	logger.Printf("[Send] Ringsize: %d\n", tx.Ringsize)

	tx.Pending = append(tx.Pending, rpc.Transfer{Amount: tx.Amount, Destination: tx.Address.String(), Payload_RPC: arguments})
	logger.Printf("[Send] Added transfer to the pending list.\n")

	return nil
}

// Send all batched transfers (TODO: export offline transactions to file in Offline mode)
func sendTransfers() (txid crypto.Hash, err error) {
	if session.Offline {
		return
	}

	fees := ((tx.Ringsize + 1) * config.FEE_PER_KB) / 4
	if fees < 85 {
		fees = 85
	}

	logger.Printf("[Send] Calculated Fees: %d\n", fees*uint64(len(tx.Pending)))

	tx.TX, err = engram.Disk.TransferPayload0(tx.Pending, tx.Ringsize, false, rpc.Arguments{}, fees, false)
	if err != nil {
		logger.Errorf("[Send] Error while building transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx.TX); err != nil {
		logger.Errorf("[Send] Error while dispatching transaction: %s\n", err)
		return
	}

	tx.Fees = tx.TX.Fees()
	tx.TXID = tx.TX.GetHash()

	logger.Printf("[Send] Dispatched transaction: %s\n", tx.TXID)

	txid = tx.TX.GetHash()

	tx = Transfers{}

	return
}

// Go Routine for account registration
func registerAccount() {
	session.Domain = "app.register"
	if engram.Disk == nil {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		session.Domain = "app.main"
		return
	}

	link := widget.NewHyperlinkWithStyle(i18n.T("common.cancel"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	link.OnTapped = func() {
		session.Gif.Stop()
		session.Gif = nil
		closeWallet()
	}

	title := canvas.NewText(i18n.T("registration.title"), colors.Green)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 16

	heading := canvas.NewText(i18n.T("registration.wait"), colors.Gray)
	heading.TextSize = 22
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	sub := canvas.NewText(i18n.T("registration.take_time"), colors.Gray)
	sub.TextSize = 14
	sub.Alignment = fyne.TextAlignCenter
	sub.TextStyle = fyne.TextStyle{Bold: true}

	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	session.Window.SetContent(layoutTransition())
	session.Window.SetContent(layoutWaiting(title, heading, sub, link))

	// Registration PoW
	go func() {
		var reg_tx *transaction.Transaction
		successful_regs := make(chan *transaction.Transaction)
		counter := 0
		session.RegHashes = 0

		for i := 0; i < runtime.GOMAXPROCS(0)-1; i++ {
			go func() {
				for counter == 0 {
					if engram.Disk == nil {
						break
					} else if engram.Disk.IsRegistered() {
						break
					}

					lreg_tx := engram.Disk.GetRegistrationTX()
					hash := lreg_tx.GetHash()
					session.RegHashes++

					if hash[0] == 0 && hash[1] == 0 && hash[2] == 0 {
						successful_regs <- lreg_tx
						counter++
						break
					}
				}
			}()
		}

		if engram.Disk == nil {
			session.Gif.Stop()
			session.Gif = nil

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMain())
			})

			session.Domain = "app.main"
			return
		}

		reg_tx = <-successful_regs

		logger.Printf("[Registration] Registration TXID: %s\n", reg_tx.GetHash())
		err := engram.Disk.SendTransaction(reg_tx)
		if err != nil {
			session.Gif.Stop()
			session.Gif = nil
			logger.Errorf("[Registration] Error: %s\n", err)

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMain())
			})

			session.Domain = "app.main"
		} else {
			session.Gif.Stop()
			session.Gif = nil
			logger.Printf("[Registration] Registration transaction dispatched successfully.\n")
			session.Domain = "app.wallet"

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutDashboard())
			})
		}
	}()
}

// Set the ring size for transactions
func setRingSize(wallet *walletapi.Wallet_Disk, s int) bool {
	if wallet == nil {
		logger.Errorf("[Engram] No wallet found.\n")
		return false
	}

	// Minimum ring size is 2, only accept powers of 2.
	if s < 2 {
		wallet.SetRingSize(2)
		logger.Printf("[Engram] Set minimum ring size: 2\n")
	} else {
		wallet.SetRingSize(s)
		logger.Printf("[Engram] Set default ring size: %d\n", s)
	}

	return true
}

// Check if a username exists, return the registered address if so
func checkUsername(s string, h int64) (address string, err error) {
	if session.Offline {
		return
	}

	if h < 0 {
		address, err = engram.Disk.NameToAddress(s)
	} else {
		var params rpc.NameToAddress_Params
		var response *jrpc2.Response
		var result rpc.NameToAddress_Result

		rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
		if err != nil {
			return
		}

		input_output := rwc.New(rpc_client.WS)
		rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

		if rpc_client.RPC != nil {
			params.Name = s
			params.TopoHeight = h

			address = ""
			response, err = rpc_client.RPC.Call(context.Background(), "DERO.NameToAddress", params)

			rpc_client.WS.Close()
			rpc_client.RPC.Close()

			if err != nil {
				return
			}

			err = response.UnmarshalResult(&result)
			if err != nil {
				return
			}

			if result.Status != "OK" {
				err = errors.New("username does not exist")
				return
			}

			address = result.Address
		}
	}

	return
}

// Get the transaction fees to be paid
func getGasEstimate(gp rpc.GasEstimate_Params) (gas uint64, err error) {
	var result rpc.GasEstimate_Result

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	if err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetGasEstimate", gp, &result); err != nil {
		return
	}

	if result.Status != "OK" {
		return
	}

	gas = result.GasStorage

	return
}

// Register a new DERO username
func registerUsername(s string) (storage uint64, err error) {
	// Check first if the name is taken
	valid, _ := checkUsername(s, -1)
	if valid != "" {
		logger.Errorf("[Username] Error: skipping registration - username exists.\n")
		err = errors.New("username already exists")
		return
	}

	scid := crypto.HashHexToHash("0000000000000000000000000000000000000000000000000000000000000001")

	var args = rpc.Arguments{}
	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: "Register"})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})
	args = append(args, rpc.Argument{Name: "name", DataType: "S", Value: s})

	var p rpc.Transfer_Params
	var dest string

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}
	p.Transfers = append(p.Transfers, rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	})

	gp := rpc.GasEstimate_Params{SC_RPC: args, Ringsize: 2, Signer: engram.Disk.GetAddress().String(), Transfers: p.Transfers}

	storage, err = getGasEstimate(gp)
	if err != nil {
		logger.Errorf("[Username] Error estimating fees: %s\n", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0(p.Transfers, 2, false, args, storage, false)
	if err != nil {
		logger.Errorf("[Username] Error while building transaction: %s\n", err)
		return
	}

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[Username] Error while dispatching transaction: %s\n", err)
		return
	}

	logger.Printf("[Username] Username Registration TXID:  %s\n", tx.GetHash().String())

	return
}

// Check to make sure the message transaction meets criteria
func checkMessagePack(m string, s string, r string) (err error) {
	if m == "" {
		return
	}

	mapAddress := ""
	a, err := globals.ParseValidateAddress(r)
	if err != nil {
		//mapAddress, err = engram.Disk.NameToAddress(r)
		mapAddress, err = checkUsername(r, -1)
		if err != nil {
			return
		}
		a, err = globals.ParseValidateAddress(mapAddress)
		if err != nil {
			return
		}
	}

	if s == "" {
		s = engram.Disk.GetAddress().String()
	}

	var amount uint64

	if a.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) { // but only it is present
		//logger.Info("Transaction", "Value", globals.FormatMoney(a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)))
		amount = a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		amount, err = globals.ParseAmount("0.00001")
		if err != nil {
			//logger.Error(err, "Err parsing amount\n")
			return
		}
	}

	var arguments = rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
		{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: amount},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m},
		{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s},
	}

	if a.IsIntegratedAddress() { // read everything from the address
		if a.Arguments.Validate_Arguments() != nil {
			//fmt.Printf(err, "Integrated Address  arguments could not be validated.\n")
			return
		}

		if !a.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) { // but only it is present
			//fmt.Printf(fmt.Errorf("Integrated Address does not contain destination port.\n"), "")
			return
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})

		if a.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) { // but only it is present
			if a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				//fmt.Printf(nil, "This address has expired.", "expiry time", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				return
			}
		}

		logger.Printf("Destination port is integrated in address. %d\n", a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if a.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) { // but only it is present
			logger.Printf("Integrated Message: %s\n", a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	for _, arg := range arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	if a.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s})
	}

	// if no arguments, use space by embedding a small comment
	if len(arguments) == 0 { // allow user to enter Comment
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m})
	}

	if _, err = arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Message] Arguments packing err: %s\n", err)
		return
	}

	return
}

// Send a private message to another account
func sendMessage(m string, s string, r string) (txid crypto.Hash, err error) {
	if m == "" {
		return
	}

	mapAddress := ""
	a, err := globals.ParseValidateAddress(r)
	if err != nil {
		//mapAddress, err = engram.Disk.NameToAddress(r)
		mapAddress, err = checkUsername(r, -1)
		if err != nil {
			return
		}
		a, err = globals.ParseValidateAddress(mapAddress)
		if err != nil {
			return
		}
	}

	if s == "" {
		s = engram.Disk.GetAddress().String()
	}

	amount, err := globals.ParseAmount("0.00001")
	if err != nil {
		//logger.Error(err, "Err parsing amount\n")
		return
	}

	var arguments = rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
		{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: amount},
		{Name: rpc.RPC_EXPIRY, DataType: rpc.DataTime, Value: time.Now().Add(time.Hour).UTC()},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m},
		{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s},
	}

	if a.IsIntegratedAddress() {
		if a.Arguments.Validate_Arguments() != nil {
			return
		}

		if !a.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
			logger.Errorf("[Send Message] Integrated Address does not contain destination port.\n")
			return
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})

		if a.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) {
			if a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				logger.Errorf("[Send Message] This address has expired on %x\n", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				return
			} else {
				logger.Warnf("[Send Message] This address will expire on %x\n", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
			}
		}

		logger.Printf("[Send Message] Destination port is integrated in address. %x\n", a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if a.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) {
			logger.Printf("[Send Message] Integrated Message: %s\n", a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	for _, arg := range arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	if a.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
		amount = a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		amount, err = globals.ParseAmount("0.00001")
		if err != nil {
			logger.Errorf("[Send Message] Failed parsing transfer amount: %s\n", err)
			return
		}
	}

	if a.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s})
	}

	if len(arguments) == 0 {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m})
	}

	if _, err = arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Message] Arguments packing err: %s\n", err)
		return
	}

	fees := ((uint64(engram.Disk.GetRingSize()) + 1) * config.FEE_PER_KB) / 4

	logger.Printf("[Message] Calculated Fees: %d\n", fees)

	transfer := rpc.Transfer{Amount: amount, Destination: a.String(), Payload_RPC: arguments}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 0, false, rpc.Arguments{}, fees, false)
	if err != nil {
		logger.Errorf("[Message] Error while building transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Message] Error while dispatching transaction: %s\n", err)
		return
	}

	txid = tx.GetHash()

	logger.Printf("[Message] Dispatched transaction: %s\n", txid)

	return
}

type MessageRecord struct {
	Entry      rpc.Entry
	ContactKey string
	Label      string
	Comment    string
}

type MessageThreadSummary struct {
	ContactKey string
	Label      string
	LastText   string
	LastTime   time.Time
	LastTXID   string
	Count      int
}

type MessageCache struct {
	Height   uint64
	Records  []MessageRecord
	ByTXID   map[string]MessageRecord
	Address  string
	Primed   bool
	Loaded   bool
	Threads  []MessageThreadSummary
	ByThread map[string][]MessageRecord
}

type RenderedThreadMessage struct {
	Sender     string
	Comment    string
	Timestamp  string
	IsIncoming bool
}

type HistoryRowCache struct {
	Height       uint64
	Address      string
	Transfers    []rpc.Entry
	NormalRows   []string
	CoinbaseRows []string
	MessageRows  []string
	Loaded       bool
	sync.RWMutex
}

var messageCache MessageCache
var addressDisplayCache = map[string]string{}
var historyRowCache HistoryRowCache
var historyRowCacheMu sync.RWMutex
var renderedThreadCacheMu sync.RWMutex
var renderedThreadCache = map[string][]RenderedThreadMessage{}

func uiDo(fn func()) {
	if fn == nil || appExitFlag.Load() {
		return
	}

	start := time.Now()
	fyne.Do(func() {
		if appExitFlag.Load() {
			return
		}
		elapsed := time.Since(start)
		if elapsed > 1*time.Second {
			logger.Debugf("[uiDo] LAGGING: took %v to start\n", elapsed)
		}
		fn()
	})
}

func safeOpenURL(u *url.URL) {
	if u == nil {
		return
	}

	// If it's a TELA-like URL (localhost/127.0.0.1), ensure XSWD is ready
	if strings.HasPrefix(u.Host, "localhost") || strings.HasPrefix(u.Host, "127.0.0.1") {
		EnsureXSWD()
	}

	logger.Printf("[OpenURL] Requesting to open: %s\n", u.String())
	go func() {
		err := fyne.CurrentApp().OpenURL(u)
		if err != nil {
			logger.Errorf("[OpenURL] Error opening %s: %v\n", u.String(), err)
		} else {
			logger.Printf("[OpenURL] Successfully requested open for %s\n", u.String())
		}
	}()
}

func safeWalletOpen() bool {
	return engram.Disk != nil && session.WalletOpen
}

func getCachedThreadMessages(contact string, minHeight uint64) []MessageRecord {
	messageCacheMu.RLock()
	defer messageCacheMu.RUnlock()

	if len(messageCache.ByThread) == 0 {
		return nil
	}

	key, _ := resolveMessageContact(contact, -1)
	if key == "" {
		key = strings.TrimSpace(contact)
	}
	records := messageCache.ByThread[key]
	if len(records) == 0 {
		return nil
	}

	result := make([]MessageRecord, 0, len(records))
	for _, record := range records {
		if minHeight > 0 && record.Entry.Height < minHeight {
			continue
		}
		result = append(result, record)
	}
	return result
}

func getRenderedThreadCache(contact string, minHeight uint64) ([]RenderedThreadMessage, bool) {
	renderedThreadCacheMu.RLock()
	defer renderedThreadCacheMu.RUnlock()
	key, _ := resolveMessageContact(contact, -1)
	if key == "" {
		key = strings.TrimSpace(contact)
	}
	if minHeight > 0 {
		key = fmt.Sprintf("%s:%d", key, minHeight)
	}
	items, ok := renderedThreadCache[key]
	if !ok {
		return nil, false
	}
	copyItems := append([]RenderedThreadMessage(nil), items...)
	return copyItems, true
}

func setRenderedThreadCache(contact string, minHeight uint64, items []RenderedThreadMessage) {
	renderedThreadCacheMu.Lock()
	defer renderedThreadCacheMu.Unlock()
	key, _ := resolveMessageContact(contact, -1)
	if key == "" {
		key = strings.TrimSpace(contact)
	}
	if minHeight > 0 {
		key = fmt.Sprintf("%s:%d", key, minHeight)
	}
	renderedThreadCache[key] = append([]RenderedThreadMessage(nil), items...)
}

func getHistoryRowCache() (transfers []rpc.Entry, normalRows []string, coinbaseRows []string, messageRows []string, height uint64, ok bool) {
	historyRowCache.RLock()
	defer historyRowCache.RUnlock()

	if engram.Disk == nil || !historyRowCache.Loaded {
		return nil, nil, nil, nil, 0, false
	}

	address := engram.Disk.GetAddress().String()
	if historyRowCache.Address != address {
		return nil, nil, nil, nil, 0, false
	}

	transfers = append([]rpc.Entry(nil), historyRowCache.Transfers...)
	normalRows = append([]string(nil), historyRowCache.NormalRows...)
	coinbaseRows = append([]string(nil), historyRowCache.CoinbaseRows...)
	messageRows = append([]string(nil), historyRowCache.MessageRows...)
	return transfers, normalRows, coinbaseRows, messageRows, historyRowCache.Height, true
}

func getTransferTime(txid string) time.Time {
	transfers, _, _, _, _, ok := getHistoryRowCache()
	if !ok {
		return time.Time{}
	}
	for _, t := range transfers {
		if t.TXID == txid {
			return t.Time
		}
	}
	return time.Time{}
}

func setHistoryRowCache(transfers []rpc.Entry, normalRows []string, coinbaseRows []string, messageRows []string) {
	if engram.Disk == nil {
		return
	}

	historyRowCache.Lock()
	defer historyRowCache.Unlock()
	historyRowCache.Address = engram.Disk.GetAddress().String()
	historyRowCache.Height = engram.Disk.Get_Height()
	historyRowCache.Transfers = append([]rpc.Entry(nil), transfers...)
	historyRowCache.NormalRows = append([]string(nil), normalRows...)
	historyRowCache.CoinbaseRows = append([]string(nil), coinbaseRows...)
	historyRowCache.MessageRows = append([]string(nil), messageRows...)
	historyRowCache.Loaded = true
}

// syncHistoryRows incrementally updates the history cache and returns the latest data.
// It is optimized for fast loading on mobile and remote nodes.
var historySortOrder string = "Descending"

func syncHistoryRows() (transfers []rpc.Entry, normalRows []string, coinbaseRows []string, messageRows []string) {
	if engram.Disk == nil {
		return nil, nil, nil, nil
	}

	// 1. Check if we can use the cache as-is or as a starting point
	cachedTransfers, cachedNormal, cachedCoinbase, _, cachedHeight, ok := getHistoryRowCache()

	// Detect if cache needs a rebuild (e.g. after update adding headers or changing sort)
	forceRebuild := false
	if ok && len(cachedTransfers) > 0 && (len(cachedNormal) == 0 || !strings.Contains(cachedNormal[0], "HEADER")) {
		forceRebuild = true
	}

	if ok && !forceRebuild && cachedHeight == engram.Disk.Get_Height() {
		// Even if height matches, we return the cached rows (which include messages)
		_, _, _, messageRows, _, _ := getHistoryRowCache()
		return cachedTransfers, cachedNormal, cachedCoinbase, messageRows
	}

	// 2. Determine start height for delta fetch
	startHeight := uint64(0)
	if ok && !forceRebuild && cachedHeight > 0 && cachedHeight <= engram.Disk.Get_Height() {
		// Massive overlap (5000 blocks ~ 20 hours) to ensure no transactions are ever missed
		// due to indexing delays or network inconsistencies.
		if cachedHeight > 5000 {
			startHeight = cachedHeight - 5000
		} else {
			startHeight = 0
		}
	} else {
		// For the very first load or forced rebuild, fetch EVERYTHING to ensure full history
		startHeight = 0
	}

	// 3. Fetch delta transfers (Normal and Coinbase separately to be sure)
	var zeroscid crypto.Hash
	newEntries := engram.Disk.Show_Transfers(zeroscid, false, true, true, startHeight, 0, "", "", 0, 0)
	newCoinbase := engram.Disk.Show_Transfers(zeroscid, true, true, true, startHeight, 0, "", "", 0, 0)
	newEntries = append(newEntries, newCoinbase...)
	logger.Printf("[SyncHistory] Fetched %d new entries (%d normal, %d coinbase) starting from height %d\n", len(newEntries), len(newEntries)-len(newCoinbase), len(newCoinbase), startHeight)

	// 4. Also sync messages incrementally
	allMessages := scanMessageTransfers(0)

	// 5. Merge and Deduplicate by TXID + Height + Amount (to handle multiple outputs/rewards in one TX)
	uniqueTransfers := make(map[string]rpc.Entry)
	if !forceRebuild {
		for _, t := range cachedTransfers {
			key := fmt.Sprintf("%s-%d-%d", t.TXID, t.Height, t.Amount)
			uniqueTransfers[key] = t
		}
	}
	for _, t := range newEntries {
		key := fmt.Sprintf("%s-%d-%d", t.TXID, t.Height, t.Amount)
		uniqueTransfers[key] = t
	}

	transfers = make([]rpc.Entry, 0, len(uniqueTransfers))
	coinbaseCount := 0
	normalCount := 0
	for _, t := range uniqueTransfers {
		transfers = append(transfers, t)
		if t.Coinbase {
			coinbaseCount++
		} else {
			normalCount++
		}
	}
	logger.Printf("[SyncHistory] Deduplicated totals: %d coinbase, %d normal (from %d fetched)\n", coinbaseCount, normalCount, len(newEntries))

	// 6. Sort according to user preference
	sort.Slice(transfers, func(i, j int) bool {
		if historySortOrder == "Descending" {
			if transfers[i].Height != transfers[j].Height {
				return transfers[i].Height > transfers[j].Height
			}
			return transfers[i].Time.After(transfers[j].Time)
		} else {
			if transfers[i].Height != transfers[j].Height {
				return transfers[i].Height < transfers[j].Height
			}
			return transfers[i].Time.Before(transfers[j].Time)
		}
	})

	// 6. Build ALL rows from the sorted list to ensure perfect order
	normalRows, coinbaseRows, _ = buildHistoryRows(transfers, nil)
	_, _, messageRows = buildHistoryRows(nil, allMessages)

	// 7. Update cache
	setHistoryRowCache(transfers, normalRows, coinbaseRows, messageRows)

	return transfers, normalRows, coinbaseRows, messageRows
}

func getDateLabel(t time.Time) string {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	txUTC := t.UTC()
	txDate := time.Date(txUTC.Year(), txUTC.Month(), txUTC.Day(), 0, 0, 0, 0, time.UTC)

	if txDate.Equal(today) {
		return "T O D A Y  -  " + txUTC.Format("JAN 02, 2006")
	} else if txDate.Equal(yesterday) {
		return "Y E S T E R D A Y  -  " + txUTC.Format("JAN 02, 2006")
	} else {
		return strings.ToUpper(txUTC.Format("January 02, 2006"))
	}
}

func buildHistoryRows(entries []rpc.Entry, messages []MessageRecord) (normalRows []string, coinbaseRows []string, messageRows []string) {
	normalRows = make([]string, 0, len(entries))
	coinbaseRows = make([]string, 0, len(entries))
	messageRows = make([]string, 0, len(messages))

	// Track dates separately for each tab so headers aren't skipped
	var lastNormalDate string
	var lastCoinbaseDate string
	for e := range entries {
		var direction string
		stamp := entries[e].Time.Format("2006-01-02")
		height := strconv.FormatUint(entries[e].Height, 10)
		txid := entries[e].TXID
		status := fmt.Sprintf("%d", entries[e].Status)
		comment := messageComment(entries[e])
		if len(comment) > 20 {
			comment = comment[0:20] + ".."
		}

		currentDateLabel := getDateLabel(entries[e].Time)

		if entries[e].Coinbase {
			if currentDateLabel != lastCoinbaseDate {
				coinbaseRows = append(coinbaseRows, "HEADER;;;"+currentDateLabel+";;; ;;; ;;; ")
			}
			amount := entries[e].Amount
			coinbaseRows = append(coinbaseRows, "Received;;;"+globals.FormatMoney(amount)+";;;"+height+";;;"+stamp+";;;"+txid+";;;"+status+";;;"+comment)
			lastCoinbaseDate = currentDateLabel
		} else {
			if currentDateLabel != lastNormalDate {
				normalRows = append(normalRows, "HEADER;;;"+currentDateLabel+";;; ;;; ;;; ")
			}
			if !entries[e].Incoming {
				direction = "Sent"
				normalRows = append(normalRows, direction+";;;("+globals.FormatMoney(entries[e].Amount)+");;;"+height+";;;"+stamp+";;;"+txid+";;;"+status+";;;"+comment)
			} else {
				direction = "Received"
				normalRows = append(normalRows, direction+";;;"+globals.FormatMoney(entries[e].Amount)+";;;"+height+";;;"+stamp+";;;"+txid+";;;"+status+";;;"+comment)
			}
			lastNormalDate = currentDateLabel
		}
	}

	var lastMessageDate string
	for _, message := range messages {
		direction := "Received"
		if !message.Entry.Incoming {
			direction = "Sent    "
		}

		currentDateLabel := getDateLabel(message.Entry.Time)
		if currentDateLabel != lastMessageDate {
			messageRows = append(messageRows, "HEADER;;;"+currentDateLabel+";;; ;;; ;;; ")
		}

		username := message.Label
		if username == "" {
			username = message.ContactKey
		}
		if len(username) > 10 {
			username = username[0:10] + ".."
		}
		comment := message.Comment
		if len(comment) > 15 {
			comment = comment[0:15] + ".."
		}
		stamp := message.Entry.Time.Format("2006-01-02")

		messageRows = append(messageRows, direction+";;;"+username+";;;"+comment+";;;"+stamp+";;;"+message.Entry.TXID+";;;"+message.ContactKey)
		lastMessageDate = currentDateLabel
	}

	return normalRows, coinbaseRows, messageRows
}

var historyRefreshState struct {
	sync.Mutex
	running bool
}

// refreshHistoryAsync performs a background update of the transaction history cache.
func refreshHistoryAsync(force bool) {
	if engram.Disk == nil {
		return
	}
	generation := currentWalletGeneration()

	historyRefreshState.Lock()
	if historyRefreshState.running {
		historyRefreshState.Unlock()
		return
	}
	historyRefreshState.running = true
	historyRefreshState.Unlock()

	go func() {
		defer func() {
			historyRefreshState.Lock()
			historyRefreshState.running = false
			historyRefreshState.Unlock()
		}()

		if !isWalletGenerationActive(generation) || globals.Exit_In_Progress {
			return
		}

		if force {
			historyRowCache.Lock()
			historyRowCache.Loaded = false
			historyRowCache.Height = 0
			historyRowCache.Unlock()
		}

		syncHistoryRows()
	}()
}

type persistedMessageRecord struct {
	TXID            string `json:"txid"`
	Height          uint64 `json:"height"`
	TimeUnix        int64  `json:"time_unix"`
	Incoming        bool   `json:"incoming"`
	Destination     string `json:"destination"`
	DestinationPort uint64 `json:"destination_port"`
	SourcePort      uint64 `json:"source_port"`
	Replyback       string `json:"replyback"`
	ContactKey      string `json:"contact_key"`
	Label           string `json:"label"`
	Comment         string `json:"comment"`
}

type persistedMessageCache struct {
	Version       int                      `json:"version"`
	Network       string                   `json:"network"`
	WalletAddress string                   `json:"wallet_address"`
	Height        uint64                   `json:"height"`
	SavedAtUnix   int64                    `json:"saved_at_unix"`
	Records       []persistedMessageRecord `json:"records"`
	Threads       []persistedMessageThread `json:"threads"`
}

type persistedMessageThread struct {
	ContactKey string `json:"contact_key"`
	Label      string `json:"label"`
	LastText   string `json:"last_text"`
	LastTime   int64  `json:"last_time"`
	LastTXID   string `json:"last_txid"`
	Count      int    `json:"count"`
}

type threadViewState struct {
	LastViewedUnix int64 `json:"last_viewed_unix"`
}

const messageCacheVersion = 1

var messageRefreshState struct {
	sync.Mutex
	running bool
}

func messageComment(entry rpc.Entry) string {
	if entry.Payload_RPC.HasValue(rpc.RPC_COMMENT, rpc.DataString) {
		return strings.TrimSpace(entry.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string))
	}

	if entry.Payload_RPC.HasValue("C", rpc.DataString) {
		return strings.TrimSpace(entry.Payload_RPC.Value("C", rpc.DataString).(string))
	}

	return ""
}

func decodePayloadWithTrim(payload []byte) (rpc.Arguments, error) {
	var args rpc.Arguments
	if len(payload) == 0 {
		return nil, errors.New("zero length payload")
	}

	if err := args.UnmarshalBinary(payload); err == nil {
		return args, nil
	}

	trimmed := bytes.TrimRight(payload, "\x00")
	if len(trimmed) != len(payload) {
		var trimmedArgs rpc.Arguments
		if err := trimmedArgs.UnmarshalBinary(trimmed); err == nil {
			return trimmedArgs, nil
		}
	}

	for end := len(payload) - 1; end >= 1; end-- {
		candidate := payload[:end]
		var try rpc.Arguments
		if err := try.UnmarshalBinary(candidate); err == nil {
			return try, nil
		}
	}

	return nil, errors.New("unable to decode trimmed payload")
}

func applyDecodedArgs(entry *rpc.Entry, args rpc.Arguments) {
	entry.Payload_RPC = append([]rpc.Argument{}, args...)
	entry.PayloadError = ""
	if args.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
		entry.DestinationPort = args.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)
	}
	if args.Has(rpc.RPC_SOURCE_PORT, rpc.DataUint64) {
		entry.SourcePort = args.Value(rpc.RPC_SOURCE_PORT, rpc.DataUint64).(uint64)
	}
}

func enrichMessageEntry(base rpc.Entry) rpc.Entry {
	if engram.Disk == nil || !session.WalletOpen {
		return base
	}

	var zeroscid crypto.Hash

	if _, err := base.ProcessPayload(); err == nil && messageComment(base) != "" {
		return base
	}
	if args, err := decodePayloadWithTrim(base.Payload); err == nil {
		applyDecodedArgs(&base, args)
		if messageComment(base) != "" {
			return base
		}
	}
	if engram.Disk == nil || !session.WalletOpen {
		return base
	}

	_, detail := engram.Disk.Get_Payments_TXID(zeroscid, base.TXID)
	if detail.TXID == "" {
		return base
	}

	if _, err := detail.ProcessPayload(); err == nil {
		if messageComment(detail) != "" {
			return detail
		}
	}
	if args, err := decodePayloadWithTrim(detail.Payload); err == nil {
		applyDecodedArgs(&detail, args)
		if messageComment(detail) != "" {
			return detail
		}
	}

	if len(base.Payload_RPC) == 0 && len(detail.Payload_RPC) > 0 {
		base.Payload_RPC = detail.Payload_RPC
	}
	if len(base.Payload) == 0 && len(detail.Payload) > 0 {
		base.Payload = detail.Payload
	}
	if base.PayloadError == "" && detail.PayloadError != "" {
		base.PayloadError = detail.PayloadError
	}
	if base.Destination == "" {
		base.Destination = detail.Destination
	}
	if base.DestinationPort == 0 {
		base.DestinationPort = detail.DestinationPort
	}
	if base.SourcePort == 0 {
		base.SourcePort = detail.SourcePort
	}

	if _, err := base.ProcessPayload(); err != nil {
		if args, trimErr := decodePayloadWithTrim(base.Payload); trimErr == nil {
			applyDecodedArgs(&base, args)
		}
	}
	return base
}

func messageReplyback(entry rpc.Entry) string {
	if entry.Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		return strings.TrimSpace(entry.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string))
	}

	return ""
}

func messageDstPort(entry rpc.Entry) uint64 {
	if entry.DestinationPort != 0 {
		return entry.DestinationPort
	}

	if entry.Payload_RPC.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
		return entry.Payload_RPC.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)
	}

	return 0
}

func resolveMessageContact(contact string, height int64) (key string, label string) {
	contact = strings.TrimSpace(contact)
	if contact == "" {
		return "", ""
	}

	if addr, err := globals.ParseValidateAddress(contact); err == nil {
		return addr.String(), ""
	}

	if resolved, err := checkUsername(contact, height); err == nil && strings.TrimSpace(resolved) != "" {
		return strings.TrimSpace(resolved), contact
	}

	return contact, ""
}

func resolveAddressDisplay(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}

	addressDisplayCacheMu.RLock()
	if cached, ok := addressDisplayCache[address]; ok {
		addressDisplayCacheMu.RUnlock()
		return cached
	}
	addressDisplayCacheMu.RUnlock()

	usernames, err := queryUsernames(address)
	if err == nil && len(usernames) > 0 && strings.TrimSpace(usernames[0]) != "" {
		addressDisplayCacheMu.Lock()
		addressDisplayCache[address] = usernames[0]
		addressDisplayCacheMu.Unlock()
		return usernames[0]
	}

	stored, err := getUsernames()
	if err == nil {
		for _, username := range stored {
			username = strings.TrimSpace(username)
			if username == "" {
				continue
			}

			if resolved, resolveErr := checkUsername(username, -1); resolveErr == nil && strings.TrimSpace(resolved) == address {
				addressDisplayCacheMu.Lock()
				addressDisplayCache[address] = username
				addressDisplayCacheMu.Unlock()
				return username
			}
		}
	}

	addressDisplayCacheMu.Lock()
	addressDisplayCache[address] = ""
	addressDisplayCacheMu.Unlock()
	return ""
}

func canonicalThreadKeyForMessage(message MessageRecord) string {
	if key, _ := resolveMessageContact(message.ContactKey, int64(message.Entry.Height)); key != "" {
		return key
	}

	if message.Entry.Incoming {
		if replyback := messageReplyback(message.Entry); replyback != "" {
			if key, _ := resolveMessageContact(replyback, int64(message.Entry.Height)); key != "" {
				return key
			}
		}
	} else {
		if key, _ := resolveMessageContact(message.Entry.Destination, -1); key != "" {
			return key
		}
	}

	return strings.TrimSpace(message.ContactKey)
}

func currentMessageCacheKey() string {
	if engram.Disk == nil {
		return ""
	}

	return fmt.Sprintf("cache_%s_%s", session.Network, engram.Disk.GetAddress().String())
}

func resetMessageCache() {
	messageCacheMu.Lock()
	messageCache = MessageCache{}
	messageCacheMu.Unlock()
	addressDisplayCacheMu.Lock()
	addressDisplayCache = map[string]string{}
	addressDisplayCacheMu.Unlock()
	messageRefreshState.Lock()
	messageRefreshState.running = false
	messageRefreshState.Unlock()
}

func buildMessageCacheSnapshot(records []MessageRecord, height uint64, address string) {
	messageCacheMu.Lock()
	defer messageCacheMu.Unlock()

	messageCache.Height = height
	messageCache.Address = address
	messageCache.Primed = true
	messageCache.Loaded = true
	messageCache.Records = make([]MessageRecord, len(records))
	copy(messageCache.Records, records)
	messageCache.ByTXID = make(map[string]MessageRecord, len(records))
	messageCache.ByThread = make(map[string][]MessageRecord)
	for _, record := range records {
		messageCache.ByTXID[record.Entry.TXID] = record
		key := canonicalThreadKeyForMessage(record)
		messageCache.ByThread[key] = append(messageCache.ByThread[key], record)
	}
	for key := range messageCache.ByThread {
		sort.Slice(messageCache.ByThread[key], func(i, j int) bool {
			return messageCache.ByThread[key][i].Entry.Time.Before(messageCache.ByThread[key][j].Entry.Time)
		})
	}
	messageCache.Threads = buildMessageThreadSummaries(records)
}

func mergeMessageRecordsIntoCache(records []MessageRecord, height uint64, address string) {
	messageCacheMu.Lock()
	defer messageCacheMu.Unlock()

	if !messageCache.Primed || messageCache.Address != address || messageCache.ByTXID == nil || messageCache.ByThread == nil {
		messageCacheMu.Unlock()
		buildMessageCacheSnapshot(records, height, address)
		messageCacheMu.Lock()
		return
	}

	changedThreads := map[string]struct{}{}
	for _, record := range records {
		messageCache.ByTXID[record.Entry.TXID] = record
		changedThreads[canonicalThreadKeyForMessage(record)] = struct{}{}
	}

	messageCache.Records = messageCache.Records[:0]
	for _, record := range messageCache.ByTXID {
		messageCache.Records = append(messageCache.Records, record)
	}
	sort.Slice(messageCache.Records, func(i, j int) bool {
		return messageCache.Records[i].Entry.Time.Before(messageCache.Records[j].Entry.Time)
	})

	for key := range changedThreads {
		threadRecords := make([]MessageRecord, 0)
		for _, record := range messageCache.Records {
			if canonicalThreadKeyForMessage(record) == key {
				threadRecords = append(threadRecords, record)
			}
		}
		messageCache.ByThread[key] = threadRecords
	}

	threadMap := make(map[string]MessageThreadSummary, len(messageCache.Threads))
	for _, thread := range messageCache.Threads {
		threadMap[thread.ContactKey] = thread
	}
	for key := range changedThreads {
		threadRecords := messageCache.ByThread[key]
		if len(threadRecords) == 0 {
			delete(threadMap, key)
			continue
		}
		summaries := buildMessageThreadSummaries(threadRecords)
		if len(summaries) > 0 {
			threadMap[key] = summaries[0]
		}
	}

	messageCache.Threads = make([]MessageThreadSummary, 0, len(threadMap))
	for _, thread := range threadMap {
		messageCache.Threads = append(messageCache.Threads, thread)
	}
	sort.Slice(messageCache.Threads, func(i, j int) bool {
		return messageCache.Threads[i].LastTime.After(messageCache.Threads[j].LastTime)
	})

	messageCache.Height = height
	messageCache.Address = address
	messageCache.Primed = true
	messageCache.Loaded = true
}

func buildMessageThreadSummaries(records []MessageRecord) []MessageThreadSummary {
	threads := make(map[string]MessageThreadSummary)
	for _, record := range records {
		key := canonicalThreadKeyForMessage(record)
		summary := threads[key]
		summary.ContactKey = key
		if summary.Label == "" && record.Label != "" {
			summary.Label = record.Label
		}
		summary.Count++
		if summary.LastTXID == "" || summary.LastTime.Before(record.Entry.Time) {
			summary.LastTime = record.Entry.Time
			summary.LastTXID = record.Entry.TXID
			summary.LastText = record.Comment
			if record.Label != "" {
				summary.Label = record.Label
			}
		}
		if summary.Label == "" {
			summary.Label = resolveAddressDisplay(summary.ContactKey)
		}
		threads[key] = summary
	}

	result := make([]MessageThreadSummary, 0, len(threads))
	for _, summary := range threads {
		result = append(result, summary)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LastTime.After(result[j].LastTime)
	})

	return result
}

func savePersistedMessageCache() {
	messageCacheMu.RLock()
	defer messageCacheMu.RUnlock()

	if engram.Disk == nil || !messageCache.Primed {
		return
	}

	persisted := persistedMessageCache{
		Version:       messageCacheVersion,
		Network:       session.Network,
		WalletAddress: messageCache.Address,
		Height:        messageCache.Height,
		SavedAtUnix:   time.Now().Unix(),
		Records:       make([]persistedMessageRecord, 0, len(messageCache.Records)),
		Threads:       make([]persistedMessageThread, 0, len(messageCache.Threads)),
	}

	for _, record := range messageCache.Records {
		persisted.Records = append(persisted.Records, persistedMessageRecord{
			TXID:            record.Entry.TXID,
			Height:          record.Entry.Height,
			TimeUnix:        record.Entry.Time.Unix(),
			Incoming:        record.Entry.Incoming,
			Destination:     record.Entry.Destination,
			DestinationPort: record.Entry.DestinationPort,
			SourcePort:      record.Entry.SourcePort,
			Replyback:       messageReplyback(record.Entry),
			ContactKey:      record.ContactKey,
			Label:           record.Label,
			Comment:         record.Comment,
		})
	}

	for _, thread := range messageCache.Threads {
		persisted.Threads = append(persisted.Threads, persistedMessageThread{
			ContactKey: thread.ContactKey,
			Label:      thread.Label,
			LastText:   thread.LastText,
			LastTime:   thread.LastTime.Unix(),
			LastTXID:   thread.LastTXID,
			Count:      thread.Count,
		})
	}

	data, err := json.Marshal(persisted)
	if err != nil {
		logger.Errorf("[MsgDebug] Failed to marshal message cache: %s", err)
		return
	}

	if err := StoreEncryptedValue("Messages", []byte(currentMessageCacheKey()), data); err != nil {
		logger.Errorf("[MsgDebug] Failed to persist message cache: %s", err)
	}
}

func loadPersistedMessageCache() {
	resetMessageCache()
	if engram.Disk == nil {
		return
	}

	data, err := GetEncryptedValue("Messages", []byte(currentMessageCacheKey()))
	if err != nil || len(data) == 0 {
		return
	}

	var persisted persistedMessageCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		logger.Errorf("[MsgDebug] Failed to decode persisted message cache: %s", err)
		return
	}

	if persisted.Version != messageCacheVersion || persisted.Network != session.Network || persisted.WalletAddress != engram.Disk.GetAddress().String() {
		return
	}

	currentHeight := engram.Disk.Get_Height()
	if persisted.Height > currentHeight {
		return
	}

	records := make([]MessageRecord, 0, len(persisted.Records))
	for _, item := range persisted.Records {
		if item.TXID == "" || item.ContactKey == "" {
			continue
		}

		entry := rpc.Entry{
			TXID:            item.TXID,
			Height:          item.Height,
			Time:            time.Unix(item.TimeUnix, 0),
			Incoming:        item.Incoming,
			Destination:     item.Destination,
			DestinationPort: item.DestinationPort,
			SourcePort:      item.SourcePort,
		}
		if item.Replyback != "" {
			entry.Payload_RPC = append(entry.Payload_RPC, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: item.Replyback})
		}
		if item.Comment != "" {
			entry.Payload_RPC = append(entry.Payload_RPC, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: item.Comment})
		}

		records = append(records, MessageRecord{
			Entry:      entry,
			ContactKey: item.ContactKey,
			Label:      item.Label,
			Comment:    item.Comment,
		})
	}

	buildMessageCacheSnapshot(records, persisted.Height, persisted.WalletAddress)
	if len(persisted.Threads) > 0 {
		messageCacheMu.Lock()
		messageCache.Threads = make([]MessageThreadSummary, 0, len(persisted.Threads))
		for _, thread := range persisted.Threads {
			label := thread.Label
			if label == "" {
				label = resolveAddressDisplay(thread.ContactKey)
			}
			messageCache.Threads = append(messageCache.Threads, MessageThreadSummary{
				ContactKey: thread.ContactKey,
				Label:      label,
				LastText:   thread.LastText,
				LastTime:   time.Unix(thread.LastTime, 0),
				LastTXID:   thread.LastTXID,
				Count:      thread.Count,
			})
		}
		messageCacheMu.Unlock()
	}
}

func getMessageCacheSnapshot() []MessageRecord {
	messageCacheMu.RLock()
	defer messageCacheMu.RUnlock()

	if !messageCache.Loaded || len(messageCache.Records) == 0 {
		return nil
	}

	result := make([]MessageRecord, len(messageCache.Records))
	copy(result, messageCache.Records)
	return result
}

func getMessageThreadSnapshot() []MessageThreadSummary {
	messageCacheMu.RLock()
	defer messageCacheMu.RUnlock()

	if len(messageCache.Threads) == 0 {
		return nil
	}

	result := make([]MessageThreadSummary, len(messageCache.Threads))
	copy(result, messageCache.Threads)
	return result
}

func getThreadViewKey(contactKey string) []byte {
	if engram.Disk == nil {
		return nil
	}

	return []byte(fmt.Sprintf("view_%s_%s_%s", session.Network, engram.Disk.GetAddress().String(), strings.TrimSpace(contactKey)))
}

func GetThreadLastViewed(contactKey string) time.Time {
	key := getThreadViewKey(contactKey)
	if len(key) == 0 {
		return time.Time{}
	}

	raw, err := GetEncryptedValue("Messages", key)
	if err != nil || len(raw) == 0 {
		return time.Time{}
	}

	var state threadViewState
	if err := json.Unmarshal(raw, &state); err != nil {
		return time.Time{}
	}

	if state.LastViewedUnix <= 0 {
		return time.Time{}
	}

	return time.Unix(state.LastViewedUnix, 0)
}

func SetThreadLastViewed(contactKey string, t time.Time) error {
	key := getThreadViewKey(contactKey)
	if len(key) == 0 {
		return nil
	}

	data, err := json.Marshal(threadViewState{LastViewedUnix: t.Unix()})
	if err != nil {
		return err
	}

	return StoreEncryptedValue("Messages", key, data)
}

func SearchMessageThreads(query string, height uint64) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	matches := map[string]struct{}{}
	for _, thread := range getMessageThreadSnapshot() {
		if strings.Contains(strings.ToLower(thread.ContactKey), query) || strings.Contains(strings.ToLower(thread.Label), query) {
			matches[thread.ContactKey] = struct{}{}
		}
	}

	records := getMessageCacheSnapshot()
	if len(records) == 0 {
		records = scanMessageTransfers(height)
	}
	for _, record := range records {
		if height > 0 && record.Entry.Height < height {
			continue
		}
		if strings.Contains(strings.ToLower(record.Comment), query) || strings.Contains(strings.ToLower(record.Label), query) {
			matches[canonicalThreadKeyForMessage(record)] = struct{}{}
		}
	}

	result := make([]string, 0, len(matches))
	for key := range matches {
		result = append(result, key)
	}

	return result
}

func rebuildMessageHistory() {
	if engram.Disk == nil {
		return
	}

	if err := DeleteKey("Messages", []byte(currentMessageCacheKey())); err != nil {
		logger.Debugf("[Messages] Could not clear persisted cache: %s\n", err)
	}
	resetMessageCache()
	refreshMessageHistoryAsync(true)
}

func refreshMessageHistoryAsync(force bool) {
	if engram.Disk == nil {
		return
	}
	generation := currentWalletGeneration()

	messageRefreshState.Lock()
	if messageRefreshState.running {
		messageRefreshState.Unlock()
		return
	}
	messageRefreshState.running = true
	messageRefreshState.Unlock()

	go func() {
		defer func() {
			messageRefreshState.Lock()
			messageRefreshState.running = false
			messageRefreshState.Unlock()
		}()

		if !isWalletGenerationActive(generation) || globals.Exit_In_Progress {
			return
		}

		if force {
			messageCacheMu.Lock()
			messageCache.Primed = false
			messageCache.Height = 0
			messageCacheMu.Unlock()
		}

		messageCacheMu.RLock()
		initialCount := len(messageCache.Records)
		messageCacheMu.RUnlock()

		scanMessageTransfers(0)
		if !isWalletGenerationActive(generation) {
			return
		}

		messageCacheMu.RLock()
		finalCount := len(messageCache.Records)
		messageCacheMu.RUnlock()

		if !force && initialCount == finalCount && messageCache.Primed {
			return
		}

		savePersistedMessageCache()

		uiDo(func() {
			if !isWalletGenerationActive(generation) {
				return
			}
			if session.Window == nil {
				return
			}
			if session.Domain == "app.messages" {
				session.Window.SetContent(layoutMessages())
			} else if session.Domain == "app.messages.contact" {
				session.Window.SetContent(layoutPM())
			}
		})
	}()
}

func messageMatchesContact(message MessageRecord, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return false
	}

	selectedKey, selectedLabel := resolveMessageContact(selected, -1)
	messageKey := canonicalThreadKeyForMessage(message)

	if selectedKey != "" && messageKey == selectedKey {
		return true
	}

	if messageKey == selected {
		return true
	}

	if selectedLabel != "" && strings.EqualFold(strings.TrimSpace(message.Label), strings.TrimSpace(selectedLabel)) {
		return true
	}

	if strings.EqualFold(strings.TrimSpace(message.Label), selected) {
		return true
	}

	return false
}

func scanMessageTransfers(minHeight uint64) (result []MessageRecord) {
	if engram.Disk == nil {
		return nil
	}

	currentHeight := engram.Disk.Get_Height()
	currentAddress := engram.Disk.GetAddress().String()
	if minHeight == 0 && messageCache.Primed && messageCache.Address == currentAddress && messageCache.Height == currentHeight && len(messageCache.Records) > 0 {
		cached := make([]MessageRecord, len(messageCache.Records))
		copy(cached, messageCache.Records)
		return cached
	}

	startHeight := minHeight
	cacheReusable := minHeight == 0 && messageCache.Primed && messageCache.Address == currentAddress && messageCache.Height <= currentHeight
	if cacheReusable && messageCache.Height > 0 {
		startHeight = messageCache.Height + 1
	}

	var zeroscid crypto.Hash
	messageAmount, _ := globals.ParseAmount("0.00001")
	entries := engram.Disk.Show_Transfers(zeroscid, false, true, true, startHeight, currentHeight, "", "", 0, 0)
	logger.Printf("[MsgDebug] Show_Transfers returned %d entries for message scan", len(entries))

	if cacheReusable {
		result = make([]MessageRecord, len(messageCache.Records))
		copy(result, messageCache.Records)
	}

	for i := range entries {
		if engram.Disk == nil || !session.WalletOpen {
			break
		}

		entry := entries[i]
		entry = enrichMessageEntry(entry)
		entryErr := error(nil)
		if len(entry.Payload_RPC) == 0 {
			_, entryErr = entry.ProcessPayload()
		}

		if engram.Disk == nil || !session.WalletOpen {
			break
		}

		_, detail := engram.Disk.Get_Payments_TXID(zeroscid, entry.TXID)
		if detail.TXID != "" {
			_, _ = detail.ProcessPayload()
			if entry.Destination == "" {
				entry.Destination = detail.Destination
			}
			if len(entry.Payload_RPC) == 0 && len(detail.Payload_RPC) > 0 {
				entry.Payload_RPC = detail.Payload_RPC
			}
			if len(entry.Payload) == 0 && len(detail.Payload) > 0 {
				entry.Payload = detail.Payload
			}
			if entry.PayloadError == "" && detail.PayloadError != "" {
				entry.PayloadError = detail.PayloadError
			}
			if entry.DestinationPort == 0 {
				entry.DestinationPort = detail.DestinationPort
			}
			if entry.SourcePort == 0 {
				entry.SourcePort = detail.SourcePort
			}
			if entry.Time.IsZero() {
				entry.Time = detail.Time
			}
			if entry.Height == 0 {
				entry.Height = detail.Height
			}
		}

		port := messageDstPort(entry)
		comment := messageComment(entry)
		replyback := messageReplyback(entry)
		isMessageAmount := !entry.Incoming && entry.Amount == messageAmount
		hasMessagePayload := comment != "" || replyback != ""

		if port != 1337 && !isMessageAmount && !hasMessagePayload {
			continue
		}

		_ = entryErr

		record := MessageRecord{
			Entry:   entry,
			Comment: comment,
		}

		if entry.Incoming {
			if replyback == "" {
				continue
			}

			record.ContactKey, record.Label = resolveMessageContact(replyback, int64(entry.Height))
		} else {
			record.ContactKey, record.Label = resolveMessageContact(entry.Destination, -1)
		}

		if record.ContactKey == "" {
			continue
		}
		record.ContactKey = canonicalThreadKeyForMessage(record)

		if record.Label == "" {
			record.Label = resolveAddressDisplay(record.ContactKey)
		}

		if record.Comment == "" {
			record.Comment = "[message]"
		}

		if cacheReusable {
			if _, exists := messageCache.ByTXID[record.Entry.TXID]; exists {
				for idx := range result {
					if result[idx].Entry.TXID == record.Entry.TXID {
						result[idx] = record
						break
					}
				}
			} else {
				result = append(result, record)
			}
		} else {
			result = append(result, record)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Entry.Time.After(result[j].Entry.Time)
	})

	if minHeight == 0 {
		if cacheReusable && startHeight > 0 {
			newRecords := []MessageRecord{}
			for _, record := range result {
				if _, exists := messageCache.ByTXID[record.Entry.TXID]; !exists {
					newRecords = append(newRecords, record)
				}
			}
			mergeMessageRecordsIntoCache(newRecords, currentHeight, currentAddress)
		} else {
			buildMessageCacheSnapshot(result, currentHeight, currentAddress)
		}
	}

	logger.Printf("[MsgDebug] scanMessageTransfers returning %d messages", len(result))
	return result
}

// Get a list of all message transactions and sort them by address
func getMessages(h uint64) (result []string) {
	latestByContact := make(map[string]MessageRecord)
	for _, message := range scanMessageTransfers(h) {
		previous, ok := latestByContact[message.ContactKey]
		if !ok || previous.Entry.Time.Before(message.Entry.Time) {
			latestByContact[message.ContactKey] = message
		}
	}

	type contactRow struct {
		key   string
		label string
		time  time.Time
	}

	rows := make([]contactRow, 0, len(latestByContact))
	for _, message := range latestByContact {
		rows = append(rows, contactRow{key: canonicalThreadKeyForMessage(message), label: message.Label, time: message.Entry.Time})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].time.After(rows[j].time)
	})

	for _, row := range rows {
		result = append(result, row.key+"~~~"+row.label)
	}

	return
}

// Returns a list of registered usernames from Gnomon
func queryUsernames(address string) (result []string, err error) {
	generation := currentWalletGeneration()
	if gnomon.Index != nil && engram.Disk != nil {
		if !isWalletGenerationActive(generation) {
			return nil, nil
		}
		result, _ = gnomon.Graviton.GetSCIDKeysByValue("0000000000000000000000000000000000000000000000000000000000000001", address, engram.Disk.Get_Daemon_TopoHeight(), false)
		if !isWalletGenerationActive(generation) {
			return nil, nil
		}
		if len(result) <= 0 {
			result, _, err = gnomon.Index.GetSCIDKeysByValue(nil, "0000000000000000000000000000000000000000000000000000000000000001", address, engram.Disk.Get_Daemon_TopoHeight())
			if !isWalletGenerationActive(generation) {
				return nil, nil
			}
			if err != nil {
				if !strings.Contains(err.Error(), "closed network connection") {
					logger.Errorf("[Gnomon] Querying usernames failed: %s\n", err)
				}
				return
			}
		}

		sort.Strings(result)
	}

	return
}

// Get the local list of registered usernames saved from previous Gnomon scans
func getUsernames() (result []string, err error) {
	usernames, err := GetEncryptedValue("Usernames", []byte("usernames"))
	if err != nil {
		return
	}

	result = strings.Split(string(usernames), ",")
	return
}

// Set the Primary Username saved to a wallet's datashard
func setPrimaryUsername(s string) (err error) {
	err = StoreEncryptedValue("settings", []byte("username"), []byte(s))
	return
}

// Get the Primary Username saved to a wallet's datashard
func getPrimaryUsername() (err error) {
	u, err := GetEncryptedValue("settings", []byte("username"))
	if err != nil {
		session.Username = ""
		return
	}
	session.Username = string(u)
	return
}

// Start the Gnomon indexer
func startGnomon() {
	defer func() {
		if r := recover(); r != nil {
			gnomon.setBootstrapError("Gnomon stopped unexpectedly")
			logger.Printf("[Gnomon] Panic recovered in startGnomon: %v\n", r)
		}
	}()

	generation := currentWalletGeneration()
	if !isWalletGenerationActive(generation) {
		return
	}
	if globals.Exit_In_Progress {
		return
	}

	gnomonMu.Lock()
	defer gnomonMu.Unlock()

	if walletapi.Connected {
		if gnomon.Index == nil && gnomon.Active == 1 {
			gnomon.resetBootstrapState()
			gnomon.setBootstrapPhase(i18n.T("tela.status_connecting_gnomon"), 0, 0)
			gnomon.Active = 2
			path := filepath.Join(AppPath(), "datashards", "gnomon")
			switch session.Network {
			case NETWORK_TESTNET:
				path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
			case NETWORK_SIMULATOR:
				path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
			}

			// Check if database exists and might be corrupted
			if _, err := os.Stat(path); err == nil {
				// Database directory exists, validate it
				if isDatabaseCorrupted(path) {
					logger.Printf("[Gnomon] Database appears corrupted, attempting recovery...\n")

					// Backup corrupted database
					backupPath := path + "_corrupted_" + time.Now().Format("20060102_150405")
					if err := os.Rename(path, backupPath); err == nil {
						logger.Printf("[Gnomon] Backed up corrupted database to: %s\n", backupPath)

						// Show recovery message to user
						fyne.Do(func() {
							if session.Window != nil {
								dialog.ShowInformation("Database Recovery",
									fmt.Sprintf("Corrupted Gnomon database detected and backed up.\nDatabase will be recreated with fresh sync.\nBackup location: %s", backupPath),
									session.Window)
							}
						})
					}
				}
			}

			// Initialize fresh databases with retry logic and configurable timeout
			var err error
			maxRetries := 3
			baseTimeout := 5 * time.Second // Start with 5 seconds, increase on retry

			// Try to load timeout from settings
			timeoutStr, err := GetValue("settings", []byte("db_timeout"))
			if err == nil {
				if timeout, err := time.ParseDuration(string(timeoutStr)); err == nil {
					baseTimeout = timeout
					logger.Printf("[Gnomon] Using database timeout from settings: %v", baseTimeout)
				}
			} else {
				logger.Printf("[Gnomon] Using default database timeout: %v", baseTimeout)
			}

			for attempt := 0; attempt < maxRetries; attempt++ {
				gnomon.BBolt, err = storage.NewBBoltDB(path, "gnomon")
				if err == nil {
					logger.Printf("[Gmonon] Successfully created BBoltDB on attempt %d\n", attempt+1)
					break
				}

				logger.Printf("[Gmonon] Error creating BBoltDB on attempt %d: %v\n", attempt+1, err)
				if !strings.Contains(strings.ToLower(err.Error()), "timeout") {
					break
				}

				if attempt < maxRetries-1 {
					time.Sleep(time.Duration(attempt+1) * baseTimeout)
				}
			}

			if err != nil || gnomon.BBolt == nil {
				gnomon.setBootstrapError("Connection timeout")
				logger.Printf("[Gnomon] Failed to initialize BBoltDB: %v\n", err)
				return
			}
			gnomon.Graviton, err = storage.NewGravDB(path, "25ms")
			if err != nil {
				gnomon.setBootstrapError("Connection timeout")
				logger.Printf("[Gmonon] Error creating GravDB: %v\n", err)
				return
			}

			term := []string(nil)
			term = append(term, "Function Initialize")
			height, err := gnomon.Graviton.GetLastIndexHeight()
			if err != nil {
				height = 0
			}

			// Fastsync Config
			config := &structures.FastSyncConfig{
				Enabled:           true,
				SkipFSRecheck:     true,
				ForceFastSync:     true,
				ForceFastSyncDiff: 20,
				NoCode:            true,
			}

			// exclude the Gnomon SC, etc. to keep faster sync times
			var exclusions []string

			if !isWalletGenerationActive(generation) || globals.Exit_In_Progress {
				gnomon.resetBootstrapState()
				gnomon.Active = 1
				return
			}

			gnomon.Index = indexer.NewIndexer(gnomon.Graviton, gnomon.BBolt, "gravdb", term, height, session.Daemon, "daemon", false, false, config, exclusions, false)
			gnomon.setBootstrapPhase("Validating fastsync contract...", 0, 0)
			indexer.InitLog(globals.Arguments, os.Stdout)
			parallelBlocks := 8
			if isMobile() {
				parallelBlocks = 4
			}

			// We can allow parallel processing of x blocks at a time
			go func() {
				defer func() {
					if r := recover(); r != nil {
						gnomon.setBootstrapError("Gnomon stopped unexpectedly")
						logger.Printf("[Gnomon] Critical error in StartDaemonMode: %v\n", r)
						logger.Printf("[Gnomon] This usually indicates corrupted database. Please use 'Clear Local Data' in Advanced settings.\n")

						// Try to gracefully handle the panic by stopping gnomon
						stopGnomon()

						// Show recovery dialog on main thread
						fyne.Do(func() {
							if session.Window != nil {
								dialog.ShowError(fmt.Errorf("Gnomon database corrupted. Please use 'Clear Local Data' in Advanced settings to recover."), session.Window)
							}
						})
					}
				}()
				if !isWalletGenerationActive(generation) || gnomon.Index == nil {
					return
				}
				gnomon.setBootstrapPhase("Starting index routine...", 0, 0)
				gnomon.Index.StartDaemonMode(parallelBlocks)
				if !isWalletGenerationActive(generation) {
					return
				}
			}()

			logger.Printf("[Gnomon] Scan Status: [%d / %d]\n", height, gnomon.Index.LastIndexedHeight)
			gnomon.Active = 1
			if gnomon.telaBootstrapReady() {
				gnomon.setBootstrapReady()
			}
		}
	}
}

// Check if database might be corrupted
func isDatabaseCorrupted(path string) bool {
	// Enhanced validation - multiple checks
	logger.Printf("[Gnomon] Validating database at: %s\n", path)

	// Check file permissions and basic integrity
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false // No database = not corrupted
		}
		return true // Permission issues = corruption
	}

	// Try basic validation with recovery
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("[Gnomon] Database validation panic: %v\n", r)
		}
	}()

	// Try to open database safely
	grav, err := storage.NewGravDB(path, "25ms")
	if err != nil {
		logger.Printf("[Gnomon] Cannot open database: %v\n", err)
		return true
	}

	// Multiple validation attempts
	_, err = grav.GetLastIndexHeight()
	if err != nil {
		logger.Printf("[Gnomon] Database validation error: %v\n", err)
		return true
	}

	// Additional validation - try snapshot creation (often exposes corruption)
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("[Gnomon] Snapshot validation panic: %v\n", r)
		}
	}()

	logger.Printf("[Gnomon] Database appears valid, but being cautious...\n")
	return false
}

// Stop all indexers and close Gnomon
func stopGnomon() {
	gnomonMu.Lock()
	defer gnomonMu.Unlock()
	gnomon.resetBootstrapState()

	if gnomon.Index != nil {
		gnomon.Active = 0
		gnomon.Index.Close()
		gnomon.Index = nil
		logger.Printf("[Gnomon] Closed all indexers.\n")
	}
	if gnomon.Index == nil {
		gnomon.Active = 0
	}
	// CRITICAL FIX: Also close BBolt database to release file lock
	if gnomon.BBolt != nil && gnomon.BBolt.DB != nil {
		gnomon.BBolt.DB.Close()
		gnomon.BBolt = nil
		logger.Printf("[Gnomon] Closed BBolt database.\n")
	}
}

// Save TELA scan progress to encrypted storage
func saveScanProgress(position, total int, lastSCID, state string) {
	progress := ScanProgress{
		Position:  position,
		Total:     total,
		LastSCID:  lastSCID,
		State:     state,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(progress)
	if err != nil {
		logger.Printf("[Gnomon] Error marshaling scan progress: %s\n", err)
		return
	}

	err = StoreEncryptedValue("TELA Search", []byte("ScanProgress"), data)
	if err != nil {
		logger.Printf("[Gnomon] Error saving scan progress: %s\n", err)
	} else {
		logger.Printf("[Gnomon] Saved scan progress: %d/%d, state: %s\n", position, total, state)
	}
}

// Load TELA scan progress from encrypted storage
func loadScanProgress() ScanProgress {
	var progress ScanProgress

	stored, err := GetEncryptedValue("TELA Search", []byte("ScanProgress"))
	if err != nil || stored == nil {
		return progress
	}

	if err := json.Unmarshal(stored, &progress); err != nil {
		logger.Printf("[Gnomon] Error loading scan progress: %s\n", err)
		return ScanProgress{}
	}

	logger.Printf("[Gnomon] Loaded scan progress: %d/%d, state: %s, lastSCID: %s\n",
		progress.Position, progress.Total, progress.State, progress.LastSCID)

	return progress
}

// Clear TELA scan progress from storage
func clearScanProgress() {
	err := StoreEncryptedValue("TELA Search", []byte("ScanProgress"), nil)
	if err != nil {
		logger.Printf("[Gnomon] Error clearing scan progress: %s\n", err)
	} else {
		logger.Printf("[Gnomon] Cleared scan progress\n")
	}
}

// Clear all TELA cache from storage (comprehensive cache clear)
func clearAllTELACache() {
	keysToClear := [][]byte{
		[]byte("ScanProgress"),
		[]byte("SCIDs"),
		[]byte("Searched SCIDs"),
		[]byte("NegativeCache"),
		[]byte("IndexCache"),
		[]byte("CandidateCache"),
		[]byte("DisplayCache"),
		[]byte("Last Scan"),
		[]byte("Last Indexed Height"),
	}
	for _, key := range keysToClear {
		if err := DeleteKey("TELA Search", key); err != nil {
			logger.Printf("[TELA] Error clearing key %s: %v\n", string(key), err)
		} else {
			logger.Printf("[TELA] Cleared key: %s\n", string(key))
		}
	}
	logger.Printf("[TELA] Cleared all TELA cache from storage\n")
}

// Reset Gnomon index for a complete fresh scan
func resetGnomonIndex() error {
	if gnomon.Index != nil {
		logger.Printf("[Gnomon] Stopping indexer for full reset...\n")
		stopGnomon()
		time.Sleep(1 * time.Second)
	}

	path := filepath.Join(AppPath(), "datashards", "gnomon")
	switch session.Network {
	case NETWORK_TESTNET:
		path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
	case NETWORK_SIMULATOR:
		path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
	}

	if _, err := os.Stat(path); err == nil {
		backupPath := path + "_reset_" + time.Now().Format("20060102_150405")
		if err := os.Rename(path, backupPath); err != nil {
			logger.Printf("[Gnomon] Error backing up database: %v\n", err)
			return err
		}
		logger.Printf("[Gnomon] Backed up database to: %s\n", backupPath)
	}

	gnomon.Active = 1
	go startGnomon()
	logger.Printf("[Gnomon] Started fresh indexing\n")
	return nil
}

// Check if scan progress is stale (older than specified hours)
func isScanProgressStale(progress ScanProgress, hours int) bool {
	if progress.Timestamp == 0 {
		return true
	}
	age := time.Now().Unix() - progress.Timestamp
	return age > int64(hours*3600)
}

// Check if an error is a connection-related error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"timeout",
		"no route to host",
		"no such host",
		"network is unreachable",
		"broken pipe",
		"EOF",
		"i/o timeout",
		"context deadline exceeded",
		"dial tcp",
		"lookup",
	}
	errStrLower := strings.ToLower(errStr)
	for _, ce := range connectionErrors {
		if strings.Contains(errStrLower, ce) {
			return true
		}
	}
	return false
}

// Method of Gnomon GetAllOwnersAndSCIDs() where DB type is defined by Indexer.DBType
func (g *Gnomon) resetBootstrapState() {
	g.bootMu.Lock()
	g.boot = GnomonBootstrapState{}
	g.bootMu.Unlock()
}

func (g *Gnomon) setBootstrapPhase(phase string, current, total int64) {
	now := time.Now()
	g.bootMu.Lock()
	if g.boot.StartedAt.IsZero() {
		g.boot.StartedAt = now
	}
	g.boot.Phase = phase
	g.boot.Current = current
	g.boot.Total = total
	g.boot.Active = true
	g.boot.Ready = false
	g.boot.Err = ""
	g.boot.UpdatedAt = now
	g.bootMu.Unlock()
}

func (g *Gnomon) setBootstrapReady() {
	now := time.Now()
	g.bootMu.Lock()
	if g.boot.StartedAt.IsZero() {
		g.boot.StartedAt = now
	}
	g.boot.Active = false
	g.boot.Ready = true
	g.boot.Err = ""
	g.boot.UpdatedAt = now
	g.bootMu.Unlock()
}

func (g *Gnomon) setBootstrapError(msg string) {
	now := time.Now()
	g.bootMu.Lock()
	if g.boot.StartedAt.IsZero() {
		g.boot.StartedAt = now
	}
	g.boot.Active = false
	g.boot.Ready = false
	g.boot.Err = msg
	g.boot.UpdatedAt = now
	g.bootMu.Unlock()
}

func (g *Gnomon) bootstrapState() GnomonBootstrapState {
	g.bootMu.RLock()
	defer g.bootMu.RUnlock()
	return g.boot
}

func (g *Gnomon) telaBootstrapReady() bool {
	if g.Index == nil {
		return false
	}
	if (g.Index.DBType == "gravdb" && g.Index.GravDBBackend == nil) || (g.Index.DBType == "boltdb" && g.Index.BBSBackend == nil) {
		return false
	}
	state := g.bootstrapState()
	if state.Err != "" {
		return false
	}
	if state.Ready {
		return true
	}
	if state.Phase != "Starting index routine..." {
		return false
	}
	if state.UpdatedAt.IsZero() {
		return false
	}
	if time.Since(state.UpdatedAt) < 1500*time.Millisecond {
		return false
	}
	return true
}

func (g *Gnomon) telaSearchReady() bool {
	if !g.telaBootstrapReady() {
		return false
	}
	if g.Index == nil {
		return false
	}
	if g.Index.LastIndexedHeight <= 0 {
		return false
	}
	state := g.bootstrapState()
	if state.Phase == "Starting index routine..." && time.Since(state.UpdatedAt) < 3*time.Second {
		return false
	}
	return true
}

func (g *Gnomon) GetAllOwnersAndSCIDs() (scids map[string]string) {
	if g.Index == nil {
		return
	}
	if g.Index.GravDBBackend == nil && g.Index.BBSBackend == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetAllOwnersAndSCIDs()
	case "boltdb":
		return g.Index.BBSBackend.GetAllOwnersAndSCIDs()
	default:
		return
	}
}

// GetTelaCandidates returns pre-computed TELA candidate SCIDs from Gnomon DB.
// Falls back to an embedded list when the DB is empty (fresh install), making
// the first TELA click fast without waiting for the background backfill.
func (g *Gnomon) GetTelaCandidates() []string {
	if g.Index == nil {
		return nil
	}
	candidates := g.Index.GetTelaCandidates()

	// Merge with embedded list to ensure we don't miss anything known,
	// especially during fresh sync when DB is partially populated.
	if len(embeddedTelaSCIDs) > 0 {
		candidateMap := make(map[string]bool)
		for _, c := range candidates {
			candidateMap[c] = true
		}
		added := 0
		for _, ec := range embeddedTelaSCIDs {
			if !candidateMap[ec] {
				candidates = append(candidates, ec)
				added++
			}
		}
		if added > 0 {
			logger.Printf("[Gnomon] Added %d embedded TELA SCIDs to %d DB candidates\n", added, len(candidates)-added)
		}
	}
	return candidates
}

// Method of Gnomon GetAllSCIDVariableDetails() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetAllSCIDVariableDetails(scid string) (vars []*structures.SCIDVariable) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetAllSCIDVariableDetails(scid)
	case "boltdb":
		return g.Index.BBSBackend.GetAllSCIDVariableDetails(scid)
	default:
		return
	}
}

// Method of Gnomon GetSCIDValuesByKey() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetSCIDValuesByKey(scid string, key interface{}) (valuesstring []string, valuesuint64 []uint64) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetSCIDValuesByKey(scid, key, g.Index.ChainHeight, true)
	case "boltdb":
		return g.Index.BBSBackend.GetSCIDValuesByKey(scid, key, g.Index.ChainHeight, true)
	default:
		return
	}
}

// Method of Gnomon GetSCIDInteractionHeight() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetSCIDInteractionHeight(scid string) (heights []int64) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetSCIDInteractionHeight(scid)
	case "boltdb":
		return g.Index.BBSBackend.GetSCIDInteractionHeight(scid)
	default:
		return
	}
}

// Add a var store only scid to Gnomon DB
func (g *Gnomon) AddSCIDToIndex(scid string) (err error) {
	if g.Index == nil {
		return fmt.Errorf("gnomon index is nil")
	}
	add := make(map[string]*structures.FastSyncImport)
	add[scid] = &structures.FastSyncImport{}

	return gnomon.Index.AddSCIDToIndex(add, false, true)
}

// preIndexFavorites indexes all favorited SCIDs in the background
func preIndexFavorites(favs map[string]*TELAFavoriteData) {
	if gnomon.Index == nil {
		return
	}

	batch := make(map[string]*structures.FastSyncImport)
	for scid := range favs {
		if gnomon.GetAllSCIDVariableDetails(scid) == nil {
			batch[scid] = &structures.FastSyncImport{}
		}
	}

	if len(batch) > 0 {
		if err := gnomon.Index.AddSCIDToIndex(batch, false, true); err != nil {
			logger.Errorf("[Engram] Failed to pre-index favorites: %v\n", err)
		} else {
			logger.Printf("[Engram] Pre-indexed %d favorite SCIDs\n", len(batch))
		}
	}
}

// ScanDependentSCIDs scans a SCID's variables for values that look like valid SCIDs
// (64-character hex strings). Returns deduplicated list of dependent SCIDs.
func ScanDependentSCIDs(scid string) []string {
	if gnomon.Index == nil {
		return nil
	}

	var vars []*structures.SCIDVariable
	switch gnomon.Index.DBType {
	case "gravdb":
		vars = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(scid)
	case "boltdb":
		vars = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(scid)
	}

	deps := make(map[string]bool)
	for _, v := range vars {
		if val, ok := v.Value.(string); ok && len(val) == 64 {
			if _, err := hex.DecodeString(val); err == nil && val != scid {
				deps[val] = true
			}
		}
	}

	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	return result
}

// ScanTELAAppSourceFiles scans cloned TELA app files (.js, .html) for hardcoded SCIDs.
// It looks for 64-character hex strings that match valid SCID format.
// dURL is looked up first from Gnomon's local DB, then falls back to the active TELA server list.
func ScanTELAAppSourceFiles(scid string) []string {
	if gnomon.Index == nil {
		return nil
	}

	var dURL string

	// Try Gnomon local DB for dURL
	vars := gnomon.GetAllSCIDVariableDetails(scid)
	if vars != nil {
		for _, v := range vars {
			if key, ok := v.Key.(string); ok && key == "dURL" {
				if val, ok := v.Value.(string); ok {
					dURL = decodeHex(val)
					logger.Debugf("[Engram] AutoIndex: found dURL '%s' from Gnomon for %s", dURL, scid[:16])
					break
				}
			}
		}
	}

	// Fallback: active TELA server list (server.Name is the dURL)
	if dURL == "" {
		for _, s := range tela.GetServerInfo() {
			if s.SCID == scid {
				dURL = s.Name
				logger.Debugf("[Engram] AutoIndex: found dURL '%s' from active server for %s", dURL, scid[:16])
				break
			}
		}
	}

	if dURL == "" {
		logger.Debugf("[Engram] AutoIndex: could not resolve dURL for %s, skipping source scan", scid[:16])
		return nil
	}

	clonePath := filepath.Join(AppPath(), "datashards", "tela", dURL)
	if _, err := os.Stat(clonePath); os.IsNotExist(err) {
		logger.Debugf("[Engram] AutoIndex: clone path does not exist: %s", clonePath)
		return nil
	}

	scidRe := regexp.MustCompile(`[0-9a-fA-F]{64}`)
	deps := make(map[string]bool)
	filesScanned := 0

	err := filepath.Walk(clonePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".js" && ext != ".html" {
			return nil
		}
		filesScanned++
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Debugf("[Engram] AutoIndex: could not read %s: %v", path, err)
			return nil
		}
		for _, match := range scidRe.FindAll(data, -1) {
			s := string(match)
			if s != scid {
				deps[s] = true
			}
		}
		return nil
	})

	if err != nil {
		logger.Errorf("[Engram] AutoIndex: error scanning TELA source files for %s: %v", scid[:16], err)
		return nil
	}

	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	logger.Debugf("[Engram] AutoIndex: scanned %d files in %s, found %d SCID(s)", filesScanned, dURL, len(result))
	return result
}

// AutoIndexDependentSCIDs finds any SCIDs stored as variable values or hardcoded in
// TELA app source files inside parentSCID and automatically adds them to Gnomon's index.
// This enables TELA apps like DeroBeats to fetch their dependent contracts without manual user action.
func AutoIndexDependentSCIDs(parentSCID string) {
	if gnomon.Index == nil {
		return
	}

	// Scan contract variables
	varDeps := ScanDependentSCIDs(parentSCID)

	// Scan app source files (JS/HTML)
	sourceDeps := ScanTELAAppSourceFiles(parentSCID)

	// Merge and deduplicate
	allDeps := make(map[string]bool)
	for _, dep := range varDeps {
		allDeps[dep] = true
	}
	for _, dep := range sourceDeps {
		allDeps[dep] = true
	}

	if len(allDeps) == 0 {
		logger.Printf("[Engram] AutoIndex: no dependent SCIDs found in %s", parentSCID[:16])
		return
	}

	logger.Printf("[Engram] AutoIndex: found %d dependent SCID(s) in %s (%d from vars, %d from source)",
		len(allDeps), parentSCID[:16], len(varDeps), len(sourceDeps))

	indexed := 0
	already := 0
	for dep := range allDeps {
		// Skip if already indexed
		if gnomon.GetAllSCIDVariableDetails(dep) != nil {
			already++
			continue
		}

		add := map[string]*structures.FastSyncImport{
			dep: {},
		}
		if err := gnomon.Index.AddSCIDToIndex(add, false, true); err != nil {
			logger.Errorf("[Engram] AutoIndex: failed to index %s...: %v", dep[:16], err)
		} else {
			logger.Printf("[Engram] AutoIndex: indexed dependent SCID %s...", dep[:16])
			indexed++
		}
	}
	logger.Printf("[Engram] AutoIndex: done for %s (new=%d, already=%d, total=%d)",
		parentSCID[:16], indexed, already, len(allDeps))
}

// Get the current code of a smart contract
func getContractCode(scid string) (code string, err error) {
	var params = rpc.GetSC_Params{SCID: scid, Variables: false, Code: true}
	var result rpc.GetSC_Result

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetSC", params, &result)
	if err != nil {
		logger.Errorf("[Engram] Error getting SC code: %s\n", err)
		return
	}

	code = strings.ReplaceAll(result.Code, "\x00", "")

	return
}

// DVM starter InitializePrivate() example for smart contract builder
func dvmInitFuncExample() string {
	return `Function InitializePrivate() Uint64
10 IF EXISTS("owner") == 0 THEN GOTO 30
20 RETURN 1
30 STORE("owner", SIGNER())
31 STORE("var_header_name", "")
32 STORE("var_header_description", "")
33 STORE("var_header_icon", "")
40 RETURN 0
End Function`
}

// DVM starter function for smart contract builder
func dvmFuncExample(increment int) string {
	return `Function new` + fmt.Sprintf("%d", increment) + `() Uint64
10
20
30 RETURN 0
End Function`
}

// Verification overlay for user actions with or without password
func verificationOverlay(password bool, headerText, subText, dismiss string, callback func(bool)) {
	overlay := session.Window.Canvas().Overlays()

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 5))

	if password {
		headerText = "ACCOUNT  VERIFICATION  REQUIRED"
		dismiss = "Submit"
	}

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 14
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	btnConfirm := widget.NewButton(dismiss, nil)
	btnConfirm.Disable()

	entryPassword := NewReturnEntry()
	entryPassword.Password = true
	entryPassword.PlaceHolder = "Password"
	entryPassword.OnChanged = func(s string) {
		if s == "" {
			btnConfirm.Text = dismiss
			btnConfirm.Disable()
			btnConfirm.Refresh()
		} else {
			btnConfirm.Text = dismiss
			btnConfirm.Enable()
			btnConfirm.Refresh()
		}
	}

	subHeader := widget.NewRichText()
	subHeader.Wrapping = fyne.TextWrapWord

	if password {
		subText = "Confirm Password"
	} else {
		entryPassword.Hide()
		btnConfirm.Enable()
		btnConfirm.Refresh()
	}

	subHeader.Segments = []widget.RichTextSegment{
		&widget.TextSegment{
			Text: subText,
			Style: widget.RichTextStyle{
				Alignment: fyne.TextAlignCenter,
				TextStyle: fyne.TextStyle{Bold: true},
				ColorName: theme.ColorNameForeground,
				SizeName:  theme.SizeNameHeadingText,
			},
		},
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		callback(false)
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	btnConfirm.OnTapped = func() {
		btnConfirm.Disable()
		if password {
			if engram.Disk.Check_Password(entryPassword.Text) {
				callback(true)
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
			} else {
				btnConfirm.Text = "Invalid Password..."
				btnConfirm.Refresh()
			}
		} else {
			callback(true)
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}
	}

	entryPassword.OnReturn = btnConfirm.OnTapped

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	overlay.Add(
		container.NewStack(
			&iframe{},
			canvas.NewRectangle(colors.DarkMatter),
		),
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(header),
		rectSpacer,
		rectSpacer,
	)

	center := container.NewCenter(
		container.NewVBox(
			subHeader,
			widget.NewLabel(""),
			container.NewCenter(
				container.NewStack(
					span,
					entryPassword,
				),
			),
			rectSpacer,
			rectSpacer,
			wrapMobileButton(btnConfirm),
		),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	overlay.Add(
		container.NewStack(
			&iframe{},
			container.NewBorder(
				top,
				bottom,
				nil,
				nil,
				center,
			),
		),
	)

	if password {
		safeCanvasFocus(entryPassword)
	}
}

// Color for the TELA likes ratio and individual rating numbers
func telaRatingColor(r uint64) color.Color {
	if r > 65 {
		return colors.Green
	} else if r > 32 {
		return colors.Yellow
	} else {
		return colors.Red
	}
}

// Color for the TELA average rating number hexagon
func telaHexagonColor(ratings tela.Rating_Result) fyne.Resource {
	// If there are more dislikes than likes, it's a poor app - show Red
	if ratings.Dislikes > ratings.Likes {
		return resourceTelaHexagonRed
	}

	r := ratings.Average
	if r <= 0 {
		return resourceTelaHexagonGray
	}

	if r >= 9.0 {
		return resourceTelaHexagonPurple
	}
	if r > 6.5 {
		return resourceTelaHexagonGreen
	} else if r > 3.2 {
		return resourceTelaHexagonYellow
	} else {
		return resourceTelaHexagonRed
	}
}

// Display ratings overview and the details of each rating for the TELA SCID
func viewTELARatingsOverlay(name, scid string) (err error) {
	ratings, err := tela.GetRating(scid, session.Daemon, 0)
	if err != nil {
		logger.Errorf("[Engram] GetRating: %s\n", err)
		err = fmt.Errorf("error could not get ratings")
		return
	}

	uiDo(func() {
		rectWidth90 := canvas.NewRectangle(color.Transparent)
		rectWidth90.SetMinSize(fyne.NewSize(ui.Width, 10))

		rectSpacer := canvas.NewRectangle(color.Transparent)
		rectSpacer.SetMinSize(compactSpacerSize())

		isMobileLayout := ui.Width <= 360
		if isMobileLayout {
			rectSpacer.SetMinSize(fyne.NewSize(scaleSize(6), scaleSize(2)))
		}

		header := canvas.NewText(i18n.T("tela.ratings_header"), colors.Gray)
		header.TextSize = scaleFont(16)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		if len(name) > 30 {
			name = fmt.Sprintf("%s...", name[0:30])
		}

		nameHdr := canvas.NewText(name, colors.Account)
		nameHdr.Alignment = fyne.TextAlignCenter
		nameHdr.TextStyle = fyne.TextStyle{Bold: true}

		labelSCID := canvas.NewText(i18n.T("assets.scid"), colors.Gray)
		labelSCID.TextSize = 14
		labelSCID.Alignment = fyne.TextAlignLeading
		labelSCID.TextStyle = fyne.TextStyle{Bold: true}

		textSCID := widget.NewRichTextWithText(scid)
		textSCID.Wrapping = fyne.TextWrapWord

		textLikes := widget.NewRichTextFromMarkdown(i18n.T("tela.likes"))
		textDislikes := widget.NewRichTextFromMarkdown(i18n.T("tela.dislikes"))
		textAverage := widget.NewRichTextFromMarkdown(i18n.T("tela.average"))

		ratingsBox := container.NewVBox(labelSCID, textSCID)

		overlays := session.Window.Canvas().Overlays()
		for _, o := range overlays.List() {
			overlays.Remove(o)
		}
		if res.loading != nil {
			res.loading.Hide()
			res.loading.Stop()
			res.loading = nil
		}

		overlay := session.Window.Canvas().Overlays()

		ratingsBox.Add(container.NewHBox(textLikes, canvas.NewText(fmt.Sprintf("%d", ratings.Likes), colors.Green)))
		ratingsBox.Add(container.NewHBox(textDislikes, canvas.NewText(fmt.Sprintf("%d", ratings.Dislikes), colors.Red)))
		ratingsBox.Add(container.NewHBox(textAverage, canvas.NewText(fmt.Sprintf("%0.1f/10", ratings.Average), colors.Account)))

		linkRate := widget.NewHyperlinkWithStyle(i18n.T("tela.rate_scid"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		linkRate.OnTapped = func() {
			rateTELAOverlay(name, scid)
		}
		linkRate.Hide()

		// Check if wallet has rated SCID
		if gnomon.Index != nil {
			ratingStore, _ := gnomon.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String())
			if ratingStore == nil {
				linkRate.Show()
			}
		}

		linkBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		})

		labelSeparator := widget.NewRichTextFromMarkdown("")
		labelSeparator.Wrapping = fyne.TextWrapOff
		labelSeparator.ParseMarkdown("---")
		labelSeparator2 := widget.NewRichTextFromMarkdown("")
		labelSeparator2.Wrapping = fyne.TextWrapOff
		labelSeparator2.ParseMarkdown("---")

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, 10))

		userRatingsBox := container.NewVBox()

		for _, r := range ratings.Ratings {
			ratingString, errRating := tela.Ratings.ParseString(r.Rating)
			if errRating != nil {
				ratingString = fmt.Sprintf("%d", r.Rating)
			}

			labelSeparator := widget.NewRichTextFromMarkdown("")
			labelSeparator.Wrapping = fyne.TextWrapOff
			labelSeparator.ParseMarkdown("---")

			labelAddress := widget.NewRichTextFromMarkdown(r.Address)
			labelAddress.Wrapping = fyne.TextWrapWord

			userRatingsBox.Add(
				container.NewVBox(
					labelAddress,
					container.NewHBox(widget.NewRichTextFromMarkdown(i18n.T("tela.height_label")), canvas.NewText(fmt.Sprintf("%d", r.Height), colors.Account)),
					container.NewHBox(widget.NewRichTextFromMarkdown(i18n.T("tela.rating_label")), canvas.NewText(fmt.Sprintf("%d", r.Rating), telaRatingColor(r.Rating))),
					widget.NewRichTextFromMarkdown(ratingString),
					labelSeparator,
				),
			)
		}

		userRatingsBoxScroll := container.NewVScroll(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewVBox(
					ratingsBox,
					rectWidth90,
					container.NewHBox(
						linkRate,
						layout.NewSpacer(),
					),
					rectSpacer,
					rectSpacer,
					labelSeparator2,
					rectSpacer,
					rectSpacer,
					userRatingsBox,
				),
				layout.NewSpacer(),
			),
		)
		userRatingsBoxScroll.SetMinSize(fyne.NewSize(ui.Width*0.80, ui.Height*0.50))

		// Build top section per TELA BROWSER layout
		top := container.NewVBox(
			rectSpacer,
			rectSpacer,
			container.NewCenter(header),
			rectSpacer,
			rectSpacer,
		)

		// Build bottom section per TELA BROWSER layout
		bottom := container.NewStack(
			container.NewVBox(
				rectSpacer,
				container.NewCenter(
					container.New(layout.NewGridLayoutWithColumns(1),
						linkBack,
					),
				),
				rectSpacer,
			),
		)

		// Build center content using Border to allow scroll view to expand fully
		topElements := container.NewVBox(
			container.NewCenter(nameHdr),
			rectSpacer,
			rectSpacer,
			labelSeparator,
			rectSpacer,
		)

		overlayCont := container.NewBorder(
			topElements,
			nil,
			nil,
			nil,
			userRatingsBoxScroll,
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewBorder(
					top,
					bottom,
					nil,
					nil,
					overlayCont,
				),
			),
		)
	})

	return
}

// TELA smart contract rating overlay with password confirmation
func rateTELAOverlay(name, scid string) {
	overlay := session.Window.Canvas().Overlays()

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	isMobileLayout := ui.Width <= 360
	if isMobileLayout {
		rectSpacer.SetMinSize(fyne.NewSize(scaleSize(6), scaleSize(2)))
	}

	header := canvas.NewText(i18n.T("tela.rate_app_header"), colors.Gray)
	header.TextSize = scaleFont(16)
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(scalePoint(320, 1))

	if len(name) > 30 {
		name = fmt.Sprintf("%s...", name[0:30])
	}

	nameHdr := canvas.NewText(name, colors.Account)
	nameHdr.Alignment = fyne.TextAlignCenter
	nameHdr.TextStyle = fyne.TextStyle{Bold: true}

	btnConfirm := widget.NewButton(i18n.T("tela.rate"), nil)
	btnConfirm.Disable()

	ratingSlider := widget.NewSlider(0, 9)
	ratingSlider.Step = 1
	ratingSlider.SetValue(5)

	ratingHeader := canvas.NewText(i18n.T("tela.rating_category"), colors.Account)
	ratingHeader.TextSize = scaleFont(18)
	ratingHeader.Alignment = fyne.TextAlignCenter
	ratingHeader.TextStyle = fyne.TextStyle{Bold: true}

	ratingDesc := canvas.NewText("5 - Good", colors.Gray)
	ratingDesc.TextSize = scaleFont(14)
	ratingDesc.Alignment = fyne.TextAlignCenter

	detailSlider := widget.NewSlider(0, 9)
	detailSlider.Step = 1
	detailSlider.SetValue(5)

	detailHeader := canvas.NewText(i18n.T("tela.rating_detail"), colors.Account)
	detailHeader.TextSize = scaleFont(18)
	detailHeader.Alignment = fyne.TextAlignCenter
	detailHeader.TextStyle = fyne.TextStyle{Bold: true}

	detailDesc := canvas.NewText("5 - Solid", colors.Gray)
	detailDesc.TextSize = scaleFont(14)
	detailDesc.Alignment = fyne.TextAlignCenter

	labelRatingAverage := canvas.NewText("5.5", colors.Account)
	labelRatingAverage.TextSize = scaleFont(24)
	labelRatingAverage.Alignment = fyne.TextAlignCenter
	labelRatingAverage.TextStyle = fyne.TextStyle{Bold: true}

	hexagonImg := canvas.NewImageFromResource(telaHexagonColor(tela.Rating_Result{Average: 5.5}))
	hexagonImg.SetMinSize(fyne.NewSize(80, 86))

	hexagonContainer := container.NewStack(
		hexagonImg,
		container.NewCenter(
			labelRatingAverage,
		),
	)

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	btnConfirm.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		if gnomon.Index != nil {
			var ratingStore []string
			switch gnomon.Index.DBType {
			case "gravdb":
				ratingStore, _ = gnomon.Index.GravDBBackend.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String(), gnomon.Index.LastIndexedHeight, false)
			case "boltdb":
				ratingStore, _ = gnomon.Index.BBSBackend.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String(), gnomon.Index.LastIndexedHeight, false)
			}
			if ratingStore != nil {
				errorText.Text = i18n.T("tela.already_rated")
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		category := int(ratingSlider.Value)
		if category < 0 {
			errorText.Text = i18n.T("tela.select_rating")
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		detail := int(detailSlider.Value)
		rating := (category * 10) + detail

		verificationOverlay(false, i18n.T("tela.confirm_rating_header"), i18n.T("tela.confirm_rating_body"), i18n.T("common.yes"), func(confirm bool) {
			if confirm {
				btnConfirm.Disable()
				txid, err := tela.Rate(engram.Disk, scid, uint64(rating))
				if err != nil {
					logger.Errorf("[Engram] Rate TX: %s\n", err)
					errorText.Text = i18n.T("tela.error_submitting_rating")
					errorText.Color = colors.Red
					errorText.Refresh()
					btnConfirm.Enable()
					return
				}

				logger.Printf("[Engram] Rate TXID: %s\n", txid)
				errorText.Text = i18n.T("tela.rating_submitted")
				errorText.Color = colors.Green
				errorText.Refresh()
			}
		})
	}

	// Build top section per TELA BROWSER layout
	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(header),
		rectSpacer,
		rectSpacer,
	)

	// Build bottom section per TELA BROWSER layout
	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	// Build center section with full-width sliders
	center := container.NewVBox(
		container.NewCenter(nameHdr),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		ratingHeader,
		ratingSlider,
		ratingDesc,
		rectSpacer,
		rectSpacer,
		detailHeader,
		detailSlider,
		detailDesc,
		rectSpacer,
		rectSpacer,
		container.NewCenter(hexagonContainer),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		errorText,
		rectSpacer,
		container.NewCenter(
			container.NewStack(
				rectWidth90,
				wrapMobileButton(btnConfirm),
			),
		),
	)

	ratingSlider.OnChanged = func(value float64) {
		ratingVal := int(value)
		ratingDesc.Text = fmt.Sprintf("%d - %s", ratingVal, tela.Ratings.Category(uint64(ratingVal)))
		ratingDesc.Refresh()
		ratingFloat := float64((ratingVal*10)+int(detailSlider.Value)) / 10.0
		labelRatingAverage.Text = fmt.Sprintf("%.1f", ratingFloat)
		labelRatingAverage.Refresh()
		hexagonImg.Resource = telaHexagonColor(tela.Rating_Result{Average: ratingFloat})
		hexagonImg.Refresh()
		btnConfirm.Enable()
	}

	detailSlider.OnChanged = func(value float64) {
		ratingVal := int(ratingSlider.Value)
		detailVal := int(value)
		detailDesc.Text = fmt.Sprintf("%d - %s", detailVal, tela.Ratings.Detail(uint64(detailVal), ratingVal > 4))
		detailDesc.Refresh()
		ratingFloat := float64((ratingVal*10)+detailVal) / 10.0
		labelRatingAverage.Text = fmt.Sprintf("%.1f", ratingFloat)
		labelRatingAverage.Refresh()
		hexagonImg.Resource = telaHexagonColor(tela.Rating_Result{Average: ratingFloat})
		hexagonImg.Refresh()
		btnConfirm.Enable()
	}

	overlay.Add(
		container.NewStack(
			&iframe{},
			canvas.NewRectangle(colors.DarkMatter),
		),
	)

	overlay.Add(
		container.NewStack(
			&iframe{},
			container.NewBorder(top, bottom, nil, nil, center),
		),
	)
}

// Install a new smart contract
func installSC(code string, args []rpc.Argument) (txid string, err error) {
	var dest string
	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	case NETWORK_TESTNET:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	transfer := rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	}

	_, err = transfer.Payload_RPC.CheckPack(transaction.PAYLOAD0_LIMIT)
	if err != nil {
		logger.Errorf("[Engram] Install arguments packing err: %s\n", err)
		err = fmt.Errorf("contract install pack error")
		return
	}

	// decode SC from base64 if possible
	if sc, err := base64.StdEncoding.DecodeString(code); err == nil {
		code = string(sc)
	}

	args = append(args, rpc.Argument{Name: rpc.SCACTION, DataType: rpc.DataUint64, Value: uint64(rpc.SC_INSTALL)})
	args = append(args, rpc.Argument{Name: rpc.SCCODE, DataType: rpc.DataString, Value: code})

	fees := uint64(0)
	gasParams := rpc.GasEstimate_Params{
		Transfers: []rpc.Transfer{transfer},
		SC_Code:   code,
		SC_Value:  0,
		SC_RPC:    args,
		Ringsize:  2,
		Signer:    engram.Disk.GetAddress().String(),
	}

	if gas, err := getGasEstimate(gasParams); err == nil {
		fees = gas
		logger.Printf("[Engram] SC install fees: %d\n", fees)
	} else {
		// uses default fees
		logger.Errorf("[Engram] Error estimating fees: %s\n", err)
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 2, false, args, fees, false)
	if err != nil {
		logger.Errorf("[Engram] Error while building install transaction: %s\n", err)
		err = fmt.Errorf("contract install build error")
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Engram] Error while dispatching install transaction: %s\n", err)
		err = fmt.Errorf("contract install dispatch error")
		return
	}

	txid = tx.GetHash().String()

	logger.Printf("[Engram] SC Installed: %s\n", txid)

	return
}

// Set the Remote Access password
func newRPCPassword() (s string) {
	r := make([]byte, 20)
	_, err := rand.Read(r)
	if err != nil {
		panic(err)
	}

	s = base64.URLEncoding.EncodeToString(r)
	remoteAccess.RPC.pass = s
	return
}

// Set the Remote Access username
func newRPCUsername() (s string) {
	r, _ := rand.Int(rand.Reader, big.NewInt(1600))
	w := mnemonics.Key_To_Words(r, "english")
	l := strings.Split(string(w), " ")
	s = l[len(l)-2]
	remoteAccess.RPC.user = s
	return
}

// Start an RPC server to allow decentralized application communication
func toggleRPCServer(port string) {
	var err error
	if engram.Disk == nil {
		return
	}

	if remoteAccess.RPC.server != nil {
		remoteAccess.RPC.server.RPCServer_Stop()
		remoteAccess.RPC.server = nil
		remoteAccess.RPC.status.Text = "Blocked"
		remoteAccess.RPC.status.Color = colors.Gray
		remoteAccess.RPC.status.Refresh()
		remoteAccess.RPC.toggle.Text = "Turn On"
		remoteAccess.RPC.toggle.Refresh()
		status.RemoteAccess.FillColor = colors.Gray
		status.RemoteAccess.StrokeColor = colors.Gray
		status.RemoteAccess.Refresh()
		remoteAccess.RPC.userText.Text = remoteAccess.RPC.user
		remoteAccess.RPC.passText.Text = remoteAccess.RPC.pass
		remoteAccess.RPC.userText.Enable()
		remoteAccess.RPC.passText.Enable()
		logger.Printf("[Engram] RPC server closed\n")
	} else {
		logger.Printf("[Engram] Starting RPC server %s\n", port)

		globals.Arguments["--rpc-bind"] = port

		if remoteAccess.RPC.user == "" {
			remoteAccess.RPC.user = newRPCUsername()
		}

		if remoteAccess.RPC.pass == "" {
			remoteAccess.RPC.pass = newRPCPassword()
		}

		globals.Arguments["--rpc-login"] = remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass

		remoteAccess.RPC.server, err = rpcserver.RPCServer_Start(engram.Disk, "RemoteAccess")
		if err != nil {
			remoteAccess.RPC.server = nil
			remoteAccess.RPC.status.Text = "Blocked"
			remoteAccess.RPC.status.Color = colors.Gray
			remoteAccess.RPC.status.Refresh()
			remoteAccess.RPC.toggle.Text = "Turn On"
			remoteAccess.RPC.toggle.Refresh()
			status.RemoteAccess.FillColor = colors.Gray
			status.RemoteAccess.StrokeColor = colors.Gray
			status.RemoteAccess.Refresh()
			remoteAccess.RPC.userText.Text = remoteAccess.RPC.user
			remoteAccess.RPC.passText.Text = remoteAccess.RPC.pass
			remoteAccess.RPC.userText.Enable()
			remoteAccess.RPC.passText.Enable()
		} else {
			remoteAccess.RPC.status.Text = "Allowed"
			remoteAccess.RPC.status.Color = colors.Green
			remoteAccess.RPC.status.Refresh()
			remoteAccess.RPC.toggle.Text = "Turn Off"
			remoteAccess.RPC.toggle.Refresh()
			status.RemoteAccess.FillColor = colors.Green
			status.RemoteAccess.StrokeColor = colors.Green
			status.RemoteAccess.Refresh()
			remoteAccess.RPC.userText.Text = remoteAccess.RPC.user
			remoteAccess.RPC.passText.Text = remoteAccess.RPC.pass
			remoteAccess.RPC.userText.Disable()
			remoteAccess.RPC.passText.Disable()
		}
	}
}

// Get the latest smart contract header data (must follow the standard here: https://github.com/civilware/artificer-nfa-standard/blob/main/Headers/README.md)
func getContractHeader(scid crypto.Hash) (name string, desc string, icon string, owner string, code string) {
	var headerData []*structures.SCIDVariable
	var found bool

	switch gnomon.Index.DBType {
	case "gravdb":
		headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(scid.String())
	case "boltdb":
		headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(scid.String())
	}
	if headerData == nil {
		addIndex := make(map[string]*structures.FastSyncImport)
		addIndex[scid.String()] = &structures.FastSyncImport{}
		if err := gnomon.Index.AddSCIDToIndex(addIndex, false, true); err != nil {
			logger.Debugf("[Gnomon] Could not add %s to index: %s\n", scid.String(), err)
		}
		switch gnomon.Index.DBType {
		case "gravdb":
			headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(scid.String())
		case "boltdb":
			headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(scid.String())
		}
	}

	for _, h := range headerData {
		switch key := h.Key.(type) {
		case string:
			if key == "var_header_name" {
				found = true
				name = h.Value.(string)
			} else if name == "" && key == "nameHdr" {
				found = true
				name = h.Value.(string)
			}

			if key == "var_header_description" {
				found = true
				desc = h.Value.(string)
			} else if desc == "" && key == "descrHdr" {
				found = true
				desc = h.Value.(string)
			}

			if key == "var_header_icon" {
				found = true
				icon = h.Value.(string)
			} else if icon == "" && key == "iconURLHdr" {
				found = true
				icon = h.Value.(string)
			}

			if key == "owner" {
				owner = h.Value.(string)
			}

			if key == "C" {
				code = strings.ReplaceAll(h.Value.(string), "\x00", "")
			}
		}
	}

	// Secondary check for headers in Gnomon SC
	if !found {
		switch gnomon.Index.DBType {
		case "gravdb":
			headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
		case "boltdb":
			headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
		}
		if headerData == nil {
			addIndex := make(map[string]*structures.FastSyncImport)
			addIndex[structures.MAINNET_GNOMON_SCID] = &structures.FastSyncImport{}
			if err := gnomon.Index.AddSCIDToIndex(addIndex, false, true); err != nil {
				logger.Debugf("[Gnomon] Could not add mainnet gnomon SCID to index: %s\n", err)
			}
			switch gnomon.Index.DBType {
			case "gravdb":
				headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
			case "boltdb":
				headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
			}
		}

		for _, h := range headerData {
			if strings.Contains(h.Key.(string), scid.String()) {
				switch key := h.Key.(type) {
				case string:
					if key == scid.String() {
						query := h.Value.(string)
						header := strings.Split(query, ";")

						if len(header) > 2 {
							name = header[0]
							desc = header[1]
							icon = header[2]
						}
					}

					if key == scid.String()+"owner" {
						owner = h.Value.(string)
					}
				}
			}
		}
	}

	return
}

// Send an asset from one account to another
func transferAsset(scid crypto.Hash, ringsize uint64, address string, amount string) (txid crypto.Hash, err error) {
	var amount_to_transfer uint64

	if amount == "" {
		amount = ".00001"
	}

	amount_to_transfer, err = globals.ParseAmount(amount)
	if err != nil {
		logger.Errorf("[Transfer] Failed parsing transfer amount: %s\n", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{{SCID: scid, Amount: amount_to_transfer, Destination: address}}, ringsize, false, rpc.Arguments{}, 0, false)
	if err != nil {
		logger.Errorf("[Transfer] Failed to build transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Transfer] Failed to send asset: %s - %s\n", scid, err)
		return
	}

	txid = tx.GetHash()

	logger.Printf("[Transfer] Successfully sent asset: %s - TXID: %s\n", scid, tx.GetHash().String())
	return
}

// Transfer a username to another account
func transferUsername(username string, address string) (storage uint64, err error) {
	var args = rpc.Arguments{}
	var dest string

	scid := crypto.HashHexToHash("0000000000000000000000000000000000000000000000000000000000000001")

	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: "TransferOwnership"})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})
	args = append(args, rpc.Argument{Name: "name", DataType: "S", Value: username})
	args = append(args, rpc.Argument{Name: "newowner", DataType: "S", Value: address})

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	transfer := rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	}

	gasParams := rpc.GasEstimate_Params{
		SC_RPC:    args,
		SC_Value:  0,
		Ringsize:  2,
		Signer:    engram.Disk.GetAddress().String(),
		Transfers: []rpc.Transfer{transfer},
	}

	storage, err = getGasEstimate(gasParams)
	if err != nil {
		logger.Errorf("[%s] Error estimating fees: %s\n", "TransferOwnership", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 2, false, args, storage, false)
	if err != nil {
		logger.Errorf("[%s] Error while building transaction: %s\n", "TransferOwnership", err)
		return
	}

	txid := tx.GetHash().String()

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[%s] Error while dispatching transaction: %s\n", "TransferOwnership", err)
		return
	}

	walletapi.WaitNewHeightBlock()
	logger.Printf("[%s] Username transfer successful - TXID:  %s\n", "TransferOwnership", txid)
	_ = tx

	return
}

// Execute arbitrary exportable smart contract functions
func executeContractFunction(scid crypto.Hash, ringsize uint64, dero_amount uint64, asset_amount uint64, funcName string, params []dvm.Variable) (storage uint64, err error) {
	var args = rpc.Arguments{}
	var zero uint64
	var dest string

	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: funcName})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})

	for p := range params {
		if params[p].Type == 0x4 {
			args = append(args, rpc.Argument{Name: params[p].Name, DataType: "U", Value: params[p].ValueUint64})
		} else {
			args = append(args, rpc.Argument{Name: params[p].Name, DataType: "S", Value: params[p].ValueString})
		}
	}

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	var transfers []rpc.Transfer

	if dero_amount != zero {
		burn := dero_amount

		transfer := rpc.Transfer{
			Destination: dest,
			Amount:      0,
			Burn:        burn,
		}

		transfers = append(transfers, transfer)
	}
	if asset_amount != zero {
		burn := asset_amount

		transfer := rpc.Transfer{
			SCID:        scid,
			Destination: dest,
			Amount:      0,
			Burn:        burn,
		}

		transfers = append(transfers, transfer)
	}

	if len(transfers) < 1 {
		transfer := rpc.Transfer{
			Destination: dest,
			Amount:      0,
			Burn:        0,
		}

		transfers = append(transfers, transfer)
	}

	gasParams := rpc.GasEstimate_Params{
		SC_RPC:    args,
		SC_Value:  0,
		Ringsize:  ringsize,
		Signer:    engram.Disk.GetAddress().String(),
		Transfers: transfers,
	}

	storage, err = getGasEstimate(gasParams)
	if err != nil {
		logger.Errorf("[%s] Error estimating fees: %s\n", funcName, err)
		return
	}

	tx, err := engram.Disk.TransferPayload0(transfers, ringsize, false, args, storage, false)
	if err != nil {
		logger.Errorf("[%s] Error while building transaction: %s\n", funcName, err)
		return
	}

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[%s] Error while dispatching transaction: %s\n", funcName, err)
		return
	}

	walletapi.WaitNewHeightBlock()
	logger.Printf("[%s] Function execution successful - TXID:  %s\n", funcName, tx.GetHash().String())
	_ = tx

	return
}

// Delete the Gnomon directory
func cleanGnomonData() error {
	path := filepath.Join(AppPath(), "datashards", "gnomon")
	switch session.Network {
	case NETWORK_TESTNET:
		path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
	case NETWORK_SIMULATOR:
		path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
	}

	dir, err := os.ReadDir(path)
	if err != nil {
		logger.Errorf("[Gnomon] Error purging local Gnomon data: %s\n", err)
		return err
	}

	for _, d := range dir {
		os.RemoveAll(filepath.Join([]string{path, d.Name()}...))
		logger.Printf("[Gnomon] Local Gnomon data has been purged successfully\n")
	}

	return nil
}

// Delete the datashard directory for the active wallet
func cleanWalletData() (err error) {
	path, err := GetShard()
	if err != nil {
		return
	}

	// Check if path exists - nothing to clean is still success
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Printf("[Engram] Datashard path doesn't exist, nothing to clean\n")
		// Still clear TELA cache including negative cache
		clearAllTELACache()
		return nil
	}

	dir, err := os.ReadDir(path)
	if err != nil {
		logger.Errorf("[Engram] Error reading datashard directory: %s\n", err)
		return err
	}

	for _, d := range dir {
		if err := os.RemoveAll(filepath.Join([]string{path, d.Name()}...)); err != nil {
			logger.Errorf("[Engram] Error removing %s: %s\n", d.Name(), err)
		}
	}

	// Clear TELA cache including negative cache to allow proper re-scan
	clearAllTELACache()

	logger.Printf("[Engram] Local datashard data has been purged successfully\n")
	return nil
}

// Get transaction data for any TXID from the daemon
func getTxData(txid string) (result rpc.GetTransaction_Result, err error) {
	if engram.Disk == nil || session.Offline {
		return
	}

	var params rpc.GetTransaction_Params

	params.Tx_Hashes = append(params.Tx_Hashes, txid)

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	if err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetTransaction", params, &result); err != nil {
		logger.Errorf("[Engram] getTxData TXID: %s (Failed: %s)\n", txid, err)
		return
	}

	rpc_client.WS.Close()
	rpc_client.RPC.Close()

	if result.Status != "OK" {
		logger.Errorf("[Engram] getTxData TXID: %s (Failed: %s)\n", txid, result.Status)
		return
	}

	if len(result.Txs_as_hex[0]) < 50 {
		return
	}

	return
}

// Methods Engram will use as XSWD noStore
func engramNoStoreMethods() []string {
	return []string{
		"Subscribe",
		"SignData",
		"CheckSignature",
		"GetDaemon",
		"GetPrimaryUsername",
		"query_key",
		"QueryKey",
		"HandleTELALinks"}
}

// Check if method is in Engram's noStore list
func engramCanStoreMethod(method string) bool {
	noStoreMethods := engramNoStoreMethods()
	for _, m := range noStoreMethods {
		if m == method {
			return false
		}
	}

	return true
}

// Set XSWD permissions to the local Graviton tree
func setPermissions() {
	// Save permissions with dual storage
	data, err := json.Marshal(remoteAccess.WS.global.permissions)
	if err != nil {
		logger.Errorf("[Engram] setPermissions: %s\n", err)
		return
	}

	// Validate that we have valid JSON before storing
	var test map[string]xswd.Permission
	if err := json.Unmarshal(data, &test); err != nil {
		logger.Errorf("[Engram] setPermissions: Generated invalid JSON: %v", err)
		// Continue anyway - the data was successfully marshaled above
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err = StoreEncryptedValue("XSWD", []byte("Globals"), data)
		if err != nil {
			logger.Debugf("[Engram] setPermissions (encrypted): %s\n", err)
		} else {
			logger.Printf("[Engram] Permissions saved to encrypted storage: %d bytes", len(data))
		}
	}

	// Always save to unencrypted storage as fallback with synchronization
	remoteAccess.WS.Lock()
	err = StoreValue("XSWDUnencrypted", []byte("Globals"), data)
	if err != nil {
		logger.Debugf("[Engram] setPermissions (fallback): %s\n", err)
	} else {
		logger.Printf("[Engram] Permissions saved to fallback storage: %d bytes", len(data))
	}
	remoteAccess.WS.Unlock()

	// Force immediate save to ensure data is written
	logger.Printf("[Engram] setPermissions completed - permissions saved to dual storage")

	// Save enabled state separately with dual storage
	enabledValue := "0"
	if remoteAccess.WS.global.enabled {
		enabledValue = "1"
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err = StoreEncryptedValue("XSWD", []byte("Enabled"), []byte(enabledValue))
		if err != nil {
			logger.Debugf("[Engram] setPermissions enabled (encrypted): %s\n", err)
		} else {
			logger.Printf("[Engram] WebSocket enabled state saved to encrypted storage: %s\n", enabledValue)
		}
	}

	// Always save to unencrypted storage as fallback
	err = StoreValue("XSWDUnencrypted", []byte("Enabled"), []byte(enabledValue))
	if err != nil {
		logger.Debugf("[Engram] setPermissions enabled (fallback): %s\n", err)
	} else {
		logger.Printf("[Engram] WebSocket enabled state saved to fallback storage: %s\n", enabledValue)
	}
}

// getWalletXSWDID returns a stable identifier for the current wallet (short hash of address)
func getWalletXSWDID() string {
	if engram.Disk == nil {
		return ""
	}
	addr := engram.Disk.GetAddress().String()
	if addr == "" {
		return ""
	}
	h := sha1.Sum([]byte(addr))
	return fmt.Sprintf("%x", h[:8])
}

// hasAskedXSWD checks if the first-time XSWD prompt has already been shown for this wallet
func hasAskedXSWD() bool {
	wid := getWalletXSWDID()
	logger.Printf("[XSWD-PROMPT] hasAskedXSWD: wid=%q (empty=%v)\n", wid, wid == "")
	if wid == "" {
		logger.Printf("[XSWD-PROMPT] hasAskedXSWD: returning false (no wallet ID, show prompt)\n")
		return false
	}
	_, err := GetValue("XSWDUnencrypted", []byte("Asked_"+wid))
	result := err == nil
	logger.Printf("[XSWD-PROMPT] hasAskedXSWD: key exists=%v (err=%v)\n", result, err)
	return result
}

// setAskedXSWD records that the first-time XSWD prompt has been shown for this wallet
func setAskedXSWD() {
	wid := getWalletXSWDID()
	if wid == "" {
		return
	}
	err := StoreValue("XSWDUnencrypted", []byte("Asked_"+wid), []byte("1"))
	if err != nil {
		logger.Errorf("[Engram] setAskedXSWD: failed to save for wallet %s: %s\n", wid, err)
	}
}

// showXSWDPrompt shows a first-time popup asking whether to allow WebSocket connections for TELA apps.
// Returns true if the user allows, false if they deny.
func showXSWDPrompt() bool {
	if session.Window == nil {
		return false
	}

	allowed := false
	done := make(chan struct{})

	header := canvas.NewText(i18n.T("tela.app_connections_prompt"), colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	message := widget.NewLabel(i18n.T("tela.app_connections_body"))
	message.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	btnAllow := widget.NewButton(i18n.T("settings.permissions.allow"), func() {
		allowed = true
		done <- struct{}{}
	})
	btnAllow.Importance = widget.MediumImportance

	btnDeny := widget.NewButton(i18n.T("settings.permissions.deny"), func() {
		allowed = false
		done <- struct{}{}
	})
	btnDeny.Importance = widget.MediumImportance

	btnRow := container.NewHBox(layout.NewSpacer(), btnAllow, rectSpacer, btnDeny, layout.NewSpacer())

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				btnRow,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						message,
						rectSpacer,
					),
				),
			),
		),
	)

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	uiDo(func() {
		overlay := session.Window.Canvas().Overlays()
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)
		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						container.NewCenter(
							container.NewStack(
								span,
							),
						),
						rectSpacer,
						rectSpacer,
						rectSpacer,
						content,
						btnRow,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	})

	<-done

	uiDo(func() {
		overlay := session.Window.Canvas().Overlays()
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	setAskedXSWD()
	return allowed
}

// GetPermissionGroup returns the group a method belongs to
func GetPermissionGroup(method string) *PermissionGroup {
	for i := range permissionGroups {
		for _, m := range permissionGroups[i].Methods {
			if m == method {
				return &permissionGroups[i]
			}
		}
	}
	return nil
}

// getSimpleDefault returns the default permission for a category in Simple Mode
func getSimpleDefault(category string) xswd.Permission {
	switch category {
	case "utility":
		return xswd.AlwaysAllow
	case "mining":
		return xswd.AlwaysAllow
	case "readonly":
		return xswd.Ask
	default:
		return xswd.Ask
	}
}

// IsSimpleMode checks if user is in simple mode
func IsSimpleMode() bool {
	// Skip encrypted storage if wallet not available yet
	if engram.Disk != nil {
		data, err := GetEncryptedValue("XSWD", []byte("SimpleMode"))
		if err == nil {
			return string(data) == "true"
		}
	}

	// Check unencrypted fallback
	data, err := GetValue("XSWDUnencrypted", []byte("SimpleMode"))
	if err != nil {
		return true // Default to simple mode
	}
	return string(data) == "true"
}

// SetSimpleMode toggles between simple and advanced mode
func SetSimpleMode(simple bool) {
	val := "false"
	if simple {
		val = "true"
	}

	// Try encrypted storage first
	if engram.Disk != nil {
		err := StoreEncryptedValue("XSWD", []byte("SimpleMode"), []byte(val))
		if err != nil {
			logger.Debugf("[Engram] SetSimpleMode (encrypted): %s\n", err)
		}
	}

	// Always save to unencrypted as fallback
	err := StoreValue("XSWDUnencrypted", []byte("SimpleMode"), []byte(val))
	if err != nil {
		logger.Debugf("[Engram] SetSimpleMode (fallback): %s\n", err)
	}
}

// GetGroupPermission returns the current permission for a group from storage
func GetGroupPermission(groupName string) xswd.Permission {
	// Skip encrypted storage if wallet not available yet
	if engram.Disk != nil {
		data, err := GetEncryptedValue("XSWD", []byte("Group:"+groupName))
		if err == nil {
			switch string(data) {
			case "AlwaysAllow":
				return xswd.AlwaysAllow
			case "AlwaysDeny":
				return xswd.AlwaysDeny
			default:
				return xswd.Ask
			}
		}
	}

	// Check unencrypted fallback
	data, err := GetValue("XSWDUnencrypted", []byte("Group:"+groupName))
	if err != nil {
		// Return default based on group category
		for _, group := range permissionGroups {
			if group.Name == groupName {
				return getSimpleDefault(group.Category)
			}
		}
		return xswd.Ask
	}

	switch string(data) {
	case "AlwaysAllow":
		return xswd.AlwaysAllow
	case "AlwaysDeny":
		return xswd.AlwaysDeny
	default:
		return xswd.Ask
	}
}

// SetGroupPermission sets permission for all methods in a group
func SetGroupPermission(groupName string, perm xswd.Permission) {
	permStr := perm.String()

	// Store group-level permission
	if engram.Disk != nil {
		err := StoreEncryptedValue("XSWD", []byte("Group:"+groupName), []byte(permStr))
		if err != nil {
			logger.Debugf("[Engram] SetGroupPermission (encrypted): %s\n", err)
		}
	}

	err := StoreValue("XSWDUnencrypted", []byte("Group:"+groupName), []byte(permStr))
	if err != nil {
		logger.Debugf("[Engram] SetGroupPermission (fallback): %s\n", err)
	}

	// Apply to all methods in the group
	for _, group := range permissionGroups {
		if group.Name == groupName {
			for _, method := range group.Methods {
				if remoteAccess.WS.global.permissions != nil {
					remoteAccess.WS.global.permissions[method] = perm
				}
			}
			break
		}
	}

	// Save updated permissions
	setPermissions()
	logger.Printf("[Engram] Set group '%s' permission to %s", groupName, permStr)
}

// ApplySimpleModeDefaults applies group permissions to individual methods
func ApplySimpleModeDefaults() {
	if !IsSimpleMode() {
		return
	}

	for _, group := range permissionGroups {
		groupPerm := GetGroupPermission(group.Name)
		for _, method := range group.Methods {
			if remoteAccess.WS.global.permissions != nil {
				remoteAccess.WS.global.permissions[method] = groupPerm
			}
		}
	}

	logger.Printf("[Engram] Applied Simple Mode defaults to %d permission groups", len(permissionGroups))
}

// Set all noStore methods to XSWD Ask permission
func SetDefaultPermissions() (defaults map[string]xswd.Permission) {
	defaults = make(map[string]xswd.Permission)
	for method := range rpcserver.WalletHandler {
		defaults[method] = xswd.Ask
	}

	// XSWD methods
	defaults["Subscribe"] = xswd.Ask
	defaults["HasMethod"] = xswd.Ask
	defaults["Unsubscribe"] = xswd.Ask
	defaults["GetDaemon"] = xswd.Ask
	defaults["SignData"] = xswd.Ask
	defaults["CheckSignature"] = xswd.Ask

	// Engram methods
	defaults["GetPrimaryUsername"] = xswd.Ask
	defaults["HandleTELALinks"] = xswd.Ask

	// EPOCH methods - Comprehensive registration with fallback
	logger.Printf("[Engram] EPOCH: Starting comprehensive method registration")

	// Register all known EPOCH methods statically as fallback
	epochMethods := []string{
		"AttemptEPOCH", "AttemptEPOCHWithAddr", "CheckSignature", "Echo", "GetAddress",
		"GetAddressEPOCH", "GetBalance", "GetDaemon", "GetHeight", "GetMaxHashesEPOCH",
		"GetPrimaryUsername", "GetSessionEPOCH", "GetTransferbyTXID", "GetTransfers",
		"HandleTELALinks", "HasMethod", "MakeIntegratedAddress", "QueryKey", "SignData",
		"SplitIntegratedAddress", "SubmitEPOCH", "Subscribe", "Transfer", "Unsubscribe",
		"get-transfer_by_txid", "get_transfers", "getaddress", "getbalance", "getheight",
		"make_integrated_address", "query_key", "scinvoke", "split_integrated_address",
		"transfer_split",
	}

	// First, try dynamic registration from epoch.GetHandler()
	dynamicMethods := make(map[string]bool)
	if epochHandler := epoch.GetHandler(); epochHandler != nil {
		logger.Printf("[Engram] EPOCH: epoch.GetHandler() returned handler with %d methods", len(epochHandler))
		for method := range epochHandler {
			defaults[method] = xswd.Ask
			dynamicMethods[method] = true
			logger.Printf("[Engram] EPOCH: Dynamically registered method: %s", method)
		}
	} else {
		logger.Printf("[Engram] EPOCH: epoch.GetHandler() returned nil - no dynamic methods available")
	}

	// Then, ensure all expected EPOCH methods are registered (fallback)
	registeredCount := 0
	for _, method := range epochMethods {
		if !dynamicMethods[method] {
			defaults[method] = xswd.Ask
			logger.Printf("[Engram] EPOCH: Statically registered fallback method: %s", method)
			registeredCount++
		}
	}

	logger.Printf("[Engram] EPOCH: Registration complete. %d static fallback methods registered, %d total EPOCH methods available",
		registeredCount, len(epochMethods))

	// Add specific methods that might be missing from your list
	defaults["Echo"] = xswd.Ask
	// Explicitly add methods that were missing from your list
	defaults["AttemptEPOCH"] = xswd.Ask
	defaults["CheckSignature"] = xswd.Ask
	defaults["GetAddress"] = xswd.Ask
	defaults["GetAddressEPOCH"] = xswd.Ask
	defaults["GetBalance"] = xswd.Ask
	defaults["GetDaemon"] = xswd.Ask
	defaults["GetHeight"] = xswd.Ask
	defaults["GetMaxHashesEPOCH"] = xswd.Ask
	defaults["GetPrimaryUsername"] = xswd.Ask
	defaults["GetSessionEPOCH"] = xswd.Ask
	defaults["GetTransferbyTXID"] = xswd.Ask
	defaults["GetTransfers"] = xswd.Ask
	defaults["MakeIntegratedAddress"] = xswd.Ask
	defaults["SplitIntegratedAddress"] = xswd.Ask
	defaults["SubmitEPOCH"] = xswd.Ask
	defaults["Transfer"] = xswd.Ask
	defaults["get_transfer_by_txid"] = xswd.Ask
	defaults["get_transfers"] = xswd.Ask
	defaults["getaddress"] = xswd.Ask
	defaults["getbalance"] = xswd.Ask
	defaults["getheight"] = xswd.Ask
	defaults["make_integrated_address"] = xswd.Ask
	defaults["query_key"] = xswd.Ask
	defaults["split_integrated_address"] = xswd.Ask
	defaults["transfer_split"] = xswd.Ask
	defaults["scinvoke"] = xswd.Ask

	// Debug: Print a few key methods to verify they're in the defaults
	logger.Printf("[Engram] DEBUG - Key methods in defaults: AttemptEPOCH=%v, GetAddress=%v, Transfer=%v",
		defaults["AttemptEPOCH"], defaults["GetAddress"], defaults["Transfer"])

	// Apply Simple Mode grouped defaults if enabled
	if IsSimpleMode() {
		logger.Printf("[Engram] Applying Simple Mode permission defaults")
		for _, group := range permissionGroups {
			groupPerm := GetGroupPermission(group.Name)
			// If no stored permission, use category default
			if groupPerm == xswd.Ask {
				groupPerm = getSimpleDefault(group.Category)
			}
			for _, method := range group.Methods {
				defaults[method] = groupPerm
			}
		}
		logger.Printf("[Engram] Applied Simple Mode defaults to %d permission groups", len(permissionGroups))
	}

	return
}

// Get XSWD permissions from local Graviton tree and sorted wallet methods
func getPermissions() (handler map[string]xswd.Permission, methods []string) {
	remoteAccess.WS.Lock()
	defer remoteAccess.WS.Unlock()

	logger.Printf("[Engram] getPermissions() called - wallet available: %v", engram.Disk != nil)
	remoteAccess.WS.global.permissions = SetDefaultPermissions()
	logger.Printf("[Engram] SetDefaultPermissions created %d methods", len(remoteAccess.WS.global.permissions))

	// Debug: Print all methods that should be available
	logger.Printf("[Engram] DEBUG - All methods in remoteAccess.WS.global.permissions:")
	for methodName := range remoteAccess.WS.global.permissions {
		logger.Printf("[Engram]   - %s", methodName)
	}

	// Load permissions (only if wallet is available for encrypted storage)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("XSWD", []byte("Globals"))
		if err != nil {
			logger.Printf("[Engram] getPermissions: stored permissions not found: %s (using defaults)\n", err)
		} else {
			// Load stored permissions into a temporary map
			var storedPermissions map[string]xswd.Permission
			if err := json.Unmarshal(stored, &storedPermissions); err != nil {
				logger.Errorf("[Engram] getPermissions: JSON unmarshal error: %s (corrupted data, deleting and using defaults)\n", err)
				// Delete corrupted data so it will be recreated on next save
				if delErr := DeleteKey("XSWD", []byte("Globals")); delErr != nil {
					logger.Debugf("[Engram] getPermissions: could not delete corrupted encrypted globals: %s\n", delErr)
				}
			} else {
				logger.Printf("[Engram] getPermissions: Successfully loaded %d stored permissions\n", len(storedPermissions))
				// Merge stored permissions with defaults (stored takes precedence)
				for method, permission := range storedPermissions {
					if _, exists := remoteAccess.WS.global.permissions[method]; exists {
						remoteAccess.WS.global.permissions[method] = permission
						logger.Printf("[Engram] getPermissions: Merged stored permission for %s: %s", method, permission)
					}
				}
				logger.Printf("[Engram] getPermissions: After merge, total permissions: %d", len(remoteAccess.WS.global.permissions))
			}
		}

		// Load enabled state
		storedEnabled, err := GetEncryptedValue("XSWD", []byte("Enabled"))
		if err != nil {
			logger.Printf("[Engram] WebSocket enabled state NOT FOUND (error: %v), defaulting to false", err)
			remoteAccess.WS.global.enabled = false // Default to disabled
		} else {
			enabledStr := string(storedEnabled)
			remoteAccess.WS.global.enabled = enabledStr == "1"
			logger.Printf("[Engram] WebSocket enabled state loaded: '%s' -> %v", enabledStr, remoteAccess.WS.global.enabled)
		}
	} else {
		// Try to load permissions from unencrypted fallback storage
		stored, err := GetValue("XSWDUnencrypted", []byte("Globals"))
		if err != nil {
			logger.Printf("[Engram] getPermissions: fallback permissions not found: %s (using defaults)\n", err)
		} else {
			// Validate we have data before attempting to unmarshal
			if len(stored) == 0 {
				logger.Printf("[Engram] getPermissions: fallback storage empty (using defaults)\n")
			} else {
				// Load fallback permissions into a temporary map
				var storedPermissions map[string]xswd.Permission
				if err := json.Unmarshal(stored, &storedPermissions); err != nil {
					logger.Errorf("[Engram] getPermissions: fallback JSON unmarshal error: %s (corrupted data, deleting and using defaults)\n", err)
					// Delete corrupted data so it will be recreated on next save
					if delErr := DeleteKey("XSWDUnencrypted", []byte("Globals")); delErr != nil {
						logger.Debugf("[Engram] getPermissions: could not delete corrupted fallback globals: %s\n", delErr)
					}
				} else {
					logger.Printf("[Engram] getPermissions: Successfully loaded %d fallback permissions\n", len(storedPermissions))
					// Merge fallback permissions with defaults (stored takes precedence)
					for method, permission := range storedPermissions {
						if _, exists := remoteAccess.WS.global.permissions[method]; exists {
							remoteAccess.WS.global.permissions[method] = permission
							logger.Printf("[Engram] getPermissions: Merged fallback permission for %s: %s", method, permission)
						}
					}
					logger.Printf("[Engram] getPermissions: After fallback merge, total permissions: %d", len(remoteAccess.WS.global.permissions))
				}
			}
		}

		// Try to load enabled state from unencrypted fallback storage
		storedEnabled, err := GetValue("XSWDUnencrypted", []byte("Enabled"))
		if err != nil {
			logger.Printf("[Engram] WebSocket enabled state NOT FOUND in fallback (error: %v), defaulting to false", err)
			remoteAccess.WS.global.enabled = false // Default to disabled
		} else {
			enabledStr := string(storedEnabled)
			remoteAccess.WS.global.enabled = enabledStr == "1"
			logger.Printf("[Engram] WebSocket enabled state loaded from fallback: '%s' -> %v", enabledStr, remoteAccess.WS.global.enabled)
		}
		logger.Printf("[Engram] getPermissions: Wallet not available, using fallback permissions and enabled state")
	}

	for k := range remoteAccess.WS.global.permissions {
		methods = append(methods, k)
		logger.Printf("[Engram] Found method in permissions: %s", k)
	}

	sort.Strings(methods)
	logger.Printf("[Engram] Total methods for UI: %d", len(methods))

	return remoteAccess.WS.global.permissions, methods
}

// CleanStaleXSWDConnections removes all existing XSWD application connections.
// This is called before reopening a TELA app to prevent "App ID is already used" errors
// that occur on mobile when browsers cache/suspend tabs with stale WebSocket state.
// Safe to call even if XSWD is not running or has no connections.
func CleanStaleXSWDConnections() {
	xswdStateMu.RLock()
	server := remoteAccess.WS.server
	xswdStateMu.RUnlock()

	if server == nil || !server.IsRunning() {
		return
	}

	// Get all connected applications and remove them
	apps := server.GetApplications()
	if len(apps) == 0 {
		return
	}

	logger.Printf("[Engram] Cleaning %d stale XSWD connection(s) before TELA reopen\n", len(apps))
	for _, app := range apps {
		server.RemoveApplication(&app)
	}
}

// EnsureXSWD handles XSWD server lifecycle, ensuring it is bound to dual-stack :44326
// and is ready for dApp connections.
func EnsureXSWD() {
	if engram.Disk == nil || session.Offline {
		return
	}

	// Always use port 44326 for modern XSWD/Villager compatibility
	const targetPort = ":44326"
	if remoteAccess.WS.port != targetPort {
		logger.Printf("[Engram] EnsureXSWD: Migrating port from %s to %s for dual-stack support\n", remoteAccess.WS.port, targetPort)
		remoteAccess.WS.port = targetPort
		setRemoteAccessDual(targetPort, "WS")
	}

	// If not enabled, we may need to prompt (if calling from a goroutine)
	if !remoteAccess.WS.global.enabled {
		if !hasAskedXSWD() {
			if showXSWDPrompt() {
				remoteAccess.WS.global.enabled = true
				setPermissions()
			} else {
				return // User declined
			}
		} else {
			// Already asked before. If it's disabled, we respect that.
			return
		}
	}

	// Start server if not running
	if remoteAccess.WS.server == nil {
		logger.Printf("[Engram] EnsureXSWD: Starting server on %s\n", targetPort)
		toggleXSWD(targetPort)

		// Wait up to 2 seconds for server to be ready
		for i := 0; i < 20; i++ {
			xswdStateMu.RLock()
			server := remoteAccess.WS.server
			xswdStateMu.RUnlock()
			if server != nil && server.IsRunning() {
				logger.Printf("[Engram] EnsureXSWD: Server is ready after %dms\n", i*100)
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		// Even if server is running, ensure global state matches for UI consistency
		if !remoteAccess.WS.global.enabled {
			remoteAccess.WS.global.enabled = true
			setPermissions()
		}
	}
}

// Start a permissioned web socket server to allow decentralized application communication
func toggleXSWD(endpoint string) {
	if engram.Disk == nil {
		return
	}

	if remoteAccess.WS.server != nil {
		xswdStateMu.Lock()
		remoteAccess.WS.server.Stop()
		remoteAccess.WS.server = nil
		remoteAccess.WS.apps = []xswd.ApplicationData{}
		xswdStateMu.Unlock()
		uiDo(func() {
			if remoteAccess.WS.status != nil {
				remoteAccess.WS.status.Text = "Blocked"
				remoteAccess.WS.status.Color = colors.Gray
				remoteAccess.WS.status.Refresh()
			}
			if remoteAccess.WS.toggle != nil {
				remoteAccess.WS.toggle.Text = "Turn On"
				remoteAccess.WS.toggle.Refresh()
			}
			if status.RemoteAccess != nil {
				status.RemoteAccess.FillColor = colors.Gray
				status.RemoteAccess.StrokeColor = colors.Gray
				status.RemoteAccess.Refresh()
			}
		})
		remoteAccess.WS.advanced = false
		remoteAccess.WS.global.enabled = false
		remoteAccess.WS.global.connect = false

		// Save WebSocket disabled state to storage
		setPermissions()
		if remoteAccess.WS.list != nil {
			uiDo(func() {
				if remoteAccess.WS.list != nil {
					remoteAccess.WS.list.Refresh()
				}
			})
		}
		logger.Printf("[Engram] XSWD server closed\n")
	} else {
		_, portNum, err := net.SplitHostPort(endpoint)
		if err != nil {
			logger.Errorf("[Engram] Invalid XSWD server endpoint: %s\n", err)
			return
		}

		portInt, err := strconv.Atoi(portNum)
		if err != nil {
			logger.Errorf("[Engram] Invalid XSWD server port: %s\n", err)
			return
		}

		logger.Printf("[Engram] Starting XSWD server %s\n", endpoint)

		if remoteAccess.WS.toggle != nil {
			uiDo(func() {
				remoteAccess.WS.toggle.Disable()
				remoteAccess.WS.toggle.Text = "Initializing"
				remoteAccess.WS.toggle.Refresh()
			})
		}

		go func() {
			// Check if port is already in use and wait up to 5 seconds for release
			if addr, err := net.ResolveTCPAddr("tcp", endpoint); err == nil {
				for i := 0; i < 5; i++ {
					if listener, err := net.ListenTCP("tcp", addr); err == nil {
						listener.Close()
						logger.Printf("[Engram] Port %s is available\n", endpoint)
						break
					}
					if i < 4 {
						logger.Printf("[Engram] Port %s in use (Socket Refused likely if client tries now), retrying in 1s... (%d/5)\n", endpoint, i+1)
						time.Sleep(time.Second)
					} else {
						logger.Errorf("[Engram] Port %s still in use after 5 seconds - check for other running instances\n", endpoint)
					}
				}
			}

			noStoreMethods := engramNoStoreMethods()

			xswdStateMu.Lock()
			if remoteAccess.WS.server != nil {
				xswdStateMu.Unlock()
				return
			}
			remoteAccess.WS.server = xswd.NewXSWDServerWithPort(portInt, engram.Disk, false, noStoreMethods, func(ad *xswd.ApplicationData) bool {
				return XSWDPrompt(ad)
			}, func(ad *xswd.ApplicationData, r *jrpc2.Request) xswd.Permission {
				return AskPermissionForRequest(ad, r)
			})

			server := remoteAccess.WS.server
			xswdStateMu.Unlock()

			if server == nil {
				logger.Errorf("[Engram] Failed to create XSWD server")
				uiDo(func() {
					if remoteAccess.WS.toggle != nil {
						remoteAccess.WS.toggle.Enable()
						remoteAccess.WS.toggle.Text = "Turn On"
						remoteAccess.WS.toggle.Refresh()
					}
				})
				return
			}

			// Add handlers
			for method, h := range EngramHandler {
				server.SetCustomMethod(method, h)
			}
			server.SetCustomMethod("HandleTELALinks", handler.New(HandleTELALinks))
			server.SetCustomMethod("AttemptEPOCHWithAddr", handler.New(AttemptEPOCHWithAddr))
			for method, h := range epoch.GetHandler() {
				server.SetCustomMethod(method, h)
			}

			if server.IsRunning() {
				logger.Printf("[Engram] WebSocket server started successfully on %s", endpoint)
				uiDo(func() {
					if remoteAccess.WS.status != nil {
						remoteAccess.WS.status.Text = "Allowed"
						remoteAccess.WS.status.Color = colors.Green
						remoteAccess.WS.status.Refresh()
					}
					if remoteAccess.WS.toggle != nil {
						remoteAccess.WS.toggle.Enable()
						remoteAccess.WS.toggle.Text = "Turn Off"
						remoteAccess.WS.toggle.Refresh()
					}
					if remoteAccess.WS.portText != nil {
						remoteAccess.WS.portText.Disable()
						remoteAccess.WS.portText.Refresh()
					}
					if status.RemoteAccess != nil {
						status.RemoteAccess.FillColor = colors.Green
						status.RemoteAccess.StrokeColor = colors.Green
						status.RemoteAccess.Refresh()
					}
				})

				remoteAccess.WS.global.enabled = true
				remoteAccess.WS.advanced = true
				setPermissions()
			} else {
				logger.Errorf("[Engram] XSWD server failed to start on %s", endpoint)
				uiDo(func() {
					if remoteAccess.WS.toggle != nil {
						remoteAccess.WS.toggle.Enable()
						remoteAccess.WS.toggle.Text = "Turn On"
						remoteAccess.WS.toggle.Refresh()
					}
				})
			}
		}()
	}
}

// Prompt when an application submits request to connect to wallet with XSWD
func XSWDPrompt(ad *xswd.ApplicationData) (confirmed bool) {
	generation := currentWalletGeneration()
	if !isWalletGenerationActive(generation) || session.Window == nil {
		return false
	}

	if remoteAccess.WS.advanced {
		// If global permissions enabled set them here
		if remoteAccess.WS.global.enabled {
			logger.Printf("[Engram] Applied global XSWD permissions to %s\n", ad.Name)
			// Initialize ad.Permissions if nil to avoid panic
			if ad.Permissions == nil {
				ad.Permissions = make(map[string]xswd.Permission)
			}
			remoteAccess.WS.RLock()
			for k, v := range remoteAccess.WS.global.permissions {
				ad.Permissions[k] = v
			}
			remoteAccess.WS.RUnlock()

			// Load stored per-app permissions (they persist across reconnections and override globals)
			if storedPerms, _ := GetAppPermissions(ad.Name); storedPerms != nil {
				for k, v := range storedPerms {
					ad.Permissions[k] = v
				}
				logger.Printf("[Engram] Loaded %d stored permissions for app %s\n", len(storedPerms), ad.Name)
			}
		}

		// If wallet is set to connect to all requests, connect to app
		if remoteAccess.WS.global.connect {
			logger.Printf("[Engram] Applied automatic XSWD connection to %s\n", ad.Name)
			fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: i18n.T("xswd.connection_approved")})
			go refreshXSWDList()
			return true
		}
	} else {
		// Restrictive mode overwrites any requested permissions to default Ask, and sets certain methods to AlwaysDeny
		ad.Permissions = map[string]xswd.Permission{}
		ad.Permissions["QueryKey"] = xswd.AlwaysDeny
		ad.Permissions["query_key"] = xswd.AlwaysDeny
	}

	overlay := session.Window.Canvas().Overlays()

	headerText := i18n.T("xswd.new_connection_request")

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText(i18n.T("xswd.connection_app_name"), colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### " + ad.Name)
	textApp.Wrapping = fyne.TextWrapWord

	labelID := canvas.NewText(i18n.T("xswd.connection_app_id"), colors.Gray)
	labelID.TextSize = 14
	labelID.Alignment = fyne.TextAlignLeading
	labelID.TextStyle = fyne.TextStyle{Bold: true}

	textID := widget.NewRichTextFromMarkdown(ad.Id)
	textID.Wrapping = fyne.TextWrapWord

	labelURL := canvas.NewText(i18n.T("xswd.connection_url"), colors.Gray)
	labelURL.TextSize = 14
	labelURL.Alignment = fyne.TextAlignLeading
	labelURL.TextStyle = fyne.TextStyle{Bold: true}

	textURL := widget.NewRichTextFromMarkdown(ad.Url)
	textURL.Wrapping = fyne.TextWrapWord

	labelPermissions := canvas.NewText(i18n.T("xswd.connection_permissions"), colors.Gray)
	labelPermissions.TextSize = 14
	labelPermissions.Alignment = fyne.TextAlignLeading
	labelPermissions.TextStyle = fyne.TextStyle{Bold: true}

	// Get permissioned methods from xswd.ApplicationData and create permission objects
	var methods []string
	for k := range ad.Permissions {
		methods = append(methods, k)
	}

	sort.Strings(methods)

	permForm := container.NewVBox()

	textSpacer := canvas.NewRectangle(color.Transparent)
	textSpacer.SetMinSize(fyne.NewSize(10, 3))

	for _, k := range methods {
		perm := ad.Permissions[k]
		permColor := colors.Account
		switch perm {
		case xswd.AlwaysAllow:
			permColor = colors.Green
		case xswd.AlwaysDeny:
			permColor = colors.Red
		}

		textMethod := widget.NewRichTextFromMarkdown("### " + k)
		textMethod.Wrapping = fyne.TextWrapWord

		sep := canvas.NewRectangle(colors.Gray)
		sep.SetMinSize(fyne.NewSize(ui.Width*0.5, 2))

		permText := perm.String()
		switch perm {
		case xswd.AlwaysAllow:
			permText = i18n.T("settings.permissions.always_allow")
		case xswd.AlwaysDeny:
			permText = i18n.T("settings.permissions.always_deny")
		case xswd.Allow:
			permText = i18n.T("settings.permissions.allow")
		case xswd.Deny:
			permText = i18n.T("settings.permissions.deny")
		case xswd.Ask:
			permText = i18n.T("settings.permissions.ask")
		}

		add := container.NewVBox(
			textMethod,
			container.NewHBox(
				textSpacer,
				canvas.NewText(permText, permColor),
			),
			textSpacer,
			container.NewHBox(
				sep,
			),
		)

		permForm.Add(add)
	}

	if len(permForm.Objects) == 0 {
		permForm.Add(
			container.NewVBox(
				widget.NewRichTextFromMarkdown(i18n.T("xswd.connection_no_permissions")),
			),
		)
	} else {
		permForm.Add(textSpacer)
	}

	labelEvents := canvas.NewText(i18n.T("xswd.connection_events"), colors.Gray)
	labelEvents.TextSize = 14
	labelEvents.Alignment = fyne.TextAlignLeading
	labelEvents.TextStyle = fyne.TextStyle{Bold: true}

	eventsForm := container.NewVBox()

	// Get registered events from xswd.ApplicationData and create event objects
	for name, b := range ad.RegisteredEvents {
		eventColor := colors.Red
		if b {
			eventColor = colors.Green
		}

		textEvent := widget.NewRichTextFromMarkdown(fmt.Sprintf("### %s", name))
		textEvent.Wrapping = fyne.TextWrapWord

		sep := canvas.NewRectangle(colors.Gray)
		sep.SetMinSize(fyne.NewSize(ui.Width*0.5, 2))

		add := container.NewVBox(
			textEvent,
			container.NewHBox(
				textSpacer,
				canvas.NewText(strconv.FormatBool(b), eventColor),
			),
			textSpacer,
			container.NewHBox(
				sep,
			),
		)

		eventsForm.Add(add)
	}

	if len(eventsForm.Objects) == 0 {
		eventsForm.Add(
			container.NewVBox(
				widget.NewRichTextFromMarkdown(i18n.T("xswd.connection_no_events")),
			),
		)
	}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	done := make(chan struct{})

	btnAllow := widget.NewButton(i18n.T("xswd.allow"), func() {
		if !isWalletGenerationActive(generation) {
			done <- struct{}{}
			return
		}
		confirmed = true
		done <- struct{}{}
	})
	btnAllow.Importance = widget.MediumImportance

	btnDeny := widget.NewButton(i18n.T("xswd.deny"), func() {
		if !isWalletGenerationActive(generation) {
			done <- struct{}{}
			return
		}
		confirmed = false
		done <- struct{}{}
	})
	btnDeny.Importance = widget.MediumImportance

	btnRow := container.NewHBox(layout.NewSpacer(), btnAllow, rectSpacer, btnDeny, layout.NewSpacer())

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				btnRow,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						rectSpacer,
						rectSpacer,
						labelApp,
						textApp,
						rectSpacer,
						labelID,
						textID,
						rectSpacer,
						labelURL,
						textURL,
						rectSpacer,
						labelPermissions,
						permForm,
						rectSpacer,
						labelEvents,
						eventsForm,
						rectSpacer,
					),
				),
			),
		),
	)

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	fyne.Do(func() {
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						container.NewCenter(
							container.NewStack(
								span,
							),
						),
						rectSpacer,
						rectSpacer,
						content,
						btnRow,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)

		if a.Driver().Device().IsMobile() {
			fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: i18n.T("xswd.connection_received")})
			session.Window.RequestFocus()
		} else {
			session.Window.RequestFocus()
		}
	})

	// Wait for user input or socket close
	select {
	case <-done:
	case <-ad.OnClose:
	}

	if !isWalletGenerationActive(generation) || session.Window == nil {
		return false
	}

	fyne.Do(func() {
		if len(overlay.List()) >= 2 {
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}
	})

	go refreshXSWDList()

	return confirmed
}

// Handle incoming TELA link requests and return params to be displayed in approval prompt
func handleTELALinkRequest(linkParams TELALink_Params) (params string, err error) {
	var args []string
	var target string
	target, args, err = tela.ParseTELALink(linkParams.TelaLink)
	if err != nil {
		return
	}

	switch target {
	case "tela":
		switch args[0] {
		case "open": // open TELA content similar to a hyperlink
			if len(args) < 2 || len(args[1]) != 64 {
				err = fmt.Errorf("/open/ request has invalid scid argument")
				return
			}

			// Engram will check content rating and show it in prompt
			var rating tela.Rating_Result
			rating, _ = tela.GetRating(args[1], session.Daemon, 0)

			var index tela.INDEX
			index, err = tela.GetINDEXInfo(args[1], session.Daemon)
			if err != nil {
				// Fallback to cache or placeholder if Gnomon isn't ready
				cache := loadTelaIndexCache()
				if cached, ok := cache[args[1]]; ok {
					index = cached
					err = nil
				} else {
					// Minimal placeholder so the prompt can still show
					index.SCID = args[1]
					index.NameHdr = "TELA App (" + args[1][:8] + "...)"
					index.DescrHdr = "SCID: " + args[1]
					err = nil
				}
			}

			var linkDisplay TELALink_Display
			linkDisplay.Name = index.NameHdr
			linkDisplay.Descr = index.DescrHdr
			linkDisplay.DURL = index.DURL
			linkDisplay.TelaLink = linkParams.TelaLink
			rating.Ratings = nil // don't need to show each individual rating in prompt
			linkDisplay.Rating = &rating

			params = fmt.Sprintf("%+v", linkDisplay)
			if indentParams, err := json.MarshalIndent(linkDisplay, "", " "); err == nil {
				params = string(indentParams)
			}
		default:
			err = fmt.Errorf("invalid argument: %s", args[0])
			return
		}
	case "engram":
		if len(args) < 3 {
			err = fmt.Errorf("invalid engram link format")
			return
		}

		switch args[0] {
		case "asset":
			switch args[1] {
			case "manager": // open asset manager module with scid data
				if len(args[2]) != 64 {
					err = fmt.Errorf("/manager/ request has invalid scid argument")
					return
				}
			default:
				err = fmt.Errorf("invalid argument: %s", args[1])
				return
			}
		default:
			err = fmt.Errorf("invalid argument: %s", args[0])
			return
		}

		params = fmt.Sprintf("%+v", linkParams)
		if indentParams, err := json.MarshalIndent(linkParams, "", " "); err == nil {
			params = string(indentParams) // indent params if able
		}
	default:
		err = fmt.Errorf("invalid target: %s", target)
		return
	}

	return
}

// Ask permission to complete a specific request from a connected application,
// can choose to Allow, Always Allow, Deny, Always Deny the request
func AskPermissionForRequest(ad *xswd.ApplicationData, request *jrpc2.Request) (choice xswd.Permission) {
	generation := currentWalletGeneration()
	if !isWalletGenerationActive(generation) || session.Window == nil {
		return xswd.Deny
	}

	method := request.Method()

	// Check if we already have a stored permission for this app and method
	if ad.Permissions != nil {
		if p, ok := ad.Permissions[method]; ok && p != xswd.Ask {
			logger.Printf("[Engram] Using stored permission for %s -> %s: %s\n", ad.Name, method, p)
			return p
		}
	}

	// Gnomon methods behave as AlwaysAllow
	if strings.HasPrefix(method, "Gnomon.") {
		return xswd.Allow
	}

	// All other methods require approval
	choice = xswd.Deny

	// EPOCH methods - auto-start EPOCH if not active (HOLOGRAM-style, no dialog)
	if strings.HasSuffix(method, "EPOCH") {
		if !epoch.IsActive() {
			// Auto-start EPOCH with user's address or dApp address
			// Start with user's default address - dApp can use AttemptEPOCHWithAddr to specify different address
			err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
			if err != nil {
				logger.Errorf("[EPOCH] Auto-start failed: %s\n", err)
				return xswd.Deny
			}
			remoteAccess.EPOCH.enabled = true
			remoteAccess.EPOCH.err = nil
			setRemoteAccess(epoch.GetPort(), "EPOCH")
			logger.Printf("[EPOCH] Auto-started for dApp request\n")
		}
		// Auto-allow EPOCH methods (no permission dialog needed)
		return xswd.Allow
	}

	overlay := session.Window.Canvas().Overlays()

	headerText := i18n.T("xswd.new_permission_request")

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText(i18n.T("xswd.from"), colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### " + ad.Name)
	textApp.Wrapping = fyne.TextWrapWord

	labelRequest := canvas.NewText(i18n.T("xswd.requesting"), colors.Gray)
	labelRequest.TextSize = 14
	labelRequest.Alignment = fyne.TextAlignLeading
	labelRequest.TextStyle = fyne.TextStyle{Bold: true}

	textRequest := widget.NewRichTextFromMarkdown("### " + method)
	textRequest.Wrapping = fyne.TextWrapWord

	labelParams := canvas.NewText(i18n.T("xswd.parameters"), colors.Gray)
	labelParams.TextSize = 14
	labelParams.Alignment = fyne.TextAlignLeading
	labelParams.TextStyle = fyne.TextStyle{Bold: true}

	params := i18n.T("xswd.none")
	if method == "HandleTELALinks" {
		var linkParams TELALink_Params
		err := request.UnmarshalParams(&linkParams)
		if err != nil {
			logger.Errorf("[Engram] Denied TELA link request %s from %s: %s\n", request.ParamString(), ad.Name, err)
			return
		}

		params, err = handleTELALinkRequest(linkParams)
		if err != nil {
			logger.Errorf("[Engram] Denied TELA link request %q from %s: %s\n", linkParams.TelaLink, ad.Name, err)
			return
		}
	} else if request.ParamString() != "" {
		params = strings.ReplaceAll(strings.Join(strings.Fields(request.ParamString()), " "), "\n", " ")

		// Unmarshall and indent params if able
		var buffer interface{}
		if request.UnmarshalParams(&buffer) == nil {
			if indentParams, err := json.MarshalIndent(buffer, "", " "); err == nil {
				params = string(indentParams)
			}
		}
	}

	textParams := widget.NewLabel(params)
	textParams.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	canStorePermission := remoteAccess.WS.server != nil && remoteAccess.WS.server.CanStorePermission(method)
	alwaysRemember := false
	rememberCheck := widget.NewCheck(i18n.T("xswd.always_remember"), func(checked bool) {
		alwaysRemember = checked
	})
	if !canStorePermission {
		rememberCheck.Hide()
	}

	btnAllow := widget.NewButtonWithIcon(i18n.T("xswd.allow"), theme.ConfirmIcon(), nil)
	btnAllow.Importance = widget.MediumImportance
	btnDeny := widget.NewButtonWithIcon(i18n.T("xswd.deny"), theme.CancelIcon(), nil)

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					btnAllow,
					btnDeny,
					layout.NewSpacer(),
				),
				rememberCheck,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						labelApp,
						textApp,
						rectSpacer,
						labelRequest,
						textRequest,
						rectSpacer,
						labelParams,
						textParams,
					),
				),
			),
		),
	)

	// Create and show request prompt
	done := make(chan struct{})
	btnAllow.OnTapped = func() {
		if !isWalletGenerationActive(generation) {
			done <- struct{}{}
			return
		}
		if alwaysRemember && canStorePermission {
			choice = xswd.AlwaysAllow
		} else {
			choice = xswd.Allow
		}
		done <- struct{}{}
	}

	btnDeny.OnTapped = func() {
		if !isWalletGenerationActive(generation) {
			done <- struct{}{}
			return
		}
		if alwaysRemember && canStorePermission {
			choice = xswd.AlwaysDeny
		} else {
			choice = xswd.Deny
		}
		done <- struct{}{}
	}

	linkRemove := widget.NewHyperlinkWithStyle(i18n.T("xswd.remove_application"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRemove.OnTapped = func() {
		if !isWalletGenerationActive(generation) {
			return
		}
		verificationOverlay(
			false,
			ad.Name,
			i18n.T("xswd.remove_this_application"),
			i18n.T("xswd.remove"),
			func(b bool) {
				if b {
					remoteAccess.WS.server.RemoveApplication(ad)
				}
			},
		)
	}

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	fyne.Do(func() {
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						container.NewCenter(
							container.NewStack(
								span,
							),
						),
						rectSpacer,
						rectSpacer,
						content,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkRemove,
							layout.NewSpacer(),
						),
						rectSpacer,
					),
				),
			),
		)

		if fyne.CurrentApp().Driver().Device().IsMobile() {
			fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: i18n.T("xswd.new_permission_notification")})
			session.Window.RequestFocus()
		} else {
			session.Window.RequestFocus()
		}
	})

	// Wait for user input or socket close
	select {
	case <-done:
	case <-ad.OnClose:
	}

	if !isWalletGenerationActive(generation) || session.Window == nil {
		return xswd.Deny
	}

	fyne.Do(func() {
		if len(overlay.List()) >= 2 {
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}
	})

	go refreshXSWDList()

	// Persist permission for this app if applicable
	if choice == xswd.AlwaysAllow || choice == xswd.AlwaysDeny {
		if ad.Permissions == nil {
			ad.Permissions = make(map[string]xswd.Permission)
		}
		ad.Permissions[method] = choice
		StoreAppPermissions(ad.Name, ad.Permissions)
	} else if choice.IsPositive() {
		StoreAppPermissions(ad.Name, ad.Permissions)
	}

	return choice
}

// Ask user to enable EPOCH when an app requests it but EPOCH is not active
func askEnableEPOCH(ad *xswd.ApplicationData, method string) (choice xswd.Permission) {
	generation := currentWalletGeneration()
	if !isWalletGenerationActive(generation) || session.Window == nil {
		return xswd.Deny
	}

	overlay := session.Window.Canvas().Overlays()

	headerText := "EPOCH  REQUEST"

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText("FROM", colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### " + ad.Name)
	textApp.Wrapping = fyne.TextWrapWord

	labelRequest := canvas.NewText("REQUESTING", colors.Gray)
	labelRequest.TextSize = 14
	labelRequest.Alignment = fyne.TextAlignLeading
	labelRequest.TextStyle = fyne.TextStyle{Bold: true}

	textRequest := widget.NewRichTextFromMarkdown("### Enable EPOCH")
	textRequest.Wrapping = fyne.TextWrapWord

	infoText := widget.NewLabel("This app needs EPOCH (Proof-of-Work) to interact with your wallet.\nEPOCH allows the app to perform mining operations.")
	infoText.Wrapping = fyne.TextWrapWord

	labelMiningAddr := canvas.NewText("Mining address:", colors.Gray)
	labelMiningAddr.TextSize = 12
	labelMiningAddr.Alignment = fyne.TextAlignLeading

	// Radio group for mining address selection - default to dApp Chooses
	miningAddrOptions := []string{"My Address", "dApp Chooses"}
	miningAddrRadio := widget.NewRadioGroup(miningAddrOptions, nil)
	miningAddrRadio.SetSelected("dApp Chooses")

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	choice = xswd.Deny

	btnEnable := widget.NewButtonWithIcon("Enable", theme.ConfirmIcon(), nil)
	btnEnable.Importance = widget.HighImportance
	btnDeny := widget.NewButtonWithIcon("Deny", theme.CancelIcon(), nil)

	done := make(chan struct{})

	btnEnable.OnTapped = func() {
		if !isWalletGenerationActive(generation) {
			done <- struct{}{}
			return
		}

		// Set allowWithAddress based on radio selection
		remoteAccess.EPOCH.allowWithAddress = (miningAddrRadio.Selected == "dApp Chooses")

		// Start EPOCH
		err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
		if err != nil {
			logger.Errorf("[EPOCH] Failed to start: %s\n", err)
			remoteAccess.EPOCH.err = err
			choice = xswd.Deny
		} else {
			remoteAccess.EPOCH.enabled = true
			remoteAccess.EPOCH.err = nil
			setRemoteAccess(epoch.GetPort(), "EPOCH")
			choice = xswd.Allow
			logger.Printf("[EPOCH] Started successfully via permission prompt (allowWithAddress: %v)\n", remoteAccess.EPOCH.allowWithAddress)
		}
		done <- struct{}{}
	}

	btnDeny.OnTapped = func() {
		choice = xswd.Deny
		done <- struct{}{}
	}

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					btnEnable,
					btnDeny,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						labelApp,
						textApp,
						rectSpacer,
						labelRequest,
						textRequest,
						rectSpacer,
						infoText,
						rectSpacer,
						labelMiningAddr,
						miningAddrRadio,
					),
				),
			),
		),
	)

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	overlay.Add(
		container.NewStack(
			&iframe{},
			canvas.NewRectangle(colors.DarkMatter),
		),
	)

	overlay.Add(
		container.NewStack(
			&iframe{},
			container.NewCenter(
				container.NewVBox(
					span,
					container.NewCenter(
						header,
					),
					container.NewCenter(
						container.NewStack(
							span,
						),
					),
					rectSpacer,
					rectSpacer,
					content,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
				),
			),
		),
	)

	if a.Driver().Device().IsMobile() {
		fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: "EPOCH permission request"})
	} else {
		session.Window.RequestFocus()
	}

	// Wait for user input
	select {
	case <-done:
	case <-ad.OnClose:
	}

	if !isWalletGenerationActive(generation) || session.Window == nil {
		return xswd.Deny
	}

	overlay.Top().Hide()
	overlay.Remove(overlay.Top())
	overlay.Remove(overlay.Top())

	go refreshXSWDList()

	// Persist permission for this app if granted
	if choice.IsPositive() {
		StoreAppPermissions(ad.Name, ad.Permissions)
	}

	return choice
}

// Refresh list of connected XSWD apps
func refreshXSWDList() {
	time.Sleep(time.Second)
	xswdStateMu.RLock()
	server := remoteAccess.WS.server
	xswdStateMu.RUnlock()
	if server == nil {
		return
	}

	apps := server.GetApplications()
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	xswdStateMu.Lock()
	remoteAccess.WS.apps = apps
	xswdStateMu.Unlock()
	if remoteAccess.WS.list != nil {
		uiDo(func() {
			if remoteAccess.WS.list == nil {
				return
			}
			remoteAccess.WS.list.UnselectAll()
			remoteAccess.WS.list.FocusLost()
			remoteAccess.WS.list.Refresh()
		})
	}
}

// StoreAppPermissions saves per-app permissions to encrypted storage
func StoreAppPermissions(appName string, permissions map[string]xswd.Permission) error {
	if appName == "" || len(permissions) == 0 {
		return nil
	}
	data, err := json.Marshal(permissions)
	if err != nil {
		logger.Errorf("[Engram] StoreAppPermissions: marshal error: %s\n", err)
		return err
	}
	err = StoreEncryptedValue("XSWD", []byte("AppName:"+appName), data)
	if err != nil {
		logger.Errorf("[Engram] StoreAppPermissions: storage error: %s\n", err)
		return err
	}
	logger.Printf("[Engram] Stored permissions for app %s: %d methods\n", appName, len(permissions))
	return nil
}

// GetAppPermissions retrieves per-app permissions from encrypted storage
func GetAppPermissions(appName string) (map[string]xswd.Permission, error) {
	if appName == "" {
		return nil, nil
	}
	data, err := GetEncryptedValue("XSWD", []byte("AppName:"+appName))
	if err != nil {
		logger.Printf("[Engram] GetAppPermissions: no stored permissions for app %s: %s\n", appName, err)
		return nil, nil // Not an error, just no stored permissions
	}
	var permissions map[string]xswd.Permission
	if err := json.Unmarshal(data, &permissions); err != nil {
		logger.Errorf("[Engram] GetAppPermissions: unmarshal error: %s\n", err)
		return nil, err
	}
	logger.Printf("[Engram] GetAppPermissions: found %d permissions for app %s\n", len(permissions), appName)
	return permissions, nil
}

// DeleteAppPermissions removes per-app permissions from storage
func DeleteAppPermissions(appName string) error {
	if appName == "" {
		return nil
	}
	err := DeleteKey("XSWD", []byte("AppName:"+appName))
	if err != nil {
		logger.Debugf("[Engram] DeleteAppPermissions: %s\n", err)
	}
	return nil
}

// Ask permission to complete a specific Engram action, using xswd permissions to match existing requests that have params to display
func AskPermissionForRequestE(headerText string, params interface{}, cancelChans ...chan struct{}) (choice xswd.Permission, err error) {
	var cancelChan chan struct{}
	if len(cancelChans) > 0 {
		cancelChan = cancelChans[0]
	}

	logger.Debugf("[AskPermissionForRequestE] Prompting for permission: %s\n", headerText)
	choice = xswd.Deny

	var paramString string

	switch p := params.(type) {
	case TELALink_Params:
		paramString, err = handleTELALinkRequest(p)
		if err != nil {
			err = fmt.Errorf("denied TELA link request %s: %s", p.TelaLink, err)
			return
		}
	case string:
		switch p {
		case "TELA R OFF":
			paramString = "You will be viewing all TELA content as per your TELA settings.\n\n"
			paramString += "Min Likes will omit results if they are below the set likes ratio.\n\n"
			paramString += "Search exclusions will omit results that include the set exclusion text in their dURL."
		default:
			err = fmt.Errorf("unknown Engram request param string: %s", p)
			return
		}
	default:
		err = fmt.Errorf("unknown Engram request params: %T", p)
		return
	}

	overlay := session.Window.Canvas().Overlays()

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText("FROM", colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### Engram")
	textApp.Wrapping = fyne.TextWrapWord

	labelParams := canvas.NewText("PARAMETERS", colors.Gray)
	labelParams.TextSize = 14
	labelParams.Alignment = fyne.TextAlignLeading
	labelParams.TextStyle = fyne.TextStyle{Bold: true}

	textParams := widget.NewLabel(paramString)
	textParams.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	done := make(chan struct{})

	btnAllow := widget.NewButton(xswd.Allow.String(), func() {
		choice = xswd.Allow
		done <- struct{}{}
	})
	btnAllow.Importance = widget.MediumImportance

	btnDeny := widget.NewButton(xswd.Deny.String(), func() {
		choice = xswd.Deny
		done <- struct{}{}
	})
	btnDeny.Importance = widget.MediumImportance

	btnRow := container.NewHBox(layout.NewSpacer(), btnAllow, rectSpacer, btnDeny, layout.NewSpacer())

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				btnRow,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						labelApp,
						textApp,
						rectSpacer,
						labelParams,
						textParams,
						rectSpacer,
					),
				),
			),
		),
	)

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	uiDo(func() {
		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)
	})

	uiDo(func() {
		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						container.NewCenter(
							container.NewStack(
								span,
							),
						),
						rectSpacer,
						rectSpacer,
						rectSpacer,
						content,
						btnRow,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	})

	// Wait for user input or cancellation
	select {
	case <-done:
	case <-cancelChan:
		choice = xswd.Deny
		err = fmt.Errorf("cancelled")
	}

	logger.Debugf("[AskPermissionForRequestE] User input received for: %s\n", headerText)

	uiDo(func() {
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	return
}

func isASCII(s string) bool {
	for _, c := range s {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// telaStaleCloneDirFromServeErr extracts the clone directory to remove when ServeTELA fails with
// a "file … already exists" collision inside the TELA shard path.
func telaStaleCloneDirFromServeErr(err error) (dir string, ok bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	const pfx = "file "
	const sfx = " already exists"
	idx := strings.LastIndex(msg, pfx)
	if idx < 0 {
		return "", false
	}
	rest := msg[idx+len(pfx):]
	j := strings.Index(rest, sfx)
	if j < 0 {
		return "", false
	}
	filePath := filepath.Clean(rest[:j])
	if filePath == "" || filePath == "." {
		return "", false
	}
	telaRoot := filepath.Clean(getTelaPath())
	rel, errRel := filepath.Rel(telaRoot, filePath)
	if errRel != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	dir = filepath.Dir(filePath)
	relDir, errRel2 := filepath.Rel(telaRoot, dir)
	if errRel2 != nil || relDir == ".." || strings.HasPrefix(relDir, ".."+string(filepath.Separator)) {
		return "", false
	}
	return dir, true
}

// cleanTELALink ensures the TELA link is safe for the current platform.
// We must preserve localhost on Android because the WebView Network Security
// Policy specifically allows cleartext HTTP for localhost, but blocks 127.0.0.1.
func cleanTELALink(link string) string {
	return strings.TrimSpace(link)
}

// verifyTELAServerIsUp polls the TELA server for up to 5 seconds to ensure it is ready.
func verifyTELAServerIsUp(link string) bool {
	if link == "" {
		return false
	}

	client := http.Client{
		Timeout: 1 * time.Second,
	}

	for i := 0; i < 50; i++ {
		resp, err := client.Get(link)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	return false
}

// serveTELAWithCancel runs tela.ServeTELA in a background goroutine and returns
// early if the cancelled flag is set, preserving the cancel-during-launch UX.
func serveTELAWithCancel(scid, endpoint string, cancelled *atomic.Bool) (string, error) {
	type result struct {
		link string
		err  error
	}
	done := make(chan result, 1)

	go func() {
		link, err := tela.ServeTELA(scid, endpoint)
		if err == nil {
			PatchTELAAppSourceFiles(scid)
		}
		done <- result{link, err}
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r := <-done:
			return cleanTELALink(r.link), r.err
		case <-ticker.C:
			if cancelled != nil && cancelled.Load() {
				return "", fmt.Errorf("cancelled")
			}
		}
	}
}

// serveTELACollisionRecovery calls ServeTELA and, on stale clone file collisions, removes the
// affected clone tree and retries (then falls back to clearing non-running top-level dirs).
func serveTELACollisionRecovery(scid, endpoint string, cancelled ...*atomic.Bool) (link string, err error) {
	var isCancelled *atomic.Bool
	if len(cancelled) > 0 {
		isCancelled = cancelled[0]
	}

	if isCancelled != nil && isCancelled.Load() {
		return "", fmt.Errorf("cancelled")
	}

	link, err = serveTELAWithCancel(scid, endpoint, isCancelled)
	if err != nil {
		if strings.Contains(err.Error(), "cancelled") {
			return "", err
		}
	}
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		return link, err
	}
	if cloneDir, ok := telaStaleCloneDirFromServeErr(err); ok {
		if rmErr := os.RemoveAll(cloneDir); rmErr != nil {
			logger.Printf("[TELA] Remove stale clone dir %s: %v\n", cloneDir, rmErr)
		}

		if isCancelled != nil && isCancelled.Load() {
			return "", fmt.Errorf("cancelled")
		}

		link, err = serveTELAWithCancel(scid, endpoint, isCancelled)
		if err != nil {
			if strings.Contains(err.Error(), "cancelled") {
				return "", err
			}
		}
		if err == nil {
			return link, nil
		}
	}
	telaPath := getTelaPath()
	runningDirs := make(map[string]bool)
	for _, s := range getTelaActiveServers() {
		runningDirs[s.Name] = true
	}
	if entries, readErr := os.ReadDir(telaPath); readErr == nil {
		for _, entry := range entries {
			if entry.IsDir() && !runningDirs[entry.Name()] {
				_ = os.RemoveAll(filepath.Join(telaPath, entry.Name()))
			}
		}
	}
	if isCancelled != nil && isCancelled.Load() {
		return "", fmt.Errorf("cancelled")
	}

	link, err = serveTELAWithCancel(scid, endpoint, isCancelled)

	return link, err
}

// serveTELAWithStaleRecovery reuses an already-running server for the SCID when possible,
// otherwise serves with collision recovery for leftover clone files after shutdown.
func serveTELAWithStaleRecovery(scid, endpoint string, cancelled ...*atomic.Bool) (link string, err error) {

	for _, s := range getTelaActiveServers() {
		if s.SCID == scid {
			PatchTELAAppSourceFiles(scid)
			return cleanTELALink(fmt.Sprintf("http://localhost%s/%s", s.Address, s.Entrypoint)), nil
		}
	}
	return serveTELACollisionRecovery(scid, endpoint, cancelled...)

}

// Wrapper for serving TELA content toggling tela.updates if disabled, updated content should be checked for and presented to the user before calling serveTELAUpdates
func serveTELAUpdates(scid string, cancelled ...*atomic.Bool) (link string, err error) {
	var toggledUpdates bool
	if !areTelaUpdatesAllowed() {
		tela.AllowUpdates(true)
		toggledUpdates = true
	}

	link, err = serveTELACollisionRecovery(scid, session.Daemon, cancelled...)

	if toggledUpdates {
		tela.AllowUpdates(false)
	}

	return
}

// Convert TELA error to shortened string for display
func telaErrorToString(err error) string {
	str := "serving TELA"
	errMsg := err.Error()
	if strings.Contains(errMsg, "user defined no updates and content has been updated to") {
		str = "content has been updated"
	} else if strings.Contains(errMsg, "file ") && strings.Contains(errMsg, " already exists") {
		str = "stale TELA files on disk (remove app folder or retry)"
	} else if strings.Contains(errMsg, "already exists") {
		str = "content already exists"
	}

	return fmt.Sprintf("%s %s", "error", str)
}

// PatchTELAAppSourceFiles scans the cloned TELA app directory and replaces localhost with 127.0.0.1 on mobile
func PatchTELAAppSourceFiles(scid string) {
	if !fyne.CurrentApp().Driver().Device().IsMobile() {
		return
	}

	telaPath := getTelaPath()
	var appDir string
	// Use tela.GetServerInfo() directly to avoid race conditions with the global cached info
	for _, s := range tela.GetServerInfo() {
		if s.SCID == scid {
			appDir = filepath.Join(telaPath, s.Name)
			break
		}
	}

	if appDir == "" {
		logger.Printf("[TELA] PatchTELAAppSourceFiles: SCID %s not found in active servers\n", scid)
		return
	}

	logger.Printf("[TELA] PatchTELAAppSourceFiles: Scanning %s for mobile compatibility patches\n", appDir)

	count := 0
	err := filepath.Walk(appDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".js" || ext == ".html" || ext == ".json" {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// We specifically target localhost:44326 (XSWD) to avoid breaking the origin check
			// while still enabling the explicit loopback IP for mobile WebSocket connectivity.
			if bytes.Contains(content, []byte("localhost:44326")) {
				newContent := bytes.ReplaceAll(content, []byte("localhost:44326"), []byte("127.0.0.1:44326"))
				tmpPath := path + ".tmp"
				err = os.WriteFile(tmpPath, newContent, info.Mode())
				if err != nil {
					return err
				}
				err = os.Rename(tmpPath, path)
				if err != nil {
					os.Remove(tmpPath) // Cleanup on failure
					return err
				}
				count++
				logger.Printf("[TELA] PatchTELAAppSourceFiles: Patched %s (localhost:44326 -> 127.0.0.1:44326)\n", path)
			}
		}
		return nil
	})

	if err != nil {
		logger.Errorf("[TELA] PatchTELAAppSourceFiles error: %v\n", err)
	} else {
		logger.Printf("[TELA] PatchTELAAppSourceFiles: Completed. Patched %d files for SCID %s\n", count, scid)
	}
}

// Get the ratio of likes for a TELA SCID, if ratio < minLines an error will be returned
func getLikesRatioCached(scid, dURL, searchExclusions string, minLikes float64, cachedRatings map[string]tela.Rating_Result) (ratio float64, ratings tela.Rating_Result, err error) {
	if r, ok := cachedRatings[scid]; ok {
		// Check URL exclusion still
		err = telaFilterSearchExclusions(dURL, searchExclusions)
		if err != nil {
			return
		}
		ratings = r
		total := float64(r.Likes + r.Dislikes)
		if total == 0 {
			ratio = 50
		} else {
			ratio = (float64(r.Likes) / total) * 100
		}
		if ratio < minLikes {
			err = fmt.Errorf("%s is below min rating setting", scid)
		}
		return
	}
	return getLikesRatio(scid, dURL, searchExclusions, minLikes)
}

func getLikesRatio(scid, dURL, searchExclusions string, minLikes float64) (ratio float64, ratings tela.Rating_Result, err error) {
	if gnomon.Index == nil {
		err = fmt.Errorf("gnomon is not online")
		return
	}

	err = telaFilterSearchExclusions(dURL, searchExclusions)
	if err != nil {
		return
	}

	ratings, err = tela.GetRating(scid, session.Daemon, 0)
	if err != nil {
		return
	}

	total := float64(ratings.Likes + ratings.Dislikes)
	if total == 0 {
		ratio = 50
	} else {
		ratio = (float64(ratings.Likes) / total) * 100
	}

	if ratio < minLikes {
		err = fmt.Errorf("%s is below min rating setting", scid)
	}

	return
}

// Check if a search exclusion is found in a TELA dURL
func telaFilterSearchExclusions(dURL, searchExclusions string) (err error) {
	for _, split := range strings.Split(searchExclusions, ",") {
		exclude := strings.TrimSpace(split)
		if exclude != "" && strings.Contains(dURL, exclude) {
			err = fmt.Errorf("found search exclusion %q in dURL %s", exclude, dURL)
			return
		}
	}

	return
}

// Sort and return search display strings for list widget
func telaSearchDisplayAll(in []INDEXwithRatings, sortBy string, descending bool) (display []string) {
	telaSearch := make([]INDEXwithRatings, len(in))
	copy(telaSearch, in)
	activeSCIDs := map[string]struct{}{}
	for _, serv := range getTelaActiveServers() {
		activeSCIDs[serv.SCID] = struct{}{}
	}

	switch sortBy {
	case "A-Z", "Z-A":
		sort.Slice(telaSearch, func(i, j int) bool {
			_, iActive := activeSCIDs[telaSearch[i].SCID]
			_, jActive := activeSCIDs[telaSearch[j].SCID]
			if iActive != jActive {
				return iActive
			}
			if descending {
				return telaSearch[i].NameHdr > telaSearch[j].NameHdr
			}
			return telaSearch[i].NameHdr < telaSearch[j].NameHdr
		})
	default: // Ratings
		sort.Slice(telaSearch, func(i, j int) bool {
			iNeg := telaSearch[i].ratings.Dislikes > telaSearch[i].ratings.Likes
			jNeg := telaSearch[j].ratings.Dislikes > telaSearch[j].ratings.Likes

			if descending {
				// Default: Highest to Lowest, unrated at bottom, negative at very bottom
				if iNeg != jNeg {
					return !iNeg // Non-negative apps come first
				}

				if telaSearch[i].ratings.Average != telaSearch[j].ratings.Average {
					return telaSearch[i].ratings.Average > telaSearch[j].ratings.Average
				}
			} else {
				// Ascending: Unrated (0.0) at top, then lowest to highest, negative at bottom
				iUnrated := telaSearch[i].ratings.Average == 0 && !iNeg
				jUnrated := telaSearch[j].ratings.Average == 0 && !jNeg

				if iUnrated != jUnrated {
					return iUnrated // Unrated comes first
				}

				if iNeg != jNeg {
					return !iNeg // Rated/Unrated before Negative
				}

				if telaSearch[i].ratings.Average != telaSearch[j].ratings.Average {
					return telaSearch[i].ratings.Average < telaSearch[j].ratings.Average
				}
			}

			// If averages are equal, check if one is currently active
			_, iActive := activeSCIDs[telaSearch[i].SCID]
			_, jActive := activeSCIDs[telaSearch[j].SCID]
			if iActive != jActive {
				return iActive
			}

			// Finally sort by positive likes, then least dislikes
			if descending {
				if telaSearch[i].ratings.Likes != telaSearch[j].ratings.Likes {
					return telaSearch[i].ratings.Likes > telaSearch[j].ratings.Likes
				}
				if telaSearch[i].ratings.Dislikes != telaSearch[j].ratings.Dislikes {
					return telaSearch[i].ratings.Dislikes < telaSearch[j].ratings.Dislikes
				}
			} else {
				if telaSearch[i].ratings.Likes != telaSearch[j].ratings.Likes {
					return telaSearch[i].ratings.Likes < telaSearch[j].ratings.Likes
				}
				if telaSearch[i].ratings.Dislikes != telaSearch[j].ratings.Dislikes {
					return telaSearch[i].ratings.Dislikes > telaSearch[j].ratings.Dislikes
				}
			}

			return telaSearch[i].NameHdr < telaSearch[j].NameHdr
		})
	}

	for _, ind := range telaSearch {
		display = append(display, ind.NameHdr+";;;"+ind.SCID)
	}

	return
}

// Validate the URL as URI or SC image and return it as a canvas.Image
var telaImageCache sync.Map

func handleImageURL(nameHdr, imageURL string, size fyne.Size) (image *canvas.Image, err error) {
	if res, ok := telaImageCache.Load(imageURL); ok {
		image = canvas.NewImageFromResource(res.(fyne.Resource))
		image.SetMinSize(size)
		image.FillMode = canvas.ImageFillContain
		return
	}

	scImage, err := tela.ValidateImageURL(imageURL, session.Daemon)
	if err != nil {
		return
	}

	var resource fyne.Resource
	image = canvas.NewImageFromResource(nil)

	if scImage != "" {
		// Clean the SVG/image code of any null bytes or leading/trailing whitespace
		cleanCode := bytes.Trim([]byte(scImage), "\x00\n\r\t ")
		resource = fyne.NewStaticResource(nameHdr, cleanCode)
	} else {
		resource, err = fyne.LoadResourceFromURLString(imageURL)
		if err != nil {
			return
		}
	}

	if resource != nil {
		// Filter out HTML masked as images (prevents SVG parser console spam)
		content := resource.Content()
		if len(content) > 10 {
			snippet := strings.ToLower(string(content[:min(len(content), 500)]))
			if strings.Contains(snippet, "<html") || strings.Contains(snippet, "<!doctype") {
				return nil, fmt.Errorf("URL returned HTML instead of image")
			}
		}
		telaImageCache.Store(imageURL, resource)
	}

	image.Resource = resource
	image.SetMinSize(size)
	image.FillMode = canvas.ImageFillContain

	return
}

// Convert session.Domain to string for display
func sessionDomainToString(domain string) string {
	str := strings.TrimPrefix(domain, "app.")
	switch str {
	// case "main":
	// case "create":
	// case "restore":
	// case "settings":
	case "wallet":
		return "Dashboard"
	// case "register":
	case "explorer":
		return "Asset Explorer"
	case "manager":
		return "Asset Manager"
	case "send", "transfers", "messages", "remoteaccess", "Identity", "datapad":
		return fmt.Sprintf("%s%s", strings.ToUpper(str[0:1]), str[1:])
	case "tela", "tela.manager":
		return "TELA"
	case "service":
		return "Services"
	case "sign", "verify":
		return "File Manager"
	case "messages.contact":
		return "Message Contact"
	case "remoteaccess.manager":
		return "Remote Access Manager"
	case "remoteaccess.permissions":
		return "Remote Access Settings"
	case "sc.builder":
		return "Contract Builder"
	case "sc.editor":
		return "Contract Editor"
	default:
		return ""
	}
}

// Add EPOCH session values to the account total stores
func storeEPOCHTotal(timeout time.Duration) {
	epochSession, err := epoch.GetSession(timeout)
	if err == nil {
		remoteAccess.EPOCH.total.Hashes += epochSession.Hashes
		remoteAccess.EPOCH.total.MiniBlocks += epochSession.MiniBlocks

		var eMar []byte
		if eMar, err = json.Marshal(remoteAccess.EPOCH.total); err == nil {
			err = StoreEncryptedValue("RemoteAccess", []byte("EPOCH"), eMar)
		}
	}

	if err != nil {
		logger.Errorf("[EPOCH] Storing total: %s\n", err)
	}
}

// Store account EPOCH session and stop EPOCH
func stopEPOCH() {
	if remoteAccess.EPOCH.enabled {
		storeEPOCHTotal(time.Second * 4)
	}

	epoch.StopGetWork()
	remoteAccess.EPOCH.enabled = false
	//remoteAccess.EPOCH.allowWithAddress = false
}

// Recovery form constants
const (
	MaxAccountNameLength = 25
	HexKeyLength         = 64
	SeedWordCount24      = 24
	SeedWordCount25      = 25
	MaxDisplayFileLen    = 50
)

// Database configuration
const (
	DefaultDBTimeout = 5 * time.Second
	MaxDBRetries     = 3
)

// showFormError displays an error message on a canvas.Text element
func showFormError(text *canvas.Text, msg string) {
	text.Text = msg
	text.Color = colors.Red
	text.Refresh()
}

// showFormSuccess displays a success message on a canvas.Text element
func showFormSuccess(text *canvas.Text, msg string) {
	text.Text = msg
	text.Color = colors.Green
	text.Refresh()
}

// clearFormText clears a canvas.Text element
func clearFormText(text *canvas.Text) {
	text.Text = ""
	text.Refresh()
}

// validateRecoveryForm checks if the recovery form has valid account name and matching passwords
func validateRecoveryForm(name, password, passwordConfirm string) bool {
	return len(password) > 0 && password == passwordConfirm && name != ""
}

// PasswordStrength represents the strength level of a password
type PasswordStrength int

const (
	PasswordWeak PasswordStrength = iota
	PasswordFair
	PasswordGood
	PasswordStrong
)

// getPasswordStrength evaluates password strength based on length and character variety
func getPasswordStrength(password string) PasswordStrength {
	if len(password) == 0 {
		return PasswordWeak
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
		}
	}

	score := 0
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 12 {
		score++
	}
	if hasUpper && hasLower {
		score++
	}
	if hasDigit {
		score++
	}
	if hasSpecial {
		score++
	}

	switch {
	case score >= 4:
		return PasswordStrong
	case score >= 3:
		return PasswordGood
	case score >= 2:
		return PasswordFair
	default:
		return PasswordWeak
	}
}

// getPasswordStrengthText returns a human-readable description of password strength
func getPasswordStrengthText(strength PasswordStrength) string {
	switch strength {
	case PasswordStrong:
		return "Strong"
	case PasswordGood:
		return "Good"
	case PasswordFair:
		return "Fair"
	default:
		return "Weak"
	}
}

// getPasswordStrengthColor returns the color associated with password strength
func getPasswordStrengthColor(strength PasswordStrength) color.Color {
	switch strength {
	case PasswordStrong:
		return colors.Green
	case PasswordGood:
		return colors.Green
	case PasswordFair:
		return colors.Yellow
	default:
		return colors.Red
	}
}

// Debouncer provides debounced function execution
type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	duration time.Duration
}

// NewDebouncer creates a new Debouncer with the specified duration
func NewDebouncer(duration time.Duration) *Debouncer {
	return &Debouncer{duration: duration}
}

// Debounce executes the function after the debounce duration, canceling any pending execution
func (d *Debouncer) Debounce(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, fn)
}

// parseTelaListEntry parses a raw TELA list string into name and SCID
func parseTelaListEntry(raw string) (name, scid string) {
	split := strings.Split(raw, ";;;")
	if len(split) > 0 {
		name = split[0]
	}
	if len(split) > 1 {
		scid = split[1]
	}
	return
}

// normalizeTelaSearch normalizes a search string for TELA
func normalizeTelaSearch(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isDisplayableTelaApp checks if a TELA index should be displayed in the browser
func isDisplayableTelaApp(index tela.INDEX) bool {
	if len(index.DOCs) < 1 {
		return false
	}

	if strings.HasSuffix(index.DURL, tela.TAG_LIBRARY) || strings.HasSuffix(index.DURL, tela.TAG_DOC_SHARDS) || strings.HasSuffix(index.DURL, tela.TAG_BOOTSTRAP) {
		return false
	}

	return true
}
