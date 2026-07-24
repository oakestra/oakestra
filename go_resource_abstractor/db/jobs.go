package db

import (
	"context"
	"errors"
	"maps"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrInstanceConflict is returned by AppendJobInstance when the instance
// already exists, the supplied instance_list is empty, or the parent job
// does not exist. Mirrors jobs_db.append_job_instance returning None in all
// three cases; the API layer surfaces it as 400 "Instance already exists"
// to match the Python blueprint's (slightly misleading, but preserved)
// behavior.
var ErrInstanceConflict = errors.New("job instance already exists or payload has no instance to append")

// Application operations #####################################################

// FindApps lists applications matching filter.
func (s *Store) FindApps(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.Apps, filter)
}

// FindAppByID looks up a single application by id, additionally constrained
// by extraFilter (e.g. userId from query params).
func (s *Store) FindAppByID(ctx context.Context, id string, extraFilter bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{}
	maps.Copy(filter, extraFilter)
	filter["_id"] = oid

	var result bson.M
	if err := s.Apps.FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteApp removes an application by id and returns the deleted document.
func (s *Store) DeleteApp(ctx context.Context, id string) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var deleted bson.M
	if err := s.Apps.FindOneAndDelete(ctx, bson.M{"_id": oid}).Decode(&deleted); err != nil {
		return nil, err
	}
	return deleted, nil
}

// UpdateApp applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateApp(ctx context.Context, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.Apps, id, data)
}

// CreateApp inserts a new application, populating applicationID with its own
// stringified _id. The id is generated client-side so both fields can be set
// in a single insert, rather than jobs_db.create_app's insert-then-update.
func (s *Store) CreateApp(ctx context.Context, data bson.M) (bson.M, error) {
	delete(data, "_id")
	id := bson.NewObjectID()
	data["_id"] = id
	data["applicationID"] = id.Hex()

	if _, err := s.Apps.InsertOne(ctx, data); err != nil {
		return nil, err
	}
	return data, nil
}

// Job operations ##############################################################

// BuildJobFilter translates the query params accepted by GET /jobs/<id>
// into a MongoDB filter, mirroring jobs_helper.build_filter: an
// instance_number query param narrows the match to jobs whose instance_list
// contains that instance.
func BuildJobFilter(query map[string]any) bson.M {
	filter := bson.M{}
	maps.Copy(filter, query)
	if instanceNumber, ok := filter["instance_number"]; ok {
		filter["instance_list"] = bson.M{"$elemMatch": bson.M{"instance_number": instanceNumber}}
	}
	return filter
}

// FindJobs lists jobs matching filter.
func (s *Store) FindJobs(ctx context.Context, filter bson.M) ([]bson.M, error) {
	return findAll(ctx, s.Jobs, filter)
}

// FindJobByID looks up a single job by id, additionally constrained by
// filter (e.g. the instance_list $elemMatch built by BuildJobFilter).
func (s *Store) FindJobByID(ctx context.Context, id string, filter bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	f := bson.M{}
	maps.Copy(f, filter)
	f["_id"] = oid

	var job bson.M
	if err := s.Jobs.FindOne(ctx, f).Decode(&job); err != nil {
		return nil, err
	}
	return job, nil
}

// FindJobByName looks up a job by its job_name field, used by the PUT
// /jobs/ upsert path.
func (s *Store) FindJobByName(ctx context.Context, name string) (bson.M, error) {
	var job bson.M
	if err := s.Jobs.FindOne(ctx, bson.M{"job_name": name}).Decode(&job); err != nil {
		return nil, err
	}
	return job, nil
}

// ResolveJobCandidate looks up jobID and returns the candidate id stored in
// its "candidate" field, translating the ?job_id= filter on GET /resources/
// into a ?candidate_id= filter (see BuildCandidateFilter). Returns
// ErrNotFound if the job doesn't exist or has no candidate assigned yet.
func (s *Store) ResolveJobCandidate(ctx context.Context, jobID string) (string, error) {
	job, err := s.FindJobByID(ctx, jobID, bson.M{})
	if err != nil {
		return "", err
	}

	candidateID, ok := job["candidate"].(string)
	if !ok || candidateID == "" {
		return "", ErrNotFound
	}
	return candidateID, nil
}

// DeleteJob removes a job by id and returns the deleted document.
func (s *Store) DeleteJob(ctx context.Context, id string) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var deleted bson.M
	if err := s.Jobs.FindOneAndDelete(ctx, bson.M{"_id": oid}).Decode(&deleted); err != nil {
		return nil, err
	}
	return deleted, nil
}

// UpdateJob applies a plain $set update, dropping any client-supplied _id.
func (s *Store) UpdateJob(ctx context.Context, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.Jobs, id, data)
}

// CreateJob inserts a new job document and returns it as stored.
func (s *Store) CreateJob(ctx context.Context, data bson.M) (bson.M, error) {
	return insertReturning(ctx, s.Jobs, data)
}

// FindJobInstance returns the job document with instance_list filtered down
// to just the matching instance_number, mirroring jobs_db.find_job_instance.
// Returns ErrNotFound if the job doesn't exist or has no such instance.
func (s *Store) FindJobInstance(ctx context.Context, jobID string, instanceNumber int) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(jobID)
	if err != nil {
		return nil, err
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

	cursor, err := s.Jobs.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}

	var result bson.M
	if err := cursor.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// AppendJobInstance pushes the last element of jobData's instance_list onto
// the job's instance_list, refusing (ErrInstanceConflict) if the instance
// already exists, the payload has none, or the job itself doesn't exist.
// Mirrors jobs_db.append_job_instance.
func (s *Store) AppendJobInstance(ctx context.Context, jobID string, instanceNumber int, jobData bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(jobID)
	if err != nil {
		return nil, err
	}

	if _, err := s.FindJobInstance(ctx, jobID, instanceNumber); err == nil {
		return nil, ErrInstanceConflict
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	instanceList := asSlice(jobData["instance_list"])
	if len(instanceList) == 0 {
		return nil, ErrInstanceConflict
	}
	instanceInfo := instanceList[len(instanceList)-1]

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated bson.M
	err = s.Jobs.FindOneAndUpdate(ctx, bson.M{"_id": oid},
		bson.M{"$push": bson.M{"instance_list": instanceInfo}}, opts).Decode(&updated)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrInstanceConflict
		}
		return nil, err
	}
	return updated, nil
}

// asSlice normalizes a JSON- or BSON-decoded array value (typically
// []any from encoding/json, or bson.A from the driver) into a plain []any.
func asSlice(v any) []any {
	switch val := v.(type) {
	case []any:
		return val
	case bson.A:
		return []any(val)
	default:
		return nil
	}
}

// UpdateJobInstance applies a positional update to the matching instance:
// pushes new cpu/memory history samples (capped at the most recent 100),
// and $sets the scalar status fields. Returns ErrNotFound if no instance
// matches. Mirrors jobs_db.update_job_instance.
func (s *Store) UpdateJobInstance(ctx context.Context, jobID string, instanceNumber int, jobData bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(jobID)
	if err != nil {
		return nil, err
	}
	delete(jobData, "_id")

	now := isoFormatUTC(time.Now())
	cpuUpdate := bson.M{"value": jobData["cpu_percent"], "timestamp": now}
	memUpdate := bson.M{"value": jobData["memory_percent"], "timestamp": now}

	statusDetail := jobData["status_detail"]
	if statusDetail == nil {
		statusDetail = "No extra information"
	}
	logs := jobData["logs"]
	if logs == nil {
		logs = ""
	}

	filter := bson.M{
		"_id":           oid,
		"instance_list": bson.M{"$elemMatch": bson.M{"instance_number": instanceNumber}},
	}
	update := bson.M{
		"$push": bson.M{
			"instance_list.$.cpu_history":    bson.M{"$each": bson.A{cpuUpdate}, "$slice": historySliceSize},
			"instance_list.$.memory_history": bson.M{"$each": bson.A{memUpdate}, "$slice": historySliceSize},
		},
		"$set": bson.M{
			"instance_list.$.cpu_percent":             jobData["cpu_percent"],
			"instance_list.$.memory_percent":          jobData["memory_percent"],
			"instance_list.$.publicip":                jobData["publicip"],
			"instance_list.$.disk":                    jobData["disk"],
			"instance_list.$.status":                  jobData["status"],
			"instance_list.$.status_detail":           statusDetail,
			"instance_list.$.logs":                    logs,
			"instance_list.$.worker_id":               jobData["worker_id"],
			"instance_list.$.host_ip":                 jobData["host_ip"],
			"instance_list.$.host_port":               jobData["host_port"],
			"instance_list.$.last_modified_timestamp": jobData["last_modified_timestamp"],
		},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated bson.M
	err = s.Jobs.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updated)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteJobInstance removes the instance with the given instance_number
// from the job's instance_list via $pull. Returns ErrNotFound only if the
// job itself doesn't exist (a no-op pull on an already-absent instance
// still returns the job, matching jobs_db.delete_job_instance).
func (s *Store) DeleteJobInstance(ctx context.Context, jobID string, instanceNumber int) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(jobID)
	if err != nil {
		return nil, err
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated bson.M
	err = s.Jobs.FindOneAndUpdate(ctx, bson.M{"_id": oid},
		bson.M{"$pull": bson.M{"instance_list": bson.M{"instance_number": instanceNumber}}}, opts).Decode(&updated)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// isoFormatUTC approximates Python's datetime.now(timezone.utc).isoformat(),
// e.g. "2024-01-01T12:00:00.123456+00:00", for the job-instance history
// timestamps (unlike candidate history, which stores a float timestamp).
func isoFormatUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000") + "+00:00"
}
