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

	// The reconcile loop doesn't need the key, so it loads in the background.
	gate := auth.KeyLoaderFromEnv().StartGate(ctx, logger)

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
