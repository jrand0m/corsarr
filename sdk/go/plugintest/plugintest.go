//go:build !wasip1

// Package plugintest runs a module's operations natively, in-process, with a
// fake host: settings, secrets, a real or test HTTP client, a clock, and
// captured logs and events. Use it from the module's own go test; run the
// built .wasm through `corsarr plugin test` for the full conformance suite.
package plugintest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// Host is the fake host's configuration.
type Host struct {
	Settings map[string]any
	Secrets  map[string]string
	// Client sends http_do requests (default http.DefaultClient).
	Client *http.Client
	// Hosts, when set, are the only host names http_do may reach.
	Hosts []string
	Now   func() time.Time
}

// Harness is an installed fake host.
type Harness struct {
	h      Host
	funcs  map[string]func([]byte) (any, error)
	mu     sync.Mutex
	logs   []abi.LogRequest
	events []abi.EventRequest
}

// New installs a fake host for the module code linked into this test binary.
func New(h Host) *Harness {
	if h.Client == nil {
		h.Client = http.DefaultClient
	}
	if h.Now == nil {
		h.Now = time.Now
	}
	hs := &Harness{h: h}
	hs.funcs = map[string]func([]byte) (any, error){
		abi.HostLog:         hs.log,
		abi.HostEmitEvent:   hs.emit,
		abi.HostSettingsGet: hs.settings,
		abi.HostSecretGet:   hs.secret,
		abi.HostNow:         hs.now,
		abi.HostSleep:       sleep,
		abi.HostHTTPDo:      hs.http,
	}
	plugin.SetHost(hs.serve)
	return hs
}

// Call runs one operation and decodes its result into resp (nil discards).
func (hs *Harness) Call(op string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	var env abi.Envelope
	if err := json.Unmarshal(plugin.Dispatch(op, body), &env); err != nil {
		return err
	}
	if env.Error != nil {
		return env.Error
	}
	if resp != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, resp)
	}
	return nil
}

// Capabilities asks the module for its capability list.
func (hs *Harness) Capabilities() (abi.Capabilities, error) {
	var c abi.Capabilities
	err := hs.Call(abi.OpCapabilities, struct{}{}, &c)
	return c, err
}

// Logs returns the captured log lines.
func (hs *Harness) Logs() []abi.LogRequest {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return append([]abi.LogRequest(nil), hs.logs...)
}

// Events returns the captured events.
func (hs *Harness) Events() []abi.EventRequest {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return append([]abi.EventRequest(nil), hs.events...)
}

func (hs *Harness) serve(name string, req []byte) []byte {
	res, err := hs.handle(name, req)
	var env abi.Envelope
	if err != nil {
		var e *abi.Error
		if !errors.As(err, &e) {
			e = &abi.Error{Class: abi.ClassServerError, Message: err.Error()}
		}
		env.Error = e
	} else if res != nil {
		env.Result, _ = json.Marshal(res)
	}
	b, _ := json.Marshal(env)
	return b
}

func (hs *Harness) handle(name string, req []byte) (any, error) {
	fn, ok := hs.funcs[name]
	if !ok {
		return nil, denied("unknown host function " + name)
	}
	return fn(req)
}

func denied(msg string) error { return &abi.Error{Class: abi.ClassDenied, Message: msg} }

func (hs *Harness) log(req []byte) (any, error) {
	var r abi.LogRequest
	_ = json.Unmarshal(req, &r)
	hs.mu.Lock()
	hs.logs = append(hs.logs, r)
	hs.mu.Unlock()
	return nil, nil
}

func (hs *Harness) emit(req []byte) (any, error) {
	var r abi.EventRequest
	_ = json.Unmarshal(req, &r)
	hs.mu.Lock()
	hs.events = append(hs.events, r)
	hs.mu.Unlock()
	return nil, nil
}

func (hs *Harness) settings([]byte) (any, error) {
	return abi.SettingsResponse{Values: hs.h.Settings}, nil
}

func (hs *Harness) secret(req []byte) (any, error) {
	var r abi.SecretRequest
	_ = json.Unmarshal(req, &r)
	v, ok := hs.h.Secrets[r.Name]
	if !ok {
		return nil, denied("secret " + r.Name + " is not declared")
	}
	return abi.SecretResponse{Value: v}, nil
}

func (hs *Harness) now([]byte) (any, error) {
	return abi.NowResponse{UnixNano: hs.h.Now().UnixNano()}, nil
}

func sleep([]byte) (any, error) { return nil, nil }

func (hs *Harness) http(raw []byte) (any, error) {
	var r abi.HTTPRequest
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}
	if len(hs.h.Hosts) > 0 && !allowed(hs.h.Hosts, u.Hostname()) {
		return nil, denied("host " + u.Hostname() + " is not declared")
	}
	req, err := http.NewRequest(r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	req.Header = r.Headers
	resp, err := hs.h.Client.Do(req)
	if err != nil {
		return nil, &abi.Error{Class: abi.ClassUnreachable, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return abi.HTTPResponse{Status: resp.StatusCode, Headers: resp.Header, Body: body}, err
}

func allowed(hosts []string, h string) bool {
	for _, p := range hosts {
		if p == h || (strings.HasPrefix(p, "*.") && strings.HasSuffix(h, p[1:])) {
			return true
		}
	}
	return false
}
