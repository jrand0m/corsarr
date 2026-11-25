package declarative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// guardPage is a block page written for these tests.
const guardPage = `<html><head><title>DDoS-Guard</title></head><body>checking</body></html>`

func challengeServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "ddos-guard")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(guardPage))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChallengeOnLoginIsNotBadCredentials(t *testing.T) {
	srv := challengeServer(t)
	ix := instance(t, exampleDef(t), srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	err := ix.Test(context.Background())
	if plugin.ClassOf(err) != plugin.ClassChallenge || !strings.Contains(err.Error(), "DDoS-Guard") {
		t.Errorf("%v", err)
	}
}

func TestChallengeOnSearchAndDownload(t *testing.T) {
	srv := challengeServer(t)
	ix := instance(t, bundled(t, "thepiratebay"), srv.URL, nil)
	_, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "x"})
	if plugin.ClassOf(err) != plugin.ClassChallenge {
		t.Errorf("search: %v", err)
	}
	_, err = ix.Resolve(context.Background(), domain.Release{ID: "y", DownloadURL: srv.URL + "/file"})
	if plugin.ClassOf(err) != plugin.ClassChallenge {
		t.Errorf("download: %v", err)
	}
}
