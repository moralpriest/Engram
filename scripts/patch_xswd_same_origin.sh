#!/bin/bash
# Patches the derohe xswd library with two fixes TELA apps need:
#
# 1. Same-origin reconnect: Hologram allows reconnect by origin
#    (xswd_permissions.go:784 XSWDOriginKey); Engram's derohe library rejects
#    any duplicate ID, breaking TELA retries. This patch evicts a stale
#    connection only when ID+origin match, preserving cross-origin squatting
#    protection.
#
# 2. Response-id echo: jrpc2 stores the request id as raw JSON and
#    Request.ID() returns it verbatim, so echoing it back through a string
#    field double-encodes string ids (`"\"1\""`). TELA apps (deaddrop) key
#    pending requests by the plain string id, never match the double-quoted
#    echo, and silently drop every response — the registry appears to never
#    load until the app's 30s timeout fires. Echoing the raw id bytes keeps
#    string and number ids identical to what the client sent.
#
# Applied to the derohe module directory resolved via `go list -m` (which
# honors go.mod's replace directive), plus any fork checkout given in
# DEROHE_FORK_DIR, so the fix survives `go clean -modcache` and can be
# committed upstream.
#
# The module cache is normally read-only; go re-verifies checksums on build,
# so this must be re-applied after `go clean -modcache` / a fresh clone.
set -e

TARGETS=()
if [ -n "${DEROHE_FORK_DIR:-}" ] && [ -f "$DEROHE_FORK_DIR/walletapi/xswd/xswd.go" ]; then
  TARGETS+=("$DEROHE_FORK_DIR/walletapi/xswd/xswd.go")
fi
DEROHE_DIR="$(go list -m -f '{{.Dir}}' github.com/deroproject/derohe 2>/dev/null || true)"
if [ -n "$DEROHE_DIR" ] && [ -f "$DEROHE_DIR/walletapi/xswd/xswd.go" ]; then
  TARGETS+=("$DEROHE_DIR/walletapi/xswd/xswd.go")
fi
if [ "${#TARGETS[@]}" -eq 0 ]; then
  echo "skip (no derohe xswd found; run inside the Engram module or set DEROHE_FORK_DIR)"
  exit 0
fi

for XSWD in "${TARGETS[@]}"; do
  if [ ! -f "$XSWD" ]; then
    echo "skip (not found): $XSWD"
    continue
  fi
  chmod u+w "$XSWD" 2>/dev/null || true
  XSWD_PATH="$XSWD" python3 << 'PY'
import os, pathlib
path = os.environ["XSWD_PATH"]
s = pathlib.Path(path).read_text()
changed = False

if "Replaced stale connection with same ID+origin" not in s:
    old = """\tif x.HasApplicationId(app_data.Id) {
\t\tx.logger.Info("App ID is already used", "ID", app_data.Name)
\t\tconn.WriteJSON(AuthorizationResponse{
\t\t\tMessage:  "App ID is already used",
\t\t\tAccepted: false,
\t\t})

\t\treturn
\t}"""
    new = """\tif x.HasApplicationId(app_data.Id) {
\t\t// Same-origin reconnect: allow a TELA app to replace its own stale
\t\t// WebSocket when the browser kills the old one on app-switch (Android)
\t\t// or manual retry. This is safe because we compare the origin string
\t\t// (Url) that is validated against the HTTP Origin header in addApplication.
\t\t// Cross-origin ID squatting remains rejected.
\t\tnewOrigin := strings.TrimSpace(app_data.Url)
\t\tif newOrigin == "" {
\t\t\tnewOrigin = strings.TrimSpace(app_data.Name)
\t\t}
\t\tvar sameOrigin bool
\t\tvar existingApp *ApplicationData
\t\tx.Lock()
\t\tfor _, a := range x.applications {
\t\t\tif strings.EqualFold(a.Id, app_data.Id) {
\t\t\t\toldOrigin := strings.TrimSpace(a.Url)
\t\t\t\tif oldOrigin == "" {
\t\t\t\t\toldOrigin = strings.TrimSpace(a.Name)
\t\t\t\t}
\t\t\t\tif newOrigin != "" && strings.EqualFold(oldOrigin, newOrigin) {
\t\t\t\t\tsameOrigin = true
\t\t\t\t\tcopyA := a
\t\t\t\t\texistingApp = &copyA
\t\t\t\t}
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tx.Unlock()
\t\tif sameOrigin && existingApp != nil {
\t\t\tx.RemoveApplication(existingApp)
\t\t\tx.logger.Info("Replaced stale connection with same ID+origin", "ID", app_data.Id, "origin", newOrigin)
\t\t} else {
\t\t\tx.logger.Info("App ID is already used", "ID", app_data.Name)
\t\t\tconn.WriteJSON(AuthorizationResponse{
\t\t\t\tMessage:  "App ID is already used",
\t\t\t\tAccepted: false,
\t\t\t})

\t\t\treturn
\t\t}
\t}"""
    if old not in s:
        print("  same-origin pattern not found - manual check needed:", path)
        exit(1)
    s = s.replace(old, new)
    changed = True
    print("  patched same-origin reconnect")

if "rawRequestID" not in s:
    old_resp = """type RPCResponse struct {
\tJsonRPC string      `json:"jsonrpc"`
\tID      string      `json:"id"`
\tResult  interface{} `json:"result,omitempty"`
\tError   interface{} `json:"error,omitempty"`
}

func ResponseWithError(request *jrpc2.Request, err *jrpc2.Error) RPCResponse {
\tvar id string
\tif request != nil {
\t\tid = request.ID()
\t}

\treturn RPCResponse{
\t\tJsonRPC: "2.0",
\t\tID:      id,
\t\tError:   err,
\t}
}

func ResponseWithResult(request *jrpc2.Request, result interface{}) RPCResponse {
\tvar id string
\tif request != nil {
\t\tid = request.ID()
\t}

\treturn RPCResponse{
\t\tJsonRPC: "2.0",
\t\tID:      id,
\t\tResult:  result,
\t}
}"""
    new_resp = """type RPCResponse struct {
\tJsonRPC string          `json:"jsonrpc"`
\tID      json.RawMessage `json:"id,omitempty"`
\tResult  interface{}     `json:"result,omitempty"`
\tError   interface{}     `json:"error,omitempty"`
}

// rawRequestID returns the request id in its original JSON form (e.g. `"1"`
// for a string id, `1` for a number). jrpc2 stores the id as raw JSON and
// Request.ID() returns it verbatim, so echoing it back through a string field
// double-encodes string ids (`"\\"1\\""`), which TELA apps never match when
// correlating responses. Echoing the raw bytes keeps string and number ids
// identical to what the client sent.
func rawRequestID(request *jrpc2.Request) json.RawMessage {
\tif request == nil {
\t\treturn nil
\t}
\traw := request.ID()
\tif raw == "" {
\t\treturn nil
\t}
\treturn json.RawMessage(raw)
}

func ResponseWithError(request *jrpc2.Request, err *jrpc2.Error) RPCResponse {
\treturn RPCResponse{
\t\tJsonRPC: "2.0",
\t\tID:      rawRequestID(request),
\t\tError:   err,
\t}
}

func ResponseWithResult(request *jrpc2.Request, result interface{}) RPCResponse {
\treturn RPCResponse{
\t\tJsonRPC: "2.0",
\t\tID:      rawRequestID(request),
\t\tResult:  result,
\t}
}"""
    if old_resp not in s:
        print("  RPCResponse pattern not found - manual check needed:", path)
        exit(1)
    s = s.replace(old_resp, new_resp)
    changed = True
    print("  patched response-id echo")

if changed:
    pathlib.Path(path).write_text(s)
else:
    print("  already patched")
PY
done
echo "Done"
