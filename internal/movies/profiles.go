package movies

import (
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// DefaultProfileKey is the profile a movie gets when none is chosen.
const DefaultProfileKey = "hd1080p"

// Profile is a quality profile with its stable key.
type Profile struct {
	Key     string
	Profile decide.QualityProfile
}

// ProfileKey derives the stable key of a shipped profile name ("HD-1080p" → "hd1080p").
func ProfileKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(name))
}

// Profiles lists the movie quality profiles (shipped defaults in the MVP).
func Profiles() []Profile {
	var out []Profile
	for _, p := range decide.DefaultProfiles(domain.KindMovie) {
		out = append(out, Profile{Key: ProfileKey(p.Spec().Name), Profile: p})
	}
	return out
}

// ProfileByKey finds a profile.
func ProfileByKey(key string) (decide.QualityProfile, bool) {
	for _, p := range Profiles() {
		if p.Key == key {
			return p.Profile, true
		}
	}
	return decide.QualityProfile{}, false
}
