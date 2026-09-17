// Command resource-abstractor is the Go port of resource-abstractor: a REST
// API in front of MongoDB for reading/writing Oakestra's resource state
// (candidates, jobs, apps, webhooks, custom resources). It runs at both the
// root and cluster level, differentiated purely by env vars
// (RESOURCE_ABSTRACTOR_PORT, MONGO_URL, MONGO_PORT).
//
// It's a thin wrapper around package abstractor and package rest: config
// and slog setup, dialing Mongo, and graceful shutdown are the only things
// that happen here.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/config"
	"github.com/oakestra/oakestra/go_resource_abstractor/rest"
)

const (
	mongoConnectTimeout = 30 * time.Second
	shutdownGracePeriod = 10 * time.Second
)

func main() {
	cfg := config.Load()
	logger := config.NewLogger(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := dialMongo(ctx, cfg.MongoURI())
	if err != nil {
		logger.Error("failed to connect to mongo", "error", err)
		os.Exit(1)
	}

	svc, err := abstractor.New(abstractor.Options{
		Client:             client,
		HookConnectTimeout: cfg.HookConnectTimeout,
		HookRequestTimeout: cfg.HookRequestTimeout,
		Logger:             logger,
	})
	if err != nil {
		logger.Error("failed to build abstractor service", "error", err)
		os.Exit(1)
	}

	indexCtx, cancelIndex := context.WithTimeout(ctx, mongoConnectTimeout)
	err = svc.EnsureIndexes(indexCtx)
	cancelIndex()
	if err != nil {
		logger.Error("failed to ensure indexes", "error", err)
		os.Exit(1)
	}

	handler := rest.NewHandler(svc, logger)

	// ":<port>" binds all interfaces, matching the Python service's
	// host="::" dual-stack bind.
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	}

	logger.Info("resource abstractor listening", "port", cfg.Port)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			logger.Error("server error", "error", err)
			exitCode = 1
		}
	}

	// Shutdown order: stop accepting/draining HTTP requests first, then
	// drain the hooks those requests may have fired (svc.Close), then
	// disconnect Mongo. Each step gets its own fresh deadline, since an
	// earlier one may already be exhausted by a slow drain.
	if err := withTimeout(shutdownGracePeriod, srv.Shutdown); err != nil {
		logger.Error("error during server shutdown", "error", err)
	}
	if err := withTimeout(shutdownGracePeriod, svc.Close); err != nil {
		logger.Error("error draining hooks", "error", err)
	}
	if err := withTimeout(shutdownGracePeriod, client.Disconnect); err != nil {
		logger.Error("error disconnecting from mongo", "error", err)
	}

	os.Exit(exitCode)
}

// withTimeout runs fn with a fresh context.Background() bounded by d,
// canceling it as soon as fn returns. main uses it for each shutdown step,
// since a step needs its own deadline instead of one an earlier step may
// have already spent.
func withTimeout(d time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return fn(ctx)
}

// dialMongo connects to and pings uri within mongoConnectTimeout, bounded
// by ctx (so a shutdown signal received mid-connect aborts it too).
func dialMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	connectCtx, cancel := context.WithTimeout(ctx, mongoConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		return nil, err
	}
	return client, nil
}
