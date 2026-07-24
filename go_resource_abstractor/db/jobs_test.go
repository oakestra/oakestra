package db

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCreateAppSetsApplicationID(t *testing.T) {
	ctx := context.Background()

	created, err := testStore.CreateApp(ctx, bson.M{"application_name": uniqueName("app")})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	id := ExtractID(created)
	if id == "" {
		t.Fatalf("created app has no usable _id: %v", created)
	}
	if created["applicationID"] != id {
		t.Errorf("applicationID = %v, want %v (own _id)", created["applicationID"], id)
	}
}

func TestAppendJobInstanceIdempotency(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{"job_name": uniqueName("job"), "instance_list": bson.A{}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	instance := map[string]any{"instance_number": 1, "status": "RUNNING"}

	// First append succeeds.
	updated, err := testStore.AppendJobInstance(ctx, jobID, 1, bson.M{
		"instance_list": []any{instance},
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	instanceList, _ := updated["instance_list"].(bson.A)
	if len(instanceList) != 1 {
		t.Fatalf("instance_list length = %d, want 1: %v", len(instanceList), updated)
	}

	// Appending the same instance_number again is refused.
	if _, err := testStore.AppendJobInstance(ctx, jobID, 1, bson.M{
		"instance_list": []any{instance},
	}); !errors.Is(err, ErrInstanceConflict) {
		t.Errorf("re-append of existing instance: got %v, want ErrInstanceConflict", err)
	}

	// An empty instance_list is refused.
	if _, err := testStore.AppendJobInstance(ctx, jobID, 2, bson.M{
		"instance_list": []any{},
	}); !errors.Is(err, ErrInstanceConflict) {
		t.Errorf("empty instance_list: got %v, want ErrInstanceConflict", err)
	}

	// Appending to a job that doesn't exist is refused the same way,
	// matching jobs_db.append_job_instance's None-on-missing-job behavior.
	missingJobID := bson.NewObjectID().Hex()
	if _, err := testStore.AppendJobInstance(ctx, missingJobID, 1, bson.M{
		"instance_list": []any{instance},
	}); !errors.Is(err, ErrInstanceConflict) {
		t.Errorf("append to missing job: got %v, want ErrInstanceConflict", err)
	}
}

// TestAppendJobInstanceConcurrentSameNumberOnlyOneWins is a regression test
// for a TOCTOU race: AppendJobInstance used to check for an existing
// instance via a separate read (FindJobInstance) before pushing, so two
// concurrent requests for the same instance_number could both pass the
// check and both push, producing duplicate instance_number entries. The
// check now lives in the FindOneAndUpdate filter itself
// (instance_list.instance_number: {$ne: N}), making it atomic with the
// write.
func TestAppendJobInstanceConcurrentSameNumberOnlyOneWins(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{"job_name": uniqueName("race-job"), "instance_list": bson.A{}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	const attempts = 20
	instance := map[string]any{"instance_number": 1, "status": "RUNNING"}

	var wg sync.WaitGroup
	successes := make(chan struct{}, attempts)
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if _, err := testStore.AppendJobInstance(ctx, jobID, 1, bson.M{
				"instance_list": []any{instance},
			}); err == nil {
				successes <- struct{}{}
			} else if !errors.Is(err, ErrInstanceConflict) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	close(successes)

	successCount := 0
	for range successes {
		successCount++
	}
	if successCount != 1 {
		t.Errorf("successful appends = %d, want exactly 1", successCount)
	}

	final, err := testStore.FindJobByID(ctx, jobID, bson.M{})
	if err != nil {
		t.Fatalf("find job: %v", err)
	}
	instanceList, _ := final["instance_list"].(bson.A)
	if len(instanceList) != 1 {
		t.Errorf("instance_list length = %d, want exactly 1 (no duplicates): %v", len(instanceList), instanceList)
	}
}

func TestUpdateJobInstanceHistoryCapAndPositionalUpdate(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{
		"job_name":      uniqueName("job"),
		"instance_list": bson.A{bson.M{"instance_number": 1}},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	const updates = 150
	var last bson.M
	for i := range updates {
		last, err = testStore.UpdateJobInstance(ctx, jobID, 1, bson.M{
			"cpu_percent":    float64(i),
			"memory_percent": float64(i) / 2,
			"status":         "RUNNING",
		})
		if err != nil {
			t.Fatalf("update job instance (iteration %d): %v", i, err)
		}
	}

	instanceList, ok := last["instance_list"].(bson.A)
	if !ok || len(instanceList) != 1 {
		t.Fatalf("expected exactly one instance, got: %#v", last["instance_list"])
	}
	instance, ok := instanceList[0].(bson.D)
	if !ok {
		t.Fatalf("instance is not a document: %#v", instanceList[0])
	}

	cpuHistory, ok := docValue(instance, "cpu_history").(bson.A)
	if !ok {
		t.Fatalf("cpu_history is not an array: %#v", docValue(instance, "cpu_history"))
	}
	if len(cpuHistory) != 100 {
		t.Errorf("cpu_history length = %d, want 100 (capped)", len(cpuHistory))
	}

	if got := docValue(instance, "cpu_percent"); got != float64(updates-1) {
		t.Errorf("cpu_percent = %v, want %v (last $set value)", got, updates-1)
	}
	if got := docValue(instance, "status"); got != "RUNNING" {
		t.Errorf("status = %v, want RUNNING", got)
	}

	// An instance_number that doesn't exist on the job is a 404, not a
	// silent no-op.
	if _, err := testStore.UpdateJobInstance(ctx, jobID, 999, bson.M{"cpu_percent": 1.0}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of missing instance: got %v, want ErrNotFound", err)
	}
}

func TestUpdateJobInstanceDefaultsStatusDetailAndLogs(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{
		"job_name":      uniqueName("job"),
		"instance_list": bson.A{bson.M{"instance_number": 1}},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	updated, err := testStore.UpdateJobInstance(ctx, jobID, 1, bson.M{"cpu_percent": 1.0})
	if err != nil {
		t.Fatalf("update job instance: %v", err)
	}

	instanceList := updated["instance_list"].(bson.A)
	instance := instanceList[0].(bson.D)

	if got := docValue(instance, "status_detail"); got != "No extra information" {
		t.Errorf("status_detail default = %v, want %q", got, "No extra information")
	}
	if got := docValue(instance, "logs"); got != "" {
		t.Errorf("logs default = %v, want empty string", got)
	}
}

func TestDeleteJobInstance(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{
		"job_name": uniqueName("job"),
		"instance_list": bson.A{
			bson.M{"instance_number": 1},
			bson.M{"instance_number": 2},
		},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	updated, err := testStore.DeleteJobInstance(ctx, jobID, 1)
	if err != nil {
		t.Fatalf("delete job instance: %v", err)
	}
	instanceList := updated["instance_list"].(bson.A)
	if len(instanceList) != 1 {
		t.Fatalf("instance_list length = %d, want 1 after deleting instance 1", len(instanceList))
	}
	if got := docValue(instanceList[0].(bson.D), "instance_number"); got != int32(2) {
		t.Errorf("remaining instance_number = %v, want 2", got)
	}

	// Deleting an instance_number that doesn't exist is a no-op that still
	// returns the job (not a 404) - matches jobs_db.delete_job_instance,
	// where only a missing *job* (not a missing instance) yields None.
	updated, err = testStore.DeleteJobInstance(ctx, jobID, 999)
	if err != nil {
		t.Fatalf("delete of already-absent instance should not error: %v", err)
	}
	if len(updated["instance_list"].(bson.A)) != 1 {
		t.Errorf("instance_list should be unchanged by deleting an absent instance_number")
	}

	// Deleting from a job that doesn't exist at all is ErrNotFound.
	missingJobID := bson.NewObjectID().Hex()
	if _, err := testStore.DeleteJobInstance(ctx, missingJobID, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete instance on missing job: got %v, want ErrNotFound", err)
	}
}

func TestFindJobInstanceFiltersToOneInstance(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, bson.M{
		"job_name": uniqueName("job"),
		"instance_list": bson.A{
			bson.M{"instance_number": 1, "status": "RUNNING"},
			bson.M{"instance_number": 2, "status": "PENDING"},
		},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := ExtractID(job)

	result, err := testStore.FindJobInstance(ctx, jobID, 2)
	if err != nil {
		t.Fatalf("find job instance: %v", err)
	}

	instanceList := result["instance_list"].(bson.A)
	if len(instanceList) != 1 {
		t.Fatalf("expected exactly one instance in the filtered result, got %d", len(instanceList))
	}
	if got := docValue(instanceList[0].(bson.D), "status"); got != "PENDING" {
		t.Errorf("filtered instance status = %v, want PENDING", got)
	}

	if _, err := testStore.FindJobInstance(ctx, jobID, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("find missing instance: got %v, want ErrNotFound", err)
	}
}

func TestUpsertJobByName(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("upsert-job")

	if _, err := testStore.FindJobByName(ctx, name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before creation, got %v", err)
	}

	created, err := testStore.CreateJob(ctx, bson.M{"job_name": name, "status": "PENDING"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	found, err := testStore.FindJobByName(ctx, name)
	if err != nil {
		t.Fatalf("find job by name: %v", err)
	}
	if ExtractID(found) != ExtractID(created) {
		t.Errorf("found job id %v, want %v", ExtractID(found), ExtractID(created))
	}

	updated, err := testStore.UpdateJob(ctx, ExtractID(found), bson.M{"status": "RUNNING"})
	if err != nil {
		t.Fatalf("update job: %v", err)
	}
	if updated["status"] != "RUNNING" {
		t.Errorf("status = %v, want RUNNING", updated["status"])
	}
}
