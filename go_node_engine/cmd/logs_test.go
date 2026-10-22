package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrintLastLines(t *testing.T) {
	cases := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{"last two", "a\nb\nc\n", 2, "b\nc\n"},
		{"fewer lines than asked", "a\nb\n", 5, "a\nb\n"},
		{"no trailing newline", "a\nb\nc", 1, "c"},
		{"zero shows nothing", "a\nb\n", 0, ""},
		{"empty file", "", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var out bytes.Buffer
			offset, err := printLastLines(&out, f, tc.n)
			if err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Errorf("got %q; want %q", out.String(), tc.want)
			}
			if offset != int64(len(tc.content)) {
				t.Errorf("offset %d; want %d (end of file)", offset, len(tc.content))
			}
		})
	}
}
