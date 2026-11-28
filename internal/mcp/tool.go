package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Tool is one MCP tool. Write tools need a write-scoped key; the hints tell
// clients how careful to be before calling.
type Tool struct {
	Name        string
	Title       string
	Description string
	Input       schema
	Output      schema // the structuredContent's shape (outputOf the result type)
	Write       bool
	Destructive bool
	Idempotent  bool
	OpenWorld   bool // reaches metadata providers or indexers
	Run         func(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result is a tool's answer: a short text for the conversation and the
// structured data behind it.
type Result struct {
	Text string
	Data any
}

type toolInfo struct {
	Name         string      `json:"name"`
	Title        string      `json:"title,omitempty"`
	Description  string      `json:"description"`
	InputSchema  schema      `json:"inputSchema"`
	OutputSchema schema      `json:"outputSchema,omitempty"`
	Annotations  annotations `json:"annotations"`
}

type annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

func (t Tool) info() toolInfo {
	return toolInfo{Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: t.Input, OutputSchema: t.Output,
		Annotations: annotations{Title: t.Title, ReadOnlyHint: !t.Write, DestructiveHint: t.Destructive,
			IdempotentHint: t.Idempotent || !t.Write, OpenWorldHint: t.OpenWorld}}
}

// bind decodes the arguments into T before running the tool.
func bind[T any](run func(context.Context, T) (Result, error)) func(context.Context, json.RawMessage) (Result, error) {
	return func(ctx context.Context, raw json.RawMessage) (Result, error) {
		var in T
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &in); err != nil {
				return Result{}, invalidArg("arguments", err.Error())
			}
		}
		return run(ctx, in)
	}
}

// ArgError is a bad tool argument.
type ArgError struct{ Field, Msg string }

func (e ArgError) Error() string { return e.Field + ": " + e.Msg }

func invalidArg(field, msg string) error { return ArgError{Field: field, Msg: msg} }

// errUnsupported marks an operation corsarr cannot do (yet).
var errUnsupported = errors.New("unsupported")

func unsupported(msg string) error { return fmt.Errorf("%w: %s", errUnsupported, msg) }

// ── JSON Schema helpers ──────────────────────────────────────────────────────

type schema map[string]any

func object(props map[string]schema, required ...string) schema {
	s := schema{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(doc string) schema     { return schema{"type": "string", "description": doc} }
func integer(doc string) schema { return schema{"type": "integer", "description": doc} }
func boolean(doc string) schema { return schema{"type": "boolean", "description": doc} }

func enum(doc string, values ...string) schema {
	return schema{"type": "string", "description": doc, "enum": values}
}

func array(doc string, items schema) schema {
	return schema{"type": "array", "description": doc, "items": items}
}

// kindEnum is the library kinds tools address.
func kindEnum(doc string) schema { return enum(doc, "movie", "tv", "music") }

func withPaging(props map[string]schema) map[string]schema {
	props["cursor"] = str("Opaque cursor from a previous page's nextCursor.")
	props["limit"] = integer("Items per page (default 50, at most 200).")
	return props
}

// ── paging ───────────────────────────────────────────────────────────────────

// DefaultLimit and MaxLimit bound every list.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

type paging struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

func (p paging) size() int {
	if p.Limit <= 0 {
		return DefaultLimit
	}
	return min(p.Limit, MaxLimit)
}

// offset reads a numeric cursor ("" = start).
func (p paging) offset() (int64, error) {
	if p.Cursor == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(p.Cursor, 10, 64)
	if err != nil || n < 0 {
		return 0, invalidArg("cursor", "not a cursor from a previous page")
	}
	return n, nil
}

func nextCursor(n int64) string {
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// page is a bounded list with the cursor of the next page.
type page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// slicePage pages an in-memory list by offset cursor.
func slicePage[T any](all []T, p paging) (page[T], error) {
	from, err := p.offset()
	if err != nil {
		return page[T]{}, err
	}
	from = min(from, int64(len(all)))
	to := min(from+int64(p.size()), int64(len(all)))
	out := page[T]{Items: append([]T{}, all[from:to]...)}
	if to < int64(len(all)) {
		out.NextCursor = nextCursor(to)
	}
	return out, nil
}

func countText(n int, noun string, next string) string {
	s := fmt.Sprintf("%d %s", n, noun)
	if next != "" {
		s += " (more: pass cursor " + strconv.Quote(next) + ")"
	}
	return s
}
