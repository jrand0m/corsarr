package plugin

import (
	"fmt"
	"strings"
)

// PathRule maps one folder as a remote program reports it (a download client
// on another host or in a container with other mounts) onto the same folder
// as corsarr sees it. Both sides are kept with forward slashes.
type PathRule struct {
	Remote string
	Local  string
}

// PathMap is a set of path rules; the longest matching prefix wins.
type PathMap []PathRule

// WindowsStyle reports whether p is a Windows path: a drive letter or a UNC
// share.
func WindowsStyle(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && isLetter(p[0])
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// canon trims blanks and trailing separators and turns Windows separators
// into forward slashes, so prefixes compare one way.
func canon(p string) string {
	p = strings.TrimSpace(p)
	if WindowsStyle(p) {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func absolute(p string) bool { return strings.HasPrefix(p, "/") || WindowsStyle(p) }

// ParsePathRule reads one "remote=local" entry; both sides must be absolute.
func ParsePathRule(s string) (PathRule, error) {
	parts := strings.Split(s, "=")
	if len(parts) != 2 {
		return PathRule{}, fmt.Errorf("path mapping %q must be one remote=local pair", s)
	}
	r := PathRule{Remote: canon(parts[0]), Local: canon(parts[1])}
	if !absolute(r.Remote) || !absolute(r.Local) {
		return PathRule{}, fmt.Errorf("path mapping %q needs an absolute path on both sides", s)
	}
	return r, nil
}

// CheckPathRule validates one entry (for config validation).
func CheckPathRule(s string) error {
	_, err := ParsePathRule(s)
	return err
}

// ParsePathMap reads "remote=local" entries; a remote prefix may appear once.
func ParsePathMap(entries []string) (PathMap, error) {
	var out PathMap
	for _, e := range entries {
		r, err := ParsePathRule(e)
		if err != nil {
			return nil, err
		}
		for _, have := range out {
			if samePath(have.Remote, r.Remote) {
				return nil, fmt.Errorf("path mapping for %s is given twice", r.Remote)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func samePath(a, b string) bool {
	if WindowsStyle(a) || WindowsStyle(b) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// under returns the part of c below prefix (with its leading slash) and
// whether c is prefix or lies below it on a path boundary.
func under(c, prefix string) (string, bool) {
	switch {
	case prefix == "/":
		return strings.TrimPrefix(c, "/"), strings.HasPrefix(c, "/")
	case samePath(c, prefix):
		return "", true
	case len(c) > len(prefix) && c[len(prefix)] == '/' && samePath(c[:len(prefix)], prefix):
		return c[len(prefix)+1:], true
	}
	return "", false
}

// render joins base and rest in the separator style of base.
func render(base, rest string) string {
	out := base
	if rest != "" {
		out = strings.TrimRight(base, "/") + "/" + rest
	}
	if WindowsStyle(out) {
		out = strings.ReplaceAll(out, "/", `\`)
	}
	return out
}

// translate rewrites p from one side of the rules to the other with the
// longest matching prefix; p comes back unchanged when no rule covers it.
func (m PathMap) translate(p string, from, to func(PathRule) string) (string, bool) {
	c := canon(p)
	best, rest := -1, ""
	for i, r := range m {
		if tail, ok := under(c, from(r)); ok && (best < 0 || len(from(r)) > len(from(m[best]))) {
			best, rest = i, tail
		}
	}
	if best < 0 || p == "" {
		return p, false
	}
	return render(to(m[best]), rest), true
}

func remote(r PathRule) string { return r.Remote }
func local(r PathRule) string  { return r.Local }

// ToLocal turns a path the remote program reported into corsarr's view.
func (m PathMap) ToLocal(p string) (string, bool) { return m.translate(p, remote, local) }

// ToRemote turns a local path into the remote program's view.
func (m PathMap) ToRemote(p string) (string, bool) { return m.translate(p, local, remote) }
