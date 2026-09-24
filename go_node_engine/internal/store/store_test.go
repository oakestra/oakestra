package store

import (
	"go_node_engine/model"
	"testing"
)

func TestUpsertRemoveLoad(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if recs, err := Load(); err != nil || len(recs) != 0 {
		t.Fatalf("fresh store: got %v (err %v); want empty", recs, err)
	}

	svcA := model.Service{Sname: "app.ns.a.ns", Instance: 0, Runtime: "docker"}
	svcB := model.Service{Sname: "app.ns.a.ns", Instance: 1, Runtime: "docker"}

	if err := Upsert(Record{JobName: svcA.Sname, Service: svcA, DesiredState: DesiredRunning}); err != nil {
		t.Fatal(err)
	}
	if err := Upsert(Record{JobName: svcB.Sname, Service: svcB, DesiredState: DesiredRunning}); err != nil {
		t.Fatal(err)
	}

	recs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("after 2 upserts: got %d records; want 2", len(recs))
	}

	// Upsert same (name, instance) updates in place, not appends.
	if err := Upsert(Record{JobName: svcA.Sname, Service: svcA, DesiredState: DesiredRunning, Generation: 5}); err != nil {
		t.Fatal(err)
	}
	recs, _ = Load()
	if len(recs) != 2 {
		t.Fatalf("upsert of existing key changed count: got %d; want 2", len(recs))
	}

	// Remove one instance, the other survives.
	if err := Remove(svcA.Sname, 0); err != nil {
		t.Fatal(err)
	}
	recs, _ = Load()
	if len(recs) != 1 || recs[0].Service.Instance != 1 {
		t.Fatalf("after remove: got %v; want only instance 1", recs)
	}
}
