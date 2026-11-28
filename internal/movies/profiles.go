package movies

import (
	"log/slog"
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

// Profiles lists the profiles movies may use: built-ins, then custom ones.
func Profiles() []Profile {
	var out []Profile
	for _, p := range decide.ProfilesFor(domain.KindMovie) {
		out = append(out, Profile{Key: p.Key(), Profile: p})
	}
	return out
}

// ProfileByKey finds a profile movies may use.
func ProfileByKey(key string) (decide.QualityProfile, bool) {
	return decide.Lookup(domain.KindMovie, key)
}

// ProfileOrDefault is the movie's profile, or the default one when its key
// no longer names a profile (removed from the config file).
func ProfileOrDefault(key string) decide.QualityProfile {
	return OrDefault(domain.KindMovie, key, DefaultProfileKey)
}

// OrDefault looks a kind's profile up and falls back to the kind's default
// profile, logging the missing key.
func OrDefault(kind domain.MediaKind, key, fallback string) decide.QualityProfile {
	if p, ok := decide.Lookup(kind, key); ok {
		return p
	}
	slog.Warn("quality profile not found; using the default profile", "kind", kind, "profile", key, "default", fallback)
	p, _ := decide.Lookup(kind, fallback)
	return p
}
