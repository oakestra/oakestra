// Command go_resource_abstractor is the Go port of resource-abstractor: a
// REST API in front of MongoDB for reading/writing Oakestra's resource
// state (candidates, jobs, apps, webhooks, custom resources). It runs at
// both the root and cluster level, differentiated purely by env vars
// (RESOURCE_ABSTRACTOR_PORT, MONGO_URL, MONGO_PORT).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_resource_abstractor/api"
	"go_resource_abstractor/config"
	"go_resource_abstractor/db"
	"go_resource_abstractor/logger"
	"go_resource_abstractor/services"
)

const (
	mongoConnectTimeout = 30 * time.Second
	shutdownGracePeriod = 10 * time.Second
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, mongoConnectTimeout)
	store, err := db.Connect(connectCtx, cfg.MongoURI())
	cancel()
	if err != nil {
		slog.Error("failed to connect to mongo", "error", err)
		os.Exit(1)
	}

	hooks := services.NewHooks(store, cfg.HookConnectTimeout, cfg.HookRequestTimeout)
	router := api.NewRouter(store, hooks)

	// ":<port>" binds all interfaces, the equivalent on Linux (this
	// service's deployment target) of the Python service's explicit
	// host="::" dual-stack bind.
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	slog.Info("resource abstractor listening", "port", cfg.Port)

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
		slog.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			slog.Error("server error", "error", err)
			exitCode = 1
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownGracePeriod)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("error during server shutdown", "error", err)
	}
	cancelShutdown()

	// A fresh deadline rather than the one above: if draining in-flight
	// requests consumed the whole grace period, shutdownCtx is already
	// expired and Disconnect would fail without ever having tried.
	disconnectCtx, cancelDisconnect := context.WithTimeout(context.Background(), shutdownGracePeriod)
	if err := store.Disconnect(disconnectCtx); err != nil {
		slog.Error("error disconnecting from mongo", "error", err)
	}
	cancelDisconnect()

	os.Exit(exitCode)
}
