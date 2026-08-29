package main

// Live integration test for the XSWD flow TELA apps (deaddrop) depend on.
//
// It opens a real wallet exactly like login() does, starts the XSWD server
// with the SAME handlers Engram wires (custom DERO.GetSC + Gnomon methods),
// and drives the deaddrop protocol over a real WebSocket:
//
//	1. handshake {id,name,description,url}  -> accepted:true
//	2. DERO.GetSC (registry)                -> 305 stringkeys AND the response
//	   id must echo the request id exactly. The derohe xswd server used to
//	   echo string ids double-quoted (`"id":"\"1\""`); TELA apps key pending
//	   requests by the plain id and silently drop the response, so the
//	   registry appeared to never load until the app's 30s timeout fired.
//	3. close + reconnect with the SAME app id (same-origin eviction) -> the
//	   new socket is accepted and DERO.GetSC works again.
//
// Gated behind ENGRAM_XSWD_LIVE=1. Run with:
//
//	ENGRAM_XSWD_LIVE=1 ENGRAM_TEST_WALLET=<path> ENGRAM_TEST_WALLET_PASS=<pw> \
//	  go test -tags migrated_fynedo -count=1 -run TestXSWDDeaddropLive -v -timeout 120s .
//
// SECURITY: wallet path and password come from the environment only; there
// are no defaults.

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/handler"
	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/xswd"
	"github.com/gorilla/websocket"
)

const testRegistrySCID = "9381ff5d7763835352eb6c036bc365968c121e9bca3fab990791fc73b1bfb0e6"

type xswdClient struct {
	ws  *websocket.Conn
	seq int
}

func newXSWDClient(t *testing.T, url, origin, appID string) *xswdClient {
	t.Helper()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	h := map[string][]string{"Origin": {origin}}
	ws, _, err := dialer.Dial(url, h)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	c := &xswdClient{ws: ws}
	app := map[string]interface{}{
		"id":          appID,
		"name":        "DEADDROP",
		"description": "On-chain archive on DERO",
		"url":         origin,
	}
	if err := ws.WriteJSON(app); err != nil {
		t.Fatalf("handshake write: %v", err)
	}
	ws.SetReadDeadline(time.Now().Add(8 * time.Second))
	var auth struct {
		Message  string `json:"message"`
		Accepted bool   `json:"accepted"`
	}
	if err := ws.ReadJSON(&auth); err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	t.Logf("handshake accepted=%v msg=%q", auth.Accepted, auth.Message)
	if !auth.Accepted {
		t.Fatalf("connection rejected: %s", auth.Message)
	}
	return c
}

// callRaw sends an RPC and returns the raw response object so the id echo can
// be asserted byte-for-byte.
func (c *xswdClient) callRaw(t *testing.T, method string, params map[string]interface{}) map[string]interface{} {
	t.Helper()
	c.seq++
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("%d", c.seq),
		"method":  method,
	}
	if len(params) > 0 {
		req["params"] = params
	}
	c.ws.SetWriteDeadline(time.Now().Add(8 * time.Second))
	if err := c.ws.WriteJSON(req); err != nil {
		t.Fatalf("send %s: %v", method, err)
	}
	c.ws.SetReadDeadline(time.Now().Add(40 * time.Second))
	var resp map[string]interface{}
	if err := c.ws.ReadJSON(&resp); err != nil {
		t.Fatalf("read %s response: %v", method, err)
	}
	return resp
}

func (c *xswdClient) close() {
	if c.ws != nil {
		_ = c.ws.Close()
		c.ws = nil
	}
}

func TestXSWDDeaddropLive(t *testing.T) {
	if os.Getenv("ENGRAM_XSWD_LIVE") != "1" {
		t.Skip("set ENGRAM_XSWD_LIVE=1 to run the live XSWD deaddrop test")
	}

	walletPath := os.Getenv("ENGRAM_TEST_WALLET")
	walletPass := os.Getenv("ENGRAM_TEST_WALLET_PASS")
	if walletPath == "" || walletPass == "" {
		t.Fatal("ENGRAM_TEST_WALLET and ENGRAM_TEST_WALLET_PASS are required")
	}
	daemon := os.Getenv("ENGRAM_TEST_DAEMON")
	if daemon == "" {
		daemon = "127.0.0.1:10102"
	}

	wd, err := walletapi.Open_Encrypted_Wallet(walletPath, walletPass)
	if err != nil {
		t.Fatalf("wallet open: %v", err)
	}
	t.Logf("wallet opened: %s", wd.GetAddress().String()[:24])

	// Mirror EnsureXSWD wiring: same custom methods, auto-approving prompt.
	// Pick a free ephemeral port so the test never collides with a running
	// Hologram / Engram XSWD server (which also likes 44326).
	tmpL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := tmpL.Addr().(*net.TCPAddr).Port
	_ = tmpL.Close()
	server := xswd.NewXSWDServerWithPort(port, wd, false, engramNoStoreMethods(),
		func(ad *xswd.ApplicationData) bool { return true },
		func(ad *xswd.ApplicationData, r *jrpc2.Request) xswd.Permission { return xswd.Allow })
	if server == nil {
		t.Fatal("NewXSWDServerWithPort returned nil")
	}
	defer server.Stop()

	for method, h := range EngramHandler {
		server.SetCustomMethod(method, h)
	}
	server.SetCustomMethod("HandleTELALinks", handler.New(HandleTELALinks))

	// Point the custom DERO.GetSC handler at the local daemon.
	session.Daemon = daemon

	// The constructor starts ListenAndServe in a goroutine; give it a moment
	// to bind and fail loudly if it never does.
	deadline := time.Now().Add(5 * time.Second)
	up := false
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			up = true
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !up {
		t.Fatalf("XSWD server never bound :%d (IsRunning=%v)", port, server.IsRunning())
	}

	origin := "http://localhost:8082"
	url := fmt.Sprintf("ws://localhost:%d/xswd", port)
	appID := strings.Repeat("ab", 32) // 64-hex-char stable app id (localStorage)

	// Hold mode: keep the server alive so an external browser can be driven
	// against it (ENGRAM_XSWD_HOLD=1) — skip the assertion legs entirely.
	if os.Getenv("ENGRAM_XSWD_HOLD") == "1" {
		t.Logf("HOLD: XSWD server on :%d kept alive for external driver", port)
		time.Sleep(10 * time.Minute)
		return
	}

	// --- Leg 1: fresh connect + registry read with exact id echo ---
	c1 := newXSWDClient(t, url, origin, appID)
	res1 := c1.callRaw(t, "DERO.GetSC", map[string]interface{}{
		"scid": testRegistrySCID, "code": false, "variables": true, "topoheight": -1,
	})
	if e, ok := res1["error"]; ok {
		t.Fatalf("DERO.GetSC leg1 error: %v", e)
	}
	// The response id must be the plain string "1" — the derohe server used to
	// echo it double-quoted (`"1"`), which TELA apps never match.
	idRaw, _ := json.Marshal(res1["id"])
	if string(idRaw) != `"1"` {
		t.Fatalf("leg1 response id echo = %s, want \"1\" (double-quoted ids break TELA apps)", idRaw)
	}
	result1, _ := res1["result"].(map[string]interface{})
	sk, _ := result1["stringkeys"].(map[string]interface{})
	t.Logf("leg1 DERO.GetSC stringkeys=%d id-echo=%s", len(sk), idRaw)
	if len(sk) < 100 {
		t.Fatalf("leg1 stringkeys unexpectedly small: %d", len(sk))
	}

	// --- Leg 2: close + reconnect with same app id (same-origin eviction) ---
	c1.close()
	c2 := newXSWDClient(t, url, origin, appID)
	res2 := c2.callRaw(t, "DERO.GetSC", map[string]interface{}{
		"scid": testRegistrySCID, "code": false, "variables": true, "topoheight": -1,
	})
	if e, ok := res2["error"]; ok {
		t.Fatalf("DERO.GetSC leg2 error: %v", e)
	}
	idRaw2, _ := json.Marshal(res2["id"])
	if string(idRaw2) != `"1"` {
		t.Fatalf("leg2 response id echo = %s, want \"1\"", idRaw2)
	}
	result2, _ := res2["result"].(map[string]interface{})
	sk2, _ := result2["stringkeys"].(map[string]interface{})
	t.Logf("leg2 (reconnect same id) DERO.GetSC stringkeys=%d", len(sk2))
	if len(sk2) < 100 {
		t.Fatalf("leg2 stringkeys unexpectedly small: %d", len(sk2))
	}
	c2.close()
}
