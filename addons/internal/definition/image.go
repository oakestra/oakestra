package definition

import (
	"fmt"
	"regexp"
	"strings"
)

// This follows the grammar of github.com/distribution/reference closely enough for validation,
// without pulling in the dependency. IPv6 registry hosts aren't supported.
const (
	domainComponent = `(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])`
	domainPart      = domainComponent + `(?:\.` + domainComponent + `)*(?::[0-9]+)?`
	pathComponent   = `[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*`
	namePart        = `(?:` + domainPart + `/)?` + pathComponent + `(?:/` + pathComponent + `)*`
	tagPart         = `[\w][\w.-]{0,127}`
	digestPart      = `sha256:[a-f0-9]{64}`
)

var imagePattern = regexp.MustCompile(`^(` + namePart + `)(?::(` + tagPart + `))?(?:@(` + digestPart + `))?$`)

const maxImageNameLength = 255

// ImageRef is a parsed image reference.
type ImageRef struct {
	// Name is the repository including the registry host, e.g. "ghcr.io/org/web".
	Name string
	// Tag is empty when the reference has none.
	Tag string
	// Digest is "sha256:<hex>" or empty.
	Digest string
}

// Pinned reports whether the reference names an exact image by digest.
func (r ImageRef) Pinned() bool {
	return r.Digest != ""
}

// String formats the reference back into "name[:tag][@digest]".
func (r ImageRef) String() string {
	var b strings.Builder
	b.WriteString(r.Name)
	if r.Tag != "" {
		b.WriteString(":" + r.Tag)
	}
	if r.Digest != "" {
		b.WriteString("@" + r.Digest)
	}
	return b.String()
}

// ParseImage parses "name[:tag][@sha256:<64 hex>]". Only sha256 digests are accepted.
func ParseImage(s string) (ImageRef, error) {
	m := imagePattern.FindStringSubmatch(s)
	if m == nil {
		return ImageRef{}, fmt.Errorf("%q is not a valid image reference", s)
	}
	if len(m[1]) > maxImageNameLength {
		return ImageRef{}, fmt.Errorf("image name is longer than %d characters", maxImageNameLength)
	}
	return ImageRef{Name: m[1], Tag: m[2], Digest: m[3]}, nil
}
