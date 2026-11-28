package client

import (
	"fmt"
	"sort"
	"strings"
)

// PathRule maps one folder as Transmission reports it onto the same folder as
// the host sees it. Both sides are kept with forward slashes.
type PathRule struct {
	Remote string
	Local  string
}

// PathMap is a set of path rules; the longest matching prefix wins.
type PathMap []PathRule

// windowsStyle reports whether p is a Windows path: a drive letter or a UNC share.
func windowsStyle(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

// canon trims blanks and trailing separators and turns Windows separators
// into forward slashes, so prefixes compare one way.
func canon(p string) string {
	p = strings.TrimSpace(p)
	if windowsStyle(p) {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func absolute(p string) bool { return strings.HasPrefix(p, "/") || windowsStyle(p) }

// ParsePathMap reads remote→local entries; both sides must be absolute.
func ParsePathMap(entries map[string]string) (PathMap, error) {
	out := make(PathMap, 0, len(entries))
	seen := map[string]bool{}
	for remote, local := range entries {
		r := PathRule{Remote: canon(remote), Local: canon(local)}
		if !absolute(r.Remote) || !absolute(r.Local) {
			return nil, fmt.Errorf("path mapping %s=%s needs an absolute path on both sides", remote, local)
		}
		key := strings.ToLower(r.Remote)
		if seen[key] {
			return nil, fmt.Errorf("path mapping for %s is given twice", r.Remote)
		}
		seen[key] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Remote < out[j].Remote })
	return out, nil
}

func samePath(a, b string) bool {
	if windowsStyle(a) || windowsStyle(b) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// under returns the part of c below prefix and whether c is prefix or lies
// below it on a path boundary.
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
	if windowsStyle(out) {
		out = strings.ReplaceAll(out, "/", `\`)
	}
	return out
}

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

func ruleRemote(r PathRule) string { return r.Remote }
func ruleLocal(r PathRule) string  { return r.Local }

// ToLocal turns a path Transmission reported into the host's view.
func (m PathMap) ToLocal(p string) (string, bool) { return m.translate(p, ruleRemote, ruleLocal) }

// ToRemote turns a host path into Transmission's view.
func (m PathMap) ToRemote(p string) (string, bool) { return m.translate(p, ruleLocal, ruleRemote) }

// Local maps a path Transmission reported into the host's view.
func (c *Client) Local(p string) string {
	out, _ := c.paths.ToLocal(p)
	return out
}

// Remote maps a host path into Transmission's view.
func (c *Client) Remote(p string) string {
	out, _ := c.paths.ToRemote(p)
	return out
}

// joinPath appends a name to a directory in the directory's separator style.
func joinPath(dir, name string) string {
	switch {
	case dir == "":
		return name
	case windowsStyle(dir):
		return strings.TrimRight(dir, `\/`) + `\` + name
	}
	return strings.TrimRight(dir, "/") + "/" + name
}
