// Package registry resolves image tags to digests through the registry HTTP API, so the
// marketplace can pin local registrations without a docker socket or pulling any layers.
package registry

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
)

// Resolver turns an image reference into the digest it currently points to.
type Resolver interface {
	// Resolve returns "sha256:<hex>" for the manifest (or index, for multi-arch images) the
	// reference names.
	Resolve(ctx context.Context, image string) (string, error)
}

// Remote resolves digests against the image's registry, anonymously.
//
// Only anonymous access is supported. Reading credentials would mean either a docker config
// inside the marketplace container or the docker socket, and the point of this package is to
// need neither. Private images can still be registered by pinning the digest in the
// definition, which skips resolution.
type Remote struct {
	// Timeout bounds one resolution; it defaults to 15 s.
	Timeout time.Duration
	// AllowPrivate lets the registry live on a private or loopback address and be reached
	// over plain http. Off by default: registries in local definitions are admin-chosen, but a
	// definition still shouldn't turn the marketplace into a probe of the control-plane
	// network. Operators with an internal registry opt in with MARKETPLACE_REGISTRY_ALLOW_PRIVATE.
	AllowPrivate bool
}

// Resolve implements Resolver. HEAD is tried first because it doesn't count against Docker
// Hub's pull rate limit; some registries don't return the digest header on HEAD, hence the GET
// fallback.
func (r Remote) Resolve(ctx context.Context, image string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("parsing image reference %q: %w", image, err)
	}
	// go-containerregistry picks http by itself for localhost, RFC 1918 and ".local" hosts.
	// That is only acceptable when private registries are explicitly allowed.
	if !r.AllowPrivate && ref.Context().Registry.Scheme() == "http" {
		return "", fmt.Errorf("resolving %q: registry %q would be reached over plain http, set MARKETPLACE_REGISTRY_ALLOW_PRIVATE to permit local registries", image, ref.Context().RegistryStr())
	}
	// The guarded transport replaces the default one, which honors proxy variables and would
	// connect to any address, including ones the auth realm or a redirect points at.
	transport := source.NewGuardedTransport(r.AllowPrivate)
	defer transport.CloseIdleConnections()
	opts := []remote.Option{
		remote.WithTransport(transport),
		remote.WithContext(ctx),
		remote.WithAuth(authn.Anonymous),
		remote.WithUserAgent("oakestra-marketplace"),
	}
	if desc, err := remote.Head(ref, opts...); err == nil && desc.Digest.Algorithm == "sha256" {
		return desc.Digest.String(), nil
	}
	desc, err := remote.Get(ref, opts...)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", image, err)
	}
	if desc.Digest.Algorithm != "sha256" {
		return "", fmt.Errorf("resolving %q: registry returned a %s digest, only sha256 is supported", image, desc.Digest.Algorithm)
	}
	return desc.Digest.String(), nil
}
