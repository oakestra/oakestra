package p2p

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"go_node_engine/util/iotools"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// CustodyRecord is the SLA copy held by a job's custodians so the job survives its host
// dying (plan §7.1). Pending=true parks an unschedulable job for retry (plan §7.6).
type CustodyRecord struct {
	Job        DeployableService `json:"job"`
	HostUUID   string            `json:"host_uuid"`  // "" while pending (no host yet)
	Generation int               `json:"generation"` // bumped on every (re)schedule (plan §7.4)
	Pending    bool              `json:"pending"`
}

var custodyMu sync.Mutex

func custodyDir() (string, error) {
	dir, err := iotools.CreateOakestraStateDir()
	if err != nil {
		return "", err
	}
	sub := filepath.Join(dir, "custodian")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		return "", err
	}
	return sub, nil
}

func custodyPath(jobName string) (string, error) {
	dir, err := custodyDir()
	if err != nil {
		return "", err
	}
	// job names are dot-separated alnum segments; safe as a filename
	return filepath.Join(dir, jobName+".json"), nil
}

// SaveCustody persists (or updates) a custody record for a job.
func SaveCustody(rec CustodyRecord) error {
	custodyMu.Lock()
	defer custodyMu.Unlock()
	path, err := custodyPath(rec.Job.JobName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// DeleteCustody removes the custody copy (on undeploy).
func DeleteCustody(jobName string) {
	custodyMu.Lock()
	defer custodyMu.Unlock()
	if path, err := custodyPath(jobName); err == nil {
		_ = os.Remove(path)
	}
}

// LoadCustody returns all custody records held by this node.
func LoadCustody() []CustodyRecord {
	custodyMu.Lock()
	defer custodyMu.Unlock()
	dir, err := custodyDir()
	if err != nil {
		return nil
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]CustodyRecord, 0)
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var rec CustodyRecord
		if json.Unmarshal(data, &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

// hrwScore is the Highest-Random-Weight hash of (job, node) — deterministic on every
// node, so all members agree on a job's custodian order with zero coordination (plan §7.1).
func hrwScore(jobName, nodeUUID string) uint64 {
	sum := sha256.Sum256([]byte(jobName + "|" + nodeUUID))
	return binary.BigEndian.Uint64(sum[:8])
}

// HRWOrder ranks node UUIDs for a job, highest score first.
func HRWOrder(jobName string, nodeUUIDs []string) []string {
	ranked := append([]string(nil), nodeUUIDs...)
	sort.Slice(ranked, func(a, b int) bool {
		sa, sb := hrwScore(jobName, ranked[a]), hrwScore(jobName, ranked[b])
		if sa == sb {
			return ranked[a] < ranked[b]
		}
		return sa > sb
	})
	return ranked
}

// Custodians returns the job's custodian set: the host itself plus the top k−1 live
// nodes by HRW (host included by decision — it already stores what it runs, plan §7.1).
func Custodians(jobName string, memberUUIDs []string, k int, hostUUID string) []string {
	if k < 1 {
		k = 1
	}
	out := []string{}
	if hostUUID != "" {
		out = append(out, hostUUID)
	}
	for _, uuid := range HRWOrder(jobName, memberUUIDs) {
		if len(out) >= k {
			break
		}
		if uuid == hostUUID {
			continue
		}
		out = append(out, uuid)
	}
	return out
}

// PrimaryLiveCustodian returns the node that owns rescheduling a job after its host died:
// the first node in HRW order that is alive and not the dead host (plan §7.2). Because
// HRW is deterministic, every node computes the same answer — exactly one rescheduler.
func PrimaryLiveCustodian(jobName string, liveUUIDs []string, deadUUID string) string {
	for _, uuid := range HRWOrder(jobName, liveUUIDs) {
		if uuid != deadUUID {
			return uuid
		}
	}
	return ""
}

func (r CustodyRecord) String() string {
	state := "hosted@" + r.HostUUID
	if r.Pending {
		state = "PENDING"
	}
	return fmt.Sprintf("%s gen=%d %s", r.Job.JobName, r.Generation, state)
}
