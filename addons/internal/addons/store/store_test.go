package store

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/grant"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

func sample() addons.Installed {
	def := definition.Addon{
		Name: "tool", Source: definition.TapSource("acme"), Version: "1.0.0",
		UI:          &definition.UI{Service: "web", SDK: "^1"},
		Permissions: definition.Permissions{HostPaths: []string{"/var/log:ro"}, HostPorts: []int{8080}},
		Services: []definition.Service{{
			ServiceName: "web", Image: "x@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Ports: map[string]string{"80": "8080"}, Labels: map[string]string{"a": "b"},
		}},
	}
	def.Normalize()
	return addons.Installed{
		AddonID: def.ID(), QualifiedName: def.QualifiedName(), Source: def.Source, Version: def.Version,
		Definition: def, Granted: def.Permissions, Status: addons.Installing,
	}
}

func TestMemoryRules(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	a, err := m.Insert(ctx, sample())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Insert(ctx, sample()); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate insert: %v", err)
	}
	if _, err := m.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get unknown: %v", err)
	}

	// Disabling frees the addon id for a new installation.
	if _, err := m.Update(ctx, a.ID, func(x *addons.Installed) error { x.Current = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Insert(ctx, sample()); err != nil {
		t.Fatalf("reinstall: %v", err)
	}

	got, err := m.Update(ctx, a.ID, func(x *addons.Installed) error { x.Version = "x"; return ErrAbort })
	if err != nil || got.Version == "x" {
		t.Fatalf("abort wrote: %+v %v", got, err)
	}

	list, _ := m.List(ctx, addons.Installing)
	list[0].Definition.Services[0].Labels["a"] = "mutated"
	again, _ := m.Get(ctx, list[0].ID)
	if again.Definition.Services[0].Labels["a"] != "b" {
		t.Error("callers can mutate stored data")
	}

	if err := m.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

// The Mongo store relies on the client's UseJSONStructTags option to store definitions. This
// checks that the stored shape survives a BSON round trip with that option, without a server.
func TestMongoDocRoundTrip(t *testing.T) {
	in := sample()
	in.ID = NewID()
	key, _ := grant.NewKey(bytes.Repeat([]byte{1}, grant.KeySize))
	in.Signature = key.Sign(in, "root")
	doc := fromInstalled(in)
	doc.ID, _ = bson.ObjectIDFromHex(in.ID)

	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.UseJSONStructTags()
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}

	raw := bson.Raw(buf.Bytes())
	if _, err := raw.LookupErr("definition", "services"); err != nil {
		t.Fatalf("definition not stored with json field names: %v", err)
	}
	if v := raw.Lookup("addon_id").StringValue(); v != "acme--tool" {
		t.Errorf("addon_id = %v", v)
	}

	var out mongoDoc
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(buf.Bytes())))
	dec.UseJSONStructTags()
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	got := out.toInstalled()
	if got.ID != in.ID || got.Definition.Services[0].Ports["80"] != "8080" || got.Definition.UI.SDK != "^1" ||
		got.Definition.Source != definition.TapSource("acme") || got.Granted.HostPorts[0] != 8080 {
		t.Errorf("round trip lost data: %+v", got)
	}
	if got.Definition.Permissions.Hash() != in.Definition.Permissions.Hash() {
		t.Error("permissions hash changed")
	}
	// The signature is over a canonical encoding, so a BSON round trip must not break it.
	if err := key.Verify(got, "root"); err != nil {
		t.Errorf("signature after round trip: %v", err)
	}
}
