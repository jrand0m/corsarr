package main

import (
	"fmt"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// defaults prints every setting with its default and explanation; it needs
// no configuration source.
func (c cli) defaults() int {
	s := config.NewSchema(plugin.Default)
	var out []byte
	switch c.flags["format"] {
	case "", "table":
		out = s.Reference().Table()
	case "markdown":
		out = s.Reference().Markdown()
	case "yaml":
		out = s.DefaultsYAML()
	case "env":
		out = s.Reference().Env()
	default:
		fmt.Fprintln(c.stderr, "--format must be table, markdown, yaml or env")
		return 2
	}
	_, _ = c.stdout.Write(out)
	return 0
}
