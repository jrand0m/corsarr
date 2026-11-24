package definition

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Filter is one value transformation on an extracted field. In files it is
// written as a bare name (trim) or a one-key map (regex: "(\d+)").
type Filter struct {
	Name string
	Args []string
	re   *regexp.Regexp
}

// filterArity is the number of arguments each value filter takes.
var filterArity = map[string]int{
	"trim": 0, "lower": 0, "upper": 0, "urldecode": 0, "urlencode": 0, "digits": 0,
	"regex": 1, "prepend": 1, "append": 1, "remove": 1,
	"replace": 2, "split": 2,
}

// UnmarshalYAML accepts "name", {name: arg} or {name: [args]}.
func (f *Filter) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		f.Name = n.Value
	case yaml.MappingNode:
		if len(n.Content) != 2 {
			return fmt.Errorf("line %d: a filter is one name with its arguments", n.Line)
		}
		f.Name = n.Content[0].Value
		if err := filterArgs(n.Content[1], &f.Args); err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
	default:
		return fmt.Errorf("line %d: a filter is a name or a one-key map", n.Line)
	}
	return f.compile()
}

func filterArgs(n *yaml.Node, out *[]string) error {
	if n.Kind == yaml.ScalarNode {
		*out = []string{n.Value}
		return nil
	}
	return n.Decode(out)
}

func (f *Filter) compile() error {
	if f.Name == "regex" && len(f.Args) == 1 {
		re, err := regexp.Compile(f.Args[0])
		if err != nil {
			return fmt.Errorf("regex %q: %w", f.Args[0], err)
		}
		f.re = re
	}
	return nil
}

func (f Filter) check() error {
	n, ok := filterArity[f.Name]
	if !ok {
		return fmt.Errorf("unknown filter %q", f.Name)
	}
	if len(f.Args) != n {
		return fmt.Errorf("filter %s takes %d argument(s)", f.Name, n)
	}
	if f.Name == "split" {
		if _, err := strconv.Atoi(f.Args[1]); err != nil {
			return fmt.Errorf("split index %q is not a number", f.Args[1])
		}
	}
	if f.Name == "regex" && f.re == nil {
		return f.compile()
	}
	return nil
}

var nonDigits = regexp.MustCompile(`[^0-9]`)

// Apply runs the filter. Regex keeps the first capture group (or the whole
// match), and an unmatched regex yields "". Go regexps run in linear time.
func (f Filter) Apply(v string) string {
	if fn, ok := valueFilters[f.Name]; ok {
		return fn(f, v)
	}
	return v
}

var valueFilters = map[string]func(f Filter, v string) string{
	"trim":      func(_ Filter, v string) string { return strings.Join(strings.Fields(v), " ") },
	"lower":     func(_ Filter, v string) string { return strings.ToLower(v) },
	"upper":     func(_ Filter, v string) string { return strings.ToUpper(v) },
	"urldecode": urlDecode,
	"urlencode": func(_ Filter, v string) string { return url.QueryEscape(v) },
	"digits":    func(_ Filter, v string) string { return nonDigits.ReplaceAllString(v, "") },
	"regex":     Filter.regex,
	"prepend":   func(f Filter, v string) string { return f.Args[0] + v },
	"append":    func(f Filter, v string) string { return v + f.Args[0] },
	"remove":    func(f Filter, v string) string { return strings.ReplaceAll(v, f.Args[0], "") },
	"replace":   func(f Filter, v string) string { return strings.ReplaceAll(v, f.Args[0], f.Args[1]) },
	"split":     split,
}

func urlDecode(_ Filter, v string) string {
	if d, err := url.QueryUnescape(v); err == nil {
		return d
	}
	return v
}

// split keeps one part; a negative index counts from the end.
func split(f Filter, v string) string {
	parts := strings.Split(v, f.Args[0])
	i, _ := strconv.Atoi(f.Args[1])
	if i < 0 {
		i += len(parts)
	}
	if i < 0 || i >= len(parts) {
		return ""
	}
	return parts[i]
}

func (f Filter) regex(v string) string {
	re := f.re
	if re == nil {
		var err error
		if re, err = regexp.Compile(f.Args[0]); err != nil {
			return ""
		}
	}
	m := re.FindStringSubmatch(v)
	switch {
	case m == nil:
		return ""
	case len(m) > 1:
		return m[1]
	}
	return m[0]
}

// applyAll runs a filter chain and caps the value size.
func applyAll(fs []Filter, v string) string {
	for _, f := range fs {
		v = f.Apply(v)
	}
	if len(v) > MaxValue {
		v = v[:MaxValue]
	}
	return v
}
