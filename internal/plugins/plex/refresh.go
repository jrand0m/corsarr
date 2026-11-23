package plex

import (
	"context"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

type section struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Locations []struct {
		Path string `json:"path"`
	} `json:"Location"`
}

type sectionList struct {
	Directory []section `json:"Directory"`
}

func (s *Server) sections(ctx context.Context) ([]section, error) {
	var c container[sectionList]
	if err := s.get(ctx, "/library/sections", nil, &c); err != nil {
		return nil, err
	}
	return c.MediaContainer.Directory, nil
}

// mapping translates a path prefix as corsarr sees it into Plex's view.
type mapping struct{ from, to string }

func parseMappings(raw map[string]any) ([]mapping, error) {
	var out []mapping
	for from, v := range raw {
		to, _ := v.(string)
		if from == "" || to == "" {
			return nil, plugin.Errorf(plugin.ClassConfig, "sectionMap needs a Plex path for %q", from)
		}
		out = append(out, mapping{trimSlash(from), trimSlash(to)})
	}
	// Longest prefix first so nested mappings win.
	sort.Slice(out, func(i, j int) bool { return len(out[i].from) > len(out[j].from) })
	return out, nil
}

func trimSlash(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}

// hasPrefix reports whether p equals prefix or lies below it, on a path boundary.
func hasPrefix(p, prefix string) bool {
	return p == prefix || prefix == "/" || strings.HasPrefix(p, prefix+"/")
}

// plexPath applies the first matching mapping.
func (s *Server) plexPath(p string) string {
	p = trimSlash(p)
	for _, m := range s.mappings {
		if hasPrefix(p, m.from) {
			return m.to + strings.TrimPrefix(p, m.from)
		}
	}
	return p
}

// owner returns the section whose location is the longest prefix of p.
func owner(sections []section, p string) (key string, ok bool) {
	best := -1
	for _, sec := range sections {
		for _, loc := range sec.Locations {
			if l := trimSlash(loc.Path); hasPrefix(p, l) && len(l) > best {
				best, key, ok = len(l), sec.Key, true
			}
		}
	}
	return key, ok
}

type refreshKey struct{ section, path string }

// plan groups paths into distinct (section, path) refreshes and lists paths no library covers.
func (s *Server) plan(sections []section, paths []string) (reqs []refreshKey, uncovered []string) {
	seen := map[refreshKey]bool{}
	for _, p := range paths {
		pp := s.plexPath(p)
		key, ok := owner(sections, pp)
		if !ok {
			uncovered = append(uncovered, p)
			continue
		}
		if rk := (refreshKey{key, pp}); !seen[rk] {
			seen[rk] = true
			reqs = append(reqs, rk)
		}
	}
	return reqs, uncovered
}

// RefreshPaths asks Plex for a partial scan of each path in its owning
// section. Every covered path is refreshed even when others are not; paths no
// library covers are reported afterwards as a `path` failure.
func (s *Server) RefreshPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	secs, err := s.sections(ctx)
	if err != nil {
		return err
	}
	reqs, uncovered := s.plan(secs, paths)
	for _, r := range reqs {
		if err := s.get(ctx, "/library/sections/"+r.section+"/refresh", map[string][]string{"path": {r.path}}, nil); err != nil {
			return err
		}
	}
	if len(uncovered) > 0 {
		return plugin.Errorf(plugin.ClassPath, "no Plex library covers %s", strings.Join(uncovered, ", "))
	}
	return nil
}
