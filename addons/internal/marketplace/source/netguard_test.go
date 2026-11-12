package source

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "0.0.0.0", "::", "10.1.2.3", "172.16.0.1", "192.168.0.1",
		"169.254.169.254", "fe80::1", "100.64.0.1", "100.127.255.254", "fc00::1", "fd12::1",
		"224.0.0.1", "ff02::1", "255.255.255.255", "198.18.0.1",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254", "::ffff:10.0.0.1",
		"64:ff9b::a9fe:a9fe", "64:ff9b::7f00:1", "2002:a9fe:a9fe::1", "::127.0.0.1",
	}
	allowed := []string{
		"8.8.8.8", "93.184.216.34", "100.128.0.1", "2606:4700:4700::1111", "64:ff9b::808:808",
		"::ffff:8.8.8.8", "2002:808:808::1",
	}
	for _, s := range blocked {
		if !isBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s is not blocked", s)
		}
	}
	for _, s := range allowed {
		if isBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s is blocked", s)
		}
	}
}

func TestRedactURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://tap.example/index.json":                        "https://tap.example/index.json",
		"https://tap.example/index.json?X-Amz-Signature=secret": "https://tap.example/index.json?<redacted>",
		"https://tap.example:8443/a/b?token=abc":                "https://tap.example:8443/a/b?<redacted>",
		"https://tap.example/a?":                                "https://tap.example/a?<redacted>",
		"file:///srv/index.json":                                "file:///srv/index.json",
	} {
		if got := RedactURL(in); got != want {
			t.Errorf("RedactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGuardedTransportRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	for allow, wantErr := range map[bool]bool{false: true, true: false} {
		client := &http.Client{Transport: NewGuardedTransport(allow)}
		resp, err := client.Get(srv.URL)
		if err == nil {
			resp.Body.Close()
		}
		if wantErr && !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("allowPrivate=%v: expected ErrBlockedAddress, got %v", allow, err)
		}
		if !wantErr && err != nil {
			t.Errorf("allowPrivate=%v: %v", allow, err)
		}
	}
}

func TestGuardedTransportIgnoresProxyEnv(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	if NewGuardedTransport(false).Proxy != nil {
		t.Error("proxy must be off")
	}
}
