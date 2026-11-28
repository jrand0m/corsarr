package profiles

import (
	"errors"
	"net/http"
	"slices"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers the custom format and quality profile API.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/customformats", s.listFormats)
	m.Handle("POST /api/v1/customformats", s.createFormat)
	m.Handle("POST /api/v1/customformats/test", s.test)
	m.Handle("GET /api/v1/customformats/{key}", s.getFormat)
	m.Handle("PUT /api/v1/customformats/{key}", s.updateFormat)
	m.Handle("DELETE /api/v1/customformats/{key}", s.deleteFormat)
	m.Handle("GET /api/v1/qualityprofiles", s.listProfiles)
	m.Handle("POST /api/v1/qualityprofiles", s.createProfile)
	m.Handle("GET /api/v1/qualityprofiles/{key}", s.getProfile)
	m.Handle("PUT /api/v1/qualityprofiles/{key}", s.updateProfile)
	m.Handle("DELETE /api/v1/qualityprofiles/{key}", s.deleteProfile)
	m.Handle("POST /api/v1/qualityprofiles/{key}/duplicate", s.duplicateProfile)
	m.Handle("GET /api/v1/quality/choices", choices)
}

// writeErr maps service errors onto the API error shape.
func writeErr(w http.ResponseWriter, err error) {
	var inv InvalidError
	var use InUseError
	switch {
	case errors.As(err, &inv):
		first := inv.Errs[0]
		api.WriteError(w, http.StatusUnprocessableEntity, "invalid", inv.Error(), first.Path)
	case errors.As(err, &use):
		api.WriteError(w, http.StatusConflict, "in_use", err.Error(), "")
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "key")
	case errors.Is(err, ErrExists):
		api.WriteError(w, http.StatusConflict, "exists", err.Error(), "key")
	case errors.Is(err, ErrManaged):
		api.WriteError(w, http.StatusConflict, "managed", err.Error(), "")
	case errors.Is(err, ErrBuiltin):
		api.WriteError(w, http.StatusConflict, "builtin", err.Error(), "")
	default:
		api.WriteError(w, http.StatusInternalServerError, "internal", "internal error", "")
	}
}

func reply[T any](w http.ResponseWriter, status int, v T, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, status, v)
}

func (s *Service) listFormats(w http.ResponseWriter, _ *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": s.Formats()})
}

func (s *Service) getFormat(w http.ResponseWriter, r *http.Request) {
	row, err := s.Format(r.PathValue("key"))
	reply(w, http.StatusOK, row, err)
}

func (s *Service) createFormat(w http.ResponseWriter, r *http.Request) {
	var d decide.FormatDoc
	if !api.DecodeJSON(w, r, &d) {
		return
	}
	row, err := s.CreateFormat(r.Context(), d)
	reply(w, http.StatusCreated, row, err)
}

func (s *Service) updateFormat(w http.ResponseWriter, r *http.Request) {
	var d decide.FormatDoc
	if !api.DecodeJSON(w, r, &d) {
		return
	}
	row, err := s.UpdateFormat(r.Context(), r.PathValue("key"), d)
	reply(w, http.StatusOK, row, err)
}

func (s *Service) deleteFormat(w http.ResponseWriter, r *http.Request) {
	if err := s.DeleteFormat(r.Context(), r.PathValue("key")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) test(w http.ResponseWriter, r *http.Request) {
	var req TestRequest
	if !api.DecodeJSON(w, r, &req) {
		return
	}
	res, err := s.Test(req)
	reply(w, http.StatusOK, map[string]any{"results": res}, err)
}

// profileListRow keeps the fields the item editors read: whether the profile
// is the default for the asked kind.
type profileListRow struct {
	ProfileRow
	IsDefault bool `json:"isDefault"`
}

func (s *Service) listProfiles(w http.ResponseWriter, r *http.Request) {
	kind := domain.MediaKind(r.URL.Query().Get("kind"))
	rows, err := s.Profiles(r.Context(), kind)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]profileListRow, len(rows))
	for i, row := range rows {
		out[i] = profileListRow{ProfileRow: row, IsDefault: kind != "" && slices.Contains(row.Default, kind)}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Service) getProfile(w http.ResponseWriter, r *http.Request) {
	row, err := s.Profile(r.Context(), r.PathValue("key"), domain.MediaKind(r.URL.Query().Get("kind")))
	reply(w, http.StatusOK, row, err)
}

func (s *Service) createProfile(w http.ResponseWriter, r *http.Request) {
	var d decide.ProfileDoc
	if !api.DecodeJSON(w, r, &d) {
		return
	}
	row, err := s.CreateProfile(r.Context(), d)
	reply(w, http.StatusCreated, row, err)
}

func (s *Service) updateProfile(w http.ResponseWriter, r *http.Request) {
	var d decide.ProfileDoc
	if !api.DecodeJSON(w, r, &d) {
		return
	}
	row, err := s.UpdateProfile(r.Context(), r.PathValue("key"), d)
	reply(w, http.StatusOK, row, err)
}

func (s *Service) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.DeleteProfile(r.Context(), r.PathValue("key")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) duplicateProfile(w http.ResponseWriter, r *http.Request) {
	var req DuplicateRequest
	if r.ContentLength != 0 && !api.DecodeJSON(w, r, &req) {
		return
	}
	row, err := s.DuplicateProfile(r.Context(), r.PathValue("key"), req)
	reply(w, http.StatusCreated, row, err)
}

func choices(w http.ResponseWriter, _ *http.Request) {
	api.WriteJSON(w, http.StatusOK, EditorChoices())
}
