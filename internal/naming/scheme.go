package naming

import "path/filepath"

// Scheme is one media kind's naming: whether imported files are renamed,
// how colons are replaced, and the templates of the item folder, the folder
// inside it (season or album; empty for movies) and the file.
type Scheme struct {
	Rename bool
	Colon  ColonMode
	Folder string
	Sub    string
	File   string
}

// DefaultTV is the built-in TV naming: series folder, season folder, episode file.
func DefaultTV() Scheme {
	return Scheme{Rename: true, Colon: ColonSmart, Folder: DefaultSeriesFolder, Sub: DefaultSeasonFolder, File: DefaultEpisodeFile}
}

// DefaultMovie is the built-in movie naming: movie folder and movie file.
func DefaultMovie() Scheme {
	return Scheme{Rename: true, Colon: ColonSmart, Folder: DefaultMovieFolder, File: DefaultMovieFile}
}

// DefaultMusic is the built-in music naming: artist folder, album folder, track file.
func DefaultMusic() Scheme {
	return Scheme{Rename: true, Colon: ColonSmart, Folder: DefaultArtistFolder, Sub: DefaultAlbumFolder, File: DefaultTrackFile}
}

// Or fills the scheme's empty templates from def.
func (s Scheme) Or(def Scheme) Scheme {
	if s.Folder == "" {
		s.Folder = def.Folder
	}
	if s.Sub == "" {
		s.Sub = def.Sub
	}
	if s.File == "" {
		s.File = def.File
	}
	if s.Colon == "" {
		s.Colon = def.Colon
	}
	return s
}

// FolderName renders the item folder; def is used when the template does
// not parse.
func (s Scheme) FolderName(v Values, def string) string { return s.render(s.Folder, def, v, "") }

// SubName renders the folder inside the item folder.
func (s Scheme) SubName(v Values, def string) string { return s.render(s.Sub, def, v, "") }

// FileName renders the file name with its extension; with renaming off it
// is the original file's own name.
func (s Scheme) FileName(v Values, def, ext, original string) string {
	if !s.Rename && original != "" {
		return filepath.Base(original)
	}
	return s.render(s.File, def, v, ext)
}

func (s Scheme) render(tmpl, def string, v Values, ext string) string {
	o := Options{Colon: s.Colon, Extension: ext}
	t, err := ParseTemplate(tmpl)
	if err != nil {
		t, _ = ParseTemplate(def)
	}
	name, _ := o.Render(t, v)
	return name
}
