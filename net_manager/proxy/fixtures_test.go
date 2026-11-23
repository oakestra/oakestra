package proxy

import (
	"NetManager/TableEntryCache"
	"NetManager/resolver"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// The fixture topology used by the proxy tests. Two nodes, each running a
// Datapath over a shared (globally consistent) translation table, which
// is what makes a full request/response round trip reproducible in-process:
//
//	node A (10.0.0.1)                     node B (10.0.0.2)
//	  clientapp  ns 10.19.1.1               serverapp  ns 10.19.2.12
//	             inst 10.30.255.253                    inst 10.30.255.254
//	                                                   vip  10.30.255.255
const (
	nodeAIP = "10.0.0.1"
	nodeBIP = "10.0.0.2"

	clientNsIP     = "10.19.1.1"
	clientNsIPv6   = "fc00::1"
	clientInstIP   = "10.30.255.253"
	clientInstIPv6 = "fdff::fd"
	clientVIP      = "10.30.255.252"
	clientVIPv6    = "fdff:2000::fc"

	serverNsIP     = "10.19.2.12"
	serverNsIPv6   = "fd00::12"
	serverInstIP   = "10.30.255.254"
	serverInstIPv6 = "fdff::fe"
	serverVIP      = "10.30.255.255"
	serverVIPv6    = "fdff:2000::ff"

	// a third service, on a third node, used to prove that two flows sharing
	// a source port but targeting different Service VIPs stay distinct
	nodeCIP       = "10.0.0.3"
	otherNsIP     = "10.19.3.20"
	otherNsIPv6   = "fd00::20"
	otherInstIP   = "10.30.255.250"
	otherInstIPv6 = "fdff::fa"
	otherVIP      = "10.30.255.251"
	otherVIPv6    = "fdff:2000::fb"

	tunnelPort = 50103
)

// the fixture proxy prefixes, shared by every Datapath/Tunnel built for tests.
var (
	fixtureIPv4Prefix = netip.MustParsePrefix("10.30.0.0/16")
	fixtureIPv6Prefix = netip.MustParsePrefix("fdff::/16")
)

// fixtureEntries is built once and shared by every test and benchmark. The
// benchmarks depend on this: the fake environment used to rebuild a TableEntry
// (eight net.ParseIP calls) on every lookup, so the allocations it reported
// belonged to the fixture rather than to the datapath under test.
var fixtureEntries = []TableEntryCache.TableEntry{
	tableEntry("clientapp", nodeAIP, clientNsIP, clientNsIPv6, clientVIP, clientVIPv6, clientInstIP, clientInstIPv6),
	tableEntry("serverapp", nodeBIP, serverNsIP, serverNsIPv6, serverVIP, serverVIPv6, serverInstIP, serverInstIPv6),
	tableEntry("otherapp", nodeCIP, otherNsIP, otherNsIPv6, otherVIP, otherVIPv6, otherInstIP, otherInstIPv6),
}

func tableEntry(job, nodeip, nsip, nsipv6, vip, vipv6, instip, instipv6 string) TableEntryCache.TableEntry {
	return TableEntryCache.TableEntry{
		JobName:          job + ".ns.svc.ns",
		Appname:          job,
		Appns:            "ns",
		Servicename:      "svc",
		Servicenamespace: "ns",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP(nodeip),
		Nodeport:         tunnelPort,
		Nsip:             net.ParseIP(nsip),
		Nsipv6:           net.ParseIP(nsipv6),
		ServiceIP: []TableEntryCache.ServiceIP{
			{IpType: TableEntryCache.Closest, Address: net.ParseIP(vip), Address_v6: net.ParseIP(vipv6)},
			{IpType: TableEntryCache.InstanceNumber, Address: net.ParseIP(instip), Address_v6: net.ParseIP(instipv6)},
		},
	}
}

// FakeEnv answers table lookups from a real TableManager, so tests exercise
// the same index and validation code the daemon does. tableLookups counts the
// Service IP lookups made against it, so tests can assert that a warm flow
// never reaches the table at all.
type FakeEnv struct {
	table        TableEntryCache.TableManager
	tableLookups int
}

func newFakeEnv(entries ...TableEntryCache.TableEntry) *FakeEnv {
	if len(entries) == 0 {
		entries = fixtureEntries
	}
	e := &FakeEnv{table: TableEntryCache.NewTableManager()}
	for _, entry := range entries {
		if err := e.table.Add(entry); err != nil {
			panic(err)
		}
	}
	return e
}

func (fakeenv *FakeEnv) GetTableEntryByServiceIP(addr netip.Addr) resolver.ServiceLookup {
	fakeenv.tableLookups++
	entries, generation := fakeenv.table.SearchByServiceIP(addr)
	return resolver.ServiceLookup{Entries: entries, Generation: generation}
}

func (fakeenv *FakeEnv) GetInstanceIP(addr netip.Addr, version uint8) (netip.Addr, bool) {
	return fakeenv.table.SearchInstanceIPByNsIP(addr, version)
}

func (fakeenv *FakeEnv) TableGeneration() uint64 {
	return fakeenv.table.Generation()
}

// replaceJob models a route refresh arriving from the cluster.
func (fakeenv *FakeEnv) replaceJob(t testing.TB, job string, entries ...TableEntryCache.TableEntry) {
	t.Helper()
	if err := fakeenv.table.ReplaceJobEntries(job, entries); err != nil {
		t.Fatal(err)
	}
}

// discardSink is a Sink that throws away every Action it is given, for tests
// that never trigger the replay goroutine (i.e. never hit a cold miss).
type discardSink struct{}

func (discardSink) Emit(Action) {}

// recordingSink records every Action Emit is called with, in order. It
// stands in for a Tunnel in tests that only care about what the replay
// goroutine (see Datapath.replayWhenResolved) decided, not about actual
// socket I/O.
type recordingSink struct {
	mu      sync.Mutex
	actions []Action
}

func (s *recordingSink) Emit(a Action) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, a)
}

// drain returns everything recorded so far and resets the recording.
func (s *recordingSink) drain() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.actions
	s.actions = nil
	return out
}

// getFakeDatapath returns a Datapath standing in for node A.
func getFakeDatapath() *Datapath {
	return fakeDatapathOn(nodeAIP, newFakeEnv())
}

// fakeDatapathOn returns a Datapath standing in for the node at localIP,
// answering table lookups from env.
func fakeDatapathOn(localIP string, env resolver.Resolver) *Datapath {
	return NewDatapath(env, mustAddr(localIP), fixtureIPv4Prefix, fixtureIPv6Prefix, discardSink{})
}

// getFakeTunnel returns a Tunnel standing in for node A. Prefer
// getFakeDatapath for tests that only exercise translation logic; this is
// for tests that need real socket I/O (see loopbackTunnel) or the
// connection pool.
func getFakeTunnel() *Tunnel {
	return fakeTunnelOn(nodeAIP)
}

func fakeTunnelOn(localIP string) *Tunnel {
	tunnel := &Tunnel{
		tunNetIP:          "10.19.1.254",
		tun:               nil,
		HostTUNDeviceName: "goProxyTun",
		TunnelPort:        tunnelPort,
		sock:              nil,
		tunNetIPv6:        "fdfe::1337",
		connectionBuffer:  make(map[netip.AddrPort]*tunnelConn),
	}
	tunnel.dp = NewDatapath(newFakeEnv(), mustAddr(localIP), fixtureIPv4Prefix, fixtureIPv6Prefix, tunnel)
	return tunnel
}

// buildTestPacketV4/V6 build a valid, correctly-checksummed wire-format packet
// for feeding into iputils.Parse. gopacket is only used here, as a test
// fixture builder, never by the production code in this package.
func buildTestPacketV4(t testing.TB, srcIP, dstIP string, srcPort, dstPort int) []byte {
	return buildTCPv4(t, srcIP, dstIP, srcPort, dstPort)
}

func buildTCPv4(t testing.TB, srcIP, dstIP string, srcPort, dstPort int) []byte {
	t.Helper()
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), SYN: true}
	_ = tcp.SetNetworkLayerForChecksum(ip)
	return serialize(t, ip, tcp)
}

func buildUDPv4(t testing.TB, srcIP, dstIP string, srcPort, dstPort int, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	udp := &layers.UDP{SrcPort: layers.UDPPort(srcPort), DstPort: layers.UDPPort(dstPort)}
	_ = udp.SetNetworkLayerForChecksum(ip)
	return serialize(t, ip, udp, gopacket.Payload(payload))
}

func buildUDPv6(t testing.TB, srcIP, dstIP string, srcPort, dstPort int, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv6{
		Version:    6,
		HopLimit:   64,
		NextHeader: layers.IPProtocolUDP,
		SrcIP:      net.ParseIP(srcIP),
		DstIP:      net.ParseIP(dstIP),
	}
	udp := &layers.UDP{SrcPort: layers.UDPPort(srcPort), DstPort: layers.UDPPort(dstPort)}
	_ = udp.SetNetworkLayerForChecksum(ip)
	return serialize(t, ip, udp, gopacket.Payload(payload))
}

func buildTestPacketV6(t testing.TB, srcIP, dstIP string, srcPort, dstPort int) []byte {
	t.Helper()
	ip := &layers.IPv6{
		Version:    6,
		HopLimit:   64,
		NextHeader: layers.IPProtocolTCP,
		SrcIP:      net.ParseIP(srcIP),
		DstIP:      net.ParseIP(dstIP),
	}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), SYN: true}
	_ = tcp.SetNetworkLayerForChecksum(ip)
	return serialize(t, ip, tcp)
}

func serialize(t testing.TB, ls ...gopacket.SerializableLayer) []byte {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ls...); err != nil {
		t.Fatalf("build packet: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

// loopbackTunnel returns a Tunnel whose target service resolves to a real UDP
// socket on loopback, so tests can drive the complete outgoing path
// (translation, fragment state and an actual forward over the wire) through
// Tunnel.Emit and read back exactly what went on the wire.
func loopbackTunnel(t testing.TB) (*Tunnel, *net.UDPConn) {
	t.Helper()
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.LocalAddr().(*net.UDPAddr).Port

	server := tableEntry("serverapp", "127.0.0.1", serverNsIP, serverNsIPv6,
		serverVIP, serverVIPv6, serverInstIP, serverInstIPv6)
	server.Nodeport = port

	tunnel := fakeTunnelOn(nodeAIP)
	tunnel.SetResolver(newFakeEnv(fixtureEntries[0], server))
	return tunnel, listener
}

// readForwarded reads one datagram the tunnel forwarded over the wire.
func readForwarded(t testing.TB, listener *net.UDPConn) []byte {
	t.Helper()
	_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 65536)
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("expected a forwarded packet: %v", err)
	}
	return buf[:n]
}

// replicatedFixture returns the standard table with serverapp scaled out to n
// instances sharing one Service VIP.
func replicatedFixture(t testing.TB, n int) []TableEntryCache.TableEntry {
	t.Helper()
	entries := []TableEntryCache.TableEntry{fixtureEntries[0]}
	for i := range n {
		replica := tableEntry("serverapp",
			fmt.Sprintf("10.%d.%d.1", i/250, i%250),
			fmt.Sprintf("10.19.%d.%d", i/250+2, i%250+1),
			fmt.Sprintf("fd00::%x", i+1),
			serverVIP, serverVIPv6, serverInstIP, serverInstIPv6)
		replica.Instancenumber = i
		entries = append(entries, replica)
	}
	return entries
}
