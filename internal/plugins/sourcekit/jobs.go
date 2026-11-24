package sourcekit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// job is one album being fetched.
type job struct {
	id       string
	name     string
	category string
	folder   string
	state    domain.DownloadState
	size     int64
	done     int64
	err      string
	at       time.Time
}

// maxFile bounds one fetched file (a hi-res track is well below it).
const maxFile = 2 << 30

// Add starts fetching an album's files into <downloadDir>/<release title>/;
// adding the same album again returns the fetch already under way.
func (s *Source) Add(_ context.Context, req plugin.AddRequest) (string, error) {
	if req.Payload.Kind != "source" || req.Payload.URL == "" {
		return "", plugin.Errorf(plugin.ClassConfig, "a source fetches only its own releases")
	}
	id := fmt.Sprintf("%s-%x", s.key, req.Payload.URL)
	s.mu.Lock()
	if _, ok := s.jobs[id]; ok {
		s.mu.Unlock()
		return id, nil
	}
	j := &job{id: id, name: req.Payload.Name, category: req.Category,
		folder: filepath.Join(s.dir, safeName(req.Payload.Name)), state: domain.StateDownloading, at: time.Now()}
	s.jobs[j.id] = j
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.finish(j, s.fetch(j, req.Payload.URL))
	}()
	return j.id, nil
}

// safeName keeps a folder name inside the download folder.
func safeName(s string) string {
	s = strings.NewReplacer("/", "-", `\`, "-", "..", ".", ":", "-").Replace(strings.TrimSpace(s))
	if s == "" || s == "." {
		return "album"
	}
	return s
}

func (s *Source) fetch(j *job, itemID string) error {
	files, err := s.site.Files(s.ctx, itemID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("the source lists no files for this album")
	}
	for _, f := range files {
		s.update(j, func(j *job) { j.size += f.Size })
	}
	for _, f := range files {
		if err := s.fetchOne(j, f); err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) fetchOne(j *job, f FileRef) error {
	dest := filepath.Join(j.folder, safeName(f.Name))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	body, err := s.site.Fetch(s.ctx, f)
	if err != nil {
		return err
	}
	defer body.Close()
	tmp := dest + ".partial"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(body, maxFile))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	s.update(j, func(j *job) { j.done += n })
	return os.Rename(tmp, dest)
}

func (s *Source) update(j *job, fn func(*job)) {
	s.mu.Lock()
	fn(j)
	s.mu.Unlock()
}

func (s *Source) finish(j *job, err error) {
	s.update(j, func(j *job) {
		j.state, j.at = domain.StateCompleted, time.Now()
		if err != nil {
			j.state, j.err = domain.StateFailed, err.Error()
		}
		j.size = max(j.size, j.done)
	})
}

// List reports every fetch in the categories as a snapshot.
func (s *Source) List(_ context.Context, categories []string) ([]plugin.Snapshot, error) {
	want := map[string]bool{}
	for _, c := range categories {
		want[c] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []plugin.Snapshot
	for _, j := range s.jobs {
		if len(want) > 0 && !want[j.category] {
			continue
		}
		at := j.at
		progress := 1.0
		if j.state == domain.StateDownloading && j.size > 0 {
			progress = float64(j.done) / float64(j.size)
		}
		out = append(out, plugin.Snapshot{DownloadID: j.id, Name: j.name, Category: j.category, State: j.state, Progress: progress,
			Size: j.size, Downloaded: j.done, SavePath: s.dir, ContentPath: j.folder, LastActivityAt: &at, Error: j.err})
	}
	return out, nil
}

// Remove forgets a fetch and, when asked, deletes its folder.
func (s *Source) Remove(_ context.Context, id string, deleteFiles bool) error {
	s.mu.Lock()
	j, ok := s.jobs[id]
	delete(s.jobs, id)
	s.mu.Unlock()
	if ok && deleteFiles {
		return os.RemoveAll(j.folder)
	}
	return nil
}
