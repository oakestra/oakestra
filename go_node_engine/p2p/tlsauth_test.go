package p2p

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newForeignIdentity mints a keypair+cert OUTSIDE the roster (an intruder).
func newForeignIdentity(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "intruder"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestControlAPIMutualTLSRosterPinned verifies the connections plane (plan §9.1, §9.4):
// a rostered member completes the mTLS handshake; an un-rostered cert is rejected.
func TestControlAPIMutualTLSRosterPinned(t *testing.T) {
	useTempCreds(t)
	if _, err := Genesis("node-a"); err != nil {
		t.Fatal(err)
	}

	rv := newRosterVerifier(nil) // roster from local creds only (self)
	serverCfg, err := rv.serverTLSConfig("node-a")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = serverCfg
	srv.StartTLS()
	defer srv.Close()

	// 1) Rostered member (self identity) — must succeed.
	clientCfg, err := rv.clientTLSConfig("node-a")
	if err != nil {
		t.Fatal(err)
	}
	member := &http.Client{Transport: &http.Transport{TLSClientConfig: clientCfg}}
	resp, err := member.Get(srv.URL)
	if err != nil {
		t.Fatalf("rostered member rejected: %v", err)
	}
	_ = resp.Body.Close()

	// 2) Un-rostered cert — the server must refuse the handshake.
	intruderCfg := &tls.Config{
		Certificates:       []tls.Certificate{newForeignIdentity(t)},
		InsecureSkipVerify: true, //nolint:gosec // test intruder skips server verification
	}
	intruder := &http.Client{Transport: &http.Transport{TLSClientConfig: intruderCfg}}
	if resp, err := intruder.Get(srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatal("un-rostered client completed the mTLS handshake — roster pinning broken")
	}

	// 3) No client cert at all — also refused.
	nocert := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}}
	if resp, err := nocert.Get(srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatal("client without a certificate was accepted")
	}
}
