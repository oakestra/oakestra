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

// TestHookEventsUpdateInjectsIDButUpdateFoundDoesNot pins entity.go's one
// behavioral difference between Update and UpdateFound: Update sets
// data["_id"] = id before running pre_update (the id came from the URL
// path, so the hook is told which document it's writing), while
// UpdateFound - used by upsertByName's update-by-name branch - leaves data
// untouched, matching Python's perform_update signature where the id is a
// separate argument alongside an unmodified payload. Swapping the two
// methods' bodies leaves every other test in this file green, since none of
// them inspect the payload a pre_update hook actually receives.
//
// A single hook, registered once for the "jobs" entity's pre_update event,
// observes both a path-addressed PATCH (Update) and a name-addressed PUT
// update (UpdateFound via upsertByName): the sync dispatch in
// services.Hooks.processSyncHook blocks the request until the hook
// responds, so by the time doRequest returns, the body it received is
// already sitting on the channel - no post-event-style polling needed.
func TestHookEventsUpdateInjectsIDButUpdateFoundDoesNot(t *testing.T) {
	bodies := make(chan map[string]any, 1)
	registerHook(t, "jobs", db.EventPreUpdate, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies <- body
		// Echo the body back unchanged: a sync hook must return a JSON
		// object or the dispatcher fails open and keeps the original
		// payload, which would make this recorder inert either way.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})

	t.Run("PATCH path-addressed update injects _id", func(t *testing.T) {
		id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
			"job_name": uniqueName("job"),
		}))

		// No _id in the request body: the only way one can show up in the
		// hook's payload is entity.Update's injection.
		rec := doRequest(t, http.MethodPatch, "/api/v1/jobs/"+id, map[string]any{
			"status": "RUNNING",
		})
		if rec.Code >= 300 {
			t.Fatalf("PATCH /api/v1/jobs/%s failed: status %d: %s", id, rec.Code, rec.Body.String())
		}

		body := <-bodies
		if got, _ := body["_id"].(string); got != id {
			t.Errorf("PATCH /api/v1/jobs/%s: pre_update hook body _id = %v, want %v", id, body["_id"], id)
		}
	})

	t.Run("PUT name-addressed update branch omits _id", func(t *testing.T) {
		name := uniqueName("job")
		// Seed via POST so the tracked PUT below matches by name and takes
		// upsertByName's update branch, which calls UpdateFound.
		doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{"job_name": name})

		rec := doRequest(t, http.MethodPut, "/api/v1/jobs/", map[string]any{
			"job_name": name,
			"status":   "RUNNING",
		})
		if rec.Code >= 300 {
			t.Fatalf("PUT /api/v1/jobs/ (update branch) failed: status %d: %s", rec.Code, rec.Body.String())
		}

		body := <-bodies
		if _, ok := body["_id"]; ok {
			t.Errorf("PUT /api/v1/jobs/ (update branch): pre_update hook body unexpectedly has _id = %v, want no _id key", body["_id"])
		}
	})
}

// TestHookEventsPostDeleteEntityID pins entity.Delete's contract that
// post_delete's entity_id is always the id taken from the request path, not
// the deleted document's own _id (see entity.Delete's doc comment for why:
// DeleteResource and a never-existed DeleteCustomResourceInstance have no
// document to read an _id from at all).
//
// DeleteJobInstance is the row where a document-keyed implementation
// wouldn't obviously produce the wrong id: DeleteJobInstance's store call
// doesn't return a document with its own top-level _id the way
// Create/Update do (see db.Store.DeleteJobInstance). DeleteResource is the
// row that actually discriminates path-id from document-id, since
// DeleteCandidate returns no document whatsoever.
func TestHookEventsPostDeleteEntityID(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) (entity, pathID string, doDelete func(t *testing.T) *httptest.ResponseRecorder)
	}{
		{
			name: "DeleteApplication",
			build: func(t *testing.T) (string, string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/applications/", map[string]any{
					"application_name": uniqueName("app"),
				}))
				return "applications", id, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/applications/"+id, nil)
				}
			},
		},
		{
			name: "DeleteJob",
			build: func(t *testing.T) (string, string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/jobs/", map[string]any{
					"job_name": uniqueName("job"),
				}))
				return "jobs", id, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/jobs/"+id, nil)
				}
			},
		},
		{
			// The path is /jobs/{jobID}/{instanceNumber}: the expected
			// entity_id is jobID, not the instance number "1", proving
			// post_delete keys off the job id rather than whatever other
			// path segment happens to also be present.
			name: "DeleteJobInstance",
			build: func(t *testing.T) (string, string, func(t *testing.T) *httptest.ResponseRecorder) {
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
				return "jobs", id, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/jobs/"+id+"/1", nil)
				}
			},
		},
		{
			// DeleteCandidate (the store call behind DeleteResource) returns
			// no document at all, so this row is the one that actually
			// discriminates "keyed off the path id" from "keyed off the
			// deleted document": a document-keyed implementation has
			// nothing to fall back to here.
			name: "DeleteResource",
			build: func(t *testing.T) (string, string, func(t *testing.T) *httptest.ResponseRecorder) {
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/resources/", map[string]any{
					"candidate_name": uniqueName("candidate"),
				}))
				return "resources", id, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/resources/"+id, nil)
				}
			},
		},
		{
			name: "DeleteCustomResourceInstance",
			build: func(t *testing.T) (string, string, func(t *testing.T) *httptest.ResponseRecorder) {
				resourceType := uniqueName("cr-delete-id")
				registerCustomResourceType(t, resourceType)
				id := mustID(t, doRequest(t, http.MethodPost, "/api/v1/custom-resources/"+resourceType, map[string]any{
					"name": "instance-1",
				}))
				return resourceType, id, func(t *testing.T) *httptest.ResponseRecorder {
					return doRequest(t, http.MethodDelete, "/api/v1/custom-resources/"+resourceType+"/"+id, nil)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entity, pathID, doDelete := tc.build(t)

			received := make(chan map[string]any, 1)
			registerHook(t, entity, db.EventPostDelete, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				received <- body
				w.WriteHeader(http.StatusOK)
			})

			rec := doDelete(t)
			if rec.Code >= 300 {
				t.Fatalf("%s: delete failed: status %d: %s", tc.name, rec.Code, rec.Body.String())
			}

			body := waitForHookEvent(t, received, string(db.EventPostDelete), tc.name)
			if body["entity_id"] != pathID {
				t.Errorf("%s: post_delete entity_id = %v, want %v (the path id)", tc.name, body["entity_id"], pathID)
			}
		})
	}
}

// TestHookEventsDeleteNeverExistedFiresPostDelete pins
// DeleteCustomResourceInstance's deliberate not-found swallow (see the
// comment above its store call in api/customresources.go): deleting an
// instance id that was never created still answers 200 with {"_id": id} and
// still fires post_delete, matching Python's handler, which returns a
// hardcoded {"_id": resource_id} regardless of whether anything was
// actually deleted.
func TestHookEventsDeleteNeverExistedFiresPostDelete(t *testing.T) {
	resourceType := uniqueName("cr-delete-missing")
	registerCustomResourceType(t, resourceType)

	// A well-formed ObjectID hex that was never used to create an instance,
	// so the store call underneath finds nothing to delete.
	id := newObjectIDHex()

	received := make(chan map[string]any, 1)
	registerHook(t, resourceType, db.EventPostDelete, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(http.StatusOK)
	})

	rec := doRequest(t, http.MethodDelete, "/api/v1/custom-resources/"+resourceType+"/"+id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/v1/custom-resources/%s/%s (never existed) status = %d, want 200: %s",
			resourceType, id, rec.Code, rec.Body.String())
	}
	got := decodeJSON[map[string]any](t, rec)
	if got["_id"] != id {
		t.Errorf("DELETE /api/v1/custom-resources/%s/%s (never existed) body _id = %v, want %v",
			resourceType, id, got["_id"], id)
	}

	body := waitForHookEvent(t, received, string(db.EventPostDelete), "DeleteCustomResourceInstance (never existed)")
	if body["entity_id"] != id {
		t.Errorf("DeleteCustomResourceInstance (never existed): post_delete entity_id = %v, want %v", body["entity_id"], id)
	}
}
