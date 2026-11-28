package migrate

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Mount registers the migration endpoints.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("POST /api/v1/migration/plan", s.handleRun("plan"))
	m.Handle("POST /api/v1/migration/apply", s.handleRun("apply"))
	m.Handle("POST /api/v1/migration/check/source", s.checkSource)
	m.Handle("POST /api/v1/migration/check/paths", s.checkPaths)
	m.Handle("GET /api/v1/migration/runs", s.listRuns)
	m.Handle("GET /api/v1/migration/runs/{id}/report", s.runReport)
	m.Handle("GET /api/v1/migration/modes", s.listModes)
	m.Handle("PUT /api/v1/migration/modes/{kind}", s.setMode)
}

type sourceBody struct {
	App    string `json:"app"`
	URL    string `json:"url"`
	APIKey Secret `json:"apiKey"`
}

// runBody is the plan/apply request.
type runBody struct {
	Sources     []sourceBody `json:"sources"`
	Scope       []string     `json:"scope"`
	PathRewrite string       `json:"pathRewrite"`
	HistoryDays int          `json:"historyDays"`
	Activate    bool         `json:"activate"`
	Exclude     []string     `json:"exclude"`
	PlanHash    string       `json:"planHash"`
}

// request parses the body once, at the boundary.
func (b runBody) request() (Request, error) {
	var req Request
	for _, sb := range b.Sources {
		ep, err := ParseEndpoint(sb.App, sb.URL, sb.APIKey)
		if err != nil {
			return req, err
		}
		req.Sources = append(req.Sources, ep)
	}
	var err error
	if req.Options.Scope, err = ParseScope(joinComma(b.Scope)); err != nil {
		return req, err
	}
	if req.Options.Rewrite, err = ParseRewrite(b.PathRewrite); err != nil {
		return req, err
	}
	req.Options.HistoryDays, req.Options.Activate = b.HistoryDays, b.Activate
	req.Options.Exclude = map[string]bool{}
	for _, id := range b.Exclude {
		req.Options.Exclude[id] = true
	}
	return req, nil
}

func joinComma(list []string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

// handleRun streams progress lines, then the report in the summary line.
func (s *Service) handleRun(mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body runBody
		if !api.DecodeJSON(w, r, &body) {
			return
		}
		req, err := body.request()
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "")
			return
		}
		nd := api.NewNDJSON(w)
		progress := func(p Progress) { _ = nd.Record(map[string]any{"progress": p}) }
		var rep Report
		if mode == "apply" {
			rep, err = s.Apply(r.Context(), req, body.PlanHash, progress)
		} else {
			rep, err = s.Plan(r.Context(), req, progress)
		}
		if err != nil {
			_ = nd.Error("migration", err)
		}
		_ = nd.Done(map[string]any{"report": rep, "planChanged": errors.Is(err, ErrPlanChanged)})
	}
}

// guard admits one quick check under the rate limit, or answers 429.
func (s *Service) guard(w http.ResponseWriter) bool {
	if s.checks.enter(s.d.Now()) {
		return true
	}
	w.Header().Set("Retry-After", "1")
	api.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many checks at once; try again in a moment", "")
	return false
}

// checkSource tests one source (the key is optional) and answers the result;
// the key is only sent to the source, never logged or echoed.
func (s *Service) checkSource(w http.ResponseWriter, r *http.Request) {
	var body sourceBody
	if !api.DecodeJSON(w, r, &body) {
		return
	}
	ep, err := ParseTarget(body.App, body.URL, body.APIKey)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "url")
		return
	}
	if !s.guard(w) {
		return
	}
	defer s.checks.leave()
	api.WriteJSON(w, http.StatusOK, s.Check(r.Context(), ep))
}

// pathsBody is the path check request.
type pathsBody struct {
	Sources    []sourceBody `json:"sources"`
	FromPrefix string       `json:"fromPrefix"`
	ToPrefix   string       `json:"toPrefix"`
}

func (b pathsBody) parse() ([]Endpoint, Rewrite, error) {
	var rw Rewrite
	var err error
	if rw.From, err = ParsePrefix("source path prefix", b.FromPrefix); err != nil {
		return nil, rw, err
	}
	if rw.To, err = ParsePrefix("corsarr path prefix", b.ToPrefix); err != nil {
		return nil, rw, err
	}
	eps := make([]Endpoint, 0, len(b.Sources))
	for _, sb := range b.Sources {
		ep, err := ParseEndpoint(sb.App, sb.URL, sb.APIKey)
		if err != nil {
			return nil, rw, err
		}
		eps = append(eps, ep)
	}
	return eps, rw, nil
}

// checkPaths reports how the path rewrite maps the sources' root folders.
func (s *Service) checkPaths(w http.ResponseWriter, r *http.Request) {
	var body pathsBody
	if !api.DecodeJSON(w, r, &body) {
		return
	}
	eps, rw, err := body.parse()
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "")
		return
	}
	if !s.guard(w) {
		return
	}
	defer s.checks.leave()
	api.WriteJSON(w, http.StatusOK, s.CheckPaths(r.Context(), eps, rw))
}

func (s *Service) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.Runs(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	if runs == nil {
		runs = []store.MigrationRun{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": runs})
}

func (s *Service) runReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a positive number", "id")
		return
	}
	body, ok, err := s.RunReport(r.Context(), id)
	switch {
	case err != nil:
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
	case !ok || len(body) == 0:
		api.WriteError(w, http.StatusNotFound, "not_found", "no report for this run", "id")
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="migration-`+strconv.FormatInt(id, 10)+`.json"`)
		_, _ = w.Write(body)
	}
}

// KindState is a media kind's mode and who decides it.
type KindState struct {
	Kind    domain.MediaKind `json:"kind"`
	Mode    domain.KindMode  `json:"mode"`
	Managed bool             `json:"managedByConfig"` // the config file sets it; changes go there
	Snippet string           `json:"snippet,omitempty"`
	By      *config.Managed  `json:"managedBy,omitempty"` // the key and the source that set it
}

var allKinds = []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic, domain.KindBook}

// Modes lists every kind's effective mode.
func (s *Service) Modes() []KindState {
	out := make([]KindState, 0, len(allKinds))
	for _, k := range allKinds {
		ks := KindState{Kind: k, Mode: s.mode(k)}
		if cfg := s.d.Config(); cfg != nil && ConfigSetsKind(cfg, k) {
			ks.Managed, ks.Snippet = true, kindSnippet(k, domain.ModeActive)
			ks.By = cfg.ManagedBy(kindModeKey(k))
		}
		out = append(out, ks)
	}
	return out
}

func (s *Service) mode(k domain.MediaKind) domain.KindMode {
	if s.d.Mode == nil {
		return domain.ModeActive
	}
	return s.d.Mode(k)
}

func kindSnippet(k domain.MediaKind, m domain.KindMode) string {
	return "mediaKinds:\n  " + string(k) + ":\n    mode: " + string(m) + "\n"
}

func (s *Service) listModes(w http.ResponseWriter, _ *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": s.Modes()})
}

// ErrManagedByConfig refuses a mode change the config file decides.
var ErrManagedByConfig = errors.New("the config file sets this kind's mode; change it there")

// SetMode switches a kind between shadow and active without a restart.
func (s *Service) SetMode(ctx context.Context, k domain.MediaKind, m domain.KindMode) error {
	if cfg := s.d.Config(); cfg != nil && ConfigSetsKind(cfg, k) {
		return ErrManagedByConfig
	}
	if err := s.d.Store.ModeOverrides().Set(ctx, store.ModeOverride{Scope: store.ScopeKind, Name: string(k), Mode: string(m),
		SetAt: s.d.Now()}); err != nil {
		return err
	}
	if s.d.AfterApply != nil {
		s.d.AfterApply(ctx)
	}
	return nil
}

func (s *Service) setMode(w http.ResponseWriter, r *http.Request) {
	k, err := domain.ParseMediaKind(r.PathValue("kind"))
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "kind")
		return
	}
	var body struct {
		Mode domain.KindMode `json:"mode"`
	}
	if !api.DecodeJSON(w, r, &body) {
		return
	}
	if body.Mode != domain.ModeShadow && body.Mode != domain.ModeActive {
		api.WriteError(w, http.StatusBadRequest, "invalid", "mode must be shadow or active", "mode")
		return
	}
	switch err := s.SetMode(r.Context(), k, body.Mode); {
	case errors.Is(err, ErrManagedByConfig):
		api.WriteError(w, http.StatusConflict, "managed_by_config", err.Error()+":\n"+kindSnippet(k, body.Mode), "mode")
	case err != nil:
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
	default:
		api.WriteJSON(w, http.StatusOK, KindState{Kind: k, Mode: s.mode(k)})
	}
}
