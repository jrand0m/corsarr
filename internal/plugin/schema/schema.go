// Package schema validates a plugin's settings form, which arrives as data:
// a closed set of field types, identifier keys, plain text checked by
// plaintext, optional one-level groups and up to three action buttons. The
// same rules validate the values saved for an instance. Every problem is
// reported with its path and a fixed, server-authored message.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jrand0m/corsarr/internal/plaintext"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Re-exported wire types: the schema is defined once, in the ABI package.
type (
	Schema = abi.Schema
	Field  = abi.Field
	Group  = abi.Group
	Action = abi.Action
	Option = abi.Option
)

// Limits on a schema's size.
const (
	MaxFields  = 50
	MaxGroups  = 10
	MaxActions = 3
	MaxOptions = 100
	MaxItems   = 1000 // ceiling for list maxItems and map maxEntries
	MaxText    = 4096 // ceiling for string maxLength
)

// identRe is the rule for keys, enum values and action ids.
var identRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,47}$`)

// IsIdentifier reports whether s is a valid key or enum value.
func IsIdentifier(s string) bool { return identRe.MatchString(s) }

// Problem is one validation failure.
type Problem struct {
	Path string `json:"path"`
	Msg  string `json:"message"`
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// Problems is a list of failures; it is an error when non-empty.
type Problems []Problem

func (ps Problems) Error() string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return strings.Join(out, "; ")
}

// Err returns ps as an error, or nil when empty.
func (ps Problems) Err() error {
	if len(ps) == 0 {
		return nil
	}
	return ps
}

type collector struct{ ps Problems }

func (c *collector) add(path, format string, a ...any) {
	c.ps = append(c.ps, Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
}

func (c *collector) text(path, s string, kind plaintext.Kind) {
	if _, err := plaintext.Check(s, kind); err != nil {
		c.add(path, "%v", err)
	}
}

// Decode parses a schema strictly: unknown properties are problems.
func Decode(data []byte) (Schema, Problems) {
	var s Schema
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Schema{}, Problems{{Path: "settings", Msg: "is not a valid settings schema: " + err.Error()}}
	}
	return s, Check(s)
}

// Check validates a schema and returns every problem.
func Check(s Schema) Problems {
	c := &collector{}
	if len(s.Fields) > MaxFields {
		c.add("fields", "has %d fields; the limit is %d", len(s.Fields), MaxFields)
		return c.ps
	}
	keys := map[string]bool{}
	for i, f := range s.Fields {
		path := fmt.Sprintf("fields[%d]", i)
		if keys[f.Key] {
			c.add(path+".key", "duplicate key %q", f.Key)
		}
		keys[f.Key] = true
		checkField(c, path, f)
	}
	checkGroups(c, s.Groups, keys)
	checkActions(c, s.Actions)
	return c.ps
}

func checkField(c *collector, path string, f Field) {
	if !IsIdentifier(f.Key) {
		c.add(path+".key", "must start with a lower-case letter followed by letters or digits (at most 48)")
	}
	c.text(path+".label", f.Label, plaintext.Label)
	if strings.TrimSpace(f.Label) == "" {
		c.add(path+".label", "is required")
	}
	c.text(path+".description", f.Description, plaintext.Description)
	c.text(path+".hint", f.Hint, plaintext.Description)
	rule, ok := typeRules[f.Type]
	if !ok {
		c.add(path+".type", "unknown type %q", f.Type)
		return
	}
	for _, prop := range presentProps(f) {
		if !rule.props[prop] {
			c.add(path+"."+prop, "does not apply to type %s", f.Type)
		}
	}
	rule.check(c, path, f)
	checkDefault(c, path, f)
}

func checkDefault(c *collector, path string, f Field) {
	if f.Default == nil {
		return
	}
	if f.Type == abi.TypeSecret {
		c.add(path+".default", "a secret cannot have a default")
		return
	}
	if msg := valueProblem(f, f.Default); msg != "" {
		c.add(path+".default", "%s", msg)
	}
}

func checkGroups(c *collector, groups []Group, keys map[string]bool) {
	if len(groups) > MaxGroups {
		c.add("groups", "has %d groups; the limit is %d", len(groups), MaxGroups)
		return
	}
	placed := map[string]bool{}
	for i, g := range groups {
		path := fmt.Sprintf("groups[%d]", i)
		c.text(path+".title", g.Title, plaintext.Label)
		if strings.TrimSpace(g.Title) == "" {
			c.add(path+".title", "is required")
		}
		c.text(path+".description", g.Description, plaintext.Description)
		for j, k := range g.Fields {
			fp := fmt.Sprintf("%s.fields[%d]", path, j)
			switch {
			case !keys[k]:
				c.add(fp, "names no field (%q)", k)
			case placed[k]:
				c.add(fp, "field %q is already in a group", k)
			}
			placed[k] = true
		}
	}
}

var builtinActions = map[string]bool{"test": true, "discover": true}

func checkActions(c *collector, actions []Action) {
	if len(actions) > MaxActions {
		c.add("actions", "has %d actions; the limit is %d", len(actions), MaxActions)
		return
	}
	seen := map[string]bool{}
	for i, a := range actions {
		path := fmt.Sprintf("actions[%d]", i)
		if !builtinActions[a.ID] && !IsIdentifier(a.ID) {
			c.add(path+".id", "must be test, discover or an identifier")
		}
		if seen[a.ID] {
			c.add(path+".id", "duplicate action %q", a.ID)
		}
		seen[a.ID] = true
		c.text(path+".label", a.Label, plaintext.Label)
		if strings.TrimSpace(a.Label) == "" {
			c.add(path+".label", "is required")
		}
	}
}

// HasAction reports whether the schema declares the action id.
func HasAction(s Schema, id string) bool {
	for _, a := range s.Actions {
		if a.ID == id {
			return true
		}
	}
	return false
}

// FieldOf returns the field with key.
func FieldOf(s Schema, key string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}
