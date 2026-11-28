package client

import (
	"sort"
	"strings"
)

// PathRule maps one folder as rTorrent reports it onto the same folder as
// the caller sees it. Both sides are kept with forward slashes and without a
// trailing slash.
type PathRule struct {
	Remote string
	Local  string
}

// PathMap is a set of path rules; the longest matching prefix wins.
type PathMap []PathRule

// windowsStyle reports whether p starts with a drive letter or a UNC share.
func windowsStyle(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func canonPath(p string) string {
	p = strings.TrimSpace(p)
	if windowsStyle(p) {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func absPath(p string) bool { return strings.HasPrefix(p, "/") || windowsStyle(p) }

// ParsePathMap builds rules from remote → local pairs; both sides must be
// absolute and a remote folder may appear once.
func ParsePathMap(pairs map[string]string) (PathMap, error) {
	out := make(PathMap, 0, len(pairs))
	seen := map[string]bool{}
	for remote, local := range pairs {
		r := PathRule{Remote: canonPath(remote), Local: canonPath(local)}
		if !absPath(r.Remote) || !absPath(r.Local) {
			return nil, errorf(KindConfig, "path mapping %s → %s needs an absolute path on both sides", remote, local)
		}
		if seen[r.Remote] {
			return nil, errorf(KindConfig, "path mapping for %s is given twice", r.Remote)
		}
		seen[r.Remote] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Remote < out[j].Remote })
	return out, nil
}

// below returns the part of c under prefix and whether c is prefix or lies
// below it on a folder boundary.
func below(c, prefix string) (string, bool) {
	switch {
	case prefix == "/":
		return strings.TrimPrefix(c, "/"), strings.HasPrefix(c, "/")
	case c == prefix:
		return "", true
	case strings.HasPrefix(c, prefix+"/"):
		return c[len(prefix)+1:], true
	}
	return "", false
}

func joinStyled(base, rest string) string {
	out := base
	if rest != "" {
		out = strings.TrimRight(base, "/") + "/" + rest
	}
	if windowsStyle(out) {
		out = strings.ReplaceAll(out, "/", `\`)
	}
	return out
}

func (m PathMap) translate(p string, from, to func(PathRule) string) string {
	if p == "" {
		return p
	}
	c := canonPath(p)
	best, rest := -1, ""
	for i, r := range m {
		if tail, ok := below(c, from(r)); ok && (best < 0 || len(from(r)) > len(from(m[best]))) {
			best, rest = i, tail
		}
	}
	if best < 0 {
		return p
	}
	return joinStyled(to(m[best]), rest)
}

func remoteSide(r PathRule) string { return r.Remote }
func localSide(r PathRule) string  { return r.Local }

// ToLocal turns a path rTorrent reported into the caller's view; a path no
// rule covers comes back unchanged.
func (m PathMap) ToLocal(p string) string { return m.translate(p, remoteSide, localSide) }

// ToRemote turns a caller path into rTorrent's view.
func (m PathMap) ToRemote(p string) string { return m.translate(p, localSide, remoteSide) }
