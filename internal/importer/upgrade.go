package importer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/naming"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/rootfolders"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/store"
)

// candidate is what the new file is, read from the grabbed release title.
type candidate struct {
	quality decide.Quality
	parsed  parse.ParsedRelease
	score   int
}

func (c candidate) upgrade() decide.Candidate {
	return decide.Candidate{Quality: c.quality.ID, FormatScore: c.score, Group: c.parsed.Group.Value,
		Revision: c.parsed.Revision(), Proper: c.parsed.Proper}
}

func (s *Service) candidate(ctx context.Context, d store.StoredDownload, video File, p decide.QualityProfile) candidate {
	title := filepath.Base(video.Path)
	if g, ok, err := s.d.Queue.GrabOf(ctx, d); err == nil && ok {
		title = g.Release.Title
	}
	parsed := parse.ParseRelease(title)
	q, _ := decide.QualityOf(parsed)
	score, _ := p.Score(title)
	return candidate{quality: q, parsed: parsed, score: score}
}

// FileName renders the movie file name from the default template.
func FileName(m domain.Movie, c candidate, ext string) string {
	name, _ := naming.Options{Extension: ext}.Preview(naming.DefaultMovieFile, naming.Values{MovieTitle: m.Title,
		ReleaseYear: m.Year, QualityFull: c.quality.Name, Edition: c.parsed.Edition.Value, ReleaseGroup: c.parsed.Group.Value,
		IMDbID: m.IMDBID})
	return name
}

func (s *Service) current(ctx context.Context, m domain.Movie) (domain.LibraryFile, bool) {
	if m.FileID == 0 {
		return domain.LibraryFile{}, false
	}
	f, ok, err := s.d.Library.File(ctx, m.FileID)
	return f, ok && err == nil
}

func currentFile(f domain.LibraryFile) decide.CurrentFile {
	return decide.CurrentFile{Quality: decide.QualityID(f.Quality), FormatScore: f.FormatScore, Group: f.Group, Revision: f.Revision}
}

// place runs the upgrade check, puts the file into the library and records it.
func (s *Service) place(ctx context.Context, d store.StoredDownload, m domain.Movie, video File) result {
	items := []domain.EventItem{m.EventItem()}
	if dest, _, ok, err := s.d.Library.ImportRecord(ctx, d.ID, video.Path); err == nil && ok {
		return imported(d.ID, m, items, dest, nil, "imported", "") // already imported: same end state
	}
	profile, _ := movies.ProfileByKey(m.ProfileKey)
	cand := s.candidate(ctx, d, video, profile)
	cur, hasCur := s.current(ctx, m)
	if hasCur {
		if ok, why := decide.IsUpgrade(profile, currentFile(cur), cand.upgrade()); !ok {
			return blocked(d.ID, "not-upgrade", items, why)
		}
	}
	dest := filepath.Join(m.Path, FileName(m, cand, filepath.Ext(video.Path)))
	err := s.room(rootOf(m))
	if err == nil {
		var replaced []string
		if replaced, err = s.put(ctx, video.Path, dest, cur, hasCur); err == nil {
			return s.finish(ctx, d, m, video, dest, cand, replaced, hasCur)
		}
	}
	if errors.Is(err, ErrNoSpace) && s.d.NoSpace != nil {
		s.d.NoSpace(rootOf(m))
	}
	return placeFailure(d.ID, items, err)
}

// rootOf is the movie's root folder (its parent folder for old records).
func rootOf(m domain.Movie) string {
	if m.RootFolder != "" {
		return m.RootFolder
	}
	return filepath.Dir(m.Path)
}

// room checks the root folder keeps its reserve before any placement; a
// hardlink writes nothing but still respects the reserve, a copy checks its
// own size on top.
func (s *Service) room(root string) error {
	return EnoughSpace(root, s.d.FreeSpace(existing(root)), 0)
}

// existing is path or its nearest existing parent: the folder about to be
// created lives on that file system.
func existing(path string) string {
	for !exists(path) && filepath.Dir(path) != path {
		path = filepath.Dir(path)
	}
	return path
}

// EnoughSpace is the free-space rule: free must cover need plus the reserve.
func EnoughSpace(path string, free, need int64) error {
	if free < need+rootfolders.Reserve {
		return fmt.Errorf("%w in %s: %s free, %s needed including the %s reserve", ErrNoSpace, path,
			mb(free), mb(need+rootfolders.Reserve), mb(rootfolders.Reserve))
	}
	return nil
}

func mb(n int64) string { return strconv.FormatInt(n/1e6, 10) + " MB" }

func (s *Service) finish(ctx context.Context, d store.StoredDownload, m domain.Movie, video File, dest string, cand candidate,
	replaced []string, hasCur bool) result {
	items := []domain.EventItem{m.EventItem()}
	if err := s.record(ctx, d, m, video, dest, cand); err != nil {
		return failed(d.ID, "path-not-found", items, err.Error())
	}
	outcome := "imported"
	if hasCur {
		outcome = "upgraded"
	}
	return imported(d.ID, m, items, dest, replaced, outcome, cand.quality.Name)
}

var errConflict = errors.New("a file corsarr does not own is in the way")

// put places the new file first and then recycles the replaced one; when
// both share a path the old one is recycled first.
func (s *Service) put(ctx context.Context, src, dest string, cur domain.LibraryFile, hasCur bool) ([]string, error) {
	samePath := hasCur && cur.Path == dest
	if err := s.clear(ctx, src, dest, cur, samePath); err != nil {
		return nil, err
	}
	if _, err := Place(src, dest, s.d.FreeSpace); err != nil {
		return nil, err
	}
	if !hasCur {
		return nil, nil
	}
	if !samePath {
		if err := s.replace(ctx, cur); err != nil {
			return nil, err
		}
	}
	return []string{cur.Path}, nil
}

// clear makes room at dest: nothing to do when it is free or already the
// source; the current file of the item is recycled; anything else is a conflict.
func (s *Service) clear(ctx context.Context, src, dest string, cur domain.LibraryFile, samePath bool) error {
	if !exists(dest) || same(src, dest) {
		return nil
	}
	if !samePath {
		return errConflict
	}
	return s.replace(ctx, cur)
}

func placeFailure(id int64, items []domain.EventItem, err error) result {
	switch {
	case errors.Is(err, ErrNoSpace):
		return blocked(id, "no-space", items, err.Error())
	case errors.Is(err, errConflict):
		return blocked(id, "conflict", items, err.Error())
	}
	return failed(id, "path-not-found", items, err.Error())
}

func (s *Service) record(ctx context.Context, d store.StoredDownload, m domain.Movie, video File, dest string, c candidate) error {
	f, err := s.d.Library.SaveFile(ctx, domain.LibraryFile{Item: m.Ref(), Path: dest, Size: video.Size, Quality: string(c.quality.ID),
		FormatScore: c.score, Group: c.parsed.Group.Value, Revision: c.parsed.Revision(), Edition: c.parsed.Edition.Value,
		AddedAt: s.d.Now().UTC()})
	if err != nil {
		return err
	}
	if err := s.d.Movies.SetFile(ctx, m.ID, f.ID); err != nil {
		return err
	}
	return s.d.Library.RecordImport(ctx, d.ID, video.Path, dest, f.ID)
}

// replace recycles the file an upgrade replaced.
func (s *Service) replace(ctx context.Context, f domain.LibraryFile) error {
	return s.recycleFile(ctx, RuleUpgradeReplace, f)
}

// recycleFile recycles a library file through a recycling rule and drops its record.
func (s *Service) recycleFile(ctx context.Context, rule string, f domain.LibraryFile) error {
	a, err := s.d.Rules.Apply(ctx, rule, rules.Candidate{Key: f.Path,
		Evidence: map[string]any{"item": f.Item.String(), "quality": f.Quality}, Ops: []string{"move " + f.Path + " to the recycle area"}})
	if err != nil {
		return err
	}
	if a.Status != "acted" {
		return errors.New("recycling " + f.Path + " failed: " + a.Error)
	}
	if err := s.d.Library.DeleteFile(ctx, f.ID); err != nil {
		return err
	}
	s.publish(domain.EvFileDeleted, []domain.EventItem{{Ref: f.Item}}, domain.FileEventData{Path: f.Path, Reason: "recycled"})
	return nil
}

func imported(id int64, m domain.Movie, items []domain.EventItem, dest string, replaced []string, outcome, quality string) result {
	o := domain.ImportOutcome{DownloadID: id, Status: domain.ImportImported, Items: []domain.ImportItem{{Item: m.Ref(),
		Outcome: outcome, NewPaths: []string{dest}, ReplacedPaths: replaced, Quality: quality}}}
	return result{outcome: o, folders: []string{m.Path}, items: items}
}

// OnItemDeleted recycles the files of a deleted item when the user asked for it.
func (s *Service) OnItemDeleted(ctx context.Context, ev domain.Event) {
	data, ok := ev.Data.(domain.ItemEventData)
	if !ok || !data.FilesDeleted {
		return
	}
	for _, it := range ev.Items {
		if !s.d.Modes.Active(it.Ref.Kind) {
			continue // no file changes outside active kinds
		}
		files, err := s.d.Library.FilesOf(ctx, it.Ref)
		s.warn("list item files", err, "item", it.Ref.String())
		for _, f := range files {
			s.warn("recycle file", s.recycleFile(ctx, RuleItemDelete, f), "path", f.Path)
		}
	}
}
