// Package domain holds corsarr's shared data shapes.
// It has no I/O and no dependencies beyond the standard library: every other
// package builds on these types, so illegal states are made unrepresentable here.
package domain

import (
	"fmt"
	"regexp"
	"time"
)

// MediaKind is one of the supported media kinds.
type MediaKind string

const (
	KindTV    MediaKind = "tv"
	KindMovie MediaKind = "movie"
	KindMusic MediaKind = "music"
	KindBook  MediaKind = "book"
)

// ParseMediaKind validates outside input once, at the boundary.
func ParseMediaKind(s string) (MediaKind, error) {
	switch k := MediaKind(s); k {
	case KindTV, KindMovie, KindMusic, KindBook:
		return k, nil
	}
	return "", fmt.Errorf("unknown media kind %q", s)
}

// KindMode controls whether a media kind runs.
type KindMode string

const (
	ModeDisabled KindMode = "disabled"
	ModeShadow   KindMode = "shadow"
	ModeActive   KindMode = "active"
)

// Modes looks up a media kind's mode at call time, so a config change takes
// effect without a restart. A nil Modes treats every kind as active.
type Modes func(MediaKind) KindMode

// Of returns the kind's mode; unknown or unset kinds are active.
func (m Modes) Of(k MediaKind) KindMode {
	if m == nil {
		return ModeActive
	}
	if mode := m(k); mode == ModeShadow || mode == ModeDisabled {
		return mode
	}
	return ModeActive
}

// Active reports whether the kind may grab, import, write to clients, change
// files and notify.
func (m Modes) Active(k MediaKind) bool { return m.Of(k) == ModeActive }

// AllActive reports whether every item of an event belongs to an active kind.
func (m Modes) AllActive(items []EventItem) bool {
	for _, it := range items {
		if !m.Active(it.Ref.Kind) {
			return false
		}
	}
	return true
}

// ModeError says an action was refused because of the kind's mode.
type ModeError struct {
	Kind MediaKind
	Mode KindMode
}

func (e ModeError) Error() string {
	return fmt.Sprintf("%s is in %s mode: corsarr does not change anything for it", e.Kind, e.Mode)
}

// CheckActive returns a ModeError unless the kind is active.
func (m Modes) CheckActive(k MediaKind) error {
	if mode := m.Of(k); mode != ModeActive {
		return ModeError{Kind: k, Mode: mode}
	}
	return nil
}

// ItemRef identifies one wantable item (movie, episode, album, …).
type ItemRef struct {
	Kind    MediaKind `json:"kind"`
	ID      int64     `json:"id"`
	Variant string    `json:"variant,omitempty"` // e.g. ebook|audiobook
}

func (r ItemRef) String() string { return fmt.Sprintf("%s:%d", r.Kind, r.ID) }

// ObjectKey is a stable config object key: `[a-z][a-z0-9]*`.
type ObjectKey string

var objectKeyRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// ParseObjectKey enforces the key grammar so env paths stay unambiguous.
func ParseObjectKey(s string) (ObjectKey, error) {
	if !objectKeyRe.MatchString(s) {
		return "", fmt.Errorf("object key %q must match [a-z][a-z0-9]*", s)
	}
	return ObjectKey(s), nil
}

// Protocol is how a release is transported.
type Protocol string

const (
	ProtoTorrent Protocol = "torrent"
	ProtoUsenet  Protocol = "usenet"
	ProtoSource  Protocol = "source"
)

// IndexerRef names the indexer a release came from.
type IndexerRef struct {
	Key      ObjectKey `json:"key"`
	Name     string    `json:"name"`
	Priority int       `json:"priority"`
}

// Release is one search result. Download links stay
// inside the indexer layer and are resolved at grab time.
type Release struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Protocol    Protocol     `json:"protocol"`
	Size        int64        `json:"size"`
	PublishedAt time.Time    `json:"publishedAt"`
	Indexers    []IndexerRef `json:"indexers"`
	Categories  []int        `json:"categories,omitempty"`
	InfoHash    string       `json:"infoHash,omitempty"`
	GUID        string       `json:"guid,omitempty"`
	DetailsURL  string       `json:"detailsUrl,omitempty"`
	Seeders     int          `json:"seeders"`
	Peers       int          `json:"peers"`
	Grabs       int          `json:"grabs"`
	Flags       []string     `json:"flags,omitempty"`
	FileList    []string     `json:"fileList,omitempty"`
	// SwarmReported is true when the indexer sent seeders or peers, so a
	// zero means an empty swarm rather than "not reported".
	SwarmReported bool `json:"swarmReported,omitempty"`
	// Files is the number of files the indexer reports (0 = not reported).
	Files int `json:"files,omitempty"`
	// MinimumRatio is the seed ratio the tracker asks for (0 = not reported).
	MinimumRatio float64           `json:"minimumRatio,omitempty"`
	ExternalIDs  map[string]string `json:"externalIds,omitempty"`
	// DownloadURL is the indexer's payload link (magnet or .torrent URL). It
	// may carry indexer credentials, so it is never serialised to JSON; the
	// queue persists it sealed with the grab so a payload resolves after a restart.
	DownloadURL string `json:"-"`
}

// SearchMode is the indexer query form.
type SearchMode string

const (
	SearchText  SearchMode = "search"
	SearchTV    SearchMode = "tv"
	SearchMovie SearchMode = "movie"
	SearchMusic SearchMode = "music"
	SearchBook  SearchMode = "book"
)

// SearchOrigin says why a search runs.
type SearchOrigin string

const (
	OriginRSS         SearchOrigin = "rss"
	OriginAutomatic   SearchOrigin = "automatic"
	OriginInteractive SearchOrigin = "interactive"
)

// SearchRequest is built by a media-kind adapter and translated by indexers.
type SearchRequest struct {
	Kind     MediaKind         `json:"kind"`
	Mode     SearchMode        `json:"mode"`
	Text     string            `json:"text"`
	Titles   []string          `json:"titles,omitempty"`
	Year     int               `json:"year,omitempty"`
	IDs      map[string]string `json:"ids,omitempty"` // imdb, tmdb, tvdb, tvmaze, …
	Season   int               `json:"season,omitempty"`
	Episodes []int             `json:"episodes,omitempty"`
	Origin   SearchOrigin      `json:"origin"`
	ItemRefs []ItemRef         `json:"itemRefs"`
	// MaxResolution is a one-off resolution cap for this search (0 = none).
	MaxResolution int `json:"maxResolution,omitempty"`
}

// ReasonCategory classifies a decision reason.
type ReasonCategory string

const (
	ReasonSafety     ReasonCategory = "safety"
	ReasonPermanent  ReasonCategory = "permanent"
	ReasonPreference ReasonCategory = "preference"
	ReasonTemporary  ReasonCategory = "temporary"
)

// Reason is one failed rule with its evidence.
type Reason struct {
	RuleID   string         `json:"ruleId"`
	Category ReasonCategory `json:"category"`
	Message  string         `json:"message"`
	// Summary is a short plain line for a card ("Too large: 14.2 GB, max 9.5 GB").
	Summary string         `json:"summary,omitempty"`
	Values  map[string]any `json:"values,omitempty"`
}

// Override says whether a rejected release may still be grabbed by hand.
type Override string

const (
	OverrideAllowed Override = "allowed"
	OverrideConfirm Override = "confirm"
	OverrideNever   Override = "never"
)

// Decision is the full verdict for one release against N items; every rule is
// evaluated, so Reasons lists all failures.
type Decision struct {
	ReleaseID   string    `json:"releaseId"`
	ItemRefs    []ItemRef `json:"itemRefs"`
	Accepted    bool      `json:"accepted"`
	Reasons     []Reason  `json:"reasons"`
	Quality     string    `json:"quality"`
	Resolution  int       `json:"resolution"`
	FormatScore int       `json:"formatScore"`
	// Formats are the custom formats the release matched, with their scores.
	Formats  []FormatMatch `json:"formats,omitempty"`
	RankKey  string        `json:"rankKey"`
	Override Override      `json:"override"`
	// Desirability is how close the release is to the profile's target;
	// nil when its quality is outside the profile.
	Desirability *Desirability `json:"desirability,omitempty"`
}

// FormatMatch is a custom format a release matched under a profile.
type FormatMatch struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reject bool   `json:"reject,omitempty"`
}

// Position says on which side of the profile's target a release lies.
type Position string

const (
	PositionAbove    Position = "above"
	PositionOnTarget Position = "on_target"
	PositionBelow    Position = "below"
)

// Desirability measures a release against the profile's target from both
// sides: quality steps from the cutoff, size against the preferred size and
// custom-format score short of the cutoff format score.
type Desirability struct {
	Score           int      `json:"score"` // 0-100, 100 = on target
	Position        Position `json:"position"`
	QualitySteps    int      `json:"qualitySteps"` // + above the cutoff, - below
	Cutoff          string   `json:"cutoff"`
	SizeDelta       *float64 `json:"sizeDelta,omitempty"` // (size - preferred) / preferred
	PreferredSize   int64    `json:"preferredSize,omitempty"`
	FormatShortfall int      `json:"formatShortfall,omitempty"`
	FormatTarget    int      `json:"formatTarget,omitempty"`
}

// OverrideFor derives the strongest override rule from the reasons.
func OverrideFor(reasons []Reason) Override {
	o := OverrideAllowed
	for _, r := range reasons {
		switch r.Category {
		case ReasonSafety:
			return OverrideNever
		case ReasonPermanent:
			o = OverrideConfirm
		}
	}
	return o
}

// LinkStatus is the per-item status inside one download.
type LinkStatus string

const (
	LinkPending  LinkStatus = "pending"
	LinkImported LinkStatus = "imported"
	LinkBlocked  LinkStatus = "blocked"
	LinkSkipped  LinkStatus = "skipped"
	LinkFailed   LinkStatus = "failed"
)

// DownloadLink ties one item to a download (one grab → N items).
type DownloadLink struct {
	Item   ItemRef    `json:"item"`
	Status LinkStatus `json:"status"`
}

// Download is the queue aggregate owned by the download-clients module.
type Download struct {
	ID             int64          `json:"id"`
	Downloader     ObjectKey      `json:"downloader"`
	DownloadID     string         `json:"downloadId"` // torrent hash / nzo id
	Protocol       Protocol       `json:"protocol"`
	Kind           MediaKind      `json:"kind"`
	GrabID         int64          `json:"grabId"`
	Title          string         `json:"title"`
	State          DownloadState  `json:"state"`
	Links          []DownloadLink `json:"links"`
	Progress       float64        `json:"progress"`
	Size           int64          `json:"size"`
	Downloaded     int64          `json:"downloaded"`
	Speed          int64          `json:"speed"`
	UpSpeed        int64          `json:"upSpeed"` // bytes/s sent to peers
	Ratio          float64        `json:"ratio"`   // uploaded / downloaded, as the client reports it
	ETASeconds     int64          `json:"eta"`
	Seeders        int            `json:"seeders"`
	Peers          int            `json:"peers"`
	Category       string         `json:"category"`
	SavePath       string         `json:"savePath,omitempty"`
	Held           bool           `json:"held"`
	StatusReason   string         `json:"statusReason,omitempty"`
	AddedAt        time.Time      `json:"addedAt"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	LastActivityAt *time.Time     `json:"lastActivityAt,omitempty"`
}

// HoldCategory is the downloader category that puts a download on hold.
const HoldCategory = "hold"

// OnHold reports whether a download is held, by its flag or by sitting in the
// hold category. Automation and rules never touch a held download; the safety
// action still removes an unsafe payload.
func (d Download) OnHold() bool { return d.Held || d.Category == HoldCategory }

// Grab records sending one accepted release to a downloader.
type Grab struct {
	ID         int64        `json:"id"`
	Kind       MediaKind    `json:"kind"`
	ItemRefs   []ItemRef    `json:"itemRefs"`
	Release    Release      `json:"release"`
	Decision   Decision     `json:"decision"`
	Origin     SearchOrigin `json:"origin"`
	Downloader ObjectKey    `json:"downloader"`
	DownloadID string       `json:"downloadId"`
	SentAt     time.Time    `json:"sentAt"`
}

// ImportStatus is the outcome of importing one download.
type ImportStatus string

const (
	ImportImported ImportStatus = "imported"
	ImportPartial  ImportStatus = "partial"
	ImportBlocked  ImportStatus = "blocked"
	ImportFailed   ImportStatus = "failed"
)

// ImportItem is the per-item result of an import.
type ImportItem struct {
	Item          ItemRef  `json:"item"`
	Outcome       string   `json:"outcome"` // imported|upgraded|skipped|blocked
	NewPaths      []string `json:"newPaths,omitempty"`
	ReplacedPaths []string `json:"replacedPaths,omitempty"`
	Quality       string   `json:"quality,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// ImportOutcome is reported back to the queue and published as import.* events.
type ImportOutcome struct {
	DownloadID int64        `json:"downloadId"`
	Status     ImportStatus `json:"status"`
	Items      []ImportItem `json:"items"`
	ReasonCode string       `json:"reasonCode,omitempty"`
}
