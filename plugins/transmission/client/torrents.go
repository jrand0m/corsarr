package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// rawTorrent is the subset of torrent-get this package reads.
type rawTorrent struct {
	HashString         string        `json:"hashString"`
	Name               string        `json:"name"`
	Status             int           `json:"status"`
	Error              int           `json:"error"`
	ErrorString        string        `json:"errorString"`
	PercentDone        float64       `json:"percentDone"`
	SizeWhenDone       int64         `json:"sizeWhenDone"`
	LeftUntilDone      int64         `json:"leftUntilDone"`
	ETA                int64         `json:"eta"`
	RateDownload       int64         `json:"rateDownload"`
	RateUpload         int64         `json:"rateUpload"`
	UploadRatio        float64       `json:"uploadRatio"`
	PeersSendingToUs   int           `json:"peersSendingToUs"`
	PeersGettingFromUs int           `json:"peersGettingFromUs"`
	DownloadDir        string        `json:"downloadDir"`
	Labels             []string      `json:"labels"`
	ActivityDate       int64         `json:"activityDate"`
	QueuePosition      int           `json:"queuePosition"`
	BandwidthPriority  int           `json:"bandwidthPriority"`
	Files              []rawFile     `json:"files"`
	TrackerStats       []trackerStat `json:"trackerStats"`
}

type rawFile struct {
	Name   string `json:"name"`
	Length int64  `json:"length"`
}

type trackerStat struct {
	Announce              string `json:"announce"`
	AnnounceState         int    `json:"announceState"`
	HasAnnounced          bool   `json:"hasAnnounced"`
	LastAnnounceSucceeded bool   `json:"lastAnnounceSucceeded"`
	LastAnnounceResult    string `json:"lastAnnounceResult"`
	SeederCount           int    `json:"seederCount"`
	LeecherCount          int    `json:"leecherCount"`
}

// listFields are the torrent-get fields a list needs; labels are added on
// servers that have them.
var listFields = []string{
	"hashString", "name", "status", "error", "errorString", "percentDone", "sizeWhenDone", "leftUntilDone",
	"eta", "rateDownload", "rateUpload", "uploadRatio", "peersSendingToUs", "peersGettingFromUs",
	"downloadDir", "activityDate", "queuePosition", "bandwidthPriority",
}

// Torrent is one torrent as the host sees it. Paths are mapped into the
// host's view.
type Torrent struct {
	ID          string // lower-case info hash
	Name        string
	Category    string
	Labels      []string
	State       State
	QueueState  State // State folded onto the host's queue states
	Progress    float64
	Size        int64 // bytes wanted
	Downloaded  int64 // wanted bytes present
	DownSpeed   int64 // bytes/s
	UpSpeed     int64 // bytes/s
	ETASeconds  int64 // 0 when unknown
	Ratio       float64
	Seeders     int // peers we download from
	Peers       int // peers we upload to
	SavePath    string
	ContentPath string   // the payload's file or root folder
	Files       []string // the content path once complete
	// LastActivity is the last transfer as Unix seconds; 0 when none.
	LastActivity int64
	// Error explains a state that needs a fix outside the host.
	Error string
	// ErrorCode and ErrorString are Transmission's own error report,
	// including tracker warnings that do not change the state.
	ErrorCode         int
	ErrorString       string
	QueuePosition     int
	BandwidthPriority int
}

// List returns every torrent in the given categories (all torrents when none
// are named) from one batched torrent-get that asks only for the fields a
// list needs.
func (c *Client) List(ctx context.Context, categories []string) ([]Torrent, error) {
	fields, err := c.fieldsFor(ctx, listFields)
	if err != nil {
		return nil, err
	}
	var res struct {
		Torrents []rawTorrent `json:"torrents"`
	}
	if err := c.call(ctx, "torrent-get", map[string]any{"fields": fields}, &res); err != nil {
		return nil, err
	}
	out := make([]Torrent, 0, len(res.Torrents))
	for _, rt := range res.Torrents {
		cat := c.categoryOf(rt, categories)
		if len(categories) > 0 && cat == "" {
			continue
		}
		out = append(out, c.torrent(rt, cat))
	}
	return out, nil
}

// fieldsFor adds the labels field on servers that report labels.
func (c *Client) fieldsFor(ctx context.Context, base []string) ([]string, error) {
	labels, err := c.hasLabels(ctx)
	if err != nil || !labels {
		return base, err
	}
	return append(append([]string{}, base...), "labels"), nil
}

func (c *Client) torrent(rt rawTorrent, category string) Torrent {
	st := stateOf(rt)
	t := Torrent{
		ID: strings.ToLower(rt.HashString), Name: rt.Name, Category: category, Labels: rt.Labels,
		State: st, QueueState: QueueState(st, rt.done()), Progress: rt.PercentDone,
		Size: rt.SizeWhenDone, Downloaded: rt.SizeWhenDone - rt.LeftUntilDone,
		DownSpeed: rt.RateDownload, UpSpeed: rt.RateUpload,
		Seeders: rt.PeersSendingToUs, Peers: rt.PeersGettingFromUs,
		SavePath: c.Local(rt.DownloadDir), ContentPath: c.Local(joinPath(rt.DownloadDir, rt.Name)),
		LastActivity: rt.ActivityDate, ErrorCode: rt.Error, ErrorString: rt.ErrorString,
		QueuePosition: rt.QueuePosition, BandwidthPriority: rt.BandwidthPriority,
	}
	if rt.ETA > 0 {
		t.ETASeconds = rt.ETA
	}
	if rt.UploadRatio > 0 {
		t.Ratio = rt.UploadRatio
	}
	if rt.Error == errLocal {
		t.Error = LocalErrorHint
	}
	if t.QueueState == StateCompleted {
		t.Files = []string{t.ContentPath}
	}
	return t
}

// categoryOf names the category a torrent belongs to among the wanted ones
// (any known one when none are wanted): a label first, then the category's
// download directory, then, with nothing wanted, the first label.
func (c *Client) categoryOf(rt rawTorrent, wanted []string) string {
	if cat := labelCategory(rt.Labels, wanted); cat != "" {
		return cat
	}
	if cat := c.dirCategory(rt.DownloadDir, wanted); cat != "" {
		return cat
	}
	if len(wanted) == 0 && len(rt.Labels) > 0 {
		return rt.Labels[0]
	}
	return ""
}

func labelCategory(labels, wanted []string) string {
	for _, w := range wanted {
		for _, l := range labels {
			if l == w {
				return w
			}
		}
	}
	return ""
}

// dirCategory finds the category whose directory is dir.
func (c *Client) dirCategory(dir string, wanted []string) string {
	dirs := c.categoryDirs()
	cats := wanted
	if len(cats) == 0 {
		cats = make([]string, 0, len(dirs))
		for cat := range dirs {
			cats = append(cats, cat)
		}
		sort.Strings(cats)
	}
	for _, cat := range cats {
		if d, ok := dirs[cat]; ok && samePath(canon(d), canon(dir)) {
			return cat
		}
	}
	return ""
}

func (c *Client) categoryDirs() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.dirs))
	for k, v := range c.dirs {
		out[k] = v
	}
	return out
}

// hashID checks a torrent id: an empty id would address every torrent.
func hashID(id string) ([]string, error) {
	h := strings.ToLower(strings.TrimSpace(id))
	if h == "" {
		return nil, errorf(ClassConfig, "torrent id is empty")
	}
	return []string{h}, nil
}

// one reads the named fields of one torrent. The answer is read from the
// returned rows, never from their count: a server that ignores the id
// filter answers with every torrent.
func (c *Client) one(ctx context.Context, id string, fields ...string) (rawTorrent, error) {
	ids, err := hashID(id)
	if err != nil {
		return rawTorrent{}, err
	}
	var res struct {
		Torrents []rawTorrent `json:"torrents"`
	}
	args := map[string]any{"ids": ids, "fields": append([]string{"hashString"}, fields...)}
	if err := c.call(ctx, "torrent-get", args, &res); err != nil {
		return rawTorrent{}, err
	}
	for _, rt := range res.Torrents {
		if strings.EqualFold(rt.HashString, ids[0]) {
			return rt, nil
		}
	}
	return rawTorrent{}, fmt.Errorf("torrent %s: %w", ids[0], ErrNotFound)
}

// Exists reports whether Transmission holds the torrent.
func (c *Client) Exists(ctx context.Context, id string) (bool, error) {
	_, err := c.one(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// FileEntry is one file of a torrent's payload.
type FileEntry struct {
	Path string // relative to the download directory
	Size int64
}

// Files returns a torrent's file list; it is empty while the metadata of a
// magnet is still being fetched.
func (c *Client) Files(ctx context.Context, id string) ([]FileEntry, error) {
	rt, err := c.one(ctx, id, "files")
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, len(rt.Files))
	for i, f := range rt.Files {
		out[i] = FileEntry{Path: f.Name, Size: f.Length}
	}
	return out, nil
}

// Tracker is one tracker of a torrent. URL holds the scheme and host only:
// private trackers put the user's passkey in the announce path or query.
type Tracker struct {
	URL     string
	Status  string // working | updating | not_contacted | not_working | disabled
	Message string
	Seeds   int
	Peers   int
}

// announceActive is the trackerStats announceState of an announce in flight.
const announceActive = 3

// Trackers lists a torrent's trackers.
func (c *Client) Trackers(ctx context.Context, id string) ([]Tracker, error) {
	rt, err := c.one(ctx, id, "trackerStats")
	if err != nil {
		return nil, err
	}
	out := make([]Tracker, 0, len(rt.TrackerStats))
	for _, s := range rt.TrackerStats {
		t := Tracker{URL: TrackerOrigin(s.Announce), Status: trackerStatus(s),
			Seeds: max(s.SeederCount, 0), Peers: max(s.LeecherCount, 0)}
		if t.Status == "not_working" {
			t.Message = scrubURLs(s.LastAnnounceResult)
		}
		out = append(out, t)
	}
	return out, nil
}

func trackerStatus(s trackerStat) string {
	switch {
	case s.AnnounceState == announceActive:
		return "updating"
	case !s.HasAnnounced:
		return "not_contacted"
	case s.LastAnnounceSucceeded:
		return "working"
	}
	return "not_working"
}

// TrackerOrigin keeps only the scheme and host of a tracker address.
func TrackerOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "(unparsable tracker address)"
	}
	return u.Scheme + "://" + u.Host
}

// scrubURLs cuts any address in a tracker message down to its origin.
func scrubURLs(msg string) string {
	words := strings.Fields(msg)
	for i, w := range words {
		if strings.Contains(w, "://") {
			words[i] = TrackerOrigin(w)
		}
	}
	return strings.Join(words, " ")
}
