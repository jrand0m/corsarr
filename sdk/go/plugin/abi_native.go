//go:build !wasip1

package plugin

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// HostFunc answers a host-function call natively: name is the host function
// (abi.HostLog, abi.HostHTTPDo, …), req its JSON request, the result a JSON
// envelope. The plugintest package installs one.
type HostFunc func(name string, req []byte) []byte

var host HostFunc

// SetHost installs the native host (tests only; inside a module the host is
// corsarr itself).
func SetHost(h HostFunc) { host = h }

func rawHost(name string, req []byte) ([]byte, error) {
	if host == nil {
		return nil, Errorf(abi.ClassDenied, "no host: run the module in corsarr or under plugintest")
	}
	return host(name, req), nil
}

// Dispatch runs one operation natively, exactly as corsarr_call does inside
// the module, and returns the JSON envelope.
func Dispatch(op string, req []byte) []byte { return dispatch(op, req) }

// Main makes the module's main package a small tool when run natively:
//
//	go run .                       print plugin.json
//	go run . -embed in.wasm -o out.wasm
//	                               write a copy of in.wasm with plugin.json embedded
//
// Inside the module (wasip1) Main does nothing.
func Main(m Manifest) { os.Exit(runMain(m, os.Args[1:], os.Stdout, os.Stderr)) }

func runMain(m Manifest, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plugin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	embed := fs.String("embed", "", "module to embed the manifest into")
	out := fs.String("o", "", "output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	data, err := m.JSON()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *embed != "" {
		module, err := os.ReadFile(*embed)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		data = EmbedManifest(module, data)
	}
	if *out == "" {
		_, _ = stdout.Write(data)
		return 0
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// JSON is the manifest as plugin.json.
func (m Manifest) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(abi.Manifest(m), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
