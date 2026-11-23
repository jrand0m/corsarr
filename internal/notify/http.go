package notify

import (
	"errors"
	"net/http"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers POST /api/v1/notifications/{key}/test: it renders a sample
// import through the connection's template and sends it (?send=false only
// previews).
func (s *Service) Mount(m api.Mounter) {
	m.Handle("POST /api/v1/notifications/{key}/test", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Test(r.Context(), domain.ObjectKey(r.PathValue("key")), r.URL.Query().Get("send") != "false")
		if errors.Is(err, ErrUnknownConnection) {
			api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "key")
			return
		}
		api.WriteJSON(w, http.StatusOK, p)
	})
}
