package proxy

import (
	"NetManager/TableEntryCache"
	"NetManager/logger"
	"NetManager/proxy/iputils"
	"NetManager/resolver"
	"math/rand/v2"
	"net/netip"
	"sync"
)

// Direction says which side of the tunnel a packet was read from.
type Direction uint8

const (
	Outgoing Direction = iota // read off the TUN device
	Ingoing                   // read off the tunnel socket
)

// ActionKind says what the caller should do with the Action's Packet.
type ActionKind uint8

const (
	ActionDrop    ActionKind = iota
	ActionForward            // send over the tunnel to Dst
	ActionDeliver            // write to the TUN device
)

// Action is the datapath's verdict on one packet: what to do with it, and,
// for ActionForward, where to send it. Packet aliases the caller's buffer
// rather than copying it, so the caller must not reuse that buffer until the
// Action has been acted on.
type Action struct {
	Kind   ActionKind
	Dst    netip.AddrPort // ActionForward only
	Packet []byte
}

// Sink receives Actions produced off the synchronous path, today only from
// the replay goroutine (see replayWhenResolved).
type Sink interface {
	Emit(Action)
}

// Datapath decides what happens to a packet: translate it, queue it for
// replay, or drop it. It owns the flow cache, fragment state, replay queues,
// the resolver, the local IP and the proxy prefixes, and performs no socket
// or TUN I/O itself; that is Tunnel's job, driven by the Action this returns.
type Datapath struct {
	environment resolver.Resolver
	localIP     netip.Addr
	// netip.Prefix, not net.IPNet, so the per-packet containment check needs
	// no conversion of the parsed address.
	ProxyIPv4Prefix netip.Prefix
	ProxyIPv6Prefix netip.Prefix
	proxycache      *ProxyCache
	// out is where the replay goroutine (the only asynchronous producer of
	// Actions) sends what it decides. The synchronous path never uses it;
	// Handle returns its Action directly to the caller instead.
	out Sink
	// replayLock guards replays and replayBytes. Both are only touched on a
	// cold miss, never on the steady-state packet path.
	replayLock  sync.Mutex
	replays     map[netip.Addr]*pendingReplay
	replayBytes int
}

// NewDatapath builds a Datapath. out receives whatever the replay goroutine
// decides once a Service IP resolves; see Sink.
func NewDatapath(r resolver.Resolver, localIP netip.Addr, v4, v6 netip.Prefix, out Sink) *Datapath {
	return &Datapath{
		environment:     r,
		localIP:         localIP,
		ProxyIPv4Prefix: v4,
		ProxyIPv6Prefix: v6,
		proxycache:      NewProxyCache(),
		out:             out,
	}
}

// SetResolver replaces the resolver used to answer table lookups.
func (d *Datapath) SetResolver(r resolver.Resolver) {
	d.environment = r
}

// Handle decides what to do with one packet read from dir, returning the
// Action for the caller to perform. This is the entry point for both read
// loops, so the caller can batch the resulting I/O without a channel hop.
func (d *Datapath) Handle(dir Direction, buf []byte) Action {
	if dir == Outgoing {
		return d.handleOutgoing(buf, true)
	}
	return d.handleIngoing(buf)
}

// handleOutgoing decides what to do with one packet read from the TUN device:
// translate it (if it targets the semantic-routing subnetwork) and forward
// it on. mayRetain must be false when called from a replay goroutine itself,
// or a resolution that succeeds but still misses the table would re-enter
// and replay forever.
func (d *Datapath) handleOutgoing(buf []byte, mayRetain bool) Action {
	pkt, ok := iputils.Parse(buf)
	if !ok {
		return Action{Kind: ActionDrop}
	}
	if logger.IsDebug() {
		logger.DebugLogger().Printf("Outgoing packet:\t\t\t%s ---> %s\n", pkt.SrcIP(), pkt.DstIP())
	}

	// A later fragment carries no transport header, so it can only reuse the
	// translation its first fragment established; queue it behind that
	// fragment if the route is still pending, rather than dropping it.
	if isLaterFragment(&pkt) {
		if action, ok := d.forwardLaterFragment(&pkt); ok {
			return action
		}
		if mayRetain {
			d.retainFragmentForReplay(buf, pkt.DstIP())
		}
		return Action{Kind: ActionDrop}
	}
	if !pkt.HasTransport() {
		return Action{Kind: ActionDrop}
	}

	// The fragment key has to be taken before outgoingProxy rewrites the
	// addresses, so it matches the later fragments, which arrive untranslated.
	var fragKey fragmentKey
	firstFragment := pkt.IsFragment()
	if firstFragment {
		fragKey = keyFor(&pkt)
	}

	dstHost, dstPort, resolving, ok := d.outgoingProxy(&pkt)
	if !ok {
		if mayRetain && resolving != nil {
			d.retainForReplay(buf, pkt.DstIP(), resolving)
		}
		return Action{Kind: ActionDrop}
	}

	if firstFragment {
		d.proxycache.frags.remember(fragKey, fragmentTranslation{
			newSrc:      pkt.SrcIP(),
			newDst:      pkt.DstIP(),
			dstNode:     dstHost,
			dstNodePort: dstPort,
		})
	}
	return d.forwardResult(dstHost, dstPort, pkt.Bytes())
}

// forwardResult turns a resolved destination into the Action the caller
// should take. If dstHost is this machine, skip the network round trip and
// feed the packet straight into the ingoing pipeline.
func (d *Datapath) forwardResult(dstHost netip.Addr, dstPort int, packetBytes []byte) Action {
	if dstHost == d.localIP {
		if logger.IsDebug() {
			logger.DebugLogger().Println("Packet forwarded locally")
		}
		return d.handleIngoing(packetBytes)
	}

	return Action{
		Kind:   ActionForward,
		Dst:    netip.AddrPortFrom(dstHost, uint16(dstPort)),
		Packet: packetBytes,
	}
}

// forwardLaterFragment translates a non-first fragment using the state its
// first fragment left behind, sending it to the same node. ok is false when
// there is no such state.
func (d *Datapath) forwardLaterFragment(pkt *iputils.Packet) (action Action, ok bool) {
	translation, ok := d.proxycache.frags.lookup(keyFor(pkt))
	if !ok {
		return Action{}, false
	}
	if !pkt.Rewrite(translation.newSrc, translation.newDst) {
		return Action{}, false
	}
	return d.forwardResult(translation.dstNode, translation.dstNodePort, pkt.Bytes()), true
}

// isLaterFragment reports whether pkt is a fragment carrying no transport
// header. Non-TCP/UDP protocols are excluded: this proxy only ever translates
// those two, so fragment state is never kept for anything else.
func isLaterFragment(pkt *iputils.Packet) bool {
	if !pkt.IsFragment() || pkt.IsFirstFragment() {
		return false
	}
	return pkt.Protocol() == iputils.ProtoTCP || pkt.Protocol() == iputils.ProtoUDP
}

// maxReplayPacketsPerVIP bounds how many packets may queue behind one
// unresolved Service IP; maxReplayBytes bounds the total across all of them.
const (
	maxReplayPacketsPerVIP = 32
	maxReplayBytes         = 1 << 20
)

// pendingReplay is the FIFO of packets waiting on one Service IP's resolution.
// Guarded by Datapath.replayLock.
type pendingReplay struct {
	packets [][]byte
	bytes   int
}

// retainForReplay holds a copy of a packet whose ServiceIP is still being
// resolved and re-runs it once resolution finishes; without it a cold flow's
// first packet is just dropped, which silently loses a one-shot UDP datagram
// (TCP just retransmits). Packets queue per Service IP and replay in arrival
// order; one goroutine per packet, all waiting on the same channel, would
// leave replay order up to the scheduler instead.
func (d *Datapath) retainForReplay(buf []byte, vip netip.Addr, resolving <-chan struct{}) {
	d.replayLock.Lock()

	if queue, waiting := d.replays[vip]; waiting {
		d.enqueueReplayLocked(queue, buf)
		d.replayLock.Unlock()
		return
	}

	queue := &pendingReplay{}
	if !d.enqueueReplayLocked(queue, buf) {
		d.replayLock.Unlock()
		return
	}
	if d.replays == nil {
		d.replays = make(map[netip.Addr]*pendingReplay)
	}
	d.replays[vip] = queue
	d.replayLock.Unlock()

	// Exactly one waiter per Service IP, started when its queue is created.
	go d.replayWhenResolved(vip, resolving)
}

// retainFragmentForReplay queues a later fragment behind its datagram's first
// fragment, which is already waiting on this Service IP. A later fragment is
// still addressed to the untranslated VIP, so it keys into the same queue,
// and FIFO replay guarantees the first fragment installs the translation
// before these reach forwardLaterFragment again.
//
// Only appends to a queue that already exists: a later fragment carries no
// ports, so it can't start a resolution of its own, and with no first
// fragment already waiting there's nothing for it to stay consistent with;
// buffering it speculatively would just hold attacker-controllable bytes for
// a first fragment that may never come. outgoingLoop reads the TUN one packet
// at a time, so a datagram's own fragments always arrive in order; the only
// interleaving is with a replay in flight, which is why replayWhenResolved
// keeps the queue published until it drains.
func (d *Datapath) retainFragmentForReplay(buf []byte, vip netip.Addr) {
	d.replayLock.Lock()
	defer d.replayLock.Unlock()

	if queue, waiting := d.replays[vip]; waiting {
		d.enqueueReplayLocked(queue, buf)
	}
}

// enqueueReplayLocked copies buf onto queue unless the per-VIP packet cap or
// the global byte budget is already reached. buf is outgoingLoop's pool
// buffer and gets reused as soon as handleOutgoing returns, so it must be
// copied here. Drops the newest packet when full, rather than evicting the
// head, to avoid reordering what does get through. Caller must hold
// replayLock.
func (d *Datapath) enqueueReplayLocked(queue *pendingReplay, buf []byte) bool {
	if len(queue.packets) >= maxReplayPacketsPerVIP {
		return false
	}
	if d.replayBytes+len(buf) > maxReplayBytes {
		return false
	}
	queue.packets = append(queue.packets, append([]byte(nil), buf...))
	queue.bytes += len(buf)
	d.replayBytes += len(buf)
	return true
}

// replayWhenResolved waits for one Service IP's resolution attempt to finish
// and then re-runs everything queued behind it, in order, emitting the result
// of each through out. A failed attempt needs no special case: the replays
// miss the table again and are dropped, because they are not allowed to
// re-queue themselves.
func (d *Datapath) replayWhenResolved(vip netip.Addr, resolving <-chan struct{}) {
	<-resolving

	// Drain in rounds rather than detaching the queue up front: outgoingLoop
	// keeps reading the TUN while this runs, and a later fragment of a
	// datagram being replayed right now needs the queue still there to join.
	// Terminates because resolution has already finished by the time we get
	// here, so nothing new can join once a round finds the queue empty.
	for {
		d.replayLock.Lock()
		queue := d.replays[vip]
		if queue == nil || len(queue.packets) == 0 {
			delete(d.replays, vip)
			if queue != nil {
				d.replayBytes -= queue.bytes
			}
			d.replayLock.Unlock()
			return
		}
		packets := queue.packets
		d.replayBytes -= queue.bytes
		queue.packets, queue.bytes = nil, 0
		d.replayLock.Unlock()

		for _, packet := range packets {
			d.out.Emit(d.handleOutgoing(packet, false))
		}
	}
}

// handleIngoing processes one packet received on the tunnel UDP socket (or
// forwarded locally, see forwardResult): reverse-translate it if it matches
// an outstanding flow, then hand it back to be written to the TUN device.
// buf is only used for the duration of this call.
func (d *Datapath) handleIngoing(buf []byte) Action {
	pkt, ok := iputils.Parse(buf)
	if !ok {
		return Action{Kind: ActionDrop}
	}
	if logger.IsDebug() {
		logger.DebugLogger().Printf("Ingoing packet:\t\t\t %s <--- %s\n", pkt.DstIP(), pkt.SrcIP())
	}

	if isLaterFragment(&pkt) {
		// Unlike outgoing, an unknown fragment is still delivered to the TUN
		// device unchanged: ingoingProxy only matches flows this node
		// originated, so every inbound request fragment is unmatched by
		// design. Known gap: if the tunnel reorders and a later fragment
		// beats the first fragment that does get reverse-translated, the
		// datagram won't reassemble.
		if translation, known := d.proxycache.frags.lookup(keyFor(&pkt)); known {
			pkt.Rewrite(translation.newSrc, translation.newDst)
		}
	} else {
		if !pkt.HasTransport() {
			return Action{Kind: ActionDrop}
		}

		var fragKey fragmentKey
		firstFragment := pkt.IsFragment()
		if firstFragment {
			fragKey = keyFor(&pkt)
		}

		// No reverse mapping just means the packet is forwarded unchanged.
		if d.ingoingProxy(&pkt) && firstFragment {
			d.proxycache.frags.remember(fragKey, fragmentTranslation{
				newSrc: pkt.SrcIP(),
				newDst: pkt.DstIP(),
			})
		}
	}

	return Action{Kind: ActionDeliver, Packet: pkt.Bytes()}
}

// outgoingProxy rewrites pkt in place if its destination falls in the
// semantic-routing subnetwork, resolving the target instance via the
// per-flow ProxyCache and, only when that cache can't answer on its own,
// the translation table. ok is false if the packet should be dropped.
// resolving is non-nil only on a cold miss still being resolved in the
// background, letting the caller hold the packet for retry instead of losing
// it.
func (d *Datapath) outgoingProxy(pkt *iputils.Packet) (dstHost netip.Addr, dstPort int, resolving <-chan struct{}, ok bool) {
	dstIP := pkt.DstIP()
	version := pkt.Version()

	var inProxySubnet bool
	if version == 4 {
		inProxySubnet = d.ProxyIPv4Prefix.Contains(dstIP)
	} else {
		inProxySubnet = d.ProxyIPv6Prefix.Contains(dstIP)
	}
	if !inProxySubnet {
		return netip.Addr{}, 0, nil, false
	}

	key := FlowKey{
		Protocol:     pkt.Protocol(),
		SrcIP:        pkt.SrcIP(),
		DstServiceIP: dstIP,
		SrcPort:      int(pkt.SrcPort()),
		DstPort:      int(pkt.DstPort()),
	}

	// Steady state: the flow is cached and the translation table has not been
	// rebuilt since its route was chosen, so the route is known current and
	// this packet never touches the table at all. Everything below is the
	// cold path: a new flow, or a table that has moved on.
	var route Route
	if !d.proxycache.Lookup(&key, d.environment.TableGeneration(), &route) {
		route, resolving, ok = d.resolveRoute(key, version)
		if !ok {
			return netip.Addr{}, 0, resolving, false
		}
	}

	if !pkt.Rewrite(route.SrcInstanceIP, route.DstIP) {
		return netip.Addr{}, 0, nil, false
	}
	return route.DstNode, route.DstNodePort, nil, true
}

// resolveRoute picks the route for a flow Lookup couldn't answer for: either
// it has never been seen, or the table has changed since its route was
// chosen. Off the steady-state path, so it is the only place that consults
// the translation table.
func (d *Datapath) resolveRoute(key FlowKey, version uint8) (Route, <-chan struct{}, bool) {
	lookup := d.environment.GetTableEntryByServiceIP(key.DstServiceIP)
	if len(lookup.Entries) < 1 {
		return Route{}, lookup.Resolving, false
	}

	instanceIP, ok := d.convertToInstanceIp(version, key.SrcIP)
	if !ok {
		return Route{}, nil, false
	}

	// A flow that already exists keeps the replica it was pinned to, as long
	// as that replica is still in the table; only a genuinely new or broken
	// flow gets to pick.
	if route, revalidated := d.proxycache.Revalidate(key, instanceIP, version, lookup); revalidated {
		return route, nil, true
	}

	// TODO: only does round-robin so far; ServiceIP policies belong here.
	// rand.IntN is safe for concurrent use unlike a shared *rand.Rand,
	// needed since replay goroutines can call in here too.
	tableEntry := &lookup.Entries[rand.IntN(len(lookup.Entries))]

	entryDstIPnet := tableEntry.Nsip
	if version == 6 {
		entryDstIPnet = tableEntry.Nsipv6
	}
	entryDstIP, ok := TableEntryCache.AddrFromIP(entryDstIPnet)
	if !ok {
		return Route{}, nil, false
	}
	nodeAddr, ok := TableEntryCache.AddrFromIP(tableEntry.Nodeip)
	if !ok {
		return Route{}, nil, false
	}

	// dstNode/dstNodePort are cached here too, since an Nsip is only ever
	// valid on the node that issued it: no need to look it back up by
	// Nsip on every packet.
	route := Route{
		SrcInstanceIP: instanceIP,
		DstIP:         entryDstIP,
		DstNode:       nodeAddr,
		DstNodePort:   tableEntry.Nodeport,
	}
	d.proxycache.Install(key, route, tableEntry, version, lookup.Generation)
	return route, nil, true
}

// convertToInstanceIp resolves the stable "instance IP" that identifies
// srcIP's own service instance, for use as the translated source address.
func (d *Datapath) convertToInstanceIp(version uint8, srcIP netip.Addr) (netip.Addr, bool) {
	addr, ok := d.environment.GetInstanceIP(srcIP, version)
	if !ok {
		logger.ErrorLogger().Println("Unable to find instance IP for service: ", srcIP)
		return netip.Addr{}, false
	}
	return addr, true
}

// ingoingProxy checks the ProxyCache for a reverse mapping (a flow this node
// itself originated via outgoingProxy) and, if found, rewrites pkt in place
// back to its original semantic addressing. Returns false if there is no
// such mapping.
func (d *Datapath) ingoingProxy(pkt *iputils.Packet) bool {
	// The reply arrives addressed to the local namespace IP/port the flow
	// left from, sourced from the instance IP/port it was sent to.
	r, exist := d.proxycache.Reverse(
		pkt.Protocol(), pkt.DstIP(), int(pkt.DstPort()), pkt.SrcIP(), int(pkt.SrcPort()))
	if !exist {
		return false
	}

	return pkt.Rewrite(r.DstServiceIP, r.SrcIP)
}
