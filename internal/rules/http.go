package rules

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
)

// Mount registers GET /api/v1/rules/actions and POST /api/v1/rules/actions/{id}/undo.
func (r *Runner) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/rules/actions", r.listActions)
	m.Handle("POST /api/v1/rules/actions/{id}/undo", r.undo)
}

func (r *Runner) listActions(w http.ResponseWriter, req *http.Request) {
	cursor, limit, ok := api.Page(w, req)
	if !ok {
		return
	}
	items, next, err := r.Actions(req.Context(), cursor, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	api.WritePage(w, items, next)
}

func (r *Runner) undo(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", "action id must be a number", "id")
		return
	}
	a, err := r.Undo(req.Context(), id)
	switch {
	case errors.Is(err, ErrUnknownRule):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "id")
	case errors.Is(err, ErrNoUndo):
		api.WriteError(w, http.StatusConflict, "no_undo", "this action cannot be undone (not acted, expired or not reversible)", "")
	case err != nil:
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
	default:
		api.WriteJSON(w, http.StatusOK, a)
	}
}
