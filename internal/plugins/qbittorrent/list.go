package qbittorrent

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// torrentInfo is the subset of /torrents/info this plugin reads.
type torrentInfo struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	State        string  `json:"state"`
	Progress     float64 `json:"progress"`
	Size         int64   `json:"size"`
	Downloaded   int64   `json:"downloaded"`
	DLSpeed      int64   `json:"dlspeed"`
	ETA          int64   `json:"eta"`
	NumSeeds     int     `json:"num_seeds"`
	NumLeechs    int     `json:"num_leechs"`
	SavePath     string  `json:"save_path"`
	ContentPath  string  `json:"content_path"`
	LastActivity int64   `json:"last_activity"`
}

// etaInfinity is qBittorrent's "no estimate" value.
const etaInfinity = 8640000

var states = map[string]domain.DownloadState{
	"error": domain.StateFailed, "missingFiles": domain.StateFailed,
	"uploading": domain.StateCompleted, "stalledUP": domain.StateCompleted, "pausedUP": domain.StateCompleted,
	"stoppedUP": domain.StateCompleted, "queuedUP": domain.StateCompleted, "checkingUP": domain.StateCompleted,
	"forcedUP":   domain.StateCompleted,
	"allocating": domain.StateDownloading, "downloading": domain.StateDownloading, "metaDL": domain.StateDownloading,
	"forcedMetaDL": domain.StateDownloading, "forcedDL": domain.StateDownloading, "checkingDL": domain.StateDownloading,
	"checkingResumeData": domain.StateDownloading, "moving": domain.StateDownloading,
	"stalledDL": domain.StateStalled,
	"queuedDL":  domain.StateQueued, "pausedDL": domain.StateQueued, "stoppedDL": domain.StateQueued,
}

// mapState translates a qBittorrent state; unknown states count as queued.
func mapState(s string) domain.DownloadState {
	if st, ok := states[s]; ok {
		return st
	}
	return domain.StateQueued
}

// List returns one snapshot per torrent in the given categories (all torrents
// when none are named). For completed torrents Files holds the content path
// (a file or the torrent's root folder) so the importer can find the payload.
func (c *Client) List(ctx context.Context, categories []string) ([]plugin.Snapshot, error) {
	if len(categories) == 0 {
		categories = []string{""}
	}
	seen := map[string]bool{}
	var out []plugin.Snapshot
	for _, cat := range categories {
		infos, err := c.info(ctx, cat)
		if err != nil {
			return nil, err
		}
		for _, ti := range infos {
			if !seen[ti.Hash] {
				seen[ti.Hash] = true
				out = append(out, ti.snapshot())
			}
		}
	}
	return out, nil
}

func (c *Client) info(ctx context.Context, category string) ([]torrentInfo, error) {
	q := url.Values{}
	if category != "" {
		q.Set("category", category)
	}
	resp, err := c.get(ctx, "torrents/info", q, maxList)
	if err != nil {
		return nil, err
	}
	var infos []torrentInfo
	if err := json.Unmarshal(resp.Body, &infos); err != nil {
		return nil, plugin.Wrap(plugin.ClassParse, "torrents/info", err)
	}
	return infos, nil
}

func (ti torrentInfo) snapshot() plugin.Snapshot {
	s := plugin.Snapshot{
		DownloadID: ti.Hash, Name: ti.Name, Category: ti.Category, State: mapState(ti.State),
		Progress: ti.Progress, Size: ti.Size, Downloaded: ti.Downloaded, Speed: ti.DLSpeed,
		Seeders: ti.NumSeeds, Peers: ti.NumLeechs, SavePath: ti.SavePath, ContentPath: ti.ContentPath,
	}
	if ti.ETA > 0 && ti.ETA < etaInfinity {
		s.ETASeconds = ti.ETA
	}
	if ti.LastActivity > 0 {
		t := time.Unix(ti.LastActivity, 0).UTC()
		s.LastActivityAt = &t
	}
	if s.State == domain.StateCompleted && ti.ContentPath != "" {
		s.Files = []string{ti.ContentPath}
	}
	if s.State == domain.StateFailed {
		s.Error = ti.State
	}
	return s
}

// exists reports whether the client holds a torrent with this hash.
func (c *Client) exists(ctx context.Context, hash string) (bool, error) {
	resp, err := c.get(ctx, "torrents/info", url.Values{"hashes": {hash}}, maxList)
	if err != nil {
		return false, err
	}
	var infos []torrentInfo
	if err := json.Unmarshal(resp.Body, &infos); err != nil {
		return false, plugin.Wrap(plugin.ClassParse, "torrents/info", err)
	}
	return len(infos) > 0, nil
}
