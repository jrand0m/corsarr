package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// RecentReleases is the bounded cache of releases shown by recent searches
// (writer: automation). Download links are stored sealed.
type RecentReleases struct{ s *Store }

// RecentReleases returns the recent-release repository.
func (s *Store) RecentReleases() RecentReleases { return RecentReleases{s} }

// PutMany stores releases seen at t in one transaction, then keeps only the
// newest max rows not older than expire.
func (r RecentReleases) PutMany(ctx context.Context, rels []domain.Release, t, expire time.Time, max int) error {
	sealed := make([]string, len(rels))
	for i, rel := range rels {
		var err error
		if sealed[i], err = r.s.Encrypt(rel.DownloadURL); err != nil {
			return err
		}
	}
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for i, rel := range rels {
			if _, err := tx.ExecContext(ctx, `INSERT INTO recent_releases (id, release, download_url_sealed, seen_at) VALUES (?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET release = excluded.release, download_url_sealed = excluded.download_url_sealed,
				seen_at = excluded.seen_at`, rel.ID, jsonText(rel), sealed[i], ms(t)); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM recent_releases WHERE seen_at < ? OR id NOT IN
			(SELECT id FROM recent_releases ORDER BY seen_at DESC, id LIMIT ?)`, ms(expire), max)
		return err
	})
}

// Get returns a release seen at or after since.
func (r RecentReleases) Get(ctx context.Context, id string, since time.Time) (domain.Release, bool, error) {
	var body, sealed string
	err := r.s.R().QueryRowContext(ctx, "SELECT release, download_url_sealed FROM recent_releases WHERE id = ? AND seen_at >= ?",
		id, ms(since)).Scan(&body, &sealed)
	if ok, err := found(err); !ok {
		return domain.Release{}, false, err
	}
	var rel domain.Release
	if err := json.Unmarshal([]byte(body), &rel); err != nil {
		return rel, false, err
	}
	url, err := r.s.Decrypt(sealed)
	rel.DownloadURL = url
	return rel, err == nil, err
}
