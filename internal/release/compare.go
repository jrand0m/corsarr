package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result is the verdict for one artifact: Same, or Detail names the first difference.
type Result struct {
	Name   string
	Same   bool
	Detail string
}

func (r Result) String() string {
	if r.Same {
		return fmt.Sprintf("%-34s identical", r.Name)
	}
	return fmt.Sprintf("%-34s DIFFERS: %s", r.Name, r.Detail)
}

// CompareArchives compares every release file (archives and Linux packages)
// in two folders by name.
func CompareArchives(a, b string) ([]Result, error) {
	names, err := archiveNames(a, b)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(names))
	for _, n := range names {
		out = append(out, compareArchive(n, filepath.Join(a, n), filepath.Join(b, n)))
	}
	return out, nil
}

func archiveNames(dirs ...string) ([]string, error) {
	seen := map[string]bool{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if n := e.Name(); isArchive(n) || isPackage(n) {
				seen[n] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

func compareArchive(name, a, b string) Result {
	ab, aerr := os.ReadFile(a)
	bb, berr := os.ReadFile(b)
	switch {
	case aerr != nil:
		return Result{Name: name, Detail: "missing on the left: " + aerr.Error()}
	case berr != nil:
		return Result{Name: name, Detail: "missing on the right: " + berr.Error()}
	case bytes.Equal(ab, bb):
		return Result{Name: name, Same: true}
	}
	if isPackage(name) {
		return Result{Name: name, Detail: fmt.Sprintf("package bytes differ (%d vs %d bytes); diffoscope shows where", len(ab), len(bb))}
	}
	return Result{Name: name, Detail: firstEntryDiff(name, ab, bb)}
}

func isArchive(n string) bool { return strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".zip") }

func isPackage(n string) bool {
	return strings.HasSuffix(n, ".deb") || strings.HasSuffix(n, ".rpm") || strings.HasSuffix(n, ".apk")
}

// member is one archive entry reduced to what a difference can be in.
type member struct{ name, meta, sum string }

func firstEntryDiff(name string, a, b []byte) string {
	am, aerr := members(name, a)
	bm, berr := members(name, b)
	if err := errors.Join(aerr, berr); err != nil {
		return "unreadable: " + err.Error()
	}
	for i := 0; i < len(am) && i < len(bm); i++ {
		x, y := am[i], bm[i]
		switch {
		case x.name != y.name:
			return fmt.Sprintf("entry %d is %s on the left and %s on the right", i+1, x.name, y.name)
		case x.sum != y.sum:
			return x.name + " (content)"
		case x.meta != y.meta:
			return fmt.Sprintf("%s (header: %s vs %s)", x.name, x.meta, y.meta)
		}
	}
	if len(am) != len(bm) {
		return fmt.Sprintf("%d entries on the left, %d on the right", len(am), len(bm))
	}
	return "same entries, different compression or container bytes"
}

func members(name string, b []byte) ([]member, error) {
	if strings.HasSuffix(name, ".zip") {
		return zipMembers(b)
	}
	return tarMembers(b)
}

func tarMembers(b []byte) ([]member, error) {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	var out []member
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		sum, err := digest(tr)
		if err != nil {
			return out, err
		}
		meta := fmt.Sprintf("mode %o, owner %d:%d %s:%s, mtime %d", h.Mode, h.Uid, h.Gid, h.Uname, h.Gname, h.ModTime.Unix())
		out = append(out, member{name: h.Name, meta: meta, sum: sum})
	}
}

func zipMembers(b []byte) ([]member, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	out := make([]member, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return out, err
		}
		sum, err := digest(rc)
		_ = rc.Close()
		if err != nil {
			return out, err
		}
		meta := fmt.Sprintf("mode %v, mtime %d, method %d", f.Mode(), f.Modified.Unix(), f.Method)
		out = append(out, member{name: f.Name, meta: meta, sum: sum})
	}
	return out, nil
}

func digest(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Index is the part of an OCI image index (or a registry's manifest list) compared here.
type Index struct {
	Digest    string `json:"digest,omitempty"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform *struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant,omitempty"`
		} `json:"platform,omitempty"`
	} `json:"manifests"`
}

// ReadImage loads a multi-platform image description. dir is either an OCI
// image layout (index.json + blobs) or holds image.json, the manifest list a
// registry returned for the published tag (with its digest).
func ReadImage(dir string) (Index, error) {
	if b, err := os.ReadFile(filepath.Join(dir, "image.json")); err == nil {
		var ix Index
		return ix, json.Unmarshal(b, &ix)
	}
	var layout Index
	if err := readJSON(filepath.Join(dir, "index.json"), &layout); err != nil {
		return Index{}, err
	}
	if len(layout.Manifests) != 1 {
		return Index{}, fmt.Errorf("%s: want one image index in the layout, found %d", dir, len(layout.Manifests))
	}
	top := layout.Manifests[0].Digest
	var ix Index
	err := readJSON(blobPath(dir, top), &ix)
	ix.Digest = top
	return ix, err
}

func blobPath(dir, d string) string {
	algo, hexd, _ := strings.Cut(d, ":")
	return filepath.Join(dir, "blobs", algo, hexd)
}

func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// CompareImages compares the index digest and, when it differs, names the
// first platform whose manifest differs.
func CompareImages(a, b Index) Result {
	r := Result{Name: "image " + shortDigest(a.Digest)}
	if a.Digest != "" && a.Digest == b.Digest {
		r.Same = true
		return r
	}
	left, right := platforms(a), platforms(b)
	for _, p := range sortedKeys(left, right) {
		if l, rr := left[p], right[p]; l != rr {
			r.Detail = fmt.Sprintf("%s manifest %s vs %s", p, orNone(l), orNone(rr))
			return r
		}
	}
	r.Detail = fmt.Sprintf("index %s vs %s (same platform manifests: index metadata differs)", orNone(a.Digest), orNone(b.Digest))
	return r
}

func platforms(ix Index) map[string]string {
	out := map[string]string{}
	for _, m := range ix.Manifests {
		if m.Platform == nil || m.Platform.OS == "unknown" {
			continue // attestation manifests are not part of the image
		}
		p := m.Platform.OS + "/" + m.Platform.Architecture
		if m.Platform.Variant != "" {
			p += "/" + m.Platform.Variant
		}
		out[p] = m.Digest
	}
	return out
}

func sortedKeys(ms ...map[string]string) []string {
	seen := map[string]bool{}
	for _, m := range ms {
		for k := range m {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19]
	}
	return d
}
