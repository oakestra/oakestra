package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

func TestAppendJobInstanceIdempotency(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{JobName: model.Ptr(uniqueName("job")), InstanceList: model.Ptr([]model.JobInstance{})})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	instance := model.JobInstance{InstanceNumber: model.Ptr(int64(1)), Status: model.Ptr("RUNNING")}

	// First append succeeds.
	updated, err := testStore.AppendJobInstance(ctx, jobID, 1, instance)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	if updated.InstanceList == nil || len(*updated.InstanceList) != 1 {
		t.Fatalf("instance_list = %+v, want 1 entry", updated.InstanceList)
	}

	// The same instance_number again is refused.
	if _, err := testStore.AppendJobInstance(ctx, jobID, 1, instance); !errors.Is(err, errs.ErrInstanceExists) {
		t.Errorf("re-append of existing instance: got %v, want errs.ErrInstanceExists", err)
	}

	// Appending to a missing job is refused the same way.
	missingJobID := bson.NewObjectID().Hex()
	if _, err := testStore.AppendJobInstance(ctx, missingJobID, 1, instance); !errors.Is(err, errs.ErrInstanceExists) {
		t.Errorf("append to missing job: got %v, want errs.ErrInstanceExists", err)
	}
}

// Two concurrent appends for the same instance_number must not both
// succeed: the check lives in the FindOneAndUpdate filter, atomic with the
// write.
func TestAppendJobInstanceConcurrentSameNumberOnlyOneWins(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{JobName: model.Ptr(uniqueName("race-job")), InstanceList: model.Ptr([]model.JobInstance{})})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	const attempts = 20
	instance := model.JobInstance{InstanceNumber: model.Ptr(int64(1)), Status: model.Ptr("RUNNING")}

	var wg sync.WaitGroup
	successes := make(chan struct{}, attempts)
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if _, err := testStore.AppendJobInstance(ctx, jobID, 1, instance); err == nil {
				successes <- struct{}{}
			} else if !errors.Is(err, errs.ErrInstanceExists) {
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
	if final.InstanceList == nil || len(*final.InstanceList) != 1 {
		t.Errorf("instance_list = %+v, want exactly 1 entry (no duplicates)", final.InstanceList)
	}
}

func TestUpdateJobInstanceHistoryCapAndPositionalUpdate(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName:      model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{{InstanceNumber: model.Ptr(int64(1))}}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	const updates = 150
	var last model.Job
	for i := range updates {
		last, err = testStore.UpdateJobInstance(ctx, jobID, 1, model.JobInstance{
			CPUPercent:    model.Ptr(model.Float(i)),
			MemoryPercent: model.Ptr(model.Float(i) / 2),
			Status:        model.Ptr("RUNNING"),
		})
		if err != nil {
			t.Fatalf("update job instance (iteration %d): %v", i, err)
		}
	}

	if last.InstanceList == nil || len(*last.InstanceList) != 1 {
		t.Fatalf("expected exactly one instance, got: %+v", last.InstanceList)
	}
	instance := (*last.InstanceList)[0]

	if instance.CPUHistory == nil || len(*instance.CPUHistory) != 100 {
		t.Errorf("cpu_history = %+v, want 100 entries (capped)", instance.CPUHistory)
	}

	if instance.CPUPercent == nil || *instance.CPUPercent != model.Float(updates-1) {
		t.Errorf("cpu_percent = %v, want %v (last $set value)", instance.CPUPercent, updates-1)
	}
	if instance.Status == nil || *instance.Status != "RUNNING" {
		t.Errorf("status = %v, want RUNNING", instance.Status)
	}

	// An instance_number that doesn't exist on the job is a 404, not a
	// silent no-op.
	if _, err := testStore.UpdateJobInstance(ctx, jobID, 999, model.JobInstance{CPUPercent: model.Ptr(model.Float(1))}); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("update of missing instance: got %v, want errs.ErrNotFound", err)
	}
}

func TestUpdateJobInstanceDefaultsStatusDetailAndLogs(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName:      model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{{InstanceNumber: model.Ptr(int64(1))}}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	updated, err := testStore.UpdateJobInstance(ctx, jobID, 1, model.JobInstance{CPUPercent: model.Ptr(model.Float(1))})
	if err != nil {
		t.Fatalf("update job instance: %v", err)
	}

	instance := (*updated.InstanceList)[0]
	if instance.StatusDetail == nil || *instance.StatusDetail != "No extra information" {
		t.Errorf("status_detail default = %v, want %q", instance.StatusDetail, "No extra information")
	}
	if instance.Logs == nil || *instance.Logs != "" {
		t.Errorf("logs default = %v, want empty string", instance.Logs)
	}
}

// TestUpdateJobInstanceNullFieldsSurviveARead is the regression this fix
// targets: UpdateJobInstance intentionally $sets a literal null for every
// instance field the request body omits (status, host_ip, worker_id,
// publicip, host_port, last_modified_timestamp - status_detail/logs are
// the two exceptions, defaulted instead), matching HEAD's
// jobs_db.update_job_instance. A GET of the job afterwards must return
// those keys as JSON null, not omit them - cluster_manager's
// job_management.py reads e.g. i["status"] unconditionally and would
// KeyError on an omitted key.
func TestUpdateJobInstanceNullFieldsSurviveARead(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName:      model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{{InstanceNumber: model.Ptr(int64(1))}}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	// Only cpu_percent is set; every other instance field is omitted from
	// the request body, so UpdateJobInstance writes null for each of them.
	if _, err := testStore.UpdateJobInstance(ctx, jobID, 1, model.JobInstance{
		CPUPercent: model.Ptr(model.Float(50)),
	}); err != nil {
		t.Fatalf("update job instance: %v", err)
	}

	found, err := testStore.FindJobByID(ctx, jobID, bson.M{})
	if err != nil {
		t.Fatalf("find job: %v", err)
	}

	out, err := json.Marshal(found)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	var decoded struct {
		InstanceList []map[string]json.RawMessage `json:"instance_list"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal job json: %v", err)
	}
	if len(decoded.InstanceList) != 1 {
		t.Fatalf("instance_list = %+v, want 1 entry", decoded.InstanceList)
	}
	instance := decoded.InstanceList[0]

	for _, key := range []string{"status", "host_ip", "worker_id", "publicip", "host_port", "last_modified_timestamp"} {
		raw, ok := instance[key]
		if !ok {
			t.Errorf("%s missing from JSON output, want present with a null value", key)
			continue
		}
		if string(raw) != "null" {
			t.Errorf("%s = %s, want null", key, raw)
		}
	}
	// status_detail/logs default to placeholder strings instead of null.
	if raw, ok := instance["status_detail"]; !ok || string(raw) != `"No extra information"` {
		t.Errorf(`status_detail = %v, want "No extra information"`, instance["status_detail"])
	}
	if raw, ok := instance["logs"]; !ok || string(raw) != `""` {
		t.Errorf(`logs = %v, want ""`, instance["logs"])
	}
	// cpu_percent, the field that was actually set, must carry its value.
	if raw, ok := instance["cpu_percent"]; !ok || string(raw) != "50" {
		t.Errorf("cpu_percent = %v, want 50", instance["cpu_percent"])
	}
}

func TestDeleteJobInstance(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName: model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{
			{InstanceNumber: model.Ptr(int64(1))},
			{InstanceNumber: model.Ptr(int64(2))},
		}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	updated, err := testStore.DeleteJobInstance(ctx, jobID, 1)
	if err != nil {
		t.Fatalf("delete job instance: %v", err)
	}
	if updated.InstanceList == nil || len(*updated.InstanceList) != 1 {
		t.Fatalf("instance_list = %+v, want 1 entry after deleting instance 1", updated.InstanceList)
	}
	remaining := (*updated.InstanceList)[0].InstanceNumber
	if remaining == nil || *remaining != 2 {
		t.Errorf("remaining instance_number = %v, want 2", remaining)
	}

	// Deleting an instance_number that doesn't exist is a no-op that still
	// returns the job, not a 404.
	updated, err = testStore.DeleteJobInstance(ctx, jobID, 999)
	if err != nil {
		t.Fatalf("delete of already-absent instance should not error: %v", err)
	}
	if updated.InstanceList == nil || len(*updated.InstanceList) != 1 {
		t.Errorf("instance_list should be unchanged by deleting an absent instance_number")
	}

	// Deleting from a job that doesn't exist at all is errs.ErrNotFound.
	missingJobID := bson.NewObjectID().Hex()
	if _, err := testStore.DeleteJobInstance(ctx, missingJobID, 1); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("delete instance on missing job: got %v, want errs.ErrNotFound", err)
	}
}

func TestFindJobInstanceFiltersToOneInstance(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName: model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{
			{InstanceNumber: model.Ptr(int64(1)), Status: model.Ptr("RUNNING")},
			{InstanceNumber: model.Ptr(int64(2)), Status: model.Ptr("PENDING")},
		}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobID := *job.ID

	result, err := testStore.FindJobInstance(ctx, jobID, 2)
	if err != nil {
		t.Fatalf("find job instance: %v", err)
	}

	if result.InstanceList == nil || len(*result.InstanceList) != 1 {
		t.Fatalf("expected exactly one instance in the filtered result, got %+v", result.InstanceList)
	}
	resultInstances := *result.InstanceList
	if resultInstances[0].Status == nil || *resultInstances[0].Status != "PENDING" {
		t.Errorf("filtered instance status = %v, want PENDING", resultInstances[0].Status)
	}

	if _, err := testStore.FindJobInstance(ctx, jobID, 999); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("find missing instance: got %v, want errs.ErrNotFound", err)
	}
}

func TestUpsertJobByName(t *testing.T) {
	ctx := context.Background()
	name := uniqueName("upsert-job")

	if _, err := testStore.FindJobByName(ctx, name); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected errs.ErrNotFound before creation, got %v", err)
	}

	created, err := testStore.CreateJob(ctx, model.Job{JobName: model.Ptr(name), Extra: model.Extra{"status": "PENDING"}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if created.JobName == nil || *created.JobName != name {
		t.Fatalf("job_name = %v, want %v", created.JobName, name)
	}

	found, err := testStore.FindJobByName(ctx, name)
	if err != nil {
		t.Fatalf("find job by name after create: %v", err)
	}
	if *found.ID != *created.ID {
		t.Errorf("found id %v, want %v", *found.ID, *created.ID)
	}
}

func TestBuildJobFilterNarrowsByInstanceNumber(t *testing.T) {
	ctx := context.Background()

	job, err := testStore.CreateJob(ctx, model.Job{
		JobName: model.Ptr(uniqueName("job")),
		InstanceList: model.Ptr([]model.JobInstance{
			{InstanceNumber: model.Ptr(int64(1))},
		}),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	matching := 1
	if _, err := testStore.FindJobByID(ctx, *job.ID, BuildJobFilter(&matching)); err != nil {
		t.Errorf("find by id with matching instance_number filter: %v", err)
	}

	missing := 999
	if _, err := testStore.FindJobByID(ctx, *job.ID, BuildJobFilter(&missing)); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("find by id with non-matching instance_number filter: got %v, want errs.ErrNotFound", err)
	}
}

// TestJobInstanceStringNumericFieldsFromOldDocument inserts a raw document
// shaped the way the Python service actually wrote it (and the way
// NodeEngine/cluster_manager still send a PATCH): cpu_percent, memory_percent,
// disk and host_port as strings. FindJobByID used to 500 decoding a BSON
// string into *float64/*int64; it must now decode via model.Float/model.Int,
// and a subsequent UpdateJobInstance normalizes the stored value to a BSON
// number.
func TestJobInstanceStringNumericFieldsFromOldDocument(t *testing.T) {
	ctx := context.Background()

	legacy := bson.D{
		{Key: "job_name", Value: uniqueName("legacy-job")},
		{Key: "instance_list", Value: bson.A{
			bson.D{
				{Key: "instance_number", Value: int64(1)},
				{Key: "cpu_percent", Value: "12.345600"},
				{Key: "memory_percent", Value: "34.560000"},
				{Key: "disk", Value: "0"},
				{Key: "host_port", Value: "50011"},
			},
		}},
	}
	inserted, err := testStore.jobs.InsertOne(ctx, legacy)
	if err != nil {
		t.Fatalf("insert legacy document: %v", err)
	}
	jobOID, ok := inserted.InsertedID.(bson.ObjectID)
	if !ok {
		t.Fatalf("InsertedID = %#v, want a bson.ObjectID", inserted.InsertedID)
	}
	jobID := jobOID.Hex()

	found, err := testStore.FindJobByID(ctx, jobID, bson.M{})
	if err != nil {
		t.Fatalf("find job with string-valued fields: %v", err)
	}
	if found.InstanceList == nil || len(*found.InstanceList) != 1 {
		t.Fatalf("instance_list = %+v, want 1 entry", found.InstanceList)
	}
	instance := (*found.InstanceList)[0]
	if instance.CPUPercent == nil || *instance.CPUPercent != model.Float(12.3456) {
		t.Errorf("cpu_percent = %v, want 12.3456", instance.CPUPercent)
	}
	if instance.MemoryPercent == nil || *instance.MemoryPercent != model.Float(34.56) {
		t.Errorf("memory_percent = %v, want 34.56", instance.MemoryPercent)
	}
	if instance.HostPort == nil || *instance.HostPort != model.Int(50011) {
		t.Errorf("host_port = %v, want 50011", instance.HostPort)
	}

	// A status report with the same fields sent as strings - what NodeEngine
	// and cluster_manager actually send - must decode instead of failing.
	if _, err := testStore.UpdateJobInstance(ctx, jobID, 1, model.JobInstance{
		CPUPercent: model.Ptr(model.Float(99.9)),
		HostPort:   model.Ptr(model.Int(50012)),
	}); err != nil {
		t.Fatalf("update job instance with string-valued fields present: %v", err)
	}

	// The write normalizes the field it touched to a BSON number. Decoding
	// cpu_percent as `any` (not model.Float) reveals the raw stored type
	// instead of letting Float's lenient UnmarshalBSONValue mask it.
	var raw struct {
		InstanceList []struct {
			CPUPercent any `bson:"cpu_percent"`
		} `bson:"instance_list"`
	}
	if err := testStore.jobs.FindOne(ctx, bson.M{"_id": jobOID}).Decode(&raw); err != nil {
		t.Fatalf("read raw document: %v", err)
	}
	if len(raw.InstanceList) != 1 {
		t.Fatalf("instance_list = %+v, want 1 entry", raw.InstanceList)
	}
	switch v := raw.InstanceList[0].CPUPercent.(type) {
	case float64, int32, int64:
		// normalized to a BSON number, as this service always writes it
	default:
		t.Errorf("cpu_percent stored as %T after update, want a BSON number", v)
	}
}
