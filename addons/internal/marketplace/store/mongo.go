package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

// Collection names. Existing deployments may still have an unvalidated flat list in
// "marketplace"; using other collections means those documents are ignored rather than
// migrated or trusted.
const (
	SourcesCollection = "sources"
	CatalogCollection = "catalog"
)

// Mongo is the MongoDB Store.
type Mongo struct {
	sources *mongo.Collection
	catalog *mongo.Collection
}

// NewMongo wraps a database. Call EnsureIndexes before serving.
func NewMongo(db *mongo.Database) *Mongo {
	return &Mongo{sources: db.Collection(SourcesCollection), catalog: db.Collection(CatalogCollection)}
}

// EnsureIndexes creates the unique index on the source name. Snapshots are keyed by _id, which
// is unique already.
func (m *Mongo) EnsureIndexes(ctx context.Context) error {
	_, err := m.sources.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("name_unique"),
	})
	return err
}

type sourceDoc struct {
	Name          string    `bson:"name"`
	Kind          string    `bson:"kind"`
	URL           string    `bson:"url"`
	Token         string    `bson:"token,omitempty"`
	AllowInsecure bool      `bson:"allow_insecure"`
	AllowPrivate  bool      `bson:"allow_private"`
	CreatedAt     time.Time `bson:"created_at"`
	LastAttemptAt time.Time `bson:"last_attempt_at"`
	LastSyncedAt  time.Time `bson:"last_synced_at"`
	LastError     string    `bson:"last_error"`
}

func (d *sourceDoc) toSource() Source {
	return Source{
		Name:          d.Name,
		Kind:          definition.SourceKind(d.Kind),
		URL:           d.URL,
		Token:         d.Token,
		AllowInsecure: d.AllowInsecure,
		AllowPrivate:  d.AllowPrivate,
		CreatedAt:     d.CreatedAt,
		Status:        Status{LastAttemptAt: d.LastAttemptAt, LastSyncedAt: d.LastSyncedAt, LastError: d.LastError},
	}
}

// snapshotDoc keeps the addons as a JSON string rather than as BSON subdocuments. Definitions
// contain user-chosen map keys (labels, environment, ports) that may hold dots or start with
// "$", which BSON field names handle badly, and JSON keeps the field names identical to the API.
type snapshotDoc struct {
	ID          string    `bson:"_id"`
	Serial      int64     `bson:"serial"`
	ContentHash string    `bson:"content_hash"`
	ETag        string    `bson:"etag"`
	GeneratedAt time.Time `bson:"generated_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
	AddonCount  int       `bson:"addon_count"`
	Addons      string    `bson:"addons_json"`
}

func (m *Mongo) ListSources(ctx context.Context) ([]Source, error) {
	cur, err := m.sources.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []sourceDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]Source, len(docs))
	for i := range docs {
		out[i] = docs[i].toSource()
	}
	return out, nil
}

func (m *Mongo) GetSource(ctx context.Context, name string) (*Source, error) {
	var d sourceDoc
	err := m.sources.FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s := d.toSource()
	return &s, nil
}

func (m *Mongo) CreateSource(ctx context.Context, s *Source) error {
	_, err := m.sources.InsertOne(ctx, sourceDoc{
		Name:          s.Name,
		Kind:          string(s.Kind),
		URL:           s.URL,
		Token:         s.Token,
		AllowInsecure: s.AllowInsecure,
		AllowPrivate:  s.AllowPrivate,
		CreatedAt:     s.CreatedAt,
		LastAttemptAt: s.Status.LastAttemptAt,
		LastSyncedAt:  s.Status.LastSyncedAt,
		LastError:     s.Status.LastError,
	})
	if mongo.IsDuplicateKeyError(err) {
		return ErrExists
	}
	return err
}

func (m *Mongo) UpdateSourceConfig(ctx context.Context, s *Source) error {
	set := bson.D{
		{Key: "kind", Value: string(s.Kind)},
		{Key: "url", Value: s.URL},
		{Key: "allow_insecure", Value: s.AllowInsecure},
		{Key: "allow_private", Value: s.AllowPrivate},
	}
	if s.Token != "" {
		set = append(set, bson.E{Key: "token", Value: s.Token})
	}
	update := bson.D{{Key: "$set", Value: set}}
	if s.Token == "" {
		update = append(update, bson.E{Key: "$unset", Value: bson.D{{Key: "token", Value: ""}}})
	}
	return m.updateSource(ctx, s.Name, update)
}

func (m *Mongo) SetStatus(ctx context.Context, name string, st Status) error {
	return m.updateSource(ctx, name, bson.D{{Key: "$set", Value: bson.D{
		{Key: "last_attempt_at", Value: st.LastAttemptAt},
		{Key: "last_synced_at", Value: st.LastSyncedAt},
		{Key: "last_error", Value: st.LastError},
	}}})
}

func (m *Mongo) updateSource(ctx context.Context, name string, update bson.D) error {
	res, err := m.sources.UpdateOne(ctx, bson.D{{Key: "name", Value: name}}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) DeleteSource(ctx context.Context, name string) error {
	res, err := m.sources.DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	// Without transactions (standalone mongo) this can leave an orphaned snapshot if it fails.
	// That is harmless: the merged list only shows snapshots of existing sources, and
	// re-creating the source replaces it on the first sync.
	_, err = m.catalog.DeleteOne(ctx, bson.D{{Key: "_id", Value: name}})
	return err
}

func (m *Mongo) GetSnapshot(ctx context.Context, source string) (*Snapshot, error) {
	var d snapshotDoc
	err := m.catalog.FindOne(ctx, bson.D{{Key: "_id", Value: source}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d.toSnapshot()
}

func (m *Mongo) ListSnapshotHeaders(ctx context.Context) ([]Snapshot, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(bson.D{{Key: "addons_json", Value: 0}})
	cur, err := m.catalog.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	var docs []snapshotDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]Snapshot, len(docs))
	for i, d := range docs {
		out[i] = d.header()
	}
	return out, nil
}

func (m *Mongo) PutSnapshot(ctx context.Context, s *Snapshot) error {
	addons := s.Addons
	if addons == nil {
		addons = []definition.Addon{}
	}
	js, err := json.Marshal(addons)
	if err != nil {
		return err
	}
	if s.Serial > 1<<63-1 {
		return fmt.Errorf("serial %d does not fit in the database", s.Serial)
	}
	d := snapshotDoc{
		ID:          s.Source,
		Serial:      int64(s.Serial),
		ContentHash: s.ContentHash,
		ETag:        s.ETag,
		GeneratedAt: s.GeneratedAt,
		UpdatedAt:   s.UpdatedAt,
		AddonCount:  len(addons),
		Addons:      string(js),
	}
	// One document per source, replaced whole, so readers never see a half-written catalog.
	_, err = m.catalog.ReplaceOne(ctx, bson.D{{Key: "_id", Value: s.Source}}, d, options.Replace().SetUpsert(true))
	return err
}

func (d *snapshotDoc) toSnapshot() (*Snapshot, error) {
	var addons []definition.Addon
	if err := json.Unmarshal([]byte(d.Addons), &addons); err != nil {
		return nil, fmt.Errorf("decoding snapshot of %q: %w", d.ID, err)
	}
	for i := range addons {
		addons[i].Normalize()
	}
	if addons == nil {
		addons = []definition.Addon{}
	}
	s := d.header()
	s.AddonCount, s.Addons = len(addons), addons
	return &s, nil
}

func (d *snapshotDoc) header() Snapshot {
	return Snapshot{
		Source:      d.ID,
		Serial:      uint64(d.Serial),
		ContentHash: d.ContentHash,
		ETag:        d.ETag,
		GeneratedAt: d.GeneratedAt,
		UpdatedAt:   d.UpdatedAt,
		AddonCount:  d.AddonCount,
	}
}
