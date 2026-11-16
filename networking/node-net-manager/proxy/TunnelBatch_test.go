package proxy

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

// fakeTunDevice is a counting, in-memory TunDevice that honours the real
// adapter's header-offset convention (see tunHeaderOffset), so tests run
// without a real TUN device.
type fakeTunDevice struct {
	mu        sync.Mutex
	batchSize int
	readQueue [][]byte

	writeBatches int
	written      [][]byte
}

func (f *fakeTunDevice) ReadBatch(bufs [][]byte, sizes []int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for n < len(bufs) && len(f.readQueue) > 0 {
		pkt := f.readQueue[0]
		f.readQueue = f.readQueue[1:]
		copy(bufs[n][tunHeaderOffset:], pkt)
		sizes[n] = len(pkt)
		n++
	}
	return n, nil
}

func (f *fakeTunDevice) WriteBatch(bufs [][]byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeBatches++
	for _, b := range bufs {
		f.written = append(f.written, append([]byte(nil), b[tunHeaderOffset:]...))
	}
	return len(bufs), nil
}

func (f *fakeTunDevice) BatchSize() int { return f.batchSize }
func (f *fakeTunDevice) Name() string   { return "faketun" }
func (f *fakeTunDevice) Close() error   { return nil }

// fakeTunnelSocket is a counting, in-memory TunnelSocket standing in for the
// listen socket.
type fakeTunnelSocket struct {
	mu    sync.Mutex
	queue [][]byte
}

func (f *fakeTunnelSocket) ReadBatch(bufs [][]byte, sizes []int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for n < len(bufs) && len(f.queue) > 0 {
		pkt := f.queue[0]
		f.queue = f.queue[1:]
		copy(bufs[n], pkt)
		sizes[n] = len(pkt)
		n++
	}
	return n, nil
}

func (f *fakeTunnelSocket) WriteBatch(netip.AddrPort, [][]byte) (int, error) { return 0, nil }
func (f *fakeTunnelSocket) Close() error                                     { return nil }

// batchTestTunnel wires a real Datapath to the counting fakes above, so
// tests exercise real translation without a real TunDevice/TunnelSocket.
func batchTestTunnel(sock *fakeTunnelSocket, tun *fakeTunDevice) *Tunnel {
	tunnel := &Tunnel{
		sock:             sock,
		tun:              tun,
		connectionBuffer: make(map[netip.AddrPort]*tunnelConn),
	}
	tunnel.dp = fakeDatapathOn(nodeAIP, newFakeEnv())
	return tunnel
}

// TestIngoingBatchAmortisesSyscalls checks that N packets delivered in one
// TunnelSocket.ReadBatch reach the TUN device through exactly one
// TunDevice.WriteBatch call.
func TestIngoingBatchAmortisesSyscalls(t *testing.T) {
	const n = 16
	var queue [][]byte
	for i := 0; i < n; i++ {
		queue = append(queue, buildUDPv4(t, serverInstIP, clientNsIP, 80, 40000+i, []byte("payload")))
	}
	sock := &fakeTunnelSocket{queue: queue}
	tunDev := &fakeTunDevice{batchSize: n}
	tunnel := batchTestTunnel(sock, tunDev)

	batch := newIngoingBatch(tunDev.BatchSize())
	if err := tunnel.runIngoingBatch(batch); err != nil {
		t.Fatalf("runIngoingBatch: %v", err)
	}

	if tunDev.writeBatches != 1 {
		t.Errorf("WriteBatch called %d times for a single ReadBatch of %d packets; want 1", tunDev.writeBatches, n)
	}
	if len(tunDev.written) != n {
		t.Errorf("delivered %d packets; want %d", len(tunDev.written), n)
	}
	for i, got := range tunDev.written {
		if !bytes.Equal(got, queue[i]) {
			t.Errorf("packet %d delivered unchanged with no reverse mapping was mutated", i)
		}
	}
}

// TestIngoingBatchCorrectAtBatchSizeOne covers the no-IFF_VNET_HDR fallback
// (Darwin always, an older Linux kernel sometimes): every packet must still
// arrive at the TUN device, one WriteBatch call per read.
func TestIngoingBatchCorrectAtBatchSizeOne(t *testing.T) {
	packets := [][]byte{
		buildUDPv4(t, serverInstIP, clientNsIP, 80, 41000, []byte("one")),
		buildUDPv4(t, serverInstIP, clientNsIP, 80, 41001, []byte("two")),
		buildUDPv4(t, serverInstIP, clientNsIP, 80, 41002, []byte("three")),
	}
	sock := &fakeTunnelSocket{queue: append([][]byte(nil), packets...)}
	tunDev := &fakeTunDevice{batchSize: 1}
	tunnel := batchTestTunnel(sock, tunDev)

	batch := newIngoingBatch(tunDev.BatchSize())
	for i := range packets {
		if err := tunnel.runIngoingBatch(batch); err != nil {
			t.Fatalf("runIngoingBatch #%d: %v", i, err)
		}
	}

	if tunDev.writeBatches != len(packets) {
		t.Errorf("WriteBatch called %d times for %d single-packet reads; want %d",
			tunDev.writeBatches, len(packets), len(packets))
	}
	if len(tunDev.written) != len(packets) {
		t.Fatalf("delivered %d packets; want %d", len(tunDev.written), len(packets))
	}
	for i, got := range tunDev.written {
		if !bytes.Equal(got, packets[i]) {
			t.Errorf("packet %d corrupted at batch size 1", i)
		}
	}
}

// TestHandleIngoingOnlyDeliversOrDrops checks that Handle(Ingoing, ...) never
// returns ActionForward (runIngoingBatch relies on this) for a matched
// flow, an unmatched one, and a malformed packet.
func TestHandleIngoingOnlyDeliversOrDrops(t *testing.T) {
	dp := getFakeDatapath()
	dp.proxycache.Add(ConversionEntry{
		srcip:         mustAddr(clientNsIP),
		dstip:         mustAddr(serverNsIP),
		dstServiceIp:  mustAddr(serverVIP),
		srcInstanceIp: mustAddr(clientInstIP),
		dstInstanceIp: mustAddr(serverInstIP),
		srcport:       666,
		dstport:       80,
		protocol:      6,
	})

	cases := map[string][]byte{
		"matched flow":   buildTestPacketV4(t, serverInstIP, clientNsIP, 80, 666),
		"unmatched flow": buildTestPacketV4(t, "10.0.9.9", clientNsIP, 12345, 666),
		"malformed":      {0x45, 0x00},
		"udp, no transport": func() []byte {
			b := buildUDPv4(t, serverInstIP, clientNsIP, 80, 40000, nil)
			return b[:1] // truncate below a full IP header
		}(),
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			action := dp.Handle(Ingoing, wire)
			if action.Kind == ActionForward {
				t.Errorf("Handle(Ingoing, ...) returned ActionForward for %q - runIngoingBatch cannot act on that", name)
			}
		})
	}
}

// fakeWgDevice is a minimal tun.Device fake for testing the real adapter's
// handling of tun.ErrTooManySegments without opening a real TUN device
// (which needs root).
//
// The overflow case copies gsoSplit exactly: fill every buffer, then report
// one fewer. Both halves matter: (0, err) would let an adapter that throws n
// away pass, and the honest count would hide the off-by-one it has to undo.
type fakeWgDevice struct {
	calls int
}

func (d *fakeWgDevice) File() *os.File { return nil }

func (d *fakeWgDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	d.calls++
	pkt := []byte{0x45, 0x00, 0x00, 0x14}
	if d.calls == 1 {
		// What gsoSplit does when it runs out of room: every buffer
		// written, count one short.
		for i := range bufs {
			sizes[i] = copy(bufs[i][offset:], pkt)
		}
		return len(bufs) - 1, tun.ErrTooManySegments
	}
	n := copy(bufs[0][offset:], pkt)
	sizes[0] = n
	return 1, nil
}

func (d *fakeWgDevice) Write(bufs [][]byte, offset int) (int, error) { return len(bufs), nil }
func (d *fakeWgDevice) MTU() (int, error)                            { return 1500, nil }
func (d *fakeWgDevice) Name() (string, error)                        { return "faketun", nil }
func (d *fakeWgDevice) Events() <-chan tun.Event                     { return nil }
func (d *fakeWgDevice) Close() error                                 { return nil }
func (d *fakeWgDevice) BatchSize() int                               { return 4 }

// TestWgTunDeviceErrTooManySegmentsIsNotFatal checks that a GSO superpacket
// wider than the read batch is absorbed rather than propagated, and that
// later reads still succeed.
func TestWgTunDeviceErrTooManySegmentsIsNotFatal(t *testing.T) {
	const batch = 4
	w := newWgTunDevice(&fakeWgDevice{})
	bufs := make([][]byte, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunHeaderOffset+64)
	}
	sizes := make([]int, batch)

	if _, err := w.ReadBatch(bufs, sizes); err != nil {
		t.Fatalf("first read: got err %v; want nil - ErrTooManySegments must be absorbed, not propagated", err)
	}

	n, err := w.ReadBatch(bufs, sizes)
	if err != nil || n != 1 {
		t.Fatalf("second read: got (%d, %v); want (1, nil) - the device must still work after the error", n, err)
	}
}

// TestWgTunDeviceErrTooManySegmentsKeepsTheSegmentsThatFit checks that every
// segment that fit survives the error, the last one included: gsoSplit writes
// it but leaves it out of its count. Dropping segments here costs good packets
// on every oversized superpacket, on top of those that really didn't fit.
func TestWgTunDeviceErrTooManySegmentsKeepsTheSegmentsThatFit(t *testing.T) {
	const batch = 4
	w := newWgTunDevice(&fakeWgDevice{})
	bufs := make([][]byte, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunHeaderOffset+64)
	}
	sizes := make([]int, batch)

	n, err := w.ReadBatch(bufs, sizes)
	if err != nil {
		t.Fatalf("ReadBatch returned err %v; want nil", err)
	}
	if n != batch {
		t.Fatalf("ReadBatch kept %d segments; want %d - every populated buffer must survive the error", n, batch)
	}
	for i := 0; i < n; i++ {
		if sizes[i] != 4 {
			t.Errorf("segment %d: size %d; want 4", i, sizes[i])
		}
		if got := bufs[i][tunHeaderOffset]; got != 0x45 {
			t.Errorf("segment %d: first byte 0x%02x at tunHeaderOffset; want 0x45", i, got)
		}
	}
}

// fakeOverflowWgDevice fails with ErrTooManySegments in whatever shape the
// test picks: it writes filled buffers and claims report of them. The pinned
// gsoSplit is always exactly one short of a full batch; this covers the
// upstreams that wouldn't be.
type fakeOverflowWgDevice struct {
	filled int
	report int
}

func (d *fakeOverflowWgDevice) File() *os.File { return nil }

func (d *fakeOverflowWgDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	pkt := []byte{0x45, 0x00, 0x00, 0x14}
	for i := 0; i < d.filled; i++ {
		sizes[i] = copy(bufs[i][offset:], pkt)
	}
	return d.report, tun.ErrTooManySegments
}

func (d *fakeOverflowWgDevice) Write(bufs [][]byte, offset int) (int, error) { return len(bufs), nil }
func (d *fakeOverflowWgDevice) MTU() (int, error)                            { return 1500, nil }
func (d *fakeOverflowWgDevice) Name() (string, error)                        { return "faketun", nil }
func (d *fakeOverflowWgDevice) Events() <-chan tun.Event                     { return nil }
func (d *fakeOverflowWgDevice) Close() error                                 { return nil }
func (d *fakeOverflowWgDevice) BatchSize() int                               { return 4 }

// TestWgTunDeviceErrTooManySegmentsNeverInventsASegment checks that the +1
// only fires on the pinned split's off-by-one. Every other shape is taken at
// face value: handing back a buffer nobody wrote trades a lost packet for a
// stale one, which is the worse bug.
func TestWgTunDeviceErrTooManySegmentsNeverInventsASegment(t *testing.T) {
	const batch = 4
	tests := []struct {
		name           string
		filled, report int
		want           int
	}{
		{"upstream corrects the off-by-one", batch, batch, batch},
		{"upstream short by more than one", 2, 1, 1},
		{"upstream over-reports", 2, 3, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newWgTunDevice(&fakeOverflowWgDevice{filled: tc.filled, report: tc.report})
			bufs := make([][]byte, batch)
			for i := range bufs {
				bufs[i] = make([]byte, tunHeaderOffset+64)
			}

			n, err := w.ReadBatch(bufs, make([]int, batch))
			if err != nil {
				t.Fatalf("ReadBatch returned err %v; want nil", err)
			}
			if n != tc.want {
				t.Fatalf("ReadBatch returned %d segments; want %d", n, tc.want)
			}
		})
	}
}

// TestWgTunDeviceErrTooManySegmentsIgnoresStaleSizes checks that the +1 reads
// a size this read wrote, not one left over from the last. ingoingBatch reuses
// its sizes slice across reads, so without the clear an untouched buffer
// inherits a plausible length and passes for a packet.
func TestWgTunDeviceErrTooManySegmentsIgnoresStaleSizes(t *testing.T) {
	const batch = 4
	bufs := make([][]byte, batch)
	for i := range bufs {
		bufs[i] = make([]byte, tunHeaderOffset+64)
	}
	sizes := make([]int, batch)

	// Prime the slice so the last slot holds a stale, non-zero size.
	w := newWgTunDevice(&fakeWgDevice{})
	if n, err := w.ReadBatch(bufs, sizes); err != nil || n != batch {
		t.Fatalf("priming read: got (%d, %v); want (%d, nil)", n, err, batch)
	}
	if sizes[batch-1] == 0 {
		t.Fatal("priming read left the last size at 0 - it can no longer go stale")
	}

	// This one stops short without writing the last buffer, yet reports the
	// same count as the off-by-one case: only the size entry tells them apart.
	w = newWgTunDevice(&fakeOverflowWgDevice{filled: batch - 1, report: batch - 1})
	n, err := w.ReadBatch(bufs, sizes)
	if err != nil {
		t.Fatalf("ReadBatch returned err %v; want nil", err)
	}
	if n != batch-1 {
		t.Fatalf("ReadBatch returned %d segments; want %d - the untouched buffer must not be reclaimed on a stale size", n, batch-1)
	}
}

// The TUN write path stops coalescing once the head buffer's capacity runs
// out, so the batch buffers have to fit a whole GRO superpacket, not just one
// packet. Easy to lose to a well-meaning memory tweak, hence the test.
func TestBatchEnvelopesFitAGROSuperpacket(t *testing.T) {
	const wantCap = tunHeaderOffset + 65535

	out := newOutgoingBatch(4)
	for i, env := range out.envelopes {
		if cap(env) < wantCap {
			t.Errorf("outgoing envelope %d: cap %d; want >= %d", i, cap(env), wantCap)
		}
		if cap(out.bufs[i]) < wantCap {
			t.Errorf("outgoing bufs %d: cap %d; want >= %d", i, cap(out.bufs[i]), wantCap)
		}
	}

	in := newIngoingBatch(4)
	for i, env := range in.envelopes {
		if cap(env) < wantCap {
			t.Errorf("ingoing envelope %d: cap %d; want >= %d", i, cap(env), wantCap)
		}
		// bufs is envelope sliced past tunHeaderOffset, so its cap is smaller
		// by exactly that much; it should still cover a full superpacket.
		if cap(in.bufs[i]) < 65535 {
			t.Errorf("ingoing bufs %d: cap %d; want >= %d", i, cap(in.bufs[i]), 65535)
		}
	}
}

// TestUDPTunnelSocketDualStackReceive checks that udpTunnelSocket's
// ipv6.PacketConn wrapper receives both IPv4 and IPv6 loopback datagrams
// over the same dual-stack listen socket.
func TestUDPTunnelSocketDualStackReceive(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port

	sock := newUDPTunnelSocket(conn)
	defer sock.Close()

	v4, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatalf("dial v4: %v", err)
	}
	defer v4.Close()
	v6, err := net.DialUDP("udp6", nil, &net.UDPAddr{IP: net.ParseIP("::1"), Port: port})
	if err != nil {
		t.Fatalf("dial v6: %v", err)
	}
	defer v6.Close()

	if _, err := v4.Write([]byte("hello-v4")); err != nil {
		t.Fatalf("write v4: %v", err)
	}
	if _, err := v6.Write([]byte("hello-v6")); err != nil {
		t.Fatalf("write v6: %v", err)
	}

	bufs := [][]byte{make([]byte, 64), make([]byte, 64)}
	sizes := make([]int, 2)
	got := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for len(got) < 2 && time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := sock.ReadBatch(bufs, sizes)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			t.Fatalf("ReadBatch: %v", err)
		}
		for i := 0; i < n; i++ {
			got[string(bufs[i][:sizes[i]])] = true
		}
	}

	if !got["hello-v4"] || !got["hello-v6"] {
		t.Fatalf("expected both v4 and v6 datagrams through one dual-stack socket; got %v", got)
	}
}

// scriptedReadErrors hands out one error per read, in order.
type scriptedReadErrors struct {
	mu   sync.Mutex
	errs []error
	used int
}

func (s *scriptedReadErrors) next() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.errs[s.used]
	s.used++
	return err
}

func (s *scriptedReadErrors) reads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

type failingTunnelSocket struct {
	fakeTunnelSocket
	scriptedReadErrors
}

func (f *failingTunnelSocket) ReadBatch([][]byte, []int) (int, error) { return 0, f.next() }

type failingTunDevice struct {
	fakeTunDevice
	scriptedReadErrors
}

func (f *failingTunDevice) ReadBatch([][]byte, []int) (int, error) { return 0, f.next() }

func waitForLoopExit(t *testing.T, loop func()) {
	t.Helper()
	var exited atomic.Bool
	go func() {
		loop()
		exited.Store(true)
	}()
	waitFor(t, "read loop to exit", exited.Load)
}

// Nothing supervises the read loops, so a read error must not end the datapath:
// only a closed source stops them.
func TestIngoingLoopSurvivesReadErrorsUntilClosed(t *testing.T) {
	sock := &failingTunnelSocket{}
	sock.errs = []error{errors.New("transient"), net.ErrClosed}
	tunnel := &Tunnel{sock: sock, tun: &fakeTunDevice{batchSize: 1}}

	waitForLoopExit(t, tunnel.ingoingLoop)

	if got := sock.reads(); got != 2 {
		t.Errorf("loop read %d times, want 2", got)
	}
}

func TestOutgoingLoopSurvivesReadErrorsUntilClosed(t *testing.T) {
	dev := &failingTunDevice{}
	dev.batchSize = 1
	dev.errs = []error{errors.New("transient"), os.ErrClosed}
	tunnel := &Tunnel{tun: dev}

	waitForLoopExit(t, tunnel.outgoingLoop)

	if got := dev.reads(); got != 2 {
		t.Errorf("loop read %d times, want 2", got)
	}
}

// The error a real close produces has to be one keepReadingAfter recognises,
// or the loop retries a dead socket forever.
func TestIngoingLoopExitsWhenRealSocketCloses(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	sock := newUDPTunnelSocket(conn)
	tunnel := &Tunnel{sock: sock, tun: &fakeTunDevice{batchSize: 1}}

	time.AfterFunc(50*time.Millisecond, func() { _ = sock.Close() })
	waitForLoopExit(t, tunnel.ingoingLoop)
}

type eventingWgDevice struct {
	fakeWgDevice
	events chan tun.Event
}

func (d *eventingWgDevice) Events() <-chan tun.Event { return d.events }

// wireguard-go's status goroutines block once the 5-slot channel is full, so
// more events than that must get through.
func TestLogTunEventsDrainsUntilDeviceCloses(t *testing.T) {
	dev := &eventingWgDevice{events: make(chan tun.Event, 5)}
	go func() {
		for i := 0; i < 20; i++ {
			dev.events <- tun.Event(tun.EventUp << (i % 3))
		}
		close(dev.events)
	}()

	waitForLoopExit(t, func() { logTunEvents(dev) })

	if _, open := <-dev.events; open {
		t.Error("logTunEvents returned before the device closed its event channel")
	}
}
