// Command addons is the addons engine: the REST API for installing, updating and removing
// addons, and the loop that runs them as containers.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oakestra/oakestra/addons/internal/addons/api"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/reconcile"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/addons/statedir"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/addons/updates"
	"github.com/oakestra/oakestra/addons/internal/auth"
)

func main() {
	if err := run(); err != nil {
		slog.Error("addons engine stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Both files are created on the first start and must survive restarts: a new grant key
	// invalidates every approval, a new instance id orphans every running addon.
	if err := statedir.Ensure(cfg.StateDir); err != nil {
		return err
	}
	grantKey, err := statedir.LoadOrCreateGrantKey(cfg.StateDir)
	if err != nil {
		return err
	}
	instance, err := statedir.LoadOrCreateInstanceID(cfg.StateDir)
	if err != nil {
		return err
	}

	mongoClient, err := store.Connect(cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	st := store.NewMongo(mongoClient)
	go retryUntil(ctx, logger, "creating the unique index", st.EnsureIndexes)

	docker, err := runtime.NewDocker()
	if err != nil {
		return err
	}
	defer docker.Close()
	docker.Limits = cfg.Limits

	// The key loads in the background: jwt_generator may start after us, and the reconcile
	// loop doesn't need it. Until it arrives, authenticated endpoints answer 503.
	loader := auth.KeyLoaderFromEnv()
	gate := newGate(loader, logger)
	go func() {
		key, err := loader.Load(ctx)
		if err != nil {
			return
		}
		gate.SetKey(key)
		logger.Info("JWT public key loaded")
	}()

	market := marketplaceclient.New(cfg.MarketplaceAddr)
	go (&reconcile.Reconciler{
		Store: st, Runtime: docker, Plane: cfg.Plane, Instance: instance, GrantKey: grantKey,
		Interval: cfg.ReconcileInterval, Logger: logger,
	}).Run(ctx)
	go (&updates.Checker{Store: st, Marketplace: market, Interval: cfg.UpdateInterval, Logger: logger}).Run(ctx)

	handler := api.New(api.Config{
		Store: st, Marketplace: market, Gate: gate, GrantKey: grantKey, Plane: cfg.Plane,
		CORSOrigin: cfg.CORSOrigin, Logger: logger,
	})
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Installs wait on the marketplace (10 s), so leave room above that.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("addons engine started", "port", cfg.Port, "plane", cfg.Plane, "instance", instance,
		"marketplace", cfg.MarketplaceAddr, "state_dir", cfg.StateDir)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	// Addon containers are left running on purpose. They have a restart policy, and the next
	// start of the engine picks them up again, so restarting or upgrading the engine doesn't
	// take every addon down with it.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newGate builds the auth gate. Only a key from jwt_generator is re-fetched when a token fails
// to verify. A key from JWT_PUBLIC_KEY_URL usually crosses hosts over plain HTTP, where a
// refetch that anyone can trigger with a bad token would let an on-path attacker swap in their
// own key; it is loaded once and kept until the engine restarts.
func newGate(loader *auth.KeyLoader, logger *slog.Logger) *auth.Gate {
	if loader.Source == auth.KeySourceURL {
		logger.Warn("the JWT public key is fetched from JWT_PUBLIC_KEY_URL without pinning; set JWT_PUBLIC_KEY to the root's public key instead, the URL is only trusted on first fetch and key rotation needs a restart",
			"url", loader.URL)
	}
	if !loader.RefreshAllowed() {
		return auth.NewGate(nil, logger)
	}
	return auth.NewGate(loader.Fetch, logger)
}

func retryUntil(ctx context.Context, logger *slog.Logger, what string, fn func(context.Context) error) {
	for {
		err := fn(ctx)
		if err == nil {
			return
		}
		logger.Warn("will retry", "what", what, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}
