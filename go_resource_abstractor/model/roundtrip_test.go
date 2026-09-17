package model

import (
	"encoding/json"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestJobJSONRoundTripPreservesExtraAndNumberKinds checks that unknown
// fields survive in Extra and that json.Number resolution keeps an integer
// distinct from a float across a round trip. The test uses 2.5, not 2.0:
// encoding/json prints a whole-number float64 without a decimal point, so
// 2.0 would come back as an int64 on the second decode.
func TestJobJSONRoundTripPreservesExtraAndNumberKinds(t *testing.T) {
	input := []byte(`{
		"_id": "65f1c0d2a1b2c3d4e5f60718",
		"job_name": "my-job",
		"applicationID": "app-1",
		"instance_list": [{"instance_number": 1, "status": "RUNNING"}],
		"replicas": 3,
		"cpu_weight": 2.5,
		"nested": {"a": 1, "b": 2.5}
	}`)

	var job Job
	if err := json.Unmarshal(input, &job); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if job.JobName == nil || *job.JobName != "my-job" {
		t.Fatalf("job_name = %v, want my-job", job.JobName)
	}
	if job.InstanceList == nil || len(*job.InstanceList) != 1 {
		t.Fatalf("instance_list not decoded correctly: %+v", job.InstanceList)
	}
	instances := *job.InstanceList
	if instances[0].Status == nil || *instances[0].Status != "RUNNING" {
		t.Fatalf("instance_list[0].status = %v, want RUNNING", instances[0].Status)
	}

	if v, ok := job.Extra["replicas"].(int64); !ok || v != 3 {
		t.Errorf("Extra[replicas] = %#v, want int64(3)", job.Extra["replicas"])
	}
	if v, ok := job.Extra["cpu_weight"].(float64); !ok || v != 2.5 {
		t.Errorf("Extra[cpu_weight] = %#v, want float64(2.5)", job.Extra["cpu_weight"])
	}
	nested, ok := job.Extra["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Extra[nested] = %#v, want map[string]any", job.Extra["nested"])
	}
	if v, ok := nested["a"].(int64); !ok || v != 1 {
		t.Errorf("Extra[nested][a] = %#v, want int64(1)", nested["a"])
	}
	if v, ok := nested["b"].(float64); !ok || v != 2.5 {
		t.Errorf("Extra[nested][b] = %#v, want float64(2.5)", nested["b"])
	}

	// Re-marshal and decode again: typed fields and Extra should both
	// survive a second trip unchanged.
	out, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var again Job
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatalf("unmarshal round 2: %v", err)
	}
	if !reflect.DeepEqual(job, again) {
		t.Errorf("job changed across a round trip:\nfirst:  %#v\nsecond: %#v", job, again)
	}
}

// TestResourceJSONOptionalFields exercises the states
// virtualization/supported_addons/csi_drivers can be in: an explicit null
// decodes with the typed field left nil (same as an absent field, since no
// consumer of the pointer can tell the two apart), but round-trips back out
// as an explicit null instead of vanishing - HEAD's update_resource $sets
// exactly what the caller sent, nulls included, and a plain absent field
// still stays omitted.
func TestResourceJSONOptionalFields(t *testing.T) {
	input := []byte(`{
		"candidate_name": "node-1",
		"virtualization": null,
		"csi_drivers": ["driver-a", 1]
	}`)

	var r Resource
	if err := json.Unmarshal(input, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if r.Virtualization != nil {
		t.Errorf("virtualization = %+v, want nil", r.Virtualization)
	}
	if r.SupportedAddons != nil {
		t.Errorf("supported_addons = %+v, want nil (field was absent)", r.SupportedAddons)
	}
	if r.CSIDrivers == nil || len(*r.CSIDrivers) != 2 {
		t.Errorf("csi_drivers = %+v, want a 2-element list", r.CSIDrivers)
	}

	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal to raw map: %v", err)
	}

	if raw, ok := decoded["virtualization"]; !ok || string(raw) != "null" {
		t.Errorf(`marshaled output should carry "virtualization": null (explicit null), got present=%v raw=%s`, ok, raw)
	}
	if _, ok := decoded["supported_addons"]; ok {
		t.Errorf(`marshaled output should omit "supported_addons" (it was never set), got %v`, string(decoded["supported_addons"]))
	}
	if _, ok := decoded["csi_drivers"]; !ok {
		t.Errorf(`marshaled output missing "csi_drivers"`)
	}
}

// TestResourceBSONRoundTrip covers Extra plus the states a pointer field can
// be in through BSON, using the driver directly the way internal/store
// does. nil is omitted from the document entirely, so a $set built from it
// never touches the stored value. A non-nil pointer, even to an empty slice,
// writes the real value instead of collapsing to the same shape as "unset"
// the way a plain omitempty slice tag would.
func TestResourceBSONRoundTrip(t *testing.T) {
	name := "node-1"
	original := Resource{
		CandidateName:   Ptr(name),
		Virtualization:  nil,
		SupportedAddons: Ptr([]string{}),
		CSIDrivers:      Ptr([]any{"driver-a"}),
		Extra: Extra{
			"custom_flag":  int64(7),
			"custom_ratio": 1.5,
		},
	}

	data, err := bson.Marshal(original)
	if err != nil {
		t.Fatalf("bson marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("bson unmarshal to raw doc: %v", err)
	}
	if v, ok := doc["virtualization"]; ok {
		t.Errorf("virtualization should be omitted from the BSON document when nil, got %#v", v)
	}
	if addons, ok := doc["supported_addons"].(bson.A); !ok || len(addons) != 0 {
		t.Errorf("supported_addons = %#v, want a present, empty array", doc["supported_addons"])
	}

	var decoded Resource
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("bson unmarshal: %v", err)
	}

	if decoded.CandidateName == nil || *decoded.CandidateName != name {
		t.Errorf("candidate_name = %v, want %q", decoded.CandidateName, name)
	}
	if decoded.Virtualization != nil {
		t.Errorf("virtualization = %+v, want nil", decoded.Virtualization)
	}
	if decoded.SupportedAddons == nil || len(*decoded.SupportedAddons) != 0 {
		t.Errorf("supported_addons = %+v, want a present, empty list", decoded.SupportedAddons)
	}
	if decoded.CSIDrivers == nil || len(*decoded.CSIDrivers) != 1 {
		t.Errorf("csi_drivers = %+v, want a 1-element list", decoded.CSIDrivers)
	}

	if v, ok := decoded.Extra["custom_flag"].(int64); !ok || v != 7 {
		t.Errorf("Extra[custom_flag] = %#v, want int64(7)", decoded.Extra["custom_flag"])
	}
	if v, ok := decoded.Extra["custom_ratio"].(float64); !ok || v != 1.5 {
		t.Errorf("Extra[custom_ratio] = %#v, want float64(1.5)", decoded.Extra["custom_ratio"])
	}
}

// TestHookEventValid checks the enum membership test PostHook/PatchHook
// validation will eventually delegate to.
func TestHookEventValid(t *testing.T) {
	for _, e := range []HookEvent{EventPreCreate, EventPreUpdate, EventPreDelete, EventPostCreate, EventPostUpdate, EventPostDelete} {
		if !e.Valid() {
			t.Errorf("%q should be a valid HookEvent", e)
		}
	}
	if HookEvent("not_a_real_event").Valid() {
		t.Error(`"not_a_real_event" should not be a valid HookEvent`)
	}
}

// TestCustomResourceInstanceExtraOnly checks that everything but _id lands
// in Extra, since a custom resource instance's shape is entirely defined by
// its type's own JSON Schema.
func TestCustomResourceInstanceExtraOnly(t *testing.T) {
	input := []byte(`{"_id": "65f1c0d2a1b2c3d4e5f60718", "status": "new", "count": 4}`)

	var inst CustomResourceInstance
	if err := json.Unmarshal(input, &inst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if inst.ID == nil || *inst.ID != "65f1c0d2a1b2c3d4e5f60718" {
		t.Errorf("ID = %v, want the _id value", inst.ID)
	}
	if inst.Extra["status"] != "new" {
		t.Errorf(`Extra["status"] = %v, want "new"`, inst.Extra["status"])
	}
	if v, ok := inst.Extra["count"].(int64); !ok || v != 4 {
		t.Errorf(`Extra["count"] = %#v, want int64(4)`, inst.Extra["count"])
	}
}

// jsonKeyState reports how key appears in a marshaled JSON object: absent
// entirely, present holding a literal null, or present holding raw (the
// verbatim encoded value otherwise).
func jsonKeyState(t *testing.T, out []byte, key string) string {
	t.Helper()
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal to raw map: %v", err)
	}
	raw, ok := decoded[key]
	if !ok {
		return "absent"
	}
	if string(raw) == "null" {
		return "null"
	}
	return string(raw)
}

// TestJobFieldThreeStatesJSON checks that candidate and instance_list each
// preserve absent/null/empty/populated across a JSON decode-then-encode
// round trip. An explicit null decodes with the typed field left nil (same
// as an absent field - no consumer of the pointer can tell the two apart),
// but round-trips back out as an explicit null rather than disappearing:
// HEAD's update_job $sets exactly what the PATCH body sent, nulls included
// (see jobs_blueprint.py's patch handler calling jobs_db.update_job
// straight through), so an absent field must still stay untouched/omitted,
// but a null one must come back as null.
func TestJobFieldThreeStatesJSON(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCand string
		wantList string
	}{
		{"absent", `{"job_name": "j"}`, "absent", "absent"},
		{"null", `{"job_name": "j", "candidate": null, "instance_list": null}`, "null", "null"},
		{"empty", `{"job_name": "j", "candidate": "", "instance_list": []}`, `""`, "[]"},
		{"value", `{"job_name": "j", "candidate": "c1", "instance_list": [{"instance_number": 1}]}`, `"c1"`, `[{"instance_number":1}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var job Job
			if err := json.Unmarshal([]byte(tt.body), &job); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			out, err := json.Marshal(job)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if got := jsonKeyState(t, out, "candidate"); got != tt.wantCand {
				t.Errorf("candidate = %s, want %s", got, tt.wantCand)
			}
			if got := jsonKeyState(t, out, "instance_list"); got != tt.wantList {
				t.Errorf("instance_list = %s, want %s", got, tt.wantList)
			}
		})
	}
}

// TestJobFieldThreeStatesBSON checks the same three states through BSON,
// using bson.Marshal directly (the path internal/store's toSetDoc takes,
// via Job's own MarshalBSON): an absent field is omitted from the document,
// so a $set built from it never touches the stored value; a non-nil
// pointer, even to an empty slice, writes the real value instead of
// collapsing to the same shape as "unset" like a plain omitempty slice tag
// would; and an explicit null - decoded from a JSON body first, since null
// decodes to the same nil pointer as an absent field and only Extra
// remembers which one it was - writes back as a literal BSON null rather
// than being omitted, matching what HEAD's update_job/update_job_instance
// store for a null field.
func TestJobFieldThreeStatesBSON(t *testing.T) {
	var nullDecoded Job
	if err := json.Unmarshal([]byte(`{"candidate": null, "instance_list": null}`), &nullDecoded); err != nil {
		t.Fatalf("unmarshal explicit nulls: %v", err)
	}

	tests := []struct {
		name        string
		job         Job
		presentCand bool
		wantCand    any
		presentList bool
		// wantListLen < 0 means the document holds a literal null rather
		// than an array.
		wantListLen int
	}{
		{"absent", Job{}, false, nil, false, 0},
		{"null", nullDecoded, true, nil, true, -1},
		{
			"empty",
			Job{Candidate: Ptr(""), InstanceList: Ptr([]JobInstance{})},
			true, "", true, 0,
		},
		{
			"value",
			Job{Candidate: Ptr("c1"), InstanceList: Ptr([]JobInstance{{}})},
			true, "c1", true, 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := bson.Marshal(tt.job)
			if err != nil {
				t.Fatalf("bson marshal: %v", err)
			}

			var doc bson.M
			if err := bson.Unmarshal(data, &doc); err != nil {
				t.Fatalf("bson unmarshal to raw doc: %v", err)
			}

			candVal, candPresent := doc["candidate"]
			if candPresent != tt.presentCand {
				t.Errorf("candidate present = %v, want %v", candPresent, tt.presentCand)
			}
			if candPresent && candVal != tt.wantCand {
				t.Errorf("candidate = %#v, want %#v", candVal, tt.wantCand)
			}

			listVal, listPresent := doc["instance_list"]
			if listPresent != tt.presentList {
				t.Errorf("instance_list present = %v, want %v", listPresent, tt.presentList)
			}
			if listPresent {
				if tt.wantListLen < 0 {
					if listVal != nil {
						t.Errorf("instance_list = %#v, want a literal null", listVal)
					}
				} else if list, ok := listVal.(bson.A); !ok || len(list) != tt.wantListLen {
					t.Errorf("instance_list = %#v, want a %d-element array", listVal, tt.wantListLen)
				}
			}

			var decoded Job
			if err := bson.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("bson unmarshal into Job: %v", err)
			}
			if !reflect.DeepEqual(decoded.Candidate, tt.job.Candidate) {
				t.Errorf("decoded candidate = %+v, want %+v", decoded.Candidate, tt.job.Candidate)
			}
			if !reflect.DeepEqual(decoded.InstanceList, tt.job.InstanceList) {
				t.Errorf("decoded instance_list = %+v, want %+v", decoded.InstanceList, tt.job.InstanceList)
			}
		})
	}
}

// TestApplicationFieldThreeStatesJSON checks that application_name and
// microservices each preserve absent/null/empty/populated across a round
// trip: an explicit null decodes with the typed field left nil (same as
// absent), but round-trips back out as null instead of disappearing,
// matching HEAD's update_app, which $sets exactly what the PATCH body
// sent.
func TestApplicationFieldThreeStatesJSON(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantMS   string
	}{
		{"absent", `{}`, "absent", "absent"},
		{"null", `{"application_name": null, "microservices": null}`, "null", "null"},
		{"empty", `{"application_name": "", "microservices": []}`, `""`, "[]"},
		{"value", `{"application_name": "a", "microservices": ["m1"]}`, `"a"`, `["m1"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var app Application
			if err := json.Unmarshal([]byte(tt.body), &app); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			out, err := json.Marshal(app)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if got := jsonKeyState(t, out, "application_name"); got != tt.wantName {
				t.Errorf("application_name = %s, want %s", got, tt.wantName)
			}
			if got := jsonKeyState(t, out, "microservices"); got != tt.wantMS {
				t.Errorf("microservices = %s, want %s", got, tt.wantMS)
			}
		})
	}
}

// TestApplicationFieldThreeStatesBSON checks the same three states for
// Application through BSON, via Application's own MarshalBSON: an explicit
// null writes back as a literal BSON null instead of being omitted, the
// same as TestJobFieldThreeStatesBSON.
func TestApplicationFieldThreeStatesBSON(t *testing.T) {
	var nullDecoded Application
	if err := json.Unmarshal([]byte(`{"application_name": null, "microservices": null}`), &nullDecoded); err != nil {
		t.Fatalf("unmarshal explicit nulls: %v", err)
	}

	tests := []struct {
		name        string
		app         Application
		presentName bool
		wantName    any
		presentMS   bool
		// wantMSLen < 0 means the document holds a literal null rather than
		// an array.
		wantMSLen int
	}{
		{"absent", Application{}, false, nil, false, 0},
		{"null", nullDecoded, true, nil, true, -1},
		{
			"empty",
			Application{ApplicationName: Ptr(""), Microservices: Ptr([]string{})},
			true, "", true, 0,
		},
		{
			"value",
			Application{ApplicationName: Ptr("a"), Microservices: Ptr([]string{"m1"})},
			true, "a", true, 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := bson.Marshal(tt.app)
			if err != nil {
				t.Fatalf("bson marshal: %v", err)
			}

			var doc bson.M
			if err := bson.Unmarshal(data, &doc); err != nil {
				t.Fatalf("bson unmarshal to raw doc: %v", err)
			}

			nameVal, namePresent := doc["application_name"]
			if namePresent != tt.presentName {
				t.Errorf("application_name present = %v, want %v", namePresent, tt.presentName)
			}
			if namePresent && nameVal != tt.wantName {
				t.Errorf("application_name = %#v, want %#v", nameVal, tt.wantName)
			}

			msVal, msPresent := doc["microservices"]
			if msPresent != tt.presentMS {
				t.Errorf("microservices present = %v, want %v", msPresent, tt.presentMS)
			}
			if msPresent {
				if tt.wantMSLen < 0 {
					if msVal != nil {
						t.Errorf("microservices = %#v, want a literal null", msVal)
					}
				} else if list, ok := msVal.(bson.A); !ok || len(list) != tt.wantMSLen {
					t.Errorf("microservices = %#v, want a %d-element array", msVal, tt.wantMSLen)
				}
			}

			var decoded Application
			if err := bson.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("bson unmarshal into Application: %v", err)
			}
			if !reflect.DeepEqual(decoded.ApplicationName, tt.app.ApplicationName) {
				t.Errorf("decoded application_name = %+v, want %+v", decoded.ApplicationName, tt.app.ApplicationName)
			}
			if !reflect.DeepEqual(decoded.Microservices, tt.app.Microservices) {
				t.Errorf("decoded microservices = %+v, want %+v", decoded.Microservices, tt.app.Microservices)
			}
		})
	}
}

// TestJobInstanceBSONNullSurvivesToJSON is the regression this fix targets,
// isolated at the model layer: a document read straight out of Mongo (built
// here the way UpdateJobInstance actually writes one - see
// internal/store/jobs.go - with status/host_ip explicitly null and
// worker_id simply never set) must come back through JSON with the null
// keys present and the absent key omitted, not the other way around.
func TestJobInstanceBSONNullSurvivesToJSON(t *testing.T) {
	stored := bson.D{
		{Key: "instance_number", Value: int64(1)},
		{Key: "status", Value: nil},
		{Key: "host_ip", Value: nil},
		{Key: "cpu_percent", Value: 50.0},
		// worker_id is entirely absent, not null.
	}
	data, err := bson.Marshal(stored)
	if err != nil {
		t.Fatalf("bson marshal fixture: %v", err)
	}

	var instance JobInstance
	if err := bson.Unmarshal(data, &instance); err != nil {
		t.Fatalf("bson unmarshal: %v", err)
	}
	if instance.Status != nil {
		t.Errorf("Status = %v, want nil", instance.Status)
	}
	if instance.WorkerID != nil {
		t.Errorf("WorkerID = %v, want nil", instance.WorkerID)
	}

	out, err := json.Marshal(instance)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if got := jsonKeyState(t, out, "status"); got != "null" {
		t.Errorf(`status = %s, want "null" (explicitly stored null must survive)`, got)
	}
	if got := jsonKeyState(t, out, "host_ip"); got != "null" {
		t.Errorf(`host_ip = %s, want "null" (explicitly stored null must survive)`, got)
	}
	if got := jsonKeyState(t, out, "worker_id"); got != "absent" {
		t.Errorf(`worker_id = %s, want "absent" (was never stored)`, got)
	}
	if got := jsonKeyState(t, out, "cpu_percent"); got != "50" {
		t.Errorf("cpu_percent = %s, want 50", got)
	}
}
