package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
)

// Downloaders is the download-client control surface (queue.Service).
type Downloaders interface {
	Clients(ctx context.Context) []queue.ClientView
	ListFiltered(ctx context.Context, f queue.Filter, cursor int64, limit int) ([]store.StoredDownload, int64, error)
	Detail(ctx context.Context, id int64) (queue.Detail, error)
	Pause(ctx context.Context, id int64) error
	Resume(ctx context.Context, id int64) error
	SetPriority(ctx context.Context, id int64, p plugin.Priority) error
	Recheck(ctx context.Context, id int64) error
	Reannounce(ctx context.Context, id int64) error
	SetCategory(ctx context.Context, id int64, category string) error
	Remove(ctx context.Context, id int64, o queue.RemoveOptions) error
	RetryImport(ctx context.Context, id int64) error
	PauseAll(ctx context.Context, key domain.ObjectKey) error
	ResumeAll(ctx context.Context, key domain.ObjectKey) error
	SetSpeedLimits(ctx context.Context, key domain.ObjectKey, l plugin.SpeedLimits) error
	SetAltSpeed(ctx context.Context, key domain.ObjectKey, on *bool) (bool, error)
	TestClient(ctx context.Context, key domain.ObjectKey) error
	AddURL(ctx context.Context, req queue.URLRequest) (domain.Grab, error)
}

// ManualGrab grabs a release from a recent search by hand (the movie, TV
// and music search services).
type ManualGrab interface {
	GrabReleaseTo(ctx context.Context, itemID int64, releaseID string, confirm bool, to domain.ObjectKey) (domain.Grab, automation.Decided, error)
}

// downloaderTools are the download-client tools: the queue, per-download
// and per-client control and manual grabs. Every action goes through the
// queue service, so shadow mode records instead of sending and a client
// without a capability answers unsupported.
func (s *Service) downloaderTools() []Tool {
	if s.d.Downloaders == nil {
		return nil
	}
	tools := []Tool{
		{Name: "list_downloaders", Title: "List download clients",
			Description: "Every download client with its health, protocols, capabilities, speeds, free space, speed limits and paused state.",
			Input:       object(map[string]schema{}), Run: bind(s.listDownloaders)},
		{Name: "get_queue", Title: "Download queue", Description: "Downloads with progress, ETA, ratio, seeds and peers and the linked item, newest first.",
			Input: object(withPaging(map[string]schema{"downloader": str("Only this client (key from list_downloaders)."),
				"kind":   enum("Only this media kind.", "movie", "tv", "music", "book"),
				"states": array("Only these states, e.g. downloading, stalled, completed, failed.", str("A download state."))})),
			Run: bind(s.getQueue)},
		{Name: "get_download", Title: "Get a download", Description: "One download in detail: files, trackers and its history of state changes.",
			Input: object(map[string]schema{"id": integer("Download id from get_queue.")}, "id"), Run: bind(s.getDownload)},
		{Name: "set_download_priority", Title: "Set download priority", Write: true, Idempotent: true,
			Description: "Move a download in its client's queue.",
			Input:       object(map[string]schema{"id": integer("Download id."), "priority": enum("Queue move (top, up, down, bottom) or priority level (low, normal, high, force; force starts it regardless of queue limits).", "top", "up", "down", "bottom", "low", "normal", "high", "force")}, "id", "priority"),
			Run:         bind(s.setPriority)},
		{Name: "set_download_category", Title: "Set download category", Write: true, Idempotent: true,
			Description: "Change a download's category in its client.",
			Input:       object(map[string]schema{"id": integer("Download id."), "category": str("The category.")}, "id", "category"),
			Run:         bind(s.setCategory)},
		{Name: "remove_download", Title: "Remove a download", Write: true, Destructive: true, Idempotent: true,
			Description: "Remove a download from its client; optionally delete its data, blocklist the release and search for another.",
			Input: object(map[string]schema{"id": integer("Download id."), "deleteFiles": boolean("Delete the downloaded data."),
				"blocklist": boolean("Never grab this release again."), "searchAgain": boolean("Search for another release.")}, "id"),
			Run: bind(s.removeDownload)},
		{Name: "set_speed_limits", Title: "Set speed limits", Write: true, Idempotent: true,
			Description: "Set a client's global download and upload limits in KiB/s; 0 means unlimited.",
			Input: object(map[string]schema{"downloader": str("Client key."), "downKiB": integer("Download limit in KiB/s."),
				"upKiB": integer("Upload limit in KiB/s.")}, "downloader"), Run: bind(s.setSpeedLimits)},
		{Name: "toggle_alt_speed", Title: "Alternative speed mode", Write: true,
			Description: "Switch a client's alternative (scheduler) speed limits on or off; without on, flip them.",
			Input:       object(map[string]schema{"downloader": str("Client key."), "on": boolean("Turn the alternative limits on or off.")}, "downloader"),
			Run:         bind(s.toggleAltSpeed)},
		{Name: "test_downloader", Title: "Test a download client", Description: "Check that corsarr can reach and log in to a client.",
			Input: object(map[string]schema{"downloader": str("Client key.")}, "downloader"), Run: bind(s.testDownloader)},
		{Name: "grab_release", Title: "Grab a release", Write: true, OpenWorld: true,
			Description: "Grab one release from a recent search_releases result for the item, on the given download client or the first one that takes it. A rejected release needs override: true.",
			Input: object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Movie id, series id or music album id."),
				"releaseId": str("The release's id (decision.releaseId in search_releases)."), "override": boolean("Grab even though it was rejected."),
				"downloader": str("Client key from list_downloaders; the first client that takes the release when omitted.")},
				"kind", "id", "releaseId"), Run: bind(s.grabRelease)},
		s.addURLTool(),
	}
	for _, a := range s.downloadActions() {
		tools = append(tools, a.tool())
	}
	for _, a := range s.clientActions() {
		tools = append(tools, a.tool())
	}
	return tools
}

// ── per-download and per-client one-shot actions ─────────────────────────────

type action[K any] struct {
	name, title, doc string
	idempotent       bool
	input            schema
	run              func(context.Context, K) error
	arg              func(idArgs) (K, error)
}

type idArgs struct {
	ID         int64  `json:"id"`
	Downloader string `json:"downloader"`
}

func (a action[K]) tool() Tool {
	return Tool{Name: a.name, Title: a.title, Description: a.doc, Write: true, Idempotent: a.idempotent, Input: a.input,
		Run: bind(func(ctx context.Context, in idArgs) (Result, error) {
			k, err := a.arg(in)
			if err != nil {
				return Result{}, err
			}
			return actionResult(a.name, a.run(ctx, k))
		})}
}

func downloadID(in idArgs) (int64, error) {
	if in.ID <= 0 {
		return 0, invalidArg("id", "give the download id from get_queue")
	}
	return in.ID, nil
}

func clientKey(in idArgs) (domain.ObjectKey, error) {
	if in.Downloader == "" {
		return "", invalidArg("downloader", "give the client key from list_downloaders")
	}
	return domain.ObjectKey(in.Downloader), nil
}

func (s *Service) downloadActions() []action[int64] {
	in := object(map[string]schema{"id": integer("Download id from get_queue.")}, "id")
	dl := s.d.Downloaders
	mk := func(name, title, doc string, run func(context.Context, int64) error) action[int64] {
		return action[int64]{name: name, title: title, doc: doc, idempotent: true, input: in, run: run, arg: downloadID}
	}
	return []action[int64]{
		mk("pause_download", "Pause a download", "Pause one download in its client.", dl.Pause),
		mk("resume_download", "Resume a download", "Resume one paused download.", dl.Resume),
		mk("recheck_download", "Recheck a download", "Re-verify a download's data in its client.", dl.Recheck),
		mk("reannounce_download", "Reannounce a download", "Ask the trackers for peers again now.", dl.Reannounce),
		mk("retry_import", "Retry an import", "Run the import again for a finished download that failed or was held back.", dl.RetryImport),
	}
}

func (s *Service) clientActions() []action[domain.ObjectKey] {
	in := object(map[string]schema{"downloader": str("Client key from list_downloaders.")}, "downloader")
	dl := s.d.Downloaders
	return []action[domain.ObjectKey]{
		{name: "pause_all", title: "Pause a client", doc: "Pause every download of one client.", idempotent: true, input: in, run: dl.PauseAll, arg: clientKey},
		{name: "resume_all", title: "Resume a client", doc: "Resume every download of one client.", idempotent: true, input: in, run: dl.ResumeAll, arg: clientKey},
	}
}

// actionResult reports a control action: done, recorded in shadow mode, or
// unsupported by the client.
func actionResult(what string, err error) (Result, error) {
	switch {
	case errors.Is(err, queue.ErrShadowed):
		return Result{Text: what + ": recorded, not sent (shadow mode)",
			Data: ActionOut{Action: what, Recorded: true, Reason: err.Error()}}, nil
	case errors.Is(err, plugin.ErrUnsupported):
		return Result{}, unsupported(err.Error())
	case err != nil:
		return Result{}, err
	}
	return Result{Text: what + ": done", Data: ActionOut{Action: what, Sent: true}}, nil
}

// ── reads ────────────────────────────────────────────────────────────────────

func (s *Service) listDownloaders(ctx context.Context, _ struct{}) (Result, error) {
	list := s.d.Downloaders.Clients(ctx)
	return Result{Text: fmt.Sprintf("%d download clients", len(list)), Data: DownloadersOut{Downloaders: list}}, nil
}

type queueArgs struct {
	paging
	Downloader string   `json:"downloader"`
	Kind       string   `json:"kind"`
	States     []string `json:"states"`
}

func (s *Service) getQueue(ctx context.Context, in queueArgs) (Result, error) {
	cursor, err := in.offset()
	if err != nil {
		return Result{}, err
	}
	f := queue.Filter{Downloader: domain.ObjectKey(in.Downloader), Kind: domain.MediaKind(in.Kind)}
	for _, st := range in.States {
		f.States = append(f.States, domain.DownloadState(st))
	}
	list, next, err := s.d.Downloaders.ListFiltered(ctx, f, cursor, in.size())
	p := page[store.StoredDownload]{Items: list, NextCursor: nextCursor(next)}
	return Result{Text: countText(len(list), "downloads", p.NextCursor), Data: p}, err
}

func (s *Service) getDownload(ctx context.Context, in idArgs) (Result, error) {
	d, err := s.d.Downloaders.Detail(ctx, in.ID)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("%s: %s, %.0f%%", d.Title, d.State, d.Progress*100), Data: d}, nil
}

func (s *Service) testDownloader(ctx context.Context, in idArgs) (Result, error) {
	key, err := clientKey(in)
	if err == nil {
		err = s.d.Downloaders.TestClient(ctx, key)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Text: in.Downloader + ": ok", Data: TestOut{Downloader: in.Downloader, OK: true}}, nil
}

// ── writes with arguments ────────────────────────────────────────────────────

type priorityArgs struct {
	ID       int64  `json:"id"`
	Priority string `json:"priority"`
}

func (s *Service) setPriority(ctx context.Context, in priorityArgs) (Result, error) {
	if p := plugin.Priority(in.Priority); p.IsMove() || p.IsLevel() {
		return actionResult("set_download_priority", s.d.Downloaders.SetPriority(ctx, in.ID, p))
	}
	return Result{}, invalidArg("priority", "must be top, up, down, bottom, low, normal, high or force")
}

type categoryArgs struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
}

func (s *Service) setCategory(ctx context.Context, in categoryArgs) (Result, error) {
	return actionResult("set_download_category", s.d.Downloaders.SetCategory(ctx, in.ID, in.Category))
}

type removeArgs struct {
	ID          int64 `json:"id"`
	DeleteFiles bool  `json:"deleteFiles"`
	Blocklist   bool  `json:"blocklist"`
	SearchAgain bool  `json:"searchAgain"`
}

func (s *Service) removeDownload(ctx context.Context, in removeArgs) (Result, error) {
	o := queue.RemoveOptions{DeleteFiles: in.DeleteFiles, Blocklist: in.Blocklist, Research: in.SearchAgain}
	return actionResult("remove_download", s.d.Downloaders.Remove(ctx, in.ID, o))
}

type speedArgs struct {
	Downloader string `json:"downloader"`
	DownKiB    int64  `json:"downKiB"`
	UpKiB      int64  `json:"upKiB"`
}

func (s *Service) setSpeedLimits(ctx context.Context, in speedArgs) (Result, error) {
	key, err := clientKey(idArgs{Downloader: in.Downloader})
	if err != nil {
		return Result{}, err
	}
	return actionResult("set_speed_limits", s.d.Downloaders.SetSpeedLimits(ctx, key, plugin.SpeedLimits{DownKiB: in.DownKiB, UpKiB: in.UpKiB}))
}

type altArgs struct {
	Downloader string `json:"downloader"`
	On         *bool  `json:"on"`
}

func (s *Service) toggleAltSpeed(ctx context.Context, in altArgs) (Result, error) {
	key, err := clientKey(idArgs{Downloader: in.Downloader})
	if err != nil {
		return Result{}, err
	}
	on, err := s.d.Downloaders.SetAltSpeed(ctx, key, in.On)
	res, err := actionResult("toggle_alt_speed", err)
	if out, ok := res.Data.(ActionOut); ok {
		out.On = &on
		res.Data = out
	}
	return res, err
}

// ── grab_release ─────────────────────────────────────────────────────────────

type grabArgs struct {
	Kind       string `json:"kind"`
	ID         int64  `json:"id"`
	ReleaseID  string `json:"releaseId"`
	Override   bool   `json:"override"`
	Downloader string `json:"downloader"`
}

func (s *Service) grabRelease(ctx context.Context, in grabArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	grabber := map[domain.MediaKind]ManualGrab{domain.KindMovie: s.d.MovieSearch, domain.KindTV: s.d.TVSearch, domain.KindMusic: s.d.MusicSearch}[kind]
	g, d, err := grabber.GrabReleaseTo(ctx, in.ID, in.ReleaseID, in.Override, domain.ObjectKey(in.Downloader))
	if errors.Is(err, automation.ErrShadowed) {
		return Result{Text: "grab recorded, not sent (shadow mode): " + d.Release.Title,
			Data: GrabOut{Recorded: true, Release: &d.Release, Decision: d.Decision}}, nil
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("grabbed %s on %s", d.Release.Title, g.Downloader),
		Data: GrabOut{Sent: true, Grab: &g, Decision: d.Decision}}, nil
}
