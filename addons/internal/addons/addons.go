// Package addons holds the model of an installed addon, shared by the API, the store and the
// reconcile loop.
package addons

import (
	"fmt"
	"time"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

// Status is the lifecycle state of an installed addon. The API only ever sets Installing,
// Updating and Disabling; the reconcile loop moves everything else.
type Status string

const (
	Installing      Status = "installing"
	Updating        Status = "updating"
	Disabling       Status = "disabling"
	Disabled        Status = "disabled"
	Failed          Status = "failed"
	Active          Status = "active"
	PartiallyActive Status = "partially_active"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case Installing, Updating, Disabling, Disabled, Failed, Active, PartiallyActive:
		return true
	}
	return false
}

// Running reports whether the addon is meant to have containers and is being watched.
func (s Status) Running() bool {
	return s == Active || s == PartiallyActive
}

// Settled reports whether no install, update or removal is in flight, which is when an admin
// may update or re-approve the addon.
func (s Status) Settled() bool {
	return s.Running() || s == Failed
}

// StatusDetails explains a status. It serializes to {} when empty, which is what the dashboard
// expects rather than null.
type StatusDetails struct {
	FailedServices []string `json:"failed_services,omitempty" bson:"failed_services,omitempty"`
	Message        string   `json:"message,omitempty" bson:"message,omitempty"`
}

// Installed is an addon installed on this orchestration plane.
type Installed struct {
	// ID is the store's identifier (a Mongo ObjectID in hex). It is what the API paths and the
	// oak.addon.id container label carry.
	ID string
	// AddonID is definition.ID of the definition, e.g. "monitoring" or "acme--tool". At most
	// one non-disabled installation per AddonID exists.
	AddonID       string
	QualifiedName string
	Source        definition.Source
	Version       string
	// Definition is the pinned snapshot the containers are created from. It never changes
	// except through an accepted install or update.
	Definition definition.Addon
	// Granted is what the admin confirmed. The runtime refuses a snapshot whose declared
	// permissions differ from it.
	Granted definition.Permissions
	// Signature is the grant HMAC (see package grant) over the id, the snapshot and Granted.
	// Anyone who can write to Mongo can rewrite the snapshot and the grant consistently, so
	// the reconcile loop only trusts a record whose signature checks out with the key in the
	// engine's state directory.
	Signature     string
	Status        Status
	StatusDetails StatusDetails
	Logs          string
	// LatestVersion is the version the marketplace listed at the last update check.
	LatestVersion string
	// Current is true until the addon is disabled. A unique index over AddonID where Current
	// is true is what keeps a second installation out.
	Current   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// HostBindConflict checks def's host bind sources against those of every other addon in all
// that isn't disabled (definition.OverlapConflict). selfID is the record def belongs to, or
// empty for an addon that isn't stored yet. CheckPolicy only sees one definition at a time,
// so without this one addon could plant a symlink in a directory another one mounts.
func HostBindConflict(def *definition.Addon, selfID string, all []Installed) (string, bool) {
	mine := definition.HostBindSources(def)
	if len(mine) == 0 {
		return "", false
	}
	for i := range all {
		o := &all[i]
		if (selfID != "" && o.ID == selfID) || o.Status == Disabled {
			continue
		}
		if msg, ok := definition.OverlapConflict(mine, definition.HostBindSources(&o.Definition)); ok {
			return fmt.Sprintf("%s of addon %q", msg, o.QualifiedName), true
		}
	}
	return "", false
}
