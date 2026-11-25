// Package definition is corsarr's declarative tracker definition format and
// the pure engine that evaluates it: request templates per search mode,
// response parsing for JSON APIs and HTML pages, value filters, and the
// mapping of extracted fields onto releases. A definition is data only; every
// evaluation is bounded in size and steps so a broken or hostile file cannot
// exhaust memory or CPU. Network and session handling live in the plugin that
// runs a definition.
package definition

import (
	"time"
)

// SchemaVersion is the definition schema version this build reads.
const SchemaVersion = 1

// Limits that bound every evaluation.
const (
	MaxFileSize     = 256 << 10 // a definition file
	MaxResponseSize = 4 << 20   // one response body parsed by a definition
	MaxRows         = 500       // rows read from one response
	MaxTemplate     = 4 << 10   // one template's source
	MaxExpanded     = 16 << 10  // one template's output
	MaxFilters      = 12        // filters on one value
	MaxValue        = 8 << 10   // one extracted value
)

// Privacy says who can use a tracker.
type Privacy string

const (
	Public      Privacy = "public"
	SemiPrivate Privacy = "semi-private"
	Private     Privacy = "private"
)

// LoginMethod is how a definition obtains a session.
type LoginMethod string

const (
	LoginNone   LoginMethod = "none"
	LoginCookie LoginMethod = "cookie" // the owner pastes a session cookie
	LoginForm   LoginMethod = "form"   // credentials are posted once for a session cookie
)

// ResponseType is how a results page is read.
type ResponseType string

const (
	ResponseJSON ResponseType = "json"
	ResponseHTML ResponseType = "html"
)

// Definition is one tracker described as data.
type Definition struct {
	Schema      int      `yaml:"schema" json:"schema"`
	ID          string   `yaml:"id" json:"id"`
	Version     string   `yaml:"version" json:"version"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Language    string   `yaml:"language" json:"language"`
	Privacy     Privacy  `yaml:"privacy" json:"privacy"`
	Links       []string `yaml:"links" json:"links"`
	// Encoding is the site's text encoding (for example windows-1251) used to
	// read its pages and to write queries and forms; empty means the response
	// Content-Type or the page's own declaration, else UTF-8.
	Encoding  string    `yaml:"encoding" json:"encoding,omitempty"`
	Settings  []Setting `yaml:"settings" json:"settings"`
	Login     Login     `yaml:"login" json:"login"`
	RateLimit RateLimit `yaml:"rateLimit" json:"rateLimit"`
	Headers   Strings   `yaml:"headers" json:"headers"`
	Caps      Caps      `yaml:"caps" json:"caps"`
	Search    Search    `yaml:"search" json:"search"`
	Download  *Download `yaml:"download" json:"download"`
	// MagnetTrackers are announce URLs added to magnets built from an info hash.
	MagnetTrackers []string `yaml:"magnetTrackers" json:"magnetTrackers"`
	Test           SelfTest `yaml:"test" json:"test"`
}

// Strings is a string map; templates may appear in its values.
type Strings map[string]string

// Setting is one owner-supplied value a definition refers to as
// {{settings.<key>}}.
type Setting struct {
	Key     string   `yaml:"key" json:"key"`
	Type    string   `yaml:"type" json:"type"` // text | password | checkbox | select
	Label   string   `yaml:"label" json:"label"`
	Help    string   `yaml:"help" json:"help"`
	Default any      `yaml:"default" json:"default"`
	Options []string `yaml:"options" json:"options"`
}

// Login describes how to get and keep a session.
type Login struct {
	Method LoginMethod `yaml:"method" json:"method"`
	// Form login: where the credentials go and which fields carry them.
	Path   string  `yaml:"path" json:"path"`
	Fields Strings `yaml:"fields" json:"fields"`
	// Success must hold on the page after logging in (form login).
	Success Check `yaml:"success" json:"success"`
	// Failure, when it holds, carries the site's error message.
	Failure Check `yaml:"failure" json:"failure"`
	// Expired holds on a results page served to a logged-out visitor.
	Expired Check `yaml:"expired" json:"expired"`
}

// Check is a condition on an HTML page or a redirect.
type Check struct {
	Selector string `yaml:"selector" json:"selector"` // an element matching it exists
	Contains string `yaml:"contains" json:"contains"` // the body contains this text
	Redirect string `yaml:"redirect" json:"redirect"` // the response redirects to a path containing this
}

// Empty reports whether the check has no condition.
func (c Check) Empty() bool { return c.Selector == "" && c.Contains == "" && c.Redirect == "" }

// RateLimit is the site's minimum spacing between requests.
type RateLimit struct {
	Interval Duration `yaml:"interval" json:"interval"`
}

// Duration is a time.Duration written as "2s" in files.
type Duration time.Duration

// Caps are the search modes, their parameters and the site's categories.
type Caps struct {
	// Modes maps search, tv, movie, music, book to the parameters each accepts
	// (q, season, ep, imdbid, tmdbid, tvdbid, year).
	Modes      map[string][]string `yaml:"modes" json:"modes"`
	Categories []Category          `yaml:"categories" json:"categories"`
}

// Category maps a site category to a standard Newznab category.
type Category struct {
	Site     string `yaml:"site" json:"site"`
	Standard int    `yaml:"standard" json:"standard"`
	Name     string `yaml:"name" json:"name"`
}

// Search says how to ask the site and how to read its answer.
type Search struct {
	// Requests are tried in order; the first whose modes and condition match runs.
	Requests []Request `yaml:"requests" json:"requests"`
	Response Response  `yaml:"response" json:"response"`
}

// Request is one HTTP request template.
type Request struct {
	Modes []string `yaml:"modes" json:"modes"` // empty = every mode
	// When names a variable that must be non-empty for this request to run
	// (for example imdbid, so an id search falls back to a text search).
	When    string  `yaml:"when" json:"when"`
	Method  string  `yaml:"method" json:"method"`
	Path    string  `yaml:"path" json:"path"`
	Query   Strings `yaml:"query" json:"query"`
	Body    string  `yaml:"body" json:"body"` // template; JSON or form encoded
	Type    string  `yaml:"contentType" json:"contentType"`
	Headers Strings `yaml:"headers" json:"headers"`
}

// Response reads rows and their fields.
type Response struct {
	Type ResponseType `yaml:"type" json:"type"`
	// Rows is a JSON path to an array, or a CSS selector matching one element per row.
	Rows string `yaml:"rows" json:"rows"`
	// Skip drops rows matching any condition (header rows, "no results" placeholders).
	Skip []Condition `yaml:"skip" json:"skip"`
	// NoResults holds on an HTML page that legitimately lists nothing; an HTML
	// page with zero rows where it does not hold is a parse failure.
	NoResults Check                `yaml:"noResults" json:"noResults"`
	Fields    map[string]FieldRule `yaml:"fields" json:"fields"`
}

// Condition tests one extracted field of a row.
type Condition struct {
	Field  string `yaml:"field" json:"field"`
	Equals string `yaml:"equals" json:"equals"`
	Empty  bool   `yaml:"empty" json:"empty"`
}

// FieldRule extracts one value from a row.
type FieldRule struct {
	Path     string   `yaml:"path" json:"path"`         // JSON: path inside the row
	Selector string   `yaml:"selector" json:"selector"` // HTML: element inside the row ("" = the row)
	Attr     string   `yaml:"attr" json:"attr"`         // HTML: attribute instead of text
	Value    string   `yaml:"value" json:"value"`       // template over the row's other fields
	Default  string   `yaml:"default" json:"default"`
	Formats  []string `yaml:"formats" json:"formats"` // date layouts tried first (Go reference time)
	Filters  []Filter `yaml:"filters" json:"filters"`
}

// Download is the follow-up step that finds the payload link on a details page.
type Download struct {
	Selector string `yaml:"selector" json:"selector"`
	Attr     string `yaml:"attr" json:"attr"`
}

// SelfTest is the query a test runs and the rows it must find.
type SelfTest struct {
	Query   string `yaml:"query" json:"query"`
	MinRows int    `yaml:"minRows" json:"minRows"`
}

// Field names a definition may extract.
const (
	FieldTitle    = "title"
	FieldDetails  = "details"
	FieldDownload = "download"
	FieldMagnet   = "magnet"
	FieldInfoHash = "infohash"
	FieldSize     = "size"
	FieldSeeders  = "seeders"
	FieldPeers    = "peers"
	FieldLeechers = "leechers"
	FieldGrabs    = "grabs"
	FieldDate     = "date"
	FieldCategory = "category"
	FieldIMDB     = "imdb"
	FieldDLFactor = "downloadFactor"
	FieldULFactor = "uploadFactor"
	FieldFiles    = "files"
)

// knownFields also allows helper fields: any name starting with "_" is
// extracted for use in other fields' value templates only.
var knownFields = map[string]bool{
	FieldTitle: true, FieldDetails: true, FieldDownload: true, FieldMagnet: true, FieldInfoHash: true,
	FieldSize: true, FieldSeeders: true, FieldPeers: true, FieldLeechers: true, FieldGrabs: true, FieldDate: true,
	FieldCategory: true, FieldIMDB: true, FieldDLFactor: true, FieldULFactor: true, FieldFiles: true,
}

// Modes a definition may declare.
var knownModes = map[string]bool{"search": true, "tv": true, "movie": true, "music": true, "book": true}

// Parameters a mode may accept.
var knownParams = map[string]bool{"q": true, "season": true, "ep": true, "imdbid": true, "tmdbid": true, "tvdbid": true, "year": true, "artist": true, "album": true}
