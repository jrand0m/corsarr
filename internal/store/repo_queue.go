package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Queue stores grabs, downloads and their item links. The queue
// module is the only writer.
type Queue struct{ s *Store }

// Queue returns the queue repository.
func (s *Store) Queue() Queue { return Queue{s} }

const downloadCols = `id, grab_id, downloader, download_id, protocol, kind, title, state, progress, size, downloaded,
	speed, eta, seeders, peers, category, save_path, content_path, held, status_reason, added_at, completed_at, last_activity_at`

// ContentPath is kept beside the domain shape: the payload's file or folder.
func scanDownload(sc scanner) (domain.Download, string, error) {
	var d domain.Download
	var content, downloader, proto, kind, state string
	var added int64
	var completed, activity sql.NullInt64
	err := sc.Scan(&d.ID, &d.GrabID, &downloader, &d.DownloadID, &proto, &kind, &d.Title, &state, &d.Progress, &d.Size,
		&d.Downloaded, &d.Speed, &d.ETASeconds, &d.Seeders, &d.Peers, &d.Category, &d.SavePath, &content, &d.Held,
		&d.StatusReason, &added, &completed, &activity)
	d.Downloader, d.Protocol, d.Kind, d.State = domain.ObjectKey(downloader), domain.Protocol(proto), domain.MediaKind(kind), domain.DownloadState(state)
	d.AddedAt, d.CompletedAt, d.LastActivityAt = fromMS(added), ptrMS(completed), ptrMS(activity)
	return d, content, err
}

// StoredDownload is a download plus its persisted content path.
type StoredDownload struct {
	domain.Download
	ContentPath string `json:"contentPath,omitempty"`
}

// CreateGrab persists a grab with its download and links in one transaction.
// When a live download with the same (downloader, download id) exists, that
// one is returned and nothing is written (idempotent retry).
func (r Queue) CreateGrab(ctx context.Context, g domain.Grab, d domain.Download) (domain.Grab, StoredDownload, error) {
	if existing, ok, err := r.liveByClientID(ctx, d.Downloader, d.DownloadID); err != nil || ok {
		if err != nil {
			return domain.Grab{}, StoredDownload{}, err
		}
		grab, _, err := r.Grab(ctx, existing.GrabID)
		return grab, existing, err
	}
	sealed, err := r.s.Encrypt(g.Release.DownloadURL)
	if err != nil {
		return domain.Grab{}, StoredDownload{}, err
	}
	err = r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO grabs (kind, item_refs, release, download_url_sealed, decision, origin,
			downloader, download_id, guid, info_hash, title, quality, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(g.Kind), jsonText(g.ItemRefs), jsonText(g.Release), sealed, jsonText(g.Decision), string(g.Origin),
			string(g.Downloader), g.DownloadID, g.Release.GUID, g.Release.InfoHash, g.Release.Title, g.Decision.Quality, ms(g.SentAt))
		if err != nil {
			return err
		}
		g.ID, _ = res.LastInsertId()
		d.GrabID = g.ID
		return insertDownload(ctx, tx, &d)
	})
	return g, StoredDownload{Download: d}, err
}

func insertDownload(ctx context.Context, tx *sql.Tx, d *domain.Download) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO downloads (grab_id, downloader, download_id, protocol, kind, title, state,
		size, category, added_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.GrabID, string(d.Downloader), d.DownloadID,
		string(d.Protocol), string(d.Kind), d.Title, string(d.State), d.Size, d.Category, ms(d.AddedAt))
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	return writeLinks(ctx, tx, d.ID, d.Links)
}

func writeLinks(ctx context.Context, tx *sql.Tx, id int64, links []domain.DownloadLink) error {
	for _, l := range links {
		if _, err := tx.ExecContext(ctx, `INSERT INTO download_links (download_id, item_kind, item_id, status) VALUES (?, ?, ?, ?)
			ON CONFLICT (download_id, item_kind, item_id) DO UPDATE SET status = excluded.status`,
			id, string(l.Item.Kind), l.Item.ID, string(l.Status)); err != nil {
			return err
		}
	}
	return nil
}

func (r Queue) liveByClientID(ctx context.Context, downloader domain.ObjectKey, clientID string) (StoredDownload, bool, error) {
	return r.one(ctx, `SELECT `+downloadCols+` FROM downloads WHERE downloader = ? AND download_id = ?
		AND state NOT IN ('imported', 'removed', 'failed')`, string(downloader), clientID)
}

// Download returns one download with its links.
func (r Queue) Download(ctx context.Context, id int64) (StoredDownload, bool, error) {
	return r.one(ctx, "SELECT "+downloadCols+" FROM downloads WHERE id = ?", id)
}

func (r Queue) one(ctx context.Context, q string, args ...any) (StoredDownload, bool, error) {
	d, content, err := scanDownload(r.s.R().QueryRowContext(ctx, q, args...))
	if ok, err := found(err); !ok {
		return StoredDownload{}, false, err
	}
	sd := StoredDownload{Download: d, ContentPath: content}
	sd.Links, err = r.links(ctx, d.ID)
	return sd, err == nil, err
}

func (r Queue) links(ctx context.Context, id int64) ([]domain.DownloadLink, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT item_kind, item_id, status FROM download_links WHERE download_id = ? ORDER BY item_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.DownloadLink{}
	for rows.Next() {
		var l domain.DownloadLink
		var kind, status string
		if err := rows.Scan(&kind, &l.Item.ID, &status); err != nil {
			return nil, err
		}
		l.Item.Kind, l.Status = domain.MediaKind(kind), domain.LinkStatus(status)
		out = append(out, l)
	}
	return out, rows.Err()
}

// Downloads returns every download in one of the states, oldest first.
func (r Queue) Downloads(ctx context.Context, states ...domain.DownloadState) ([]StoredDownload, error) {
	args := make([]any, len(states))
	for i, s := range states {
		args[i] = string(s)
	}
	q := "SELECT " + downloadCols + " FROM downloads WHERE state IN (" + placeholders(len(states)) + ") ORDER BY id"
	return r.many(ctx, q, args...)
}

// List pages the queue newest first (downloads not yet imported or removed).
func (r Queue) List(ctx context.Context, cursor int64, limit int) ([]StoredDownload, int64, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	out, err := r.many(ctx, "SELECT "+downloadCols+` FROM downloads WHERE id < ? AND state NOT IN ('imported', 'removed')
		ORDER BY id DESC LIMIT ?`, cursor, limit)
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, err
}

func (r Queue) many(ctx context.Context, q string, args ...any) ([]StoredDownload, error) {
	rows, err := r.s.R().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []StoredDownload
	for rows.Next() {
		d, content, err := scanDownload(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, StoredDownload{Download: d, ContentPath: content})
	}
	rows.Close()
	for i := range out {
		if out[i].Links, err = r.links(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// Save writes a download's mutable fields and link statuses.
func (r Queue) Save(ctx context.Context, d StoredDownload) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE downloads SET title = ?, state = ?, progress = ?, size = ?, downloaded = ?,
			speed = ?, eta = ?, seeders = ?, peers = ?, category = ?, save_path = ?, content_path = ?, held = ?,
			status_reason = ?, completed_at = ?, last_activity_at = ? WHERE id = ?`, d.Title, string(d.State), d.Progress,
			d.Size, d.Downloaded, d.Speed, d.ETASeconds, d.Seeders, d.Peers, d.Category, d.SavePath, d.ContentPath, d.Held,
			d.StatusReason, nullMS(d.CompletedAt), nullMS(d.LastActivityAt), d.ID)
		if err != nil {
			return err
		}
		return writeLinks(ctx, tx, d.ID, d.Links)
	})
}

// PayloadChecked reports whether the pre-download payload verdict was taken.
func (r Queue) PayloadChecked(ctx context.Context, id int64) (bool, error) {
	var v bool
	err := r.s.R().QueryRowContext(ctx, "SELECT payload_checked FROM downloads WHERE id = ?", id).Scan(&v)
	return v, err
}

// SetPayloadChecked records that the payload verdict was taken.
func (r Queue) SetPayloadChecked(ctx context.Context, id int64) error {
	return r.s.exec(ctx, "UPDATE downloads SET payload_checked = 1 WHERE id = ?", id)
}

// Grab returns a grab with its release (including the unsealed download URL).
func (r Queue) Grab(ctx context.Context, id int64) (domain.Grab, bool, error) {
	var g domain.Grab
	var kind, items, rel, sealed, dec, origin, downloader string
	var sent int64
	err := r.s.R().QueryRowContext(ctx, `SELECT id, kind, item_refs, release, download_url_sealed, decision, origin,
		downloader, download_id, sent_at FROM grabs WHERE id = ?`, id).
		Scan(&g.ID, &kind, &items, &rel, &sealed, &dec, &origin, &downloader, &g.DownloadID, &sent)
	if ok, err := found(err); !ok {
		return g, false, err
	}
	g.Kind, g.Origin, g.Downloader, g.SentAt = domain.MediaKind(kind), domain.SearchOrigin(origin), domain.ObjectKey(downloader), fromMS(sent)
	err = joinErr(json.Unmarshal([]byte(items), &g.ItemRefs), json.Unmarshal([]byte(rel), &g.Release), json.Unmarshal([]byte(dec), &g.Decision))
	if err != nil {
		return g, false, err
	}
	g.Release.DownloadURL, err = r.s.Decrypt(sealed)
	return g, err == nil, err
}

func joinErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// QueuedQualities lists the decision qualities of live downloads for an item.
func (r Queue) QueuedQualities(ctx context.Context, item domain.ItemRef) ([]string, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT g.quality FROM downloads d JOIN grabs g ON g.id = d.grab_id
		JOIN download_links l ON l.download_id = d.id WHERE l.item_kind = ? AND l.item_id = ?
		AND d.state NOT IN ('imported', 'removed', 'failed')`, string(item.Kind), item.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GrabbedBefore reports whether this release was grabbed for the item before
// (matched by guid, info hash or exact title).
func (r Queue) GrabbedBefore(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error) {
	var one int
	return found(r.s.R().QueryRowContext(ctx, `SELECT 1 FROM grabs g JOIN downloads d ON d.grab_id = g.id
		JOIN download_links l ON l.download_id = d.id WHERE l.item_kind = ? AND l.item_id = ?
		AND ((g.guid <> '' AND g.guid = ?) OR (g.info_hash <> '' AND g.info_hash = ?) OR g.title = ?) LIMIT 1`,
		string(item.Kind), item.ID, id.GUID, id.InfoHash, id.Title).Scan(&one))
}
