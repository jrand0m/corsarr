// Package importer turns completed downloads into library files:
// on download.completed it requests import-pending → importing from the
// queue, scans and classifies the payload, matches the linked movie,
// hardlinks (copy fallback) into the folder from the naming templates, runs
// the upgrade check, recycles the replaced file through the rule runner,
// reports the ImportOutcome to the queue and publishes import.*. Imports are
// idempotent per (download id, file).
package importer

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
	"github.com/jrand0m/corsarr/internal/rootfolders"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/store"
)

// Queue is the part of the queue module the importer talks to.
type Queue interface {
	Get(ctx context.Context, id int64) (store.StoredDownload, error)
	GrabOf(ctx context.Context, d store.StoredDownload) (domain.Grab, bool, error)
	Transition(ctx context.Context, id int64, to domain.DownloadState, by domain.Actor, reason string) error
	ReportImport(ctx context.Context, o domain.ImportOutcome) error
}

// Movies is the part of the movies module the importer needs.
type Movies interface {
	Get(ctx context.Context, id int64) (domain.Movie, error)
	SetFile(ctx context.Context, id, fileID int64) error
}

// Library is the importer's persistence (store.Library implements it).
type Library interface {
	SaveFile(ctx context.Context, f domain.LibraryFile) (domain.LibraryFile, error)
	File(ctx context.Context, id int64) (domain.LibraryFile, bool, error)
	FilesOf(ctx context.Context, item domain.ItemRef) ([]domain.LibraryFile, error)
	DeleteFile(ctx context.Context, id int64) error
	SetPath(ctx context.Context, id int64, path string) error
	ImportRecord(ctx context.Context, downloadID int64, source string) (string, int64, bool, error)
	RecordImport(ctx context.Context, downloadID int64, source, dest string, fileID int64) error
}

// RuleStore lets the purge rule find old recycle entries (store.Rules).
type RuleStore interface {
	ActedBefore(ctx context.Context, ruleID string, t time.Time, fn func(store.RuleAction) error) error
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the importer.
type Deps struct {
	Queue     Queue
	Movies    Movies
	TV        TV    // nil = no TV imports
	Music     Music // nil = no music imports
	Library   Library
	Rules     *rules.Runner
	RuleStore RuleStore
	Pub       Publisher
	DataDir   string
	Payload   decide.PayloadPolicy
	FreeSpace func(path string) int64
	// Files lists a download's files relative to its save path, as the
	// download client reports them; nil when no client can tell.
	Files func(ctx context.Context, d store.StoredDownload) ([]string, error)
	// NoSpace is told the root folder an import found short of space, so the
	// root folder's health updates at once; may be nil.
	NoSpace func(root string)
	Modes   domain.Modes // media-kind modes; nil = all active
	// Naming is each kind's file naming; nil = the built-in naming.
	Naming func(domain.MediaKind) naming.Scheme
	Now    func() time.Time
	Log    *slog.Logger
}

// Service is the import module.
type Service struct {
	d     Deps
	lanes *lanes
}

// New returns the importer.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.FreeSpace == nil {
		d.FreeSpace = rootfolders.FreeSpace
	}
	return &Service{d: d, lanes: newLanes()}
}

// result is one import's outcome plus what follows from it.
type result struct {
	outcome domain.ImportOutcome
	folders []string
	unsafe  []string
	items   []domain.EventItem
}

// OnDownloadCompleted queues the import of one completed download.
func (s *Service) OnDownloadCompleted(ctx context.Context, ev domain.Event) {
	data, ok := ev.Data.(domain.DownloadEventData)
	if !ok {
		return
	}
	s.Enqueue(ctx, data.DownloadID)
}

// Enqueue queues an import on its root folder's lane: imports into one root
// folder run one at a time, different root folders in parallel.
func (s *Service) Enqueue(ctx context.Context, downloadID int64) {
	s.lanes.add(ctx, s.laneOf(ctx, downloadID), downloadID, func(ctx context.Context, id int64) {
		s.warn("import", s.Import(ctx, id), "download", id)
	})
}

// Wait blocks until every queued import has run (shutdown and tests).
func (s *Service) Wait() { s.lanes.wait() }

// laneOf is the root folder of the download's linked movie ("" when unknown:
// the import itself reports the problem).
func (s *Service) laneOf(ctx context.Context, downloadID int64) string {
	d, err := s.d.Queue.Get(ctx, downloadID)
	if err != nil {
		return ""
	}
	m, err := s.linkedMovie(ctx, d)
	if err != nil {
		return ""
	}
	return m.RootFolder
}

// Import imports one download; re-running it after a crash ends in the same state.
func (s *Service) Import(ctx context.Context, downloadID int64) error {
	d, err := s.d.Queue.Get(ctx, downloadID)
	if err != nil {
		return err
	}
	if !s.d.Modes.Active(d.Kind) {
		// shadow/disabled: no import, no exception; it waits in completed
		// and the reconciliation sweep picks it up once the kind is active
		s.d.Log.Info("import skipped: media kind not active", "download", downloadID, "kind", d.Kind)
		return nil
	}
	if d.OnHold() {
		// held: no automatic import; releasing the hold hands it back
		s.d.Log.Info("import skipped: download on hold", "download", downloadID)
		return nil
	}
	if !s.begin(ctx, d) {
		return nil
	}
	r := s.run(ctx, d)
	if err := s.d.Queue.ReportImport(ctx, r.outcome); err != nil {
		return err
	}
	s.announce(r)
	return nil
}

// begin requests import-pending → importing, resuming an interrupted import.
func (s *Service) begin(ctx context.Context, d store.StoredDownload) bool {
	steps := map[domain.DownloadState][]domain.DownloadState{
		domain.StateCompleted:     {domain.StateImportPending, domain.StateImporting},
		domain.StateImportPending: {domain.StateImporting},
		domain.StateImportBlocked: {domain.StateImporting},
		domain.StateImporting:     {},
	}
	path, ok := steps[d.State]
	for _, to := range path {
		if s.d.Queue.Transition(ctx, d.ID, to, domain.ActorImport, "") != nil {
			return false
		}
	}
	return ok
}

func (s *Service) announce(r result) {
	t := map[domain.ImportStatus]domain.EventType{domain.ImportImported: domain.EvImportCompleted, domain.ImportPartial: domain.EvImportCompleted,
		domain.ImportBlocked: domain.EvImportBlocked, domain.ImportFailed: domain.EvImportFailed}[r.outcome.Status]
	s.publish(t, r.items, domain.ImportEventData{Outcome: r.outcome, Folders: r.folders})
	if len(r.unsafe) > 0 { // post-download payload verdict: the queue's safety action removes it
		s.publish(domain.EvPayloadRejected, r.items, domain.PayloadEventData{DownloadID: r.outcome.DownloadID,
			Stage: domain.StagePostDownload, Verdict: string(decide.PayloadDangerous), Files: r.unsafe})
	}
}

func (s *Service) publish(t domain.EventType, items []domain.EventItem, data any) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: "automation", Kind: domain.KindOf(items, domain.KindMovie), Items: items, Data: data})
}

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

func blocked(id int64, code string, items []domain.EventItem, reason string) result {
	o := domain.ImportOutcome{DownloadID: id, Status: domain.ImportBlocked, ReasonCode: code}
	for _, it := range items {
		o.Items = append(o.Items, domain.ImportItem{Item: it.Ref, Outcome: "blocked", Reason: reason})
	}
	return result{outcome: o, items: items}
}

func failed(id int64, code string, items []domain.EventItem, reason string) result {
	r := blocked(id, code, items, reason)
	r.outcome.Status = domain.ImportFailed
	return r
}

// run does the work of one import and always returns an outcome.
func (s *Service) run(ctx context.Context, d store.StoredDownload) result {
	switch d.Kind {
	case domain.KindTV:
		return s.runTV(ctx, d)
	case domain.KindMusic:
		return s.runMusic(ctx, d)
	}
	m, err := s.linkedMovie(ctx, d)
	items := []domain.EventItem{m.EventItem()}
	if err != nil {
		return blocked(d.ID, "no-match", itemsOf(d), err.Error())
	}
	files, r, ok := s.payload(ctx, d, items)
	if !ok {
		return r
	}
	video, ok := MainVideo(files)
	if !ok {
		return blocked(d.ID, "no-match", items, "no video file in the payload")
	}
	return s.place(ctx, d, m, video)
}

// errNoFileList: a payload sharing its folder cannot be told apart without
// the client's file list.
var errNoFileList = errors.New("the download has no folder of its own and the download client's file list is not available")

// payload scans and checks the download's files; when it cannot, the result
// is the outcome to report.
func (s *Service) payload(ctx context.Context, d store.StoredDownload, items []domain.EventItem) ([]File, result, bool) {
	files, err := s.scanPayload(ctx, d)
	switch {
	case errors.Is(err, errNoFileList):
		return nil, blocked(d.ID, "no-root-folder", items, err.Error()), false
	case err != nil || len(files) == 0:
		// a missing payload is a local problem (mounts, mapping), not a bad
		// release: block it for a retry instead of failing it
		return nil, blocked(d.ID, "path-not-found", items, "payload not found at "+source(d)+
			"; if the download client sees its folders under other paths, add a pathMap entry to it"), false
	}
	if bad := Executables(files); len(bad) > 0 {
		r := failed(d.ID, "payload-unsafe", items, "dangerous files: "+strings.Join(bad, ", "))
		r.unsafe = bad
		return nil, r, false
	}
	return files, result{}, true
}

// scanPayload reads the payload. A torrent without a root folder reports its
// save folder as content path; that folder holds other downloads too, so only
// the files the client lists for this download are read then.
func (s *Service) scanPayload(ctx context.Context, d store.StoredDownload) ([]File, error) {
	if !sharesFolder(d) {
		return Scan(source(d), s.d.Payload)
	}
	if s.d.Files == nil {
		return nil, errNoFileList
	}
	list, err := s.d.Files(ctx, d)
	if err != nil || len(list) == 0 {
		return nil, errNoFileList
	}
	return ScanList(d.SavePath, list, s.d.Payload)
}

// sharesFolder reports whether the content path is the save folder itself.
func sharesFolder(d store.StoredDownload) bool {
	return d.ContentPath != "" && d.SavePath != "" && filepath.Clean(d.ContentPath) == filepath.Clean(d.SavePath)
}

func source(d store.StoredDownload) string {
	if d.ContentPath != "" {
		return d.ContentPath
	}
	return filepath.Join(d.SavePath, d.Title)
}

func itemsOf(d store.StoredDownload) []domain.EventItem {
	out := make([]domain.EventItem, len(d.Links))
	for i, l := range d.Links {
		out[i] = domain.EventItem{Ref: l.Item}
	}
	return out
}

// linkedMovie is the grab's linked movie; it outweighs any parsing.
func (s *Service) linkedMovie(ctx context.Context, d store.StoredDownload) (domain.Movie, error) {
	for _, l := range d.Links {
		if l.Item.Kind == domain.KindMovie {
			return s.d.Movies.Get(ctx, l.Item.ID)
		}
	}
	return domain.Movie{}, errors.New("download has no linked movie")
}

// naming is the file naming in effect for a kind.
func (s *Service) naming(k domain.MediaKind) naming.Scheme {
	if s.d.Naming != nil {
		return s.d.Naming(k)
	}
	switch k {
	case domain.KindTV:
		return naming.DefaultTV()
	case domain.KindMusic:
		return naming.DefaultMusic()
	}
	return naming.DefaultMovie()
}
