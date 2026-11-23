package store

import (
	"context"
	"database/sql"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Blocklist stores blocklist entries (writer: automation).
type Blocklist struct{ s *Store }

// Blocklist returns the blocklist repository.
func (s *Store) Blocklist() Blocklist { return Blocklist{s} }

const blocklistCols = "id, item_kind, item_id, indexer, guid, info_hash, title, source, reason, added_at"

func scanBlock(sc scanner) (domain.BlocklistEntry, error) {
	var e domain.BlocklistEntry
	var kind, indexer string
	var added int64
	err := sc.Scan(&e.ID, &kind, &e.Item.ID, &indexer, &e.Release.GUID, &e.Release.InfoHash, &e.Release.Title, &e.Source, &e.Reason, &added)
	e.Item.Kind, e.Release.Indexer, e.AddedAt = domain.MediaKind(kind), domain.ObjectKey(indexer), fromMS(added)
	return e, err
}

// Add stores an entry; the same (item, release identity) twice keeps the first
// (created=false).
func (r Blocklist) Add(ctx context.Context, e domain.BlocklistEntry) (domain.BlocklistEntry, bool, error) {
	var created bool
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO blocklist (item_kind, item_id, indexer, guid, info_hash, title,
			source, reason, added_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, string(e.Item.Kind), e.Item.ID,
			string(e.Release.Indexer), e.Release.GUID, e.Release.InfoHash, e.Release.Title, e.Source, e.Reason, ms(e.AddedAt))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		created = n == 1
		return nil
	})
	if err != nil {
		return e, false, err
	}
	out, err := scanBlock(r.s.R().QueryRowContext(ctx, "SELECT "+blocklistCols+` FROM blocklist WHERE item_kind = ? AND item_id = ?
		AND indexer = ? AND guid = ? AND info_hash = ? AND title = ?`, string(e.Item.Kind), e.Item.ID, string(e.Release.Indexer),
		e.Release.GUID, e.Release.InfoHash, e.Release.Title))
	return out, created, err
}

// Has reports whether the release is blocklisted for the item (by guid, info
// hash or exact title); the item index keeps it one indexed lookup.
func (r Blocklist) Has(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error) {
	var one int
	return found(r.s.R().QueryRowContext(ctx, `SELECT 1 FROM blocklist WHERE item_kind = ? AND item_id = ?
		AND ((guid <> '' AND guid = ?) OR (info_hash <> '' AND info_hash = ?) OR title = ?) LIMIT 1`,
		string(item.Kind), item.ID, id.GUID, id.InfoHash, id.Title).Scan(&one))
}

// List pages entries newest first.
func (r Blocklist) List(ctx context.Context, cursor int64, limit int) ([]domain.BlocklistEntry, int64, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	rows, err := r.s.R().QueryContext(ctx, "SELECT "+blocklistCols+" FROM blocklist WHERE id < ? ORDER BY id DESC LIMIT ?", cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.BlocklistEntry{}
	for rows.Next() {
		e, err := scanBlock(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

// Remove deletes an entry and returns it (ok=false when it did not exist).
func (r Blocklist) Remove(ctx context.Context, id int64) (domain.BlocklistEntry, bool, error) {
	e, err := scanBlock(r.s.R().QueryRowContext(ctx, "SELECT "+blocklistCols+" FROM blocklist WHERE id = ?", id))
	if ok, err := found(err); !ok {
		return e, false, err
	}
	return e, true, r.s.exec(ctx, "DELETE FROM blocklist WHERE id = ?", id)
}

// FeedCursors stores the per-indexer RSS cursor (advanced by automation
// only after a batch is processed).
type FeedCursors struct{ s *Store }

// FeedCursors returns the cursor repository.
func (s *Store) FeedCursors() FeedCursors { return FeedCursors{s} }

// Get returns the cursor ("" when none).
func (r FeedCursors) Get(ctx context.Context, indexer domain.ObjectKey, kind domain.MediaKind) (string, error) {
	var c string
	_, err := found(r.s.R().QueryRowContext(ctx, "SELECT cursor FROM feed_cursors WHERE indexer = ? AND kind = ?",
		string(indexer), string(kind)).Scan(&c))
	return c, err
}

// Set stores the cursor.
func (r FeedCursors) Set(ctx context.Context, indexer domain.ObjectKey, kind domain.MediaKind, cursor string) error {
	return r.s.exec(ctx, `INSERT INTO feed_cursors (indexer, kind, cursor, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (indexer, kind) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at`,
		string(indexer), string(kind), cursor, ms(now()))
}
