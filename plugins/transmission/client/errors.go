package client

import (
	"context"
	"errors"
	"fmt"
)

// Class is the failure class health reporting needs; the values match the
// host's integration health classes.
type Class string

const (
	ClassAuth        Class = "auth"
	ClassRateLimited Class = "rate_limited"
	ClassTimeout     Class = "timeout"
	ClassUnreachable Class = "unreachable"
	ClassParse       Class = "parse"
	ClassServerError Class = "server_error"
	ClassConfig      Class = "config"
	ClassPath        Class = "path"
)

// ErrUnsupported is returned (wrapped) when the connected Transmission cannot
// perform an operation at all; it is a normal outcome, not a health failure.
var ErrUnsupported = errors.New("not supported by this client")

// ErrNotFound is returned (wrapped) when the named torrent does not exist.
var ErrNotFound = errors.New("not found")

// Error is a classified failure.
type Error struct {
	Class Class
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

func errorf(class Class, format string, args ...any) *Error {
	return &Error{Class: class, Msg: fmt.Sprintf(format, args...)}
}

func wrap(class Class, msg string, err error) *Error {
	return &Error{Class: class, Msg: msg, Err: err}
}

// ClassOf returns the failure class of err, or "" when it carries none.
func ClassOf(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ""
}

// transportClass maps a transport error to timeout or unreachable.
func transportClass(err error) Class {
	var t interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &t) && t.Timeout()) {
		return ClassTimeout
	}
	return ClassUnreachable
}

// statusClass maps an HTTP error status to a failure class.
func statusClass(status int) Class {
	switch {
	case status == 401 || status == 403:
		return ClassAuth
	case status == 429:
		return ClassRateLimited
	}
	return ClassServerError
}
