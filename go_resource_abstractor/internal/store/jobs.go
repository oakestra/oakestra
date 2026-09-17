package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// BuildJobFilter translates the query params accepted by GET /jobs/<id>
// into a MongoDB filter: a non-nil instanceNumber (from ?instance_number=)
// narrows the match to jobs whose instance_list contains that instance.
//
// Python's jobs_helper.build_filter builds the same $elemMatch but also
// leaves a stray top-level instance_number key in the filter, which no job
// document has, so every such lookup 404'd there. Dropped here so the
// filter actually matches.
func BuildJobFilter(instanceNumber *int) bson.M {
	filter := bson.M{}
	if instanceNumber != nil {
		filter["instance_list"] = bson.M{"$elemMatch": bson.M{"instance_number": *instanceNumber}}
	}
	return filter
}

// FindJobs lists jobs matching filter.
func (s *Store) FindJobs(ctx context.Context, filter bson.M) ([]model.Job, error) {
	return findAll[model.Job](ctx, s.jobs, filter)
}

// FindJobByID looks up a single job by id, additionally constrained by
// filter (e.g. the instance_list $elemMatch built by BuildJobFilter).
func (s *Store) FindJobByID(ctx context.Context, id string, filter bson.M) (model.Job, error) {
	return findByID[model.Job](ctx, s.jobs, id, filter)
}

// FindJobByName looks up a job by its job_name field, used by the
// upsert-by-name write path.
func (s *Store) FindJobByName(ctx context.Context, name string) (model.Job, error) {
	return findOne[model.Job](ctx, s.jobs, bson.M{"job_name": name})
}

// ResolveJobCandidate looks up jobID and returns the candidate id stored in
// its "candidate" field, so the ?job_id= filter on GET /resources/ can be
// translated into a ?candidate_id= filter. Returns errs.ErrNotFound if the
// job doesn't exist or has no candidate assigned yet.
func (s *Store) ResolveJobCandidate(ctx context.Context, jobID string) (string, error) {
	job, err := s.FindJobByID(ctx, jobID, bson.M{})
	if err != nil {
		return "", err
	}

	if job.Candidate == nil || *job.Candidate == "" {
		return "", errs.ErrNotFound
	}
	return *job.Candidate, nil
}

// DeleteJob removes a job by id and returns the deleted document.
func (s *Store) DeleteJob(ctx context.Context, id string) (model.Job, error) {
	return deleteByIDReturning[model.Job](ctx, s.jobs, id)
}

// UpdateJob applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateJob(ctx context.Context, id string, data model.Job) (model.Job, error) {
	return updateByID(ctx, s.jobs, id, data)
}

// CreateJob inserts a new job document and returns it as stored.
func (s *Store) CreateJob(ctx context.Context, data model.Job) (model.Job, error) {
	return insertReturning(ctx, s.jobs, data)
}

// FindJobInstance returns the job document with instance_list filtered
// down to just the matching instance_number. Returns errs.ErrNotFound if
// the job doesn't exist or has no such instance.
func (s *Store) FindJobInstance(ctx context.Context, jobID string, instanceNumber int) (model.Job, error) {
	var zero model.Job

	oid, err := parseObjectID(jobID)
	if err != nil {
		return zero, err
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"_id":                           oid,
			"instance_list.instance_number": instanceNumber,
		}}},
		{{Key: "$set", Value: bson.M{
			"instance_list": bson.M{"$filter": bson.M{
				"input": "$instance_list",
				"as":    "item",
				"cond":  bson.M{"$eq": bson.A{"$$item.instance_number", instanceNumber}},
			}},
		}}},
	}

	results, err := aggregateAll[model.Job](ctx, s.jobs, pipeline)
	if err != nil {
		return zero, err
	}
	if len(results) == 0 {
		return zero, errs.ErrNotFound
	}
	return results[0], nil
}

// AppendJobInstance pushes instance onto the job's instance_list. It
// refuses with errs.ErrInstanceExists if instanceNumber already has an
// entry on the job, or the job itself doesn't exist.
//
// It takes the single instance to append rather than a raw request body:
// resolving which element of an incoming instance_list array to append is
// the caller's job.
//
// The check and the push happen in one atomic FindOneAndUpdate: the filter
// only matches a job with no instance_list entry for this instance_number
// yet, so two concurrent requests can't both pass. The loser matches
// nothing, and mongo.ErrNoDocuments maps to errs.ErrInstanceExists below.
func (s *Store) AppendJobInstance(ctx context.Context, jobID string, instanceNumber int, instance model.JobInstance) (model.Job, error) {
	oid, err := parseObjectID(jobID)
	if err != nil {
		return model.Job{}, err
	}

	filter := bson.M{
		"_id":                           oid,
		"instance_list.instance_number": bson.M{"$ne": instanceNumber},
	}
	updated, err := findOneAndUpdate[model.Job](ctx, s.jobs, filter,
		bson.M{"$push": bson.M{"instance_list": instance}})
	if errors.Is(err, errs.ErrNotFound) {
		return model.Job{}, errs.ErrInstanceExists
	}
	return updated, err
}

// UpdateJobInstance applies a positional update to the matching instance:
// pushes new cpu/memory history samples (capped at the most recent 100)
// and $sets the scalar status fields from update. Returns errs.ErrNotFound
// if no instance matches.
//
// Every $set target is always written, even when the corresponding field in
// update is nil: a worker's status report is a full replacement of the
// instance's live fields, not a sparse patch, so a nil field means "clear
// it" (e.g. no publicip once a job stops forwarding a port), not "leave
// it". status_detail and logs are the exceptions - they default to
// placeholder values instead of null.
func (s *Store) UpdateJobInstance(ctx context.Context, jobID string, instanceNumber int, update model.JobInstance) (model.Job, error) {
	oid, err := parseObjectID(jobID)
	if err != nil {
		return model.Job{}, err
	}

	now := isoFormatUTC(time.Now())
	// bson.M encodes a nil pointer as BSON null, which is what "clear it"
	// means here.
	cpuUpdate := bson.M{"value": update.CPUPercent, "timestamp": now}
	memUpdate := bson.M{"value": update.MemoryPercent, "timestamp": now}

	statusDetail := "No extra information"
	if update.StatusDetail != nil {
		statusDetail = *update.StatusDetail
	}
	logs := ""
	if update.Logs != nil {
		logs = *update.Logs
	}

	filter := bson.M{
		"_id":           oid,
		"instance_list": bson.M{"$elemMatch": bson.M{"instance_number": instanceNumber}},
	}
	mongoUpdate := bson.M{
		"$push": bson.M{
			"instance_list.$.cpu_history":    bson.M{"$each": bson.A{cpuUpdate}, "$slice": historySliceSize},
			"instance_list.$.memory_history": bson.M{"$each": bson.A{memUpdate}, "$slice": historySliceSize},
		},
		"$set": bson.M{
			"instance_list.$.cpu_percent":             update.CPUPercent,
			"instance_list.$.memory_percent":          update.MemoryPercent,
			"instance_list.$.publicip":                update.PublicIP,
			"instance_list.$.disk":                    update.Disk,
			"instance_list.$.status":                  update.Status,
			"instance_list.$.status_detail":           statusDetail,
			"instance_list.$.logs":                    logs,
			"instance_list.$.worker_id":               update.WorkerID,
			"instance_list.$.host_ip":                 update.HostIP,
			"instance_list.$.host_port":               update.HostPort,
			"instance_list.$.last_modified_timestamp": update.LastModifiedTimestamp,
		},
	}

	return findOneAndUpdate[model.Job](ctx, s.jobs, filter, mongoUpdate)
}

// DeleteJobInstance removes the instance with the given instance_number
// from the job's instance_list via $pull. Returns errs.ErrNotFound only if
// the job itself doesn't exist; pulling an already-absent instance is a
// no-op that still returns the job.
func (s *Store) DeleteJobInstance(ctx context.Context, jobID string, instanceNumber int) (model.Job, error) {
	oid, err := parseObjectID(jobID)
	if err != nil {
		return model.Job{}, err
	}

	return findOneAndUpdate[model.Job](ctx, s.jobs, bson.M{"_id": oid},
		bson.M{"$pull": bson.M{"instance_list": bson.M{"instance_number": instanceNumber}}})
}

// isoFormatUTC formats a timestamp for job-instance history the way
// Python's datetime.now(timezone.utc).isoformat() does, e.g.
// "2024-01-01T12:00:00.123456+00:00". Candidate history stores a float
// timestamp instead.
func isoFormatUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000") + "+00:00"
}
