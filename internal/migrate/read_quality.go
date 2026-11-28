package migrate

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// Wire shapes of custom formats, full quality profiles and quality
// definitions, as the sources' public API returns them.

type wSpecOption struct {
	Value int
	Name  string
}

type wSpecField struct {
	Name          string
	Value         json.RawMessage
	SelectOptions []wSpecOption
}

type wSpec struct {
	Name           string
	Implementation string
	Negate         bool
	Required       bool
	Fields         []wSpecField
}

type wCustomFormat struct {
	ID             int
	Name           string
	Specifications []wSpec
}

type wFormatScore struct {
	Format int
	Name   string
	Score  int
}

type wSize struct {
	Quality struct {
		ID   int
		Name string
	}
	MinSize       *float64
	MaxSize       *float64
	PreferredSize *float64
}

// SrcFormat is a source custom format with its conditions.
type SrcFormat struct {
	ID    int
	Name  string
	Specs []SrcSpec
}

// SrcSpec is one source condition. Values holds each field's value as text
// ("1080", a regex, "true"); Options names a select field's chosen value.
type SrcSpec struct {
	Name     string
	Impl     string
	Negate   bool
	Required bool
	Values   map[string]string
	Options  map[string]string
}

// SrcQualityItem is one rung or group of a source profile, worst first.
type SrcQualityItem struct {
	ID      int // group id, or the quality id
	Name    string
	Group   bool
	Allowed bool
	Members []string // quality names (one for a rung)
}

// SrcScore is a profile's score for a source format.
type SrcScore struct {
	Format int
	Name   string
	Score  int
}

// SrcSize is a source quality definition in MB per minute; nil = unlimited
// (or unset).
type SrcSize struct {
	Min, Preferred, Max *float64
}

func readFormats(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wCustomFormat
	if err := c.get(ctx, "/customformat", nil, &list); err != nil {
		return err
	}
	for _, f := range list {
		s.CustomFormats = append(s.CustomFormats, srcFormat(f))
	}
	return nil
}

func srcFormat(f wCustomFormat) SrcFormat {
	out := SrcFormat{ID: f.ID, Name: f.Name}
	for _, sp := range f.Specifications {
		spec := SrcSpec{Name: sp.Name, Impl: sp.Implementation, Negate: sp.Negate, Required: sp.Required,
			Values: map[string]string{}, Options: map[string]string{}}
		for _, fl := range sp.Fields {
			v := rawText(fl.Value)
			spec.Values[fl.Name] = v
			for _, o := range fl.SelectOptions {
				if strconv.Itoa(o.Value) == v {
					spec.Options[fl.Name] = o.Name
				}
			}
		}
		out.Specs = append(out.Specs, spec)
	}
	return out
}

// rawText turns a JSON scalar into text: strings unquoted, numbers and
// booleans as written; null and absent become "".
func rawText(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil && f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return t
}

// srcItems flattens profile items into ranks, worst first.
func srcItems(items []wProfileItem) []SrcQualityItem {
	out := make([]SrcQualityItem, 0, len(items))
	for _, it := range items {
		if it.Quality != nil {
			out = append(out, SrcQualityItem{ID: it.Quality.ID, Name: it.Quality.Name, Allowed: it.Allowed, Members: []string{it.Quality.Name}})
			continue
		}
		g := SrcQualityItem{ID: it.ID, Name: it.Name, Group: true, Allowed: it.Allowed}
		for _, m := range it.Items {
			if m.Quality != nil {
				g.Members = append(g.Members, m.Quality.Name)
			}
		}
		out = append(out, g)
	}
	return out
}

func srcScores(items []wFormatScore) []SrcScore {
	out := make([]SrcScore, 0, len(items))
	for _, f := range items {
		out = append(out, SrcScore(f))
	}
	return out
}

func readDefinitions(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wSize
	if err := c.get(ctx, "/qualitydefinition", nil, &list); err != nil {
		return err
	}
	s.Sizes = map[string]SrcSize{}
	for _, d := range list {
		s.Sizes[d.Quality.Name] = SrcSize{Min: d.MinSize, Preferred: d.PreferredSize, Max: d.MaxSize}
	}
	return nil
}
