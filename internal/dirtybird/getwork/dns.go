package getwork

import (
	"context"
	"net"
	"time"
)

// Termux/Android has no /etc/resolv.conf, so Go's built-in resolver falls
// back to localhost:53 and every hostname lookup fails there (the Rust miner
// hit the same wall with musl and had to ship a bionic build; pure Go instead
// retries over well-known public resolvers). Desktops never reach the
// fallbacks: the system lookup answers first.
var fallbackDNS = []string{"1.1.1.1:53", "8.8.8.8:53"}

// Per-lookup budget. Three resolvers run inside the websocket's 10s
// handshake budget, so this must satisfy 3*lookupTimeout < handshake with
// room left for TCP+TLS+upgrade — at 5s the third resolver was born expired
// whenever the first two were black-holed.
const lookupTimeout = 3 * time.Second

type lookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func defaultLookups() []lookupFunc {
	lookups := []lookupFunc{net.DefaultResolver.LookupIPAddr}
	for _, server := range fallbackDNS {
		lookups = append(lookups, fallbackLookup(server))
	}
	return lookups
}

// fallbackDial returns the resolver dial hook: it redirects every query to
// the given DNS server while honoring the requested network — the resolver
// retries over "tcp" after a truncated UDP answer, and forcing "udp" here
// would make large RRsets unresolvable through the fallbacks.
func fallbackDial(server string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, server)
	}
}

// fallbackLookup returns a lookup backed by a pure-Go resolver that dials the
// given DNS server directly, bypassing /etc/resolv.conf.
func fallbackLookup(server string) lookupFunc {
	r := &net.Resolver{
		PreferGo: true,
		Dial:     fallbackDial(server),
	}
	return r.LookupIPAddr
}

// resolveHost tries each lookup in order and returns the first success. On
// total failure the first error — the system resolver's — comes back, as the
// most diagnostic one.
func resolveHost(ctx context.Context, host string, lookups []lookupFunc) ([]net.IPAddr, error) {
	var firstErr error
	for _, lookup := range lookups {
		lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		addrs, err := lookup(lctx, host)
		cancel()
		if err == nil && len(addrs) > 0 {
			return addrs, nil
		}
		if firstErr == nil && err != nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	if firstErr == nil {
		firstErr = &net.DNSError{Err: "no addresses", Name: host, IsNotFound: true}
	}
	return nil, firstErr
}

// joinIPPort formats one resolved address for dialing. IPAddr.String keeps
// the IPv6 zone (fe80::1%eth0) that ip.IP.String would drop — link-local
// endpoints stopped working without it.
func joinIPPort(ip net.IPAddr, port string) string {
	return net.JoinHostPort(ip.String(), port)
}

// dialContextDNSFallback is the websocket dialer's NetDialContext: IP-literal
// hosts dial straight through; hostnames resolve via the system resolver
// first, then the public fallbacks, and each address is tried until one
// connects. TLS still happens above this dial (the daemon's certificate is
// self-signed and unverified anyway, so the hostname is immaterial).
func dialContextDNSFallback(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	if net.ParseIP(host) != nil {
		return d.DialContext(ctx, network, addr)
	}
	addrs, err := resolveHost(ctx, host, defaultLookups())
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, ip := range addrs {
		conn, err := d.DialContext(ctx, network, joinIPPort(ip, port))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, firstErr
}
