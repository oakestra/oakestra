package definition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDisplayFields(t *testing.T) {
	manyMetadata := map[string]string{}
	for i := range maxMetadata + 1 {
		manyMetadata[fmt.Sprintf("k%d", i)] = "v"
	}
	tests := []struct {
		name   string
		mutate func(a *Addon)
		want   []string
	}{
		{
			name: "all set",
			mutate: func(a *Addon) {
				a.DisplayName = "Node Metrics für Prometheus"
				a.Category = "monitoring"
				a.Icon = "https://cdn.example.com/icons/node-metrics.svg?v=2"
				a.Metadata = map[string]string{"example.com/catalog-id": "1234", "featured": "true", "x": strings.Repeat("v", maxMetadataValue)}
			},
		},
		{name: "display name at the limit", mutate: func(a *Addon) { a.DisplayName = strings.Repeat("é", maxDisplayName) }},
		{name: "display name too long", mutate: func(a *Addon) { a.DisplayName = strings.Repeat("a", maxDisplayName+1) }, want: []string{"display_name"}},
		{name: "display name untrimmed", mutate: func(a *Addon) { a.DisplayName = " Metrics" }, want: []string{"display_name"}},
		{name: "display name blank", mutate: func(a *Addon) { a.DisplayName = "   " }, want: []string{"display_name"}},
		{name: "display name newline", mutate: func(a *Addon) { a.DisplayName = "Metrics\nOfficial" }, want: []string{"display_name"}},
		{name: "display name tab", mutate: func(a *Addon) { a.DisplayName = "Metrics\tx" }, want: []string{"display_name"}},
		{name: "display name bidi override", mutate: func(a *Addon) { a.DisplayName = "Metrics \u202egpj" }, want: []string{"display_name"}},

		{name: "category with space and dash", mutate: func(a *Addon) { a.Category = "machine-learning tools" }},
		{name: "category at the limit", mutate: func(a *Addon) { a.Category = strings.Repeat("c", maxCategory) }},
		{name: "category too long", mutate: func(a *Addon) { a.Category = strings.Repeat("c", maxCategory+1) }, want: []string{"category"}},
		{name: "category not normalized", mutate: func(a *Addon) { a.Category = "AI" }, want: []string{"category"}},
		{name: "category with slash", mutate: func(a *Addon) { a.Category = "ai/ml" }, want: []string{"category"}},
		{name: "category with underscore", mutate: func(a *Addon) { a.Category = "ai_ml" }, want: []string{"category"}},
		{name: "category non-ascii", mutate: func(a *Addon) { a.Category = "überwachung" }, want: []string{"category"}},
		{name: "category ending in dash", mutate: func(a *Addon) { a.Category = "ai-" }, want: []string{"category"}},

		{name: "tags", mutate: func(a *Addon) { a.Tags = []string{"gpu", "llm", "prometheus-exporter", "x1"} }},
		{name: "tag at the limit", mutate: func(a *Addon) { a.Tags = []string{strings.Repeat("t", maxTag)} }},
		{name: "tag too long", mutate: func(a *Addon) { a.Tags = []string{strings.Repeat("t", maxTag+1)} }, want: []string{"tags[0]"}},
		{name: "too many tags", mutate: func(a *Addon) { a.Tags = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} }, want: []string{"tags"}},
		{name: "empty tag", mutate: func(a *Addon) { a.Tags = []string{"", "ok"} }, want: []string{"tags[0]"}},
		{
			name:   "bad tags",
			mutate: func(a *Addon) { a.Tags = []string{"GPU", "two words", "a_b", "-x", "ü"} },
			want:   []string{"tags[0]", "tags[1]", "tags[2]", "tags[3]", "tags[4]"},
		},

		{name: "icon http", mutate: func(a *Addon) { a.Icon = "http://example.com/i.png" }, want: []string{"icon"}},
		{name: "icon data url", mutate: func(a *Addon) { a.Icon = "data:image/svg+xml;base64,PHN2Zz4=" }, want: []string{"icon"}},
		{name: "icon javascript", mutate: func(a *Addon) { a.Icon = "javascript:alert(1)" }, want: []string{"icon"}},
		{name: "icon userinfo", mutate: func(a *Addon) { a.Icon = "https://cdn.example.com@evil.example/i.png" }, want: []string{"icon"}},
		{name: "icon relative", mutate: func(a *Addon) { a.Icon = "/icons/i.png" }, want: []string{"icon"}},
		{name: "icon scheme relative", mutate: func(a *Addon) { a.Icon = "//cdn.example.com/i.png" }, want: []string{"icon"}},
		{name: "icon opaque", mutate: func(a *Addon) { a.Icon = "https:cdn.example.com/i.png" }, want: []string{"icon"}},
		{name: "icon without host", mutate: func(a *Addon) { a.Icon = "https:///i.png" }, want: []string{"icon"}},
		{name: "icon with whitespace", mutate: func(a *Addon) { a.Icon = "https://cdn.example.com/a b.png" }, want: []string{"icon"}},
		{name: "icon too long", mutate: func(a *Addon) { a.Icon = "https://cdn.example.com/" + strings.Repeat("a", maxIcon) }, want: []string{"icon"}},

		{name: "too many metadata entries", mutate: func(a *Addon) { a.Metadata = manyMetadata }, want: []string{"metadata"}},
		{
			name: "bad metadata keys",
			mutate: func(a *Addon) {
				a.Metadata = map[string]string{
					"Upper": "", "-lead": "", "trail.": "", "under_score": "", "": "",
					strings.Repeat("k", maxMetadataKey+1): "",
				}
			},
			want: []string{
				`metadata[""]`, `metadata["-lead"]`, `metadata["Upper"]`,
				`metadata["` + strings.Repeat("k", maxMetadataKey+1) + `"]`,
				`metadata["trail."]`, `metadata["under_score"]`,
			},
		},
		{name: "metadata key at the limit", mutate: func(a *Addon) { a.Metadata = map[string]string{strings.Repeat("k", maxMetadataKey): ""} }},
		{
			name:   "metadata value too long",
			mutate: func(a *Addon) { a.Metadata = map[string]string{"x": strings.Repeat("v", maxMetadataValue+1)} },
			want:   []string{`metadata["x"]`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAddon()
			tt.mutate(a)
			errs := Validate(a, ModeIndex)
			if got := errorPaths(errs); strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("Validate:\n got %q\nwant %q\n%v", got, tt.want, errs)
			}
		})
	}
}

func TestNormalizeDisplayFields(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		"   ":                    "",
		"AI":                     "ai",
		"  Machine   Learning ":  "machine learning",
		"Monitoring\t& Alerts":   "monitoring & alerts",
		"already-normalized one": "already-normalized one",
	} {
		if got := NormalizeCategory(in); got != want {
			t.Errorf("NormalizeCategory(%q) = %q, want %q", in, got, want)
		}
		if again := NormalizeCategory(want); again != want {
			t.Errorf("NormalizeCategory is not idempotent on %q: %q", want, again)
		}
	}

	for _, tt := range []struct{ in, want []string }{
		{nil, nil},
		{[]string{}, nil},
		{[]string{"LLM", " gpu ", "llm", "ai"}, []string{"ai", "gpu", "llm"}},
		{[]string{"  ", "x", ""}, []string{"", "x"}},
	} {
		got := normalizeTags(tt.in)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") || (got == nil) != (tt.want == nil) {
			t.Errorf("normalizeTags(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	a := baseAddon()
	a.DisplayName = "  Node Metrics \n"
	a.Category = " Observability "
	a.Metadata = map[string]string{}
	a.Normalize()
	if a.DisplayName != "Node Metrics" || a.Category != "observability" || a.Metadata != nil {
		t.Errorf("Normalize: display_name %q category %q metadata %#v", a.DisplayName, a.Category, a.Metadata)
	}
}

func TestDecodeDisplayFields(t *testing.T) {
	img := `"image": "ghcr.io/x/web@` + testDigest + `"`
	doc := `{"name": "a", "version": "1.0.0", "display_name": "A", "category": "Monitoring",
		"tags": ["GPU", "llm", "gpu"], "icon": "https://cdn.example.com/a.svg", "metadata": {"example.com/id": "7"},
		"services": [{"service_name": "web", ` + img + `}]}`
	a, err := DecodeAddon(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("DecodeAddon: %v", err)
	}
	if a.DisplayName != "A" || a.Category != "monitoring" || strings.Join(a.Tags, ",") != "gpu,llm" || a.Icon == "" || a.Metadata["example.com/id"] != "7" {
		t.Errorf("decoded %+v", a)
	}

	for name, in := range map[string]string{
		"miscased display_name":     strings.Replace(doc, `"display_name"`, `"Display_Name"`, 1),
		"camel case display name":   strings.Replace(doc, `"display_name"`, `"displayName"`, 1),
		"duplicate category":        strings.Replace(doc, `"category": "Monitoring",`, `"category": "monitoring", "category": "ai",`, 1),
		"tags as a string":          strings.Replace(doc, `"category"`, `"tags": "gpu", "category"`, 1),
		"duplicate metadata key":    strings.Replace(doc, `{"example.com/id": "7"}`, `{"x": "1", "x": "2"}`, 1),
		"unknown sibling field":     strings.Replace(doc, `"category"`, `"keywords": ["x"], "category"`, 1),
		"category list":             strings.Replace(doc, `"category": "Monitoring"`, `"category": ["monitoring"]`, 1),
		"non-string metadata value": strings.Replace(doc, `"7"`, `7`, 1),
		"nested metadata objects":   strings.Replace(doc, `"7"`, `{"a": "b"}`, 1),
	} {
		if _, err := DecodeAddon(strings.NewReader(in)); err == nil {
			t.Errorf("%s: DecodeAddon succeeded", name)
		}
	}

	yamlDoc := "name: a\nversion: 1.0.0\ndisplay_name: Node Metrics\ncategory: Monitoring\ntags: [Prometheus, exporter]\n" +
		"icon: https://cdn.example.com/a.svg\nmetadata:\n  example.com/id: \"7\"\n" +
		"services:\n  - service_name: web\n    image: ghcr.io/x/web@" + testDigest + "\n"
	y, err := UnmarshalAddonYAML([]byte(yamlDoc))
	if err != nil {
		t.Fatalf("UnmarshalAddonYAML: %v", err)
	}
	if y.Category != "monitoring" || strings.Join(y.Tags, ",") != "exporter,prometheus" || y.Metadata["example.com/id"] != "7" {
		t.Errorf("YAML decoded %+v", y)
	}
	y.Source = OfficialSource
	if err := Check(y, ModeIndex); err != nil {
		t.Errorf("Check(YAML): %v", err)
	}
	for name, in := range map[string]string{
		"unquoted number metadata value": strings.Replace(yamlDoc, `"7"`, `7`, 1),
		"miscased key":                   strings.Replace(yamlDoc, "category:", "Category:", 1),
	} {
		if _, err := UnmarshalAddonYAML([]byte(in)); err == nil {
			t.Errorf("YAML %s: UnmarshalAddonYAML succeeded", name)
		}
	}

	// An index entry using the new fields still decodes and validates.
	idx := strings.Replace(validIndex, `"version": "1.3.0",`, `"version": "1.3.0", "display_name": "Monitoring", "category": "Observability", "metadata": {"featured": "yes"},`, 1)
	parsed, err := DecodeIndex(strings.NewReader(idx))
	if err != nil {
		t.Fatalf("DecodeIndex: %v", err)
	}
	if errs := ValidateIndex(parsed); len(errs) != 0 {
		t.Errorf("ValidateIndex: %v", errs)
	}
	if parsed.Addons[0].Category != "observability" {
		t.Errorf("index category not normalized: %q", parsed.Addons[0].Category)
	}
}

func TestDisplayFieldsHash(t *testing.T) {
	withMeta := func() *Addon {
		a := baseAddon()
		a.DisplayName = "Monitoring"
		a.Category = "observability"
		a.Icon = "https://cdn.example.com/m.svg"
		a.Metadata = map[string]string{"b": "2", "a": "1", "example.com/id": "x"}
		a.Tags = []string{"llm", "gpu"}
		return a
	}

	t.Run("unset metadata adds nothing to the canonical JSON", func(t *testing.T) {
		// Definitions hashed before these fields existed must keep their hash.
		a := baseAddon()
		a.Metadata = map[string]string{}
		a.Tags = []string{}
		c := CanonicalJSON(*a)
		for _, k := range []string{"display_name", "category", "tags", "icon", "metadata"} {
			if bytes.Contains(c, []byte(`"`+k+`"`)) {
				t.Errorf("canonical JSON contains %q: %s", k, c)
			}
		}
		if a.Hash() != baseAddon().Hash() {
			t.Error("an empty metadata map changes the hash")
		}
	})
	t.Run("metadata key order is irrelevant", func(t *testing.T) {
		want := withMeta().Hash()
		for range 20 {
			if withMeta().Hash() != want {
				t.Fatal("hash depends on map iteration order")
			}
		}
		var a Addon
		raw := CanonicalJSON(*withMeta())
		raw = bytes.Replace(raw, []byte(`"a":"1","b":"2"`), []byte(`"b":"2","a":"1"`), 1)
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		if a.Hash() != want {
			t.Error("reordered metadata keys change the hash")
		}
	})
	t.Run("category case and tag order are irrelevant", func(t *testing.T) {
		a := withMeta()
		a.Category = "Observability"
		a.Tags = []string{"GPU", "llm", "gpu"}
		if a.Hash() != withMeta().Hash() {
			t.Error("category case or tag order changes the hash")
		}
	})
	t.Run("display fields and metadata are covered", func(t *testing.T) {
		base := withMeta().Hash()
		for name, mutate := range map[string]func(*Addon){
			"display name": func(a *Addon) { a.DisplayName = "Other" },
			"category":     func(a *Addon) { a.Category = "ai" },
			"icon":         func(a *Addon) { a.Icon = "https://evil.example/m.svg" },
			"tags":         func(a *Addon) { a.Tags = append(a.Tags, "new") },
			"metadata":     func(a *Addon) { a.Metadata["a"] = "changed" },
		} {
			a := withMeta()
			mutate(a)
			if a.Hash() == base {
				t.Errorf("changing the %s does not change the hash", name)
			}
		}
	})
}
