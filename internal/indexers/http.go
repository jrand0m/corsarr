package indexers

import (
	"errors"
	"net/http"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers the indexer settings API and the outbound Torznab endpoint.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/indexers", s.list)
	m.Handle("POST /api/v1/indexers", s.create)
	m.Handle("POST /api/v1/indexers/test", s.testDraft)
	m.Handle("GET /api/v1/indexers/definitions", s.definitions)
	m.Handle("POST /api/v1/indexers/definitions/reload", s.reload)
	m.Handle("GET /api/v1/indexers/torznab", s.torznabState)
	m.Handle("PUT /api/v1/indexers/torznab", s.setTorznab)
	m.Handle("GET /api/v1/indexers/{key}", s.get)
	m.Handle("PUT /api/v1/indexers/{key}", s.update)
	m.Handle("DELETE /api/v1/indexers/{key}", s.remove)
	m.Handle("POST /api/v1/indexers/{key}/test", s.testOne)
	if km, ok := m.(api.KeyMounter); ok {
		km.HandleAPIKey("GET /torznab/{id}/api", s.torznab, denyTorznab)
		km.HandleAPIKey("GET /torznab/{id}/dl", s.download, denyTorznab)
	}
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	rows, err := s.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	row, err := s.Get(r.Context(), domain.ObjectKey(r.PathValue("key")))
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, row)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	row, err := s.Create(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, row)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	row, err := s.Update(r.Context(), domain.ObjectKey(r.PathValue("key")), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, row)
}

func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.Delete(r.Context(), domain.ObjectKey(r.PathValue("key"))); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) testOne(w http.ResponseWriter, r *http.Request) {
	res, err := s.Test(r.Context(), domain.ObjectKey(r.PathValue("key")))
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, res)
}

func (s *Service) testDraft(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !api.DecodeJSON(w, r, &in) {
		return
	}
	res, err := s.TestDraft(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, res)
}

func (s *Service) definitions(w http.ResponseWriter, _ *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": s.d.Catalog.Entries(), "directory": s.d.UserDir})
}

func (s *Service) reload(w http.ResponseWriter, _ *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": s.ReloadDefinitions(), "directory": s.d.UserDir})
}

type torznabBody struct {
	Enabled bool `json:"enabled"`
}

func (s *Service) torznabState(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"enabled": s.TorznabEnabled(r.Context()), "all": s.d.BasePath() + "/torznab/all/api"})
}

func (s *Service) setTorznab(w http.ResponseWriter, r *http.Request) {
	var b torznabBody
	if !api.DecodeJSON(w, r, &b) {
		return
	}
	if err := s.SetTorznab(r.Context(), b.Enabled); err != nil {
		writeErr(w, err)
		return
	}
	s.torznabState(w, r)
}

func writeErr(w http.ResponseWriter, err error) {
	var inv *InvalidError
	switch {
	case errors.As(err, &inv) && inv.Test != nil:
		api.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": api.ErrorDetail{Code: "test_failed", Message: inv.Error()}, "test": inv.Test})
	case errors.As(err, &inv):
		f := FieldError{}
		if len(inv.Fields) > 0 {
			f = inv.Fields[0]
		}
		api.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": api.ErrorDetail{Code: "invalid", Message: inv.Error(), Field: f.Field}, "fields": inv.Fields})
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, http.StatusNotFound, "not_found", ErrNotFound.Error(), "")
	case errors.Is(err, ErrExists):
		api.WriteError(w, http.StatusConflict, "exists", ErrExists.Error(), "key")
	case errors.Is(err, ErrManaged):
		api.WriteError(w, http.StatusConflict, "managed", ErrManaged.Error(), "")
	default:
		api.WriteError(w, http.StatusInternalServerError, "internal", "internal error", "")
	}
}
