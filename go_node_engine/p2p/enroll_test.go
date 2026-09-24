package p2p

import (
	"strings"
	"testing"
	"time"
)

func useTempCreds(t *testing.T) {
	t.Helper()
	old := credsDirPath
	credsDirPath = t.TempDir()
	t.Cleanup(func() { credsDirPath = old })
}

func TestGenesisCreatesIdentityAndPSK(t *testing.T) {
	useTempCreds(t)

	creds, err := Genesis("node-genesis")
	if err != nil {
		t.Fatal(err)
	}
	if creds.GossipKey == "" {
		t.Fatal("genesis did not mint a gossip PSK")
	}
	fp, ok := creds.Roster["node-genesis"]
	if !ok || !strings.HasPrefix(fp, "sha256:") {
		t.Fatalf("genesis did not roster self: %v", creds.Roster)
	}

	// Idempotent: a second genesis must not rotate the PSK or identity.
	again, err := Genesis("node-genesis")
	if err != nil {
		t.Fatal(err)
	}
	if again.GossipKey != creds.GossipKey {
		t.Error("second genesis rotated the PSK — must be idempotent")
	}

	loaded, ok := LoadCreds()
	if !ok || loaded.GossipKey != creds.GossipKey {
		t.Error("LoadCreds does not round-trip genesis creds")
	}
}

func TestIdentityStableFingerprint(t *testing.T) {
	useTempCreds(t)
	_, fp1, err := EnsureIdentity("node-x")
	if err != nil {
		t.Fatal(err)
	}
	_, fp2, err := EnsureIdentity("node-x")
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("identity not stable across loads: %s vs %s", fp1, fp2)
	}
}

func TestTokenSingleUseAndExpiry(t *testing.T) {
	useTempCreds(t)

	token, err := MintToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumeToken(token); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	// single use (plan §9.6)
	if err := consumeToken(token); err == nil {
		t.Fatal("token accepted twice — must be single-use")
	}
	// unknown token
	if err := consumeToken("dead.beef"); err == nil {
		t.Fatal("unknown token accepted")
	}
	// expired token
	expired, _ := MintToken(-time.Minute)
	if err := consumeToken(expired); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestRevokeRemovesFromRoster(t *testing.T) {
	useTempCreds(t)
	if _, err := Genesis("node-a"); err != nil {
		t.Fatal(err)
	}
	creds, _ := LoadCreds()
	creds.Roster["node-b"] = "sha256:feedface"
	_ = SaveCreds(creds)

	if err := Revoke("node-b"); err != nil {
		t.Fatal(err)
	}
	after, _ := LoadCreds()
	if _, present := after.Roster["node-b"]; present {
		t.Fatal("revoked node still in roster")
	}
	if err := Revoke("node-b"); err == nil {
		t.Fatal("revoking an absent node must error")
	}
}
