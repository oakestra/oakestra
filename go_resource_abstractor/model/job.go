package model

// Job is a deployed microservice and its per-worker instances. Jobs carry
// the whole SLA the user submitted, so unknown fields are the norm here.
// They're stored and returned verbatim, in Extra.
//
// Every optional field is a pointer. Nil means absent, so it's omitted on
// write and a PATCH leaves the stored value untouched. A non-nil pointer,
// even to a zero value or an empty slice, is a real value and gets written.
//
// An explicit JSON null leaves the pointer nil too, but is not the same as
// an absent key: it's recorded in Extra (see decodePtr) and written back
// out as a literal null, so a PATCH sending one $sets the stored field to
// null rather than leaving it alone.
type Job struct {
	ID            *string `json:"_id,omitempty" bson:"_id,omitempty"`
	JobName       *string `json:"job_name,omitempty" bson:"job_name,omitempty"`
	ApplicationID *string `json:"applicationID,omitempty" bson:"applicationID,omitempty"`
	// Candidate is the id of the candidate resource this job is placed on.
	Candidate    *string        `json:"candidate,omitempty" bson:"candidate,omitempty"`
	InstanceList *[]JobInstance `json:"instance_list,omitempty" bson:"instance_list,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges Job's typed fields with Extra so unknown fields (the
// rest of the caller's SLA) round-trip.
func (j Job) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", j.ID)
	encodePtr(e, "job_name", j.JobName)
	encodePtr(e, "applicationID", j.ApplicationID)
	encodePtr(e, "candidate", j.Candidate)
	encodePtr(e, "instance_list", j.InstanceList)
	return e.finish(j.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (j *Job) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "_id", &j.ID)
	decodePtr(d, "job_name", &j.JobName)
	decodePtr(d, "applicationID", &j.ApplicationID)
	decodePtr(d, "candidate", &j.Candidate)
	decodePtr(d, "instance_list", &j.InstanceList)

	return d.finish(&j.Extra)
}

// MarshalBSON builds the stored document by hand instead of letting the
// driver reflect over Job's struct tags, so a field the Extra null-tracking
// in decodePtr/markBSONNulls recorded as explicitly null can be written
// back as a literal null even though the typed field itself is nil, the
// same as MarshalJSON does for a JSON response.
func (j Job) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(j.Extra)
	encodeBSONPtr(e, "_id", j.ID)
	encodeBSONPtr(e, "job_name", j.JobName)
	encodeBSONPtr(e, "applicationID", j.ApplicationID)
	encodeBSONPtr(e, "candidate", j.Candidate)
	encodeBSONPtr(e, "instance_list", j.InstanceList)
	return e.finish()
}

// UnmarshalBSON decodes a stored Job through a shadow type carrying the
// same fields but none of Job's methods (avoiding recursion back into this
// method), then records any of Job's named fields the document holds as an
// explicit null in Extra - the default codec's inline-map handling only
// ever attaches a key it doesn't recognize as a named field there, so a
// null stored for e.g. "candidate" would otherwise come back as a nil
// pointer indistinguishable from the key being absent, and the next
// encode would drop it instead of returning "candidate": null.
func (j *Job) UnmarshalBSON(data []byte) error {
	type jobAlias Job
	var alias jobAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*j = Job(alias)
	markBSONNulls[Job](data, &j.Extra)
	return nil
}

// JobInstance is one running copy of a job on one worker.
type JobInstance struct {
	InstanceNumber *int64  `json:"instance_number,omitempty" bson:"instance_number,omitempty"`
	WorkerID       *string `json:"worker_id,omitempty" bson:"worker_id,omitempty"`
	Status         *string `json:"status,omitempty" bson:"status,omitempty"`
	StatusDetail   *string `json:"status_detail,omitempty" bson:"status_detail,omitempty"`
	HostIP         *string `json:"host_ip,omitempty" bson:"host_ip,omitempty"`
	// HostPort, CPUPercent, MemoryPercent and Disk are Float/Int, not
	// float64/int64: NodeEngine and cluster_manager send them as strings, and
	// existing documents written by the Python service have them stored that
	// way too. See model.Float/model.Int.
	HostPort      *Int    `json:"host_port,omitempty" bson:"host_port,omitempty"`
	PublicIP      *string `json:"publicip,omitempty" bson:"publicip,omitempty"`
	CPUPercent    *Float  `json:"cpu_percent,omitempty" bson:"cpu_percent,omitempty"`
	MemoryPercent *Float  `json:"memory_percent,omitempty" bson:"memory_percent,omitempty"`
	Disk          *Float  `json:"disk,omitempty" bson:"disk,omitempty"`
	Logs          *string `json:"logs,omitempty" bson:"logs,omitempty"`
	// LastModifiedTimestamp is Unix seconds. Unlike CPUHistory/MemoryHistory's
	// timestamp, which the server generates on every PATCH, this one comes
	// from the request body verbatim: the caller (cluster_manager) sets it
	// before sending.
	LastModifiedTimestamp *float64            `json:"last_modified_timestamp,omitempty" bson:"last_modified_timestamp,omitempty"`
	CPUHistory            *[]JobHistorySample `json:"cpu_history,omitempty" bson:"cpu_history,omitempty"`
	MemoryHistory         *[]JobHistorySample `json:"memory_history,omitempty" bson:"memory_history,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges JobInstance's typed fields with Extra so unknown
// fields round-trip.
func (i JobInstance) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "instance_number", i.InstanceNumber)
	encodePtr(e, "worker_id", i.WorkerID)
	encodePtr(e, "status", i.Status)
	encodePtr(e, "status_detail", i.StatusDetail)
	encodePtr(e, "host_ip", i.HostIP)
	encodePtr(e, "host_port", i.HostPort)
	encodePtr(e, "publicip", i.PublicIP)
	encodePtr(e, "cpu_percent", i.CPUPercent)
	encodePtr(e, "memory_percent", i.MemoryPercent)
	encodePtr(e, "disk", i.Disk)
	encodePtr(e, "logs", i.Logs)
	encodePtr(e, "last_modified_timestamp", i.LastModifiedTimestamp)
	encodePtr(e, "cpu_history", i.CPUHistory)
	encodePtr(e, "memory_history", i.MemoryHistory)
	return e.finish(i.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (i *JobInstance) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "instance_number", &i.InstanceNumber)
	decodePtr(d, "worker_id", &i.WorkerID)
	decodePtr(d, "status", &i.Status)
	decodePtr(d, "status_detail", &i.StatusDetail)
	decodePtr(d, "host_ip", &i.HostIP)
	decodePtr(d, "host_port", &i.HostPort)
	decodePtr(d, "publicip", &i.PublicIP)
	decodePtr(d, "cpu_percent", &i.CPUPercent)
	decodePtr(d, "memory_percent", &i.MemoryPercent)
	decodePtr(d, "disk", &i.Disk)
	decodePtr(d, "logs", &i.Logs)
	decodePtr(d, "last_modified_timestamp", &i.LastModifiedTimestamp)
	decodePtr(d, "cpu_history", &i.CPUHistory)
	decodePtr(d, "memory_history", &i.MemoryHistory)

	return d.finish(&i.Extra)
}

// MarshalBSON builds the stored document by hand; see Job.MarshalBSON for
// why. This is what makes UpdateJobInstance's null writes for an omitted
// field (status, host_ip, ...) come back as "field": null on the next read
// instead of silently disappearing.
func (i JobInstance) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(i.Extra)
	encodeBSONPtr(e, "instance_number", i.InstanceNumber)
	encodeBSONPtr(e, "worker_id", i.WorkerID)
	encodeBSONPtr(e, "status", i.Status)
	encodeBSONPtr(e, "status_detail", i.StatusDetail)
	encodeBSONPtr(e, "host_ip", i.HostIP)
	encodeBSONPtr(e, "host_port", i.HostPort)
	encodeBSONPtr(e, "publicip", i.PublicIP)
	encodeBSONPtr(e, "cpu_percent", i.CPUPercent)
	encodeBSONPtr(e, "memory_percent", i.MemoryPercent)
	encodeBSONPtr(e, "disk", i.Disk)
	encodeBSONPtr(e, "logs", i.Logs)
	encodeBSONPtr(e, "last_modified_timestamp", i.LastModifiedTimestamp)
	encodeBSONPtr(e, "cpu_history", i.CPUHistory)
	encodeBSONPtr(e, "memory_history", i.MemoryHistory)
	return e.finish()
}

// UnmarshalBSON decodes a stored JobInstance through a shadow type, then
// records any of JobInstance's named fields the document holds as an explicit
// null in Extra; see Job.UnmarshalBSON.
func (i *JobInstance) UnmarshalBSON(data []byte) error {
	type jobInstanceAlias JobInstance
	var alias jobInstanceAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*i = JobInstance(alias)
	markBSONNulls[JobInstance](data, &i.Extra)
	return nil
}
