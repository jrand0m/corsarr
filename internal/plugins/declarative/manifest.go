// Package declarative runs tracker definitions as indexer plugins: each valid
// definition becomes one plugin whose settings come from the definition, so a
// new tracker needs a data file and no code. Definitions ship embedded in the
// binary and can be added or overridden from the data directory.
package declarative

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Delivery is the manifest delivery form of definition plugins.
const Delivery = "declarative"

var settingTypes = map[string]plugin.FieldType{
	"text": plugin.FieldText, "password": plugin.FieldSecret, "checkbox": plugin.FieldBool, "select": plugin.FieldEnum,
}

var modeOrder = []string{"search", "tv", "movie", "music", "book"}

// Manifest describes a definition as a plugin.
func Manifest(d *definition.Definition) plugin.Manifest {
	m := plugin.Manifest{
		ID: d.ID, Kind: plugin.KindIndexer, Version: d.Version, APIVersion: plugin.APIVersion, Name: d.Name,
		Delivery: Delivery, Capabilities: []string{"search", "feed", "resolve"},
	}
	for _, mode := range modeOrder {
		if _, ok := d.Caps.Modes[mode]; ok && mode != "search" {
			m.Capabilities = append(m.Capabilities, mode+"-search")
		}
	}
	m.Settings = append(m.Settings, plugin.Field{Key: "baseUrl", Type: plugin.FieldURL, Label: "Site address", Default: d.Links[0],
		Help: mirrorsHelp(d.Links)})
	for _, s := range d.Settings {
		label := firstNonEmpty(s.Label, s.Key)
		help := firstNonEmpty(s.Help, label+" for "+d.Name+".")
		m.Settings = append(m.Settings, plugin.Field{Key: s.Key, Type: settingTypes[s.Type], Label: label, Help: help,
			Default: s.Default, Options: s.Options})
	}
	switch d.Login.Method {
	case definition.LoginCookie:
		m.Settings = append(m.Settings, plugin.Field{Key: "cookie", Type: plugin.FieldSecret, Label: "Session cookie", Required: true,
			Help: "The Cookie header your browser sends to the site while signed in."})
		m.SetupFlows = []plugin.SetupFlow{plugin.FlowCookiePaste}
	case definition.LoginForm:
		m.SetupFlows = []plugin.SetupFlow{plugin.FlowLoginForm}
	}
	m.Settings = append(m.Settings, plugin.Field{Key: "priority", Type: plugin.FieldNumber, Label: "Priority", Default: 25,
		Help: "Lower numbers win when the same release is found on several indexers (1 to 50)."})
	return m
}

func mirrorsHelp(links []string) string {
	if len(links) < 2 {
		return "Address of the site. Change it only when the site moves."
	}
	return "Address of the site. Known mirrors: " + strings.Join(links[1:], ", ") + "."
}

// Registration builds the plugin registration of a definition.
func Registration(d *definition.Definition) plugin.Registration {
	return plugin.Registration{Manifest: Manifest(d), New: func(inst plugin.Instance) (any, error) { return New(d, inst) }}
}

var searchModes = map[string]domain.SearchMode{
	"search": domain.SearchText, "tv": domain.SearchTV, "movie": domain.SearchMovie, "music": domain.SearchMusic, "book": domain.SearchBook,
}

// CapsOf reports a definition's capabilities in the indexer shape.
func CapsOf(d *definition.Definition) plugin.Caps {
	c := plugin.Caps{Categories: map[int]string{}, Limit: definition.MaxRows}
	ids := map[string]bool{}
	for _, mode := range modeOrder {
		params, ok := d.Caps.Modes[mode]
		if !ok {
			continue
		}
		c.Modes = append(c.Modes, searchModes[mode])
		for _, p := range params {
			if strings.HasSuffix(p, "id") && !ids[p] {
				ids[p] = true
				c.IDParams = append(c.IDParams, p)
			}
		}
	}
	sort.Strings(c.IDParams)
	for _, cat := range d.Caps.Categories {
		if _, ok := c.Categories[cat.Standard]; !ok || cat.Name != "" {
			c.Categories[cat.Standard] = firstNonEmpty(cat.Name, fmt.Sprint(cat.Standard))
		}
	}
	return c
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
