package db

import (
	"context"
	"maps"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrNotFound is returned by lookups that find no matching document. It is
// an alias for mongo.ErrNoDocuments so callers can use errors.Is against
// either name.
var ErrNotFound = mongo.ErrNoDocuments

const (
	// candidatesFreshnessInterval mirrors candidates_helper.py's
	// CANDIDATES_FRESHNESS_INTERVAL: a candidate is "active" if it reported
	// within the last 30 seconds.
	candidatesFreshnessInterval = 30 * time.Second

	// historySliceSize caps cpu/memory history arrays at their most recent
	// 100 entries, matching candidates_db.py's HISTORY_SLICE_SIZE.
	historySliceSize = -100
)

// canonicalResourceFields are the fields always projected by FindCandidates,
// matching CANONICAL_RESOURCES in candidates_db.py.
var canonicalResourceFields = []string{
	"_id", "cpu_percent", "vcpus", "memory_percent", "vram", "vram_percent",
	"gpu_temp", "gpu_drivers", "gpu_percent", "vgpus", "memory", "virtualization",
	"supported_addons", "active_nodes", "ip", "port", "candidate_location",
	"candidate_name", "cpu_history", "memory_history", "csi_drivers",
}

// canonicalProjection is the $project stage built once from
// canonicalResourceFields, merged with any per-request extras in
// FindCandidates rather than rebuilt from scratch on every call.
var canonicalProjection = func() bson.M {
	p := make(bson.M, len(canonicalResourceFields))
	for _, f := range canonicalResourceFields {
		p[f] = 1
	}
	return p
}()

// FreshnessThreshold returns the last_modified_timestamp cutoff (unix
// seconds, matching Python's datetime.timestamp() float) below which a
// candidate is considered stale. Mirrors
// candidates_helper.get_freshness_threshold.
func FreshnessThreshold() float64 {
	return unixSeconds(time.Now()) - candidatesFreshnessInterval.Seconds()
}

func unixSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second)
}

// BuildCandidateFilter translates the query params accepted by GET
// /resources/ into a MongoDB filter, mirroring
// candidates_helper.build_filter:
//   - active=true adds a last_modified_timestamp freshness constraint
//   - candidate_id (resolved upstream from job_id, see ResolveJobCandidate)
//     becomes an _id match
//   - job_id/active/candidate_id keys never appear as literal filter fields
func BuildCandidateFilter(query map[string]any) (bson.M, error) {
	filter := bson.M{}
	maps.Copy(filter, query)

	if active, ok := filter["active"]; ok && truthy(active) {
		filter["last_modified_timestamp"] = bson.M{"$gt": FreshnessThreshold()}
	}

	if candidateID, ok := filter["candidate_id"]; ok {
		idStr, _ := candidateID.(string)
		oid, err := bson.ObjectIDFromHex(idStr)
		if err != nil {
			return nil, err
		}
		filter["_id"] = oid
	}

	delete(filter, "candidate_id")
	delete(filter, "job_id")
	delete(filter, "active")

	return filter, nil
}

func truthy(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != "" && val != "false" && val != "0"
	default:
		return v != nil
	}
}

// FindCandidates runs the aggregation pipeline behind GET /resources/:
// $match the filter, $addFields the computed `active` freshness flag, then
// $project the canonical field set plus any extra fields requested via
// ?resources=a,b,c. Mirrors candidates_db.find_candidates.
func (s *Store) FindCandidates(ctx context.Context, filter bson.M, resources []string) ([]bson.M, error) {
	projection := make(bson.M, len(canonicalProjection)+len(resources))
	maps.Copy(projection, canonicalProjection)
	for _, f := range resources {
		if f = strings.TrimSpace(f); f != "" {
			projection[f] = 1
		}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$addFields", Value: bson.M{
			"active": bson.M{"$gt": bson.A{"$last_modified_timestamp", FreshnessThreshold()}},
		}}},
		{{Key: "$project", Value: projection}},
	}

	cursor, err := s.Candidates.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FindCandidateByID looks up a single candidate through the same
// aggregation pipeline as FindCandidates (so it gets the computed `active`
// field and canonical projection), returning ErrNotFound if none matches.
func (s *Store) FindCandidateByID(ctx context.Context, id string) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	results, err := s.FindCandidates(ctx, bson.M{"_id": oid}, nil)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}
	return results[0], nil
}

// FindCandidateByName looks up a candidate by its unique-in-practice
// candidate_name field, used by the PUT /resources/ upsert path.
func (s *Store) FindCandidateByName(ctx context.Context, name string) (bson.M, error) {
	var result bson.M
	if err := s.Candidates.FindOne(ctx, bson.M{"candidate_name": name}).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateCandidate inserts a new candidate document and returns it as stored.
func (s *Store) CreateCandidate(ctx context.Context, data bson.M) (bson.M, error) {
	return insertReturning(ctx, s.Candidates, data)
}

// UpdateCandidate applies a plain $set update, dropping any client-supplied
// _id, and returns the updated document.
func (s *Store) UpdateCandidate(ctx context.Context, id string, data bson.M) (bson.M, error) {
	return updateByID(ctx, s.Candidates, id, data)
}

// UpdateCandidateInformation persists an aggregated resource-usage report
// from a worker/cluster. Mirrors candidates_db.update_candidate_information:
// timestamps are refreshed, cpu/memory history entries are appended (capped
// at the most recent 100 via $slice), and the rest of the payload is $set.
func (s *Store) UpdateCandidateInformation(ctx context.Context, id string, data bson.M) (bson.M, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	delete(data, "_id")

	now := time.Now().UTC()
	nowTimestamp := unixSeconds(now)

	data["last_modified"] = now
	data["last_modified_timestamp"] = nowTimestamp
	// Incoming history arrays are ignored; only the single new sample below
	// is appended, to avoid duplicating history the caller may have echoed
	// back to us.
	delete(data, "cpu_history")
	delete(data, "memory_history")

	cpuUpdate := bson.M{"value": data["cpu_percent"], "timestamp": nowTimestamp}
	memUpdate := bson.M{"value": data["memory_percent"], "timestamp": nowTimestamp}

	update := bson.M{
		"$push": bson.M{
			"cpu_history":    bson.M{"$each": bson.A{cpuUpdate}, "$slice": historySliceSize},
			"memory_history": bson.M{"$each": bson.A{memUpdate}, "$slice": historySliceSize},
		},
		"$set": data,
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated bson.M
	err = s.Candidates.FindOneAndUpdate(ctx, bson.M{"_id": oid}, update, opts).Decode(&updated)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteCandidate removes a candidate document by id.
func (s *Store) DeleteCandidate(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.Candidates.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}
