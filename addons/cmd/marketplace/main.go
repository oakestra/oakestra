// Command marketplace serves the addon catalog: the official index, admin-added taps and local
// registrations, merged into one list. See internal/marketplace/api for the endpoints.
//
//	marketplace              run the service
//	marketplace healthcheck  exit 0 if the service on ADDON_MARKETPLACE_PORT answers GET /
//
// The healthcheck mode exists because the distroless image has no shell or curl.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/addons/internal/auth"
	"github.com/oakestra/oakestra/addons/internal/marketplace/api"
	"github.com/oakestra/oakestra/addons/internal/marketplace/catalog"
	"github.com/oakestra/oakestra/addons/internal/marketplace/registry"
	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
	"github.com/oakestra/oakestra/addons/internal/marketplace/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("marketplace stopped", "error", err)
		os.Exit(1)
	}
}

func healthcheck() int {
	port := os.Getenv("ADDON_MARKETPLACE_PORT")
	if _, err := strconv.Atoi(port); err != nil {
		port = "11102"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "status", resp.Status)
		return 1
	}
	return 0
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	log.Info("starting marketplace", "config", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetTimeout(10 * time.Second))
	if err != nil {
		return fmt.Errorf("configuring mongo client: %w", err)
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(dctx)
	}()
	st := store.NewMongo(client.Database("marketplace"))

	cat := catalog.New(catalog.Config{
		Store:         st,
		Fetcher:       source.NewFetcher(),
		Resolver:      registry.Remote{AllowPrivate: cfg.RegistryAllowPrivate},
		OfficialURL:   cfg.OfficialURL,
		OfficialToken: cfg.OfficialToken,
		Logger:        log,
	})

	// Mongo usually starts alongside us in compose; wait for it instead of crash-looping.
	if err := retry(ctx, log, "initializing the database", func(ctx context.Context) error {
		if err := st.EnsureIndexes(ctx); err != nil {
			return err
		}
		return cat.Init(ctx)
	}); err != nil {
		return err
	}

	keys := auth.KeyLoaderFromEnv()
	keys.Logger = log
	gate := api.NewAdminGate(keys.Fetch, log)
	go func() {
		key, err := keys.Load(ctx)
		if err != nil {
			// Shutdown, or an invalid JWT_PUBLIC_KEY, which Load already logged. Admin
			// endpoints keep answering 503.
			return
		}
		gate.SetKey(key)
		log.Info("loaded the JWT public key, admin endpoints are available")
	}()

	srv := &http.Server{
		Addr:    net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: api.New(api.Config{Catalog: cat, Gate: gate, CORSOrigin: cfg.CORSOrigin, Logger: log}),
		// Generous enough for a 6 MiB import on a slow link and for a local registration
		// that resolves several image digests, tight enough to shed stuck clients.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		syncLoop(ctx, log, cat, cfg.SyncInterval)
	}()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		stop()
		<-syncDone
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	<-syncDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// syncLoop syncs every source on start and then every interval. A slow sync delays the next
// tick rather than overlapping it; the per-source locks in catalog keep on-demand syncs from
// running concurrently with it.
func syncLoop(ctx context.Context, log *slog.Logger, cat *catalog.Service, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		log.Info("syncing sources")
		cat.SyncAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func retry(ctx context.Context, log *slog.Logger, what string, fn func(context.Context) error) error {
	for {
		actx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := fn(actx)
		cancel()
		if err == nil {
			return nil
		}
		log.Warn(what+" failed, retrying", "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", what, err)
		case <-time.After(3 * time.Second):
		}
	}
}
