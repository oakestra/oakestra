package source

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// ErrBlockedAddress is returned when a source resolves to an address that is not allowed.
var ErrBlockedAddress = errors.New("address is not allowed")

// blockedPrefixes are ranges a server-side fetch must not reach unless the source explicitly
// allows private addresses. netip's IsPrivate, IsLoopback and friends cover most of them; this
// list adds the ones they don't.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network", 0.0.0.0 reaches localhost on Linux
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT, often used for internal overlays (Tailscale)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, includes 255.255.255.255
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local
	netip.MustParsePrefix("::/96"),           // deprecated IPv4-compatible addresses
	netip.MustParsePrefix("::ffff:0:0:0/96"), // IPv4-translated (SIIT)
}

var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
)

// isBlocked reports whether a fetch must not connect to ip. IPv6 forms that embed an IPv4
// address (mapped, NAT64, 6to4) are judged by the embedded address, so ::ffff:127.0.0.1 or
// 64:ff9b::a9fe:a9fe (169.254.169.254) don't slip through.
func isBlocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Is6() {
		b := ip.As16()
		switch {
		case nat64Prefix.Contains(ip):
			return isBlocked(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case sixToFour.Contains(ip):
			return isBlocked(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Resolver looks up host addresses. *net.Resolver implements it; tests substitute their own.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// DialFunc dials a resolved address. Tests replace it to route "public" addresses to a local
// test server.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// guardedDial resolves the host itself and checks every address before connecting. Checking
// at dial time (rather than once when the source is added) is what defeats DNS rebinding: the
// address that is checked is the address that is dialed, on every connection, including the
// ones redirects open.
func guardedDial(resolver Resolver, dial DialFunc, allowPrivate bool) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var ips []netip.Addr
		if ip, err := netip.ParseAddr(host); err == nil {
			ips = []netip.Addr{ip}
		} else {
			ips, err = resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for %q", host)
		}
		// Reject the host if any address is blocked rather than skipping to an allowed one. A
		// name that mixes public and internal addresses is suspicious, and the strict rule is
		// easier to reason about.
		if !allowPrivate {
			for _, ip := range ips {
				if isBlocked(ip) {
					return nil, fmt.Errorf("%w: %q resolves to %s, which is a private or reserved address (set allow_private to permit it)", ErrBlockedAddress, host, ip)
				}
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// newTransport builds the transport every outbound marketplace request uses. Proxies are off
// because the address check happens at dial time, and with a proxy the only address dialed
// would be the proxy's.
func newTransport(resolver Resolver, dial DialFunc, allowPrivate bool, headerTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            guardedDial(resolver, dial, allowPrivate),
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  headerTimeout,
		MaxResponseHeaderBytes: 64 << 10,
		DisableKeepAlives:      true,
		ForceAttemptHTTP2:      true,
	}
}

// NewGuardedTransport returns a transport that ignores proxy settings and refuses private and
// reserved addresses unless allowPrivate is set. Other outbound clients (the registry
// resolver) use it so they get the same SSRF protection as index fetches.
func NewGuardedTransport(allowPrivate bool) *http.Transport {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return newTransport(net.DefaultResolver, d.DialContext, allowPrivate, DefaultTimeout)
}
