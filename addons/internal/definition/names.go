package definition

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// segmentPattern applies to addon names, tap names and service names. It keeps every derived
// name a valid DNS label and URL path segment.
var segmentPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// idSeparator joins the namespace and the name in an ID. Segments may not contain it, which is
// what makes IDs and container names decodable without ambiguity.
const idSeparator = "--"

// MaxDNSLabel is the longest DNS label. The UI alias and container names have to fit.
const MaxDNSLabel = 63

const (
	containerPrefix = "addon-"
	uiAliasPrefix   = "addonui-"
)

// reservedTapNames can't be used as tap names because they already mean something in a
// qualified name.
var reservedTapNames = map[string]bool{
	"local":    true,
	"official": true,
	"oakestra": true,
}

// ReservedUINames are dashboard-v2 built-in module names. The dashboard ignores a UI addon
// whose id is one of them, so we reject it up front instead of letting it install silently
// without a UI.
var ReservedUINames = map[string]bool{
	"applications":   true,
	"infrastructure": true,
	"users":          true,
	"organizations":  true,
	"help":           true,
}

// ValidateSegment checks a single name segment (addon, tap or service name).
func ValidateSegment(s string) error {
	if !segmentPattern.MatchString(s) {
		return fmt.Errorf("%q must be 1-32 lowercase letters, digits or hyphens, starting and ending with a letter or digit", s)
	}
	if strings.Contains(s, idSeparator) {
		return fmt.Errorf("%q must not contain %q, it is reserved as a separator", s, idSeparator)
	}
	return nil
}

// ValidateTapName checks a tap name: a valid segment that isn't reserved.
func ValidateTapName(s string) error {
	if err := ValidateSegment(s); err != nil {
		return err
	}
	if reservedTapNames[s] {
		return fmt.Errorf("tap name %q is reserved", s)
	}
	return nil
}

// ValidateSource checks that a source is internally consistent.
func ValidateSource(src Source) error {
	switch src.Kind {
	case SourceOfficial:
		if src.Name != string(SourceOfficial) {
			return fmt.Errorf("official source must be named %q", SourceOfficial)
		}
	case SourceLocal:
		if src.Name != string(SourceLocal) {
			return fmt.Errorf("local source must be named %q", SourceLocal)
		}
	case SourceTap:
		return ValidateTapName(src.Name)
	case "":
		return errors.New("source is missing")
	default:
		return fmt.Errorf("unknown source kind %q", src.Kind)
	}
	return nil
}

// SourceFromIndexName maps the "source" string of an index file to a Source: "official" is the
// official catalog, anything else is a tap.
func SourceFromIndexName(name string) (Source, error) {
	if name == string(SourceOfficial) {
		return OfficialSource, nil
	}
	if err := ValidateTapName(name); err != nil {
		return Source{}, err
	}
	return TapSource(name), nil
}

// ParseQualified parses a user-facing addon name: "name" (official), "oakestra/name" or
// "official/name" (official, long form), "local/name" or "<tap>/name".
func ParseQualified(s string) (Source, string, error) {
	ns, name, found := strings.Cut(s, "/")
	if !found {
		name, ns = ns, ""
	}
	if err := ValidateSegment(name); err != nil {
		return Source{}, "", fmt.Errorf("invalid addon name: %w", err)
	}
	switch ns {
	case "":
		if found {
			return Source{}, "", fmt.Errorf("invalid addon name %q: empty namespace", s)
		}
		return OfficialSource, name, nil
	case "oakestra", "official":
		return OfficialSource, name, nil
	case "local":
		return LocalSource, name, nil
	}
	if err := ValidateTapName(ns); err != nil {
		return Source{}, "", fmt.Errorf("invalid namespace: %w", err)
	}
	return TapSource(ns), name, nil
}

// QualifiedName is the user-facing name: bare for official addons, "<namespace>/<name>" for
// taps and local registrations.
func QualifiedName(src Source, name string) string {
	if src.Kind == SourceOfficial {
		return name
	}
	return src.Name + "/" + name
}

// ID is the DNS-safe form of the qualified name: bare for official addons,
// "<namespace>--<name>" otherwise. It is used in container names, the UI alias and API paths.
func ID(src Source, name string) string {
	if src.Kind == SourceOfficial {
		return name
	}
	return src.Name + idSeparator + name
}

// ParseID reverses ID.
func ParseID(id string) (Source, string, error) {
	ns, name, found := strings.Cut(id, idSeparator)
	if !found {
		if err := ValidateSegment(id); err != nil {
			return Source{}, "", fmt.Errorf("invalid addon id: %w", err)
		}
		return OfficialSource, id, nil
	}
	if err := ValidateSegment(name); err != nil {
		return Source{}, "", fmt.Errorf("invalid addon id: %w", err)
	}
	if ns == "local" {
		return LocalSource, name, nil
	}
	if err := ValidateTapName(ns); err != nil {
		return Source{}, "", fmt.Errorf("invalid addon id: %w", err)
	}
	return TapSource(ns), name, nil
}

// ContainerName is the container name for a service of an installed addon:
// "addon-<id>--<service>". The double hyphen matters: with a single one, addon "a" with service
// "b-c" and addon "a-b" with service "c" would both map to "addon-a-b-c", and the runtime
// replaces containers by name. Runtimes may add their own prefix (e.g. "root_").
func ContainerName(id, serviceName string) string {
	return containerPrefix + id + idSeparator + serviceName
}

// UIAlias is the DNS alias the dashboard uses to reach an addon's UI service.
func UIAlias(id string) string {
	return uiAliasPrefix + id
}

// VolumeName is the docker volume name for a volume declared by an installed addon. Runtimes
// must use it instead of the declared name (and rewrite service mounts to match), otherwise an
// addon declaring a volume called e.g. "root_orchestrator_mongodb_data" would mount a core
// service's data. IDs never contain "_", so the result can't collide across addons.
func VolumeName(id, volume string) string {
	return containerPrefix + id + "_" + volume
}

// NetworkName is the docker network name for a network declared by an installed addon. The
// reasoning is the same as for VolumeName.
func NetworkName(id, network string) string {
	return containerPrefix + id + "_" + network
}
