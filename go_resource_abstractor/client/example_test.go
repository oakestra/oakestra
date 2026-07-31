package client

import (
	"context"
	"errors"
	"fmt"
)

// ExampleNewFromEnv builds a Client from RESOURCE_ABSTRACTOR_URL and
// RESOURCE_ABSTRACTOR_PORT, then lists the active candidates.
func ExampleNewFromEnv() {
	c, err := NewFromEnv()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	candidates, err := c.Resources.List(ctx, Active())
	if err != nil {
		panic(err)
	}
	for _, candidate := range candidates {
		fmt.Println(Value(candidate.CandidateName))
	}
}

// ExampleClient_OpenAPI reaches an endpoint the Apps/Resources/Jobs facades
// don't wrap, by going through the generated client underneath.
func ExampleClient_OpenAPI() {
	c := New("http://root_resource_abstractor:11011")

	resp, err := c.OpenAPI().ListHooksWithResponse(context.Background())
	if err != nil {
		panic(err)
	}
	if resp.StatusCode() != 200 {
		panic(fmt.Sprintf("unexpected status: %s", resp.Status()))
	}
	for _, hook := range *resp.JSON200 {
		fmt.Println(Value(hook.HookName))
	}
}

// Example_errorHandling shows the three-way split every method's error
// falls into: ErrNotFound, *APIError, or anything else (a transport
// failure, or a 2xx body that wasn't the JSON the spec promises).
func Example_errorHandling() {
	c := New("http://cluster_resource_abstractor:11012")

	job, err := c.Jobs.GetByID(context.Background(), "507f1f77bcf86cd799439011")
	var apiErr *APIError
	switch {
	case errors.Is(err, ErrNotFound):
		fmt.Println("no such job")
	case errors.As(err, &apiErr):
		fmt.Printf("resource abstractor returned %d: %s\n", apiErr.Status, apiErr.Message)
	case err != nil:
		fmt.Println("transport error:", err)
	default:
		fmt.Println(Value(job.JobName))
	}
}

// ExampleJobsService_List shows filters narrowing List. Passing none of them
// lists every job.
func ExampleJobsService_List() {
	c := New("http://cluster_resource_abstractor:11012")
	ctx := context.Background()

	jobs, err := c.Jobs.List(ctx, OfApplication("app-1"), NamedJob("nginx"))
	if err != nil {
		panic(err)
	}
	for _, job := range jobs {
		fmt.Println(Value(job.JobName))
	}

	all, err := c.Jobs.List(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(all))
}
