package main

import "bytes"

// telaMobileWSReconnectShim is a small JS IIFE injected into cloned TELA app
// .js files on mobile. On Android the TELA site runs in the external browser,
// which kills the site's WebSocket to Engram whenever the user switches to
// Engram to approve a permission prompt. The site then needs a manual reconnect
// (press Enter), and each reconnect re-triggers the connection dialog.
//
// The shim auto-reconnects the XSWD socket (port 44326) when the tab becomes
// visible again (focus / visibilitychange / interval probes), so the permission
// flow advances without a manual Enter per permission. It is gated and safe by
// design: it only acts on port-44326 sockets and is a no-op if the site does
// not expose a connect function.
//
// TELA apps expose different connect APIs, so the shim resolves the function
// defensively: it prefers window.connectWallet (the de-facto XSWD convention)
// and falls back to window.connectWebSocket, which the Engram villager app
// exposes. Connection state is read from window.isConnected when the site
// provides it, otherwise from the socket's readyState (window.socket or the
// bare `socket` global, which classic scripts expose to later code). An
// unexpected-close latch distinguishes a deliberate user disconnect (site calls
// socket.close()) from an Android kill: closes that come through a wrapped
// addEventListener('close', ...) or an OPEN->CLOSED transition observed by the
// probe are treated as unexpected, while a recent deliberate close() suppresses
// the latch so the shim never fights the user's own disconnect.
const telaMobileWSReconnectShim = `/*__engramWsReconnect__*/
(function () {
  if (window.__engramWsReconnect) return;
  window.__engramWsReconnect = true;

  var ENG_WS_PORT = ':44326';
  var unexpectedClose = false;
  var lastDeliberateClose = 0;
  var lastState = null;
  var reconnectScheduled = false;
  var pendingSends = [];

  function isEngramSocket(s) {
    try {
      return !!s && !!s.url && s.url.indexOf(ENG_WS_PORT) !== -1;
    } catch (e) { return false; }
  }

  function currentSocket() {
    try {
      if (window.socket) return window.socket;
    } catch (e) {}
    try {
      if (typeof socket !== 'undefined' && socket) return socket;
    } catch (e) {}
    return null;
  }

  function socketIsOpen() {
    var s = currentSocket();
    return !!s && s.readyState === WebSocket.OPEN;
  }

  function socketIsConnecting() {
    var s = currentSocket();
    return !!s && s.readyState === WebSocket.CONNECTING;
  }

  function isConnected() {
    try {
      if (typeof window.isConnected === 'function') return window.isConnected();
    } catch (e) {}
    return socketIsOpen() || socketIsConnecting();
  }

  function getConnectFn() {
    try {
      if (typeof window.connectWallet === 'function') return window.connectWallet;
    } catch (e) {}
    try {
      if (typeof window.connectWebSocket === 'function') return window.connectWebSocket;
    } catch (e) {}
    return null;
  }

  function canAutoConnect() {
    return getConnectFn() !== null;
  }

  function scheduleReconnect(delay) {
    if (reconnectScheduled) return;
    reconnectScheduled = true;
    setTimeout(function () {
      reconnectScheduled = false;
      tryReconnect();
    }, delay || 80);
  }

  function flushPending() {
    if (!pendingSends.length) return;
    var s = currentSocket();
    if (!s || s.readyState !== WebSocket.OPEN) return;
    try {
      for (var i = 0; i < pendingSends.length; i++) s.send(pendingSends[i]);
    } catch (e) {}
    pendingSends = [];
  }

  function tryReconnect() {
    if (!unexpectedClose || !canAutoConnect()) return;
    try {
      if (isConnected()) { unexpectedClose = false; flushPending(); return; }
      getConnectFn()();
      // Flush will happen via onopen wrapper (immediate) – no 1200ms delay.
      // Keep latch until OPEN so probe doesn't clear it prematurely.
    } catch (e) {}
  }

  // Queue sends that occur while the Engram socket is not OPEN (e.g. the
  // app sent GetBalance while Engram was foreground showing the GetAddress
  // permission dialog and the WS was killed). When the shim reconnects, the
  // queue is flushed, so the app's sequential logic does not need a manual
  // retry or app-switch per permission.
  var _send = WebSocket.prototype.send;
  WebSocket.prototype.send = function (data) {
    if (isEngramSocket(this) && this.readyState !== WebSocket.OPEN) {
      try { pendingSends.push(data); } catch (e) {}
      // Trigger a reconnect attempt; tryReconnect will flush on OPEN. Do not
      // re-latch inside the deliberate-close window, or we would fight the
      // site's own disconnect (user tapped disconnect, app still sends).
      if (Date.now() - lastDeliberateClose > 2000) {
        unexpectedClose = true;
        scheduleReconnect(80);
      }
      return;
    }
    return _send.apply(this, arguments);
  };

  // Latch unexpected closes on the Engram socket. A deliberate close() from the
  // site (user disconnect) clears the latch and stamps a timestamp so the probe
  // below does not immediately re-latch the same close.
  var _close = WebSocket.prototype.close;
  WebSocket.prototype.close = function () {
    if (isEngramSocket(this)) {
      unexpectedClose = false;
      lastDeliberateClose = Date.now();
    }
    return _close.apply(this, arguments);
  };

  // Observe close events on Engram sockets. Many TELA apps (including villager)
  // use socket.onclose = fn or addEventListener('close'), both of which fire
  // when Android kills the WS on app-switch. We latch unexpectedClose and
  // trigger a fast reconnect. Unlike v0.6.9 which required manual Enter, the
  // shim reconnects silently; we still call the app's handler so its retry
  // logic (if any) can resend pending RPCs that were lost when the WS died
  // while a permission dialog was showing in Engram.
  var _add = EventTarget.prototype.addEventListener;
  EventTarget.prototype.addEventListener = function (type, fn, opts) {
    if (type === 'close' && isEngramSocket(this)) {
      var orig = fn;
      fn = function (e) {
        var isDeliberate = Date.now() - lastDeliberateClose < 2000;
        if (!isDeliberate && lastState === WebSocket.OPEN) {
          unexpectedClose = true;
          scheduleReconnect(80);
        }
        return orig ? orig.apply(this, arguments) : undefined;
      };
    } else if (type === 'open' && isEngramSocket(this)) {
      var origO = fn;
      fn = function (e) {
        try { unexpectedClose = false; flushPending(); } catch (ex) {}
        return origO ? origO.apply(this, arguments) : undefined;
      };
    }
    return _add.call(this, type, fn, opts);
  };

  // Also wrap onclose setter (villager uses socket.onclose = ...) for same reason.
  try {
    var desc = Object.getOwnPropertyDescriptor(WebSocket.prototype, 'onclose');
    if (desc && desc.set) {
      var origSet = desc.set;
      Object.defineProperty(WebSocket.prototype, 'onclose', {
        set: function (fn) {
          if (isEngramSocket(this) && typeof fn === 'function') {
            var orig = fn;
            fn = function (e) {
              var isDeliberate = Date.now() - lastDeliberateClose < 2000;
              if (!isDeliberate && lastState === WebSocket.OPEN) {
                unexpectedClose = true;
                scheduleReconnect(80);
              }
              return orig.apply(this, arguments);
            };
          }
          return origSet.call(this, fn);
        },
        get: desc.get,
        configurable: true
      });
    }
  } catch (e) {}

  // Wrap onopen to flush queued RPCs immediately when socket opens (fixes
  // WORSEVILLAGER 1200ms flush delay that lost requests after Allow).
  try {
    var descO = Object.getOwnPropertyDescriptor(WebSocket.prototype, 'onopen');
    if (descO && descO.set) {
      var origSetO = descO.set;
      Object.defineProperty(WebSocket.prototype, 'onopen', {
        set: function (fn) {
          if (isEngramSocket(this) && typeof fn === 'function') {
            var orig = fn;
            fn = function (e) {
              try { unexpectedClose = false; flushPending(); } catch (ex) {}
              return orig.apply(this, arguments);
            };
          }
          return origSetO.call(this, fn);
        },
        get: descO.get,
        configurable: true
      });
    }
  } catch (e) {}

  // Probe the socket state every tick: an OPEN -> CLOSED transition that was
  // not a deliberate close() (e.g. Android killed the WS while the tab was
  // backgrounded) latches unexpectedClose so tryReconnect fires. Faster 500ms
  // probe reduces the blackout after returning from Engram permission dialog.
  function probe() {
    if (!canAutoConnect()) return;
    try {
      var s = currentSocket();
      if (!s) { lastState = null; return; }
      var st = s.readyState;
      if (st === WebSocket.OPEN || st === WebSocket.CONNECTING) {
        unexpectedClose = false;
        lastState = st;
      } else if (st === WebSocket.CLOSED) {
        if (lastState === WebSocket.OPEN && Date.now() - lastDeliberateClose > 2000) {
          unexpectedClose = true;
          scheduleReconnect(80);
        }
        lastState = st;
      }
    } catch (e) {}
  }

   function onVisible() {
     if (document.visibilityState === 'visible') scheduleReconnect(80);
   }
   document.addEventListener('visibilitychange', onVisible);
   window.addEventListener('focus', function () { scheduleReconnect(80); });
   window.addEventListener('pageshow', function () { scheduleReconnect(80); });
   // Probe interval: 500ms balances fast reconnect (Android permission dialog)
   // vs battery. Tune here in the shim string; a rebuild is required either way.
   setInterval(function () { probe(); tryReconnect(); }, 500);
 })();
 /*__engramWsReconnect__*/`

// shimMarker delimits a previously injected telaMobileWSReconnectShim block so
// upgrades strip the old copy before injecting the new one (never stacking).
var shimMarker = []byte("/*__engramWsReconnect__*/")

// stripMobileWSReconnectShim removes any previously-injected reconnect shim
// from content, returning the content without it (idempotent upgrades).
func stripMobileWSReconnectShim(content []byte) []byte {
	for {
		start := bytes.Index(content, shimMarker)
		if start < 0 {
			return content
		}
		end := bytes.Index(content[start+len(shimMarker):], shimMarker)
		if end < 0 {
			return content
		}
		end += start + len(shimMarker) + len(shimMarker)
		content = append(content[:start], content[end:]...)
	}
}

// DeroBeats sw.js uses `new Request("ipfs://" + cid)` as a Cache API key.
// Chromium rejects non-http(s) schemes on Cache.put, so song/artwork fetches
// throw even after a gateway hit. Rewrite to an https URL; match/put share
// the same key so hits still work.
var (
	ipfsSchemeCacheRequest = []byte(`new Request("ipfs://" + cid)`)
	ipfsHTTPCacheRequest   = []byte(`new Request("https://ipfs.io/ipfs/" + cid)`)
)

func patchIpfsSchemeCacheKey(content []byte) ([]byte, bool) {
	if !bytes.Contains(content, ipfsSchemeCacheRequest) {
		return content, false
	}
	return bytes.ReplaceAll(content, ipfsSchemeCacheRequest, ipfsHTTPCacheRequest), true
}

// injectMobileWSReconnectShim strips any previous shim and appends a fresh copy
// at the end of the file, returning the updated content and whether it changed.
func injectMobileWSReconnectShim(content []byte) ([]byte, bool) {
	clean := stripMobileWSReconnectShim(content)
	if bytes.Contains(clean, []byte("__engramWsReconnect")) {
		return clean, false
	}
	out := make([]byte, 0, len(clean)+len(telaMobileWSReconnectShim)+2)
	out = append(out, clean...)
	out = append(out, '\n')
	out = append(out, telaMobileWSReconnectShim...)
	out = append(out, '\n')
	return out, true
}
