package config

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// referenceIntro opens the Markdown reference: how the sources combine.
const referenceIntro = `# Configuration reference

<!-- Generated from the built-in schema by ` + "`just docs config`" + `; edit the schema, not this file. -->

corsarr reads its settings from exactly one source each time it starts, picked in this order:

1. **A YAML file**: the one named by ` + "`--configFile=…`" + ` or ` + "`CORSARR_CONFIGFILE`" + `, otherwise the first
   ` + "`corsarr.yml`" + ` (or ` + "`corsarr.yaml`" + `) found in ` + "`/etc/corsarr/`" + `, ` + "`$XDG_CONFIG_HOME/corsarr/`" + `,
   ` + "`~/.config/corsarr/`" + ` and the working folder. The file is watched: settings marked live apply
   without a restart.
2. **Command-line arguments**, such as ` + "`--server.address=:8090`" + `.
3. **Environment variables** starting with ` + "`CORSARR_`" + `, such as ` + "`CORSARR_SERVER_ADDRESS=:8090`" + `.
4. **The built-in defaults** listed below.

Sources are never mixed: when a file is used, arguments and variables are ignored (corsarr warns
about them). Anything you leave out keeps its default. The key ` + "`server.address`" + ` is written
` + "`server: { address: … }`" + ` in the file, ` + "`--server.address=…`" + ` as an argument and
` + "`CORSARR_SERVER_ADDRESS=…`" + ` as a variable. Keys match in any letter case.

Secrets are best given as a reference: ` + "`${env:NAME}`" + ` reads an environment variable and
` + "`${file:/run/secrets/name}`" + ` reads a file. Their values are never printed.

In keys, ` + "`<name>`" + ` is a name you choose (lower-case letters and digits, starting with a letter)
and ` + "`<n>`" + ` is a list position starting at 0. **Live** says whether a change in the watched file
applies at once (yes) or needs a restart (no).

The same reference is printed by ` + "`corsarr config defaults`" + ` (add ` + "`--format=markdown`" + `, ` + "`yaml`" + `
or ` + "`env`" + `). A commented file with every default is in
[` + "`deploy/corsarr.example.yml`" + `](../deploy/corsarr.example.yml).
`

// Markdown renders the reference as a Markdown document: the introduction,
// the main table, then one table per plugin.
func (r Reference) Markdown() []byte {
	var b bytes.Buffer
	b.WriteString(referenceIntro)
	b.WriteString("\n## Settings\n\n")
	mdTable(&b, r.Rows)
	b.WriteString("\n## Plugin settings\n\nEach entry under a connection section names its plugin with `type`; " +
		"the plugin adds the settings below to that entry.\n")
	var kind plugin.Kind
	for _, t := range r.Plugins {
		if t.Kind != kind {
			kind = t.Kind
			fmt.Fprintf(&b, "\n### %s (`%s`)\n", capitalize(kindLabel(kind))+" plugins", t.Section)
		}
		fmt.Fprintf(&b, "\n#### %s (`type: %s`)\n\n", t.Name, t.ID)
		mdTable(&b, t.Rows)
	}
	return b.Bytes()
}

func mdTable(b *bytes.Buffer, rows []Row) {
	if len(rows) == 0 {
		b.WriteString("No settings.\n")
		return
	}
	b.WriteString("| Key | Default | Env | Flag | Live | Explanation |\n|---|---|---|---|---|---|\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| `%s` | %s | `%s` | `%s` | %s | %s |\n", row.Key(), mdDefault(row),
			row.Env(), row.Flag(), yesNo(row.Live), mdEscape(row.Explanation()))
	}
}

func mdDefault(row Row) string {
	switch d := row.ShownDefault(); d {
	case "":
		return "—"
	case secretDefault:
		return d
	default:
		return "`" + d + "`"
	}
}

func mdEscape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Table renders the reference as aligned plain text for a terminal.
func (r Reference) Table() []byte {
	var b bytes.Buffer
	textTable(&b, r.Rows)
	for _, t := range r.Plugins {
		fmt.Fprintf(&b, "\n%s plugin %s (type: %s)\n", capitalize(kindLabel(t.Kind)), t.Name, t.ID)
		textTable(&b, t.Rows)
	}
	return b.Bytes()
}

func textTable(b *bytes.Buffer, rows []Row) {
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tDEFAULT\tENV\tFLAG\tLIVE\tEXPLANATION")
	for _, row := range rows {
		d := row.ShownDefault()
		if d == "" {
			d = "—"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", row.Key(), d, row.Env(), row.Flag(), yesNo(row.Live), row.Explanation())
	}
	_ = tw.Flush() // writes to a buffer
}

// Env renders every setting as a CORSARR_* line under its explanation.
// Settings with a default are active lines; the rest are commented out.
func (r Reference) Env() []byte {
	var b bytes.Buffer
	b.WriteString("# corsarr default configuration as environment variables.\n" +
		"# Lines starting with # are settings without a default; remove the # and fill in a value to use one.\n")
	envLines(&b, r.Rows)
	for _, t := range r.Plugins {
		fmt.Fprintf(&b, "\n# ── %s plugin %s: entries with type=%s ──\n", kindLabel(t.Kind), t.Name, t.ID)
		envLines(&b, t.Rows)
	}
	return b.Bytes()
}

func envLines(b *bytes.Buffer, rows []Row) {
	for _, row := range rows {
		fmt.Fprintf(b, "\n# %s\n", row.Explanation())
		if row.Default == "" || row.Secret || hasPlaceholder(row.Path) {
			fmt.Fprintf(b, "# %s=%s\n", row.Env(), envDefault(row))
			continue
		}
		fmt.Fprintf(b, "%s=%s\n", row.Env(), row.Default)
	}
}

func envDefault(row Row) string {
	if row.Secret {
		return ""
	}
	return row.Default
}

func hasPlaceholder(path []string) bool {
	for _, p := range path {
		if strings.HasPrefix(p, "<") {
			return true
		}
	}
	return false
}
