package model

import "slices"

// HookEvent identifies a webhook trigger point. pre_* events fire
// synchronously before the write and may return a transformed payload that
// is persisted in place of the original; post_* events fire asynchronously
// after the write, receiving only the affected document's id.
type HookEvent string

const (
	EventPreCreate  HookEvent = "pre_create"
	EventPreUpdate  HookEvent = "pre_update"
	EventPreDelete  HookEvent = "pre_delete"
	EventPostCreate HookEvent = "post_create"
	EventPostUpdate HookEvent = "post_update"
	EventPostDelete HookEvent = "post_delete"
)

// AsyncEvents fire after the write completes; nothing waits on them.
var AsyncEvents = []HookEvent{EventPostCreate, EventPostUpdate, EventPostDelete}

// SyncEvents fire synchronously before the write and may transform the
// payload that gets persisted.
var SyncEvents = []HookEvent{EventPreCreate, EventPreUpdate, EventPreDelete}

// Valid reports whether e is a known member of the HookEvent enum.
func (e HookEvent) Valid() bool {
	return slices.Contains(SyncEvents, e) || slices.Contains(AsyncEvents, e)
}

// Hook is a webhook registration.
//
// Every optional field is a pointer. Nil means absent, so it's omitted on
// write and a PATCH leaves the stored value untouched. A non-nil pointer,
// even to a zero value or an empty slice, is a real value and gets written.
//
// An explicit JSON null leaves the pointer nil too, but is not the same as
// an absent key: it's recorded in Extra (see decodePtr) and written back
// out as a literal null, so a PATCH sending one $sets the stored field to
// null rather than leaving it alone.
type Hook struct {
	ID         *string      `json:"_id,omitempty" bson:"_id,omitempty"`
	HookName   *string      `json:"hook_name,omitempty" bson:"hook_name,omitempty"`
	WebhookURL *string      `json:"webhook_url,omitempty" bson:"webhook_url,omitempty"`
	Entity     *string      `json:"entity,omitempty" bson:"entity,omitempty"`
	Events     *[]HookEvent `json:"events,omitempty" bson:"events,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges Hook's typed fields with Extra so unknown fields
// round-trip.
func (h Hook) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", h.ID)
	encodePtr(e, "hook_name", h.HookName)
	encodePtr(e, "webhook_url", h.WebhookURL)
	encodePtr(e, "entity", h.Entity)
	encodePtr(e, "events", h.Events)
	return e.finish(h.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (h *Hook) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "_id", &h.ID)
	decodePtr(d, "hook_name", &h.HookName)
	decodePtr(d, "webhook_url", &h.WebhookURL)
	decodePtr(d, "entity", &h.Entity)
	decodePtr(d, "events", &h.Events)

	return d.finish(&h.Extra)
}

// MarshalBSON builds the stored document by hand instead of letting the
// driver reflect over Hook's struct tags, so a field decodePtr recorded as
// explicitly null (see markBSONNulls) can be written back as a literal null
// even though the typed field itself is nil, the same as MarshalJSON does
// for a JSON response. See model/job.go's Job.MarshalBSON for the full
// reasoning.
func (h Hook) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(h.Extra)
	encodeBSONPtr(e, "_id", h.ID)
	encodeBSONPtr(e, "hook_name", h.HookName)
	encodeBSONPtr(e, "webhook_url", h.WebhookURL)
	encodeBSONPtr(e, "entity", h.Entity)
	encodeBSONPtr(e, "events", h.Events)
	return e.finish()
}

// UnmarshalBSON decodes a stored Hook through a shadow type carrying the
// same fields but none of Hook's methods, then records any of Hook's named
// fields the document holds as an explicit null in Extra; see model/job.go's
// Job.UnmarshalBSON.
func (h *Hook) UnmarshalBSON(data []byte) error {
	type hookAlias Hook
	var alias hookAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*h = Hook(alias)
	markBSONNulls[Hook](data, &h.Extra)
	return nil
}
