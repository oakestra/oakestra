package model

// Application is a deployed application's metadata: the parent of a set of
// Jobs. Unknown fields are stored and returned verbatim, in Extra.
//
// Every optional field is a pointer. Nil means absent, so it's omitted on
// write and a PATCH leaves the stored value untouched. An explicit JSON
// null decodes the same way. A non-nil pointer, even to a zero value or an
// empty slice, is a real value and gets written.
type Application struct {
	ID *string `json:"_id,omitempty" bson:"_id,omitempty"`

	// ApplicationID is set server-side on creation to the document's own
	// _id, so a caller-supplied value here is always ignored on create.
	ApplicationID        *string   `json:"applicationID,omitempty" bson:"applicationID,omitempty"`
	ApplicationName      *string   `json:"application_name,omitempty" bson:"application_name,omitempty"`
	ApplicationNamespace *string   `json:"application_namespace,omitempty" bson:"application_namespace,omitempty"`
	ApplicationDesc      *string   `json:"application_desc,omitempty" bson:"application_desc,omitempty"`
	UserID               *string   `json:"userId,omitempty" bson:"userId,omitempty"`
	Microservices        *[]string `json:"microservices,omitempty" bson:"microservices,omitempty"`

	Extra Extra `json:"-" bson:",inline"`
}

// MarshalJSON merges Application's typed fields with Extra so unknown
// fields round-trip.
func (a Application) MarshalJSON() ([]byte, error) {
	e := newFieldEncoder()
	encodePtr(e, "_id", a.ID)
	encodePtr(e, "applicationID", a.ApplicationID)
	encodePtr(e, "application_name", a.ApplicationName)
	encodePtr(e, "application_namespace", a.ApplicationNamespace)
	encodePtr(e, "application_desc", a.ApplicationDesc)
	encodePtr(e, "userId", a.UserID)
	encodePtr(e, "microservices", a.Microservices)
	return e.finish(a.Extra)
}

// UnmarshalJSON decodes known keys into their typed fields; everything else
// becomes Extra.
func (a *Application) UnmarshalJSON(data []byte) error {
	d, err := newFieldDecoder(data)
	if err != nil {
		return err
	}

	decodePtr(d, "_id", &a.ID)
	decodePtr(d, "applicationID", &a.ApplicationID)
	decodePtr(d, "application_name", &a.ApplicationName)
	decodePtr(d, "application_namespace", &a.ApplicationNamespace)
	decodePtr(d, "application_desc", &a.ApplicationDesc)
	decodePtr(d, "userId", &a.UserID)
	decodePtr(d, "microservices", &a.Microservices)

	return d.finish(&a.Extra)
}

// MarshalBSON builds the stored document by hand instead of letting the
// driver reflect over Application's struct tags, so a field decodePtr
// recorded as explicitly null (see markBSONNulls) can be written back as a
// literal null even though the typed field itself is nil, the same as
// MarshalJSON does for a JSON response. See model/job.go's
// Job.MarshalBSON for the full reasoning.
func (a Application) MarshalBSON() ([]byte, error) {
	e := newBSONFieldEncoder(a.Extra)
	encodeBSONPtr(e, "_id", a.ID)
	encodeBSONPtr(e, "applicationID", a.ApplicationID)
	encodeBSONPtr(e, "application_name", a.ApplicationName)
	encodeBSONPtr(e, "application_namespace", a.ApplicationNamespace)
	encodeBSONPtr(e, "application_desc", a.ApplicationDesc)
	encodeBSONPtr(e, "userId", a.UserID)
	encodeBSONPtr(e, "microservices", a.Microservices)
	return e.finish()
}

// UnmarshalBSON decodes a stored Application through a shadow type carrying
// the same fields but none of Application's methods, then records any of
// Application's named fields the document holds as an explicit null in
// Extra; see model/job.go's Job.UnmarshalBSON.
func (a *Application) UnmarshalBSON(data []byte) error {
	type applicationAlias Application
	var alias applicationAlias
	if err := decodeBSONAlias(data, &alias); err != nil {
		return err
	}
	*a = Application(alias)
	markBSONNulls[Application](data, &a.Extra)
	return nil
}
