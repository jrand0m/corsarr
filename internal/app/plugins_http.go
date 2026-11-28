package app

import (
	"errors"
	"net/http"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/modules"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/internal/wasm"
)

// Mount adds the plugin module routes.
func (p *PluginAdmin) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/plugins/modules", p.httpList)
	m.Handle("POST /api/v1/plugins/modules", p.httpInstall)
	m.Handle("GET /api/v1/plugins/modules/{id}", p.httpGet)
	m.Handle("DELETE /api/v1/plugins/modules/{id}", p.httpUninstall)
	m.Handle("POST /api/v1/plugins/modules/{id}/update", p.httpUpdate)
	m.Handle("POST /api/v1/plugins/modules/{id}/rollback", p.httpRollback)
	m.Handle("POST /api/v1/plugins/modules/{id}/actions/{action}", p.httpAction)
	m.Handle("POST /api/v1/plugins/modules/{id}/validate", p.httpValidate)
	m.Handle("GET /api/v1/plugins/catalogue", p.httpCatalogue)
}

func (p *PluginAdmin) httpList(w http.ResponseWriter, _ *http.Request) {
	items, err := p.List()
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "policy": p.Policy()})
}

func (p *PluginAdmin) httpGet(w http.ResponseWriter, r *http.Request) {
	v, err := p.Get(r.PathValue("id"))
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, v)
}

func (p *PluginAdmin) httpCatalogue(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, p.Catalogue(r.Context(), r.URL.Query().Get("refresh") == "1"))
}

type installBody struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (p *PluginAdmin) httpInstall(w http.ResponseWriter, r *http.Request) {
	var in installBody
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	if in.ID == "" || in.Version == "" {
		api.WriteError(w, http.StatusBadRequest, "bad_request", "id and version (as the catalogue shows them) are required", "version")
		return
	}
	v, err := p.Install(r.Context(), in.ID, in.Version)
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, v)
}

func (p *PluginAdmin) httpUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version string `json:"version"`
	}
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	if in.Version == "" {
		api.WriteError(w, http.StatusBadRequest, "bad_request", "version (as the catalogue shows it) is required", "version")
		return
	}
	v, err := p.Update(r.Context(), r.PathValue("id"), in.Version)
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, v)
}

func (p *PluginAdmin) httpRollback(w http.ResponseWriter, r *http.Request) {
	v, err := p.Rollback(r.PathValue("id"))
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, v)
}

func (p *PluginAdmin) httpUninstall(w http.ResponseWriter, r *http.Request) {
	refs, err := p.Uninstall(r.PathValue("id"))
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"usedBy": refs})
}

type valuesBody struct {
	Instance string         `json:"instance"`
	Values   map[string]any `json:"values"`
}

func (p *PluginAdmin) httpAction(w http.ResponseWriter, r *http.Request) {
	var in valuesBody
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	res, err := p.Action(r.Context(), r.PathValue("id"), r.PathValue("action"), in.Instance, in.Values)
	if err != nil {
		p.fail(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, res)
}

func (p *PluginAdmin) httpValidate(w http.ResponseWriter, r *http.Request) {
	var in valuesBody
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	v, err := p.Get(r.PathValue("id"))
	if err != nil {
		p.fail(w, err)
		return
	}
	problems := schema.Validate(v.Manifest.Settings, in.Values)
	if problems == nil {
		problems = schema.Problems{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"problems": problems})
}

// errorStatuses maps admin errors to a status and code, first match wins.
var errorStatuses = []struct {
	err    error
	status int
	code   string
	field  string
}{
	{ErrModulesUnavailable, http.StatusServiceUnavailable, "modules_unavailable", ""},
	{ErrNotInstalledModule, http.StatusNotFound, "not_found", ""},
	{modules.ErrNotInstalled, http.StatusNotFound, "not_found", ""},
	{ErrNotInCatalogue, http.StatusNotFound, "not_found", ""},
	{ErrCatalogueChanged, http.StatusConflict, "catalogue_changed", "version"},
	{ErrUnknownInstance, http.StatusBadRequest, "bad_request", "instance"},
	{ErrIncompatible, http.StatusUnprocessableEntity, "incompatible", ""},
	{modules.ErrChecksum, http.StatusUnprocessableEntity, "verify_failed", ""},
	{ErrCatalogueUnreadable, http.StatusBadGateway, "catalogue_unreachable", ""},
	{modules.ErrBuiltin, http.StatusConflict, "builtin", ""},
}

// fail maps admin errors to the API's one error shape.
func (p *PluginAdmin) fail(w http.ResponseWriter, err error) {
	var ps schema.Problems
	switch {
	case errors.As(err, &ps):
		api.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    api.ErrorDetail{Code: "invalid_values", Message: "some settings are not valid"},
			"problems": ps})
		return
	case errors.Is(err, ErrPluginsLocked):
		api.WriteError(w, http.StatusForbidden, "plugins_locked", err.Error()+" in the configuration", "")
		return
	case errors.Is(err, plugin.ErrUnsupported):
		api.WriteError(w, http.StatusNotFound, "not_found", "the plugin has no such action", "")
		return
	case isModuleRefusal(err):
		api.WriteError(w, http.StatusUnprocessableEntity, "verify_failed", err.Error(), "")
		return
	}
	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			api.WriteError(w, e.status, e.code, err.Error(), e.field)
			return
		}
	}
	p.d.Log.Warn("plugin module request failed", "err", err)
	api.WriteError(w, http.StatusBadGateway, "plugin_failed", err.Error(), "")
}

// isModuleRefusal reports a download that did not match its entry or a
// module the engine refused (manifest), both verification failures.
func isModuleRefusal(err error) bool {
	var (
		me *wasm.ManifestError
		ve *catalogue.VerifyError
	)
	return errors.As(err, &me) || errors.As(err, &ve)
}
