package movies

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

func TestProfilesIncludeCustomOnesServingMovies(t *testing.T) {
	c, errs := decide.BuildCatalog(nil, []decide.ProfileDoc{
		{Key: "both", Name: "Both", Kinds: []domain.MediaKind{domain.KindTV, domain.KindMovie},
			Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-1080p"}}}, Cutoff: "webdl-1080p"},
		{Key: "tvonly", Name: "TV only", Kinds: []domain.MediaKind{domain.KindTV},
			Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-720p"}}}, Cutoff: "webdl-720p"},
	})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer decide.WithCatalog(c)()
	keys := map[string]bool{}
	for _, p := range Profiles() {
		keys[p.Key] = true
	}
	if !keys["both"] || keys["tvonly"] || !keys[DefaultProfileKey] {
		t.Fatalf("keys %v", keys)
	}
	if _, ok := ProfileByKey("tvonly"); ok {
		t.Fatal("tv-only profile offered to movies")
	}
	if p := ProfileOrDefault("gone"); p.Key() != DefaultProfileKey {
		t.Fatalf("fallback %q", p.Key())
	}
}
