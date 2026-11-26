package legacy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// queueRecord is one queue row: a download per item it fetches.
type queueRecord struct {
	ID                      int64           `json:"id"`
	SeriesID                int64           `json:"seriesId,omitempty"`
	EpisodeID               int64           `json:"episodeId,omitempty"`
	SeasonNumber            *int            `json:"seasonNumber,omitempty"`
	MovieID                 int64           `json:"movieId,omitempty"`
	Title                   string          `json:"title"`
	Size                    int64           `json:"size"`
	Sizeleft                int64           `json:"sizeleft"`
	Timeleft                string          `json:"timeleft,omitempty"`
	EstimatedCompletionTime string          `json:"estimatedCompletionTime,omitempty"`
	Added                   string          `json:"added"`
	Status                  string          `json:"status"`
	TrackedDownloadStatus   string          `json:"trackedDownloadStatus"`
	TrackedDownloadState    string          `json:"trackedDownloadState"`
	StatusMessages          []statusMessage `json:"statusMessages"`
	ErrorMessage            string          `json:"errorMessage,omitempty"`
	DownloadID              string          `json:"downloadId"`
	Protocol                string          `json:"protocol"`
	DownloadClient          string          `json:"downloadClient"`
	Episode                 *queueEpisode   `json:"episode,omitempty"`
	Series                  *titleRef       `json:"series,omitempty"`
	Movie                   *titleRef       `json:"movie,omitempty"`
}

type statusMessage struct {
	Title    string   `json:"title"`
	Messages []string `json:"messages"`
}

type queueEpisode struct {
	ID            int64  `json:"id"`
	SeriesID      int64  `json:"seriesId"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title"`
	AirDateUtc    string `json:"airDateUtc,omitempty"`
}

type titleRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// states maps a download state to the queue status and tracked state words.
var states = map[domain.DownloadState][2]string{
	domain.StateQueued:        {"queued", "downloading"},
	domain.StateDownloading:   {"downloading", "downloading"},
	domain.StateStalled:       {"warning", "downloading"},
	domain.StateCompleted:     {"completed", "importPending"},
	domain.StateImportPending: {"completed", "importPending"},
	domain.StateImporting:     {"completed", "importing"},
	domain.StateImportBlocked: {"completed", "importBlocked"},
	domain.StateFailed:        {"failed", "failed"},
}

// downloads lists every queued download of one kind.
func (s *Service) downloads(ctx context.Context, kind domain.MediaKind) ([]store.StoredDownload, error) {
	var out []store.StoredDownload
	var cursor int64
	for {
		list, next, err := s.d.Queue.List(ctx, cursor, 200)
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			if d.Kind == kind {
				out = append(out, d)
			}
		}
		if next == 0 {
			return out, nil
		}
		cursor = next
	}
}

// baseRecord is the download part of a queue row.
func (s *Service) baseRecord(d store.StoredDownload) queueRecord {
	st, known := states[d.State]
	if !known {
		st = [2]string{"unknown", "downloading"}
	}
	rec := queueRecord{ID: d.ID, Title: d.Title, Size: d.Size, Sizeleft: max(d.Size-d.Downloaded, 0), Status: st[0],
		TrackedDownloadStatus: "ok", TrackedDownloadState: st[1], StatusMessages: []statusMessage{}, DownloadID: d.DownloadID,
		Protocol: string(d.Protocol), DownloadClient: string(d.Downloader), Added: d.AddedAt.UTC().Format(time.RFC3339)}
	if d.OnHold() {
		rec.Status = "paused"
	}
	if d.StatusReason != "" {
		rec.TrackedDownloadStatus, rec.ErrorMessage = "warning", d.StatusReason
		rec.StatusMessages = []statusMessage{{Title: d.Title, Messages: []string{d.StatusReason}}}
	}
	if d.ETASeconds > 0 {
		eta := time.Duration(d.ETASeconds) * time.Second
		rec.Timeleft = timeleft(eta)
		rec.EstimatedCompletionTime = s.d.Now().Add(eta).UTC().Format(time.RFC3339)
	}
	return rec
}

// timeleft formats a duration as [d.]hh:mm:ss.
func timeleft(d time.Duration) string {
	sec := int64(d / time.Second)
	days, h, m, x := sec/86400, sec%86400/3600, sec%3600/60, sec%60
	if days > 0 {
		return fmt.Sprintf("%d.%02d:%02d:%02d", days, h, m, x)
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, x)
}

// itemRefs are a download's links to items of its own kind.
func itemRefs(d store.StoredDownload) []domain.ItemRef {
	var out []domain.ItemRef
	for _, l := range d.Links {
		if l.Item.Kind == d.Kind {
			out = append(out, l.Item)
		}
	}
	return out
}

// queueRecords expands downloads into one row per linked item.
func (s *Service) queueRecords(ctx context.Context, kind domain.MediaKind, fill func(context.Context, *queueRecord, domain.ItemRef)) ([]queueRecord, error) {
	list, err := s.downloads(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := []queueRecord{}
	for _, d := range list {
		refs := itemRefs(d)
		if len(refs) == 0 {
			out = append(out, s.baseRecord(d))
		}
		for _, ref := range refs {
			rec := s.baseRecord(d)
			fill(ctx, &rec, ref)
			out = append(out, rec)
		}
	}
	return out, nil
}

func (s *Service) queuePage(kind domain.MediaKind, fill func(context.Context, *queueRecord, domain.ItemRef)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := s.queueRecords(r.Context(), kind, fill)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		p := pageOf(r, recs)
		p.SortKey = "timeleft"
		ok(w, p)
	}
}

func (s *Service) queueDetails(kind domain.MediaKind, fill func(context.Context, *queueRecord, domain.ItemRef)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := s.queueRecords(r.Context(), kind, fill)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		ok(w, recs)
	}
}

type queueStatus struct {
	TotalCount      int  `json:"totalCount"`
	Count           int  `json:"count"`
	UnknownCount    int  `json:"unknownCount"`
	Errors          bool `json:"errors"`
	Warnings        bool `json:"warnings"`
	UnknownErrors   bool `json:"unknownErrors"`
	UnknownWarnings bool `json:"unknownWarnings"`
}

// queueStatus counts the downloads of a kind.
func (s *Service) queueStatus(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := s.downloads(r.Context(), kind)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		st := queueStatus{TotalCount: len(list), Count: len(list)}
		for _, d := range list {
			st.Errors = st.Errors || d.State == domain.StateFailed
			st.Warnings = st.Warnings || d.State == domain.StateStalled || d.State == domain.StateImportBlocked
		}
		ok(w, st)
	}
}
