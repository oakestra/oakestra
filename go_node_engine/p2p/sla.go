// Package p2p implements the NodeEngine side of peer-to-peer mode: SLA expansion,
// the distributed bid scheduler (plan §1.5), IP allocation from the node's claimed
// block (plan §3.3), SLA custodianship + failover (plan §7), and the node control
// API (plan §8).
package p2p

import (
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/model"
)

// SLA mirrors the deployment descriptor accepted by the root system manager, so the
// exact same file works with `oak app create` against a root orchestrator or a p2p
// worker (plan §1.2).
type SLA struct {
	SlaVersion   string           `json:"sla_version"`
	Applications []SLAApplication `json:"applications"`
}

type SLAApplication struct {
	Name          string            `json:"application_name"`
	Namespace     string            `json:"application_namespace"`
	Desc          string            `json:"application_desc"`
	Microservices []SLAMicroservice `json:"microservices"`
}

type SLAMicroservice struct {
	Name           string       `json:"microservice_name"`
	Namespace      string       `json:"microservice_namespace"`
	Virtualization string       `json:"virtualization"`
	Code           string       `json:"code"`
	Cmd            []string     `json:"cmd"`
	Environment    []string     `json:"environment"`
	Port           string       `json:"port"`
	Memory         int          `json:"memory"`
	Vcpus          int          `json:"vcpus"`
	Vgpus          int          `json:"vgpus"`
	Storage        int          `json:"storage"`
	OneShot        bool         `json:"one_shot"`
	Privileged     bool         `json:"privileged"`
	Addresses      SLAAddresses `json:"addresses"`
	Constraints    []any        `json:"constraints"`
	// RestartPolicy controls failover behaviour (plan §7.3): "reschedule" (default) moves
	// the service to another node when its host dies; "none" opts out (one-shot jobs
	// default to none — they should not re-run on failover).
	RestartPolicy string `json:"p2p_restart_policy"`
}

// Restart policies (plan §7.3).
const (
	RestartReschedule = "reschedule"
	RestartNone       = "none"
)

type SLAAddresses struct {
	RRIP string `json:"rr_ip"`
}

// DeployableService is one schedulable unit expanded from an SLA microservice.
type DeployableService struct {
	JobName       string        `json:"job_name"`       // app.appns.svc.svcns
	RRIP          string        `json:"rr_ip"`          // stable ServiceIP; from SLA or allocated
	RestartPolicy string        `json:"restart_policy"` // "reschedule" | "none" (plan §7.3)
	Service       model.Service `json:"service"`
}

// ShouldReschedule reports whether the job may be moved on host death (plan §7.3).
func (d DeployableService) ShouldReschedule() bool {
	if d.RestartPolicy == RestartNone {
		return false
	}
	// One-shot jobs default to none: they must not silently re-run elsewhere.
	if d.RestartPolicy == "" && d.Service.OneShot {
		return false
	}
	return true
}

// runtimeFor maps SLA virtualization names onto NodeEngine runtime identifiers.
func runtimeFor(virt string) string {
	switch virt {
	case "container", "docker", "":
		return "docker"
	default:
		return virt // unikernel, crosvm, custom OCI runtimes pass through
	}
}

// ExpandSLA parses a deployment descriptor and expands every microservice into a
// DeployableService (plan §1.4: job naming + per-service scheduling unit).
func ExpandSLA(data []byte) ([]DeployableService, error) {
	var sla SLA
	if err := json.Unmarshal(data, &sla); err != nil {
		return nil, fmt.Errorf("invalid SLA: %w", err)
	}
	if len(sla.Applications) == 0 {
		return nil, errors.New("SLA contains no applications")
	}
	out := make([]DeployableService, 0)
	for _, app := range sla.Applications {
		if app.Name == "" || app.Namespace == "" {
			return nil, errors.New("application_name and application_namespace are required")
		}
		for _, ms := range app.Microservices {
			if ms.Name == "" || ms.Namespace == "" {
				return nil, errors.New("microservice_name and microservice_namespace are required")
			}
			jobName := fmt.Sprintf("%s.%s.%s.%s", app.Name, app.Namespace, ms.Name, ms.Namespace)
			out = append(out, DeployableService{
				JobName:       jobName,
				RRIP:          ms.Addresses.RRIP,
				RestartPolicy: ms.RestartPolicy,
				Service: model.Service{
					JobID:      jobName,
					Sname:      jobName,
					Instance:   0,
					Image:      ms.Code,
					Commands:   ms.Cmd,
					Env:        ms.Environment,
					Ports:      ms.Port,
					Runtime:    runtimeFor(ms.Virtualization),
					Vcpus:      ms.Vcpus,
					Vgpus:      ms.Vgpus,
					Memory:     ms.Memory,
					Storage:    ms.Storage,
					OneShot:    ms.OneShot,
					Privileged: ms.Privileged,
				},
			})
		}
	}
	return out, nil
}
