// Command addonctl validates addon catalog files and builds index.json from them. The
// community-marketplace CI runs it on every pull request and on every publish.
//
//	addonctl validate [--source official|<tap>] <file>...
//	addonctl build-index --source official --serial N --out index.json [--generated-at RFC3339] <dir>
//
// Catalog files hold one definition each, in YAML (.yaml, .yml) or JSON (.json), without a
// source field. They are checked as index entries: images must be digest pinned and every
// permission the definition needs must be declared.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage:
  addonctl validate [--source official|<tap>] <file>...
  addonctl build-index --source official|<tap> --serial N --out index.json [--generated-at RFC3339] <dir>
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "build-index":
		return runBuildIndex(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceName := fs.String("source", "official", `catalog the files belong to: "official" or a tap name`)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprint(stderr, "validate: no files given\n"+usage)
		return 2
	}
	src, err := definition.SourceFromIndexName(*sourceName)
	if err != nil {
		fmt.Fprintf(stderr, "--source: %v\n", err)
		return 2
	}

	failed := 0
	for _, file := range fs.Args() {
		if _, ok := loadChecked(file, src, stderr); !ok {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "%d of %d files failed validation\n", failed, fs.NArg())
		return 1
	}
	fmt.Fprintf(stdout, "%d files valid\n", fs.NArg())
	return 0
}

func runBuildIndex(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build-index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceName := fs.String("source", "", `catalog name: "official" or a tap name (required)`)
	serialFlag := fs.String("serial", "", "index serial, must grow with every publication (required)")
	out := fs.String("out", "", "output file (required)")
	generatedAt := fs.String("generated-at", "", "timestamp to embed (RFC 3339); defaults to SOURCE_DATE_EPOCH, then now")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *sourceName == "" || *serialFlag == "" || *out == "" {
		fmt.Fprint(stderr, "build-index: --source, --serial, --out and exactly one directory are required\n"+usage)
		return 2
	}
	src, err := definition.SourceFromIndexName(*sourceName)
	if err != nil {
		fmt.Fprintf(stderr, "--source: %v\n", err)
		return 2
	}
	serial, err := strconv.ParseUint(*serialFlag, 10, 64)
	if err != nil || serial == 0 {
		fmt.Fprintf(stderr, "--serial: %q is not a positive integer\n", *serialFlag)
		return 2
	}
	at, err := timestamp(*generatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "--generated-at: %v\n", err)
		return 2
	}

	files, err := catalogFiles(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintf(stderr, "no definitions found in %s\n", fs.Arg(0))
		return 1
	}

	var addons []definition.Addon
	failed := false
	for _, file := range files {
		a, ok := loadChecked(file, src, stderr)
		if !ok {
			failed = true
			continue
		}
		// One file per addon, named after it, keeps reviews and git history per addon.
		if base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)); base != a.Name {
			fmt.Fprintf(stderr, "%s: file name must be %s%s to match the addon name\n", file, a.Name, filepath.Ext(file))
			failed = true
			continue
		}
		addons = append(addons, *a)
	}
	if failed {
		return 1
	}

	idx := definition.NewIndex(*sourceName, serial, at, addons)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encoding index: %v\n", err)
		return 1
	}
	data = append(data, '\n')

	// Read the output back the way the marketplace will, so we never publish an index that
	// readers reject (e.g. one over the size limit).
	decoded, err := definition.DecodeIndex(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(stderr, "generated index doesn't decode: %v\n", err)
		return 1
	}
	if errs := definition.ValidateIndex(decoded); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(stderr, "generated index: %v\n", e)
		}
		return 1
	}

	if err := writeFileAtomic(*out, data); err != nil {
		fmt.Fprintf(stderr, "writing %s: %v\n", *out, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s: %d addons, serial %d\n", *out, len(addons), serial)
	return 0
}

// loadChecked reads, decodes and checks one catalog file, printing every problem.
func loadChecked(file string, src definition.Source, stderr io.Writer) (*definition.Addon, bool) {
	a, err := loadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", file, err)
		return nil, false
	}
	if a.Source != (definition.Source{}) {
		fmt.Fprintf(stderr, "%s: source: must not be set in catalog files, the index sets it\n", file)
		return nil, false
	}
	a.Source = src
	err = definition.Check(a, definition.ModeIndex)
	var errs definition.Errors
	if errors.As(err, &errs) {
		for _, e := range errs {
			fmt.Fprintf(stderr, "%s: %v\n", file, e)
		}
		return nil, false
	}
	return a, true
}

func loadFile(file string) (*definition.Addon, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(file)) {
	case ".json":
		return definition.DecodeAddon(bytes.NewReader(data))
	case ".yaml", ".yml":
		return definition.UnmarshalAddonYAML(data)
	default:
		return nil, errors.New("unsupported file type, use .yaml, .yml or .json")
	}
}

// catalogFiles lists the definitions of a catalog. dir may be the catalog root (with an
// addons/ subdirectory) or the addons directory itself.
func catalogFiles(dir string) ([]string, error) {
	if st, err := os.Stat(filepath.Join(dir, "addons")); err == nil && st.IsDir() {
		dir = filepath.Join(dir, "addons")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			if !e.IsDir() {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	slices.Sort(files)
	return files, nil
}

func timestamp(flagValue string) (time.Time, error) {
	if flagValue != "" {
		return time.Parse(time.RFC3339, flagValue)
	}
	// SOURCE_DATE_EPOCH is the reproducible-builds convention for pinning build timestamps.
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		secs, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("SOURCE_DATE_EPOCH: %w", err)
		}
		return time.Unix(secs, 0).UTC(), nil
	}
	return time.Now().UTC().Truncate(time.Second), nil
}

// writeFileAtomic avoids leaving a half-written index behind for a publish step to pick up.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".index-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
