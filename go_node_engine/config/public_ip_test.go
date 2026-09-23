package config

import (
	"encoding/json"
	"testing"
)

func TestParsePublicIPMode(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   PublicIPMode
		isAuto bool
	}{
		{name: "false", in: "false", want: PublicIPFalse},
		{name: "auto", in: "auto", want: PublicIPAuto, isAuto: true},
		{name: "legacy true", in: "true", want: PublicIPAuto, isAuto: true},
		{name: "custom ip", in: "1.2.3.4", want: PublicIPMode("1.2.3.4")},
		{name: "trimmed false", in: " false ", want: PublicIPFalse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePublicIPMode(tt.in)
			if got != tt.want {
				t.Fatalf("ParsePublicIPMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if got.IsAuto() != tt.isAuto {
				t.Fatalf("IsAuto mismatch for %q", tt.in)
			}
		})
	}
}

func TestPublicIPModeUnmarshalCompatibility(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want PublicIPMode
	}{
		{name: "bool true", raw: `{"public_ip":true}`, want: PublicIPAuto},
		{name: "bool false", raw: `{"public_ip":false}`, want: PublicIPFalse},
		{name: "string auto", raw: `{"public_ip":"auto"}`, want: PublicIPAuto},
		{name: "string custom", raw: `{"public_ip":"203.0.113.9"}`, want: PublicIPMode("203.0.113.9")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ConfFile
			if err := json.Unmarshal([]byte(tt.raw), &cfg); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if cfg.PublicIp != tt.want {
				t.Fatalf("PublicIp = %q, want %q", cfg.PublicIp, tt.want)
			}
		})
	}
}
