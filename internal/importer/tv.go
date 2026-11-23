package importer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
)

// TV is the part of the tv module the importer needs.
type TV interface {
	Series(ctx context.Context, id int64) (domain.Series, error)
	Episode(ctx context.Context, id int64) (domain.Episode, error)
	Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error)
	SetEpisodeFile(ctx context.Context, ids []int64, fileID int64) error
}

// tvImport is one TV download being imported: the series, every episode
// of it (to see which file other episodes share) and the grab's links.
type tvImport struct {
	d       store.StoredDownload
	series  domain.Series
	all     []domain.Episode
	linked  []domain.Episode
	profile decide.QualityProfile
	items   []domain.EventItem
}

// runTV imports a TV download: each video file is mapped to the linked
// episodes it names (single, multi-episode or one file of a season pack;
// a lone file without a marker takes every linked episode, since the grab
// outweighs parsing), placed under the series' season folder and linked to
// those episodes. Episodes whose current file is as good are skipped with
// a reason, so a pack never downgrades.
func (s *Service) runTV(ctx context.Context, d store.StoredDownload) result {
	in, err := s.loadTV(ctx, d)
	if err != nil {
		return blocked(d.ID, "no-match", itemsOf(d), err.Error())
	}
	files, err := Scan(source(d), s.d.Payload)
	if err != nil || len(files) == 0 {
		return failed(d.ID, "path-not-found", in.items, "payload not found at "+source(d))
	}
	if bad := Executables(files); len(bad) > 0 {
		r := failed(d.ID, "payload-unsafe", in.items, "dangerous files: "+strings.Join(bad, ", "))
		r.unsafe = bad
		return r
	}
	return s.placeAll(ctx, in, assign(videos(files), in.linked))
}

func (s *Service) loadTV(ctx context.Context, d store.StoredDownload) (tvImport, error) {
	in := tvImport{d: d}
	if s.d.TV == nil {
		return in, errors.New("TV is not wired")
	}
	for _, l := range d.Links {
		if l.Item.Kind != domain.KindTV {
			continue
		}
		e, err := s.d.TV.Episode(ctx, l.Item.ID)
		if err != nil {
			continue // deleted since the grab
		}
		in.linked = append(in.linked, e)
	}
	if len(in.linked) == 0 {
		return in, errors.New("download has no linked episode")
	}
	var err error
	if in.series, err = s.d.TV.Series(ctx, in.linked[0].SeriesID); err != nil {
		return in, err
	}
	if in.all, err = s.d.TV.Episodes(ctx, in.series.ID); err != nil {
		return in, err
	}
	in.profile, _ = tv.ProfileByKey(in.series.ProfileKey)
	for _, e := range in.linked {
		in.items = append(in.items, e.EventItem(in.series.Title))
	}
	return in, nil
}

func videos(files []File) []File {
	var out []File
	for _, f := range files {
		if f.Class == ClassVideo {
			out = append(out, f)
		}
	}
	return out
}

// placement is one video file with the episodes it holds.
type placement struct {
	video    File
	episodes []domain.Episode
}

// assign maps video files onto linked episodes (pure).
func assign(vids []File, linked []domain.Episode) []placement {
	var out []placement
	for _, v := range vids {
		p := parse.ParseRelease(filepath.Base(v.Path))
		eps := tv.Covered(p.Episode, linked)
		if len(eps) == 0 && len(vids) == 1 && p.Episode.Kind == parse.EpisodeNone {
			eps = linked
		}
		if len(eps) > 0 {
			out = append(out, placement{video: v, episodes: eps})
		}
	}
	return out
}

// placeAll places each file and builds one outcome for the download.
func (s *Service) placeAll(ctx context.Context, in tvImport, ps []placement) result {
	byEp := map[int64]domain.ImportItem{}
	var folders []string
	var firstErr *result
	for _, p := range ps {
		items, folder, failure := s.placeEpisodes(ctx, in, p)
		if failure != nil && firstErr == nil {
			firstErr = failure
		}
		for _, it := range items {
			byEp[it.Item.ID] = it
		}
		folders = appendNew(folders, folder)
	}
	return tvOutcome(in, byEp, folders, firstErr)
}

func appendNew(xs []string, x string) []string {
	if x == "" {
		return xs
	}
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

// tvOutcome: every linked episode gets an item; status is imported when all
// imported, partial when some did, otherwise blocked (or the first failure).
func tvOutcome(in tvImport, byEp map[int64]domain.ImportItem, folders []string, failure *result) result {
	o := domain.ImportOutcome{DownloadID: in.d.ID}
	done := 0
	for _, e := range in.linked {
		it, ok := byEp[e.ID]
		if !ok {
			it = domain.ImportItem{Item: e.Ref(), Outcome: "skipped", Reason: "no file for this episode in the payload"}
		}
		if it.Outcome == "imported" || it.Outcome == "upgraded" {
			done++
		}
		o.Items = append(o.Items, it)
	}
	switch {
	case done == len(in.linked):
		o.Status = domain.ImportImported
	case done > 0:
		o.Status = domain.ImportPartial
	case failure != nil:
		return *failure
	default:
		o.Status, o.ReasonCode = domain.ImportBlocked, "no-match"
	}
	return result{outcome: o, folders: folders, items: in.items}
}

func episodeItems(eps []domain.Episode, outcome, reason, quality string, dest, replaced []string) []domain.ImportItem {
	out := make([]domain.ImportItem, len(eps))
	for i, e := range eps {
		out[i] = domain.ImportItem{Item: e.Ref(), Outcome: outcome, Reason: reason, Quality: quality, NewPaths: dest, ReplacedPaths: replaced}
	}
	return out
}

// placeEpisodes runs the upgrade check for one file's episodes, places the
// file and links it; it is idempotent per (download, file).
func (s *Service) placeEpisodes(ctx context.Context, in tvImport, p placement) ([]domain.ImportItem, string, *result) {
	if dest, _, ok, err := s.d.Library.ImportRecord(ctx, in.d.ID, p.video.Path); err == nil && ok {
		return episodeItems(p.episodes, "imported", "", "", []string{dest}, nil), filepath.Dir(dest), nil
	}
	cand := s.candidate(ctx, in.d, p.video, in.profile)
	curs := s.currents(ctx, p.episodes)
	if why, ok := s.upgradesAll(in.profile, curs, cand); !ok {
		return episodeItems(p.episodes, "skipped", why, "", nil, nil), "", nil
	}
	dest := tv.FilePath(in.series, p.episodes, cand.quality.Name, filepath.Ext(p.video.Path))
	replaced, err := s.putTV(ctx, in, p, dest, curs)
	if err != nil {
		r := placeFailure(in.d.ID, in.items, err)
		return nil, "", &r
	}
	if err := s.recordTV(ctx, in, p, dest, cand); err != nil {
		r := failed(in.d.ID, "path-not-found", in.items, err.Error())
		return nil, "", &r
	}
	outcome := "imported"
	if len(curs) > 0 {
		outcome = "upgraded"
	}
	return episodeItems(p.episodes, outcome, "", cand.quality.Name, []string{dest}, replaced), filepath.Dir(dest), nil
}

// currents are the distinct library files of the episodes.
func (s *Service) currents(ctx context.Context, eps []domain.Episode) []domain.LibraryFile {
	seen := map[int64]bool{}
	var out []domain.LibraryFile
	for _, e := range eps {
		if e.FileID == 0 || seen[e.FileID] {
			continue
		}
		seen[e.FileID] = true
		if f, ok, err := s.d.Library.File(ctx, e.FileID); err == nil && ok {
			out = append(out, f)
		}
	}
	return out
}

func (s *Service) upgradesAll(p decide.QualityProfile, curs []domain.LibraryFile, cand candidate) (string, bool) {
	for _, f := range curs {
		if ok, why := decide.IsUpgrade(p, currentFile(f), cand.upgrade()); !ok {
			return why, false
		}
	}
	return "", true
}

// putTV places the file; current files used only by these episodes are
// recycled, a file shared with other episodes stays for them.
func (s *Service) putTV(ctx context.Context, in tvImport, p placement, dest string, curs []domain.LibraryFile) ([]string, error) {
	var own []domain.LibraryFile
	for _, f := range curs {
		if onlyFor(f.ID, in.all, p.episodes) {
			own = append(own, f)
		}
	}
	var first domain.LibraryFile
	if len(own) > 0 {
		first = own[0]
	}
	replaced, err := s.put(ctx, p.video.Path, dest, first, len(own) > 0)
	if err != nil {
		return nil, err
	}
	for _, f := range own[min(1, len(own)):] {
		if err := s.replace(ctx, f); err != nil {
			return replaced, err
		}
		replaced = append(replaced, f.Path)
	}
	return replaced, nil
}

func onlyFor(fileID int64, all, eps []domain.Episode) bool {
	mine := map[int64]bool{}
	for _, e := range eps {
		mine[e.ID] = true
	}
	for _, e := range all {
		if e.FileID == fileID && !mine[e.ID] {
			return false
		}
	}
	return true
}

func (s *Service) recordTV(ctx context.Context, in tvImport, p placement, dest string, c candidate) error {
	f, err := s.d.Library.SaveFile(ctx, domain.LibraryFile{Item: p.episodes[0].Ref(), Path: dest, Size: p.video.Size,
		Quality: string(c.quality.ID), FormatScore: c.score, Group: c.parsed.Group.Value, Revision: c.parsed.Revision(),
		AddedAt: s.d.Now().UTC()})
	if err != nil {
		return err
	}
	ids := make([]int64, len(p.episodes))
	for i, e := range p.episodes {
		ids[i] = e.ID
	}
	if err := s.d.TV.SetEpisodeFile(ctx, ids, f.ID); err != nil {
		return err
	}
	return s.d.Library.RecordImport(ctx, in.d.ID, p.video.Path, dest, f.ID)
}
