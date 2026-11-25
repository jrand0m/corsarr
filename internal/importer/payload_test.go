package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// noRootFolder puts the download's own file next to an unrelated, bigger
// video directly in the save folder, as a torrent without a root folder does.
func noRootFolder(t *testing.T, e *env) {
	write(t, filepath.Join(e.dl, "mine.mkv"), strings.Repeat("m", 100))
	write(t, filepath.Join(e.dl, "Someone.Elses.Movie.2019.mkv"), strings.Repeat("o", 1000))
	e.q.d.ContentPath = e.dl + string(filepath.Separator)
}

func TestTorrentWithoutRootFolderImportsOnlyItsOwnFiles(t *testing.T) {
	e := newEnv(t)
	noRootFolder(t, e)
	var asked store.StoredDownload
	e.svc.d.Files = func(_ context.Context, d store.StoredDownload) ([]string, error) {
		asked = d
		return []string{"mine.mkv"}, nil
	}
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportImported || asked.ID != 7 {
		t.Fatalf("%+v", o)
	}
	src, _ := os.Stat(filepath.Join(e.dl, "mine.mkv"))
	dst, err := os.Stat(o.Items[0].NewPaths[0])
	if err != nil || !os.SameFile(src, dst) {
		t.Fatal("the torrent's own file must be imported", err)
	}
}

func TestTorrentWithoutRootFolderNeedsItsFileList(t *testing.T) {
	for name, files := range map[string]func(context.Context, store.StoredDownload) ([]string, error){
		"no lister": nil,
		"lister fails": func(context.Context, store.StoredDownload) ([]string, error) {
			return nil, errors.New("client offline")
		},
		"empty list": func(context.Context, store.StoredDownload) ([]string, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			noRootFolder(t, e)
			e.svc.d.Files = files
			e.completed(t, 7)
			o := e.q.outcomes[0]
			if o.Status != domain.ImportBlocked || o.ReasonCode != "no-root-folder" || !strings.Contains(o.Items[0].Reason, "file list") {
				t.Fatalf("%+v", o)
			}
		})
	}
}

func TestMissingPayloadBlocksWithAMappingHint(t *testing.T) {
	e := newEnv(t)
	e.q.d.ContentPath = filepath.Join(e.dl, "missing")
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportBlocked || o.ReasonCode != "path-not-found" || !strings.Contains(o.Items[0].Reason, "pathMap") {
		t.Fatalf("a missing payload is a local problem, not a bad release: %+v", o)
	}
}
