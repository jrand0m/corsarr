// Package abi is the wire contract between corsarr and a plugin module: the
// names of the imports and exports, the operation names and the JSON shapes
// that cross the module's linear memory. Both the host and the module SDK use
// these types, so there is exactly one definition of every message.
//
// Calling convention (version 1):
//
//   - The module exports corsarr_abi_v1 (a marker), corsarr_alloc(size) ptr,
//     corsarr_free(ptr) and corsarr_call(opPtr, opLen, reqPtr, reqLen) i64.
//     corsarr_call returns the module-owned response as (ptr<<32 | len); the
//     host copies it and frees it with corsarr_free.
//   - The host exports functions in the import module "corsarr_v1"; each
//     takes (reqPtr, reqLen) and returns the length of its JSON response,
//     which the module then copies with take(ptr, len).
//   - Every message is a JSON envelope: {"result": …} or {"error": {…}}.
package abi

import "encoding/json"

// Version is the ABI version this package describes.
const Version = 1

// Names of the module exports.
const (
	ExportMarker     = "corsarr_abi_v1"
	ExportMarkerBase = "corsarr_abi_v"
	ExportAlloc      = "corsarr_alloc"
	ExportFree       = "corsarr_free"
	ExportCall       = "corsarr_call"
	ExportInit       = "_initialize"
	ExportStart      = "_start"
)

// HostModule is the import module name of the host functions.
const HostModule = "corsarr_v1"

// Host function names (imports of the module).
const (
	HostLog         = "log"
	HostHTTPDo      = "http_do"
	HostSettingsGet = "settings_get"
	HostSecretGet   = "secret_get"
	HostNow         = "now"
	HostSleep       = "sleep"
	HostEmitEvent   = "emit_event"
	HostTake        = "take"
)

// Operations every module answers.
const (
	OpCapabilities = "capabilities"
	OpTest         = "test"
	OpAction       = "action"
)

// Error classes a module may return; they map onto the host's failure classes.
const (
	ClassAuth        = "auth"
	ClassRateLimited = "rate_limited"
	ClassTimeout     = "timeout"
	ClassUnreachable = "unreachable"
	ClassParse       = "parse"
	ClassServerError = "server_error"
	ClassConfig      = "config"
	ClassPath        = "path"
	ClassNotFound    = "not_found"
	ClassUnsupported = "unsupported"
	ClassDenied      = "denied" // the host refused a call (permission or limit)
)

// Error is the error half of an envelope.
type Error struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Class + ": " + e.Message }

// Envelope carries one result or one error.
type Envelope struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Capabilities answers OpCapabilities.
type Capabilities struct {
	ABI   int      `json:"abi"`
	Kind  string   `json:"kind"`
	Ops   []string `json:"ops"`
	Flags []string `json:"flags,omitempty"`
}

// ActionRequest asks the module to run a settings action with the form's
// current values.
type ActionRequest struct {
	ID     string         `json:"id"`
	Values map[string]any `json:"values"`
}

// Action outcomes.
const (
	OutcomeOK   = "ok"
	OutcomeWarn = "warn"
	OutcomeFail = "fail"
)

// ActionResult is a settings action's plain-text result.
type ActionResult struct {
	Outcome string `json:"outcome"`
	Text    string `json:"text"`
}

// ── host functions ───────────────────────────────────────────────────────────

// LogRequest writes one log line through the host logger.
type LogRequest struct {
	Level   string         `json:"level"` // debug | info | warn | error
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// HTTPRequest is one outbound request.
type HTTPRequest struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// HTTPResponse is the answer to an HTTPRequest.
type HTTPResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// SettingsResponse holds the instance's non-secret settings.
type SettingsResponse struct {
	Values map[string]any `json:"values"`
}

// SecretRequest reads one declared secret setting.
type SecretRequest struct {
	Name string `json:"name"`
}

// SecretResponse is a secret's value.
type SecretResponse struct {
	Value string `json:"value"`
}

// NowResponse is the host clock.
type NowResponse struct {
	UnixNano int64 `json:"unixNano"`
}

// SleepRequest pauses the call (bounded by the call deadline).
type SleepRequest struct {
	Millis int64 `json:"millis"`
}

// EventRequest publishes a whitelisted event.
type EventRequest struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}
