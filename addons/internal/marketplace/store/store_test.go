package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/marketplace/mptest"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

func TestMemory(t *testing.T) {
	testStore(t, store.NewMemory())
}

// TestMongo runs the same checks against a real database when MARKETPLACE_TEST_MONGO_URI is
// set, e.g. mongodb://localhost:27017.
func TestMongo(t *testing.T) {
	uri := os.Getenv("MARKETPLACE_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("MARKETPLACE_TEST_MONGO_URI not set")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db := client.Database(fmt.Sprintf("marketplace_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	m := store.NewMongo(db)
	if err := m.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	testStore(t, m)
}

func testStore(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := s.GetSource(ctx, "mytap"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing source: %v", err)
	}
	tap := &store.Source{Name: "mytap", Kind: definition.SourceTap, URL: "https://tap.test/", Token: "tok", AllowPrivate: true, CreatedAt: now}
	if err := s.CreateSource(ctx, tap); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSource(ctx, tap); !errors.Is(err, store.ErrExists) {
		t.Errorf("duplicate create: %v", err)
	}
	if err := s.CreateSource(ctx, &store.Source{Name: "official", Kind: definition.SourceOfficial, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	st := store.Status{LastAttemptAt: now, LastError: "boom"}
	if err := s.SetStatus(ctx, "mytap", st); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSourceConfig(ctx, &store.Source{Name: "mytap", Kind: definition.SourceTap, URL: "https://tap2.test/"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSource(ctx, "mytap")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://tap2.test/" || got.Token != "" || got.AllowPrivate || got.Status.LastError != "boom" || !got.Status.LastAttemptAt.Equal(now) {
		t.Errorf("after update: %+v", got)
	}

	list, err := s.ListSources(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "mytap" || list[1].Name != "official" {
		t.Errorf("list: %+v %v", list, err)
	}

	a := mptest.Addon("web", "1.0.0")
	a.Source = definition.TapSource("mytap")
	// Keys BSON dislikes must survive the round trip.
	a.Services[0].Labels = map[string]string{"com.example.team": "x", "$weird": "y"}
	a.Services[0].Environment = map[string]string{"A.B": "1"}
	snap := &store.Snapshot{Source: "mytap", Serial: 7, ContentHash: "sha256:x", ETag: `"e"`, GeneratedAt: now, UpdatedAt: now, Addons: []definition.Addon{a}}
	if err := s.PutSnapshot(ctx, snap); err != nil {
		t.Fatal(err)
	}
	snap.Addons[0].Name = "mutated"
	gotSnap, err := s.GetSnapshot(ctx, "mytap")
	if err != nil {
		t.Fatal(err)
	}
	if gotSnap.Serial != 7 || gotSnap.ETag != `"e"` || len(gotSnap.Addons) != 1 || gotSnap.Addons[0].Name != "web" ||
		gotSnap.Addons[0].Services[0].Labels["com.example.team"] != "x" || gotSnap.Addons[0].Source != a.Source {
		t.Errorf("snapshot round trip: %+v", gotSnap)
	}
	snaps, err := s.ListSnapshotHeaders(ctx)
	if err != nil || len(snaps) != 1 || snaps[0].AddonCount != 1 || snaps[0].Addons != nil || snaps[0].Serial != 7 {
		t.Errorf("list snapshot headers: %+v %v", snaps, err)
	}

	if err := s.DeleteSource(ctx, "mytap"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSnapshot(ctx, "mytap"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("snapshot survived delete: %v", err)
	}
	if err := s.DeleteSource(ctx, "mytap"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if err := s.SetStatus(ctx, "mytap", st); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("status of deleted source: %v", err)
	}
}
