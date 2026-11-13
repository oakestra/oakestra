// Package mptest has fixtures shared by the marketplace tests.
package mptest

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"net/netip"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
)

// Digest is a syntactically valid sha256 digest.
var Digest = "sha256:" + strings.Repeat("ab", 32)

// Addon returns a minimal valid, digest-pinned definition without a source.
func Addon(name, version string) definition.Addon {
	a := definition.Addon{
		Name:        name,
		Version:     version,
		Description: "test addon " + name,
		Services: []definition.Service{{
			ServiceName: "web",
			Image:       "ghcr.io/example/" + name + ":" + version + "@" + Digest,
		}},
	}
	a.Normalize()
	return a
}

// IndexJSON builds an index document.
func IndexJSON(src string, serial uint64, addons ...definition.Addon) []byte {
	idx := definition.NewIndex(src, serial, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), addons)
	b, err := json.Marshal(idx)
	if err != nil {
		panic(err)
	}
	return b
}

// Resolver is a fixed DNS table.
type Resolver map[string][]netip.Addr

// LookupNetIP implements source.Resolver.
func (r Resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// PublicIP is a public address the test resolver hands out for hosts that should pass the
// address check.
var PublicIP = netip.MustParseAddr("93.184.216.34")

// Fetcher returns a fetcher whose resolver maps each host to its address, and whose dialer
// sends every connection to PublicIP on to srv. That lets tests exercise the default
// (private addresses blocked) path against a local httptest server.
func Fetcher(srv *httptest.Server, hosts Resolver) *source.Fetcher {
	target := srv.Listener.Addr().String()
	var d net.Dialer
	return source.NewFetcher(
		source.WithResolver(hosts),
		source.WithDial(func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if host == PublicIP.String() {
				return d.DialContext(ctx, network, target)
			}
			return d.DialContext(ctx, network, addr)
		}),
		source.WithTimeout(2*time.Second),
	)
}
