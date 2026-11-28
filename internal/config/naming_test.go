package config

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
)

func TestNamingDefaultsToTheBuiltInTemplates(t *testing.T) {
	c := mustLoad(t, inputs(nil, nil, nil))
	for k, want := range map[domain.MediaKind]naming.Scheme{domain.KindTV: naming.DefaultTV(), domain.KindMovie: naming.DefaultMovie(),
		domain.KindMusic: naming.DefaultMusic()} {
		if got := c.NamingFor(k); got != want {
			t.Errorf("%s: %+v, want %+v", k, got, want)
		}
	}
	if (*Config)(nil).NamingFor(domain.KindMovie) != naming.DefaultMovie() {
		t.Fatal("nil config must name like the built-in templates")
	}
}

func TestNamingSectionIsReadAndValidated(t *testing.T) {
	file := "naming:\n  movie:\n    rename: false\n    colon: dash\n    movieFile: '{Movie Title} [{Quality Full}]'\n"
	c := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file}))
	m := c.NamingFor(domain.KindMovie)
	if m.Rename || m.Colon != naming.ColonDash || m.File != "{Movie Title} [{Quality Full}]" || m.Folder != naming.DefaultMovieFolder {
		t.Fatalf("%+v", m)
	}
	if c.ManagedBy("naming.movie.movieFile") == nil || c.ManagedBy("naming.tv.episodeFile") != nil {
		t.Fatal("managed markers")
	}
	bad := "naming:\n  tv:\n    episodeFile: '{Nope}'\n"
	_, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": bad}), testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "unknown token") {
		t.Fatalf("want template error, got %v", err)
	}
}
