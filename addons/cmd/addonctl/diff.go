package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

// runDiff compares two versions of a catalog, typically the base and head of a pull request.
// It prints a Markdown summary for reviewers to stdout and fails on changes that installed
// copies couldn't follow: a definition that changes without a higher version is never offered
// as an update, and its approve stops working because the catalog no longer serves what was
// installed.
func runDiff(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sourceName := flags.String("source", "official", `catalog the files belong to: "official" or a tap name`)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 2 {
		fmt.Fprint(stderr, "diff: a base and a head directory are required\n"+usage)
		return 2
	}
	src, err := definition.SourceFromIndexName(*sourceName)
	if err != nil {
		fmt.Fprintf(stderr, "--source: %v\n", err)
		return 2
	}

	base, err := loadBase(flags.Arg(0), src, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	headFiles, err := catalogFiles(flags.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	headList, ok := loadCatalog(headFiles, src, stderr)
	if !ok {
		return 1
	}
	head := map[string]definition.Addon{}
	for _, a := range headList {
		if _, dup := head[a.Name]; dup {
			fmt.Fprintf(stderr, "%s: defined in more than one file\n", a.Name)
			return 1
		}
		head[a.Name] = a
	}

	var rows []string
	problems := 0
	problem := func(name, format string, args ...any) {
		fmt.Fprintf(stderr, "%s: %s\n", name, fmt.Sprintf(format, args...))
		problems++
	}

	for _, name := range sortedKeys(head) {
		next := head[name]
		prev, existed := base[name]
		if !existed {
			rows = append(rows, row(name, "added", next.Version, permissionSummary(next.Permissions)))
			continue
		}
		if bytes.Equal(definition.CanonicalJSON(prev), definition.CanonicalJSON(next)) {
			continue
		}
		change := "changed"
		if newer, err := isNewer(next.Version, prev.Version); err != nil {
			problem(name, "%v", err)
		} else if !newer {
			problem(name, "the definition changed, so the version must be higher than %s (got %s)", prev.Version, next.Version)
		}
		if msg, conflict := definition.UpgradeConflict(definition.HostBindSources(&prev), definition.HostBindSources(&next)); conflict {
			problem(name, "the addons manager would refuse this update: %s", msg)
		}
		if next.Permissions.Grows(prev.Permissions) {
			change = "changed, **permissions grow**"
		}
		rows = append(rows, row(name, change, prev.Version+" → "+next.Version, permissionSummary(next.Permissions)))
	}
	for _, name := range sortedKeys(base) {
		if _, kept := head[name]; !kept {
			rows = append(rows, row(name, "removed", base[name].Version, ""))
		}
	}

	if len(rows) == 0 {
		fmt.Fprintln(stdout, "No addon definitions changed.")
	} else {
		fmt.Fprintln(stdout, "| Addon | Change | Version | Declared permissions |")
		fmt.Fprintln(stdout, "|---|---|---|---|")
		for _, r := range rows {
			fmt.Fprintln(stdout, r)
		}
	}
	if problems > 0 {
		fmt.Fprintf(stderr, "%d problems found\n", problems)
		return 1
	}
	return 0
}

// loadBase reads the catalog the changes are compared against. A missing directory is an empty
// catalog (the first pull request). Files that don't pass today's rules are skipped with a
// warning: they are already published, and a stricter addonctl shouldn't block unrelated changes.
func loadBase(dir string, src definition.Source, stderr io.Writer) (map[string]definition.Addon, error) {
	out := map[string]definition.Addon{}
	files, err := catalogFiles(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		a, ok := loadChecked(file, src, io.Discard)
		if !ok {
			fmt.Fprintf(stderr, "warning: base %s doesn't validate, not comparing against it\n", file)
			continue
		}
		out[a.Name] = *a
	}
	return out, nil
}

func isNewer(next, prev string) (bool, error) {
	n, err := semver.StrictNewVersion(next)
	if err != nil {
		return false, err
	}
	p, err := semver.StrictNewVersion(prev)
	if err != nil {
		// The base version is already published; if it doesn't parse, anything valid is newer.
		return true, nil
	}
	return n.GreaterThan(p), nil
}

func row(cells ...string) string {
	for i, c := range cells {
		cells[i] = strings.ReplaceAll(c, "|", `\|`)
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// permissionSummary lists the declared permissions on one line. The ones that amount to root on
// the host or the control plane are bold, so they stand out in review.
func permissionSummary(p definition.Permissions) string {
	p = p.Canonical()
	var parts []string
	for _, flagged := range []struct {
		on   bool
		name string
	}{
		{p.DockerSocket, "docker_socket"},
		{p.HostRoot, "host_root"},
		{p.CoreNetwork, "core_network"},
		{p.HostNetwork, "host_network"},
	} {
		if flagged.on {
			parts = append(parts, "**"+flagged.name+"**")
		}
	}
	if len(p.HostPaths) > 0 {
		parts = append(parts, "host_paths: `"+strings.Join(p.HostPaths, "`, `")+"`")
	}
	if len(p.HostPorts) > 0 {
		ports := make([]string, len(p.HostPorts))
		for i, port := range p.HostPorts {
			ports[i] = strconv.Itoa(port)
		}
		parts = append(parts, "host_ports: "+strings.Join(ports, ", "))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

func sortedKeys(m map[string]definition.Addon) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
