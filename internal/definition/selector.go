package definition

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Selector is a small CSS selector subset: type, #id, .class, attribute tests
// ([a], [a=v], [a^=v], [a$=v], [a*=v]), :first-child, :last-child,
// :nth-child(n), the descendant (space) and child (>) combinators, and
// comma-separated alternatives.
type Selector struct{ alts []chain }

// chain is compounds joined by combinators, left to right.
type chain struct {
	steps []compound
	child []bool // child[i]: steps[i+1] must be a direct child of steps[i]
}

type compound struct {
	tag     string
	id      string
	classes []string
	attrs   []attrTest
	nth     int // 1-based position among element siblings; -1 = last; 0 = any
}

type attrTest struct{ name, op, val string }

// ParseSelector parses s once at load.
func ParseSelector(s string) (*Selector, error) {
	sel := &Selector{}
	for _, alt := range strings.Split(s, ",") {
		c, err := parseChain(strings.TrimSpace(alt))
		if err != nil {
			return nil, fmt.Errorf("selector %q: %w", s, err)
		}
		sel.alts = append(sel.alts, c)
	}
	return sel, nil
}

func parseChain(s string) (chain, error) {
	if s == "" {
		return chain{}, fmt.Errorf("empty selector")
	}
	s = strings.ReplaceAll(s, ">", " > ")
	var c chain
	pendingChild := false
	for _, tok := range strings.Fields(s) {
		if tok == ">" {
			if len(c.steps) == 0 || pendingChild {
				return chain{}, fmt.Errorf("misplaced >")
			}
			pendingChild = true
			continue
		}
		cp, err := parseCompound(tok)
		if err != nil {
			return chain{}, err
		}
		if len(c.steps) > 0 {
			c.child = append(c.child, pendingChild)
		}
		c.steps, pendingChild = append(c.steps, cp), false
	}
	if pendingChild {
		return chain{}, fmt.Errorf("selector ends with >")
	}
	return c, nil
}

func parseCompound(tok string) (compound, error) {
	var c compound
	i := 0
	for i < len(tok) && isIdent(tok[i]) {
		i++
	}
	c.tag = strings.ToLower(tok[:i])
	if c.tag == "*" {
		c.tag = ""
	}
	for i < len(tok) {
		n, err := c.parseSimple(tok[i:])
		if err != nil {
			return compound{}, err
		}
		i += n
	}
	return c, nil
}

func isIdent(b byte) bool {
	return b == '-' || b == '_' || b == '*' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func identLen(s string) int {
	n := 0
	for n < len(s) && isIdent(s[n]) && s[n] != '*' {
		n++
	}
	return n
}

// parseSimple reads one #id, .class, [attr] or :pseudo and returns its length.
func (c *compound) parseSimple(s string) (int, error) {
	switch s[0] {
	case '#', '.':
		n := identLen(s[1:])
		if n == 0 {
			return 0, fmt.Errorf("empty name after %c", s[0])
		}
		if s[0] == '#' {
			c.id = s[1 : 1+n]
		} else {
			c.classes = append(c.classes, s[1:1+n])
		}
		return 1 + n, nil
	case '[':
		return c.parseAttr(s)
	case ':':
		return c.parsePseudo(s)
	}
	return 0, fmt.Errorf("unexpected %q", s[0])
}

func (c *compound) parseAttr(s string) (int, error) {
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return 0, fmt.Errorf("unclosed [")
	}
	body := s[1:end]
	for _, op := range []string{"^=", "$=", "*=", "="} {
		if name, val, ok := strings.Cut(body, op); ok {
			c.attrs = append(c.attrs, attrTest{name: strings.ToLower(strings.TrimSpace(name)), op: op, val: strings.Trim(strings.TrimSpace(val), `"'`)})
			return end + 1, nil
		}
	}
	if identLen(body) != len(body) || body == "" {
		return 0, fmt.Errorf("bad attribute test [%s]", body)
	}
	c.attrs = append(c.attrs, attrTest{name: strings.ToLower(body)})
	return end + 1, nil
}

func (c *compound) parsePseudo(s string) (int, error) {
	switch {
	case strings.HasPrefix(s, ":first-child"):
		c.nth = 1
		return len(":first-child"), nil
	case strings.HasPrefix(s, ":last-child"):
		c.nth = -1
		return len(":last-child"), nil
	case strings.HasPrefix(s, ":nth-child("):
		end := strings.IndexByte(s, ')')
		if end < 0 {
			return 0, fmt.Errorf("unclosed :nth-child(")
		}
		n, err := strconv.Atoi(s[len(":nth-child("):end])
		if err != nil || n < 1 {
			return 0, fmt.Errorf(":nth-child takes a positive number")
		}
		c.nth = n
		return end + 1, nil
	}
	return 0, fmt.Errorf("unsupported pseudo-class in %q", s)
}

// matches tests one element against a compound.
func (c compound) matches(n *html.Node) bool {
	return c.named(n) && c.classed(n) && c.attributed(n) && (c.nth == 0 || position(n, c.nth < 0) == abs(c.nth))
}

func (c compound) named(n *html.Node) bool {
	return n.Type == html.ElementNode && (c.tag == "" || n.Data == c.tag) && (c.id == "" || attr(n, "id") == c.id)
}

func (c compound) classed(n *html.Node) bool {
	classes := strings.Fields(attr(n, "class"))
	for _, want := range c.classes {
		if !contains(classes, want) {
			return false
		}
	}
	return true
}

func (c compound) attributed(n *html.Node) bool {
	for _, a := range c.attrs {
		if !a.matches(n) {
			return false
		}
	}
	return true
}

func (a attrTest) matches(n *html.Node) bool {
	v, ok := attrOK(n, a.name)
	switch a.op {
	case "":
		return ok
	case "=":
		return ok && v == a.val
	case "^=":
		return ok && strings.HasPrefix(v, a.val)
	case "$=":
		return ok && strings.HasSuffix(v, a.val)
	}
	return ok && strings.Contains(v, a.val)
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// position is the 1-based index among element siblings (from the end when fromEnd).
func position(n *html.Node, fromEnd bool) int {
	i := 1
	next := func(s *html.Node) *html.Node { return s.PrevSibling }
	if fromEnd {
		next = func(s *html.Node) *html.Node { return s.NextSibling }
	}
	for s := next(n); s != nil; s = next(s) {
		if s.Type == html.ElementNode {
			i++
		}
	}
	return i
}

func attrOK(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func attr(n *html.Node, name string) string { v, _ := attrOK(n, name); return v }

// matchChain tests n as the last step of c, walking ancestors right to left.
func (c chain) matchChain(n *html.Node, scope *html.Node) bool {
	return c.matchFrom(len(c.steps)-1, n, scope)
}

func (c chain) matchFrom(i int, n *html.Node, scope *html.Node) bool {
	if !c.steps[i].matches(n) {
		return false
	}
	if i == 0 {
		return true
	}
	for p := n.Parent; p != nil && p != scope; p = p.Parent {
		if c.matchFrom(i-1, p, scope) {
			return true
		}
		if c.child[i-1] {
			return false
		}
	}
	return false
}

// All returns the elements under root (excluding root) matching the selector, in document order.
func (s *Selector) All(root *html.Node) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) {
		for _, c := range s.alts {
			if c.matchChain(n, root) {
				out = append(out, n)
				return
			}
		}
	})
	return out
}

// First returns the first match under root, or nil.
func (s *Selector) First(root *html.Node) *html.Node {
	if m := s.All(root); len(m) > 0 {
		return m[0]
	}
	return nil
}

func walk(root *html.Node, fn func(*html.Node)) {
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			fn(c)
		}
		walk(c, fn)
	}
}

// Text returns the element's text content with whitespace collapsed.
func Text(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil && b.Len() < MaxValue; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
