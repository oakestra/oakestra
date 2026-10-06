package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFieldEncoderStickyError checks that once encodeValue fails, later
// calls are no-ops and finish reports the first error.
func TestFieldEncoderStickyError(t *testing.T) {
	e := newFieldEncoder()
	encodeValue(e, "ok", "fine")
	encodeValue(e, "bad", make(chan int)) // encoding/json can't marshal a channel
	encodeValue(e, "after", "should never be written")

	if e.err == nil {
		t.Fatal("expected encodeValue to fail on a channel value")
	}
	if _, ok := e.fields["after"]; ok {
		t.Error("encodeValue after an error should be a no-op")
	}

	if _, err := e.finish(Extra{"extra_key": "should be ignored"}); err != e.err {
		t.Errorf("finish() error = %v, want the sticky error %v", err, e.err)
	}
}

// TestFieldDecoderStickyError checks the same thing for decodeValue: a
// failed call sticks, and finish reports it instead of building Extra.
func TestFieldDecoderStickyError(t *testing.T) {
	d, err := newFieldDecoder([]byte(`{"a": "not-a-number", "b": "value"}`))
	if err != nil {
		t.Fatalf("newFieldDecoder: %v", err)
	}

	var a int
	var b string
	decodeValue(d, "a", &a)
	decodeValue(d, "b", &b)

	if d.err == nil {
		t.Fatal("expected decodeValue to fail decoding \"a\" into an int")
	}
	if b != "" {
		t.Errorf("decodeValue after an error should be a no-op, got b = %q", b)
	}

	var extra Extra
	if err := d.finish(&extra); err != d.err {
		t.Errorf("finish() error = %v, want the sticky error %v", err, d.err)
	}
}

// TestJobUnmarshalJSONFieldErrorSurfaces checks that UnmarshalJSON reports a
// per-field decode failure: candidate is a string field, so a JSON number in
// its place can't decode.
func TestJobUnmarshalJSONFieldErrorSurfaces(t *testing.T) {
	var job Job
	err := json.Unmarshal([]byte(`{"job_name": "j", "candidate": 123}`), &job)
	if err == nil {
		t.Fatal("expected an error decoding candidate from a JSON number")
	}
	if !strings.Contains(err.Error(), "candidate") {
		t.Errorf("error = %v, want it to name the failing field", err)
	}
}

// TestJobMarshalJSONExtraErrorSurfaces checks that MarshalJSON reports an
// Extra value encoding/json can't marshal, instead of dropping it silently
// or panicking.
func TestJobMarshalJSONExtraErrorSurfaces(t *testing.T) {
	job := Job{
		JobName: Ptr("j"),
		Extra:   Extra{"callback": func() {}},
	}
	_, err := json.Marshal(job)
	if err == nil {
		t.Fatal("expected an error marshaling an Extra field holding a func")
	}
	if !strings.Contains(err.Error(), "callback") {
		t.Errorf("error = %v, want it to name the failing extra field", err)
	}
}
