package torznab

import (
	"encoding/xml"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// parsedCaps is the Torznab capabilities document parsed once at the boundary.
type parsedCaps struct {
	plugin.Caps
	params map[domain.SearchMode]map[string]bool // supportedParams per mode
}

func (c parsedCaps) supports(m domain.SearchMode, param string) bool { return c.params[m][param] }

func (c parsedCaps) hasMode(m domain.SearchMode) bool { _, ok := c.params[m]; return ok }

type errorDoc struct {
	XMLName     xml.Name `xml:"error"`
	Code        int      `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

type capsDoc struct {
	XMLName xml.Name `xml:"caps"`
	Limits  struct {
		Max     int `xml:"max,attr"`
		Default int `xml:"default,attr"`
	} `xml:"limits"`
	Searching struct {
		Items []searchEl `xml:",any"`
	} `xml:"searching"`
	Categories []catEl `xml:"categories>category"`
}

type searchEl struct {
	XMLName   xml.Name
	Available string `xml:"available,attr"`
	Params    string `xml:"supportedParams,attr"`
}

type catEl struct {
	ID   int     `xml:"id,attr"`
	Name string  `xml:"name,attr"`
	Sub  []catEl `xml:"subcat"`
}

var modeElems = map[string]domain.SearchMode{
	"search": domain.SearchText, "tv-search": domain.SearchTV, "movie-search": domain.SearchMovie,
	"audio-search": domain.SearchMusic, "book-search": domain.SearchBook,
}

// serverError turns a Torznab <error code=… description=…/> document into a
// classified error; it returns nil when body is not an error document.
func serverError(body []byte) error {
	var e errorDoc
	if unmarshal(body, &e) != nil {
		return nil
	}
	switch {
	case e.Code >= 100 && e.Code < 200:
		return plugin.Errorf(plugin.ClassAuth, "indexer error %d: %s", e.Code, e.Description)
	case e.Code == 500:
		return plugin.Errorf(plugin.ClassRateLimited, "indexer error %d: %s", e.Code, e.Description)
	case e.Code >= 200 && e.Code < 300:
		return plugin.Errorf(plugin.ClassConfig, "indexer error %d: %s", e.Code, e.Description)
	}
	return plugin.Errorf(plugin.ClassServerError, "indexer error %d: %s", e.Code, e.Description)
}

func parseCaps(body []byte, contentType string) (parsedCaps, error) {
	body = cleanXML(body, contentType)
	if err := serverError(body); err != nil {
		return parsedCaps{}, err
	}
	var d capsDoc
	if err := unmarshal(body, &d); err != nil {
		return parsedCaps{}, plugin.Wrap(plugin.ClassParse, "caps document", err)
	}
	pc := parsedCaps{params: map[domain.SearchMode]map[string]bool{}}
	pc.Limit = d.Limits.Max
	pc.Categories = map[int]string{}
	for _, el := range d.Searching.Items {
		pc.addMode(el)
	}
	for _, c := range d.Categories {
		pc.addCategory(c)
	}
	if len(pc.Modes) == 0 {
		return parsedCaps{}, plugin.Errorf(plugin.ClassParse, "caps document declares no search modes")
	}
	return pc, nil
}

func (c *parsedCaps) addMode(el searchEl) {
	mode, ok := modeElems[el.XMLName.Local]
	if !ok || !strings.EqualFold(el.Available, "yes") {
		return
	}
	c.Modes = append(c.Modes, mode)
	set := map[string]bool{}
	for _, p := range strings.Split(el.Params, ",") {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
			c.addIDParam(p)
		}
	}
	c.params[mode] = set
}

func (c *parsedCaps) addIDParam(p string) {
	if !strings.HasSuffix(p, "id") {
		return
	}
	for _, have := range c.IDParams {
		if have == p {
			return
		}
	}
	c.IDParams = append(c.IDParams, p)
}

func (c *parsedCaps) addCategory(cat catEl) {
	c.Categories[cat.ID] = cat.Name
	for _, s := range cat.Sub {
		c.addCategory(s)
	}
}
