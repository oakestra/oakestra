package store

import (
	"context"
	"maps"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

const (
	// candidatesFreshnessInterval: a candidate counts as "active" if it
	// reported within this window.
	candidatesFreshnessInterval = 30 * time.Second

	// historySliceSize caps cpu/memory history arrays at their most recent
	// 100 entries; $slice keeps the last N when the size is negative. Also
	// used by jobs.go's UpdateJobInstance for instance history.
	historySliceSize = -100
)

// canonicalResourceFields are the fields always projected by FindCandidates.
var canonicalResourceFields = []string{
	"_id", "cpu_percent", "vcpus", "memory_percent", "vram", "vram_percent",
	"gpu_temp", "gpu_drivers", "gpu_percent", "vgpus", "memory", "virtualization",
	"supported_addons", "active_nodes", "ip", "port", "candidate_location",
	"candidate_name", "cpu_history", "memory_history", "csi_drivers",
}

// canonicalProjection is the $project stage built once from
// canonicalResourceFields; FindCandidates merges in any per-request extras
// rather than rebuilding it from scratch on every call.
var canonicalProjection = func() bson.M {
	p := make(bson.M, len(canonicalResourceFields))
	for _, f := range canonicalResourceFields {
		p[f] = 1
	}
	return p
}()

// FreshnessThreshold returns the last_modified_timestamp cutoff, in unix
// seconds, below which a candidate is considered stale.
func FreshnessThreshold() float64 {
	return unixSeconds(time.Now()) - candidatesFreshnessInterval.Seconds()
}

func unixSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second)
}

// CandidateFilter holds the query params accepted by GET /resources/, after
// the handler has coerced them out of their raw query-string form. Empty
// string fields mean "parameter not supplied" and add no constraint.
type CandidateFilter struct {
	CandidateName string
	IP            string
	// CandidateID is resolved upstream from ?job_id=, see ResolveJobCandidate.
	CandidateID string
	// ActiveOnly comes from ?active=, already parsed by the handler.
	ActiveOnly bool
}

// BuildCandidateFilter translates a CandidateFilter into a MongoDB filter:
//   - ActiveOnly adds a last_modified_timestamp freshness constraint
//   - CandidateID becomes an _id match
//   - job_id, active and candidate_id never appear as literal filter fields
func BuildCandidateFilter(query CandidateFilter) (bson.M, error) {
	filter := bson.M{}

	if query.CandidateName != "" {
		filter["candidate_name"] = query.CandidateName
	}
	if query.IP != "" {
		filter["ip"] = query.IP
	}
	if query.ActiveOnly {
		filter["last_modified_timestamp"] = bson.M{"$gt": FreshnessThreshold()}
	}
	if query.CandidateID != "" {
		oid, err := parseObjectID(query.CandidateID)
		if err != nil {
			return nil, err
		}
		filter["_id"] = oid
	}

	return filter, nil
}

// FindCandidates runs the aggregation pipeline behind GET /resources/:
// $match the filter, $addFields the computed `active` freshness flag, then
// $project the canonical fields plus any extras requested via
// ?resources=a,b,c.
func (s *Store) FindCandidates(ctx context.Context, filter bson.M, resources []string) ([]model.Resource, error) {
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

	return aggregateAll[model.Resource](ctx, s.candidates, pipeline)
}

// FindCandidateByID looks up one candidate through the same aggregation
// pipeline as FindCandidates, so it gets the computed `active` field and
// canonical projection. Returns errs.ErrNotFound if none matches.
func (s *Store) FindCandidateByID(ctx context.Context, id string) (model.Resource, error) {
	var zero model.Resource

	oid, err := parseObjectID(id)
	if err != nil {
		return zero, err
	}

	results, err := s.FindCandidates(ctx, bson.M{"_id": oid}, nil)
	if err != nil {
		return zero, err
	}
	if len(results) == 0 {
		return zero, errs.ErrNotFound
	}
	return results[0], nil
}

// FindCandidateByName looks up a candidate by its unique-in-practice
// candidate_name field, used by the upsert-by-name write path.
func (s *Store) FindCandidateByName(ctx context.Context, name string) (model.Resource, error) {
	return findOne[model.Resource](ctx, s.candidates, bson.M{"candidate_name": name})
}

// CreateCandidate inserts a new candidate document and returns it as stored.
func (s *Store) CreateCandidate(ctx context.Context, data model.Resource) (model.Resource, error) {
	return insertReturning(ctx, s.candidates, data)
}

// UpdateCandidate applies a plain $set update, dropping any client-supplied
// _id, and returns the updated document.
func (s *Store) UpdateCandidate(ctx context.Context, id string, data model.Resource) (model.Resource, error) {
	return updateByID(ctx, s.candidates, id, data)
}

// UpdateCandidateInformation persists an aggregated resource-usage report
// from a worker/cluster: timestamps are refreshed, cpu/memory history
// entries are appended (capped at the most recent 100 via $slice), and the
// rest of the payload is $set.
//
// report's CPUHistory/MemoryHistory are always dropped instead of $set.
// They're readOnly per openapi.yaml and never decoded from a request body,
// but dropping them here too guards against a caller assembling a report by
// hand.
func (s *Store) UpdateCandidateInformation(ctx context.Context, id string, report model.Resource) (model.Resource, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return model.Resource{}, err
	}

	setDoc, err := toSetDoc(report)
	if err != nil {
		return model.Resource{}, err
	}
	delete(setDoc, "cpu_history")
	delete(setDoc, "memory_history")

	now := time.Now().UTC()
	nowTimestamp := unixSeconds(now)
	setDoc["last_modified"] = now
	setDoc["last_modified_timestamp"] = nowTimestamp

	cpuUpdate := bson.M{"value": setDoc["cpu_percent"], "timestamp": nowTimestamp}
	memUpdate := bson.M{"value": setDoc["memory_percent"], "timestamp": nowTimestamp}

	update := bson.M{
		"$push": bson.M{
			"cpu_history":    bson.M{"$each": bson.A{cpuUpdate}, "$slice": historySliceSize},
			"memory_history": bson.M{"$each": bson.A{memUpdate}, "$slice": historySliceSize},
		},
		"$set": setDoc,
	}

	return findOneAndUpdate[model.Resource](ctx, s.candidates, bson.M{"_id": oid}, update)
}

// DeleteCandidate removes a candidate document by id.
func (s *Store) DeleteCandidate(ctx context.Context, id string) error {
	return deleteByID(ctx, s.candidates, id)
}
