package importer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/store"
)

// Music is the part of the music module the importer needs.
type Music interface {
	Artist(ctx context.Context, id int64) (domain.Artist, error)
	EnsureEdition(ctx context.Context, albumID int64) (domain.Album, []domain.Track, error)
	FitEdition(ctx context.Context, albumID int64, files []music.AudioFile) (music.EditionFit, error)
	SetTrackFile(ctx context.Context, trackID, fileID int64) error
}

// musicImport is one music download being imported.
type musicImport struct {
	d       store.StoredDownload
	artist  domain.Artist
	album   domain.Album
	tracks  []domain.Track
	profile decide.QualityProfile
	items   []domain.EventItem
	title   string // the grabbed release title
	edition string // says how the album changed edition to fit the payload
}

// runMusic imports an album download: the payload's audio files are matched
// to the album's tracks by their tags and names (titles, disc and track
// numbers, lengths, recording ids); when too few files match, or they match
// only loosely, the download waits for a person. Each matched file is
// hardlinked to Artist/Album (Year)/[Disc NN/]Artist - Album - NN - Title
// and linked to its track; a track whose current file is as good is skipped.
func (s *Service) runMusic(ctx context.Context, d store.StoredDownload) result {
	in, err := s.loadMusic(ctx, d)
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
	audio := audioFiles(files)
	tagged := readTags(audio)
	if err := in.fitEdition(ctx, s.d.Music, tagged); err != nil {
		return blocked(d.ID, "no-match", in.items, err.Error())
	}
	m := music.MatchFiles(tagged, in.tracks)
	if !m.Auto() {
		return blocked(d.ID, "no-match", in.items, matchProblem(m, len(audio), in.album.Title))
	}
	return s.placeTracks(ctx, in, m, byRel(audio))
}

// fitEdition moves the import, and the album, to the edition the payload
// fits best.
func (in *musicImport) fitEdition(ctx context.Context, m Music, files []music.AudioFile) error {
	fit, err := m.FitEdition(ctx, in.album.ID, files)
	if err != nil {
		return err
	}
	in.album, in.tracks, in.edition = fit.Album, fit.Tracks, fit.Switched
	return nil
}

// matchProblem says why a match needs a person: too few files placed, or
// placed with too little certainty.
func matchProblem(m music.FileMatch, files int, album string) string {
	msg := fmt.Sprintf("%d of %d audio files match a track of %s", len(m.Placements), files, album)
	if m.Score >= music.AutoThreshold {
		msg += ", too loosely to import unattended"
	}
	return msg
}

// readTags reads the tags of every audio file; a file whose tags cannot be
// read is matched by its name alone.
func readTags(files []File) []music.AudioFile {
	out := make([]music.AudioFile, len(files))
	for i, f := range files {
		out[i].Rel = f.Rel
		out[i].Tags, _ = audiotag.Read(f.Path)
	}
	return out
}

func (s *Service) loadMusic(ctx context.Context, d store.StoredDownload) (musicImport, error) {
	in := musicImport{d: d, title: d.Title}
	if s.d.Music == nil {
		return in, errors.New("music is not wired")
	}
	id, ok := linkedAlbum(d)
	if !ok {
		return in, errors.New("download has no linked album")
	}
	var err error
	if in.album, in.tracks, err = s.d.Music.EnsureEdition(ctx, id); err != nil {
		return in, err
	}
	if len(in.tracks) == 0 {
		return in, errors.New("the track list of " + in.album.Title + " is not known yet")
	}
	if in.artist, err = s.d.Music.Artist(ctx, in.album.ArtistID); err != nil {
		return in, err
	}
	in.profile, _ = music.ProfileByKey(in.artist.ProfileKey)
	in.items = []domain.EventItem{in.album.EventItem(in.artist.Name)}
	if g, ok, err := s.d.Queue.GrabOf(ctx, d); err == nil && ok {
		in.title = g.Release.Title
	}
	return in, nil
}

func linkedAlbum(d store.StoredDownload) (int64, bool) {
	for _, l := range d.Links {
		if l.Item.Kind == domain.KindMusic && l.Item.Variant == "" {
			return l.Item.ID, true
		}
	}
	return 0, false
}

func audioFiles(files []File) []File {
	var out []File
	for _, f := range files {
		if f.Class != ClassExecutable && music.IsAudio(f.Rel) {
			out = append(out, f)
		}
	}
	return out
}

func byRel(files []File) map[string]File {
	out := make(map[string]File, len(files))
	for _, f := range files {
		out[f.Rel] = f
	}
	return out
}

// audioCandidate is what a new track file is: the quality its stream
// headers show; when they cannot be read, the release title's audio
// quality, or the file's extension when the title states none.
func audioCandidate(title, file string, p decide.QualityProfile) candidate {
	m := parse.ParseMusic(title)
	if fm, ok := fileAudio(file, m); ok {
		m = fm
	} else if !m.Format.Known {
		m.Format = parse.Field[parse.AudioFormat]{Value: formatOfExt(file), Known: formatOfExt(file) != ""}
	}
	q, _ := decide.AudioQualityOf(m)
	score, _ := p.Score(title)
	return candidate{quality: q, parsed: parse.ParsedRelease{Group: m.Group}, score: score}
}

var extFormats = map[string]parse.AudioFormat{".flac": parse.FormatFLAC, ".mp3": parse.FormatMP3, ".m4a": parse.FormatAAC,
	".aac": parse.FormatAAC, ".ogg": parse.FormatVorbis, ".oga": parse.FormatVorbis, ".opus": parse.FormatOpus,
	".wv": parse.FormatWavPack, ".ape": parse.FormatAPE, ".wav": parse.FormatWAV, ".wma": parse.FormatWMA}

func formatOfExt(file string) parse.AudioFormat {
	return extFormats[strings.ToLower(filepath.Ext(file))]
}

// trackResult is what happened to one track.
type trackResult struct {
	outcome  string // imported | upgraded | skipped
	reason   string
	dest     string
	replaced []string
	quality  string
}

// placeTracks places every matched file and builds one outcome for the album.
func (s *Service) placeTracks(ctx context.Context, in musicImport, m music.FileMatch, files map[string]File) result {
	if err := s.room(in.artist.RootFolder); err != nil {
		if errors.Is(err, ErrNoSpace) && s.d.NoSpace != nil {
			s.d.NoSpace(in.artist.RootFolder)
		}
		return placeFailure(in.d.ID, in.items, err)
	}
	var done []trackResult
	for _, p := range m.Placements {
		r, err := s.placeTrack(ctx, in, p.Track, files[p.File])
		if err != nil {
			return placeFailure(in.d.ID, in.items, err)
		}
		done = append(done, r)
	}
	return musicOutcome(in, done, len(m.Missing))
}

// placeTrack runs the upgrade check for one track, places the file and links
// it; it is idempotent per (download, file).
func (s *Service) placeTrack(ctx context.Context, in musicImport, t domain.Track, f File) (trackResult, error) {
	if dest, _, ok, err := s.d.Library.ImportRecord(ctx, in.d.ID, f.Path); err == nil && ok {
		return trackResult{outcome: "imported", dest: dest}, nil
	}
	cand := audioCandidate(in.title, f.Path, in.profile)
	cur, hasCur := s.trackFile(ctx, t)
	if hasCur {
		if ok, why := decide.IsUpgrade(in.profile, currentFile(cur), cand.upgrade()); !ok {
			return trackResult{outcome: "skipped", reason: why}, nil
		}
	}
	dest := music.TrackPath(in.artist, in.album, t, filepath.Ext(f.Path))
	replaced, err := s.put(ctx, f.Path, dest, cur, hasCur)
	if err != nil {
		return trackResult{}, err
	}
	lf, err := s.d.Library.SaveFile(ctx, domain.LibraryFile{Item: t.Ref(), Path: dest, Size: f.Size, Quality: string(cand.quality.ID),
		FormatScore: cand.score, Group: cand.parsed.Group.Value, Revision: 1, AddedAt: s.d.Now().UTC()})
	if err == nil {
		err = s.d.Music.SetTrackFile(ctx, t.ID, lf.ID)
	}
	if err == nil {
		err = s.d.Library.RecordImport(ctx, in.d.ID, f.Path, dest, lf.ID)
	}
	outcome := "imported"
	if hasCur {
		outcome = "upgraded"
	}
	return trackResult{outcome: outcome, dest: dest, replaced: replaced, quality: cand.quality.Name}, err
}

func (s *Service) trackFile(ctx context.Context, t domain.Track) (domain.LibraryFile, bool) {
	if t.FileID == 0 {
		return domain.LibraryFile{}, false
	}
	f, ok, err := s.d.Library.File(ctx, t.FileID)
	return f, ok && err == nil
}

// musicOutcome folds the track results into the album's import item:
// imported (or upgraded) when any track was placed, with missing tracks
// named in the reason; blocked as not an upgrade when every track was skipped.
func musicOutcome(in musicImport, done []trackResult, missing int) result {
	it := domain.ImportItem{Item: in.album.Ref()}
	placed, upgraded := 0, false
	var skipped string
	for _, r := range done {
		switch r.outcome {
		case "skipped":
			skipped = r.reason
			continue
		case "upgraded":
			upgraded = true
		}
		placed++
		it.NewPaths, it.ReplacedPaths = append(it.NewPaths, r.dest), append(it.ReplacedPaths, r.replaced...)
		it.Quality = first(it.Quality, r.quality)
	}
	if placed == 0 {
		return blocked(in.d.ID, "not-upgrade", in.items, skipped)
	}
	it.Outcome = map[bool]string{true: "upgraded", false: "imported"}[upgraded]
	it.Reason = importNotes(in.edition, missing)
	o := domain.ImportOutcome{DownloadID: in.d.ID, Status: domain.ImportImported, Items: []domain.ImportItem{it}}
	return result{outcome: o, folders: []string{music.AlbumPath(in.artist, in.album)}, items: in.items}
}

// importNotes joins what a person should know about an album import.
func importNotes(edition string, missing int) string {
	var notes []string
	if edition != "" {
		notes = append(notes, edition)
	}
	if missing > 0 {
		notes = append(notes, fmt.Sprintf("%d tracks missing from the payload", missing))
	}
	return strings.Join(notes, "; ")
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
