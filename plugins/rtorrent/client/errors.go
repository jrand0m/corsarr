package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Kind classifies a failure. The values that overlap with the host's
// integration-health classes (auth, timeout, unreachable, parse,
// server_error, config) use the same strings, so a wrapper can pass them on
// unchanged.
type Kind string

const (
	KindAuth        Kind = "auth"
	KindTimeout     Kind = "timeout"
	KindUnreachable Kind = "unreachable"
	KindParse       Kind = "parse"
	KindServer      Kind = "server_error"
	KindConfig      Kind = "config"
	KindNotFound    Kind = "not_found"
	KindUnsupported Kind = "unsupported"
)

// ErrUnsupported matches (errors.Is) every failure where the client cannot
// perform an operation at all; it is a normal outcome, not a health failure.
var ErrUnsupported = errors.New("not supported by this client")

// ErrNotFound matches (errors.Is) every failure where the named download does
// not exist.
var ErrNotFound = errors.New("not found")

// Error is a classified failure.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// Is lets errors.Is match ErrUnsupported and ErrNotFound by kind.
func (e *Error) Is(target error) bool {
	return (target == ErrUnsupported && e.Kind == KindUnsupported) ||
		(target == ErrNotFound && e.Kind == KindNotFound)
}

func errorf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

func wrap(kind Kind, msg string, err error) *Error { return &Error{Kind: kind, Msg: msg, Err: err} }

// KindOf returns the kind of err, or "" when it is not a classified error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// Fault is an XML-RPC fault answer.
type Fault struct {
	Code    int
	Message string
}

func (f *Fault) Error() string { return fmt.Sprintf("fault %d: %s", f.Code, f.Message) }

// noSuchMethod is the fault code XML-RPC servers use for an unknown method.
const noSuchMethod = -506

// classifyFault turns a fault into a classified error: an unknown method is
// unsupported, an unknown info hash is not found, anything else is a server
// error.
func classifyFault(method string, f *Fault) *Error {
	msg := strings.ToLower(f.Message)
	switch {
	case f.Code == noSuchMethod || strings.Contains(msg, "not defined"):
		return wrap(KindUnsupported, method, f)
	case strings.Contains(msg, "info-hash") || strings.Contains(msg, "info hash"):
		return wrap(KindNotFound, method, f)
	}
	return wrap(KindServer, method, f)
}

// classifyTransport maps a transport error to timeout or unreachable.
func classifyTransport(err error) Kind {
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	return KindUnreachable
}

// classifyStatus maps an HTTP status to a kind ("" = success).
func classifyStatus(status int) Kind {
	switch {
	case status == 401 || status == 403:
		return KindAuth
	case status >= 200 && status < 300:
		return ""
	case status == 404:
		return KindConfig
	}
	return KindServer
}
