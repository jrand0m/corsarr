package main

import (
	"encoding/json"
	"fmt"

	"github.com/jrand0m/corsarr/internal/buildinfo"
)

func init() { commandFlags["json"] = true }

// printVersion prints the version, or with --json the full build provenance.
func (c cli) printVersion() int {
	info := buildinfo.Get(version)
	if !c.has("json") {
		fmt.Fprintln(c.stdout, "corsarr", info.Version)
		return 0
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	fmt.Fprintln(c.stdout, string(b))
	return 0
}
