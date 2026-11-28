package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Prompt is a ready-made request a person can pick in their assistant; the
// server fills it with the library's current facts.
type Prompt struct {
	Name        string
	Title       string
	Description string
	Arguments   []PromptArg
	Build       func(ctx context.Context, args map[string]string) (string, error)
}

// PromptArg is one prompt argument.
type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

func (s *Service) prompts() []Prompt {
	return []Prompt{
		{Name: "weekly_digest", Title: "Weekly digest",
			Description: "What happened in the library this week and what is coming next week.",
			Build:       s.weeklyDigest},
		{Name: "find_something_to_watch", Title: "Find something to watch",
			Description: "Suggest something to watch from what is already in the library.",
			Arguments: []PromptArg{{Name: "mood", Description: "What the person feels like, e.g. a light comedy or something tense."},
				{Name: "kind", Description: "movie or tv; both when omitted."}},
			Build: s.somethingToWatch},
		{Name: "why_not_downloaded", Title: "Why wasn't it downloaded?",
			Description: "Explain why an item has not been downloaded, from its last search's decisions, and what would fix it.",
			Arguments: []PromptArg{{Name: "kind", Description: "movie, tv or music.", Required: true},
				{Name: "title", Description: "The item's title or name as it is in the library."},
				{Name: "id", Description: "The item's library id, instead of the title."}},
			Build: s.whyNotDownloaded},
	}
}

func (s *Service) listPrompts(context.Context, *Session, json.RawMessage) (any, error) {
	out := []map[string]any{}
	for _, p := range s.prompts() {
		args := p.Arguments
		if args == nil {
			args = []PromptArg{}
		}
		out = append(out, map[string]any{"name": p.Name, "title": p.Title, "description": p.Description, "arguments": args})
	}
	return map[string]any{"prompts": out}, nil
}

type promptParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

func (s *Service) getPrompt(ctx context.Context, sess *Session, params json.RawMessage) (any, error) {
	var p promptParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	if !s.allow(sess) {
		return nil, rpcErr(codeRateLimited, "rate limit reached: at most %d calls per minute", s.cfg().RateLimit)
	}
	for _, pr := range s.prompts() {
		if pr.Name != p.Name {
			continue
		}
		text, err := pr.Build(ctx, p.Arguments)
		if err != nil {
			return nil, rpcErr(codeInvalidParams, "%s: %s", p.Name, errText(err))
		}
		return map[string]any{"description": pr.Description, "messages": []map[string]any{
			{"role": "user", "content": map[string]string{"type": "text", "text": text}}}}, nil
	}
	return nil, rpcErr(codeInvalidParams, "unknown prompt %q", p.Name)
}

// ── weekly digest ────────────────────────────────────────────────────────────

func (s *Service) weeklyDigest(ctx context.Context, _ map[string]string) (string, error) {
	now := s.d.Now().UTC()
	var b strings.Builder
	b.WriteString("Write a short weekly digest of my corsarr media library: what was added and downloaded in the last 7 days, " +
		"what is downloading now, what is still missing, what airs or releases in the next 7 days and anything unhealthy. " +
		"Group it by media kind and keep it brief. Use get_history, get_queue, list_wanted and get_calendar for details.\n\n")
	week := s.recentHistory(ctx, now.Add(-7*24*time.Hour))
	fmt.Fprintf(&b, "Activity in the last 7 days: %s.\n", week)
	for _, part := range []func(context.Context) (Result, error){
		func(ctx context.Context) (Result, error) {
			return s.getCalendar(ctx, calendarArgs{From: now.Format(time.DateOnly)})
		},
		func(ctx context.Context) (Result, error) { return s.getHealth(ctx, struct{}{}) },
	} {
		res, err := part(ctx)
		if err != nil {
			return "", err
		}
		b.WriteString(res.Text + ".\n")
	}
	return b.String(), nil
}

// recentHistory counts the event types since a time, from the newest page
// of History.
func (s *Service) recentHistory(ctx context.Context, since time.Time) string {
	list, _, err := s.d.History.List(ctx, 0, MaxLimit)
	if err != nil {
		return "unavailable (" + err.Error() + ")"
	}
	counts, order := map[domain.EventType]int{}, []domain.EventType{}
	for _, e := range list {
		if e.Event.OccurredAt.Before(since) {
			continue
		}
		if counts[e.Event.Type] == 0 {
			order = append(order, e.Event.Type)
		}
		counts[e.Event.Type]++
	}
	if len(order) == 0 {
		return "nothing recorded"
	}
	parts := make([]string, len(order))
	for i, t := range order {
		parts[i] = fmt.Sprintf("%d %s", counts[t], t)
	}
	return strings.Join(parts, ", ")
}

// ── something to watch ───────────────────────────────────────────────────────

// maxWatchList bounds the titles a prompt carries.
const maxWatchList = 100

func (s *Service) somethingToWatch(ctx context.Context, args map[string]string) (string, error) {
	kinds := []domain.MediaKind{domain.KindMovie, domain.KindTV}
	if k := args["kind"]; k != "" {
		kind, err := s.kindArg(k)
		if err != nil {
			return "", err
		}
		kinds = []domain.MediaKind{kind}
	}
	var titles []string
	for _, k := range kinds {
		if s.usable(k) != nil {
			continue
		}
		err := s.eachItem(ctx, k, func(it LibraryItem) error {
			if len(titles) < maxWatchList {
				titles = append(titles, fmt.Sprintf("- [%s %d] %s%s", it.Kind, it.ID, it.Title, yearText(it.Year)))
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	mood := strings.TrimSpace(args["mood"])
	if mood == "" {
		mood = "anything good"
	}
	return fmt.Sprintf("Suggest up to three things to watch from my library for this mood: %s. Only pick titles from the list "+
		"below; use get_item to check what is downloaded and get_item's overview to explain each pick in one line.\n\nIn the library:\n%s\n",
		mood, strings.Join(titles, "\n")), nil
}

func yearText(y int) string {
	if y == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d)", y)
}

// ── why not downloaded ───────────────────────────────────────────────────────

func (s *Service) whyNotDownloaded(ctx context.Context, args map[string]string) (string, error) {
	kind, err := s.kindArg(args["kind"])
	if err != nil {
		return "", err
	}
	t, err := s.promptTarget(ctx, kind, args)
	if err != nil {
		return "", err
	}
	st, err := s.status(ctx, kind, t)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Explain in plain words why %q has not been downloaded and what I could do about it (for example a different "+
		"quality profile, grabbing a rejected release with search_releases and grab_release, or adding a download by URL). "+
		"Use request_status and search_releases if you need more.\n\n", st.Title)
	fmt.Fprintf(&b, "Status: %s — %s.\n", st.State, st.Reason)
	b.WriteString(searchFacts(st.LastSearch))
	for _, d := range st.Downloads {
		fmt.Fprintf(&b, "Download: %s on %s, %s, %.0f%%.\n", d.Title, d.Downloader, d.State, d.Progress*100)
	}
	return b.String(), nil
}

// searchFacts describes the last search and its rejection reasons.
func searchFacts(ls *domain.SearchState) string {
	if ls == nil || ls.LastResult == nil {
		return "No search has finished for it yet.\n"
	}
	r := ls.LastResult
	out := fmt.Sprintf("Last search %s: %d indexers (%d failed), %d releases, %d accepted.\n",
		ls.LastSearched.UTC().Format(time.RFC3339), r.Indexers, r.Failed, r.Releases, r.Accepted)
	for _, rc := range r.Rejections {
		out += fmt.Sprintf("Rejected %d× by %s (%s): %s\n", rc.Count, rc.RuleID, rc.Category, rc.Message)
	}
	return out
}

// promptTarget finds the item by id or by its title in the library.
func (s *Service) promptTarget(ctx context.Context, kind domain.MediaKind, args map[string]string) (target, error) {
	if id := args["id"]; id != "" {
		var n int64
		if _, err := fmt.Sscan(id, &n); err != nil || n <= 0 {
			return target{}, invalidArg("id", "must be a library id")
		}
		return target{Kind: string(kind), ID: n}, nil
	}
	title := strings.ToLower(strings.TrimSpace(args["title"]))
	if title == "" {
		return target{}, invalidArg("title", "give the title or the id")
	}
	var found target
	err := s.eachItem(ctx, kind, func(it LibraryItem) error {
		if strings.Contains(strings.ToLower(it.Title), title) {
			found = target{Kind: string(kind), ID: it.ID}
			return stop
		}
		return nil
	})
	return found, walkDone(errOr(err, found))
}

func errOr(err error, t target) error {
	if err == nil && t.ID == 0 {
		return errNotInLibrary
	}
	return err
}
