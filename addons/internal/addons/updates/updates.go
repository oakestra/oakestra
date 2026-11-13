// Package updates records, for every installed addon, the version the marketplace currently
// lists. The API turns that into update_available.
package updates

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
)

// DefaultInterval is how often the marketplace is asked.
const DefaultInterval = 10 * time.Minute

// Checker polls the marketplace.
type Checker struct {
	Store       store.Store
	Marketplace marketplaceclient.Getter
	Interval    time.Duration
	Logger      *slog.Logger
}

func (c *Checker) log() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Run checks once and then every Interval until ctx is done.
func (c *Checker) Run(ctx context.Context) {
	interval := c.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	c.CheckOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.CheckOnce(ctx)
		}
	}
}

// CheckOnce updates LatestVersion of every current addon. When the marketplace is down the
// previous values stay, so the dashboard keeps showing the last known update hints.
func (c *Checker) CheckOnce(ctx context.Context) {
	list, err := c.Store.List(ctx, "")
	if err != nil {
		c.log().Error("update check: listing addons failed", "error", err)
		return
	}
	for _, a := range list {
		if !a.Current {
			continue
		}
		latest, err := c.Marketplace.Get(ctx, a.Source, a.Definition.Name)
		switch {
		case errors.Is(err, marketplaceclient.ErrUnavailable):
			c.log().Warn("update check: marketplace unavailable, keeping previous values", "error", err)
			return
		case err != nil:
			// Not listed any more: nothing newer to offer, and the old hint may be stale, but
			// removing it on a hiccup would be worse than keeping it.
			c.log().Info("update check: no marketplace entry", "addon", a.AddonID, "error", err)
			continue
		}
		if _, err := semver.StrictNewVersion(latest.Version); err != nil {
			c.log().Warn("update check: marketplace returned an invalid version", "addon", a.AddonID, "version", latest.Version)
			continue
		}
		if latest.Version == a.LatestVersion {
			continue
		}
		_, err = c.Store.Update(ctx, a.ID, func(x *addons.Installed) error {
			x.LatestVersion = latest.Version
			return nil
		})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			c.log().Error("update check: storing latest version failed", "addon", a.AddonID, "error", err)
		}
	}
}
