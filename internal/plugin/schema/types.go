package schema

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jrand0m/corsarr/internal/plaintext"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// typeRule lists the properties a type accepts and checks them.
type typeRule struct {
	props map[string]bool
	check func(c *collector, path string, f Field)
}

func props(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

var typeRules = map[string]typeRule{
	abi.TypeString:     {props("minLength", "maxLength", "pattern"), checkString},
	abi.TypeSecret:     {props("minLength", "maxLength"), checkString},
	abi.TypeBool:       {props(), func(*collector, string, Field) {}},
	abi.TypeInt:        {props("min", "max", "step"), checkNumeric},
	abi.TypeNumber:     {props("min", "max", "step", "decimals"), checkNumeric},
	abi.TypeEnum:       {props("options"), checkEnum},
	abi.TypeDuration:   {props("minDuration", "maxDuration"), checkDuration},
	abi.TypeListString: {props("minLength", "maxLength", "pattern", "maxItems"), checkList},
	abi.TypeListInt:    {props("min", "max", "step", "maxItems"), checkList},
	abi.TypeMap:        {props("maxEntries", "keyRule", "valueRule"), checkMap},
}

// presentProps names the type-specific properties a field sets.
func presentProps(f Field) []string {
	var out []string
	set := map[string]bool{
		"minLength": f.MinLength != nil, "maxLength": f.MaxLength != nil, "pattern": f.Pattern != "",
		"min": f.Min != nil, "max": f.Max != nil, "step": f.Step != nil, "decimals": f.Decimals != nil,
		"options": f.Options != nil, "minDuration": f.MinDuration != "", "maxDuration": f.MaxDuration != "",
		"maxItems": f.MaxItems != 0, "maxEntries": f.MaxEntries != 0, "keyRule": f.KeyRule != nil, "valueRule": f.ValueRule != nil,
	}
	for _, name := range []string{"minLength", "maxLength", "pattern", "min", "max", "step", "decimals", "options",
		"minDuration", "maxDuration", "maxItems", "maxEntries", "keyRule", "valueRule"} {
		if set[name] {
			out = append(out, name)
		}
	}
	return out
}

func checkString(c *collector, p string, f Field) {
	checkTextRule(c, p, abi.TextRule{MinLength: f.MinLength, MaxLength: f.MaxLength, Pattern: f.Pattern})
}

func checkTextRule(c *collector, p string, r abi.TextRule) {
	lo, hi := intOr(r.MinLength, 0), intOr(r.MaxLength, MaxText)
	if lo < 0 || hi < 1 || hi > MaxText || lo > hi {
		c.add(p+".maxLength", "length bounds must satisfy 0 <= minLength <= maxLength <= %d", MaxText)
	}
	if r.Pattern != "" && patterns[r.Pattern] == nil {
		c.add(p+".pattern", "unknown pattern %q (hostname, url-http, path-absolute or identifier)", r.Pattern)
	}
}

func checkNumeric(c *collector, p string, f Field) {
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		c.add(p+".min", "must not be greater than max")
	}
	checkPrecision(c, p, f)
	if f.Type == abi.TypeInt || f.Type == abi.TypeListInt {
		checkWhole(c, p, f)
	}
}

func checkPrecision(c *collector, p string, f Field) {
	if f.Step != nil && *f.Step <= 0 {
		c.add(p+".step", "must be greater than 0")
	}
	if d := intOr(f.Decimals, 0); d < 0 || d > 3 {
		c.add(p+".decimals", "must be between 0 and 3")
	}
}

func checkWhole(c *collector, p string, f Field) {
	bounds := map[string]*float64{"min": f.Min, "max": f.Max, "step": f.Step}
	for _, name := range []string{"min", "max", "step"} {
		if v := bounds[name]; v != nil && *v != math.Trunc(*v) {
			c.add(p+"."+name, "must be a whole number")
		}
	}
}

func checkEnum(c *collector, p string, f Field) {
	if len(f.Options) == 0 || len(f.Options) > MaxOptions {
		c.add(p+".options", "needs between 1 and %d options", MaxOptions)
	}
	seen := map[string]bool{}
	for i, o := range f.Options {
		op := fmt.Sprintf("%s.options[%d]", p, i)
		if !IsIdentifier(o.Value) {
			c.add(op+".value", "must start with a lower-case letter followed by letters or digits (at most 48)")
		}
		if seen[o.Value] {
			c.add(op+".value", "duplicate option %q", o.Value)
		}
		seen[o.Value] = true
		c.text(op+".label", o.Label, plaintext.Label)
	}
}

func checkDuration(c *collector, p string, f Field) {
	lo, okLo := parseDur(f.MinDuration)
	hi, okHi := parseDur(f.MaxDuration)
	if !okLo {
		c.add(p+".minDuration", "must be a duration like 30s or 5m")
	}
	if !okHi {
		c.add(p+".maxDuration", "must be a duration like 30s or 5m")
	}
	if okLo && okHi && f.MaxDuration != "" && lo > hi {
		c.add(p+".minDuration", "must not be greater than maxDuration")
	}
}

func parseDur(s string) (time.Duration, bool) {
	if s == "" {
		return 0, true
	}
	d, err := time.ParseDuration(s)
	return d, err == nil && d >= 0
}

func checkList(c *collector, p string, f Field) {
	if f.MaxItems < 0 || f.MaxItems > MaxItems {
		c.add(p+".maxItems", "must be between 1 and %d", MaxItems)
	}
	if f.Type == abi.TypeListString {
		checkString(c, p, f)
		return
	}
	checkNumeric(c, p, f)
}

func checkMap(c *collector, p string, f Field) {
	if f.MaxEntries < 0 || f.MaxEntries > MaxItems {
		c.add(p+".maxEntries", "must be between 1 and %d", MaxItems)
	}
	if f.KeyRule != nil {
		checkTextRule(c, p+".keyRule", *f.KeyRule)
	}
	if f.ValueRule != nil {
		checkTextRule(c, p+".valueRule", *f.ValueRule)
	}
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// ── string patterns ──────────────────────────────────────────────────────────

var hostnameRe = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// patterns maps each pattern name to its check; nil error means a match.
var patterns = map[string]func(string) error{
	abi.PatternHostname:     checkHostname,
	abi.PatternURLHTTP:      checkURLHTTP,
	abi.PatternPathAbsolute: checkPathAbsolute,
	abi.PatternIdentifier: func(s string) error {
		if !IsIdentifier(s) {
			return fmt.Errorf("must be an identifier (a lower-case letter followed by letters or digits)")
		}
		return nil
	},
}

func checkHostname(s string) error {
	if len(s) > 253 || !hostnameRe.MatchString(s) {
		return fmt.Errorf("must be a host name such as example.org or 10.0.0.10")
	}
	return nil
}

func checkURLHTTP(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("must be an http:// or https:// address without credentials")
	}
	return nil
}

func checkPathAbsolute(s string) error {
	p := strings.ReplaceAll(s, "\\", "/")
	windows := len(p) >= 3 && p[1] == ':' && p[2] == '/' && unicode.IsLetter(rune(p[0]))
	if !(strings.HasPrefix(p, "/") || windows) || slices.Contains(strings.Split(p, "/"), "..") || strings.ContainsAny(s, "\x00\r\n") {
		return fmt.Errorf("must be an absolute path such as /downloads")
	}
	return nil
}

// CheckPattern validates s against a named pattern ("" always matches).
func CheckPattern(pattern, s string) error {
	if fn := patterns[pattern]; fn != nil {
		return fn(s)
	}
	return nil
}
