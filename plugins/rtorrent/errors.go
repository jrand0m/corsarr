package main

import (
	"errors"

	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// classify gives a client error the class corsarr acts on. A failure the host
// reported (a refused host, a timeout) keeps the host's class; otherwise the
// client's kind applies, whose names match corsarr's classes.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var host *abi.Error
	if errors.As(err, &host) {
		return &abi.Error{Class: host.Class, Message: err.Error()}
	}
	class := string(client.KindOf(err))
	if class == "" {
		class = abi.ClassServerError
	}
	return &abi.Error{Class: class, Message: err.Error()}
}
