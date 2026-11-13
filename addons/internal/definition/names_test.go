package definition

import (
	"strings"
	"testing"
)

func TestValidateSegment(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"a", true},
		{"monitoring", true},
		{"node-exporter", true},
		{"a1", true},
		{strings.Repeat("a", 32), true},
		{strings.Repeat("a", 33), false},
		{"", false},
		{"-a", false},
		{"a-", false},
		{"Monitoring", false},
		{"a--b", false},
		{"a---b", false},
		{"a_b", false},
		{"a.b", false},
		{"a/b", false},
		{"ä", false},
	}
	for _, tt := range tests {
		if err := ValidateSegment(tt.in); (err == nil) != tt.ok {
			t.Errorf("ValidateSegment(%q) = %v, want ok=%v", tt.in, err, tt.ok)
		}
	}
}

func TestParseQualified(t *testing.T) {
	tests := []struct {
		in      string
		src     Source
		name    string
		wantErr bool
	}{
		{in: "monitoring", src: OfficialSource, name: "monitoring"},
		{in: "oakestra/monitoring", src: OfficialSource, name: "monitoring"},
		{in: "official/monitoring", src: OfficialSource, name: "monitoring"},
		{in: "local/thing", src: LocalSource, name: "thing"},
		{in: "acme/thing", src: TapSource("acme"), name: "thing"},
		{in: "acme/Thing", wantErr: true},
		{in: "Acme/thing", wantErr: true},
		{in: "a--b/thing", wantErr: true},
		{in: "acme/a--b", wantErr: true},
		{in: "acme/thing/extra", wantErr: true},
		{in: "", wantErr: true},
		{in: "/thing", wantErr: true},
	}
	for _, tt := range tests {
		src, name, err := ParseQualified(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseQualified(%q) = %v %q, want error", tt.in, src, name)
			}
			continue
		}
		if err != nil || src != tt.src || name != tt.name {
			t.Errorf("ParseQualified(%q) = %v %q %v, want %v %q", tt.in, src, name, err, tt.src, tt.name)
		}
	}
}

func TestNamesRoundTrip(t *testing.T) {
	tests := []struct {
		src       Source
		name      string
		qualified string
		id        string
	}{
		{OfficialSource, "monitoring", "monitoring", "monitoring"},
		{LocalSource, "thing", "local/thing", "local--thing"},
		{TapSource("acme"), "thing", "acme/thing", "acme--thing"},
	}
	for _, tt := range tests {
		if got := QualifiedName(tt.src, tt.name); got != tt.qualified {
			t.Errorf("QualifiedName = %q, want %q", got, tt.qualified)
		}
		if got := ID(tt.src, tt.name); got != tt.id {
			t.Errorf("ID = %q, want %q", got, tt.id)
		}
		src, name, err := ParseID(tt.id)
		if err != nil || src != tt.src || name != tt.name {
			t.Errorf("ParseID(%q) = %v %q %v", tt.id, src, name, err)
		}
		src, name, err = ParseQualified(tt.qualified)
		if err != nil || src != tt.src || name != tt.name {
			t.Errorf("ParseQualified(%q) = %v %q %v", tt.qualified, src, name, err)
		}
	}
}

func TestParseIDRejects(t *testing.T) {
	for _, id := range []string{"", "official--x", "oakestra--x", "acme--", "--x", "acme--x--y", "Acme--x"} {
		if _, _, err := ParseID(id); err == nil {
			t.Errorf("ParseID(%q) succeeded", id)
		}
	}
}

func TestTapNamesReserved(t *testing.T) {
	for _, n := range []string{"local", "official", "oakestra"} {
		if err := ValidateTapName(n); err == nil {
			t.Errorf("tap name %q accepted", n)
		}
		if _, err := SourceFromIndexName(n); n != "official" && err == nil {
			t.Errorf("index source %q accepted", n)
		}
	}
	if err := ValidateSource(Source{Kind: SourceTap, Name: "local"}); err == nil {
		t.Error("tap named local accepted")
	}
	if err := ValidateSource(Source{Kind: SourceOfficial, Name: "acme"}); err == nil {
		t.Error("official source with another name accepted")
	}
}

func TestContainerNamesDontCollide(t *testing.T) {
	// With a single hyphen these two would both be "addon-a-b-c".
	a := ContainerName(ID(OfficialSource, "a"), "b-c")
	b := ContainerName(ID(OfficialSource, "a-b"), "c")
	if a == b {
		t.Fatalf("container names collide: %q", a)
	}
	if got := ContainerName("acme--thing", "web"); got != "addon-acme--thing--web" {
		t.Errorf("ContainerName = %q", got)
	}
	if got := UIAlias("acme--thing"); got != "addonui-acme--thing" {
		t.Errorf("UIAlias = %q", got)
	}
	if got := VolumeName("acme--thing", "data"); got != "addon-acme--thing_data" {
		t.Errorf("VolumeName = %q", got)
	}
}
