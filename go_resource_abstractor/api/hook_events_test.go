package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"go_resource_abstractor/db"
)

// hookWriteRow describes one write path and the (entity, event) pair each of
// its hook stages must fire. Rows without a pre event (the delete paths -
// there is no PreDelete anywhere in services/hooks.go) leave preEvent zero.
//
// build prepares whatever fixtures the path needs (creating a parent
// application/job/candidate/custom-resource-type, as each row requires) and
// returns the entity the hooks should fire under - dynamic for the custom
// resource rows, since the entity there is the registered resource_type -
// plus a write func that performs only the tracked request.
//
// Fixture setup happens inside build, before the caller registers any hook,
// so prerequisite writes (e.g. creating a job before appending an instance
// to it) can never be mistaken for the write under test even when they
// share the same entity/event pair.
//
// injectSentinel and verifySentinel customize how the pre-event test proves
// a hook's replacement payload was persisted. They default (nil) to adding
// a new top-level field and reading it back from the same spot, which is
// true for every route except the two job-instance ones: AppendJobInstance
// only ever persists the last element of instance_list verbatim (any other
// top-level key in the hook's response is ignored, see db.Store.AppendJobInstance),
// and PatchJobInstance/UpdateJobInstance $sets a fixed whitelist of known
// instance fields rather than the payload wholesale (see
// db.Store.UpdateJobInstance) - an unrecognized field would silently vanish
// on both, so those two rows override the field they mutate/check to one
// each function actually looks at.
type hookWriteRow struct {
	name      string
	postEvent db.HookEvent
	preEvent  db.HookEvent // zero value ("") for the delete paths
	build     func(t *testing.T) (entity string, write func(t *testing.T) *httptest.ResponseRecorder)

	injectSentinel func(body map[string]any, value string)
	verifySentinel func(persisted map[string]any) any
}

// lastInstance returns the last element of job's instance_list as a
// map[string]any, or nil if there isn't one - shared by the job-instance
// rows' injectSentinel/verifySentinel overrides.
func lastInstance(job map[string]any) map[string]any {
	list, _ := job["instance_list"].([]any)
	if len(list) == 0 {
		return nil
	}
	last, _ := list[len(list)-1].(map[string]any)
	return last
}

func hookWriteRows() []hookWriteRow {
	return []hookWriteRow{
		{
			name:      "CreateApplication",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				return "applications", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
						"application_name": uniqueName("app"),
					})
				}
			},
		},
		{
			name:      "PatchApplication",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
					"application_name": uniqueName("app"),
				}))
				return "applications", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPatch, "/api/v1/applications/"+id, map[string]any{
						"application_name": uniqueName("app-patched"),
					})
				}
			},
		},
		{
			name:      "DeleteApplication",
			postEvent: db.EventPostDelete,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
					"application_name": uniqueName("app"),
				}))
				return "applications", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/applications/"+id, nil)
				}
			},
		},
		{
			name:      "CreateJob",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
						"job_name": uniqueName("job"),
					})
				}
			},
		},
		{
			name:      "UpsertJob/create",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				name := uniqueName("upsert-job-create")
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					// job_name doesn't exist yet, so this PUT takes the
					// create branch of upsertByName.
					return doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
						"job_name": name,
					})
				}
			},
		},
		{
			name:      "UpsertJob/update",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				name := uniqueName("upsert-job-update")
				// Seed via POST so the tracked PUT below matches by name and
				// takes the update branch.
				doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{"job_name": name})
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
						"job_name": name,
						"status":   "RUNNING",
					})
				}
			},
		},
		{
			name:      "PatchJob",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name": uniqueName("job"),
				}))
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{
						"status": "RUNNING",
					})
				}
			},
		},
		{
			name:      "DeleteJob",
			postEvent: db.EventPostDelete,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name": uniqueName("job"),
				}))
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/jobs/"+id, nil)
				}
			},
		},
		{
			name:      "AppendJobInstance",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name":      uniqueName("job"),
					"instance_list": []any{},
				}))
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPut, "/api/v1/jobs/"+id+"/1", map[string]any{
						"instance_list": []any{map[string]any{"instance_number": 1}},
					})
				}
			},
			// AppendJobInstance only ever persists instance_list's last
			// element, not the hook's response wholesale - so the sentinel
			// has to live inside that element to actually round-trip.
			injectSentinel: func(body map[string]any, value string) {
				if last := lastInstance(body); last != nil {
					last["status"] = value
				}
			},
			verifySentinel: func(persisted map[string]any) any {
				if last := lastInstance(persisted); last != nil {
					return last["status"]
				}
				return nil
			},
		},
		{
			name:      "PatchJobInstance",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name":      uniqueName("job"),
					"instance_list": []any{},
				}))
				appendRec := doRequest(t, http.MethodPut, "/api/v1/jobs/"+id+"/1", map[string]any{
					"instance_list": []any{map[string]any{"instance_number": 1}},
				})
				if appendRec.Code != http.StatusOK {
					t.Fatalf("fixture: append instance status = %d, want 200: %s", appendRec.Code, appendRec.Body.String())
				}
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id+"/1", map[string]any{
						"status": "RUNNING",
					})
				}
			},
			// UpdateJobInstance $sets a fixed whitelist of recognized
			// instance fields (see db/jobs.go) rather than the payload
			// wholesale, so the sentinel has to overwrite one of those -
			// status_detail here - instead of adding a new key.
			injectSentinel: func(body map[string]any, value string) {
				body["status_detail"] = value
			},
			verifySentinel: func(persisted map[string]any) any {
				if last := lastInstance(persisted); last != nil {
					return last["status_detail"]
				}
				return nil
			},
		},
		{
			name:      "DeleteJobInstance",
			postEvent: db.EventPostDelete,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name":      uniqueName("job"),
					"instance_list": []any{},
				}))
				appendRec := doRequest(t, http.MethodPut, "/api/v1/jobs/"+id+"/1", map[string]any{
					"instance_list": []any{map[string]any{"instance_number": 1}},
				})
				if appendRec.Code != http.StatusOK {
					t.Fatalf("fixture: append instance status = %d, want 200: %s", appendRec.Code, appendRec.Body.String())
				}
				return "jobs", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/jobs/"+id+"/1", nil)
				}
			},
		},
		{
			name:      "CreateResource",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				return "resources", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
						"candidate_name": uniqueName("candidate"),
					})
				}
			},
		},
		{
			name:      "UpsertResource/create",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				name := uniqueName("upsert-resource-create")
				return "resources", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPut, "/api/v1/resources/", map[string]any{
						"candidate_name": name,
					})
				}
			},
		},
		{
			name:      "UpsertResource/update",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				name := uniqueName("upsert-resource-update")
				doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{"candidate_name": name})
				return "resources", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPut, "/api/v1/resources/", map[string]any{
						"candidate_name": name,
						"cpu_percent":    10.0,
					})
				}
			},
		},
		{
			name:      "PatchResource",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
					"candidate_name": uniqueName("candidate"),
				}))
				return "resources", func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPatch, "/api/v1/resources/"+id, map[string]any{
						"cpu_percent": 55.0,
					})
				}
			},
		},
		{
			name:      "DeleteResource",
			postEvent: db.EventPostDelete,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
					"candidate_name": uniqueName("candidate"),
				}))
				return "resources", func(t *testing.T) *httptest.ResponseRecorder {
					// DeleteResource answers 204 with no body.
					return doRequest(t, http.MethodDelete, "/api/v1/resources/"+id, nil)
				}
			},
		},
		{
			name:      "CreateCustomResourceInstance",
			postEvent: db.EventPostCreate,
			preEvent:  db.EventPreCreate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				resourceType := uniqueName("cr-create")
				registerCustomResourceType(t, resourceType)
				return resourceType, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
						"name": "instance-1",
					})
				}
			},
		},
		{
			name:      "PatchCustomResourceInstance",
			postEvent: db.EventPostUpdate,
			preEvent:  db.EventPreUpdate,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				resourceType := uniqueName("cr-patch")
				registerCustomResourceType(t, resourceType)
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
					"name": "instance-1",
				}))
				return resourceType, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodPatch, "/api/v1/custom-resources/"+resourceType+"/"+id, map[string]any{
						"name": "instance-1-patched",
					})
				}
			},
		},
		{
			name:      "DeleteCustomResourceInstance",
			postEvent: db.EventPostDelete,
			build: func(t *testing.T) (string, func(t *testing.T) *httptest.ResponseRecorder) {
				resourceType := uniqueName("cr-delete")
				registerCustomResourceType(t, resourceType)
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
					"name": "instance-1",
				}))
				return resourceType, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/custom-resources/"+resourceType+"/"+id, nil)
				}
			},
		},
	}
}

// mustID decodes rec's body and returns its "_id" field, failing the test if
// the request didn't succeed or the body has no usable id.
func mustID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code >= 300 {
		t.Fatalf("fixture request failed: status %d: %s", rec.Code, rec.Body.String())
	}
	doc := decodeJSON[map[string]any](t, rec)
	id, ok := doc["_id"].(string)
	if !ok || id == "" {
		t.Fatalf("fixture response has no usable _id: %v", doc)
	}
	return id
}

// registerCustomResourceType POSTs a schema-less type definition, the
// prerequisite every /custom-resources/{resourceType}[/...] route requires
// before its instances can be touched (see api/customresources_test.go).
func registerCustomResourceType(t *testing.T, resourceType string) {
	t.Helper()
	rec := doRequest(t, http.MethodPost, "/api/v1/custom-resources/", map[string]any{
		"resource_type": resourceType,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("fixture: register custom resource type status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// registerHook creates a throwaway webhook server running handler and
// registers it in Mongo for entity/event, cleaning up both the Mongo
// registration and the server at the end of the (sub)test. Every hook gets
// a unique hook_name/webhook_url pair, satisfying the collection's unique
// indexes even when many rows share the same entity.
func registerHook(t *testing.T, entity string, event db.HookEvent, handler http.HandlerFunc) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	created, err := testStore.CreateHook(context.Background(), bson.M{
		"hook_name":   uniqueName("hook"),
		"webhook_url": server.URL,
		"entity":      entity,
		"events":      bson.A{string(event)},
	})
	if err != nil {
		t.Fatalf("register hook: %v", err)
	}
	t.Cleanup(func() {
		_ = testStore.DeleteHook(context.Background(), db.ExtractID(created))
	})
}

// waitForHookEvent drains ch until a delivery with the expected event name
// arrives, or fails the test after 3s. Draining rather than trusting the
// first delivery guards against unrelated traffic on a shared entity/event
// pair - defensive here since each row gets its own dedicated server, but
// cheap insurance against a stray delivery from a prior subtest's
// in-flight goroutine.
func waitForHookEvent(t *testing.T, ch <-chan map[string]any, event, label string) map[string]any {
	t.Helper()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case body := <-ch:
			if body["event"] == event {
				return body
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s to fire its %s hook", label, event)
			return nil
		}
	}
}

// TestHookEventsFirePostEvent walks every write path and asserts the async
// post_* webhook fires with the {entity, event, entity_id} body
// processAsyncHook builds, for exactly the (entity, event) pair the path is
// documented to use.
func TestHookEventsFirePostEvent(t *testing.T) {
	for _, row := range hookWriteRows() {
		t.Run(row.name, func(t *testing.T) {
			entity, write := row.build(t)

			received := make(chan map[string]any, 4)
			registerHook(t, entity, row.postEvent, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				received <- body
				w.WriteHeader(http.StatusOK)
			})

			rec := write(t)
			if rec.Code >= 300 {
				t.Fatalf("write failed: status %d: %s", rec.Code, rec.Body.String())
			}

			body := waitForHookEvent(t, received, string(row.postEvent), row.name)
			if body["entity"] != entity {
				t.Errorf("entity = %v, want %v", body["entity"], entity)
			}
			if id, ok := body["entity_id"].(string); !ok || id == "" {
				t.Errorf("entity_id = %v, want a non-empty string", body["entity_id"])
			}
		})
	}
}

// TestHookEventsPreEventTransformsPayload walks the rows that fire a pre_*
// event and asserts a sync hook's returned payload actually replaces what
// gets persisted: the hook here echoes back the received body with a
// sentinel value mutated in (see injectSentinel), and the test checks the
// sentinel made it into the stored document (see verifySentinel). For the
// rows using the defaults, that stored document is read via the write's own
// response body - insertReturning/updateByID return exactly what was
// written (see db/store_helpers.go), so decoding the response is equivalent
// to reading the document back from Mongo.
func TestHookEventsPreEventTransformsPayload(t *testing.T) {
	const sentinelField = "injected_by_hook"
	defaultInject := func(body map[string]any, value string) { body[sentinelField] = value }
	defaultVerify := func(persisted map[string]any) any { return persisted[sentinelField] }

	for _, row := range hookWriteRows() {
		t.Run(row.name, func(t *testing.T) {
			if row.preEvent == "" {
				t.Skip("no pre event for this path (deletes have no sync stage)")
			}
			inject, verify := row.injectSentinel, row.verifySentinel
			if inject == nil {
				inject = defaultInject
			}
			if verify == nil {
				verify = defaultVerify
			}

			entity, write := row.build(t)
			sentinelValue := uniqueName("sentinel")

			registerHook(t, entity, row.preEvent, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body == nil {
					body = map[string]any{}
				}
				inject(body, sentinelValue)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(body)
			})

			rec := write(t)
			if rec.Code >= 300 {
				t.Fatalf("write failed: status %d: %s", rec.Code, rec.Body.String())
			}

			persisted := decodeJSON[map[string]any](t, rec)
			if got := verify(persisted); got != sentinelValue {
				t.Errorf("sentinel = %v, want %v: the %s hook's replacement payload was not persisted",
					got, sentinelValue, row.preEvent)
			}
		})
	}
}
