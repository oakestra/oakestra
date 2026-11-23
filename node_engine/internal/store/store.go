// Package store persists the set of services this node is responsible for running,
// so a reboot can restore them without waiting for a p2p reschedule. It is
// used in p2p mode only: in cluster mode the cluster owns re-deployment after a reboot.
package store

import (
	"encoding/json"
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"os"
	"path/filepath"
	"sync"
)

const storeFileName = "deployments.json"

// DesiredRunning marks a record that should be (re)deployed on startup.
const DesiredRunning = "running"

// Record is one persisted deployment. Generation supports reboot-vs-failover
// reconciliation: higher generation wins.
type Record struct {
	JobName      string        `json:"job_name"`
	Service      model.Service `json:"service"`
	RRip         string        `json:"rr_ip,omitempty"`
	InstanceIP   string        `json:"instance_ip,omitempty"`
	Generation   int           `json:"generation"`
	DesiredState string        `json:"desired_state"`
}

var mu sync.Mutex

// storePath resolves the deployments file inside the oakestra state dir
// (/var/lib/oakestra/deployments.json, or a temp fallback).
func storePath() (string, error) {
	dir, err := iotools.CreateOakestraStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, storeFileName), nil
}

// Load returns all persisted records. A missing/empty file yields an empty slice.
func Load() ([]Record, error) {
	mu.Lock()
	defer mu.Unlock()
	return loadLocked()
}

func loadLocked() ([]Record, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || (err == nil && len(data) == 0) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		// A corrupt store must not wedge startup; treat as empty.
		return []Record{}, nil
	}
	return records, nil
}

func saveLocked(records []Record) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	// Atomic replace: write temp then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func key(jobName string, instance int) (string, int) { return jobName, instance }

// Upsert adds or replaces the record for (service name, instance number).
func Upsert(r Record) error {
	mu.Lock()
	defer mu.Unlock()
	records, err := loadLocked()
	if err != nil {
		return err
	}
	kName, kInst := key(r.Service.Sname, r.Service.Instance)
	replaced := false
	for i := range records {
		if records[i].Service.Sname == kName && records[i].Service.Instance == kInst {
			records[i] = r
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, r)
	}
	return saveLocked(records)
}

// Remove deletes the record for (service name, instance number). No-op if absent.
func Remove(serviceName string, instance int) error {
	mu.Lock()
	defer mu.Unlock()
	records, err := loadLocked()
	if err != nil {
		return err
	}
	filtered := records[:0]
	for _, rec := range records {
		if rec.Service.Sname == serviceName && rec.Service.Instance == instance {
			continue
		}
		filtered = append(filtered, rec)
	}
	return saveLocked(filtered)
}
