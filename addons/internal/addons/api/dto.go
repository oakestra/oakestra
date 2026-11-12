package api

import (
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/definition"
)

// addonResponse is the JSON of an installed addon. "_id" and the flattened definition fields
// are the shape the dashboard reads.
type addonResponse struct {
	ID            string                 `json:"_id"`
	Name          string                 `json:"name"`
	QualifiedName string                 `json:"qualified_name"`
	AddonID       string                 `json:"addon_id"`
	Source        definition.Source      `json:"source"`
	Version       string                 `json:"version"`
	Description   string                 `json:"description"`
	Homepage      string                 `json:"homepage,omitempty"`
	Maintainers   []string               `json:"maintainers,omitempty"`
	UI            *definition.UI         `json:"ui"`
	Services      []definition.Service   `json:"services"`
	Volumes       []definition.Volume    `json:"volumes"`
	Networks      []definition.Network   `json:"networks"`
	Permissions   definition.Permissions `json:"permissions"`
	Status        addons.Status          `json:"status"`
	StatusDetails addons.StatusDetails   `json:"status_details"`
	Logs          string                 `json:"logs,omitempty"`
	// UpdateAvailable is true when the marketplace lists a newer version than the installed
	// one, at the time of the last update check.
	UpdateAvailable bool      `json:"update_available"`
	LatestVersion   string    `json:"latest_version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// toResponse builds the JSON of a record. Logs are left out unless withLogs is set: they hold
// whatever the addon's containers printed. status_details only carries service names and
// engine errors, never log text, so it is safe to show.
func toResponse(a addons.Installed, withLogs bool) addonResponse {
	d := a.Definition
	d.Normalize()
	if !withLogs {
		a.Logs = ""
	}
	return addonResponse{
		ID: a.ID, Name: a.QualifiedName, QualifiedName: a.QualifiedName, AddonID: a.AddonID,
		Source: a.Source, Version: a.Version, Description: d.Description, Homepage: d.Homepage,
		Maintainers: d.Maintainers, UI: d.UI, Services: d.Services, Volumes: d.Volumes,
		Networks: d.Networks, Permissions: a.Granted.Canonical(), Status: a.Status,
		StatusDetails: a.StatusDetails, Logs: a.Logs, UpdateAvailable: newer(a.LatestVersion, a.Version),
		LatestVersion: a.LatestVersion, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// newer reports whether candidate is a valid version greater than current.
func newer(candidate, current string) bool {
	c, err := semver.StrictNewVersion(candidate)
	if err != nil {
		return false
	}
	cur, err := semver.StrictNewVersion(current)
	if err != nil {
		return false
	}
	return c.GreaterThan(cur)
}
