package p2p

import (
	"go_node_engine/internal/store"
	"go_node_engine/logger"
	"strings"
	"time"
)

// duplicateCheckInterval is how often a node checks the registry for another live
// placement of a job it hosts.
const duplicateCheckInterval = 15 * time.Second

// Duplicate placements arise when both sides of a network partition reschedule the same
// job, or when an assign succeeded on a host whose reply was lost and the coordinator
// placed the job again. Every node applies the same rule to the same registry, so all of
// them agree on the survivor without coordinating: the placement with the higher
// generation wins (it is the more recent scheduling decision), ties go to the lower
// node UUID, and every loser stops its own copy.

// placementWins reports whether placement a (generation, host) beats placement b.
func placementWins(genA int, hostA string, genB int, hostB string) bool {
	if genA != genB {
		return genA > genB
	}
	return hostA < hostB
}

// livePlacement returns the winning live placement of a job among registry entries,
// ignoring the given node. Entries without instances are withdrawals, not placements.
func livePlacement(entries []RegistryEntry, jobName, ignore string) (host string, generation int, ok bool) {
	for _, e := range entries {
		if e.JobName != jobName || e.NodeUUID == ignore || len(e.Instances) == 0 || strings.HasPrefix(e.JobName, "__") {
			continue
		}
		if !ok || placementWins(e.Generation, e.NodeUUID, generation, host) {
			host, generation, ok = e.NodeUUID, e.Generation, true
		}
	}
	return host, generation, ok
}

// duplicateLoop periodically resolves duplicate placements of the jobs this node hosts.
func (c *Controller) duplicateLoop() {
	t := time.NewTicker(duplicateCheckInterval)
	defer t.Stop()
	for range t.C {
		c.resolveDuplicates()
	}
}

// resolveDuplicates stops every local job that loses to a live placement elsewhere, and
// points this node's custody record at the survivor.
func (c *Controller) resolveDuplicates() {
	entries, err := c.nm.Services()
	if err != nil {
		return
	}
	records, err := store.Load()
	if err != nil {
		return
	}
	for _, rec := range records {
		if rec.DesiredState != store.DesiredRunning {
			continue
		}
		host, gen, ok := livePlacement(entries, rec.JobName, c.NodeUUID)
		if !ok || placementWins(rec.Generation, c.NodeUUID, gen, host) {
			continue
		}
		logger.InfoLogger().Printf("p2p: %s also runs on %s (gen %d vs our gen %d) — stopping our copy", rec.JobName, host, gen, rec.Generation)
		if err := c.stopLocal(rec); err != nil {
			logger.ErrorLogger().Printf("p2p: could not stop duplicate %s: %v", rec.JobName, err)
			continue
		}
		for _, cust := range LoadCustody() {
			if cust.Job.JobName == rec.JobName && cust.HostUUID == c.NodeUUID {
				cust.HostUUID, cust.Generation = host, gen
				_ = SaveCustody(cust)
			}
		}
	}
}
