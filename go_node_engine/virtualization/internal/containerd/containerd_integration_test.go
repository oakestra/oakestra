//go:build integration && linux

package containerd

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"go_node_engine/model"
	"go_node_engine/util/taskid"
	virtrt "go_node_engine/virtualization/internal/runtime"

	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/defaults"
)

const (
	testImage = "docker.io/library/busybox:latest"
	// Pulling busybox on a cold runner is the slow part, so Deploy gets more room.
	deployTimeout = 2 * time.Minute
	eventTimeout  = 30 * time.Second
)

// These tests need a real containerd and are meant for CI. They are opt-in
// because newContainerdRuntime() calls forceContainerCleanup(), which deletes
// every container in the "oakestra" namespace. On a live worker that would
// kill all running workloads.
func requireDisposableContainerd(t *testing.T) {
	t.Helper()
	if os.Getenv("OAKESTRA_CONTAINERD_INTEGRATION") != "1" {
		t.Skip("set OAKESTRA_CONTAINERD_INTEGRATION=1 to run (destroys all containers in the oakestra namespace)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root to talk to containerd")
	}
	if _, err := os.Stat(CONTAINERD_SOCKET); err != nil {
		t.Skipf("containerd socket not available: %v", err)
	}
}

type statusRecorder struct {
	mu     sync.Mutex
	events map[string]chan model.Service
}

func newStatusRecorder() *statusRecorder {
	return &statusRecorder{events: map[string]chan model.Service{}}
}

func (s *statusRecorder) channel(sname string) chan model.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.events[sname]
	if !ok {
		ch = make(chan model.Service, 8)
		s.events[sname] = ch
	}
	return ch
}

func (s *statusRecorder) handler(service model.Service) {
	s.channel(service.Sname) <- service
}

func (s *statusRecorder) waitFor(t *testing.T, sname, status string) {
	t.Helper()
	select {
	case svc := <-s.channel(sname):
		if svc.Status != status {
			t.Fatalf("service %s: expected status %s, got %s (%s)", sname, status, svc.Status, svc.StatusDetail)
		}
	case <-time.After(eventTimeout):
		t.Fatalf("service %s: timed out waiting for status %s", sname, status)
	}
}

// deployWithTimeout keeps a stuck pull or task start from hanging the whole run.
func deployWithTimeout(t *testing.T, r *ContainerRuntime, svc model.Service, handler func(model.Service)) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- r.Deploy(svc, handler) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("deploy of %s failed: %v", svc.Sname, err)
		}
	case <-time.After(deployTimeout):
		t.Fatalf("deploy of %s timed out", svc.Sname)
	}
}

func TestContainerdDaemonCompatibility(t *testing.T) {
	requireDisposableContainerd(t)

	model.GetNodeInfo().SetLogDirectory(t.TempDir())

	rt := newContainerdRuntime(virtrt.RuntimeInfo{})
	r, ok := rt.(*ContainerRuntime)
	if !ok || r.containerClient == nil {
		t.Fatal("could not connect to containerd")
	}
	// Not registered with t.Cleanup because Stop() closes the client, which
	// would race with any subtest still running.
	defer r.Stop()

	statuses := newStatusRecorder()

	// ResourceMonitoring never returns, so this goroutine just leaks until the
	// test process exits. The handler stays safe to call after the test ends.
	var resMu sync.Mutex
	var latest []model.Resources
	go r.ResourceMonitoring(1*time.Second, func(res []model.Resources) {
		resMu.Lock()
		defer resMu.Unlock()
		latest = res
	})
	isReportedRunning := func(sname string, instance int) bool {
		resMu.Lock()
		defer resMu.Unlock()
		for _, res := range latest {
			if res.Sname == sname && res.Instance == instance && res.Status == model.SERVICE_RUNNING {
				return true
			}
		}
		return false
	}

	t.Run("long running service can be monitored and undeployed", func(t *testing.T) {
		sname := "it.app.longrunning.ns"
		taskId := taskid.Generate(sname, 0)
		deployWithTimeout(t, r, model.Service{
			Sname:    sname,
			Image:    testImage,
			Commands: []string{"sleep", "300"},
			Runtime:  string(model.CONTAINER_RUNTIME),
		}, statuses.handler)

		deadline := time.Now().Add(eventTimeout)
		for !isReportedRunning(sname, 0) {
			if time.Now().After(deadline) {
				t.Fatal("service never reported as RUNNING by ResourceMonitoring")
			}
			time.Sleep(500 * time.Millisecond)
		}

		// The snapshotter name has to be read while the container still exists.
		container, err := r.containerClient.LoadContainer(r.ctx, convertTaskIdToContainerId(taskId))
		if err != nil {
			t.Fatalf("container not found after deploy: %v", err)
		}
		info, err := container.Info(r.ctx)
		if err != nil {
			t.Fatalf("unable to read container info: %v", err)
		}
		snapshotterName := info.Snapshotter
		if snapshotterName == "" {
			snapshotterName = defaults.DefaultSnapshotter
		}
		key := snapshotKey(taskId)

		if err := r.Undeploy(sname, 0); err != nil {
			t.Fatalf("undeploy failed: %v", err)
		}
		statuses.waitFor(t, sname, model.SERVICE_DEAD)

		containers, err := r.containerClient.Containers(r.ctx)
		if err != nil {
			t.Fatalf("unable to list containers: %v", err)
		}
		for _, c := range containers {
			ci, err := c.Info(r.ctx)
			if err != nil {
				continue
			}
			if ci.Labels[TASK_ID_LABEL] == taskId {
				t.Errorf("container %s for task %s still exists after undeploy", c.ID(), taskId)
			}
		}

		snapshotter := r.containerClient.SnapshotService(snapshotterName)
		defer snapshotter.Close()
		err = snapshotter.Walk(r.ctx, func(_ context.Context, si snapshots.Info) error {
			if si.Name == key {
				t.Errorf("snapshot %s still exists after undeploy", key)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unable to list snapshots: %v", err)
		}
	})

	oneShots := []struct {
		name, sname, command, want string
	}{
		{"one shot that succeeds is reported completed", "it.app.oneshotok.ns", "true", model.SERVICE_COMPLETED},
		{"one shot that fails is reported dead", "it.app.oneshotfail.ns", "false", model.SERVICE_DEAD},
	}
	for _, tc := range oneShots {
		t.Run(tc.name, func(t *testing.T) {
			deployWithTimeout(t, r, model.Service{
				Sname:    tc.sname,
				Image:    testImage,
				Commands: []string{tc.command},
				OneShot:  true,
				Runtime:  string(model.CONTAINER_RUNTIME),
			}, statuses.handler)
			statuses.waitFor(t, tc.sname, tc.want)
		})
	}
}
