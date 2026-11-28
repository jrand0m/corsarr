// Package plugin is the Go SDK for corsarr plugin modules. A module is a Go
// main package built as a WebAssembly reactor:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o mymodule.wasm .
//
// The module registers its operations from init (a kind package such as
// sdk/go/plugin/downloader does this for you), describes itself with a
// Manifest and calls Main from main so that running the package natively
// prints or embeds its plugin.json. Everything else (HTTP, settings, secrets,
// logs, the clock, events) goes through the host functions in this package;
// a module has no filesystem, sockets or environment.
//
// Natively (any GOOS other than wasip1) the same code runs against an
// in-process host installed by the plugintest package, so modules are unit
// tested with plain go test.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Handler answers one operation: req is the request JSON, the result is
// encoded as JSON.
type Handler func(req []byte) (any, error)

var (
	handlers = map[string]Handler{}
	kind     string
	flags    []string
)

// SetKind declares the module's extension point (abi.KindDownloader, …).
func SetKind(k string) { kind = k }

// AddFlag declares a capability flag such as abi.FlagRemoveKeepsData.
func AddFlag(f string) { flags = append(flags, f) }

// Handle registers an operation. Registering the same operation twice
// replaces it.
func Handle(op string, h Handler) { handlers[op] = h }

// Op registers a typed operation.
func Op[Req, Resp any](op string, fn func(Req) (Resp, error)) {
	Handle(op, func(raw []byte) (any, error) {
		var req Req
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, Errorf(abi.ClassParse, "malformed %s request: %v", op, err)
			}
		}
		return fn(req)
	})
}

// Empty is the request or result of an operation without data.
type Empty struct{}

// OnTest registers the self-test (operation "test").
func OnTest(fn func() error) {
	Op(abi.OpTest, func(Empty) (Empty, error) { return Empty{}, fn() })
}

// OnAction registers the settings actions handler: id is the action's id
// from the schema, values the form's current non-secret values (secrets are
// readable with Secret as usual). Return an outcome (abi.OutcomeOK, Warn or
// Fail) and a plain-text result.
func OnAction(fn func(id string, values map[string]any) (outcome, text string)) {
	Op(abi.OpAction, func(r abi.ActionRequest) (abi.ActionResult, error) {
		outcome, text := fn(r.ID, r.Values)
		return abi.ActionResult{Outcome: outcome, Text: text}, nil
	})
}

func capabilities() abi.Capabilities {
	ops := make([]string, 0, len(handlers))
	for op := range handlers {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return abi.Capabilities{ABI: abi.Version, Kind: kind, Ops: ops, Flags: flags}
}

// dispatch runs one operation and returns the response envelope. A panic in
// a handler becomes a server_error answer instead of killing the instance.
func dispatch(op string, req []byte) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			out = envelope(nil, Errorf(abi.ClassServerError, "panic in %s: %v", op, r))
		}
	}()
	if op == abi.OpCapabilities {
		return envelope(capabilities(), nil)
	}
	h, ok := handlers[op]
	if !ok {
		return envelope(nil, Errorf(abi.ClassUnsupported, "operation %s is not implemented", op))
	}
	return envelope(h(req))
}

func envelope(res any, err error) []byte {
	var env abi.Envelope
	if err != nil {
		env.Error = toABIError(err)
	} else if res != nil {
		b, merr := json.Marshal(res)
		if merr != nil {
			env.Error = &abi.Error{Class: abi.ClassServerError, Message: "encode result: " + merr.Error()}
		} else {
			env.Result = b
		}
	}
	b, _ := json.Marshal(env)
	return b
}

// ── errors ───────────────────────────────────────────────────────────────────

// Error is a classified module error; the class tells corsarr how to treat
// the failure (auth, unreachable, rate_limited, …).
type Error = abi.Error

// Errorf returns a classified error.
func Errorf(class, format string, a ...any) error {
	return &abi.Error{Class: class, Message: fmt.Sprintf(format, a...)}
}

// Unsupported is the answer for an operation the client cannot perform.
func Unsupported(what string) error { return Errorf(abi.ClassUnsupported, "%s is not supported", what) }

// NotFound is the answer for a missing download or item.
func NotFound(what string) error { return Errorf(abi.ClassNotFound, "%s not found", what) }

// ClassOf returns an error's class, or "" for unclassified errors.
func ClassOf(err error) string {
	var e *abi.Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ""
}

func toABIError(err error) *abi.Error {
	var e *abi.Error
	if errors.As(err, &e) {
		return e
	}
	return &abi.Error{Class: abi.ClassServerError, Message: err.Error()}
}
