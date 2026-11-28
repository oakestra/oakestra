package proxy

import (
	"NetManager/proxy/iputils"
	"encoding/hex"
	"testing"
)

// used as example packets for testing
var ipv6Packet string = "600219310028063ffc000000000000000000000000000203fdff1000000000000000000000000001b98400502a8697ed00000000a002ff322a8900000204056e0402080a7fb1168c0000000001030307"

func parseTestPacket(t *testing.T, wire []byte) iputils.Packet {
	t.Helper()
	pkt, ok := iputils.Parse(wire)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("failed to parse test packet (ok=%v)", ok)
	}
	return pkt
}

func TestOutgoingProxy(t *testing.T) {
	dp := getFakeDatapath()

	pkt := parseTestPacket(t, buildTestPacketV4(t, clientNsIP, serverVIP, 666, 80))
	noProxyPkt := parseTestPacket(t, buildTestPacketV4(t, clientNsIP, "10.20.1.1", 666, 80))

	if _, _, _, proxied := dp.outgoingProxy(&noProxyPkt); proxied {
		t.Error("Packet should not be proxied")
	}

	if _, _, _, proxied := dp.outgoingProxy(&pkt); !proxied {
		t.Fatal("packet should have been proxied")
	}
	if pkt.DstIP() != mustAddr(serverNsIP) {
		t.Error("dstIP = ", pkt.DstIP(), "; want =", serverNsIP)
	}
	if pkt.SrcIP() != mustAddr(clientInstIP) {
		t.Error("srcIP = ", pkt.SrcIP(), "; want =", clientInstIP)
	}
}

func TestIngoingProxy(t *testing.T) {
	dp := getFakeDatapath()

	// A reply for a flow this node originated: 10.19.1.1:666 -> serverVIP:80.
	dp.proxycache.Add(ConversionEntry{
		srcip:         mustAddr(clientNsIP),
		dstip:         mustAddr(serverNsIP),
		dstServiceIp:  mustAddr(serverVIP),
		srcInstanceIp: mustAddr(clientInstIP),
		dstInstanceIp: mustAddr(serverInstIP),
		srcport:       666,
		dstport:       80,
		protocol:      iputils.ProtoTCP,
	})

	pkt := parseTestPacket(t, buildTestPacketV4(t, serverInstIP, clientNsIP, 80, 666))
	noProxyPkt := parseTestPacket(t, buildTestPacketV4(t, serverNsIP, "10.19.1.12", 666, 80))

	if dp.ingoingProxy(&noProxyPkt) {
		t.Error("Packet should not be proxied")
	}

	if !dp.ingoingProxy(&pkt) {
		t.Fatal("packet should have matched the reverse cache entry")
	}
	if pkt.SrcIP() != mustAddr(serverVIP) {
		t.Error("srcIp = ", pkt.SrcIP(), "; want =", serverVIP)
	}
	if pkt.DstIP() != mustAddr(clientNsIP) {
		t.Error("dstIp = ", pkt.DstIP(), "; want =", clientNsIP)
	}
}

func TestOutgoingV6Proxy(t *testing.T) {
	dp := getFakeDatapath()

	pkt := parseTestPacket(t, buildTestPacketV6(t, clientNsIPv6, serverVIPv6, 666, 80))
	noProxyPkt := parseTestPacket(t, buildTestPacketV6(t, clientNsIPv6, serverNsIPv6, 666, 80))

	if _, _, _, proxied := dp.outgoingProxy(&noProxyPkt); proxied {
		t.Error("Packet should not be proxied")
	}

	if _, _, _, proxied := dp.outgoingProxy(&pkt); !proxied {
		t.Fatal("packet should have been proxied")
	}
	if pkt.DstIP() != mustAddr(serverNsIPv6) {
		t.Error("dstIP = ", pkt.DstIP(), "; want =", serverNsIPv6)
	}
}

func TestIngoingV6Proxy(t *testing.T) {
	dp := getFakeDatapath()

	dp.proxycache.Add(ConversionEntry{
		srcip:         mustAddr(clientNsIPv6),
		dstip:         mustAddr(serverNsIPv6),
		dstServiceIp:  mustAddr(serverVIPv6),
		srcInstanceIp: mustAddr(clientInstIPv6),
		dstInstanceIp: mustAddr(serverInstIPv6),
		srcport:       666,
		dstport:       80,
		protocol:      iputils.ProtoTCP,
	})

	pkt := parseTestPacket(t, buildTestPacketV6(t, serverInstIPv6, clientNsIPv6, 80, 666))
	noProxyPkt := parseTestPacket(t, buildTestPacketV6(t, clientNsIPv6, serverNsIPv6, 666, 80))

	if dp.ingoingProxy(&noProxyPkt) {
		t.Error("Packet should not be proxied")
	}

	if !dp.ingoingProxy(&pkt) {
		t.Fatal("packet should have matched the reverse cache entry")
	}
	if pkt.SrcIP() != mustAddr(serverVIPv6) {
		t.Error("srcIp = ", pkt.SrcIP(), "; want =", serverVIPv6)
	}
}

func TestIPv6NextHeader(t *testing.T) {
	// keep this test in, since the IPv6 extension header walk seemed to mess up the parsing of the packet afterwards.
	// for future safety
	msg, _ := hex.DecodeString(ipv6Packet)
	pkt, ok := iputils.Parse(msg)
	if !ok {
		t.Fatal("failed to parse test packet")
	}
	if pkt.Protocol() != iputils.ProtoTCP {
		t.Error("Failed to detect TCP Header in IPv6 Next Header field.")
	}
}

// TestRoundTrip runs a full request/response cycle across two nodes. The
// reply comes back from the server's instance IP, not its namespace IP,
// because node B's own outgoingProxy translates it again on the way out.
func TestRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		clientNs, clientInst, serverNs, serverInst string
		serverVip                                  string
		build                                      func(testing.TB, string, string, int, int) []byte
	}{
		{"ipv4", clientNsIP, clientInstIP, serverNsIP, serverInstIP, serverVIP, buildTestPacketV4},
		{"ipv6", clientNsIPv6, clientInstIPv6, serverNsIPv6, serverInstIPv6, serverVIPv6, buildTestPacketV6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodeA := fakeDatapathOn(nodeAIP, newFakeEnv())
			nodeB := fakeDatapathOn(nodeBIP, newFakeEnv())

			// 1. client sends to the service VIP; node A translates it.
			req := parseTestPacket(t, tc.build(t, tc.clientNs, tc.serverVip, 40000, 80))
			dstNode, dstPort, _, ok := nodeA.outgoingProxy(&req)
			if !ok {
				t.Fatal("request should have been proxied")
			}
			if got := dstNode.String(); got != nodeBIP {
				t.Errorf("forwarded to node %s; want %s", got, nodeBIP)
			}
			if dstPort != tunnelPort {
				t.Errorf("forwarded to port %d; want %d", dstPort, tunnelPort)
			}
			if req.SrcIP() != mustAddr(tc.clientInst) || req.DstIP() != mustAddr(tc.serverNs) {
				t.Fatalf("on the wire: %s -> %s; want %s -> %s",
					req.SrcIP(), req.DstIP(), tc.clientInst, tc.serverNs)
			}

			// 2. node B has no reverse mapping for it, so it reaches the
			// server namespace unchanged.
			if nodeB.ingoingProxy(&req) {
				t.Error("node B should have no reverse mapping for the request")
			}

			// 3. the server replies to the source it saw: the client's
			// instance IP. node B translates that reply on the way out too.
			reply := parseTestPacket(t, tc.build(t, tc.serverNs, tc.clientInst, 80, 40000))
			backNode, _, _, ok := nodeB.outgoingProxy(&reply)
			if !ok {
				t.Fatal("reply should have been proxied by node B")
			}
			if got := backNode.String(); got != nodeAIP {
				t.Errorf("reply forwarded to node %s; want %s", got, nodeAIP)
			}
			if reply.SrcIP() != mustAddr(tc.serverInst) {
				t.Fatalf("reply source on the wire = %s; want the server instance IP %s",
					reply.SrcIP(), tc.serverInst)
			}
			if reply.DstIP() != mustAddr(tc.clientNs) {
				t.Fatalf("reply destination on the wire = %s; want %s", reply.DstIP(), tc.clientNs)
			}

			// 4. node A reverses it back to semantic addressing.
			if !nodeA.ingoingProxy(&reply) {
				t.Fatal("node A should have reversed the reply")
			}
			if reply.SrcIP() != mustAddr(tc.serverVip) {
				t.Errorf("client sees source %s; want the service VIP %s", reply.SrcIP(), tc.serverVip)
			}
			if reply.DstIP() != mustAddr(tc.clientNs) {
				t.Errorf("client sees destination %s; want %s", reply.DstIP(), tc.clientNs)
			}
		})
	}
}
