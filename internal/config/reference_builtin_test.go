package config_test

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/plugin"

	_ "github.com/jrand0m/corsarr/internal/plugins" // every built-in plugin
)

// Every setting of the shipped schema, plugins included, has an explanation.
func TestEverySettingIsExplained(t *testing.T) {
	ref := config.NewSchema(plugin.Default).Reference()
	if missing := ref.MissingDocs(); len(missing) > 0 {
		t.Fatalf("settings without a Doc (add one sentence: what it does and when to change it):\n  %s",
			strings.Join(missing, "\n  "))
	}
	if len(ref.Rows) < 20 || len(ref.Plugins) == 0 {
		t.Fatalf("reference too small: %d rows, %d plugins", len(ref.Rows), len(ref.Plugins))
	}
}
