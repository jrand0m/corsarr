package music

import (
	"context"
	"slices"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Patch is a single-artist edit; nil fields stay. A metadata profile change
// never unmonitors an album: it only steers albums that appear later.
type Patch struct {
	Monitored       *bool     `json:"monitored"`
	ProfileKey      *string   `json:"profileKey"`
	MetadataProfile *string   `json:"metadataProfile"`
	MonitorNew      *string   `json:"monitorNew"`
	Tags            *[]string `json:"tags"`
}

// Update applies a patch and publishes item.updated with the changed fields.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Artist, error) {
	ar, err := s.Artist(ctx, id)
	if err != nil {
		return ar, err
	}
	changed, err := ApplyPatch(&ar, p)
	if err != nil || len(changed) == 0 {
		return ar, err
	}
	if err := s.d.Repo.SyncArtist(ctx, ar, nil); err != nil {
		return ar, err
	}
	s.publish(domain.EvItemUpdated, "user", []domain.EventItem{ar.EventItem()}, domain.ItemEventData{Monitored: ar.Monitored, Changed: changed})
	return ar, nil
}

type patchStep func(*domain.Artist, Patch) (string, error)

// ApplyPatch validates and applies a patch (pure); it names the changed fields.
func ApplyPatch(ar *domain.Artist, p Patch) ([]string, error) {
	var changed []string
	for _, f := range []patchStep{patchMonitored, patchProfile, patchMetadata, patchMonitorNew, patchTags} {
		name, err := f(ar, p)
		if err != nil {
			return nil, err
		}
		if name != "" {
			changed = append(changed, name)
		}
	}
	return changed, nil
}

func patchMonitored(ar *domain.Artist, p Patch) (string, error) {
	if p.Monitored == nil || *p.Monitored == ar.Monitored {
		return "", nil
	}
	ar.Monitored = *p.Monitored
	return "monitored", nil
}

func patchProfile(ar *domain.Artist, p Patch) (string, error) {
	if p.ProfileKey == nil || *p.ProfileKey == ar.ProfileKey {
		return "", nil
	}
	if _, ok := ProfileByKey(*p.ProfileKey); !ok {
		return "", FieldError{"profileKey", "unknown quality profile " + *p.ProfileKey}
	}
	ar.ProfileKey = *p.ProfileKey
	return "profile", nil
}

func patchMetadata(ar *domain.Artist, p Patch) (string, error) {
	if p.MetadataProfile == nil || *p.MetadataProfile == ar.MetadataProfile {
		return "", nil
	}
	if _, ok := domain.MetadataProfileByKey(*p.MetadataProfile); !ok {
		return "", FieldError{"metadataProfile", "unknown metadata profile " + *p.MetadataProfile}
	}
	ar.MetadataProfile = *p.MetadataProfile
	return "metadataProfile", nil
}

func patchMonitorNew(ar *domain.Artist, p Patch) (string, error) {
	if p.MonitorNew == nil || domain.MonitorNewItems(*p.MonitorNew) == ar.MonitorNew {
		return "", nil
	}
	m, err := domain.ParseMonitorNewItems(*p.MonitorNew)
	if err != nil {
		return "", FieldError{"monitorNew", err.Error()}
	}
	ar.MonitorNew = m
	return "monitorNew", nil
}

func patchTags(ar *domain.Artist, p Patch) (string, error) {
	if p.Tags == nil || slices.Equal(*p.Tags, ar.Tags) {
		return "", nil
	}
	ar.Tags = nonNil(*p.Tags)
	return "tags", nil
}
