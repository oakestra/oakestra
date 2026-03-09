package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var (
	endpoint = flag.String(
		"endpoint",
		"unix:///var/lib/oakestra/csi/hostpath.sock",
		"CSI endpoint \u2013 unix:///path/to.sock or /path/to.sock",
	)
	nodeID = flag.String(
		"nodeid",
		"",
		"Node identifier returned by NodeGetInfo (optional)",
	)
)

func main() {
	flag.Parse()

	addr, network := parseEndpoint(*endpoint)

	// Initialize mount tracker for resilient cleanup
	stateDir := "/var/lib/oakestra/csi/state"
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		log.Fatalf("[hostpath] mkdir %s: %v", stateDir, err)
	}

	mountTracker, err := NewMountTracker(filepath.Join(stateDir, "mounts.json"))
	if err != nil {
		log.Fatalf("[hostpath] Failed to initialize mount tracker: %v", err)
	}

	// Clean up any orphaned mounts from previous runs/crashes
	mountTracker.CleanupOrphanedMounts()

	// For UNIX sockets: ensure the parent directory exists and remove any
	// stale socket left by a previous run.
	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(addr), 0750); err != nil {
			log.Fatalf("[hostpath] mkdir %s: %v", filepath.Dir(addr), err)
		}
		_ = os.Remove(addr)
	}

	lis, err := net.Listen(network, addr)
	if err != nil {
		log.Fatalf("[hostpath] listen %s://%s: %v", network, addr, err)
	}

	srv := newGRPCServer(mountTracker)

	go func() {
		log.Printf("[hostpath] CSI plugin listening on %s://%s", network, addr)
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("[hostpath] gRPC server error: %v", err)
		}
	}()

	// Block until SIGTERM / SIGINT, then shut down gracefully.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	log.Println("[hostpath] Shutting down \u2026")
	srv.GracefulStop()
	// Close mount tracker and persist final state
	if err := mountTracker.Close(); err != nil {
		log.Printf("[hostpath] Warning: failed to close mount tracker: %v", err)
	}
}

// parseEndpoint splits an endpoint string into (address, network).
//
//   - "unix:///path/to.sock"  \u2192 ("/path/to.sock", "unix")
//   - "/path/to.sock"         \u2192 ("/path/to.sock", "unix")
//   - "host:port"             \u2192 ("host:port",     "tcp")
func parseEndpoint(ep string) (addr, network string) {
	const unixPrefix = "unix://"
	if len(ep) > len(unixPrefix) && ep[:len(unixPrefix)] == unixPrefix {
		return ep[len(unixPrefix):], "unix"
	}
	if len(ep) > 0 && ep[0] == '/' {
		return ep, "unix"
	}
	return ep, "tcp"
}
