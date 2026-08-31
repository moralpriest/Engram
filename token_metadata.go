// Copyright 2023-2026 DERO Foundation. All rights reserved.

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
	hgstorage "github.com/hypergnomon/hypergnomon/storage"
	hgstructures "github.com/hypergnomon/hypergnomon/structures"
)

// TokenInfo holds resolved token metadata. A portion mirrors TUI wallet's TokenInfo
// but lives in main package so dashboard/history/send can share it without import cycle.
type TokenInfo struct {
	SCID     string
	Name     string
	Ticker   string
	Decimals uint64
	Balance  uint64
}

var (
	tokenFnRe          = regexp.MustCompile(`(?is)Function\s+(Name|Symbol|Ticker|Decimals)\s*\(\s*\)\s*(String|Uint64)(.*?)End Function`)
	tokenReturnLoadRe  = regexp.MustCompile(`(?i)RETURN\s+LOAD\s*\(\s*"([^"]+)"\s*\)`)
	tokenReturnQuoteRe = regexp.MustCompile(`(?i)RETURN\s+"([^"]+)"`)
	tokenReturnUintRe  = regexp.MustCompile(`(?i)RETURN\s+(\d+)`)
)

var tokenMetaKeys = []string{
	"name", "Name", "symbol", "Symbol", "ticker", "Ticker",
	"decimals", "Decimals", "n", "s", "d",
	"metadata", "var_header_name", "nameHdr",
}

var tokenNameCache sync.Map // scid(lower)->TokenInfo

// On-disk metadata cache. tokenNameCache is in-memory only; on a thin mobile
// client pointed at a remote node the local store is empty, so without disk
// persistence every launch re-fetches every token's metadata over the network.
// We persist the resolved map (like the TELA caches do) under a single key and
// hydrate tokenNameCache from it once at first use.
const tokenMetaBucket = "Token Metadata"
const tokenMetaCacheKey = "MetadataCache"

var (
	tokenCacheMu     sync.Mutex
	tokenCacheDirty  bool
	tokenCacheSaving bool
)

// loadTokenMetadataCache hydrates tokenNameCache from the on-disk cache exactly
// once at first use. Safe to call repeatedly; no-ops after the first load.
func loadTokenMetadataCache() {
	if cached, _ := tokenNameCache.Load("__loaded__"); cached != nil {
		return
	}
	raw, err := GetEncryptedValue(tokenMetaBucket, []byte(tokenMetaCacheKey))
	if err != nil || len(raw) == 0 {
		tokenNameCache.Store("__loaded__", true)
		return
	}
	var m map[string]TokenInfo
	if err := json.Unmarshal(raw, &m); err != nil {
		tokenNameCache.Store("__loaded__", true)
		return
	}
	for scid, info := range m {
		if scid == "" {
			continue
		}
		tokenNameCache.Store(strings.ToLower(scid), info)
	}
	tokenNameCache.Store("__loaded__", true)
}

// markTokenCacheDirty flags that resolved metadata changed and kicks off the
// debounced disk writer so rapid hydration (dashboard/history/send, many SCIDs
// in quick succession) collapses into one write instead of one per token.
func markTokenCacheDirty() {
	tokenCacheMu.Lock()
	tokenCacheDirty = true
	saving := tokenCacheSaving
	tokenCacheMu.Unlock()
	if !saving {
		go tokenCacheSaveLoop()
	}
}

func tokenCacheSaveLoop() {
	for {
		tokenCacheMu.Lock()
		if !tokenCacheDirty {
			tokenCacheSaving = false
			tokenCacheMu.Unlock()
			return
		}
		tokenCacheDirty = false
		tokenCacheSaving = true
		tokenCacheMu.Unlock()

		// Snapshot only entries with real metadata (skip empty SCID stubs).
		snapshot := map[string]TokenInfo{}
		tokenNameCache.Range(func(k, v interface{}) bool {
			key, okK := k.(string)
			info, okV := v.(TokenInfo)
			if okK && okV && !strings.HasPrefix(key, "__") && (info.Name != "" || info.Ticker != "") {
				snapshot[key] = info
			}
			return true
		})
		if data, err := json.Marshal(snapshot); err == nil {
			_ = StoreEncryptedValue(tokenMetaBucket, []byte(tokenMetaCacheKey), data)
		}
		time.Sleep(800 * time.Millisecond)
	}
}

// Token family: only these 4 classes are considered tokens (fast path).
// G45-NFT, NFA, G45-C, TELA-*, SWAP, EPOCH, UNKNOWN are NOT tokens.
var tokenLikeClasses = []string{"DERO-ASSET", "G45-AT", "G45-FAT", "T345"}

func IsTokenLikeClass(class string) bool {
	switch class {
	case "DERO-ASSET", "G45-AT", "G45-FAT", "T345":
		return true
	default:
		return false
	}
}

// decodeSCString hex-decodes a DERO SC string value; guards NOT AVAILABLE.
func decodeSCString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "NOT AVAILABLE") {
		return ""
	}
	if b, err := hex.DecodeString(s); err == nil {
		decoded := string(b)
		printable := true
		for _, r := range decoded {
			if r < 32 || r == 127 {
				printable = false
				break
			}
		}
		if printable && strings.TrimSpace(decoded) != "" {
			return strings.TrimSpace(decoded)
		}
	}
	return s
}

func storeVariableToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return decodeSCString(val)
	case []byte:
		return decodeSCString(string(val))
	case uint64:
		return strconv.FormatUint(val, 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
	}
	return ""
}

// tokenMetadataFromStore tries local HyperGnomon bbolt store for name/symbol/decimals.
// Mirrors TUI wallet TokenMetadataFromStore but uses main's global gnomon.
func tokenMetadataFromStore(scid string) (name, ticker string, decimals uint64, ok bool) {
	scid = strings.ToLower(strings.TrimSpace(scid))
	if scid == "" || gnomon.Index == nil {
		return "", "", 0, false
	}
	// fast direct lookups via GetAllSCIDVariableDetails (works for both backends)
	vars := gnomon.GetAllSCIDVariableDetails(scid)
	if len(vars) == 0 {
		return "", "", 0, false
	}
	for _, v := range vars {
		if v == nil {
			continue
		}
		keyStr, isStr := v.Key.(string)
		if !isStr {
			continue
		}
		lk := strings.ToLower(keyStr)
		switch lk {
		case "name":
			if name == "" {
				if s := storeVariableToString(v.Value); s != "" {
					name = s
				}
			}
		case "symbol", "ticker":
			if ticker == "" {
				if s := storeVariableToString(v.Value); s != "" {
					ticker = s
				}
			}
		case "decimals":
			if decimals == 0 {
				if s := storeVariableToString(v.Value); s != "" {
					if d, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64); err == nil {
						decimals = d
					}
				}
			}
		case "var_header_name", "namehdr":
			if name == "" {
				if s := storeVariableToString(v.Value); s != "" {
					name = s
				}
			}
		case "n":
			if name == "" {
				if s := storeVariableToString(v.Value); s != "" {
					name = s
				}
			}
		case "s":
			if ticker == "" {
				if s := storeVariableToString(v.Value); s != "" {
					ticker = s
				}
			}
		case "d":
			if decimals == 0 {
				if s := storeVariableToString(v.Value); s != "" {
					if d, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64); err == nil {
						decimals = d
					}
				}
			}
		case "metadata":
			if s := storeVariableToString(v.Value); s != "" {
				// metadata may be JSON {"name":…, "symbol":…}
				var blob map[string]interface{}
				if err := json.Unmarshal([]byte(s), &blob); err == nil {
					if name == "" {
						if mv, _ := blob["name"].(string); strings.TrimSpace(mv) != "" {
							name = strings.TrimSpace(mv)
						}
					}
					if ticker == "" {
						if mv, _ := blob["symbol"].(string); strings.TrimSpace(mv) != "" {
							ticker = strings.TrimSpace(mv)
						}
					}
				}
			}
		}
	}
	// also check var_header_name fallback via case-insensitive scan already handled
	ok = name != "" || ticker != ""
	return name, ticker, decimals, ok
}

func getSCIDClass(scid string) string {
	scid = strings.ToLower(strings.TrimSpace(scid))
	if scid == "" {
		return ""
	}
	// BBolt compat wrapper exposes Inner() for HyperGnomon-specific methods
	if gnomon.BBolt != nil && gnomon.BBolt.Inner() != nil {
		if meta, err := gnomon.BBolt.Inner().GetSCIDClass(scid); err == nil && meta != nil {
			return meta.Class
		}
	}
	if gnomon.Index != nil && gnomon.Index.BBSBackend != nil && gnomon.Index.BBSBackend.Inner() != nil {
		if meta, err := gnomon.Index.BBSBackend.Inner().GetSCIDClass(scid); err == nil && meta != nil {
			return meta.Class
		}
	}
	return ""
}

func tokenLikeSCIDs() []string {
	seen := map[string]bool{}
	var out []string
	for _, class := range tokenLikeClasses {
		var installs []hgstructures.ClassInstall
		var err error
		if gnomon.BBolt != nil && gnomon.BBolt.Inner() != nil {
			installs, err = gnomon.BBolt.Inner().GetClassInstalls(class, 0)
		} else if gnomon.Index != nil && gnomon.Index.BBSBackend != nil && gnomon.Index.BBSBackend.Inner() != nil {
			installs, err = gnomon.Index.BBSBackend.Inner().GetClassInstalls(class, 0)
		} else {
			continue
		}
		if err != nil || len(installs) == 0 {
			continue
		}
		for _, inst := range installs {
			if inst.SCID == "" || seen[inst.SCID] {
				continue
			}
			seen[inst.SCID] = true
			out = append(out, inst.SCID)
		}
	}
	return out
}

// addressKey returns the address key form hypergnomon uses to key the
// addr_scids index. The index stores addresses as the hex encoding of the
// 33-byte compressed public key (network byte + compressed point), NOT the
// base58 "deto1..." string. Querying with base58 (as older code did) never
// matched, so the address-touch discovery source was always empty; on a
// wallet that only ever received an UNKNOWN-class token (e.g. DST before the
// indexer classifies it), this source is what surfaces it.
func addressKey() string {
	if engram.Disk == nil {
		return ""
	}
	return fmt.Sprintf("%x", engram.Disk.Get_Keys().Public.EncodeCompressed())
}

func addressSCIDs(addr string) map[string]bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	// HyperGnomon keys by the hex of the compressed public key, not base58.
	key := addressKey()
	if key == "" {
		return nil
	}
	query := func(m map[string]*hgstructures.AddrSCIDEntry) map[string]bool {
		if len(m) == 0 {
			return nil
		}
		out := make(map[string]bool, len(m))
		for scid := range m {
			out[strings.ToLower(strings.TrimSpace(scid))] = true
		}
		return out
	}

	try := func(inner *hgstorage.BboltStore) map[string]bool {
		if inner == nil {
			return nil
		}
		if m, err := inner.GetAddressSCIDs(key); err == nil {
			if out := query(m); len(out) > 0 {
				return out
			}
		}
		return nil
	}
	if gnomon.BBolt != nil {
		if out := try(gnomon.BBolt.Inner()); out != nil {
			return out
		}
	}
	if gnomon.Index != nil && gnomon.Index.BBSBackend != nil {
		if out := try(gnomon.Index.BBSBackend.Inner()); out != nil {
			return out
		}
	}
	return nil
}

func applyTokenMetadata(info *TokenInfo, code string, vals map[string]string) {
	loadNameKey, loadTickerKey := "", ""
	for _, m := range tokenFnRe.FindAllStringSubmatch(code, -1) {
		if len(m) < 4 {
			continue
		}
		fn, body := strings.ToLower(m[1]), m[3]
		if lm := tokenReturnLoadRe.FindStringSubmatch(body); len(lm) == 2 {
			switch fn {
			case "name":
				loadNameKey = lm[1]
			case "symbol", "ticker":
				loadTickerKey = lm[1]
			}
			continue
		}
		if qm := tokenReturnQuoteRe.FindStringSubmatch(body); len(qm) == 2 {
			switch fn {
			case "name":
				if info.Name == "" {
					info.Name = qm[1]
				}
			case "symbol", "ticker":
				if info.Ticker == "" {
					info.Ticker = qm[1]
				}
			}
			continue
		}
		if fn == "decimals" {
			if um := tokenReturnUintRe.FindStringSubmatch(body); len(um) == 2 && info.Decimals == 0 {
				if d, err := strconv.ParseUint(um[1], 10, 64); err == nil {
					info.Decimals = d
				}
			}
		}
	}
	setFromVals := func(keys []string, dst *string) {
		if *dst != "" {
			return
		}
		for _, k := range keys {
			if v := strings.TrimSpace(vals[k]); v != "" && !strings.HasPrefix(v, "NOT AVAILABLE") {
				*dst = v
				return
			}
		}
	}
	if loadNameKey != "" && info.Name == "" {
		if v := strings.TrimSpace(vals[strings.ToLower(loadNameKey)]); v != "" {
			info.Name = v
		}
	}
	if loadTickerKey != "" && info.Ticker == "" {
		if v := strings.TrimSpace(vals[strings.ToLower(loadTickerKey)]); v != "" {
			info.Ticker = v
		}
	}
	setFromVals([]string{"name", "var_header_name", "namehdr", "n"}, &info.Name)
	setFromVals([]string{"symbol", "ticker", "s"}, &info.Ticker)
	if info.Decimals == 0 {
		for _, k := range []string{"decimals", "d"} {
			if v := strings.TrimSpace(vals[k]); v != "" {
				if d, err := strconv.ParseUint(v, 10, 64); err == nil {
					info.Decimals = d
					break
				}
			}
		}
	}
	if meta := vals["metadata"]; meta != "" && (info.Name == "" || info.Ticker == "") {
		var blob map[string]interface{}
		if err := json.Unmarshal([]byte(meta), &blob); err == nil {
			if info.Name == "" {
				if s, _ := blob["name"].(string); strings.TrimSpace(s) != "" {
					info.Name = strings.TrimSpace(s)
				}
			}
			if info.Ticker == "" {
				if s, _ := blob["symbol"].(string); strings.TrimSpace(s) != "" {
					info.Ticker = strings.TrimSpace(s)
				}
			}
		}
	}
}

func daemonRPCURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("empty endpoint")
	}
	if strings.HasPrefix(endpoint, "http") {
		return endpoint, nil
	}
	return "http://" + endpoint + "/json_rpc", nil
}

var sharedHTTPClient = &http.Client{Timeout: 10 * time.Second}

// tokenFetchConcurrency caps how many GetSC RPCs to the remote node are in
// flight at once. A cold load of many tokens no longer fires N simultaneous
// requests (which slow down and get throttled on mobile/remote-node setups).
const tokenFetchConcurrency = 3

type tokenFetchCall struct {
	done chan struct{}
	info TokenInfo
}

var (
	tokenFetchMu    sync.Mutex
	tokenFetchInFly = map[string]*tokenFetchCall{} // scid(lower) -> in-flight call
	tokenFetchSema  = make(chan struct{}, tokenFetchConcurrency)
)

// storeTokenMetadataCache puts resolved metadata in the in-memory cache and
// flags it for the debounced on-disk persist.
func storeTokenMetadataCache(scid string, info TokenInfo) {
	lower := strings.ToLower(scid)
	tokenNameCache.Store(lower, info)
	if info.Name != "" || info.Ticker != "" {
		markTokenCacheDirty()
	}
}

// tokenStoreFallback fills info's metadata from the local store (no RPC).
func tokenStoreFallback(info TokenInfo) TokenInfo {
	if n, t, d, ok := tokenMetadataFromStore(info.SCID); ok && (n != "" || t != "") {
		info.Name, info.Ticker, info.Decimals = n, t, d
	}
	return info
}

// fetchTokenMetadata fetches token name/ticker/decimals via DERO.GetSC with targeted keys.
// Falls back to local store. Returns info with SCID populated even on failure.
// Concurrent requests for the same SCID share a single in-flight RPC
// (single-flight), and in-flight RPCs to the remote node are capped by
// tokenFetchConcurrency. All callers are already async goroutines, so blocking
// to wait on a shared call is safe.
func fetchTokenMetadata(ctx context.Context, scidStr string) TokenInfo {
	info := TokenInfo{SCID: strings.TrimSpace(scidStr)}
	loadTokenMetadataCache()
	// Fast path: cached
	if v, ok := tokenNameCache.Load(strings.ToLower(info.SCID)); ok {
		if cached, ok := v.(TokenInfo); ok && (cached.Name != "" || cached.Ticker != "") {
			cached.SCID = info.SCID
			return cached
		}
	}
	// Store fallback fast path (local bbolt, no RPC)
	if n, t, d, ok := tokenMetadataFromStore(info.SCID); ok && (n != "" || t != "") {
		info.Name, info.Ticker, info.Decimals = n, t, d
		storeTokenMetadataCache(info.SCID, info)
		return info
	}
	lower := strings.ToLower(info.SCID)

	// Single-flight: if another goroutine is already fetching this SCID, wait on it.
	tokenFetchMu.Lock()
	call := tokenFetchInFly[lower]
	if call != nil {
		tokenFetchMu.Unlock()
		select {
		case <-call.done:
			res := call.info
			res.SCID = info.SCID
			storeTokenMetadataCache(info.SCID, res)
			return res
		case <-ctx.Done():
			// Caller gave up waiting; best-effort local store only.
			if n, t, d, ok := tokenMetadataFromStore(info.SCID); ok && (n != "" || t != "") {
				info.Name, info.Ticker, info.Decimals = n, t, d
				storeTokenMetadataCache(info.SCID, info)
			}
			return info
		}
	}
	call = &tokenFetchCall{done: make(chan struct{})}
	tokenFetchInFly[lower] = call
	tokenFetchMu.Unlock()

	call.info = fetchTokenMetadataRPC(ctx, info)
	close(call.done)

	tokenFetchMu.Lock()
	if tokenFetchInFly[lower] == call {
		delete(tokenFetchInFly, lower)
	}
	tokenFetchMu.Unlock()

	storeTokenMetadataCache(info.SCID, call.info)
	return call.info
}

// tokenGetSCResult is the parsed outcome of a single DERO.GetSC call: the SC
// code string and a map of requested string values.
type tokenGetSCResult struct {
	code string
	vals map[string]string
}

// tokenGetSC executes one DERO.GetSC RPC for metadata keys. It is a package var
// so tests can substitute a recorder and verify the semaphore/concurrency cap
// without touching the network.
var tokenGetSC = func(ctx context.Context, info TokenInfo) (tokenGetSCResult, bool) {
	daemonAddr := strings.TrimSpace(session.Daemon)
	if daemonAddr == "" || daemonAddr == "Not connected" {
		if walletapi.Daemon_Endpoint_Active != "" {
			daemonAddr = walletapi.Daemon_Endpoint_Active
		} else {
			return tokenGetSCResult{}, false
		}
	}
	rpcURL, err := daemonRPCURL(daemonAddr)
	if err != nil {
		return tokenGetSCResult{}, false
	}
	keys := tokenMetaKeys
	params := rpc.GetSC_Params{
		SCID:       info.SCID,
		Code:       true,
		KeysString: keys,
	}
	payloadObj := map[string]interface{}{
		"jsonrpc": "2.0", "id": "1", "method": "DERO.GetSC", "params": params,
	}
	bodyBytes, err := json.Marshal(payloadObj)
	if err != nil {
		return tokenGetSCResult{}, false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", rpcURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return tokenGetSCResult{}, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return tokenGetSCResult{}, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenGetSCResult{}, false
	}
	var rpcResp struct {
		Result rpc.GetSC_Result `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return tokenGetSCResult{}, false
	}
	if rpcResp.Error != nil {
		return tokenGetSCResult{}, false
	}
	res := tokenGetSCResult{code: rpcResp.Result.Code, vals: map[string]string{}}
	for i, k := range keys {
		if i >= len(rpcResp.Result.ValuesString) {
			break
		}
		raw := rpcResp.Result.ValuesString[i]
		if raw == "" || strings.HasPrefix(raw, "NOT AVAILABLE") {
			continue
		}
		if s := decodeSCString(raw); s != "" {
			res.vals[strings.ToLower(k)] = s
		} else {
			res.vals[strings.ToLower(k)] = strings.TrimSpace(raw)
		}
	}
	return res, true
}

// fetchTokenMetadataRPC performs the network GetSC for one token, bounded by
// tokenFetchConcurrency. It never caches; fetchTokenMetadata owns cache updates.
func fetchTokenMetadataRPC(ctx context.Context, info TokenInfo) TokenInfo {
	// Bound concurrent RPCs to the remote node.
	select {
	case tokenFetchSema <- struct{}{}:
		defer func() { <-tokenFetchSema }()
	case <-ctx.Done():
		return tokenStoreFallback(info)
	}

	res, ok := tokenGetSC(ctx, info)
	if !ok {
		return tokenStoreFallback(info)
	}
	applyTokenMetadata(&info, res.code, res.vals)
	if info.Name == "" && info.Ticker == "" {
		if n, t, d, ok := tokenMetadataFromStore(info.SCID); ok {
			if n != "" {
				info.Name = n
			}
			if t != "" {
				info.Ticker = t
			}
			if d != 0 && info.Decimals == 0 {
				info.Decimals = d
			}
		}
	}
	return info
}

// resolveTokenDisplayName returns a human label for a SCID (ticker preferred, else name, else truncated SCID).
// Synchronous fast path: cache + local store; async fetch can hydrate later.
func resolveTokenDisplayName(scid crypto.Hash) string {
	if scid.IsZero() {
		return "DERO"
	}
	s := scid.String()
	loadTokenMetadataCache()
	if v, ok := tokenNameCache.Load(strings.ToLower(s)); ok {
		if cached, ok := v.(TokenInfo); ok {
			if cached.Ticker != "" {
				return cached.Ticker
			}
			if cached.Name != "" {
				return cached.Name
			}
		}
	}
	if n, t, _, ok := tokenMetadataFromStore(s); ok {
		if t != "" {
			tokenNameCache.Store(strings.ToLower(s), TokenInfo{SCID: s, Name: n, Ticker: t})
			return t
		}
		if n != "" {
			tokenNameCache.Store(strings.ToLower(s), TokenInfo{SCID: s, Name: n, Ticker: t})
			return n
		}
	}
	// Check classic header keys via getContractHeader (covers var_header_name etc.)
	if n, _, _, _, _ := getContractHeader(scid); n != "" && n != "--" {
		decoded := decodeSCString(n)
		if decoded != "" {
			return decoded
		}
		return n
	}
	return s[:8] + "…" // truncated until hydrated
}

func resolveTokenDisplayNameString(scidStr string) string {
	h := crypto.HashHexToHash(strings.TrimSpace(scidStr))
	zero := crypto.Hash{}
	if h.IsZero() && strings.TrimSpace(scidStr) != zero.String() {
		return strings.TrimSpace(scidStr)
	}
	return resolveTokenDisplayName(h)
}

// formatTokenAmount formats atomic amount with decimals (like TUI wallet FormatTokenAmount).
func formatTokenAmount(balance uint64, decimals uint64) string {
	if decimals == 0 {
		return strconv.FormatUint(balance, 10)
	}
	divisor := uint64(1)
	for i := uint64(0); i < decimals; i++ {
		divisor *= 10
	}
	whole := balance / divisor
	frac := balance % divisor
	fracStr := fmt.Sprintf("%0*d", decimals, frac)
	fracStr = strings.TrimRight(fracStr, "0")
	if fracStr == "" {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.%s", whole, fracStr)
}

func getTokenDecimals(scid crypto.Hash) uint64 {
	s := scid.String()
	loadTokenMetadataCache()
	if v, ok := tokenNameCache.Load(strings.ToLower(s)); ok {
		if cached, ok := v.(TokenInfo); ok && cached.Decimals != 0 {
			return cached.Decimals
		}
	}
	if _, _, d, ok := tokenMetadataFromStore(s); ok && d != 0 {
		return d
	}
	// Trigger async fetch but return 0 immediately; caller will reformat after hydration.
	go func() {
		info := fetchTokenMetadata(context.Background(), s)
		if info.Decimals != 0 {
			tokenNameCache.Store(strings.ToLower(s), info)
		}
	}()
	return 0
}

// Owned-token cache: the discovery fallback in listOwnedTokens probes the
// decrypted balance across the wallet's candidate SCID set (which, after the
// address-key fix, is every SCID this wallet has touched per the index). On a
// desktop with a full local node that candidate set can be large, and probing
// each SCID is one daemon RPC, so we persist the owned SCIDs per wallet and
// only re-probe on cache miss / when the wallet's own Balance map is empty.
// Keyed by wallet address (per-wallet, encrypted like the metadata cache).
const ownedTokenBucket = "Token Metadata"
const ownedTokenKeyPrefix = "OwnedSCIDs:" // ownedTokenPoll bounds the mobile/remote dashboard fallback: when the token
// overlay opens before the wallet has finished sync+decryption, discovery can
// return empty. Poll at the same cadence as the background daemon refresh,
// re-rendering as soon as owned tokens appear. On a remote node the wallet
// sync can take well over a minute, so the budget is generous; the loop also
// exits early once the wallet is fully synced and still holds nothing.
const ownedTokenPollBudget = 3 * time.Minute
const ownedTokenPollInterval = 3 * time.Second

var ownedTokenCacheMu sync.Mutex

// loadOwnedTokenCache returns the persisted owned SCIDs for the current wallet
// address (could be nil/empty if never cached).
func loadOwnedTokenCache(addr string) map[string]bool {
	out := map[string]bool{}
	if engram.Disk == nil || addr == "" {
		return out
	}
	raw, err := GetEncryptedValue(ownedTokenBucket, []byte(ownedTokenKeyPrefix+addr))
	if err != nil || len(raw) == 0 {
		return out
	}
	for _, s := range strings.Split(string(raw), ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && s != "0000000000000000000000000000000000000000000000000000000000000000" {
			out[s] = true
		}
	}
	return out
}

func saveOwnedTokenCache(addr string, owned map[string]bool) {
	if engram.Disk == nil || addr == "" || len(owned) == 0 {
		return
	}
	ownedTokenCacheMu.Lock()
	defer ownedTokenCacheMu.Unlock()
	var parts []string
	for s := range owned {
		parts = append(parts, s)
	}
	sort.Strings(parts)
	_ = StoreEncryptedValue(ownedTokenBucket, []byte(ownedTokenKeyPrefix+addr), []byte(strings.Join(parts, ",")))
}

// listOwnedTokens returns SCIDs with non-zero decrypted balance for current
// wallet. Discovery is balance-driven (matching what the pre-token-layer Assets
// page did), NOT class-driven: any SCID the wallet holds a balance in shows up,
// including UNKNOWN-class tokens (e.g. DST before the indexer classifies it)
// that the class-install list never includes.
//
// Sources, cheapest first:
//  1. Wallet Balance map — O(owned), instant, no bbolt/network.
//  2. Persisted owned-SCID cache (per wallet) — surfaced from a prior probe.
//  3. Full probe: every SCID the wallet touched per the index (24 groups +
//     address touches after the key fix) — probe each, keep bal>0, re-cache.
//
// prewarmOwnedTokens populates the per-wallet owned-SCID cache in the
// background so the first picker render is instant. The balance probe over the
// index candidate set is one-time but can take tens of seconds on a thin
// mobile client pointed at a remote node (each candidate is a daemon RPC).
// Doing it lazily inside the picker's first render is what made the token list
// show "Loading…" for 30-40s; running it at page load means the cache is warm
// before the user ever taps the asset button. idempotent for the current
// wallet address within a session; safe to call from dashboard and send.
var (
	prewarmMu   sync.Mutex
	prewarmAddr string
)

func prewarmOwnedTokens() {
	if engram.Disk == nil {
		return
	}
	addr := engram.Disk.GetAddress().String()
	if addr == "" {
		return
	}
	// P1: allow re-probe while cache is still cold. The original same-addr
	// guard prevented any re-probe after the first empty result, so a mobile
	// wallet that started with an empty local bbolt (remote node, index not
	// yet synced) would never retry after the index caught up. Now we only
	 // skip if we already have a persisted OwnedSCIDs cache.
	if len(loadOwnedTokenCache(addr)) > 0 {
		prewarmMu.Lock()
		same := prewarmAddr == addr
		prewarmAddr = addr
		prewarmMu.Unlock()
		if same {
			return
		}
	} else {
		prewarmMu.Lock()
		prewarmAddr = addr
		prewarmMu.Unlock()
	}
	go func() {
		// Run the expensive probe in the background so listOwnedTokens()
		// (read in the pickers) returns instantly from cache afterward.
		warmOwnedTokens(addr)
	}()
}

// knownFallbackSCIDs returns a tiny curated set of SCIDs to probe when the
// local index is still empty (mobile/remote first run). This is the P1
// fallback that makes DST and any previously-selected asset discoverable
// without requiring a full 50k allIndexedSCIDs scan or a synced HyperGnomon.
// The set is small (≤5 RPCs) so it is instant on remote.
func knownFallbackSCIDs(addr string) []string {
	var out []string
	// DST – the token used in live tests and the user's reported wallet.
	out = append(out, "d74d1bb9968e3947a9bd40c5a9bdf598135f6b07a93bc98ded1fefa6ddd36bf5")
	// Previously selected asset (if any) – covers the user's last choice.
	if !sendSelectedSCID.IsZero() {
		out = append(out, strings.ToLower(sendSelectedSCID.String()))
	} else if raw, err := GetEncryptedValue("settings", []byte(selectedAssetKey(addr))); err == nil && len(raw) > 0 {
		if s := strings.ToLower(strings.TrimSpace(string(raw))); s != "" && s != "0000000000000000000000000000000000000000000000000000000000000000" {
			out = append(out, s)
		}
	}
	// Any SCID already persisted in OwnedSCIDs cache (already handled) but keep for dedup.
	return out
}

// warmOwnedTokens runs the expensive full-index balance probe and persists the
// result to the per-wallet owned-SCID cache. It is called asynchronously (page
// load) so the picker's first render never blocks on it; the small lightweight
// poll in the picker re-reads the cache as it warms. singleflight by address.
func warmOwnedTokens(addr string) {
	if engram.Disk == nil || addr == "" {
		return
	}
	// Skip the probe only if we already have a persisted discovery for this
	// wallet. Do NOT gate this on listOwnedTokensFast() — every real wallet
	// holds DERO, so the wallet Balance map is always non-empty and would
	// never trigger a probe, leaving received UNKNOWN-class tokens (e.g.
	// DST) undiscovered. If the persisted cache is cold, run the probe once.
	var owned map[string]bool
	if len(loadOwnedTokenCache(addr)) > 0 {
		owned = loadOwnedTokenCache(addr)
	} else {
		owned = ownedProbeOnce(addr)
		saveOwnedTokenCache(addr, owned)
	}
	// Register every discovered SCID with the wallet itself (walletapi
	// TokenAdd). This is the durable, index-independent source: the wallet
	// persists the SCID in its own file and re-syncs its balance straight
	// from the daemon every few seconds (the same DERO.GetEncryptedBalance
	// RPC the probe uses, but for exactly the tracked SCIDs). Once a token
	// is tracked, listOwnedTokensFast reads it from the wallet Balance map
	// on every open — no gnomon index, no probe, no cache required, which
	// is what made tokens appear instantly on mobile/remote before.
	registerOwnedTokensWithWallet(owned)
}

// registerOwnedTokensWithWallet calls walletapi TokenAdd for each owned SCID
// so the wallet itself tracks and syncs those tokens. TokenAdd is idempotent
// (returns "token already added" for SCIDs already tracked), so re-registering
// discovered tokens on every page load is safe and cheap.
func registerOwnedTokensWithWallet(owned map[string]bool) {
	if engram.Disk == nil || len(owned) == 0 {
		return
	}
	for s := range owned {
		h := crypto.HashHexToHash(s)
		if h.IsZero() {
			continue
		}
		_ = engram.Disk.TokenAdd(h) // ignore "token already added"
	}
}

// ownedProbeOnce runs the full index-balance probe for the current wallet
// address exactly once at a time (singleflight). Concurrent callers during a
// probe are coalesced onto the in-flight probe's result, so the RPC fan-out is
// bounded to one probe even when page-load pre-warm and the picker poll run in
// the same session. Cache reads/writes bypass the lock.
var (
	ownedProbeMu    sync.Mutex
	ownedProbeBusy  bool
	ownedProbeAddr  string
	ownedProbeCache map[string]bool
)

func ownedProbeOnce(addr string) map[string]bool {
	ownedProbeMu.Lock()
	if ownedProbeBusy && ownedProbeAddr == addr {
		// Join the in-flight probe and reuse its result.
		ownedProbeMu.Unlock()
		for {
			ownedProbeMu.Lock()
			if !ownedProbeBusy {
				res := ownedProbeCache
				ownedProbeMu.Unlock()
				return res
			}
			ownedProbeMu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}
	}
	ownedProbeBusy = true
	ownedProbeAddr = addr
	ownedProbeCache = nil
	ownedProbeMu.Unlock()

	owned := probeOwnedTokenBalances(addr)

	ownedProbeMu.Lock()
	ownedProbeBusy = false
	ownedProbeCache = owned
	ownedProbeMu.Unlock()
	return owned
}

// listOwnedTokensFast returns the owned-SCID set from the wallet's own
// index-independent sources only: the wallet Balance map and the persisted
// per-wallet owned-SCID cache. It is instant (no bbolt enumeration, no daemon
// RPCs) and is what the pickers read. The expensive full-index probe is run
// separately by warmOwnedTokens() so this never blocks the UI.
func listOwnedTokensFast(addr string) map[string]bool {
	out := map[string]bool{}
	// 1) Wallet Balance map — includes tokens the wallet explicitly tracks.
	if acct := engram.Disk.GetAccount(); acct != nil && acct.Balance != nil {
		for scid, bal := range acct.Balance {
			if scid.IsZero() || bal == 0 {
				continue
			}
			out[strings.ToLower(scid.String())] = true
		}
	}
	// 2) Persisted owned-SCID cache — surfaced from a prior probe. Merged in
	// ALWAYS (not just when Balance is empty): a wallet always holds DERO in
	// its Balance map, so gating on len(out)==0 would discard every cached
	// token on wallets that also hold DERO.
	for s := range loadOwnedTokenCache(addr) {
		out[s] = true
	}
	return out
}

func listOwnedTokens() []crypto.Hash {
	if engram.Disk == nil {
		return nil
	}
	out := listOwnedTokensFast(engram.Disk.GetAddress().String())

	// NOTE: the expensive full-index balance probe (probeOwnedTokenBalances)
	// is deliberately NOT run here. On mobile/remote the candidate set is the
	// whole indexed SCID population (~50k), each one a daemon RPC — blocking
	// on it inside the picker's first render is what made the token list show
	// "Loading…" for 30-40s. The probe is instead triggered asynchronously by
	// warmOwnedTokens() (page load) and fills this same per-wallet cache; the
	// picker's lightweight poll re-reads the cache as it warms.

	var result []crypto.Hash
	for s := range out {
		h := crypto.HashHexToHash(s)
		if !h.IsZero() {
			result = append(result, h)
		}
	}
	return result
}

// allIndexedSCIDs returns the full index SCID population as an owner map,
// from whichever backing store is available (gnomon.BBolt directly, or the
// indexer's bbolt/grav backend). Returns nil if no index is up.
func allIndexedSCIDs() map[string]string {
	// Compat wrappers expose the civilware single-value shape; the inner
	// store returns (map, error). Use the compat handles directly.
	if gnomon.BBolt != nil {
		m := gnomon.BBolt.GetAllOwnersAndSCIDs()
		if len(m) > 0 {
			return m
		}
	}
	if gnomon.Index != nil {
		if gnomon.Index.BBSBackend != nil {
			m := gnomon.Index.BBSBackend.GetAllOwnersAndSCIDs()
			if len(m) > 0 {
				return m
			}
		}
		if gnomon.Index.GravDBBackend != nil {
			m := gnomon.Index.GravDBBackend.GetAllOwnersAndSCIDs()
			if len(m) > 0 {
				return m
			}
		}
	}
	return nil
}

// skipNonTokenClass reports whether a populated SCID class is a non-fungible
// or content store that is neither spendable as a token nor relevant to the
// SEND token picker. UNKNOWN/unclassified SCIDs (DST before the indexer
// classifies them) are deliberately kept.
func skipNonTokenClass(class string) bool {
	switch class {
	case "G45-NFT", "NFA", "G45-C", "TELA", "SWAP", "EPOCH":
		return true
	default:
		return false
	}
}

// probeOwnedTokenBalances enumerates the SCIDs this wallet may hold, then
// probes each one's decrypted balance concurrently and returns those with
// bal > 0. Discovery is balance-driven (matching the old assets page, which
// showed any SCID with a non-zero balance regardless of class) and class-agnostic
// for the token families the wallet actually touches.
//
// Tiered to avoid the remote-node regression: the cheap sources
// (tokenLikeSCIDs + EntriesNative + addressSCIDs) cover every token the wallet
// has ever tracked or touched via the index's per-address map. These are
// enumerated from the local bbolt store with zero daemon RPCs. Only when ALL
// cheap sources come back empty (a true first-run wallet that received an
// UNKNOWN-class token before the indexer classified it and before the address
// index recorded the touch) do we fall back to the full allIndexedSCIDs()
// population. On a remote node that enumeration is the entire indexed SCID set
// (~50k), and probing each is one daemon RPC — blocking on it inside the first
// render is what made the token list show "Loading…" for a minute+. Once any
// token is discovered it is registered with the wallet via TokenAdd, so every
// subsequent load reads from EntriesNative (the wallet's own persisted list)
// and the probe is never run again for that wallet.
func probeOwnedTokenBalances(addr string) map[string]bool {
	if engram.Disk == nil {
		return nil
	}
	zero := (&crypto.Hash{}).String()
	add := func(s string, m map[string]bool) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && s != zero {
			m[s] = true
		}
	}

	// Tier 1: cheap sources only. These are local-bbolt reads (no RPC) and
	// cover the common case — any token the wallet tracks or has touched.
	candidates := map[string]bool{}
	for _, s := range tokenLikeSCIDs() {
		add(s, candidates)
	}
	if acct := engram.Disk.GetAccount(); acct != nil && acct.EntriesNative != nil {
		for scid := range acct.EntriesNative {
			if !scid.IsZero() {
				add(scid.String(), candidates)
			}
		}
	}
	if t := addressSCIDs(addr); len(t) > 0 {
		for s := range t {
			add(s, candidates)
		}
	}

	// Tier 2: P1 fallback – when Tier 1 is empty (mobile/remote with empty
	// local bbolt on first run), do NOT enumerate the full 50k
	// allIndexedSCIDs() set – that is what made the picker show "Loading…"
	// for a minute on remote. Instead probe a tiny curated set (DST + last
	// selected asset) which is instant (≤5 RPCs) and covers the reported
	// case. Only if we have a modest local index (<5k) do we include it.
	if len(candidates) == 0 {
		for _, s := range knownFallbackSCIDs(addr) {
			add(s, candidates)
		}
		if len(candidates) == 0 {
			if idx := allIndexedSCIDs(); idx != nil && len(idx) > 0 && len(idx) < 5000 {
				for s := range idx {
					s = strings.ToLower(strings.TrimSpace(s))
					if s == "" || s == zero || skipNonTokenClass(getSCIDClass(s)) {
						continue
					}
					candidates[s] = true
				}
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		out  = map[string]bool{}
		sema = make(chan struct{}, tokenFetchConcurrency*4)
	)
	for scidStr := range candidates {
		wg.Add(1)
		go func(scidStr string) {
			defer wg.Done()
			sema <- struct{}{}
			defer func() { <-sema }()
			h := crypto.HashHexToHash(scidStr)
			if h.IsZero() {
				return
			}
			bal, _, err := engram.Disk.GetDecryptedBalanceAtTopoHeight(h, -1, addr)
			if err == nil && bal > 0 {
				mu.Lock()
				out[strings.ToLower(scidStr)] = true
				mu.Unlock()
			}
		}(scidStr)
	}
	wg.Wait()
	return out
}

func getAllTokenSCIDs() []crypto.Hash {
	if engram.Disk == nil {
		return nil
	}
	acct := engram.Disk.GetAccount()
	if acct == nil || acct.EntriesNative == nil {
		return nil
	}
	var out []crypto.Hash
	for scid := range acct.EntriesNative {
		if scid.IsZero() {
			continue
		}
		out = append(out, scid)
	}
	return out
}

func getSCIDForTXID(txid string) crypto.Hash {
	if engram.Disk == nil || txid == "" {
		return crypto.Hash{}
	}
	scid, _ := engram.Disk.Get_Payments_TXID(crypto.Hash{}, txid)
	return scid
}

var sendSelectedSCID crypto.Hash

func isSendToken() bool { return !sendSelectedSCID.IsZero() }

func selectedAssetKey(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" && engram.Disk != nil {
		addr = engram.Disk.GetAddress().String()
	}
	return "selected_asset_" + addr
}

func loadPersistedSelectedAsset() {
	if engram.Disk == nil {
		return
	}
	addr := engram.Disk.GetAddress().String()
	prev := sendSelectedSCID
	raw, err := GetEncryptedValue("settings", []byte(selectedAssetKey(addr)))
	if err != nil || len(raw) == 0 {
		// No persisted selection for this wallet — keep in-memory if it was
		// just set for this same wallet (dashboard → SEND race where store
		// hasn't flushed yet) and still valid; otherwise clear.
		if !prev.IsZero() {
			if bal, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(prev, -1, addr); bal > 0 {
				class := getSCIDClass(prev.String())
				if class == "" || class == "UNKNOWN" || IsTokenLikeClass(class) {
					return // keep just-set selection
				}
			}
		}
		sendSelectedSCID = crypto.Hash{}
		return
	}
	scidStr := strings.TrimSpace(string(raw))
	if scidStr == "" {
		if !prev.IsZero() {
			if bal, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(prev, -1, addr); bal > 0 {
				class := getSCIDClass(prev.String())
				if class == "" || class == "UNKNOWN" || IsTokenLikeClass(class) {
					return
				}
			}
		}
		sendSelectedSCID = crypto.Hash{}
		return
	}
	h := crypto.HashHexToHash(scidStr)
	if h.IsZero() {
		if !prev.IsZero() {
			if bal, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(prev, -1, addr); bal > 0 {
				class := getSCIDClass(prev.String())
				if class == "" || class == "UNKNOWN" || IsTokenLikeClass(class) {
					return
				}
			}
		}
		sendSelectedSCID = crypto.Hash{}
		return
	}
	// Only keep if still owned (balance>0) and token-like/unknown
	if bal, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(h, -1, addr); bal == 0 {
		// Persisted token no longer owned — clear but keep in-memory if it was just set and still valid
		if !prev.IsZero() && prev != h {
			if bal2, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(prev, -1, addr); bal2 > 0 {
				class := getSCIDClass(prev.String())
				if class == "" || class == "UNKNOWN" || IsTokenLikeClass(class) {
					return
				}
			}
		}
		sendSelectedSCID = crypto.Hash{}
		return
	}
	class := getSCIDClass(h.String())
	if class != "" && class != "UNKNOWN" && !IsTokenLikeClass(class) {
		if !prev.IsZero() && prev != h {
			if bal2, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(prev, -1, addr); bal2 > 0 {
				class2 := getSCIDClass(prev.String())
				if class2 == "" || class2 == "UNKNOWN" || IsTokenLikeClass(class2) {
					return
				}
			}
		}
		sendSelectedSCID = crypto.Hash{}
		return
	}
	sendSelectedSCID = h
}

func persistSelectedAsset(scid crypto.Hash) {
	if engram.Disk == nil {
		return
	}
	addr := engram.Disk.GetAddress().String()
	key := selectedAssetKey(addr)
	if scid.IsZero() {
		_ = DeleteKey("settings", []byte(key))
		sendSelectedSCID = scid
		return
	}
	_ = StoreEncryptedValue("settings", []byte(key), []byte(scid.String()))
	sendSelectedSCID = scid
}

func getSendSelectedDecimals() uint64 {
	if sendSelectedSCID.IsZero() {
		return 5 // DERO has 5 decimals (100000 atomic)
	}
	if d := getTokenDecimals(sendSelectedSCID); d != 0 {
		return d
	}
	// try store directly
	if _, _, d, ok := tokenMetadataFromStore(sendSelectedSCID.String()); ok && d != 0 {
		return d
	}
	return 0
}

func parseSendAmount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	if !isSendToken() {
		return globals.ParseAmount(s)
	}
	dec := getSendSelectedDecimals()
	return parseTokenAmount(s, dec)
}

func parseTokenAmount(s string, decimals uint64) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("amount cannot be negative")
	}
	parts := strings.SplitN(s, ".", 2)
	wholeStr := parts[0]
	if wholeStr == "" {
		wholeStr = "0"
	}
	whole, err := strconv.ParseUint(wholeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %v", err)
	}
	divisor := uint64(1)
	for i := uint64(0); i < decimals; i++ {
		divisor *= 10
	}
	if len(parts) == 1 {
		return whole * divisor, nil
	}
	fracStr := parts[1]
	if uint64(len(fracStr)) > decimals {
		return 0, fmt.Errorf("too many decimal places (max %d)", decimals)
	}
	for uint64(len(fracStr)) < decimals {
		fracStr += "0"
	}
	frac, err := strconv.ParseUint(fracStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid fractional amount: %v", err)
	}
	if whole > (^uint64(0)-frac)/divisor {
		return 0, fmt.Errorf("amount overflows")
	}
	return whole*divisor + frac, nil
}

func formatSendAmount(balance uint64) string {
	if sendSelectedSCID.IsZero() {
		return globals.FormatMoney(balance)
	}
	return formatTokenAmount(balance, getSendSelectedDecimals())
}

func getCachedTokenBalance(scid crypto.Hash) (uint64, bool) {
	if engram.Disk == nil || scid.IsZero() {
		return 0, false
	}
	if acct := engram.Disk.GetAccount(); acct != nil && acct.Balance != nil {
		if bal, ok := acct.Balance[scid]; ok {
			return bal, true
		}
	}
	return 0, false
}

func getCachedSendBalance() uint64 {
	if engram.Disk == nil {
		return 0
	}
	if sendSelectedSCID.IsZero() {
		b, _ := engram.Disk.Get_Balance()
		return b
	}
	if bal, ok := getCachedTokenBalance(sendSelectedSCID); ok {
		return bal
	}
	return 0
}

func getSendBalance() uint64 {
	if engram.Disk == nil {
		return 0
	}
	if sendSelectedSCID.IsZero() {
		b, _ := engram.Disk.Get_Balance()
		return b
	}
	if bal, ok := getCachedTokenBalance(sendSelectedSCID); ok {
		return bal
	}
	bal, _, _ := engram.Disk.GetDecryptedBalanceAtTopoHeight(sendSelectedSCID, -1, engram.Disk.GetAddress().String())
	return bal
}

func sendSelectedLabel() string {
	if sendSelectedSCID.IsZero() {
		return "DERO"
	}
	lbl := resolveTokenDisplayName(sendSelectedSCID)
	if lbl == sendSelectedSCID.String()[:8]+"…" {
		return lbl
	}
	return lbl
}
