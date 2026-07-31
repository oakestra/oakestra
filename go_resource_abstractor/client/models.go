package client

import "github.com/oakestra/oakestra/go_resource_abstractor/client/openapi"

// The document and parameter types this package's methods take and return.
//
// They are aliases, not definitions: client.Job and openapi.Job are the same
// type, so nothing converts between the two layers and a value obtained from
// Client.OpenAPI can be handed straight to a method here. That also means the
// spec stays the single source of truth for what a document contains - a
// field added to openapi.yaml appears here the moment the code is
// regenerated, with nothing to keep in step by hand.
//
// Aliased here are the types this package's own API is written in - nothing
// else, so the list stays short enough to read. Hooks and custom resources
// have no facade (see Client.OpenAPI), so their types are named through the
// openapi package.
type (
	// Application is application metadata.
	Application = openapi.Application

	// Resource is a candidate resource record - a worker node at cluster
	// level, a whole cluster at root level.
	Resource = openapi.Resource

	// HistorySample is one entry of a Resource's cpu_history or
	// memory_history. Its Timestamp is Unix seconds.
	HistorySample = openapi.HistorySample

	// Job is a deployed microservice and its per-worker instances.
	Job = openapi.Job

	// JobInstance is one running copy of a job on one worker.
	JobInstance = openapi.JobInstance

	// JobHistorySample is one entry of a JobInstance's cpu_history or
	// memory_history. Unlike HistorySample its Timestamp is an ISO-8601
	// string, not Unix seconds - the two aren't interchangeable.
	JobHistorySample = openapi.JobHistorySample

	// JobInstanceAppend is the body of JobsClient.AppendInstance. Only the
	// last element of its InstanceList is appended.
	JobInstanceAppend = openapi.JobInstanceAppend

	// ListApplicationsParams is the query the AppFilter options build.
	ListApplicationsParams = openapi.ListApplicationsParams

	// GetApplicationParams is the query AppsClient.GetByID scopes its lookup
	// with.
	GetApplicationParams = openapi.GetApplicationParams

	// ListResourcesParams is the query the ResourceFilter options build.
	ListResourcesParams = openapi.ListResourcesParams

	// ListJobsParams is the query the JobFilter options build.
	ListJobsParams = openapi.ListJobsParams
)

// Ptr returns a pointer to v, for populating the optional fields of a
// document being sent:
//
//	job, err := c.Jobs.Create(ctx, client.Job{JobName: client.Ptr("my-job")})
//
// Every field is optional because the resource abstractor stores partial
// documents and patches them field by field, so none of them can be declared
// required in the spec.
func Ptr[T any](v T) *T { return &v }

// Value dereferences an optional field of a document that was read back,
// yielding the zero value when the service didn't set it:
//
//	name := client.Value(job.JobName)
//
// Reading *job.JobName directly is a panic waiting for the first document
// that omits the field - which, on a service that stores whatever it is
// given, is any of them.
func Value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
