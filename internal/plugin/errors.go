package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// FailureClass is an IntegrationHealth failure class.
type FailureClass string

const (
	ClassAuth        FailureClass = "auth"
	ClassRateLimited FailureClass = "rate_limited"
	ClassTimeout     FailureClass = "timeout"
	ClassUnreachable FailureClass = "unreachable"
	ClassParse       FailureClass = "parse"
	ClassServerError FailureClass = "server_error"
	ClassConfig      FailureClass = "config"
	ClassPath        FailureClass = "path"
	// ClassIdentityMismatch: the server at the pinned address is not the one that was paired.
	ClassIdentityMismatch FailureClass = "identity_mismatch"
)

// ErrNotFound is returned (wrapped) when a lookup target does not exist; it is
// a normal outcome, not a health failure.
var ErrNotFound = errors.New("not found")

// Error is a plugin failure carrying the class health reporting needs.
type Error struct {
	Class FailureClass
	Msg   string
	Err   error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Class, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Class, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// Errorf builds a classified error.
func Errorf(class FailureClass, format string, args ...any) *Error {
	return &Error{Class: class, Msg: fmt.Sprintf(format, args...)}
}

// Wrap classifies an underlying error.
func Wrap(class FailureClass, msg string, err error) *Error {
	return &Error{Class: class, Msg: msg, Err: err}
}

// ClassOf returns the failure class of err, or "" when it is not a plugin error.
func ClassOf(err error) FailureClass {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Class
	}
	return ""
}

// ClassifyTransport maps a transport-level error to timeout or unreachable.
func ClassifyTransport(err error) FailureClass {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return ClassTimeout
	}
	return ClassUnreachable
}

// ClassifyStatus maps an HTTP status to a failure class ("" = not a failure
// at this level; 404 and friends are the caller's to interpret).
func ClassifyStatus(status int) FailureClass {
	switch {
	case status == 401 || status == 403:
		return ClassAuth
	case status == 429:
		return ClassRateLimited
	case status >= 500:
		return ClassServerError
	}
	return ""
}
