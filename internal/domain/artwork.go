package domain

// ArtRole says how a piece of artwork is used.
type ArtRole byte

const (
	ArtPoster   ArtRole = 'p' // posters and covers
	ArtBackdrop ArtRole = 'b' // fanart and backdrops
)

// ArtLink turns a provider artwork address into the address the web UI
// loads it from (the artwork cache). Nil keeps the address as it is.
type ArtLink func(role ArtRole, origin string) string

// Of applies the link; empty stays empty.
func (f ArtLink) Of(role ArtRole, origin string) string {
	if f == nil || origin == "" {
		return origin
	}
	return f(role, origin)
}
