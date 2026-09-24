package p2p

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/internal/store"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util/taskid"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// DefaultControlPort is the node control API port (plan §8): deploy/bid/assign between
// members, plus status/logs observability. Identical on every node.
const DefaultControlPort = 50105

// Controller is the p2p brain of the NodeEngine: coordinator (bid rounds), host
// (assign handling), custodian (failover + pending retry), and observability endpoint.
type Controller struct {
	NodeUUID    string
	Conf        config.ConfFile
	ControlPort int
	// DeployFn/UndeployFn wrap the runtime manager (wired by the daemon).
	DeployFn   func(model.Service) error
	UndeployFn func(sname string, instance int) error

	nm     *NMClient
	ipam   *IPAM
	peers  *http.Client
	scheme string // "https" (roster-pinned mTLS, plan §9.1) or "http" (pre-genesis fallback)
	roster *rosterVerifier

	// maintenance suppression (plan §7.4): nodeUUID -> unix deadline. While a node is in
	// its maintenance window, its death does NOT trigger failover — it reclaims on return.
	maintMu     sync.Mutex
	maintenance map[string]int64
}

// NewController wires the p2p controller. deploy/undeploy wrap the runtime manager.
func NewController(conf config.ConfFile, deploy func(model.Service) error, undeploy func(string, int) error) (*Controller, error) {
	ipam, err := NewIPAM(conf.NodeUUID, conf.P2P.ServiceIPBlock)
	if err != nil {
		return nil, err
	}
	nm := NewNMClient()
	peers, scheme := NewMemberHTTPClient(conf.NodeUUID, nm, 15*time.Second)
	return &Controller{
		NodeUUID:    conf.NodeUUID,
		Conf:        conf,
		ControlPort: DefaultControlPort,
		DeployFn:    deploy,
		UndeployFn:  undeploy,
		nm:          nm,
		ipam:        ipam,
		peers:       peers,
		scheme:      scheme,
		roster:      newRosterVerifier(nm),
		maintenance: map[string]int64{},
	}, nil
}

// AnnounceRoster gossips this node's cert fingerprint as a __roster__ registry entry so
// every member can verify our mTLS identity (plan §9.4). Retries until the NetManager's
// p2p layer is up.
func (c *Controller) AnnounceRoster() {
	_, fp, err := EnsureIdentity(c.NodeUUID)
	if err != nil {
		logger.ErrorLogger().Printf("p2p: cannot announce roster identity: %v", err)
		return
	}
	entry := RegistryEntry{JobName: RosterJobName, NodeUUID: c.NodeUUID, Meta: fp}
	for attempt := 0; attempt < 30; attempt++ {
		if err := c.nm.Advertise(entry); err == nil {
			logger.InfoLogger().Printf("p2p: roster identity announced (%s)", fp[:23])
			return
		}
		time.Sleep(2 * time.Second)
	}
	logger.ErrorLogger().Printf("p2p: roster announcement failed — peers may reject our mTLS connections")
}

// Serve starts the control API (blocking). Run in a goroutine from the daemon.
func (c *Controller) Serve() error {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/ctrl/deploy", c.deployHandler).Methods("POST")
	router.HandleFunc("/ctrl/bid", c.bidHandler).Methods("POST")
	router.HandleFunc("/ctrl/assign", c.assignHandler).Methods("POST")
	router.HandleFunc("/ctrl/custody", c.custodyHandler).Methods("POST")
	router.HandleFunc("/ctrl/custody/{job}", c.custodyDeleteHandler).Methods("DELETE")
	router.HandleFunc("/ctrl/nodeleave", c.nodeLeaveHandler).Methods("POST")
	router.HandleFunc("/ctrl/nodejoin", c.nodeJoinHandler).Methods("POST")
	router.HandleFunc("/ctrl/maintenance", c.maintenanceHandler).Methods("POST")
	router.HandleFunc("/ctrl/maintenance-mark", c.maintenanceMarkHandler).Methods("POST")
	router.HandleFunc("/ctrl/revoke", c.revokeHandler).Methods("POST")
	router.HandleFunc("/ctrl/rotate", c.rotateHandler).Methods("POST")
	router.HandleFunc("/ctrl/services", c.servicesHandler).Methods("GET")
	router.HandleFunc("/ctrl/services/{job}/{instance}", c.undeployHandler).Methods("DELETE")
	router.HandleFunc("/ctrl/logs", c.logsHandler).Methods("GET")
	// Root-API-compatible subset so `oak app`/`oak service` work against a p2p worker
	// (plan §1.7). Shapes match the root system_manager; see the compat section below.
	router.HandleFunc("/api/application", c.apiDeployHandler).Methods("POST", "PUT")
	router.HandleFunc("/api/applications", c.apiListApplicationsHandler).Methods("GET")
	router.HandleFunc("/api/services", c.apiListServicesHandler).Methods("GET")
	router.HandleFunc("/api/services/", c.apiListServicesHandler).Methods("GET")
	router.HandleFunc("/api/services/{appID}", c.apiListServicesHandler).Methods("GET")
	router.HandleFunc("/api/service/{id}", c.apiGetServiceHandler).Methods("GET")
	// In p2p, a service is scheduled at create time, so an explicit instance deploy is a
	// no-op success; undeploy maps onto the p2p undeploy path.
	router.HandleFunc("/api/service/{id}/instance", c.apiInstanceDeployHandler).Methods("POST")
	router.HandleFunc("/api/service/{id}/instance/{n}", c.apiInstanceUndeployHandler).Methods("DELETE")

	go c.pendingRetryLoop()

	// Roster-pinned mutual TLS (plan §9.1 connections plane): serve with our self-signed
	// cert and require every client to present a rostered cert. Plain HTTP only as a
	// pre-genesis fallback (creds always exist after daemon startup in p2p mode).
	if _, ok := LoadCreds(); ok {
		tlsCfg, err := c.roster.serverTLSConfig(c.NodeUUID)
		if err != nil {
			return err
		}
		server := &http.Server{
			Addr:      fmt.Sprintf(":%d", c.ControlPort),
			Handler:   router,
			TLSConfig: tlsCfg,
		}
		logger.InfoLogger().Printf("p2p: control API listening on :%d (mTLS, roster-pinned) 🔒", c.ControlPort)
		return server.ListenAndServeTLS("", "")
	}
	logger.InfoLogger().Printf("p2p: control API listening on :%d (PLAIN — no p2p creds found)", c.ControlPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", c.ControlPort), router)
}

// ---------- deploy (coordinator role, plan §1.4/§1.5) ----------

type DeployRequest struct {
	SLA   json.RawMessage `json:"sla"`
	Local bool            `json:"local"`
}

type DeployResult struct {
	JobName string `json:"job_name"`
	Host    string `json:"host,omitempty"`
	RRIP    string `json:"rr_ip,omitempty"`
	Status  string `json:"status"` // SCHEDULED | PENDING | ERROR
	Detail  string `json:"detail,omitempty"`
}

func (c *Controller) deployHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req DeployRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	results := c.DeploySLA(req.SLA, req.Local)
	writeJSON(w, results)
}

// DeploySLA expands the SLA and schedules every service (plan §1.4 coordinator role).
func (c *Controller) DeploySLA(sla []byte, localOnly bool) []DeployResult {
	services, err := ExpandSLA(sla)
	if err != nil {
		return []DeployResult{{Status: "ERROR", Detail: err.Error()}}
	}
	results := make([]DeployResult, 0, len(services))
	for _, job := range services {
		// RR ServiceIP: honour the SLA's rr_ip, else allocate from our block (plan §1.4).
		if job.RRIP == "" {
			if ip, err := c.ipam.Alloc(); err == nil {
				job.RRIP = ip
			}
		}
		host, err := c.ScheduleJob(job, localOnly, 0)
		switch {
		case err == nil:
			c.replicateCustody(CustodyRecord{Job: job, HostUUID: host, Generation: 0})
			results = append(results, DeployResult{JobName: job.JobName, Host: host, RRIP: job.RRIP, Status: "SCHEDULED"})
		case err == ErrUnschedulable:
			// Park as PENDING on the HRW custodians instead of dropping (plan §7.6).
			c.parkPending(job, 0)
			results = append(results, DeployResult{JobName: job.JobName, RRIP: job.RRIP, Status: "PENDING",
				Detail: "unschedulable: parked, retried when capacity appears"})
		default:
			results = append(results, DeployResult{JobName: job.JobName, Status: "ERROR", Detail: err.Error()})
		}
	}
	return results
}

// ---------- bid + assign (host role, plan §1.5) ----------

func (c *Controller) bidHandler(w http.ResponseWriter, r *http.Request) {
	var req BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, c.evaluateBid(req))
}

func (c *Controller) assignHandler(w http.ResponseWriter, r *http.Request) {
	var req AssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, c.handleAssign(req))
}

// handleAssign runs on the winning host: TOCTOU re-check, instance-IP allocation from
// OUR block (plan §1.4 host role), deploy, persist, advertise, self-custody.
func (c *Controller) handleAssign(req AssignRequest) AssignResponse {
	job := req.Job

	// TOCTOU re-check: capacity may be gone since we bid (plan §1.5).
	check := c.evaluateBid(BidRequest{JobName: job.JobName, Runtime: job.Service.Runtime,
		Memory: job.Service.Memory, Vcpus: job.Service.Vcpus})
	if !check.Ok {
		return AssignResponse{Ok: false, Reason: "resources no longer available"}
	}

	instanceIP, err := c.ipam.Alloc()
	if err != nil {
		return AssignResponse{Ok: false, Reason: err.Error()}
	}

	if err := c.DeployFn(job.Service); err != nil {
		c.ipam.Free(instanceIP)
		return AssignResponse{Ok: false, Reason: err.Error()}
	}

	// Persist for reboot recovery (plan §6) with the scheduling generation (plan §7.4).
	_ = store.Upsert(store.Record{
		JobName:      job.JobName,
		Service:      job.Service,
		RRip:         job.RRIP,
		InstanceIP:   instanceIP,
		Generation:   req.Generation,
		DesiredState: store.DesiredRunning,
	})

	// Advertise into the gossip registry; the NetManager fills the namespace IP (plan §2.4).
	c.advertise(job, instanceIP, req.Generation, "RUNNING")

	// The host is always one of its own custodians (plan §7.1).
	_ = SaveCustody(CustodyRecord{Job: job, HostUUID: c.NodeUUID, Generation: req.Generation})

	logger.InfoLogger().Printf("p2p: deployed %s (gen %d, instance IP %s)", job.JobName, req.Generation, instanceIP)
	return AssignResponse{Ok: true}
}

func (c *Controller) advertise(job DeployableService, instanceIP string, generation int, status string) {
	entry := RegistryEntry{
		JobName:    job.JobName,
		NodeUUID:   c.NodeUUID,
		Generation: generation,
		Instances: []InstanceEntry{{
			InstanceNumber: job.Service.Instance,
			Status:         status,
			HostIP:         model.GetNodeInfo().Ip,
			HostPort:       50103,
			ServiceIP: []ServiceIPEntry{
				{Type: "RR", Address: job.RRIP},
				{Type: "InstanceNumber", Address: instanceIP},
			},
		}},
	}
	if err := c.nm.Advertise(entry); err != nil {
		logger.ErrorLogger().Printf("p2p: advertise %s failed: %v", job.JobName, err)
	}
}

// ReportStatus propagates coarse app-status transitions into the registry (plan §7.5).
// Wired as the runtime status callback in p2p mode.
func (c *Controller) ReportStatus(svc model.Service) {
	status := map[string]string{
		model.SERVICE_CREATED:   "RUNNING",
		model.SERVICE_DEAD:      "DEAD",
		model.SERVICE_FAILED:    "FAILED",
		model.SERVICE_COMPLETED: "COMPLETED",
	}[svc.Status]
	if status == "" {
		return
	}
	records, _ := store.Load()
	for _, rec := range records {
		if rec.Service.Sname == svc.Sname && rec.Service.Instance == svc.Instance {
			c.advertise(DeployableService{JobName: rec.JobName, RRIP: rec.RRip, Service: rec.Service},
				rec.InstanceIP, rec.Generation, status)
			return
		}
	}
}

// ---------- custody + failover + pending (plan §7) ----------

func (c *Controller) custodyHandler(w http.ResponseWriter, r *http.Request) {
	var rec CustodyRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = SaveCustody(rec)
	w.WriteHeader(http.StatusOK)
}

func (c *Controller) custodyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	DeleteCustody(mux.Vars(r)["job"])
	w.WriteHeader(http.StatusOK)
}

// replicateCustody copies the SLA to the job's custodian set: host + k−1 HRW backups
// (plan §7.1). Directed unicast — never a broadcast.
func (c *Controller) replicateCustody(rec CustodyRecord) {
	members, err := c.nm.Members()
	if err != nil {
		return
	}
	uuids := make([]string, 0, len(members))
	byUUID := map[string]MemberInfo{}
	for _, m := range members {
		uuids = append(uuids, m.UUID)
		byUUID[m.UUID] = m
	}
	k := c.Conf.P2P.ReplicaFactor
	if k <= 0 {
		k = 3
	}
	for _, uuid := range Custodians(rec.Job.JobName, uuids, k, rec.HostUUID) {
		if uuid == c.NodeUUID {
			_ = SaveCustody(rec)
			continue
		}
		if m, ok := byUUID[uuid]; ok {
			if err := c.postPeer(m, "/ctrl/custody", rec, nil); err != nil {
				logger.ErrorLogger().Printf("p2p: custody replication of %s to %s failed: %v", rec.Job.JobName, uuid, err)
			}
		}
	}
}

// parkPending stores an unschedulable job on its custodians for automatic retry (plan §7.6).
func (c *Controller) parkPending(job DeployableService, generation int) {
	rec := CustodyRecord{Job: job, Generation: generation, Pending: true}
	_ = SaveCustody(rec)
	c.replicateCustody(rec)
	logger.InfoLogger().Printf("p2p: %s parked as PENDING (no capable node)", job.JobName)
}

type nodeEventPayload struct {
	NodeUUID string          `json:"node_uuid"`
	Lost     []RegistryEntry `json:"lost"`
}

// nodeLeaveHandler is the failover trigger (plan §7.2): the NetManager forwards the SWIM
// NodeLeave event; if this node is the primary live custodian for an affected job, it
// reschedules it with a bumped generation.
func (c *Controller) nodeLeaveHandler(w http.ResponseWriter, r *http.Request) {
	var evt nodeEventPayload
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	go c.handleNodeLeave(evt.NodeUUID)
}

func (c *Controller) handleNodeLeave(deadUUID string) {
	// A node in its maintenance window is rebooting on purpose — do not fail over its
	// jobs; it reclaims them from its own store on return (plan §7.4).
	if c.inMaintenance(deadUUID) {
		logger.InfoLogger().Printf("p2p: %s left during its maintenance window — reschedule suppressed", deadUUID)
		return
	}
	members, err := c.nm.Members()
	if err != nil {
		return
	}
	live := make([]string, 0, len(members))
	for _, m := range members {
		live = append(live, m.UUID)
	}
	for _, rec := range LoadCustody() {
		if rec.HostUUID != deadUUID {
			continue
		}
		// Per-SLA opt-out (plan §7.3): one-shot / pinned jobs are reported, not moved.
		if !rec.Job.ShouldReschedule() {
			logger.InfoLogger().Printf("p2p failover: host %s died but %s has restart policy 'none' — not rescheduling", deadUUID, rec.Job.JobName)
			continue
		}
		// Exactly one node reschedules: the primary live custodian by HRW (plan §7.2).
		if PrimaryLiveCustodian(rec.Job.JobName, live, deadUUID) != c.NodeUUID {
			continue
		}
		gen := rec.Generation + 1
		logger.InfoLogger().Printf("p2p failover: host %s died, rescheduling %s (gen %d)", deadUUID, rec.Job.JobName, gen)
		host, err := c.ScheduleJob(rec.Job, false, gen) // RR ServiceIP preserved in rec.Job
		if err == nil {
			c.replicateCustody(CustodyRecord{Job: rec.Job, HostUUID: host, Generation: gen})
		} else {
			c.parkPending(rec.Job, gen)
		}
	}
	// Membership changed: custodian sets shift — restore k live copies (plan §7.2 step 5).
	c.reReplicateCustody()
}

// reReplicateCustody re-copies each custody record this node holds for jobs it HOSTS to
// the current custodian set, restoring k live copies after membership churn (plan §7.2
// step 5). Only the host re-replicates, so exactly one node does it per job.
func (c *Controller) reReplicateCustody() {
	for _, rec := range LoadCustody() {
		if rec.HostUUID == c.NodeUUID && !rec.Pending {
			c.replicateCustody(rec)
		}
	}
}

// ---------- maintenance suppression (plan §7.4, `drain --maintenance`) ----------

type maintenanceRequest struct {
	NodeUUID string `json:"node_uuid"`
	Until    int64  `json:"until"` // unix seconds
	Duration string `json:"duration,omitempty"`
}

func (c *Controller) inMaintenance(nodeUUID string) bool {
	c.maintMu.Lock()
	defer c.maintMu.Unlock()
	return time.Now().Unix() < c.maintenance[nodeUUID]
}

// maintenanceHandler (local CLI entry): broadcast "I am going down on purpose for <dur>,
// do not reschedule my jobs" to every member (plan §7.4).
func (c *Controller) maintenanceHandler(w http.ResponseWriter, r *http.Request) {
	var req maintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dur, err := time.ParseDuration(req.Duration)
	if err != nil || dur <= 0 {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}
	mark := maintenanceRequest{NodeUUID: c.NodeUUID, Until: time.Now().Add(dur).Unix()}
	members, err := c.nm.Members()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	for _, m := range members {
		if m.UUID == c.NodeUUID {
			continue
		}
		if err := c.postPeer(m, "/ctrl/maintenance-mark", mark, nil); err != nil {
			logger.ErrorLogger().Printf("p2p: maintenance mark to %s failed: %v", m.UUID, err)
		}
	}
	logger.InfoLogger().Printf("p2p: maintenance window announced until %s — failover suppressed", time.Unix(mark.Until, 0))
	w.WriteHeader(http.StatusOK)
}

// ---------- revocation + key rotation (plan §9.6) ----------

type rotateRequest struct {
	GossipKey string            `json:"gossip_key"`
	Roster    map[string]string `json:"roster,omitempty"`
}

// revokeHandler (local CLI entry): evict a member and rotate the network keys so it can
// no longer decrypt gossip or the data plane (plan §9.6). Fan-out rides the roster-pinned
// mTLS control plane, which the PSK rotation does not disturb.
func (c *Controller) revokeHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeUUID string `json:"node_uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeUUID == "" {
		http.Error(w, "node_uuid required", http.StatusBadRequest)
		return
	}
	if req.NodeUUID == c.NodeUUID {
		http.Error(w, "refusing to revoke self", http.StatusBadRequest)
		return
	}
	if err := c.RevokeAndRotate(req.NodeUUID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RevokeAndRotate drops a node from the roster and rotates the gossip PSK + data key
// across all remaining members (plan §9.6): a compromised node knew the shared symmetric
// keys — rotating them is what actually locks it out.
func (c *Controller) RevokeAndRotate(revokedUUID string) error {
	if err := Revoke(revokedUUID); err != nil {
		return err
	}
	creds, ok := LoadCreds()
	if !ok {
		return fmt.Errorf("no p2p credentials")
	}

	// Mint the replacement PSK.
	newKeyBytes := make([]byte, 32)
	if _, err := rand.Read(newKeyBytes); err != nil {
		return err
	}
	newKey := fmt.Sprintf("%x", newKeyBytes)

	// Fan out to every remaining member over mTLS (unaffected by the PSK change).
	members, err := c.nm.Members()
	if err != nil {
		return err
	}
	rotate := rotateRequest{GossipKey: newKey, Roster: creds.Roster}
	for _, m := range members {
		if m.UUID == c.NodeUUID || m.UUID == revokedUUID {
			continue
		}
		if err := c.postPeer(m, "/ctrl/rotate", rotate, nil); err != nil {
			logger.ErrorLogger().Printf("p2p: key rotation to %s failed: %v — it can re-sync via re-enrollment", m.UUID, err)
		}
	}

	// Apply locally last.
	if err := c.applyRotation(rotate); err != nil {
		return err
	}
	logger.InfoLogger().Printf("p2p: %s revoked; network keys rotated 🔒", revokedUUID)
	return nil
}

// rotateHandler applies a key rotation received from the revoking member (mTLS-verified).
func (c *Controller) rotateHandler(w http.ResponseWriter, r *http.Request) {
	var req rotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GossipKey == "" {
		http.Error(w, "gossip_key required", http.StatusBadRequest)
		return
	}
	if err := c.applyRotation(req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (c *Controller) applyRotation(req rotateRequest) error {
	creds, ok := LoadCreds()
	if !ok {
		return fmt.Errorf("no p2p credentials")
	}
	creds.GossipKey = req.GossipKey
	if req.Roster != nil {
		creds.Roster = req.Roster
	}
	if err := SaveCreds(creds); err != nil {
		return err
	}
	// Rotate the live gossip keyring + data-plane key in the NetManager (no restart).
	if err := c.nm.Rotate(req.GossipKey); err != nil {
		logger.ErrorLogger().Printf("p2p: NetManager key rotation failed (will apply on next restart): %v", err)
	}
	return nil
}

// maintenanceMarkHandler records a peer's announced maintenance window.
func (c *Controller) maintenanceMarkHandler(w http.ResponseWriter, r *http.Request) {
	var req maintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.maintMu.Lock()
	c.maintenance[req.NodeUUID] = req.Until
	c.maintMu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// nodeJoinHandler retries PENDING jobs when capacity appears (plan §7.6 retry trigger)
// and re-replicates custody — the joiner may now be in some jobs' HRW custodian sets.
func (c *Controller) nodeJoinHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	go func() {
		c.retryPending()
		c.reReplicateCustody()
	}()
}

// pendingRetryLoop is the backstop timer for PENDING retries (plan §7.6).
func (c *Controller) pendingRetryLoop() {
	interval := time.Duration(c.Conf.P2P.SchedRetryMaxSec) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		c.retryPending()
	}
}

func (c *Controller) retryPending() {
	members, err := c.nm.Members()
	if err != nil {
		return
	}
	live := make([]string, 0, len(members))
	for _, m := range members {
		live = append(live, m.UUID)
	}
	for _, rec := range LoadCustody() {
		if !rec.Pending {
			continue
		}
		// One deterministic owner per pending job (plan §7.6).
		if PrimaryLiveCustodian(rec.Job.JobName, live, "") != c.NodeUUID {
			continue
		}
		gen := rec.Generation + 1
		if host, err := c.ScheduleJob(rec.Job, false, gen); err == nil {
			logger.InfoLogger().Printf("p2p: PENDING %s placed on %s (gen %d)", rec.Job.JobName, host, gen)
			c.replicateCustody(CustodyRecord{Job: rec.Job, HostUUID: host, Generation: gen})
		}
	}
}

// ReconcileP2P restores persisted deployments after a reboot, but yields to any newer
// placement made while this node was down (generation rule, plan §7.4): reconcile
// BEFORE reclaiming.
func (c *Controller) ReconcileP2P() {
	// Let gossip anti-entropy converge after the re-join before consulting the registry.
	time.Sleep(3 * time.Second)
	entries, _ := c.nm.Services()
	records, err := store.Load()
	if err != nil || len(records) == 0 {
		return
	}
	for _, rec := range records {
		if rec.DesiredState != store.DesiredRunning {
			continue
		}
		yield := false
		for _, e := range entries {
			// A live instance on another host with >= generation means the job was failed
			// over (or re-placed) while we were down — the newer decision wins and the
			// returning host yields (plan §7.4).
			if e.JobName == rec.JobName && e.NodeUUID != c.NodeUUID && e.Generation >= rec.Generation {
				yield = true
				break
			}
		}
		if yield {
			logger.InfoLogger().Printf("p2p reconcile: %s was re-placed while we were down — yielding (plan §7.4)", rec.JobName)
			_ = store.Remove(rec.Service.Sname, rec.Service.Instance)
			c.ipam.Free(rec.InstanceIP)
			continue
		}
		// Reclaim: restart from the store with the SAME IPs and generation, re-advertise.
		if err := c.DeployFn(rec.Service); err != nil {
			logger.ErrorLogger().Printf("p2p reconcile: restore %s failed: %v", rec.JobName, err)
			continue
		}
		c.advertise(DeployableService{JobName: rec.JobName, RRIP: rec.RRip, Service: rec.Service},
			rec.InstanceIP, rec.Generation, "RUNNING")
		logger.InfoLogger().Printf("p2p reconcile: restored %s (gen %d)", rec.JobName, rec.Generation)
	}
}

// ---------- observability + undeploy (plan §8) ----------

type ServiceView struct {
	JobName    string `json:"job_name"`
	Host       string `json:"host"`
	Local      bool   `json:"local"`
	Generation int    `json:"generation"`
	Instance   int    `json:"instance"`
	Status     string `json:"status"`
	RRIP       string `json:"rr_ip,omitempty"`
	InstanceIP string `json:"instance_ip,omitempty"`
	Pending    bool   `json:"pending,omitempty"`
}

// servicesHandler renders the network-wide view from the gossiped registry (plan §8) —
// no per-node query needed — plus any PENDING jobs this node holds custody for.
func (c *Controller) servicesHandler(w http.ResponseWriter, r *http.Request) {
	views := make([]ServiceView, 0)
	entries, err := c.nm.Services()
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.JobName, "__") { // internal entries (e.g. roster)
				continue
			}
			for _, inst := range e.Instances {
				v := ServiceView{
					JobName: e.JobName, Host: e.NodeUUID, Local: e.NodeUUID == c.NodeUUID,
					Generation: e.Generation, Instance: inst.InstanceNumber, Status: inst.Status,
				}
				for _, sip := range inst.ServiceIP {
					if sip.Type == "RR" {
						v.RRIP = sip.Address
					} else if sip.Type == "InstanceNumber" {
						v.InstanceIP = sip.Address
					}
				}
				views = append(views, v)
			}
		}
	}
	for _, rec := range LoadCustody() {
		if rec.Pending {
			views = append(views, ServiceView{JobName: rec.Job.JobName, Status: "PENDING (unschedulable, retrying)",
				Generation: rec.Generation, RRIP: rec.Job.RRIP, Pending: true})
		}
	}
	writeJSON(w, views)
}

func (c *Controller) undeployHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	job := vars["job"]
	instance, _ := strconv.Atoi(vars["instance"])

	records, _ := store.Load()
	for _, rec := range records {
		if rec.JobName == job && rec.Service.Instance == instance {
			// hosted here: stop, deregister everywhere
			if err := c.UndeployFn(rec.Service.Sname, instance); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = store.Remove(rec.Service.Sname, instance)
			_ = c.nm.Withdraw(job, instance)
			c.ipam.Free(rec.InstanceIP)
			DeleteCustody(job)
			c.dropCustodyEverywhere(job)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Not hosted here: forward to the host from the registry.
	entries, err := c.nm.Services()
	if err == nil {
		members, _ := c.nm.Members()
		for _, e := range entries {
			if e.JobName != job {
				continue
			}
			for _, m := range members {
				if m.UUID == e.NodeUUID {
					url := fmt.Sprintf("%s://%s:%d/ctrl/services/%s/%d", c.scheme, m.IP(), c.ControlPort, job, instance)
					req, _ := http.NewRequest(http.MethodDelete, url, nil)
					if resp, err := c.peers.Do(req); err == nil && resp.StatusCode == http.StatusOK {
						_ = resp.Body.Close()
						w.WriteHeader(http.StatusOK)
						return
					}
				}
			}
		}
	}
	// Maybe it is only PENDING custody: drop it.
	DeleteCustody(job)
	c.dropCustodyEverywhere(job)
	w.WriteHeader(http.StatusOK)
}

func (c *Controller) dropCustodyEverywhere(job string) {
	members, err := c.nm.Members()
	if err != nil {
		return
	}
	for _, m := range members {
		if m.UUID == c.NodeUUID {
			continue
		}
		url := fmt.Sprintf("%s://%s:%d/ctrl/custody/%s", c.scheme, m.IP(), c.ControlPort, job)
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		if resp, err := c.peers.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}
}

// logsHandler streams the tail of an app's log. If the service is hosted elsewhere, the
// request is proxied to the hosting node — the kubectl-logs→kubelet pattern (plan §8:
// logs never gossip; they are fetched from the host on demand).
func (c *Controller) logsHandler(w http.ResponseWriter, r *http.Request) {
	job := r.URL.Query().Get("job")
	instance, _ := strconv.Atoi(r.URL.Query().Get("instance"))
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 100
	}
	path := filepath.Join(model.GetNodeInfo().LogDirectory, taskid.Generate(job, instance))
	data, err := os.ReadFile(path)
	if err == nil {
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
		return
	}

	// Not hosted here: resolve the host from the registry and proxy once (no loops:
	// the proxied request only ever lands on the host, which serves the file directly).
	if r.Header.Get("X-Oakestra-Proxied") == "" {
		if entries, err := c.nm.Services(); err == nil {
			members, _ := c.nm.Members()
			for _, e := range entries {
				if e.JobName != job || e.NodeUUID == c.NodeUUID {
					continue
				}
				for _, m := range members {
					if m.UUID != e.NodeUUID {
						continue
					}
					url := fmt.Sprintf("%s://%s:%d/ctrl/logs?job=%s&instance=%d&tail=%d",
						c.scheme, m.IP(), c.ControlPort, job, instance, tail)
					req, _ := http.NewRequest(http.MethodGet, url, nil)
					req.Header.Set("X-Oakestra-Proxied", "1")
					if resp, err := c.peers.Do(req); err == nil {
						defer func() { _ = resp.Body.Close() }()
						w.Header().Set("Content-Type", "text/plain")
						w.WriteHeader(resp.StatusCode)
						_, _ = io.Copy(w, resp.Body)
						return
					}
				}
			}
		}
	}
	http.Error(w, fmt.Sprintf("no logs found for %s.%d anywhere in the network", job, instance), http.StatusNotFound)
}

// ---------- root-API compatibility for the oak CLI (plan §1.7) ----------
//
// These endpoints return the SAME JSON shapes as the root system_manager so the oak CLI
// (Python and Go) works unmodified against a p2p worker. The p2p network is flat, so we
// synthesise the root's app/service/instance hierarchy from the gossiped registry:
//   - applicationID / applicationName ← "app.appns" of a job name
//   - microserviceID / name           ← the full 4-part job name
//   - instance_list                   ← the registry instances for that job

// apiApplication mirrors the root's Application JSON shape.
type apiApplication struct {
	ApplicationID        string   `json:"applicationID"`
	ApplicationName      string   `json:"application_name"`
	ApplicationNamespace string   `json:"application_namespace"`
	ApplicationDesc      string   `json:"application_desc"`
	Microservices        []string `json:"microservices"`
}

// apiService mirrors the root's Service JSON shape (subset consumed by the CLI).
type apiService struct {
	MicroserviceID        string           `json:"microserviceID"`
	MicroserviceName      string           `json:"microservice_name"`
	MicroserviceNamespace string           `json:"microservice_namespace"`
	ApplicationID         string           `json:"applicationID"`
	ApplicationName       string           `json:"application_name"`
	ApplicationNamespace  string           `json:"application_namespace"`
	Status                string           `json:"status"`
	RRip                  string           `json:"RR_ip"`
	InstanceList          []apiInstance    `json:"instance_list"`
}

type apiInstance struct {
	InstanceNumber int    `json:"instance_number"`
	Status         string `json:"status"`
	PublicIP       string `json:"publicip"`
	HostIP         string `json:"host_ip"`
	HostPort       int    `json:"host_port"`
	WorkerID       string `json:"worker_id"`
}

// registrySnapshot returns the network-wide registry entries, skipping internal ones.
func (c *Controller) registrySnapshot() []RegistryEntry {
	entries, err := c.nm.Services()
	if err != nil {
		return nil
	}
	out := make([]RegistryEntry, 0, len(entries))
	for _, e := range entries {
		if !strings.HasPrefix(e.JobName, "__") {
			out = append(out, e)
		}
	}
	return out
}

// appIDOf returns the "app.appns" identity for a 4-part job name.
func appIDOf(jobName string) (id, name, ns string) {
	parts := strings.Split(jobName, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1], parts[0], parts[1]
	}
	return jobName, jobName, ""
}

func (c *Controller) buildServices() []apiService {
	byJob := map[string]*apiService{}
	order := []string{}
	// custody (pending, not yet in the registry) surfaces as a CREATING service too
	for _, e := range c.registrySnapshot() {
		appID, appName, appNs := appIDOf(e.JobName)
		parts := strings.Split(e.JobName, ".")
		msName, msNs := e.JobName, ""
		if len(parts) == 4 {
			msName, msNs = parts[2], parts[3]
		}
		svc, ok := byJob[e.JobName]
		if !ok {
			svc = &apiService{
				MicroserviceID: e.JobName, MicroserviceName: msName, MicroserviceNamespace: msNs,
				ApplicationID: appID, ApplicationName: appName, ApplicationNamespace: appNs,
				Status: "", InstanceList: []apiInstance{},
			}
			byJob[e.JobName] = svc
			order = append(order, e.JobName)
		}
		for _, inst := range e.Instances {
			for _, sip := range inst.ServiceIP {
				if sip.Type == "RR" && svc.RRip == "" {
					svc.RRip = sip.Address
				}
			}
			svc.Status = inst.Status
			svc.InstanceList = append(svc.InstanceList, apiInstance{
				InstanceNumber: inst.InstanceNumber, Status: inst.Status,
				PublicIP: inst.HostIP, HostIP: inst.HostIP, HostPort: inst.HostPort,
				WorkerID: e.NodeUUID,
			})
		}
	}
	out := make([]apiService, 0, len(order))
	for _, j := range order {
		out = append(out, *byJob[j])
	}
	return out
}

func (c *Controller) buildApplications() []apiApplication {
	byApp := map[string]*apiApplication{}
	order := []string{}
	for _, svc := range c.buildServices() {
		app, ok := byApp[svc.ApplicationID]
		if !ok {
			app = &apiApplication{
				ApplicationID: svc.ApplicationID, ApplicationName: svc.ApplicationName,
				ApplicationNamespace: svc.ApplicationNamespace, Microservices: []string{},
			}
			byApp[svc.ApplicationID] = app
			order = append(order, svc.ApplicationID)
		}
		app.Microservices = append(app.Microservices, svc.MicroserviceID)
	}
	out := make([]apiApplication, 0, len(order))
	for _, a := range order {
		out = append(out, *byApp[a])
	}
	return out
}

// apiDeployHandler (POST /api/application): deploy an SLA and return the created
// applications in the root shape (the CLI matches by application_name).
func (c *Controller) apiDeployHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	results := c.DeploySLA(body, false)

	// Group the scheduling results into root-shaped applications.
	byApp := map[string]*apiApplication{}
	order := []string{}
	anyError := ""
	for _, res := range results {
		if res.Status == "ERROR" {
			anyError = res.Detail
			continue
		}
		appID, appName, appNs := appIDOf(res.JobName)
		app, ok := byApp[appID]
		if !ok {
			app = &apiApplication{ApplicationID: appID, ApplicationName: appName,
				ApplicationNamespace: appNs, Microservices: []string{}}
			byApp[appID] = app
			order = append(order, appID)
		}
		app.Microservices = append(app.Microservices, res.JobName)
	}
	if len(order) == 0 && anyError != "" {
		http.Error(w, anyError, http.StatusBadRequest)
		return
	}
	apps := make([]apiApplication, 0, len(order))
	for _, a := range order {
		apps = append(apps, *byApp[a])
	}
	writeJSON(w, apps)
}

// apiListApplicationsHandler (GET /api/applications).
func (c *Controller) apiListApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, c.buildApplications())
}

// apiGetServiceHandler (GET /api/service/{id}) returns a single service by job name.
func (c *Controller) apiGetServiceHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for _, s := range c.buildServices() {
		if s.MicroserviceID == id {
			writeJSON(w, s)
			return
		}
	}
	http.Error(w, fmt.Sprintf("service %s not found", id), http.StatusNotFound)
}

// apiInstanceDeployHandler (POST /api/service/{id}/instance): no-op in p2p — the service
// was already scheduled onto a host at create time. Returns 200 so `oak app create -d`
// and `oak service deploy` succeed without a redundant placement.
func (c *Controller) apiInstanceDeployHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "already scheduled (p2p)"})
}

// apiInstanceUndeployHandler (DELETE /api/service/{id}/instance/{n}) maps onto the p2p
// undeploy path (stops + deregisters, forwarding to the host if remote).
func (c *Controller) apiInstanceUndeployHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	r2 := r.Clone(r.Context())
	r2 = mux.SetURLVars(r2, map[string]string{"job": vars["id"], "instance": vars["n"]})
	c.undeployHandler(w, r2)
}

// apiListServicesHandler (GET /api/services[/{appID}]).
func (c *Controller) apiListServicesHandler(w http.ResponseWriter, r *http.Request) {
	appID := strings.TrimPrefix(r.URL.Path, "/api/services")
	appID = strings.Trim(appID, "/")
	all := c.buildServices()
	if appID == "" {
		writeJSON(w, all)
		return
	}
	filtered := make([]apiService, 0)
	for _, s := range all {
		if s.ApplicationID == appID {
			filtered = append(filtered, s)
		}
	}
	writeJSON(w, filtered)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
