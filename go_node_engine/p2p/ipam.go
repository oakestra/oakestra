package p2p

import (
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/util/iotools"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
)

// IPAM allocates instance and RR ServiceIPs from this node's exclusively-owned /24
// block inside 10.30.0.0/16 (plan §3.3). Because every node draws only from a block it
// owns, two nodes can never mint the same IP — uniqueness by construction.
type IPAM struct {
	mu        sync.Mutex
	block     byte // 10.30.<block>.x
	path      string
	allocated map[string]bool
}

// BlockFor derives the default /24 block octet from the node UUID (plan §3.3).
// Collisions between two nodes' derived blocks are resolved at claim time via gossip
// (the block is embedded in every advertised instance IP, so a claimer can detect and
// move to the next free block).
func BlockFor(nodeUUID string) byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(nodeUUID))
	return byte(h.Sum32() % 256)
}

// NewIPAM opens (or creates) the persisted allocation set for the given block.
func NewIPAM(nodeUUID string, configuredBlock string) (*IPAM, error) {
	dir, err := iotools.CreateOakestraStateDir()
	if err != nil {
		return nil, err
	}
	block := BlockFor(nodeUUID)
	if configuredBlock != "" {
		// configured as "10.30.<B>.0/24" or plain "<B>"
		var b int
		if _, err := fmt.Sscanf(configuredBlock, "10.30.%d.0/24", &b); err == nil {
			block = byte(b)
		} else if _, err := fmt.Sscanf(configuredBlock, "%d", &b); err == nil {
			block = byte(b)
		}
	}
	ip := &IPAM{
		block:     block,
		path:      filepath.Join(dir, "ipam.json"),
		allocated: map[string]bool{},
	}
	data, err := os.ReadFile(ip.path)
	if err == nil {
		var list []string
		if json.Unmarshal(data, &list) == nil {
			for _, a := range list {
				ip.allocated[a] = true
			}
		}
	}
	return ip, nil
}

// Block returns this node's claimed block in CIDR form, gossiped as the claim (plan §3.3).
func (i *IPAM) Block() string { return fmt.Sprintf("10.30.%d.0/24", i.block) }

// Alloc returns the next free IP in the node's block and persists the allocation.
func (i *IPAM) Alloc() (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for x := 1; x < 255; x++ {
		candidate := fmt.Sprintf("10.30.%d.%d", i.block, x)
		if !i.allocated[candidate] {
			i.allocated[candidate] = true
			return candidate, i.saveLocked()
		}
	}
	return "", errors.New("instance IP block exhausted (widen to /23, plan §3.3)")
}

// Free releases an IP back to the block.
func (i *IPAM) Free(ip string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.allocated, ip)
	_ = i.saveLocked()
}

func (i *IPAM) saveLocked() error {
	list := make([]string, 0, len(i.allocated))
	for a := range i.allocated {
		list = append(list, a)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return os.WriteFile(i.path, data, 0o600)
}
