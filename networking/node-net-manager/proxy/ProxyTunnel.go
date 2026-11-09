package proxy

import (
	"NetManager/clock"
	"NetManager/logger"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// packetReadBufferSize is the size of Emit's pooled per-packet buffers. Kept
// separate from batchPacketCap even though they're nearly the same: one is a
// pool buffer for the replay path, the other the fixed per-loop set.
const packetReadBufferSize = 64 * 1024

// batchPacketCap is the per-packet capacity of the buffers both batched read
// loops hold for the process lifetime (ingoingBatch/outgoingBatch).
//
// This has to be a full IP packet, not an MTU. The TUN write path
// (wireguard-go's Write on Linux) does GRO: same-flow TCP segments get
// coalesced into one superpacket by appending onto the head packet's buffer
// in place. It never grows that buffer; once the capacity is used up it
// just starts a new superpacket, and every superpacket is a separate
// write(2). MTU-sized buffers silently cap that at a handful of segments per
// write; 65535 is the most an IP packet can hold, so it's where the cap
// stops mattering.
//
// Mostly virtual memory: pages only get touched as GRO fills them, and the
// outgoing loop's reads never write past one MTU-sized segment per buffer.
const batchPacketCap = 65535

// socketBufferSize is the requested SO_RCVBUF/SO_SNDBUF size for the tunnel
// UDP sockets, so a packet burst has room to queue instead of being dropped.
const socketBufferSize = 4 * 1024 * 1024

// Idle tunnel sockets are closed rather than kept for the process lifetime:
// every distinct peer node ever talked to would otherwise hold a descriptor
// and a socket buffer allocation forever.
const (
	connectionIdleTimeout   = 5 * time.Minute
	connectionSweepInterval = time.Minute
)

// readErrorBackoff paces a read loop whose source keeps failing, so a broken
// device costs a few log lines per second rather than a spinning core.
const readErrorBackoff = 100 * time.Millisecond

// Config
type Configuration struct {
	HostTUNDeviceName         string `json:"HostTunnelDeviceName"`
	ProxySubnetwork           string `json:"ProxySubnetwork"`
	ProxySubnetworkMask       string `json:"ProxySubnetworkMask"`
	TunNetIP                  string `json:"TunnelIP"`
	TunnelPort                int    `json:"TunnelPort"`
	Mtusize                   int    `json:"MTUSize"`
	TunNetIPv6                string `json:"TunNetIPv6"`
	ProxySubnetworkIPv6       string `json:"ProxySubnetworkIPv6"`
	ProxySubnetworkIPv6Prefix int    `json:"ProxySubnetworkIPv6Prefix"`
}

// batchWriter is the common surface of *ipv4.PacketConn and *ipv6.PacketConn.
// ipv4.Message and ipv6.Message alias the same underlying type, so both
// wrappers satisfy this with no bridging needed (see newTunnelConn).
type batchWriter interface {
	WriteBatch(ms []ipv4.Message, flags int) (int, error)
}

// tunnelConn is one UDP socket to a peer node, plus the coarse timestamp the
// idle sweeper uses to decide when to close it and a batched writer for
// grouped outgoing sends (see Tunnel.sendOverTunnelBatch).
type tunnelConn struct {
	conn     *net.UDPConn
	lastUsed atomic.Int64
	// batch is conn wrapped for WriteBatch; see newTunnelConn for why the
	// wrapper's family has to match dst rather than always being one or the
	// other.
	batch batchWriter
}

// Tunnel owns the TUN device, the listen socket, the per-peer connection pool
// and its idle eviction, and the read loops: every bit of actual socket/TUN
// I/O. It performs no translation decisions itself; those belong to Datapath,
// which it drives via Handle/Emit.
type Tunnel struct {
	dp *Datapath
	// sock is only used for the batched ingoing read (see ingoingLoop).
	// Sending to a peer goes through connectionBuffer's per-peer dialled
	// connections instead (see sendOverTunnelBatch), not this socket;
	// otherwise outgoing traffic would be sourced from the listen port
	// instead of an ephemeral one, which firewalls and NAT treat differently.
	sock              TunnelSocket
	connectionBuffer  map[netip.AddrPort]*tunnelConn
	tun               TunDevice
	HostTUNDeviceName string
	tunNetIPv6        string
	tunNetIP          string
	mtu               int
	TunnelPort        int
	// connectionBufferLock guards only connectionBuffer. net.UDPConn is
	// itself safe for concurrent use, so nothing else needs to hold this
	// while a packet is actually being written.
	connectionBufferLock sync.RWMutex
}

// packetBufPool holds MTU-sized buffers for Emit's ActionDeliver copies, so
// it doesn't allocate one per packet. The batched read loops don't use this
// pool; their buffers stay put across a whole batch instead (see
// ingoingBatch/outgoingBatch). Buffers reserve tunHeaderOffset bytes up
// front, same as TunDevice's own buffers.
var packetBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, tunHeaderOffset+packetReadBufferSize)
		return &b
	},
}

func getPacketBuf() *[]byte {
	return packetBufPool.Get().(*[]byte)
}

func putPacketBuf(b *[]byte) {
	*b = (*b)[:cap(*b)]
	packetBufPool.Put(b)
}

// Emit performs the Action the Datapath decided on. Only the replay
// goroutine calls this (see Sink); the read loops call Handle directly and
// batch the result themselves (runIngoingBatch, runOutgoingBatch). Unlike
// those, action.Packet here is a bare copy with no header room reserved (see
// Datapath.enqueueReplayLocked), so it's copied onto a pooled buffer that has
// it before writing.
func (t *Tunnel) Emit(action Action) {
	switch action.Kind {
	case ActionDrop:
		return
	case ActionDeliver:
		buf := getPacketBuf()
		n := copy((*buf)[tunHeaderOffset:], action.Packet)
		if _, err := t.tun.WriteBatch([][]byte{(*buf)[:tunHeaderOffset+n]}); err != nil {
			logger.ErrorLogger().Println(err)
		}
		putPacketBuf(buf)
	case ActionForward:
		t.sendOverTunnel(action.Dst, action.Packet, 0)
	}
}

// keepReadingAfter logs a failed batch read and reports whether the read loop
// should carry on. Nothing supervises these loops, so handing the error to a
// channel nobody reads would wedge the datapath on the first failure.
//
// An isolated failure retries at once: this is the direction's only reader, so
// sleeping on one bad packet would stall every flow behind it. A closed source
// can never recover and retrying would spin forever, but nothing closes one on
// purpose, so losing it is an error rather than a clean stop.
func keepReadingAfter(err error, consecutiveFailures int) bool {
	if errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
		logger.ErrorLogger().Println("Tunnel read loop stopping, source closed:", err)
		return false
	}
	logger.ErrorLogger().Println("Tunnel read failed:", err)
	if consecutiveFailures > 1 {
		time.Sleep(readErrorBackoff)
	}
	return true
}

// readUntilClosed drives one direction's read loop, running a batch at a time
// until keepReadingAfter gives up on the source.
func readUntilClosed(runBatch func() error) {
	failures := 0
	for {
		err := runBatch()
		if err == nil {
			failures = 0
			continue
		}
		failures++
		if !keepReadingAfter(err, failures) {
			return
		}
	}
}

// outgoingBatchGroup collects one outgoingBatch's packets bound for a single
// remote destination, so sendOverTunnelBatch resolves dst's tunnelConn once
// per group rather than once per packet.
type outgoingBatchGroup struct {
	dst  netip.AddrPort
	bufs [][]byte
}

// outgoingBatch holds the fixed set of buffers the outgoing loop reuses
// across every read (see batchPacketCap), plus the state that groups one
// read's ActionForward packets by destination.
type outgoingBatch struct {
	envelopes [][]byte
	bufs      [][]byte
	sizes     []int

	deliverable [][]byte // ActionDeliver packets, collected for one tun.WriteBatch

	// groups[:liveGroups] are the destinations seen in the batch just read,
	// in first-seen order. Found by linear scan rather than a map keyed on
	// the destination: one TUN read's packets are bound for a handful of
	// peer nodes at most, and comparing that many netip.AddrPorts costs less
	// than hashing one. Retired slots past liveGroups are kept, not
	// discarded, so their bufs capacity is reused by the next batch instead
	// of reallocated.
	groups     []outgoingBatchGroup
	liveGroups int

	msgs []ipv4.Message // scratch for sendOverTunnelBatch; see writeMessages
}

func newOutgoingBatch(size int) *outgoingBatch {
	if size < 1 {
		size = 1
	}
	b := &outgoingBatch{
		envelopes:   make([][]byte, size),
		bufs:        make([][]byte, size),
		sizes:       make([]int, size),
		deliverable: make([][]byte, 0, size),
	}
	for i := range b.envelopes {
		b.envelopes[i] = make([]byte, tunHeaderOffset+batchPacketCap)
		// TunDevice.ReadBatch wants the full envelope, offset included,
		// unlike TunnelSocket.ReadBatch, which has no offset to apply.
		b.bufs[i] = b.envelopes[i]
	}
	return b
}

// group appends packet to the batch's group for dst, opening or recycling a
// group slot as needed. packet aliases one of b.envelopes' buffers, so it's
// only valid until the next ReadBatch; callers must drain the groups before
// then.
func (b *outgoingBatch) group(dst netip.AddrPort, packet []byte) {
	for i := 0; i < b.liveGroups; i++ {
		if b.groups[i].dst == dst {
			b.groups[i].bufs = append(b.groups[i].bufs, packet)
			return
		}
	}
	if b.liveGroups == len(b.groups) {
		b.groups = append(b.groups, outgoingBatchGroup{})
	}
	g := &b.groups[b.liveGroups]
	g.dst = dst
	g.bufs = append(g.bufs[:0], packet)
	b.liveGroups++
}

// writeMessages resizes b.msgs to len(bufs) and points each message's single
// buffer at the corresponding packet. Addr is left nil: every destination
// here is a connected tunnelConn, so WriteBatch always sends to the peer
// it's dialled to. The slices are reused across calls; safe since
// sendOverTunnelBatch only ever runs on the outgoing-loop goroutine.
func (b *outgoingBatch) writeMessages(bufs [][]byte) []ipv4.Message {
	for len(b.msgs) < len(bufs) {
		b.msgs = append(b.msgs, ipv4.Message{Buffers: make([][]byte, 1)})
	}
	msgs := b.msgs[:len(bufs)]
	for i, buf := range bufs {
		msgs[i].Buffers[0] = buf
		msgs[i].Addr = nil
		msgs[i].N = 0
	}
	return msgs
}

// runOutgoingBatch performs one read-decide-send cycle: one TunDevice read,
// Datapath.Handle for everything it returned, at most one TunDevice.WriteBatch
// for local delivery, and one tunnel write per distinct remote destination. A
// send/write error is only logged here; sendOverTunnelBatch handles its own
// redial and retry.
func (t *Tunnel) runOutgoingBatch(b *outgoingBatch) error {
	n, err := t.tun.ReadBatch(b.bufs, b.sizes)
	if err != nil {
		return err
	}

	b.deliverable = b.deliverable[:0]
	b.liveGroups = 0

	for i := 0; i < n; i++ {
		packet := b.envelopes[i][tunHeaderOffset : tunHeaderOffset+b.sizes[i]]
		switch action := t.dp.Handle(Outgoing, packet); action.Kind {
		case ActionDrop:
		case ActionDeliver:
			// packet already sits in the envelope's header room, so this can
			// go straight back with no copy (unlike Emit's ActionDeliver case).
			b.deliverable = append(b.deliverable, b.envelopes[i][:tunHeaderOffset+b.sizes[i]])
		case ActionForward:
			b.group(action.Dst, action.Packet)
		default:
			logger.ErrorLogger().Println("outgoing datapath returned an unexpected action:", action.Kind)
		}
	}

	if len(b.deliverable) > 0 {
		if _, err := t.tun.WriteBatch(b.deliverable); err != nil {
			logger.ErrorLogger().Println(err)
		}
	}

	// Drained after the delivery write: sendOverTunnelBatch can block briefly
	// on redial, and there's no reason to hold the TUN write on that.
	for i := 0; i < b.liveGroups; i++ {
		g := &b.groups[i]
		t.sendOverTunnelBatch(g.dst, g.bufs, b, 0)
	}
	return nil
}

// outgoingLoop reads batches of packets off the TUN device and forwards them
// via runOutgoingBatch.
func (t *Tunnel) outgoingLoop() {
	batch := newOutgoingBatch(t.tun.BatchSize())
	readUntilClosed(func() error { return t.runOutgoingBatch(batch) })
}

// ingoingBatch holds the fixed set of buffers the ingoing loop reuses across
// every read (see batchPacketCap). envelopes are the full buffers, headroom
// included; bufs are the same buffers pre-sliced past the headroom, which is
// all TunnelSocket needs to see.
type ingoingBatch struct {
	envelopes   [][]byte
	bufs        [][]byte
	sizes       []int
	deliverable [][]byte
}

func newIngoingBatch(size int) *ingoingBatch {
	if size < 1 {
		size = 1
	}
	b := &ingoingBatch{
		envelopes:   make([][]byte, size),
		bufs:        make([][]byte, size),
		sizes:       make([]int, size),
		deliverable: make([][]byte, 0, size),
	}
	for i := range b.envelopes {
		b.envelopes[i] = make([]byte, tunHeaderOffset+batchPacketCap)
		b.bufs[i] = b.envelopes[i][tunHeaderOffset:]
	}
	return b
}

// runIngoingBatch performs one read-decide-write cycle: one TunnelSocket
// read, Datapath.Handle for everything it returned, and at most one
// TunDevice write for everything that needed delivering. A write error is
// only logged.
func (t *Tunnel) runIngoingBatch(b *ingoingBatch) error {
	n, err := t.sock.ReadBatch(b.bufs, b.sizes)
	if err != nil {
		return err
	}

	b.deliverable = b.deliverable[:0]
	for i := 0; i < n; i++ {
		packet := b.envelopes[i][tunHeaderOffset : tunHeaderOffset+b.sizes[i]]
		switch action := t.dp.Handle(Ingoing, packet); action.Kind {
		case ActionDrop:
		case ActionDeliver:
			b.deliverable = append(b.deliverable, b.envelopes[i][:tunHeaderOffset+b.sizes[i]])
		default:
			logger.ErrorLogger().Println("ingoing datapath returned an unexpected action:", action.Kind)
		}
	}

	if len(b.deliverable) > 0 {
		if _, err := t.tun.WriteBatch(b.deliverable); err != nil {
			logger.ErrorLogger().Println(err)
		}
	}
	return nil
}

// ingoingLoop reads batches of packets off the tunnel UDP socket and, for
// whatever needs delivering to the TUN device, writes them back in a single
// batched call; see runIngoingBatch.
func (t *Tunnel) ingoingLoop() {
	batch := newIngoingBatch(t.tun.BatchSize())
	readUntilClosed(func() error { return t.runIngoingBatch(batch) })
}

// connFor resolves dst's tunnelConn, dialling and storing a new one if none
// exists yet. Shared by sendOverTunnel and sendOverTunnelBatch so both agree
// on how a peer connection comes into existence.
func (t *Tunnel) connFor(dst netip.AddrPort) (*tunnelConn, error) {
	t.connectionBufferLock.RLock()
	con, exist := t.connectionBuffer[dst]
	t.connectionBufferLock.RUnlock()
	if exist {
		return con, nil
	}
	return t.dialAndStore(dst)
}

// evictDeadConn removes con from connectionBuffer, but only if it's still the
// entry for dst; another goroutine may already have replaced it with a live
// connection by the time a failed write gets here.
func (t *Tunnel) evictDeadConn(dst netip.AddrPort, con *tunnelConn) {
	t.connectionBufferLock.Lock()
	if t.connectionBuffer[dst] == con {
		delete(t.connectionBuffer, dst)
	}
	t.connectionBufferLock.Unlock()
}

// sendOverTunnel sends packetBytes to dst over the tunnel. dst is always a
// real remote peer; Datapath.forwardResult handles local delivery and
// invalid hosts before an Action gets here. Unlike runOutgoingBatch, this can
// run on several replay goroutines at once, so it gets its own scratch
// outgoingBatch rather than sharing runOutgoingBatch's.
func (t *Tunnel) sendOverTunnel(dst netip.AddrPort, packetBytes []byte, attemptNumber int) {
	t.sendOverTunnelBatch(dst, [][]byte{packetBytes}, &outgoingBatch{}, attemptNumber)
}

// sendOverTunnelBatch sends bufs to dst, resolving dst's tunnelConn once for
// the whole group rather than once per packet. scratch is reused by the
// caller across every destination and batch, so this never allocates in
// steady state.
//
// A write failure evicts the dead connection, dials a fresh one, and retries
// whatever didn't make it out, up to a 10-retry cap, so one peer being down
// only costs that peer's group latency, not the rest of the batch.
func (t *Tunnel) sendOverTunnelBatch(dst netip.AddrPort, bufs [][]byte, scratch *outgoingBatch, attemptNumber int) {
	if attemptNumber > 10 || len(bufs) == 0 {
		return
	}

	con, err := t.connFor(dst)
	if err != nil {
		return
	}
	con.lastUsed.Store(clock.Unix())

	for len(bufs) > 0 {
		sent, err := con.batch.WriteBatch(scratch.writeMessages(bufs), 0)
		// sendmmsg(2) returns -1 on failure and WriteBatch hands that straight
		// back, so slicing by it panics. Nothing went out, so the retry below
		// resends the whole group. Not an exotic path either: a restarted
		// peer replies ICMP port-unreachable, which lands here as
		// ECONNREFUSED on the next send.
		if sent > 0 {
			bufs = bufs[sent:]
		}
		if err == nil {
			if sent == 0 {
				// A silent no-op is only documented to happen on error;
				// bail rather than spin on the rest of the group forever.
				return
			}
			continue
		}

		_ = con.conn.Close()
		logger.ErrorLogger().Println(err)
		t.evictDeadConn(dst, con)

		if _, err := t.dialAndStore(dst); err != nil {
			return
		}
		// Retry only what didn't make it out before the failure.
		t.sendOverTunnelBatch(dst, bufs, scratch, attemptNumber+1)
		return
	}
}

// dialAndStore installs a fresh connection for key, unless another goroutine
// won the race to create one first; dropping a duplicate on the floor would
// leak its socket.
func (t *Tunnel) dialAndStore(key netip.AddrPort) (*tunnelConn, error) {
	connection, err := createUDPChannel(key)
	if err != nil {
		return nil, err
	}

	t.connectionBufferLock.Lock()
	defer t.connectionBufferLock.Unlock()
	if existing, exist := t.connectionBuffer[key]; exist {
		_ = connection.Close()
		return existing, nil
	}
	tc := newTunnelConn(connection, key)
	t.connectionBuffer[key] = tc
	return tc, nil
}

// newTunnelConn wraps a freshly dialled peer connection with the batched
// writer its address family needs. Unlike the listen socket (always
// dual-stack, see udpTunnelSocket), a net.DialUDP connection is single-family,
// and dst.Addr() is never a v4-mapped v6 address here (see
// TableEntryCache.AddrFromIP's Unmap), so Is4() reliably tells the two apart.
func newTunnelConn(conn *net.UDPConn, dst netip.AddrPort) *tunnelConn {
	var bw batchWriter
	if dst.Addr().Is4() {
		bw = ipv4.NewPacketConn(conn)
	} else {
		bw = ipv6.NewPacketConn(conn)
	}
	tc := &tunnelConn{conn: conn, batch: bw}
	tc.lastUsed.Store(clock.Unix())
	return tc
}

// startConnectionEviction periodically closes tunnel sockets that have gone
// unused, so talking to a node once doesn't hold its descriptor forever.
func (t *Tunnel) startConnectionEviction() {
	ticker := time.NewTicker(connectionSweepInterval)
	go func() {
		for range ticker.C {
			t.evictIdleConnections(connectionIdleTimeout)
		}
	}()
}

func (t *Tunnel) evictIdleConnections(timeout time.Duration) {
	cutoff := clock.Unix() - int64(timeout.Seconds())

	// Collect first, then close outside the lock: Close can block, and the
	// datapath needs this map back.
	var idle []*tunnelConn
	t.connectionBufferLock.Lock()
	for key, con := range t.connectionBuffer {
		if con.lastUsed.Load() > cutoff {
			continue
		}
		// Only remove the connection actually observed as idle, never a
		// replacement dialled for the same peer since.
		if t.connectionBuffer[key] == con {
			delete(t.connectionBuffer, key)
			idle = append(idle, con)
		}
	}
	t.connectionBufferLock.Unlock()

	for _, con := range idle {
		_ = con.conn.Close()
	}
}

func createUDPChannel(raddr netip.AddrPort) (*net.UDPConn, error) {
	connection, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(raddr))
	if nil != err {
		logger.ErrorLogger().Println("Unable to connect to remote addr:", err)
		return nil, err
	}
	err = connection.SetWriteBuffer(socketBufferSize)
	if nil != err {
		// Not fatal: the socket still works with whatever the kernel's
		// default/clamped size is, just more prone to drops under bursts.
		logger.ErrorLogger().Println("Unable to grow UDP write buffer:", err)
	}
	return connection, nil
}

func (t *Tunnel) GetName() string {
	return t.HostTUNDeviceName
}
