package definition

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxDisplayName   = 64
	maxCategory      = 32
	maxIcon          = 2048
	maxTags          = 8
	maxTag           = 32
	maxMetadata      = 32
	maxMetadataKey   = 63
	maxMetadataValue = 1024
)

// categoryPattern is deliberately ASCII only: categories end up in URLs (?category=) and in
// filter UIs, where look-alike letters from other scripts would split one category into two.
var categoryPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9 -]*[a-z0-9])?$`)

// tagPattern is stricter than categoryPattern because a tag is a single token.
var tagPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// metadataKeyPattern allows a domain prefix, so catalogs can namespace their
// keys ("example.com/catalog-id") without colliding with each other.
var metadataKeyPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9./-]*[a-z0-9])?$`)

// NormalizeCategory returns the canonical form of a category: trimmed, lowercased, inner
// whitespace collapsed to single spaces. Normalize stores this form and the marketplace's
// ?category= filter compares against it, so "AI", " ai " and "Ai" are one category.
func NormalizeCategory(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// NormalizeTag returns the canonical form of a tag: trimmed and lowercased. The marketplace's
// ?tag= filter compares against it.
func NormalizeTag(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeTags normalizes, sorts and dedupes. Blank tags are kept (as "") so Validate can
// reject them instead of them disappearing silently.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = NormalizeTag(t)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func validateDisplayFields(a *Addon) Errors {
	var errs Errors
	if a.DisplayName != "" {
		switch {
		case strings.TrimSpace(a.DisplayName) != a.DisplayName:
			errs = errs.add("display_name", "must not start or end with whitespace")
		case utf8.RuneCountInString(a.DisplayName) > maxDisplayName:
			errs = errs.add("display_name", "is longer than %d characters", maxDisplayName)
		case hasUnsafeRune(a.DisplayName):
			errs = errs.add("display_name", "must not contain control or bidirectional formatting characters")
		}
	}
	if a.Category != "" {
		switch {
		case NormalizeCategory(a.Category) != a.Category:
			errs = errs.add("category", "%q is not normalized, use %q", a.Category, NormalizeCategory(a.Category))
		case len(a.Category) > maxCategory:
			errs = errs.add("category", "is longer than %d characters", maxCategory)
		case !categoryPattern.MatchString(a.Category):
			errs = errs.add("category", "%q may only contain lowercase letters, digits, spaces and -", a.Category)
		}
	}
	if len(a.Tags) > maxTags {
		errs = errs.add("tags", "has more than %d entries", maxTags)
	}
	for i, t := range a.Tags {
		switch {
		case t == "":
			errs = errs.add(idx("tags", i), "must not be empty")
		case NormalizeTag(t) != t:
			errs = errs.add(idx("tags", i), "%q is not normalized, use %q", t, NormalizeTag(t))
		case len(t) > maxTag || !tagPattern.MatchString(t):
			errs = errs.add(idx("tags", i), "%q must be lowercase letters, digits and -, starting and ending with a letter or digit, at most %d characters", t, maxTag)
		}
	}
	if a.Icon != "" {
		if err := checkIcon(a.Icon); err != "" {
			errs = errs.add("icon", "%s", err)
		}
	}
	if len(a.Metadata) > maxMetadata {
		errs = errs.add("metadata", "has more than %d entries", maxMetadata)
	}
	for _, k := range slices.Sorted(maps.Keys(a.Metadata)) {
		switch {
		case len(k) > maxMetadataKey || !metadataKeyPattern.MatchString(k):
			errs = errs.add(key("metadata", k), "keys are lowercase letters, digits, '.', '-' and '/', start and end with a letter or digit, at most %d characters", maxMetadataKey)
		case len(a.Metadata[k]) > maxMetadataValue:
			errs = errs.add(key("metadata", k), "value is longer than %d bytes", maxMetadataValue)
		}
	}
	return errs
}

// checkIcon returns why s isn't an acceptable icon URL, or "". Only https: the dashboard puts it
// in an <img src>, where http would be mixed content and data:/javascript: URLs would let a
// catalog entry inject content instead of linking to it. Userinfo is refused because it is only
// useful for disguising the real host.
func checkIcon(s string) string {
	if len(s) > maxIcon {
		return fmt.Sprintf("is longer than %d characters", maxIcon)
	}
	if strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "must not contain whitespace or control characters"
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.Hostname() == "" {
		return "must be an https URL"
	}
	if u.User != nil {
		return "must not contain user information"
	}
	return ""
}

// hasUnsafeRune reports control characters and the bidi overrides and isolates, which can make
// a display name render differently from what a reviewer reads in the definition file.
func hasUnsafeRune(s string) bool {
	for _, r := range s {
		switch {
		case unicode.IsControl(r):
			return true
		case r == '\u061c', r == '\u200e', r == '\u200f',
			r >= '\u202a' && r <= '\u202e', r >= '\u2066' && r <= '\u2069':
			return true
		}
	}
	return false
}
