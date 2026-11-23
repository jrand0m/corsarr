package tv

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Patch is a single-series edit; nil fields stay. Monitor applies a
// monitoring mode once to every episode.
type Patch struct {
	Monitored     *bool     `json:"monitored"`
	ProfileKey    *string   `json:"profileKey"`
	MonitorNew    *string   `json:"monitorNew"`
	SeasonFolders *bool     `json:"seasonFolders"`
	SeriesType    *string   `json:"seriesType"`
	Tags          *[]string `json:"tags"`
	Monitor       *string   `json:"monitor"`
}

// Update applies a patch and publishes item.updated with the changed fields.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Series, error) {
	ser, err := s.Series(ctx, id)
	if err != nil {
		return ser, err
	}
	changed, err := applyPatch(&ser, p)
	if err != nil {
		return ser, err
	}
	seasons, eps, err := s.remonitor(ctx, ser, p.Monitor)
	if err != nil {
		return ser, err
	}
	if eps != nil {
		changed = append(changed, "monitoring")
	}
	if len(changed) == 0 {
		return ser, nil
	}
	if err := s.d.Repo.Sync(ctx, ser, seasons, eps, nil); err != nil {
		return ser, err
	}
	s.publish(domain.EvItemUpdated, "user", []domain.EventItem{ser.EventItem()}, domain.ItemEventData{Monitored: ser.Monitored, Changed: changed})
	s.Kick()
	return ser, nil
}

// remonitor applies a monitoring mode to the stored episodes (nil, nil when
// no mode was asked for).
func (s *Service) remonitor(ctx context.Context, ser domain.Series, mode *string) ([]domain.Season, []domain.Episode, error) {
	if mode == nil {
		return nil, nil, nil
	}
	m, err := domain.ParseMonitorMode(*mode)
	if err != nil {
		return nil, nil, FieldError{"monitor", err.Error()}
	}
	stored, err := s.d.Repo.Episodes(ctx, ser.ID)
	if err != nil {
		return nil, nil, err
	}
	eps, seasons := ApplyMode(m, stored, s.d.Now())
	return seasons, eps, nil
}

type patchStep func(*domain.Series, Patch) (string, error)

func applyPatch(ser *domain.Series, p Patch) ([]string, error) {
	var changed []string
	for _, f := range []patchStep{patchMonitored, patchProfile, patchMonitorNew, patchSeasonFolders, patchType, patchTags} {
		name, err := f(ser, p)
		if err != nil {
			return nil, err
		}
		if name != "" {
			changed = append(changed, name)
		}
	}
	return changed, nil
}

func patchMonitored(ser *domain.Series, p Patch) (string, error) {
	if p.Monitored == nil || *p.Monitored == ser.Monitored {
		return "", nil
	}
	ser.Monitored = *p.Monitored
	return "monitored", nil
}

func patchProfile(ser *domain.Series, p Patch) (string, error) {
	if p.ProfileKey == nil || *p.ProfileKey == ser.ProfileKey {
		return "", nil
	}
	if _, ok := ProfileByKey(*p.ProfileKey); !ok {
		return "", FieldError{"profileKey", "unknown quality profile " + *p.ProfileKey}
	}
	ser.ProfileKey = *p.ProfileKey
	return "profile", nil
}

func patchMonitorNew(ser *domain.Series, p Patch) (string, error) {
	if p.MonitorNew == nil || domain.MonitorNew(*p.MonitorNew) == ser.MonitorNew {
		return "", nil
	}
	m, err := domain.ParseMonitorNew(*p.MonitorNew)
	if err != nil {
		return "", FieldError{"monitorNew", err.Error()}
	}
	ser.MonitorNew = m
	return "monitorNew", nil
}

func patchSeasonFolders(ser *domain.Series, p Patch) (string, error) {
	if p.SeasonFolders == nil || *p.SeasonFolders == ser.SeasonFolders {
		return "", nil
	}
	ser.SeasonFolders = *p.SeasonFolders
	return "seasonFolders", nil
}

func patchType(ser *domain.Series, p Patch) (string, error) {
	if p.SeriesType == nil || domain.SeriesType(*p.SeriesType) == ser.Type {
		return "", nil
	}
	t, err := domain.ParseSeriesType(*p.SeriesType)
	if err != nil {
		return "", FieldError{"seriesType", err.Error()}
	}
	ser.Type = t
	return "seriesType", nil
}

func patchTags(ser *domain.Series, p Patch) (string, error) {
	if p.Tags == nil || slices.Equal(*p.Tags, ser.Tags) {
		return "", nil
	}
	ser.Tags = nonNil(*p.Tags)
	return "tags", nil
}

// SetSeasonMonitored toggles a season and every episode in it; later
// episodes of the season follow the flag.
func (s *Service) SetSeasonMonitored(ctx context.Context, id int64, season int, monitored bool) error {
	ser, err := s.Series(ctx, id)
	if err != nil {
		return err
	}
	stored, err := s.d.Repo.Episodes(ctx, id)
	if err != nil {
		return err
	}
	var ids []int64
	for _, e := range stored {
		if e.Season == season {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return ErrNotFound
	}
	if err := s.d.Repo.Sync(ctx, ser, []domain.Season{{SeriesID: id, Number: season, Monitored: monitored}}, nil, nil); err != nil {
		return err
	}
	_, err = s.setMonitored(ctx, ser, ids, monitored)
	return err
}

// SetEpisodesMonitored toggles episodes (of any series); it returns how
// many changed. Queued downloads of unmonitored episodes keep running.
func (s *Service) SetEpisodesMonitored(ctx context.Context, ids []int64, monitored bool) (int, error) {
	bySeries := map[int64][]int64{}
	var order []int64
	for _, id := range ids {
		e, err := s.Episode(ctx, id)
		if err != nil {
			return 0, err
		}
		if _, ok := bySeries[e.SeriesID]; !ok {
			order = append(order, e.SeriesID)
		}
		bySeries[e.SeriesID] = append(bySeries[e.SeriesID], id)
	}
	total := 0
	for _, sid := range order {
		ser, err := s.Series(ctx, sid)
		if err != nil {
			return total, err
		}
		n, err := s.setMonitored(ctx, ser, bySeries[sid], monitored)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *Service) setMonitored(ctx context.Context, ser domain.Series, ids []int64, monitored bool) (int, error) {
	n, err := s.d.Repo.SetMonitored(ctx, ids, monitored)
	if err != nil || n == 0 {
		return n, err
	}
	items := make([]domain.EventItem, 0, len(ids))
	for _, id := range ids {
		if !monitored {
			s.logErr(s.d.Repo.SetAvailablePublished(ctx, id, false))
		}
		items = append(items, domain.Episode{ID: id}.EventItem(ser.Title))
	}
	s.publish(domain.EvItemUpdated, "user", items, domain.ItemEventData{Monitored: monitored, Changed: []string{"monitored"}})
	s.Kick()
	return n, nil
}

// RefreshResult says what a metadata refresh changed.
type RefreshResult struct {
	Added    int `json:"added"`
	Removed  int `json:"removed"`
	Orphaned int `json:"orphaned"`
}

// Refresh re-reads a series from its provider: metadata, new episodes with
// the series' monitoring rules, changed titles and air dates. It never
// deletes an episode that has a file and is idempotent.
func (s *Service) Refresh(ctx context.Context, id int64) (RefreshResult, error) {
	md, ok := s.d.Metadata()
	if !ok {
		return RefreshResult{}, ErrNoMetadata
	}
	ser, err := s.Series(ctx, id)
	if err != nil {
		return RefreshResult{}, err
	}
	rec, err := md.Series(ctx, map[string]string{"tmdb": strconv.FormatInt(ser.TMDBID, 10)})
	if err != nil {
		return RefreshResult{}, err
	}
	seasons, err := s.d.Repo.Seasons(ctx, id)
	if err != nil {
		return RefreshResult{}, err
	}
	stored, err := s.d.Repo.Episodes(ctx, id)
	if err != nil {
		return RefreshResult{}, err
	}
	m := MergeRecord(ser, seasons, stored, rec)
	ApplyRecord(&ser, rec)
	if err := s.d.Repo.Sync(ctx, ser, m.Seasons, m.Upserts, m.Drop); err != nil {
		return RefreshResult{}, err
	}
	s.logErr(s.d.Repo.MarkRefreshed(ctx, id, s.d.Now()))
	s.publish(domain.EvItemUpdated, "system", []domain.EventItem{ser.EventItem()},
		domain.ItemEventData{Monitored: ser.Monitored, Changed: []string{"metadata"}})
	s.Kick()
	return RefreshResult{Added: m.Added, Removed: len(m.Drop), Orphaned: m.Orphaned}, nil
}

// RefreshJob is the daily metadata refresh job; RefreshInterval its cadence.
const (
	RefreshJob      = "tv.refresh"
	RefreshInterval = 24 * time.Hour
	refreshAge      = 20 * time.Hour // a series refreshed more recently is skipped
)

// RefreshAll refreshes every series not refreshed within the last day; one
// failing series does not stop the others.
func (s *Service) RefreshAll(ctx context.Context) error {
	if _, ok := s.d.Metadata(); !ok {
		return nil
	}
	var ids []int64
	err := s.d.Repo.EachSeries(ctx, func(ser domain.Series) error {
		at, err := s.d.Repo.RefreshedAt(ctx, ser.ID)
		if err == nil && s.d.Now().Sub(at) >= refreshAge {
			ids = append(ids, ser.ID)
		}
		return err
	})
	errs := []error{err}
	for _, id := range ids {
		_, err := s.Refresh(ctx, id)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
