package proxy

import (
	"NetManager/TableEntryCache"
	"errors"
	"net"
	"net/netip"
	"testing"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// fakeBatchWriter stands in for *ipv4.PacketConn/*ipv6.PacketConn (see
// tunnelConn.batch). Failed calls return -1, not 0, because that's what
// Linux's sendmmsg(2) does; a fake that returned 0 like Darwin is how the
// negative-count bug slipped through before.
type fakeBatchWriter struct {
	calls   int
	written [][]byte
	err     error
}

func (f *fakeBatchWriter) WriteBatch(ms []ipv4.Message, flags int) (int, error) {
	f.calls++
	if f.err != nil {
		return -1, f.err
	}
	for _, m := range ms {
		f.written = append(f.written, append([]byte(nil), m.Buffers[0]...))
	}
	return len(ms), nil
}

// fakeConn returns a tunnelConn whose batched writes go through writer. The
// underlying conn is a real (unused) UDP socket, since a write failure closes
// and redials it just like the real path.
func fakeConn(t testing.TB, writer batchWriter) *tunnelConn {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9})
	if err != nil {
		t.Fatalf("dial dummy conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &tunnelConn{conn: conn, batch: writer}
}

// N packets to K distinct destinations should cost K WriteBatch calls, not N.
func TestOutgoingBatchAmortisesSyscalls(t *testing.T) {
	sock := &fakeTunnelSocket{}
	tunDev := &fakeTunDevice{batchSize: 32}
	tunnel := batchTestTunnel(sock, tunDev)

	const toServer = 10
	const toOther = 6
	for i := 0; i < toServer; i++ {
		tunDev.readQueue = append(tunDev.readQueue, buildTestPacketV4(t, clientNsIP, serverVIP, 40000+i, 443))
	}
	for i := 0; i < toOther; i++ {
		tunDev.readQueue = append(tunDev.readQueue, buildTestPacketV4(t, clientNsIP, otherVIP, 50000+i, 443))
	}

	serverWriter := &fakeBatchWriter{}
	otherWriter := &fakeBatchWriter{}
	serverDst := netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))
	otherDst := netip.AddrPortFrom(mustAddr(nodeCIP), uint16(tunnelPort))
	tunnel.connectionBuffer[serverDst] = fakeConn(t, serverWriter)
	tunnel.connectionBuffer[otherDst] = fakeConn(t, otherWriter)

	batch := newOutgoingBatch(tunDev.BatchSize())
	if err := tunnel.runOutgoingBatch(batch); err != nil {
		t.Fatalf("runOutgoingBatch: %v", err)
	}

	if serverWriter.calls != 1 {
		t.Errorf("WriteBatch called %d times for the %d packets bound for node B; want 1", serverWriter.calls, toServer)
	}
	if len(serverWriter.written) != toServer {
		t.Errorf("node B received %d packets; want %d", len(serverWriter.written), toServer)
	}
	if otherWriter.calls != 1 {
		t.Errorf("WriteBatch called %d times for the %d packets bound for node C; want 1", otherWriter.calls, toOther)
	}
	if len(otherWriter.written) != toOther {
		t.Errorf("node C received %d packets; want %d", len(otherWriter.written), toOther)
	}
}

// batchSize 1 is the real fallback on Darwin (and older Linux kernels), so
// grouping still has to work one packet at a time.
func TestOutgoingBatchCorrectAtBatchSizeOne(t *testing.T) {
	sock := &fakeTunnelSocket{}
	tunDev := &fakeTunDevice{batchSize: 1}
	tunnel := batchTestTunnel(sock, tunDev)

	packets := []struct {
		vip string
		dst string
	}{
		{serverVIP, nodeBIP},
		{otherVIP, nodeCIP},
		{serverVIP, nodeBIP},
	}
	for i, p := range packets {
		tunDev.readQueue = append(tunDev.readQueue, buildTestPacketV4(t, clientNsIP, p.vip, 41000+i, 443))
	}

	serverWriter := &fakeBatchWriter{}
	otherWriter := &fakeBatchWriter{}
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))] = fakeConn(t, serverWriter)
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeCIP), uint16(tunnelPort))] = fakeConn(t, otherWriter)

	batch := newOutgoingBatch(tunDev.BatchSize())
	for i := range packets {
		if err := tunnel.runOutgoingBatch(batch); err != nil {
			t.Fatalf("runOutgoingBatch #%d: %v", i, err)
		}
	}

	if len(serverWriter.written) != 2 {
		t.Errorf("node B received %d packets; want 2", len(serverWriter.written))
	}
	if len(otherWriter.written) != 1 {
		t.Errorf("node C received %d packets; want 1", len(otherWriter.written))
	}
	// One packet per read means one WriteBatch call per read.
	if serverWriter.calls != 2 {
		t.Errorf("WriteBatch called %d times across node B's 2 single-packet reads; want 2", serverWriter.calls)
	}
	if otherWriter.calls != 1 {
		t.Errorf("WriteBatch called %d times across node C's 1 single-packet read; want 1", otherWriter.calls)
	}
}

// One batch here mixes a local packet (ActionDeliver) with a forwarded one
// (ActionForward); they shouldn't interfere with each other.
func TestOutgoingBatchMixesLocalDeliveryAndForward(t *testing.T) {
	const (
		selfVIP    = "10.30.255.240"
		selfNsIP   = "10.19.9.9"
		selfNsIPv6 = "fd00::90"
		selfVIPv6  = "fdff:2000::f0"
		selfInstIP = "10.30.255.241"
		selfInstv6 = "fdff::f1"
	)
	self := tableEntry("selfapp", nodeAIP, selfNsIP, selfNsIPv6, selfVIP, selfVIPv6, selfInstIP, selfInstv6)
	entries := append(append([]TableEntryCache.TableEntry{}, fixtureEntries...), self)
	env := newFakeEnv(entries...)

	sock := &fakeTunnelSocket{}
	tunDev := &fakeTunDevice{batchSize: 4}
	tunnel := &Tunnel{
		sock:             sock,
		tun:              tunDev,
		connectionBuffer: make(map[netip.AddrPort]*tunnelConn),
	}
	tunnel.dp = NewDatapath(env, mustAddr(nodeAIP), fixtureIPv4Prefix, fixtureIPv6Prefix, tunnel)

	remote := buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)
	local := buildTestPacketV4(t, clientNsIP, selfVIP, 40001, 443)
	tunDev.readQueue = [][]byte{remote, local}

	serverWriter := &fakeBatchWriter{}
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))] = fakeConn(t, serverWriter)

	batch := newOutgoingBatch(tunDev.BatchSize())
	if err := tunnel.runOutgoingBatch(batch); err != nil {
		t.Fatalf("runOutgoingBatch: %v", err)
	}

	if len(serverWriter.written) != 1 {
		t.Errorf("node B received %d packets; want 1 (the remote-bound one)", len(serverWriter.written))
	}
	if tunDev.writeBatches != 1 {
		t.Errorf("TUN WriteBatch called %d times; want 1 for the locally-delivered packet", tunDev.writeBatches)
	}
	if len(tunDev.written) != 1 {
		t.Fatalf("TUN device delivered %d packets; want 1", len(tunDev.written))
	}
}

func TestOutgoingBatchOnePeerFailureDoesNotBlockOthers(t *testing.T) {
	sock := &fakeTunnelSocket{}
	tunDev := &fakeTunDevice{batchSize: 4}
	tunnel := batchTestTunnel(sock, tunDev)

	tunDev.readQueue = [][]byte{
		buildTestPacketV4(t, clientNsIP, serverVIP, 42000, 443), // -> node B, will fail
		buildTestPacketV4(t, clientNsIP, otherVIP, 42001, 443),  // -> node C, must still succeed
	}

	failing := &fakeBatchWriter{err: errors.New("boom")}
	okWriter := &fakeBatchWriter{}
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))] = fakeConn(t, failing)
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeCIP), uint16(tunnelPort))] = fakeConn(t, okWriter)

	batch := newOutgoingBatch(tunDev.BatchSize())
	if err := tunnel.runOutgoingBatch(batch); err != nil {
		t.Fatalf("runOutgoingBatch: %v", err)
	}

	if failing.calls == 0 {
		t.Error("the failing destination was never attempted")
	}
	if len(okWriter.written) != 1 {
		t.Errorf("the healthy destination received %d packets; want 1 - a sibling failure must not block it", len(okWriter.written))
	}
}

func TestOutgoingBatchReusesHighFanoutGroups(t *testing.T) {
	const peers = 128
	batch := newOutgoingBatch(peers)
	destinations := make([]netip.AddrPort, peers)
	for i := range destinations {
		destinations[i] = netip.AddrPortFrom(
			netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)}), uint16(tunnelPort))
		batch.group(destinations[i], []byte{byte(i)})
	}

	if batch.liveGroups != peers {
		t.Fatalf("first batch contains %d destination groups; want %d", batch.liveGroups, peers)
	}

	// Reused slots stay allocated, so their old packet slice must be cleared.
	batch.liveGroups = 0
	for i := peers - 1; i >= 0; i-- {
		batch.group(destinations[i], []byte{byte(i)})
	}

	if batch.liveGroups != peers {
		t.Fatalf("reused batch contains %d destination groups; want %d", batch.liveGroups, peers)
	}
	for i := 0; i < peers; i++ {
		group := batch.groups[i]
		want := destinations[peers-1-i]
		if group.dst != want {
			t.Errorf("group %d destination = %s; want %s", i, group.dst, want)
		}
		if len(group.bufs) != 1 {
			t.Errorf("group %d retained %d packets; want 1", i, len(group.bufs))
			continue
		}
		if got, wantPacket := group.bufs[0][0], byte(peers-1-i); got != wantPacket {
			t.Errorf("group %d contains packet %d; want %d", i, got, wantPacket)
		}
	}
}

// A restarted peer's ICMP port-unreachable shows up as ECONNREFUSED on the
// next send, so this needs to be a routine failure, not a crash.
func TestSendOverTunnelBatchSurvivesNegativeWriteCount(t *testing.T) {
	tunnel := batchTestTunnel(&fakeTunnelSocket{}, &fakeTunDevice{batchSize: 4})
	dst := netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))
	writer := &fakeBatchWriter{err: errors.New("sendmmsg: connection refused")}
	tunnel.connectionBuffer[dst] = fakeConn(t, writer)

	packet := buildTestPacketV4(t, clientNsIP, serverVIP, 42000, 443)
	tunnel.sendOverTunnelBatch(dst, [][]byte{packet}, newOutgoingBatch(4), 0)

	if writer.calls == 0 {
		t.Fatal("the failing write was never attempted")
	}
}

func TestNewTunnelConnPicksAddressFamily(t *testing.T) {
	v4Listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen v4: %v", err)
	}
	defer v4Listener.Close()
	v6Listener, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.ParseIP("::1"), Port: 0})
	if err != nil {
		t.Fatalf("listen v6: %v", err)
	}
	defer v6Listener.Close()

	v4Dst := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(v4Listener.LocalAddr().(*net.UDPAddr).Port))
	v6Dst := netip.AddrPortFrom(netip.MustParseAddr("::1"), uint16(v6Listener.LocalAddr().(*net.UDPAddr).Port))

	tunnel := getFakeTunnel()

	batch := newOutgoingBatch(4)
	tunnel.sendOverTunnelBatch(v4Dst, [][]byte{[]byte("hello-v4")}, batch, 0)
	tunnel.sendOverTunnelBatch(v6Dst, [][]byte{[]byte("hello-v6")}, batch, 0)

	tunnel.connectionBufferLock.RLock()
	v4Conn := tunnel.connectionBuffer[v4Dst]
	v6Conn := tunnel.connectionBuffer[v6Dst]
	tunnel.connectionBufferLock.RUnlock()

	if v4Conn == nil || v6Conn == nil {
		t.Fatalf("expected both destinations to have dialled a connection")
	}
	if _, ok := v4Conn.batch.(*ipv4.PacketConn); !ok {
		t.Errorf("v4 peer's tunnelConn.batch is %T; want *ipv4.PacketConn", v4Conn.batch)
	}
	if _, ok := v6Conn.batch.(*ipv6.PacketConn); !ok {
		t.Errorf("v6 peer's tunnelConn.batch is %T; want *ipv6.PacketConn", v6Conn.batch)
	}

	if got := readUDP(t, v4Listener); got != "hello-v4" {
		t.Errorf("v4 listener got %q; want %q", got, "hello-v4")
	}
	if got := readUDP(t, v6Listener); got != "hello-v6" {
		t.Errorf("v6 listener got %q; want %q", got, "hello-v6")
	}
}

func readUDP(t testing.TB, conn *net.UDPConn) string {
	t.Helper()
	buf := make([]byte, 64)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(buf[:n])
}
