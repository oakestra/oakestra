package updates

import (
	"context"
	"testing"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

type fakeMarket struct {
	versions map[string]string
	err      error
}

func (f *fakeMarket) Get(_ context.Context, src definition.Source, name string) (*definition.Addon, error) {
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.versions[name]
	if !ok {
		return nil, marketplaceclient.ErrNotFound
	}
	return &definition.Addon{Name: name, Source: src, Version: v}, nil
}

func TestCheckOnce(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	add := func(name string) addons.Installed {
		a, err := st.Insert(ctx, addons.Installed{
			AddonID: name, Source: definition.OfficialSource, Version: "1.0.0",
			Definition: definition.Addon{Name: name}, Status: addons.Active,
		})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a, b, c := add("a"), add("b"), add("c")
	m := &fakeMarket{versions: map[string]string{"a": "1.1.0", "b": "not-semver"}}
	ch := &Checker{Store: st, Marketplace: m}

	ch.CheckOnce(ctx)
	get := func(id string) string { x, _ := st.Get(ctx, id); return x.LatestVersion }
	if get(a.ID) != "1.1.0" || get(b.ID) != "" || get(c.ID) != "" {
		t.Errorf("latest = %q %q %q", get(a.ID), get(b.ID), get(c.ID))
	}

	// Marketplace down: the last known value stays.
	m.err = marketplaceclient.ErrUnavailable
	ch.CheckOnce(ctx)
	if get(a.ID) != "1.1.0" {
		t.Errorf("latest after outage = %q", get(a.ID))
	}

	// Disabled addons aren't checked.
	m.err = nil
	m.versions["a"] = "2.0.0"
	_, _ = st.Update(ctx, a.ID, func(x *addons.Installed) error { x.Current = false; return nil })
	ch.CheckOnce(ctx)
	if get(a.ID) != "1.1.0" {
		t.Errorf("disabled addon was checked: %q", get(a.ID))
	}
}
