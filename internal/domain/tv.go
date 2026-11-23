package domain

import (
	"fmt"
	"strings"
	"time"
)

// SeriesType says how a series numbers its episodes.
type SeriesType string

const (
	SeriesStandard SeriesType = "standard"
	SeriesAnime    SeriesType = "anime"
	SeriesDaily    SeriesType = "daily"
)

// ParseSeriesType validates outside input; "" means standard.
func ParseSeriesType(s string) (SeriesType, error) {
	switch t := SeriesType(s); t {
	case "":
		return SeriesStandard, nil
	case SeriesStandard, SeriesAnime, SeriesDaily:
		return t, nil
	}
	return "", fmt.Errorf("series type %q must be standard, anime or daily", s)
}

// MonitorNew says whether seasons that appear later are monitored.
type MonitorNew string

const (
	MonitorNewAll  MonitorNew = "all"
	MonitorNewNone MonitorNew = "none"
)

// ParseMonitorNew validates outside input; "" means all.
func ParseMonitorNew(s string) (MonitorNew, error) {
	switch m := MonitorNew(s); m {
	case "":
		return MonitorNewAll, nil
	case MonitorNewAll, MonitorNewNone:
		return m, nil
	}
	return "", fmt.Errorf("monitor new seasons %q must be all or none", s)
}

// MonitorMode is a one-time action that sets every episode's monitored flag.
type MonitorMode string

const (
	MonitorAll          MonitorMode = "all"
	MonitorFuture       MonitorMode = "future"
	MonitorMissing      MonitorMode = "missing"
	MonitorExisting     MonitorMode = "existing"
	MonitorFirstSeason  MonitorMode = "firstSeason"
	MonitorLatestSeason MonitorMode = "latestSeason"
	MonitorPilot        MonitorMode = "pilot"
	MonitorNone         MonitorMode = "none"
)

// ParseMonitorMode validates outside input; "" means all.
func ParseMonitorMode(s string) (MonitorMode, error) {
	switch m := MonitorMode(s); m {
	case "":
		return MonitorAll, nil
	case MonitorAll, MonitorFuture, MonitorMissing, MonitorExisting, MonitorFirstSeason, MonitorLatestSeason,
		MonitorPilot, MonitorNone:
		return m, nil
	}
	return "", fmt.Errorf("unknown monitoring mode %q", s)
}

// VariantSeries marks a series-level item reference; episode references
// (the wanted unit) carry no variant.
const VariantSeries = "series"

// Series is the TV series item.
type Series struct {
	ID            int64      `json:"id"`
	TMDBID        int64      `json:"tmdbId"`
	TVDBID        int64      `json:"tvdbId,omitempty"`
	IMDBID        string     `json:"imdbId,omitempty"`
	Title         string     `json:"title"`
	SortTitle     string     `json:"sortTitle"`
	Year          int        `json:"year"`
	Status        string     `json:"status"` // continuing | ended | upcoming | ""
	Network       string     `json:"network,omitempty"`
	Overview      string     `json:"overview,omitempty"`
	Genres        []string   `json:"genres"`
	RuntimeMin    int        `json:"runtimeMin,omitempty"`
	AltTitles     []string   `json:"altTitles"`
	PosterURL     string     `json:"posterUrl,omitempty"`
	RootFolder    string     `json:"rootFolder"`
	Path          string     `json:"path"`
	ProfileKey    string     `json:"profileKey"`
	Tags          []string   `json:"tags"`
	Type          SeriesType `json:"seriesType"`
	SeasonFolders bool       `json:"seasonFolders"`
	MonitorNew    MonitorNew `json:"monitorNew"`
	Monitored     bool       `json:"monitored"`
	AddedAt       time.Time  `json:"addedAt"`
}

// Ref is the series-level item reference.
func (s Series) Ref() ItemRef { return ItemRef{Kind: KindTV, ID: s.ID, Variant: VariantSeries} }

// EventItem snapshots the series for an event envelope.
func (s Series) EventItem() EventItem { return EventItem{Ref: s.Ref(), Title: s.Title, Tags: s.Tags} }

// Season is one season of a series; its monitored flag decides whether
// episodes that appear in it later are monitored.
type Season struct {
	SeriesID  int64 `json:"seriesId"`
	Number    int   `json:"number"`
	Monitored bool  `json:"monitored"`
}

// Episode is the TV wanted unit.
type Episode struct {
	ID         int64      `json:"id"`
	SeriesID   int64      `json:"seriesId"`
	Season     int        `json:"season"`
	Number     int        `json:"number"`
	Absolute   int        `json:"absolute,omitempty"`
	Title      string     `json:"title"`
	AirAt      *time.Time `json:"airAt"`
	RuntimeMin int        `json:"runtimeMin,omitempty"`
	Monitored  bool       `json:"monitored"`
	FileID     int64      `json:"fileId,omitempty"` // 0 = no file
	InMetadata bool       `json:"inMetadata"`       // false: removed upstream but kept for its file
}

// Ref is the episode's item reference.
func (e Episode) Ref() ItemRef { return ItemRef{Kind: KindTV, ID: e.ID} }

// Code is the SxxEyy label of the episode.
func (e Episode) Code() string { return fmt.Sprintf("S%02dE%02d", e.Season, e.Number) }

// EventItem snapshots the episode for an event envelope; Series names the
// show so messages can collapse many episodes into one line.
func (e Episode) EventItem(series string) EventItem {
	return EventItem{Ref: e.Ref(), Title: series + " " + e.Code(), Series: series}
}

// ItemsTitle renders the item titles of an event for a short message: the
// episodes of one series collapse into "Show · S01 · 10 episodes".
func ItemsTitle(items []EventItem) string {
	var names []string
	for i := 0; i < len(items); {
		j := i + 1
		for j < len(items) && items[i].Series != "" && items[j].Series == items[i].Series {
			j++
		}
		names = append(names, groupTitle(items[i:j]))
		i = j
	}
	return strings.Join(names, ", ")
}

func groupTitle(group []EventItem) string {
	if len(group) == 1 {
		return group[0].Title
	}
	seasons := map[string]bool{}
	for _, it := range group {
		if code := strings.TrimPrefix(it.Title, it.Series+" "); len(code) >= 3 {
			seasons[code[:3]] = true
		}
	}
	scope := fmt.Sprintf("%d seasons", len(seasons))
	if len(seasons) == 1 {
		for s := range seasons {
			scope = s
		}
	}
	return fmt.Sprintf("%s · %s · %d episodes", group[0].Series, scope, len(group))
}

// KindOf is the media kind of an event's items (the first item decides);
// def applies when there are none.
func KindOf(items []EventItem, def MediaKind) MediaKind {
	if len(items) > 0 && items[0].Ref.Kind != "" {
		return items[0].Ref.Kind
	}
	return def
}
