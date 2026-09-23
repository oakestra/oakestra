package model

// HistorySample is one entry of a Resource's cpu_history/memory_history. The
// server appends it on every PATCH /api/v1/resources/{id} and caps history
// at the most recent 100 entries. Unlike JobHistorySample, timestamp is Unix
// seconds.
//
// Neither field handles unknown fields: both are written server-side, never
// accepted from a caller as-is, so there's nothing for extra fields to
// attach to.
type HistorySample struct {
	// Value is nullable because cpu_percent/memory_percent are: a report
	// that omits cpu_percent still appends a history entry, just with a
	// null value. There's no "omitempty" on the json tag, so a nil Value
	// marshals as an explicit "value": null instead of disappearing. That's
	// the opposite of every other optional field in this package, where nil
	// means absent. It's safe here because Value is only ever written
	// server-side, never decoded from a request, so there's no PATCH
	// semantics to preserve.
	Value     *float64 `json:"value" bson:"value"`
	Timestamp float64  `json:"timestamp" bson:"timestamp"`
}

// JobHistorySample is one entry of a JobInstance's cpu_history/memory_history.
// Unlike Resource's HistorySample, timestamp is an ISO-8601 string
// (datetime.now(timezone.utc).isoformat()), not Unix seconds, and Value is
// Float rather than *float64: it's built from JobInstance.CPUPercent/
// MemoryPercent (see internal/store/jobs.go's UpdateJobInstance), which are
// Float themselves, and old documents may hold it as a string too.
type JobHistorySample struct {
	Value     *Float `json:"value" bson:"value"`
	Timestamp string `json:"timestamp" bson:"timestamp"`
}
