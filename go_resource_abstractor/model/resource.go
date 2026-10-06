package model

// Resource is a candidate resource: a worker node at cluster level, a whole
// cluster at root level. Unknown fields are stored and returned verbatim, in
// Extra. vram, vram_percent, gpu_temp and gpu_drivers, for instance, are
// canonical fields the store always projects but openapi.yaml doesn't type,
// so they land here.
//
// Every optional field is a pointer. Nil means absent, so it's omitted on
// write and a PATCH leaves the stored value untouched. A non-nil pointer,
// even to a zero value or an empty slice, is a real value and gets written.
//
// An explicit JSON null leaves the pointer nil too, but is not the same as
// an absent key: it's recorded in Extra (see decodePtr) and written back
// out as a literal null, so a PATCH sending one $sets the stored field to
// null rather than leaving it alone.
type Resource struct {
	ID                *string `json:"_id,omitempty" bson:"_id,omitempty"`
	CandidateName     *string `json:"candidate_name,omitempty" bson:"candidate_name,omitempty"`
	CandidateLocation *string `json:"candidate_location,omitempty" bson:"candidate_location,omitempty"`
	IP                *string `json:"ip,omitempty" bson:"ip,omitempty"`
	Port              *string `json:"port,omitempty" bson:"port,omitempty"`
	Architecture      *string `json:"architecture,omitempty" bson:"architecture,omitempty"`

	// Active is true when LastModifiedTimestamp falls within the last 30
	// seconds. It's computed per request, not persisted: store.FindCandidates
	// sets it via an aggregation stage, so a Resource read from a plain
	// single-document lookup won't have it set.
	Active                     *bool          `json:"active,omitempty" bson:"active,omitempty"`
	ActiveNodes                *int64         `json:"active_nodes,omitempty" bson:"active_nodes,omitempty"`
	Memory                     *int64         `json:"memory,omitempty" bson:"memory,omitempty"`
	VCPUs                      *int64         `json:"vcpus,omitempty" bson:"vcpus,omitempty"`
	VGPUs                      *int64         `json:"vgpus,omitempty" bson:"vgpus,omitempty"`
	CPUPercent                 *float64       `json:"cpu_percent,omitempty" bson:"cpu_percent,omitempty"`
	MemoryPercent              *float64       `json:"memory_percent,omitempty" bson:"memory_percent,omitempty"`
	GPUPercent                 *int64         `json:"gpu_percent,omitempty" bson:"gpu_percent,omitempty"`
	AggregationPerArchitecture map[string]any `json:"aggregation_per_architecture,omitempty" bson:"aggregation_per_architecture,omitempty"`

	// Virtualization, SupportedAddons and CSIDrivers are three of many
	// optional Resource fields. rest's field-shape validation still treats
	// them as nullable to match the Python service's 422 messages, even
	// though every optional field here already treats null the same as
	// omitted.
	Virtualization  *[]string `json:"virtualization,omitempty" bson:"virtualization,omitempty"`
	SupportedAddons *[]string `json:"supported_addons,omitempty" bson:"supported_addons,omitempty"`
	CSIDrivers      *[]any    `json:"csi_drivers,omitempty" bson:"csi_drivers,omitempty"`

	// CPUHistory and MemoryHistory are appended server-side on every PATCH
	// and capped at the most recent 100 entries. They're never set directly
	// from a request; the store strips them from the $set doc before
	// applying it.
	CPUHistory    *[]HistorySample `json:"cpu_history,omitempty" bson:"cpu_history,omitempty"`
	MemoryHistory *[]HistorySample `json:"memory_history,omitempty" bson:"memory_history,omitempty"`

	// LastModifiedTimestamp is Unix seconds, refreshed server-side on every
	// usage report.
	LastModifiedTimestamp *float64 `json:"last_modified_timestamp,omitempty" bson:"last_modified_timestamp,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges Resource's typed fields with Extra so unknown fields
// round-trip.
func (r Resource) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", r.ID)
	encodePtr(e, "candidate_name", r.CandidateName)
	encodePtr(e, "candidate_location", r.CandidateLocation)
	encodePtr(e, "ip", r.IP)
	encodePtr(e, "port", r.Port)
	encodePtr(e, "architecture", r.Architecture)
	encodePtr(e, "active", r.Active)
	encodePtr(e, "active_nodes", r.ActiveNodes)
	encodePtr(e, "memory", r.Memory)
	encodePtr(e, "vcpus", r.VCPUs)
	encodePtr(e, "vgpus", r.VGPUs)
	encodePtr(e, "cpu_percent", r.CPUPercent)
	encodePtr(e, "memory_percent", r.MemoryPercent)
	encodePtr(e, "gpu_percent", r.GPUPercent)
	encodeMap(e, "aggregation_per_architecture", r.AggregationPerArchitecture)
	encodePtr(e, "virtualization", r.Virtualization)
	encodePtr(e, "supported_addons", r.SupportedAddons)
	encodePtr(e, "csi_drivers", r.CSIDrivers)
	encodePtr(e, "cpu_history", r.CPUHistory)
	encodePtr(e, "memory_history", r.MemoryHistory)
	encodePtr(e, "last_modified_timestamp", r.LastModifiedTimestamp)
	return e.finish(r.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (r *Resource) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "_id", &r.ID)
	decodePtr(d, "candidate_name", &r.CandidateName)
	decodePtr(d, "candidate_location", &r.CandidateLocation)
	decodePtr(d, "ip", &r.IP)
	decodePtr(d, "port", &r.Port)
	decodePtr(d, "architecture", &r.Architecture)
	decodePtr(d, "active", &r.Active)
	decodePtr(d, "active_nodes", &r.ActiveNodes)
	decodePtr(d, "memory", &r.Memory)
	decodePtr(d, "vcpus", &r.VCPUs)
	decodePtr(d, "vgpus", &r.VGPUs)
	decodePtr(d, "cpu_percent", &r.CPUPercent)
	decodePtr(d, "memory_percent", &r.MemoryPercent)
	decodePtr(d, "gpu_percent", &r.GPUPercent)
	decodeAnyMap(d, "aggregation_per_architecture", &r.AggregationPerArchitecture)
	decodePtr(d, "virtualization", &r.Virtualization)
	decodePtr(d, "supported_addons", &r.SupportedAddons)
	decodeAnySlice(d, "csi_drivers", &r.CSIDrivers)
	decodePtr(d, "cpu_history", &r.CPUHistory)
	decodePtr(d, "memory_history", &r.MemoryHistory)
	decodePtr(d, "last_modified_timestamp", &r.LastModifiedTimestamp)

	return d.finish(&r.Extra)
}

// MarshalBSON builds the stored document by hand instead of letting the
// driver reflect over Resource's struct tags, so a field decodePtr/
// decodeValue recorded as explicitly null (see markBSONNulls) can be
// written back as a literal null even though the typed field itself is
// nil/empty, the same as MarshalJSON does for a JSON response. See
// model/job.go's Job.MarshalBSON for the full reasoning.
func (r Resource) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(r.Extra)
	encodeBSONPtr(e, "_id", r.ID)
	encodeBSONPtr(e, "candidate_name", r.CandidateName)
	encodeBSONPtr(e, "candidate_location", r.CandidateLocation)
	encodeBSONPtr(e, "ip", r.IP)
	encodeBSONPtr(e, "port", r.Port)
	encodeBSONPtr(e, "architecture", r.Architecture)
	encodeBSONPtr(e, "active", r.Active)
	encodeBSONPtr(e, "active_nodes", r.ActiveNodes)
	encodeBSONPtr(e, "memory", r.Memory)
	encodeBSONPtr(e, "vcpus", r.VCPUs)
	encodeBSONPtr(e, "vgpus", r.VGPUs)
	encodeBSONPtr(e, "cpu_percent", r.CPUPercent)
	encodeBSONPtr(e, "memory_percent", r.MemoryPercent)
	encodeBSONPtr(e, "gpu_percent", r.GPUPercent)
	encodeBSONMap(e, "aggregation_per_architecture", r.AggregationPerArchitecture)
	encodeBSONPtr(e, "virtualization", r.Virtualization)
	encodeBSONPtr(e, "supported_addons", r.SupportedAddons)
	encodeBSONPtr(e, "csi_drivers", r.CSIDrivers)
	encodeBSONPtr(e, "cpu_history", r.CPUHistory)
	encodeBSONPtr(e, "memory_history", r.MemoryHistory)
	encodeBSONPtr(e, "last_modified_timestamp", r.LastModifiedTimestamp)
	return e.finish()
}

// UnmarshalBSON decodes a stored Resource through a shadow type carrying
// the same fields but none of Resource's methods, then records any of
// Resource's named fields the document holds as an explicit null in Extra;
// see model/job.go's Job.UnmarshalBSON.
func (r *Resource) UnmarshalBSON(data []byte) error {
	type resourceAlias Resource
	var alias resourceAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*r = Resource(alias)
	markBSONNulls[Resource](data, &r.Extra)
	return nil
}
