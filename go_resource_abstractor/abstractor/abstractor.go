// Package abstractor is the resource abstractor's library core: typed CRUD
// over applications, jobs, candidate resources, webhook registrations and
// custom resources, plus the pre/post webhook firing and validation rules
// that used to live in resource-abstractor/api's gin handlers.
//
// Package rest wraps a Service into the standalone HTTP service this module
// ships as. An orchestrator that already owns a *mongo.Client can use a
// Service directly, with no HTTP layer in between.
package abstractor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/hooks"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
)

// MongoClientOptions builds the driver options a client passed to New must
// carry, so an embedder outside this module can dial a compatible client
// without reaching into internal packages. A client built any other way
// fails to decode ObjectID `_id` fields into the model types' `ID *string`.
func MongoClientOptions(uri string) *options.ClientOptions {
	return store.ClientOptions(uri)
}

// Options configures a Service.
type Options struct {
	// Client is the caller's Mongo client, built with MongoClientOptions.
	// Service never dials, pings or disconnects it - that's the caller's
	// responsibility.
	Client *mongo.Client

	// HookConnectTimeout / HookRequestTimeout bound outbound webhook calls;
	// see internal/hooks.New.
	HookConnectTimeout time.Duration
	HookRequestTimeout time.Duration

	// Logger receives Service's own diagnostic output (failed hook
	// dispatches, etc). Defaults to slog.Default() if nil.
	Logger *slog.Logger
}

// Service is the resource abstractor's library core. Construct one with
// New, and reach the typed sub-services through Apps, Jobs, Resources,
// Hooks and CustomResources.
type Service struct {
	store      *store.Store
	dispatcher *hooks.Hooks

	// Apps is the application sub-service.
	Apps *Apps
	// Jobs is the job sub-service.
	Jobs *Jobs
	// Resources is the candidate-resource sub-service.
	Resources *Resources
	// Hooks is the webhook-registration sub-service: CRUD over hook
	// registrations themselves, distinct from the dispatcher that fires
	// them around writes to the other entities.
	Hooks *Hooks
	// CustomResources is the custom-resource sub-service: type
	// definitions and their instances.
	CustomResources *CustomResources
}

// New builds a Service around opts.Client. It never dials, pings or
// disconnects the client - the caller hands in an already-connected client
// and owns its lifecycle, both before New and after Close (which only
// drains in-flight hooks and never touches the client).
// Call EnsureIndexes separately once the Service is built, typically before
// serving traffic.
func New(opts Options) (*Service, error) {
	if opts.Client == nil {
		return nil, errors.New("abstractor: Options.Client is required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	s := store.New(opts.Client)
	d := hooks.New(s, opts.HookConnectTimeout, opts.HookRequestTimeout, logger)

	svc := &Service{store: s, dispatcher: d}
	svc.Apps = &Apps{store: s, entity: entityHooks{dispatcher: d, name: "applications"}}
	svc.Jobs = &Jobs{store: s, entity: entityHooks{dispatcher: d, name: "jobs"}}
	svc.Resources = &Resources{store: s, entity: entityHooks{dispatcher: d, name: "resources"}}
	svc.Hooks = &Hooks{store: s}
	svc.CustomResources = &CustomResources{store: s, dispatcher: d}

	return svc, nil
}

// EnsureIndexes creates the indexes internal/store relies on. Call it once
// after New, before serving traffic.
func (s *Service) EnsureIndexes(ctx context.Context) error {
	return s.store.EnsureIndexes(ctx)
}

// Close drains in-flight async (post_*) hooks, waiting for ctx or giving up
// when it expires first. It does not touch the Mongo client passed in via
// Options.Client - the caller owns that, and should disconnect it only
// after Close returns, so in-flight webhook lookups aren't cut off mid-query.
func (s *Service) Close(ctx context.Context) error {
	return s.dispatcher.Close(ctx)
}
