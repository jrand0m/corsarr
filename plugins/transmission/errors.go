package main

import (
	"errors"

	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// classify gives a client error the class corsarr acts on. A failure the host
// reported (a refused host, a timeout) keeps the host's class; otherwise the
// client's class applies, and a missing torrent or an operation the server
// cannot do become not_found and unsupported.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var host *abi.Error
	switch {
	case errors.As(err, &host):
		return &abi.Error{Class: host.Class, Message: err.Error()}
	case errors.Is(err, client.ErrNotFound):
		return &abi.Error{Class: abi.ClassNotFound, Message: err.Error()}
	case errors.Is(err, client.ErrUnsupported):
		return &abi.Error{Class: abi.ClassUnsupported, Message: err.Error()}
	}
	class := string(client.ClassOf(err))
	if class == "" {
		class = abi.ClassServerError
	}
	return &abi.Error{Class: class, Message: err.Error()}
}
