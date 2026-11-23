package msgtemplate

import (
	"strings"
	"testing"
)

func TestParseCheckExec(t *testing.T) {
	tpl, err := Parse("t", "[corsarr] {{.Title}} {{join .Files}}")
	if err != nil {
		t.Fatal(err)
	}
	if got := Exec(tpl, map[string]any{"Title": "Imported: X", "Files": []string{"a", "b"}}); got != "[corsarr] Imported: X a, b" {
		t.Fatal(got)
	}
	if err := Check("{{.Title"); err == nil || !strings.Contains(err.Error(), "not a valid message template") {
		t.Fatal(err)
	}
	if err := Check("{{nosuchfunc .X}}"); err == nil {
		t.Fatal("unknown function")
	}
	bad, _ := Parse("b", "{{.Missing.Field}}")
	if got := Exec(bad, struct{}{}); got != "" {
		t.Fatalf("a failing template renders empty: %q", got)
	}
}
