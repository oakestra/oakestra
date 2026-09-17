package abstractor

import (
	"context"
	"errors"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// ResourceFilter narrows List to candidates matching the given fields; a
// zero field imposes no constraint. JobID is resolved to the job's assigned
// candidate id via the store's ResolveJobCandidate before the query runs;
// ExtraFields adds fields beyond the canonical projection (the
// ?resources=a,b,c query parameter).
type ResourceFilter struct {
	Name        string
	IP          string
	JobID       string
	ActiveOnly  bool
	ExtraFields []string
}

// Resources is the candidate-resource sub-service: create/read/update/delete
// for candidates (a worker node at cluster level, a whole cluster at root
// level), with pre/post webhooks fired under the "resources" entity around
// every write.
type Resources struct {
	store  *store.Store
	entity entityHooks
}

// List returns candidates matching filter.
func (r *Resources) List(ctx context.Context, filter ResourceFilter) ([]model.Resource, error) {
	candidateID := ""
	if filter.JobID != "" {
		resolved, err := r.store.ResolveJobCandidate(ctx, filter.JobID)
		if err != nil {
			return nil, err
		}
		candidateID = resolved
	}

	mongoFilter, err := store.BuildCandidateFilter(store.CandidateFilter{
		CandidateName: filter.Name,
		IP:            filter.IP,
		CandidateID:   candidateID,
		ActiveOnly:    filter.ActiveOnly,
	})
	if err != nil {
		return nil, err
	}
	return r.store.FindCandidates(ctx, mongoFilter, filter.ExtraFields)
}

// Get returns a single candidate by id.
func (r *Resources) Get(ctx context.Context, id string) (model.Resource, error) {
	return r.store.FindCandidateByID(ctx, id)
}

// Create runs pre_create/post_create around inserting a new candidate.
func (r *Resources) Create(ctx context.Context, data model.Resource) (model.Resource, error) {
	return create(ctx, r.entity, data, r.store.CreateCandidate)
}

// Upsert updates the candidate named data.CandidateName if one exists, else
// creates it.
//
// candidate_name has no unique index, for the same reasons as Jobs.Upsert's
// job_name: the lookup and the create/update aren't atomic, and legacy
// duplicate names would break a unique index anyway.
func (r *Resources) Upsert(ctx context.Context, data model.Resource) (model.Resource, error) {
	if data.CandidateName != nil && *data.CandidateName != "" {
		existing, err := r.store.FindCandidateByName(ctx, *data.CandidateName)
		switch {
		case err == nil:
			// Not Update: pre_update isn't told which document matched by
			// name, so data["_id"] stays unset.
			return update(ctx, r.entity, *existing.ID, data, false, func(ctx context.Context, d model.Resource) (model.Resource, error) {
				return r.store.UpdateCandidate(ctx, *existing.ID, d)
			})
		case !errors.Is(err, errs.ErrNotFound):
			return model.Resource{}, err
		}
	}
	return r.Create(ctx, data)
}

// Report runs pre_update/post_update around persisting an aggregated
// resource-usage report from a worker/cluster. The pre_update hook sees
// data["_id"] = id, like Update elsewhere.
func (r *Resources) Report(ctx context.Context, id string, report model.Resource) (model.Resource, error) {
	return update(ctx, r.entity, id, report, true, func(ctx context.Context, d model.Resource) (model.Resource, error) {
		return r.store.UpdateCandidateInformation(ctx, id, d)
	})
}

// Delete runs post_delete around removing a candidate by id. Unlike the
// other entities' Delete, there's no document to return:
// store.DeleteCandidate reports only success or failure.
func (r *Resources) Delete(ctx context.Context, id string) error {
	if err := r.store.DeleteCandidate(ctx, id); err != nil {
		return err
	}
	r.entity.dispatcher.PostDelete(r.entity.name, id)
	return nil
}
