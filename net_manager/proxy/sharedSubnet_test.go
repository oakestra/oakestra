package proxy

import (
	"bytes"
	"net"
	"testing"
)

// Every node allocates namespace IPs from the same fixed subnet, so instances on
// different nodes routinely share one. These tests pin down that the datapath
// still translates and forwards per node.

// Two replicas behind one Service VIP share a namespace IP but live on different
// nodes: each flow must be forwarded to the node of the replica it picked.
func TestOutgoingProxyCollidingNamespaceIPs(t *testing.T) {
	replicaB := tableEntry("serverapp", nodeBIP, serverNsIP, serverNsIPv6, serverVIP, serverVIPv6, serverInstIP, serverInstIPv6)
	replicaC := tableEntry("serverapp", nodeCIP, serverNsIP, serverNsIPv6, serverVIP, serverVIPv6, otherInstIP, otherInstIPv6)
	env := newFakeEnv(fixtureEntries[0], replicaB, replicaC)
	dp := fakeDatapathOn(nodeAIP, env)

	seen := map[string]bool{}
	for port := 1000; port < 1200 && len(seen) < 2; port++ {
		action := dp.Handle(Outgoing, buildTestPacketV4(t, clientNsIP, serverVIP, port, 80))
		if action.Kind != ActionForward {
			t.Fatalf("port %d: action %v; want forward", port, action.Kind)
		}
		dst := action.Dst.Addr().String()
		if dst != nodeBIP && dst != nodeCIP {
			t.Fatalf("forwarded to %s; want one of the replica nodes", dst)
		}
		seen[dst] = true
	}
	if len(seen) != 2 {
		t.Fatalf("200 flows all went to %v: the shared namespace IP is being resolved to one node", seen)
	}
}

// The source translation must use the local instance, even when a remote instance
// with the same namespace IP is listed first.
func TestSourceTranslationPrefersLocalInstance(t *testing.T) {
	remoteTwin := tableEntry("otherapp", nodeCIP, clientNsIP, clientNsIPv6, otherVIP, otherVIPv6, otherInstIP, otherInstIPv6)
	env := newFakeEnv(remoteTwin, fixtureEntries[0], fixtureEntries[1])
	env.table.SetLocalNode(net.ParseIP(nodeAIP))
	dp := fakeDatapathOn(nodeAIP, env)

	pkt := parseTestPacket(t, buildTestPacketV4(t, clientNsIP, serverVIP, 666, 80))
	if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
		t.Fatal("packet should have been proxied")
	}
	if pkt.SrcIP() != mustAddr(clientInstIP) {
		t.Errorf("srcIP = %s; want the local instance %s, not the remote twin's %s", pkt.SrcIP(), clientInstIP, otherInstIP)
	}
}

func TestDataPlaneEncryption(t *testing.T) {
	var sender, receiver, stranger dataCrypt
	tunnel := &Tunnel{}
	if err := tunnel.EnableDataEncryption(DeriveDataKey("network-secret")); err != nil {
		t.Fatal(err)
	}
	sender.aead.Store(tunnel.crypt.aead.Load())
	receiver.aead.Store(tunnel.crypt.aead.Load())
	other := &Tunnel{}
	if err := other.EnableDataEncryption(DeriveDataKey("another-network")); err != nil {
		t.Fatal(err)
	}
	stranger.aead.Store(other.crypt.aead.Load())

	plain := [][]byte{[]byte("first packet"), []byte("second, longer packet")}
	var scratch sealScratch
	sealed := sender.seal(plain, &scratch)
	if len(sealed) != len(plain) {
		t.Fatalf("sealed %d packets; want %d", len(sealed), len(plain))
	}

	t.Run("round trip", func(t *testing.T) {
		for i := range sealed {
			if bytes.Contains(sealed[i], plain[i]) {
				t.Fatalf("packet %d travels in the clear", i)
			}
			got, ok := receiver.open(append([]byte(nil), sealed[i]...))
			if !ok || !bytes.Equal(got, plain[i]) {
				t.Fatalf("packet %d: open = %q, %v; want %q", i, got, ok, plain[i])
			}
		}
	})
	t.Run("tampered", func(t *testing.T) {
		bad := append([]byte(nil), sealed[0]...)
		bad[len(bad)-1] ^= 0x01
		if _, ok := receiver.open(bad); ok {
			t.Fatal("tampered datagram was accepted")
		}
	})
	t.Run("wrong key", func(t *testing.T) {
		if _, ok := stranger.open(append([]byte(nil), sealed[0]...)); ok {
			t.Fatal("datagram sealed under another network's key was accepted")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		if _, ok := receiver.open(sealed[0][:10]); ok {
			t.Fatal("truncated datagram was accepted")
		}
	})
	t.Run("disabled passes through", func(t *testing.T) {
		var off dataCrypt
		if got := off.seal(plain, &sealScratch{}); &got[0][0] != &plain[0][0] {
			t.Fatal("seal copied packets with encryption off")
		}
		if got, ok := off.open(plain[0]); !ok || !bytes.Equal(got, plain[0]) {
			t.Fatal("open altered a packet with encryption off")
		}
	})
}
