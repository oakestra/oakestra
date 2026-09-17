package abstractor

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// JobFilter narrows List to jobs matching the given fields; a zero field
// imposes no constraint.
type JobFilter struct {
	ApplicationID string
	JobName       string
}

func (f JobFilter) toBSON() bson.M {
	filter := bson.M{}
	if f.ApplicationID != "" {
		filter["applicationID"] = f.ApplicationID
	}
	if f.JobName != "" {
		filter["job_name"] = f.JobName
	}
	return filter
}

// Jobs is the job sub-service: create/read/update/delete for jobs and their
// per-worker instances, with pre/post webhooks fired under the "jobs"
// entity around every write.
type Jobs struct {
	store  *store.Store
	entity entityHooks
}

// List returns jobs matching filter.
func (j *Jobs) List(ctx context.Context, filter JobFilter) ([]model.Job, error) {
	return j.store.FindJobs(ctx, filter.toBSON())
}

// Get returns a single job by id, optionally narrowed to jobs carrying an
// instance with the given number.
func (j *Jobs) Get(ctx context.Context, id string, instanceNumber *int) (model.Job, error) {
	return j.store.FindJobByID(ctx, id, store.BuildJobFilter(instanceNumber))
}

// Create runs pre_create/post_create around inserting a new job.
func (j *Jobs) Create(ctx context.Context, data model.Job) (model.Job, error) {
	return create(ctx, j.entity, data, j.store.CreateJob)
}

// Upsert updates the job named data.JobName if one exists, else creates it.
//
// job_name has no unique index: a real deployment may already have legacy
// duplicate job names, which would make one fail to build. The
// FindJobByName lookup and the create/update that follows also aren't
// atomic, so two concurrent Upserts for a brand-new name can both take the
// create branch and produce duplicates. Even an atomic upsert wouldn't fix
// that, since which pre-hook fires (pre_create vs pre_update) depends on
// whether a match exists, and that's a decision it can't make before it
// runs.
func (j *Jobs) Upsert(ctx context.Context, data model.Job) (model.Job, error) {
	if data.JobName != nil && *data.JobName != "" {
		existing, err := j.store.FindJobByName(ctx, *data.JobName)
		switch {
		case err == nil:
			// Not Update: pre_update isn't told which document matched by
			// name, so data["_id"] stays unset.
			return update(ctx, j.entity, *existing.ID, data, false, func(ctx context.Context, d model.Job) (model.Job, error) {
				return j.store.UpdateJob(ctx, *existing.ID, d)
			})
		case !errors.Is(err, errs.ErrNotFound):
			return model.Job{}, err
		}
	}
	return j.Create(ctx, data)
}

// Update runs pre_update/post_update around a $set patch addressed by id.
// The pre_update hook sees data["_id"] = id.
func (j *Jobs) Update(ctx context.Context, id string, patch model.Job) (model.Job, error) {
	return update(ctx, j.entity, id, patch, true, func(ctx context.Context, d model.Job) (model.Job, error) {
		return j.store.UpdateJob(ctx, id, d)
	})
}

// Delete runs post_delete around removing a job by id, and returns the
// deleted document.
func (j *Jobs) Delete(ctx context.Context, id string) (model.Job, error) {
	return del(ctx, j.entity, id, func(ctx context.Context) (model.Job, error) {
		return j.store.DeleteJob(ctx, id)
	})
}

// GetInstance returns the job with instance_list filtered down to the
// matching instance_number. No hooks fire - it's a read.
func (j *Jobs) GetInstance(ctx context.Context, jobID string, instanceNumber int) (model.Job, error) {
	return j.store.FindJobInstance(ctx, jobID, instanceNumber)
}

// AppendInstance runs pre_create/post_create - appending an instance counts
// as a create event even though it targets an existing job - and pushes
// only the last element of instances onto the job's instance_list; earlier
// elements are ignored.
//
// The pre_create hook sees the whole payload, {"_id": jobID,
// "instance_list": instances}, not just the element being appended. What it
// returns is resolved back down to a single instance, the last element of
// its own instance_list, before being persisted.
func (j *Jobs) AppendInstance(ctx context.Context, jobID string, instanceNumber int, instances []model.JobInstance) (model.Job, error) {
	instanceMaps := make([]any, len(instances))
	for i, inst := range instances {
		m, err := toMap(inst)
		if err != nil {
			return model.Job{}, err
		}
		instanceMaps[i] = m
	}
	payload := map[string]any{"_id": jobID, "instance_list": instanceMaps}
	payload = j.entity.dispatcher.PreCreate(ctx, j.entity.name, payload)

	resolved, err := lastInstanceFromPayload(payload)
	if err != nil {
		return model.Job{}, err
	}
	updated, err := j.store.AppendJobInstance(ctx, jobID, instanceNumber, resolved)
	if err != nil {
		return model.Job{}, err
	}
	j.entity.dispatcher.PostCreate(j.entity.name, jobID)
	return updated, nil
}

// UpdateInstance runs pre_update/post_update around a worker's usage report
// for one job instance. Like Update, the pre_update hook sees
// data["_id"] = jobID.
func (j *Jobs) UpdateInstance(ctx context.Context, jobID string, instanceNumber int, patch model.JobInstance) (model.Job, error) {
	return update(ctx, j.entity, jobID, patch, true, func(ctx context.Context, d model.JobInstance) (model.Job, error) {
		return j.store.UpdateJobInstance(ctx, jobID, instanceNumber, d)
	})
}

// DeleteInstance runs post_delete, keyed off jobID, around removing one
// instance from the job's instance_list. jobID happens to be the job's own
// _id here too.
func (j *Jobs) DeleteInstance(ctx context.Context, jobID string, instanceNumber int) (model.Job, error) {
	return del(ctx, j.entity, jobID, func(ctx context.Context) (model.Job, error) {
		return j.store.DeleteJobInstance(ctx, jobID, instanceNumber)
	})
}

// lastInstanceFromPayload pulls the last element of
// payload["instance_list"] back out as a model.JobInstance. An empty or
// missing instance_list is the same "nothing to append" case
// store.AppendJobInstance reports via errs.ErrInstanceExists, so this
// reuses that error instead of inventing a new one.
func lastInstanceFromPayload(payload map[string]any) (model.JobInstance, error) {
	list, _ := payload["instance_list"].([]any)
	if len(list) == 0 {
		return model.JobInstance{}, errs.ErrInstanceExists
	}
	last, ok := list[len(list)-1].(map[string]any)
	if !ok {
		return model.JobInstance{}, errs.ErrInstanceExists
	}
	return fromMap[model.JobInstance](last)
}
