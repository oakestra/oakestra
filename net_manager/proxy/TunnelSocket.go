package proxy

import (
	"net"

	"golang.org/x/net/ipv6"
)

// TunnelSocket abstracts the tunnel's listen socket: batched reads off it
// (recvmmsg, where the kernel supports it). Sending to a peer never goes
// through this socket: every outgoing send has a specific peer, and that
// traffic goes out over connectionBuffer's per-peer dialled connections
// instead (see Tunnel.sendOverTunnelBatch and tunnelConn.batch).
type TunnelSocket interface {
	ReadBatch(bufs [][]byte, sizes []int) (int, error)
	Close() error
}

// udpTunnelSocket backs TunnelSocket with the process's single UDP listen
// socket.
//
// net.ListenUDP("udp", ...) binds a dual-stack AF_INET6 socket on both Linux
// and Darwin: a bare port with no host resolves to the unspecified address,
// and "udp" (rather than "udp4"/"udp6") leaves IPV6_V6ONLY off, so v4 peers
// arrive as v4-mapped v6 addresses on the same socket. Wrap it in
// ipv6.NewPacketConn, not ipv4's: ipv4.NewPacketConn only sees the raw v6
// addresses and fails to read the v4-mapped ones. See
// TestUDPTunnelSocketDualStackReceive.
type udpTunnelSocket struct {
	conn *net.UDPConn
	pc   *ipv6.PacketConn
	// msgs is reused across ReadBatch calls. This socket has a single
	// reader (see ingoingLoop), so batching doesn't cost an allocation per
	// call. It grows to fit the largest bufs any caller has passed so far.
	msgs []ipv6.Message
}

func newUDPTunnelSocket(conn *net.UDPConn) *udpTunnelSocket {
	return &udpTunnelSocket{conn: conn, pc: ipv6.NewPacketConn(conn)}
}

func (s *udpTunnelSocket) ReadBatch(bufs [][]byte, sizes []int) (int, error) {
	if len(s.msgs) < len(bufs) {
		msgs := make([]ipv6.Message, len(bufs))
		for i := range msgs {
			msgs[i].Buffers = make([][]byte, 1)
		}
		s.msgs = msgs
	}
	msgs := s.msgs[:len(bufs)]
	for i, b := range bufs {
		msgs[i].Buffers[0] = b
	}

	n, err := s.pc.ReadBatch(msgs, 0)
	if err != nil {
		return 0, err
	}
	for i := 0; i < n; i++ {
		sizes[i] = msgs[i].N
	}
	return n, nil
}

func (s *udpTunnelSocket) Close() error {
	return s.conn.Close()
}
