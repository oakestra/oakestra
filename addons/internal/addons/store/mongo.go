package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

const (
	// Existing deployments may have documents of a different shape in the "addons"
	// collection. Using a collection of our own means they are ignored, not misread.
	DatabaseName   = "addons"
	CollectionName = "installed"

	maxUpdateRetries = 10
)

// Mongo is the MongoDB Store.
type Mongo struct {
	coll *mongo.Collection
}

// mongoDoc is the stored shape. Definition and permissions have no bson tags, so the client
// must be created with UseJSONStructTags (see Connect).
type mongoDoc struct {
	ID            bson.ObjectID          `bson:"_id,omitempty"`
	AddonID       string                 `bson:"addon_id"`
	QualifiedName string                 `bson:"qualified_name"`
	Source        definition.Source      `bson:"source"`
	Version       string                 `bson:"version"`
	Definition    definition.Addon       `bson:"definition"`
	Granted       definition.Permissions `bson:"granted"`
	Signature     string                 `bson:"grant_signature"`
	Status        addons.Status          `bson:"status"`
	StatusDetails addons.StatusDetails   `bson:"status_details"`
	Logs          string                 `bson:"logs"`
	LatestVersion string                 `bson:"latest_version"`
	Current       bool                   `bson:"current"`
	// Rev is bumped on every write, so Update can detect a concurrent writer.
	Rev       int64     `bson:"rev"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (d mongoDoc) toInstalled() addons.Installed {
	a := addons.Installed{
		ID: d.ID.Hex(), AddonID: d.AddonID, QualifiedName: d.QualifiedName, Source: d.Source,
		Version: d.Version, Definition: d.Definition, Granted: d.Granted, Signature: d.Signature, Status: d.Status,
		StatusDetails: d.StatusDetails, Logs: d.Logs, LatestVersion: d.LatestVersion,
		Current: d.Current, CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
	a.Definition.Normalize()
	a.Granted = a.Granted.Canonical()
	return a
}

func fromInstalled(a addons.Installed) mongoDoc {
	return mongoDoc{
		AddonID: a.AddonID, QualifiedName: a.QualifiedName, Source: a.Source, Version: a.Version,
		Definition: a.Definition, Granted: a.Granted, Signature: a.Signature, Status: a.Status,
		StatusDetails: a.StatusDetails, Logs: a.Logs, LatestVersion: a.LatestVersion,
		Current: a.Current, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// Connect opens a client for uri. The driver connects lazily, so this doesn't fail while Mongo
// is still starting.
func Connect(uri string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri).
		SetServerSelectionTimeout(5 * time.Second).
		SetBSONOptions(&options.BSONOptions{UseJSONStructTags: true})
	return mongo.Connect(opts)
}

// NewMongo returns a store using the "installed" collection of db "addons". The client must
// come from Connect.
func NewMongo(client *mongo.Client) *Mongo {
	return &Mongo{coll: client.Database(DatabaseName).Collection(CollectionName)}
}

// EnsureIndexes creates the unique index that keeps one current installation per addon.
// Mongo can't express "status != disabled" in a partial index, hence the Current flag that is
// cleared when an addon is disabled. Retry it until Mongo is reachable.
func (m *Mongo) EnsureIndexes(ctx context.Context) error {
	_, err := m.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "addon_id", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.D{{Key: "current", Value: true}}),
	})
	return err
}

func objectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, ErrNotFound
	}
	return oid, nil
}

func (m *Mongo) List(ctx context.Context, status addons.Status) ([]addons.Installed, error) {
	filter := bson.D{}
	if status != "" {
		filter = bson.D{{Key: "status", Value: string(status)}}
	}
	cur, err := m.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []mongoDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]addons.Installed, len(docs))
	for i, d := range docs {
		out[i] = d.toInstalled()
	}
	return out, nil
}

func (m *Mongo) Get(ctx context.Context, id string) (addons.Installed, error) {
	d, err := m.get(ctx, id)
	if err != nil {
		return addons.Installed{}, err
	}
	return d.toInstalled(), nil
}

func (m *Mongo) get(ctx context.Context, id string) (mongoDoc, error) {
	oid, err := objectID(id)
	if err != nil {
		return mongoDoc{}, err
	}
	var d mongoDoc
	err = m.coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return mongoDoc{}, ErrNotFound
	}
	return d, err
}

// Insert relies on the unique partial index for the duplicate check, so concurrent installs
// of the same addon can't both succeed as long as EnsureIndexes has run.
func (m *Mongo) Insert(ctx context.Context, a addons.Installed) (addons.Installed, error) {
	a.Current = true
	d := fromInstalled(a)
	d.ID = bson.NewObjectID()
	if a.ID != "" {
		oid, err := bson.ObjectIDFromHex(a.ID)
		if err != nil {
			return addons.Installed{}, fmt.Errorf("invalid preset id %q: %w", a.ID, err)
		}
		d.ID = oid
	}
	if _, err := m.coll.InsertOne(ctx, d); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return addons.Installed{}, ErrDuplicate
		}
		return addons.Installed{}, err
	}
	return d.toInstalled(), nil
}

func (m *Mongo) Update(ctx context.Context, id string, fn func(*addons.Installed) error) (addons.Installed, error) {
	for range maxUpdateRetries {
		cur, err := m.get(ctx, id)
		if err != nil {
			return addons.Installed{}, err
		}
		work := cur.toInstalled()
		if err := fn(&work); err != nil {
			if errors.Is(err, ErrAbort) {
				return cur.toInstalled(), nil
			}
			return addons.Installed{}, err
		}
		next := fromInstalled(work)
		next.ID = cur.ID
		next.Rev = cur.Rev + 1
		res, err := m.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: cur.ID}, {Key: "rev", Value: cur.Rev}}, next)
		if err != nil {
			return addons.Installed{}, err
		}
		if res.MatchedCount == 1 {
			return next.toInstalled(), nil
		}
	}
	return addons.Installed{}, fmt.Errorf("addon %s changed concurrently, giving up after %d attempts", id, maxUpdateRetries)
}

func (m *Mongo) Delete(ctx context.Context, id string) error {
	oid, err := objectID(id)
	if err != nil {
		return err
	}
	res, err := m.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
