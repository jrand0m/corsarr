package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// memory fakes implementing each extension point in the simplest correct way.

type okTester struct{}

func (okTester) Test(context.Context) error { return nil }

type memIndexer struct{ okTester }

func (memIndexer) Caps(context.Context) (plugin.Caps, error) {
	return plugin.Caps{Modes: []domain.SearchMode{domain.SearchText}}, nil
}

func (memIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	return []domain.Release{{ID: "a", Title: "A", Protocol: domain.ProtoTorrent}}, nil
}

func (memIndexer) Feed(_ context.Context, _ domain.MediaKind, cursor string) ([]domain.Release, string, error) {
	if cursor != "" {
		return nil, cursor, nil
	}
	return []domain.Release{{ID: "a"}}, "c1", nil
}

func (memIndexer) Resolve(context.Context, domain.Release) (plugin.Payload, error) {
	return plugin.Payload{Kind: "magnet", URL: "magnet:?xt=urn:btih:x"}, nil
}

func TestIndexerCheck(t *testing.T) {
	Indexer(t, memIndexer{}, domain.SearchRequest{Kind: domain.KindMovie})
}

type memDownloader struct {
	okTester
	held map[string]bool
}

func (memDownloader) Protocols() []domain.Protocol { return []domain.Protocol{domain.ProtoTorrent} }

func (d memDownloader) Add(_ context.Context, r plugin.AddRequest) (string, error) {
	d.held[r.Payload.Name] = true
	return r.Payload.Name, nil
}

func (d memDownloader) List(context.Context, []string) ([]plugin.Snapshot, error) {
	var out []plugin.Snapshot
	for id := range d.held {
		out = append(out, plugin.Snapshot{DownloadID: id})
	}
	return out, nil
}

func (d memDownloader) Remove(_ context.Context, id string, _ bool) error {
	delete(d.held, id)
	return nil
}
func (memDownloader) Pause(context.Context, string) error                        { return nil }
func (memDownloader) Resume(context.Context, string) error                       { return nil }
func (memDownloader) SetPriority(context.Context, string, plugin.Priority) error { return nil }
func (memDownloader) EnsureCategory(context.Context, string, string) error       { return nil }

func TestDownloaderCheck(t *testing.T) {
	Downloader(t, memDownloader{held: map[string]bool{}}, plugin.AddRequest{Payload: plugin.Payload{Name: "h1"}, Category: "c"})
}

type memMetadata struct{ okTester }

func (memMetadata) SearchMovies(context.Context, string, int) ([]plugin.MovieRecord, error) {
	return []plugin.MovieRecord{{IDs: map[string]string{"tmdb": "1"}, Title: "M", Year: 2000}}, nil
}

func (memMetadata) Movie(context.Context, map[string]string) (plugin.MovieRecord, error) {
	return plugin.MovieRecord{Title: "M", Year: 2000}, nil
}

func TestMetadataCheck(t *testing.T) { Metadata(t, memMetadata{}, "m", 2000) }

type memMediaServer struct{ okTester }

func (memMediaServer) RefreshPaths(context.Context, []string) error { return nil }

func TestMediaServerCheck(t *testing.T) { MediaServer(t, memMediaServer{}, []string{"/x"}) }

type memNotifier struct{ okTester }

func (memNotifier) Schemes() []string { return []string{"ntfy"} }
func (memNotifier) Send(context.Context, domain.Event, plugin.Message) error {
	return nil
}

func TestNotifierCheck(t *testing.T) { Notifier(t, memNotifier{}, domain.Event{}, plugin.Message{}) }

func TestProblemListers(t *testing.T) {
	if p := releaseProblems([]domain.Release{{ID: "a", Title: "t", Protocol: "p"}}); len(p) != 0 {
		t.Errorf("clean releases flagged: %v", p)
	}
	p := strings.Join(releaseProblems([]domain.Release{{ID: "a"}, {ID: "a", Title: "t", Protocol: "p"}, {ID: "a", Title: "t", Protocol: "p"}}), "|")
	if !strings.Contains(p, "incomplete release a") || !strings.Contains(p, "duplicate release id a") {
		t.Errorf("%s", p)
	}
	if got := schemeProblems(nil); len(got) != 1 {
		t.Errorf("no schemes: %v", got)
	}
	if got := schemeProblems([]string{"ntfy", "Pushover", ""}); len(got) != 2 {
		t.Errorf("bad schemes: %v", got)
	}
	if got := schemeProblems([]string{"ntfy"}); len(got) != 0 {
		t.Errorf("good scheme flagged: %v", got)
	}
}
