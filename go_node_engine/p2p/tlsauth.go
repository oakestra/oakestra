package p2p

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Roster-pinned mutual TLS for the connections plane (plan §9.1, §9.4 — trust model D):
// every node serves and dials with its SELF-SIGNED cert; a peer is accepted iff its cert
// fingerprint is in the trusted roster. No CA, no chains — set membership.
//
// Roster propagation: each node gossips its own fingerprint as a "__roster__" registry
// entry (Meta = fingerprint), so all members converge on everyone's identity over the
// PSK-authenticated gossip — the same trust base as admission itself.

// RosterJobName is the reserved registry job name carrying member fingerprints.
const RosterJobName = "__roster__"

// rosterVerifier resolves the current trusted fingerprint set, merging the local creds
// roster with gossiped __roster__ entries. Cached briefly to keep handshakes cheap.
type rosterVerifier struct {
	nm        *NMClient
	mu        sync.Mutex
	cached    map[string]bool
	refreshed time.Time
}

func newRosterVerifier(nm *NMClient) *rosterVerifier {
	return &rosterVerifier{nm: nm, cached: map[string]bool{}}
}

func (rv *rosterVerifier) trusted() map[string]bool {
	rv.mu.Lock()
	defer rv.mu.Unlock()
	if time.Since(rv.refreshed) < 10*time.Second && len(rv.cached) > 0 {
		return rv.cached
	}
	fps := map[string]bool{}
	if creds, ok := LoadCreds(); ok {
		for _, fp := range creds.Roster {
			fps[fp] = true
		}
	}
	if rv.nm != nil {
		if entries, err := rv.nm.Services(); err == nil {
			for _, e := range entries {
				if e.JobName == RosterJobName && e.Meta != "" {
					fps[e.Meta] = true
				}
			}
		}
	}
	if len(fps) > 0 {
		rv.cached = fps
		rv.refreshed = time.Now()
	}
	return fps
}

// verifyPeer is the custom TLS callback (plan §9.4): accept iff the presented cert's
// fingerprint is in the roster. Used symmetrically for client and server certs.
func (rv *rosterVerifier) verifyPeer(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errors.New("no peer certificate presented")
	}
	if rv.trusted()[Fingerprint(rawCerts[0])] {
		return nil
	}
	return errors.New("peer certificate fingerprint not in the member roster")
}

// serverTLSConfig builds the control-API server config: our self-signed cert, and
// clients MUST present a rostered cert (mutual TLS).
func (rv *rosterVerifier) serverTLSConfig(nodeUUID string) (*tls.Config, error) {
	cert, _, err := EnsureIdentity(nodeUUID)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates:          []tls.Certificate{cert},
		ClientAuth:            tls.RequireAnyClientCert,
		VerifyPeerCertificate: rv.verifyPeer,
		MinVersion:            tls.VersionTLS12,
	}, nil
}

// clientTLSConfig builds the peer-dialing config: present our cert, verify the server
// by roster fingerprint instead of chain validation.
func (rv *rosterVerifier) clientTLSConfig(nodeUUID string) (*tls.Config, error) {
	cert, _, err := EnsureIdentity(nodeUUID)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates:          []tls.Certificate{cert},
		InsecureSkipVerify:    true, //nolint:gosec // roster-pinned via VerifyPeerCertificate
		VerifyPeerCertificate: rv.verifyPeer,
		MinVersion:            tls.VersionTLS12,
	}, nil
}

// NewMemberHTTPClient returns an mTLS client for member-to-member control-API calls.
// Falls back to a plain client when the node has no p2p identity yet (pre-genesis).
// The second return is the URL scheme to use ("https" or "http").
func NewMemberHTTPClient(nodeUUID string, nm *NMClient, timeout time.Duration) (*http.Client, string) {
	if _, ok := LoadCreds(); !ok {
		return &http.Client{Timeout: timeout}, "http"
	}
	rv := newRosterVerifier(nm)
	cfg, err := rv.clientTLSConfig(nodeUUID)
	if err != nil {
		return &http.Client{Timeout: timeout}, "http"
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: cfg},
	}, "https"
}
