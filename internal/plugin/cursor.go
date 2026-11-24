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

// feedCursor is a parsed "<publish time RFC3339>|<release id>" cursor.
type feedCursor struct {
	at  time.Time
	id  string
	set bool
}

func parseCursor(c string) feedCursor {
	ts, id, _ := strings.Cut(c, "|")
	at, _ := time.Parse(time.RFC3339Nano, ts)
	return feedCursor{at: at, id: id, set: c != ""}
}

func (c feedCursor) before(r domain.Release) bool {
	return !c.set || r.PublishedAt.After(c.at) || (r.PublishedAt.Equal(c.at) && r.ID != c.id)
}

// NewerThan keeps the releases newer than a feed cursor and returns the
// advanced cursor. An empty cursor keeps everything. The cursor never moves
// backwards.
func NewerThan(rels []domain.Release, cursor string) ([]domain.Release, string) {
	cur := parseCursor(cursor)
	var fresh []domain.Release
	var newest *domain.Release
	for i, r := range rels {
		if cur.before(r) {
			fresh = append(fresh, r)
		}
		if newest == nil || r.PublishedAt.After(newest.PublishedAt) {
			newest = &rels[i]
		}
	}
	if newest == nil || (cur.set && !newest.PublishedAt.After(cur.at)) {
		return fresh, cursor
	}
	return fresh, newest.PublishedAt.Format(time.RFC3339Nano) + "|" + newest.ID
}
