package model

// CustomResourceDefinition is a dynamically registered resource type. Its
// resource_type both names the type and picks the path segment
// (/api/v1/custom-resources/{resource_type}) and the Mongo collection its
// instances live in.
type CustomResourceDefinition struct {
	ID *string `json:"_id,omitempty" bson:"_id,omitempty"`

	// ResourceType is required.
	ResourceType string `json:"resource_type" bson:"resource_type"`

	// Schema is the JSON Schema every instance of this type is validated
	// against. An absent or empty schema imposes no constraint, so a nil or
	// empty map is treated the same way and omitted on write.
	Schema map[string]any `json:"schema,omitempty" bson:"schema,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges CustomResourceDefinition's typed fields with Extra so
// unknown fields round-trip.
func (d CustomResourceDefinition) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", d.ID)
	encodeValue(e, "resource_type", d.ResourceType)
	encodeMap(e, "schema", d.Schema)
	return e.finish(d.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (d *CustomResourceDefinition) UnmarshalJSON(data []byte) error {
	f, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(f, "_id", &d.ID)
	decodeValue(f, "resource_type", &d.ResourceType)
	decodeValue(f, "schema", &d.Schema)

	return f.finish(&d.Extra)
}

// MarshalBSON builds the stored document by hand instead of letting the
// driver reflect over CustomResourceDefinition's struct tags, so a field
// decodePtr/decodeValue recorded as explicitly null (see markBSONNulls) can
// be written back as a literal null even though the typed field itself is
// nil/empty, the same as MarshalJSON does for a JSON response. See
// model/job.go's Job.MarshalBSON for the full reasoning.
func (d CustomResourceDefinition) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(d.Extra)
	encodeBSONPtr(e, "_id", d.ID)
	encodeBSONValue(e, "resource_type", d.ResourceType)
	encodeBSONMap(e, "schema", d.Schema)
	return e.finish()
}

// UnmarshalBSON decodes a stored CustomResourceDefinition through a shadow
// type carrying the same fields but none of CustomResourceDefinition's
// methods, then records any of CustomResourceDefinition's named fields the
// document holds as an explicit null in Extra; see model/job.go's
// Job.UnmarshalBSON.
func (d *CustomResourceDefinition) UnmarshalBSON(data []byte) error {
	type customResourceDefinitionAlias CustomResourceDefinition
	var alias customResourceDefinitionAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*d = CustomResourceDefinition(alias)
	markBSONNulls[CustomResourceDefinition](data, &d.Extra)
	return nil
}

// CustomResourceInstance is one instance of a registered custom resource
// type. Its fields are whatever the type's own JSON Schema allows, so
// nothing beyond _id is fixed here. Everything else lands in Extra.
type CustomResourceInstance struct {
	ID *string `json:"_id,omitempty" bson:"_id,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges CustomResourceInstance's _id with Extra so unknown
// fields round-trip.
func (c CustomResourceInstance) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", c.ID)
	return e.finish(c.Extra)
}

// UnmarshalJSON decodes _id into ID; everything else becomes Extra.
func (c *CustomResourceInstance) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "_id", &c.ID)

	return d.finish(&c.Extra)
}
