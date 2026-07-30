package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"go_resource_abstractor/db"
	"go_resource_abstractor/openapi"
)

// TestSpecEndpointsServeTheEmbeddedSpec covers the two /docs routes, which
// restore the spec-publishing the Python service did at /docs/openapi.json.
func TestSpecEndpointsServeTheEmbeddedSpec(t *testing.T) {
	jsonRec := doRequest(t, http.MethodGet, "/docs/openapi.json", nil)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("json status = %d, want 200: %s", jsonRec.Code, jsonRec.Body.String())
	}
	served := decodeJSON[map[string]any](t, jsonRec)
	if served["openapi"] == nil || served["paths"] == nil {
		t.Errorf("served document doesn't look like an OpenAPI spec: %v", served)
	}

	yamlRec := doRequest(t, http.MethodGet, "/docs/openapi.yaml", nil)
	if yamlRec.Code != http.StatusOK {
		t.Fatalf("yaml status = %d, want 200", yamlRec.Code)
	}

	// Both routes must publish the same document, otherwise a consumer's view
	// of the contract depends on which one it fetched.
	var fromYAML map[string]any
	if err := yaml.Unmarshal(yamlRec.Body.Bytes(), &fromYAML); err != nil {
		t.Fatalf("parse served yaml: %v", err)
	}
	roundTripped := map[string]any{}
	body, err := json.Marshal(fromYAML)
	if err != nil {
		t.Fatalf("re-encode served yaml: %v", err)
	}
	if err := json.Unmarshal(body, &roundTripped); err != nil {
		t.Fatalf("decode re-encoded yaml: %v", err)
	}
	if !reflect.DeepEqual(served, roundTripped) {
		t.Error("/docs/openapi.json and /docs/openapi.yaml describe different documents")
	}
}

// TestTrailingSlashesResolveIdentically guards stripTrailingSlash, which
// replaced the per-route double registration that reproduced Flask's
// strict_slashes = False. Item routes are the interesting case: the
// collection routes are already covered incidentally by the rest of the
// suite, which addresses them with a trailing slash throughout.
func TestTrailingSlashesResolveIdentically(t *testing.T) {
	created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/resources", map[string]any{
		"candidate_name": uniqueName("trailing-slash"),
	}))
	id, _ := created["_id"].(string)
	if id == "" {
		t.Fatalf("created resource has no _id: %v", created)
	}

	for _, path := range []string{"/api/v1/resources/" + id, "/api/v1/resources/" + id + "/"} {
		rec := doRequest(t, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
		}
		if got := decodeJSON[map[string]any](t, rec)["_id"]; got != id {
			t.Errorf("GET %s returned _id %v, want %v", path, got, id)
		}
	}
}

// TestNonNumericInstanceIDIsBadRequest covers the one parameter the spec
// declares with a non-string type, and therefore the only one whose binding
// can fail inside the generated wrapper: the response has to come back in the
// service's own {"message": ...} shape, not oapi-codegen's default.
func TestNonNumericInstanceIDIsBadRequest(t *testing.T) {
	rec := doRequest(t, http.MethodGet, "/api/v1/jobs/"+newObjectIDHex()+"/not-a-number", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON[openapi.Message](t, rec); got.Message != "Bad Request" {
		t.Errorf("message = %q, want %q", got.Message, "Bad Request")
	}
}

// TestEmptyStringFilterIsIgnored guards a detail the switch to generated
// parameter binding could easily have lost: an absent filter and one supplied
// empty ("?candidate_name=") both mean "no filter", where binding the empty
// pointer straight into the query would instead match only documents whose
// field is literally "".
func TestEmptyStringFilterIsIgnored(t *testing.T) {
	name := uniqueName("empty-filter")
	doRequest(t, http.MethodPost, "/api/v1/resources", map[string]any{"candidate_name": name})

	rec := doRequest(t, http.MethodGet, "/api/v1/resources?candidate_name=", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(decodeJSON[[]map[string]any](t, rec)) == 0 {
		t.Error("empty candidate_name filtered everything out; it should be ignored")
	}
}

// TestHookEventsMatchSpecEnum keeps the event names the dispatcher fires
// (db.AsyncEvents / db.SyncEvents) in step with the HookEvent enum the API
// validates registrations against. A name in one list but not the other would
// let a hook be registered for an event that never fires, or fire an event no
// client is allowed to subscribe to.
func TestHookEventsMatchSpecEnum(t *testing.T) {
	dispatched := append(append([]db.HookEvent{}, db.AsyncEvents...), db.SyncEvents...)

	declared := []openapi.HookEvent{
		openapi.PreCreate, openapi.PreUpdate, openapi.PreDelete,
		openapi.PostCreate, openapi.PostUpdate, openapi.PostDelete,
	}
	if len(dispatched) != len(declared) {
		t.Fatalf("dispatcher knows %d events, spec declares %d", len(dispatched), len(declared))
	}

	declaredSet := map[openapi.HookEvent]bool{}
	for _, e := range declared {
		if !e.Valid() {
			t.Errorf("%q is not a member of the generated HookEvent enum", e)
		}
		declaredSet[e] = true
	}
	for _, e := range dispatched {
		if !declaredSet[openapi.HookEvent(e)] {
			t.Errorf("dispatcher fires %q, which the spec's HookEvent enum doesn't declare", e)
		}
	}
}

// TestResponsesMatchGeneratedModels checks that what the handlers actually
// write decodes into the types generated from openapi.yaml. Decoding fails on
// any field whose real type contradicts the declared one - an integer served
// where the spec says string, say - so this is what stops the spec from
// drifting into fiction while the tests around it keep passing.
func TestResponsesMatchGeneratedModels(t *testing.T) {
	t.Run("resource", func(t *testing.T) {
		name := uniqueName("model-resource")
		created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/resources", map[string]any{
			"candidate_name": name,
			"memory":         2048,
			"cpu_percent":    12.5,
			"virtualization": []string{"docker"},
		}))
		id, _ := created["_id"].(string)

		// PATCH first, so the response carries the server-appended history
		// arrays and refreshed timestamp as well as the plain fields.
		doRequest(t, http.MethodPatch, "/api/v1/resources/"+id, map[string]any{"cpu_percent": 42.5})

		got := decodeJSON[openapi.Resource](t, doRequest(t, http.MethodGet, "/api/v1/resources/"+id, nil))
		if got.ID == nil || *got.ID != id {
			t.Errorf("_id = %v, want %v", got.ID, id)
		}
		if got.CandidateName == nil || *got.CandidateName != name {
			t.Errorf("candidate_name = %v, want %v", got.CandidateName, name)
		}
		if got.CpuHistory == nil || len(*got.CpuHistory) != 1 {
			t.Errorf("cpu_history = %v, want one sample", got.CpuHistory)
		}

		// active sits outside the canonical projection, so it only appears on
		// the list route and only when requested - as the spec says. Freshness
		// itself (does active come back true) is resources_test.go's concern;
		// this only checks that the field decodes into the generated model.
		listed := decodeJSON[[]openapi.Resource](t, doRequest(t, http.MethodGet,
			"/api/v1/resources?candidate_name="+name+"&resources=active", nil))
		if len(listed) != 1 {
			t.Fatalf("listed %d candidates, want 1", len(listed))
		}
		if listed[0].Active == nil {
			t.Errorf("active = nil, want a value present in the projection")
		}
	})

	t.Run("application", func(t *testing.T) {
		name := uniqueName("model-app")
		got := decodeJSON[openapi.Application](t, doRequest(t, http.MethodPost, "/api/v1/applications", map[string]any{
			"application_name":      name,
			"application_namespace": "default",
			"microservices":         []string{},
		}))
		if got.ApplicationName == nil || *got.ApplicationName != name {
			t.Errorf("application_name = %v, want %v", got.ApplicationName, name)
		}
		if got.ApplicationID == nil || got.ID == nil || *got.ApplicationID != *got.ID {
			t.Errorf("applicationID %v should mirror _id %v", got.ApplicationID, got.ID)
		}
	})

	t.Run("job", func(t *testing.T) {
		name := uniqueName("model-job")
		created := decodeJSON[map[string]any](t, doRequest(t, http.MethodPost, "/api/v1/jobs", map[string]any{
			"job_name":      name,
			"applicationID": newObjectIDHex(),
		}))
		id, _ := created["_id"].(string)

		appended := doRequest(t, http.MethodPut, "/api/v1/jobs/"+id+"/0", map[string]any{
			"instance_list": []map[string]any{{"instance_number": 0, "worker_id": "worker-1"}},
		})
		got := decodeJSON[openapi.Job](t, appended)
		if got.JobName == nil || *got.JobName != name {
			t.Errorf("job_name = %v, want %v", got.JobName, name)
		}
		if got.InstanceList == nil || len(*got.InstanceList) != 1 {
			t.Fatalf("instance_list = %v, want one instance", got.InstanceList)
		}
		if worker := (*got.InstanceList)[0].WorkerId; worker == nil || *worker != "worker-1" {
			t.Errorf("worker_id = %v, want worker-1", worker)
		}

		// PATCH is what actually populates cpu_percent/memory_percent and their
		// history - the fields the spec's JobInstance schema got wrong (named
		// cpu/memory, and a numeric history timestamp) until it was corrected
		// to match what's really written here.
		patched := decodeJSON[openapi.Job](t, doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id+"/0", map[string]any{
			"cpu_percent":    33.5,
			"memory_percent": 61.2,
		}))
		if patched.InstanceList == nil || len(*patched.InstanceList) != 1 {
			t.Fatalf("instance_list = %v, want one instance", patched.InstanceList)
		}
		instance := (*patched.InstanceList)[0]
		if instance.CpuPercent == nil || *instance.CpuPercent != 33.5 {
			t.Errorf("cpu_percent = %v, want 33.5", instance.CpuPercent)
		}
		if instance.MemoryPercent == nil || *instance.MemoryPercent != 61.2 {
			t.Errorf("memory_percent = %v, want 61.2", instance.MemoryPercent)
		}
		if instance.CpuHistory == nil || len(*instance.CpuHistory) != 1 || (*instance.CpuHistory)[0].Timestamp == nil {
			t.Errorf("cpu_history = %v, want one sample with a timestamp", instance.CpuHistory)
		}

		// GetJobInstance returns the job with instance_list filtered down to
		// the match, not a bare JobInstance - decoding into openapi.Job (not
		// openapi.JobInstance, which the spec wrongly declared as this route's
		// response) is itself the regression check.
		fetched := decodeJSON[openapi.Job](t, doRequest(t, http.MethodGet, "/api/v1/jobs/"+id+"/0", nil))
		if fetched.InstanceList == nil || len(*fetched.InstanceList) != 1 {
			t.Fatalf("GET instance_list = %v, want one matching instance", fetched.InstanceList)
		}
		if fetched.ID == nil || *fetched.ID != id {
			t.Errorf("GET _id = %v, want the job id %v (a bare instance wouldn't have one)", fetched.ID, id)
		}
	})

	t.Run("hook", func(t *testing.T) {
		got := decodeJSON[openapi.Hook](t, doRequest(t, http.MethodPost, "/api/v1/hooks", map[string]any{
			"hook_name":   uniqueName("model-hook"),
			"webhook_url": "http://example.invalid/hook",
			"entity":      "jobs",
			"events":      []string{"post_create", "pre_update"},
		}))
		if got.Events == nil || len(*got.Events) != 2 {
			t.Fatalf("events = %v, want two", got.Events)
		}
		for _, e := range *got.Events {
			if !e.Valid() {
				t.Errorf("served event %q is not a HookEvent enum member", e)
			}
		}
	})

	t.Run("custom resource definition", func(t *testing.T) {
		resourceType := uniqueName("model-crd")
		got := decodeJSON[openapi.CustomResourceDefinition](t, doRequest(t, http.MethodPost, "/api/v1/custom-resources", map[string]any{
			"resource_type": resourceType,
			"schema":        map[string]any{"type": "object"},
		}))
		if got.ResourceType != resourceType {
			t.Errorf("resource_type = %v, want %v", got.ResourceType, resourceType)
		}
		if got.Schema == nil {
			t.Error("schema is absent")
		}
	})
}
