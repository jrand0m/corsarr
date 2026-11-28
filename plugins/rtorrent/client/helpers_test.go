package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newFileServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x.torrent" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := EncodeResponse(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// doerFunc answers every request with fn's status and body.
type doerFunc func(ctx context.Context) (int, []byte, error)

func (d doerFunc) Do(ctx context.Context, _, _ string, _ map[string]string, _ []byte) (int, map[string]string, []byte, error) {
	status, body, err := d(ctx)
	return status, nil, body, err
}

// recordingDoer keeps the last request header and answers a version.
type recordingDoer struct{ header *map[string]string }

func (d recordingDoer) Do(_ context.Context, _, _ string, header map[string]string, _ []byte) (int, map[string]string, []byte, error) {
	*d.header = header
	b, _ := EncodeResponse([]any{[]any{"0.9.8"}, []any{"0.13.8"}})
	return 200, nil, b, nil
}
