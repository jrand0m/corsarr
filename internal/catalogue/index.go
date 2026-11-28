// Package catalogue reads the signed plugin index a release publishes,
// keeps the last verified copy on disk, works out what each entry means for
// this installation (available, installed, update, incompatible) and
// downloads and verifies a module before it is handed to the module store.
//
// The index is plugins-index.json with a detached Ed25519 signature in
// plugins-index.json.sig. Nothing from an index is used before its signature
// verifies against a key compiled into this build; after that the index is
// decoded strictly and every entry is checked.
package catalogue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/jrand0m/corsarr/internal/plaintext"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// SchemaVersion is the index format this build reads.
const SchemaVersion = 1

// MaxEntries bounds the number of plugins one index may list.
const MaxEntries = 200

// MaxIndexBytes bounds the size of plugins-index.json.
const MaxIndexBytes = 4 << 20

// Index is plugins-index.json.
type Index struct {
	Schema    int       `json:"schema"`
	Release   string    `json:"release"`
	Generated time.Time `json:"generated"`
	Plugins   []Entry   `json:"plugins"`
}

// Entry is one plugin offered by an index: the identity and permissions of
// its manifest plus where its module is and what it must hash to.
type Entry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Kind        string          `json:"kind"`
	ABIVersion  int             `json:"abiVersion"`
	Description string          `json:"description,omitempty"`
	Author      string          `json:"author,omitempty"`
	License     string          `json:"license,omitempty"`
	Homepage    string          `json:"homepage,omitempty"`
	Permissions abi.Permissions `json:"permissions"`
	MinCorsarr  string          `json:"minCorsarr,omitempty"`
	Size        int64           `json:"size"`
	SHA256      string          `json:"sha256"`
	URL         string          `json:"url"`
	Attestation string          `json:"attestation,omitempty"`
}

// Find returns the entry with id.
func (ix Index) Find(id string) (Entry, bool) {
	for _, e := range ix.Plugins {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// FormatError lists every problem of an index.
type FormatError struct{ Problems schema.Problems }

func (e *FormatError) Error() string { return "invalid plugin index: " + e.Problems.Error() }

// Decode parses verified index bytes strictly (unknown fields are an error)
// and checks every entry. maxModule is the largest module size accepted.
func Decode(data []byte, maxModule int64) (Index, error) {
	var ix Index
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ix); err != nil {
		return ix, &FormatError{schema.Problems{{Path: "index", Msg: "is not a plugin index: " + err.Error()}}}
	}
	if dec.More() {
		return ix, &FormatError{schema.Problems{{Path: "index", Msg: "has data after the index object"}}}
	}
	if ps := Check(ix, maxModule); len(ps) > 0 {
		return ix, &FormatError{ps}
	}
	return ix, nil
}

var (
	shaRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	releaseRe = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]{1,32})?$`)
	semverRe  = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]{1,32})?(\+[0-9A-Za-z.-]{1,32})?$`)
)

// Check returns every problem of a decoded index.
func Check(ix Index, maxModule int64) schema.Problems {
	var ps schema.Problems
	add := func(path, format string, a ...any) {
		ps = append(ps, schema.Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
	}
	if ix.Schema != SchemaVersion {
		add("schema", "is format %d; this corsarr reads format %d and needs an update for this catalogue", ix.Schema, SchemaVersion)
		return ps
	}
	if !releaseRe.MatchString(ix.Release) {
		add("release", "must be a release tag such as v1.2.3")
	}
	if ix.Generated.IsZero() {
		add("generated", "is required")
	}
	if len(ix.Plugins) > MaxEntries {
		add("plugins", "lists %d plugins; at most %d are read", len(ix.Plugins), MaxEntries)
		return ps
	}
	for i, e := range ix.Plugins {
		path := fmt.Sprintf("plugins[%d]", i)
		if i > 0 && ix.Plugins[i-1].ID >= e.ID {
			add(path+".id", "must be unique and sorted by id")
		}
		for _, p := range CheckEntry(e, maxModule) {
			ps = append(ps, schema.Problem{Path: path + "." + p.Path, Msg: p.Msg})
		}
	}
	return ps
}

// CheckEntry returns the problems of one entry. A newer ABI or kind is not a
// problem here: the entry is shown as incompatible instead.
func CheckEntry(e Entry, maxModule int64) schema.Problems {
	ps := wasm.CheckIdentity(e.manifest())
	add := func(path, format string, a ...any) {
		ps = append(ps, schema.Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
	}
	if _, err := plaintext.Check(e.Kind, plaintext.Label); err != nil || !schema.IsIdentifier(e.Kind) {
		add("kind", "must be a plugin kind such as downloader")
	}
	if e.ABIVersion < 1 {
		add("abiVersion", "must be 1 or more")
	}
	for i, h := range e.Permissions.HTTP.Hosts {
		if msg := wasm.CheckHostPattern(h); msg != "" {
			add(fmt.Sprintf("permissions.http.hosts[%d]", i), "%s", msg)
		}
	}
	for i, s := range e.Permissions.Secrets {
		if !schema.IsIdentifier(s) {
			add(fmt.Sprintf("permissions.secrets[%d]", i), "must be a setting key")
		}
	}
	if e.MinCorsarr != "" && !semverRe.MatchString(e.MinCorsarr) {
		add("minCorsarr", "must be a version such as 1.4.0")
	}
	if e.Size <= 0 || (maxModule > 0 && e.Size > maxModule) {
		add("size", "must be between 1 byte and the largest module accepted (%d bytes)", maxModule)
	}
	if !shaRe.MatchString(e.SHA256) {
		add("sha256", "must be 64 lower-case hex characters")
	}
	if msg := urlProblem(e.URL); msg != "" {
		add("url", "%s", msg)
	}
	if e.Attestation != "" && !isHTTPS(e.Attestation) {
		add("attestation", "must be an https:// address")
	}
	return ps
}

func urlProblem(ref string) string {
	u, err := url.Parse(ref)
	switch {
	case ref == "" || err != nil:
		return "must be the module's address"
	case u.IsAbs() && u.Scheme != "https":
		return "must be relative to the index or an https:// address"
	case !u.IsAbs() && (u.Host != "" || u.User != nil):
		return "must be relative to the index or an https:// address"
	}
	return ""
}

func isHTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// manifest is the part of a module manifest an entry repeats.
func (e Entry) manifest() abi.Manifest {
	return abi.Manifest{ID: e.ID, Name: e.Name, Version: e.Version, Kind: e.Kind, ABIVersion: e.ABIVersion,
		Description: e.Description, Author: e.Author, License: e.License, Homepage: e.Homepage, Permissions: e.Permissions}
}

// ResolveURL is where the module of e is, relative to the index address.
func ResolveURL(indexURL string, e Entry) (string, error) {
	base, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("catalogue address: %w", err)
	}
	ref, err := url.Parse(e.URL)
	if err != nil {
		return "", fmt.Errorf("module address: %w", err)
	}
	return base.ResolveReference(ref).String(), nil
}
