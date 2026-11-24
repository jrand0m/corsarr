package plugin

import (
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Classes for sites that need the owner to act on a session.
const (
	ClassChallenge      FailureClass = "challenge"
	ClassSessionExpired FailureClass = "session_expired"
)

// NewerThan keeps the releases newer than a feed cursor
// ("<publish time RFC3339>|<release id>") and returns the advanced cursor.
// An empty cursor keeps everything. The cursor never moves backwards.
func NewerThan(rels []domain.Release, cursor string) ([]domain.Release, string) {
	ts, id, _ := strings.Cut(cursor, "|")
	at, _ := time.Parse(time.RFC3339Nano, ts)
	var fresh []domain.Release
	newest := domain.Release{}
	for _, r := range rels {
		if cursor == "" || r.PublishedAt.After(at) || (r.PublishedAt.Equal(at) && r.ID != id) {
			fresh = append(fresh, r)
		}
		if newest.ID == "" || r.PublishedAt.After(newest.PublishedAt) {
			newest = r
		}
	}
	if newest.ID == "" || (cursor != "" && !newest.PublishedAt.After(at)) {
		return fresh, cursor
	}
	return fresh, newest.PublishedAt.Format(time.RFC3339Nano) + "|" + newest.ID
}
