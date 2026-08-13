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
// design: it only acts on port-44326 sockets, only when the site exposes
// connectWallet() / isConnected, and is a no-op otherwise. An unexpected-close
// latch distinguishes a deliberate user disconnect from an Android kill.
const telaMobileWSReconnectShim = `/*__engramWsReconnect__*/
(function () {
  if (window.__engramWsReconnect) return;
  window.__engramWsReconnect = true;

  var ENG_WS_PORT = ':44326';
  var unexpectedClose = false;

  function isEngramSocket(s) {
    try {
      return !!s && !!s.url && s.url.indexOf(ENG_WS_PORT) !== -1;
    } catch (e) { return false; }
  }

  function canAutoConnect() {
    return typeof window.connectWallet === 'function' &&
           typeof window.isConnected === 'function';
  }

  function tryReconnect() {
    if (!unexpectedClose || !canAutoConnect()) return;
    try {
      if (window.isConnected()) { unexpectedClose = false; return; }
      window.connectWallet();
    } catch (e) {}
  }

  // Latch unexpected closes on the Engram socket. A deliberate close() from the
  // site (user disconnect) clears the latch so we don't fight the user.
  var _close = WebSocket.prototype.close;
  WebSocket.prototype.close = function () {
    if (isEngramSocket(this)) unexpectedClose = false;
    return _close.apply(this, arguments);
  };

  // Observe close events on Engram sockets via addEventListener, which is what
  // most TELA apps use for their reconnect-on-close handler.
  var _add = EventTarget.prototype.addEventListener;
  EventTarget.prototype.addEventListener = function (type, fn, opts) {
    if (type === 'close' && isEngramSocket(this)) {
      var orig = fn;
      fn = function (e) {
        unexpectedClose = true;
        return orig ? orig.apply(this, arguments) : undefined;
      };
    }
    return _add.call(this, type, fn, opts);
  };

  function onVisible() {
    if (document.visibilityState === 'visible') setTimeout(tryReconnect, 150);
  }
  document.addEventListener('visibilitychange', onVisible);
  window.addEventListener('focus', function () { setTimeout(tryReconnect, 150); });
  setInterval(tryReconnect, 2000);
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
