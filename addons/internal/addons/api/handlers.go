package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oakestra/oakestra/addons/internal/addons"
	"github.com/oakestra/oakestra/addons/internal/addons/marketplaceclient"
	"github.com/oakestra/oakestra/addons/internal/addons/store"
	"github.com/oakestra/oakestra/addons/internal/auth"
	"github.com/oakestra/oakestra/addons/internal/definition"
	"github.com/oakestra/oakestra/addons/internal/httpx"
)

func (s *server) list(c *gin.Context) {
	w, r := c.Writer, c.Request
	status := addons.Status(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		httpx.WriteError(w, http.StatusBadRequest, fmt.Sprintf("Unknown status %q", status))
		return
	}
	items, err := s.Store.List(r.Context(), status)
	if err != nil {
		s.internal(w, "listing addons", err)
		return
	}
	admin := isAdmin(c)
	out := make([]addonResponse, len(items))
	for i, a := range items {
		out[i] = toResponse(a, admin)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// isAdmin reports whether the caller's token has the Admin role. Container logs are only shown
// to admins: addons can print anything there, including secrets from their environment.
func isAdmin(c *gin.Context) bool {
	claims, ok := auth.GinClaims(c)
	return ok && claims.IsAdmin()
}

func (s *server) get(c *gin.Context) {
	a, ok := s.load(c)
	if !ok {
		return
	}
	httpx.WriteJSON(c.Writer, http.StatusOK, toResponse(a, isAdmin(c)))
}

type installRequest struct {
	Name             string `json:"name"`
	AcceptDefinition string `json:"accept_definition"`
}

func (s *server) install(c *gin.Context) {
	w, r := c.Writer, c.Request
	var req installRequest
	if !decodeBody(w, r, &req) {
		return
	}
	src, name, err := definition.ParseQualified(req.Name)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	def, ok := s.fetch(w, r, src, name)
	if !ok {
		return
	}

	id := def.ID()
	current, err := s.Store.List(r.Context(), "")
	if err != nil {
		s.internal(w, "listing addons", err)
		return
	}
	for _, o := range current {
		if o.Current && o.AddonID == id {
			httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("Addon %q is already installed.", def.QualifiedName()))
			return
		}
	}
	if msg, conflict := addons.HostBindConflict(def, "", current); conflict {
		httpx.WriteError(w, http.StatusConflict, "The addon's host paths conflict with an installed addon: "+msg)
		return
	}

	// The admin confirms the whole definition, not just the permissions: the marketplace can
	// serve something else between the dialog and this call, and images, commands and
	// environment change what runs without touching the permissions. The hash is taken over
	// exactly what gets stored.
	perms := def.Permissions.Canonical()
	def.Permissions = perms
	hash := def.Hash()
	if req.AcceptDefinition != hash {
		writeConsentRequired(w, "This addon has to be accepted before it is installed.", def, hash, nil)
		return
	}

	now := s.now()
	rec := addons.Installed{
		ID: store.NewID(), AddonID: id, QualifiedName: def.QualifiedName(), Source: def.Source, Version: def.Version,
		Definition: *def, Granted: perms, Status: addons.Installing, CreatedAt: now, UpdatedAt: now,
	}
	rec.Signature = s.GrantKey.Sign(rec, s.Plane)
	created, err := s.Store.Insert(r.Context(), rec)
	switch {
	case errors.Is(err, store.ErrDuplicate):
		// Lost a race against a concurrent install of the same addon.
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("Addon %q is already installed.", def.QualifiedName()))
	case err != nil:
		s.internal(w, "storing addon", err)
	default:
		httpx.WriteJSON(w, http.StatusCreated, toResponse(created, true))
	}
}

// acceptRequest is the body of update and approve: the hash of the definition the admin
// accepted.
type acceptRequest struct {
	AcceptDefinition string `json:"accept_definition"`
}

func (s *server) update(c *gin.Context) {
	w, r := c.Writer, c.Request
	var req acceptRequest
	if !decodeBody(w, r, &req) {
		return
	}
	cur, ok := s.load(c)
	if !ok {
		return
	}
	if !cur.Current || !cur.Status.Settled() {
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("Addon is %s and can't be updated right now.", cur.Status))
		return
	}

	latest, ok := s.fetch(w, r, cur.Source, cur.Definition.Name)
	if !ok {
		return
	}
	if latest.ID() != cur.AddonID {
		s.internal(w, "update", fmt.Errorf("marketplace returned %q for %q", latest.ID(), cur.AddonID))
		return
	}
	if !newer(latest.Version, cur.Version) {
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("No newer version than %s is available.", cur.Version))
		return
	}

	// Files the old version wrote stay on the host, so it may have planted a symlink below its
	// own read-write paths. Accepting the permissions can't make that safe, the admin can't
	// see it.
	if msg, conflict := definition.UpgradeConflict(definition.HostBindSources(&cur.Definition), definition.HostBindSources(latest)); conflict {
		httpx.WriteError(w, http.StatusConflict, "This version can't be installed over the current one: "+msg)
		return
	}
	others, err := s.Store.List(r.Context(), "")
	if err != nil {
		s.internal(w, "listing addons", err)
		return
	}
	if msg, conflict := addons.HostBindConflict(latest, cur.ID, others); conflict {
		httpx.WriteError(w, http.StatusConflict, "This version's host paths conflict with an installed addon: "+msg)
		return
	}

	// Every update needs the admin's confirmation, also when the permissions don't grow: a new
	// version can run different images and commands under the same permissions.
	perms := latest.Permissions.Canonical()
	latest.Permissions = perms
	hash := latest.Hash()
	if req.AcceptDefinition != hash {
		writeConsentRequired(w, "This version has to be accepted before it is installed.", latest, hash, &cur)
		return
	}

	applied := false
	updated, err := s.Store.Update(r.Context(), cur.ID, func(a *addons.Installed) error {
		// Someone else may have changed the addon since it was read.
		if a.Version != cur.Version || a.Status != cur.Status {
			return store.ErrAbort
		}
		applied = true
		a.Version, a.Definition, a.Granted = latest.Version, *latest, perms
		a.LatestVersion = latest.Version
		a.Status, a.StatusDetails, a.Logs = addons.Updating, addons.StatusDetails{}, ""
		a.UpdatedAt = s.now()
		a.Signature = s.GrantKey.Sign(*a, s.Plane)
		return nil
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Addon not found")
	case err != nil:
		s.internal(w, "storing update", err)
	case !applied:
		httpx.WriteError(w, http.StatusConflict, "The addon changed while the update was prepared, try again.")
	default:
		httpx.WriteJSON(w, http.StatusOK, toResponse(updated, true))
	}
}

// approve signs an installed addon's grant again. It is the way out of "grant signature
// invalid" (the state directory was lost, or the record was edited in the database): the admin
// confirms the definition again, and the snapshot is only kept if the marketplace still
// serves exactly that definition for the version, so a record rewritten in the database can't
// get approved by accident.
func (s *server) approve(c *gin.Context) {
	w, r := c.Writer, c.Request
	var req acceptRequest
	if !decodeBody(w, r, &req) {
		return
	}
	cur, ok := s.load(c)
	if !ok {
		return
	}
	if !cur.Current || !cur.Status.Settled() {
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("Addon is %s and can't be approved right now.", cur.Status))
		return
	}

	latest, ok := s.fetch(w, r, cur.Source, cur.Definition.Name)
	if !ok {
		return
	}
	latest.Permissions = latest.Permissions.Canonical()
	if latest.ID() != cur.AddonID || latest.Version != cur.Version {
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf("The marketplace lists %s %s, not the installed %s; update the addon instead.", latest.QualifiedName(), latest.Version, cur.Version))
		return
	}
	if !bytes.Equal(definition.CanonicalJSON(*latest), definition.CanonicalJSON(cur.Definition)) {
		httpx.WriteError(w, http.StatusConflict, "The installed definition differs from the marketplace's definition of the same version; remove and reinstall the addon.")
		return
	}
	others, err := s.Store.List(r.Context(), "")
	if err != nil {
		s.internal(w, "listing addons", err)
		return
	}
	if msg, conflict := addons.HostBindConflict(latest, cur.ID, others); conflict {
		httpx.WriteError(w, http.StatusConflict, "The addon's host paths conflict with an installed addon: "+msg)
		return
	}
	perms := latest.Permissions
	hash := latest.Hash()
	if req.AcceptDefinition != hash {
		writeConsentRequired(w, "This addon has to be accepted again.", latest, hash, nil)
		return
	}

	applied := false
	updated, err := s.Store.Update(r.Context(), cur.ID, func(a *addons.Installed) error {
		if a.Version != cur.Version || a.Status != cur.Status || !a.Current {
			return store.ErrAbort
		}
		applied = true
		a.Definition, a.Granted = *latest, perms
		a.QualifiedName, a.Source = latest.QualifiedName(), latest.Source
		// A failed addon is deployed again; a running one keeps running and only gets the
		// signature its next redeploy will need.
		if a.Status == addons.Failed {
			a.Status, a.StatusDetails, a.Logs = addons.Installing, addons.StatusDetails{}, ""
		}
		a.UpdatedAt = s.now()
		a.Signature = s.GrantKey.Sign(*a, s.Plane)
		return nil
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Addon not found")
	case err != nil:
		s.internal(w, "storing approval", err)
	case !applied:
		httpx.WriteError(w, http.StatusConflict, "The addon changed while the approval was prepared, try again.")
	default:
		httpx.WriteJSON(w, http.StatusOK, toResponse(updated, true))
	}
}

func (s *server) remove(c *gin.Context) {
	w, r := c.Writer, c.Request
	cur, ok := s.load(c)
	if !ok {
		return
	}
	id := cur.ID

	if cur.Status == addons.Disabled {
		if err := s.Store.Delete(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internal(w, "deleting addon", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Disabling is picked up by the reconcile loop, which removes the containers.
	updated, err := s.Store.Update(r.Context(), id, func(a *addons.Installed) error {
		if a.Status == addons.Disabled || a.Status == addons.Disabling {
			return store.ErrAbort
		}
		a.Status, a.UpdatedAt = addons.Disabling, s.now()
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "Addon not found")
			return
		}
		s.internal(w, "disabling addon", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(updated, true))
}

// load reads the addon named by the :id parameter and writes the error response itself.
func (s *server) load(c *gin.Context) (addons.Installed, bool) {
	a, err := s.Store.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(c.Writer, http.StatusNotFound, "Addon not found")
		return addons.Installed{}, false
	}
	if err != nil {
		s.internal(c.Writer, "reading addon", err)
		return addons.Installed{}, false
	}
	return a, true
}

// fetch gets a definition from the marketplace and checks it with ModeInstall: any source,
// local registrations included, but every image digest pinned. It writes the error response
// itself. The client drops everything the marketplace precomputes, so the definition is
// checked from scratch here.
func (s *server) fetch(w http.ResponseWriter, r *http.Request, src definition.Source, name string) (*definition.Addon, bool) {
	def, err := s.Marketplace.Get(r.Context(), src, name)
	switch {
	case errors.Is(err, marketplaceclient.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Marketplace addon not found")
		return nil, false
	case err != nil:
		s.log.Warn("marketplace lookup failed", "source", src.Name, "name", name, "error", err)
		httpx.WriteError(w, http.StatusBadGateway, "Marketplace is unavailable")
		return nil, false
	}
	def.Normalize()
	if err := definition.Check(def, definition.ModeInstall); err != nil {
		resp := httpx.ErrorBody{Message: "The addon definition is not valid: " + err.Error()}
		var errs definition.Errors
		if errors.As(err, &errs) {
			resp.Errors = errs
		}
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, resp)
		return nil, false
	}
	return def, true
}

// writeConsentRequired answers 409 with everything the dialog needs: the canonical definition
// the hash covers, and the permissions on their own for display. For an update, installed is
// the record being replaced, so the dialog can show what changed and whether the permissions
// grew.
func writeConsentRequired(w http.ResponseWriter, msg string, def *definition.Addon, hash string, installed *addons.Installed) {
	perms := def.Permissions.Canonical()
	body := map[string]any{
		"message":         msg,
		"definition":      json.RawMessage(definition.CanonicalJSON(*def)),
		"definition_hash": hash,
		"permissions":     perms,
	}
	if installed != nil {
		body["previous_version"] = installed.Version
		body["previous_permissions"] = installed.Granted.Canonical()
		body["permissions_grow"] = perms.Grows(installed.Granted)
	}
	httpx.WriteJSON(w, http.StatusConflict, body)
}

func (s *server) internal(w http.ResponseWriter, what string, err error) {
	s.log.Error(what+" failed", "error", err)
	httpx.WriteError(w, http.StatusInternalServerError, "Internal server error")
}

// uiAddonEntry is one element of GET /ui-addons. Name is the addon id, which is also what the
// dashboard turns into the addonui-<name> host.
type uiAddonEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SDK     string `json:"sdk"`
}

func (s *server) uiAddons(c *gin.Context) {
	w, r := c.Writer, c.Request
	entries := []uiAddonEntry{}
	// Only the root dashboard loads UIs, and only the root engine attaches the alias.
	if s.Plane != "root" {
		httpx.WriteJSON(w, http.StatusOK, entries)
		return
	}
	items, err := s.Store.List(r.Context(), addons.Active)
	if err != nil {
		s.internal(w, "listing addons", err)
		return
	}
	seen := map[string]bool{}
	for _, a := range items {
		ui := a.Definition.UI
		if ui == nil || ui.SDK == "" || seen[a.AddonID] {
			continue
		}
		if svc := a.Definition.Service(ui.Service); svc == nil || svc.Image == "" {
			s.log.Warn("skipping UI addon whose ui.service is missing", "addon", a.AddonID)
			continue
		}
		seen[a.AddonID] = true
		// Use the addon version, not the image tag. Images are pinned by digest, and an update
		// always raises the version, so the dashboard's cache busting still works.
		entries = append(entries, uiAddonEntry{Name: a.AddonID, Version: a.Version, SDK: ui.SDK})
	}
	httpx.WriteJSON(w, http.StatusOK, entries)
}
