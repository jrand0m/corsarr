package conformance

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func goodManifest() plugin.Manifest {
	return plugin.Manifest{ID: "x", Kind: plugin.KindIndexer, Version: "1", APIVersion: plugin.APIVersion, Name: "X", Delivery: "builtin",
		Settings: []plugin.Field{
			{Key: "url", Type: plugin.FieldURL, Label: "URL", Help: "h", Required: true},
			{Key: "apiKey", Type: plugin.FieldSecret, Label: "Key", Help: "h"},
		}}
}

func TestManifestProblems(t *testing.T) {
	if p := ManifestProblems(goodManifest(), plugin.KindIndexer); len(p) != 0 {
		t.Fatalf("good manifest flagged: %v", p)
	}
	bad := goodManifest()
	bad.ID = "X"
	bad.Delivery = "external"
	bad.APIVersion = 99
	bad.Version = ""
	bad.Settings = append(bad.Settings,
		plugin.Field{Key: "Bad_Key", Type: "weird", Label: "l"},
		plugin.Field{Key: "url", Type: plugin.FieldText, Label: "l", Help: "h"},
		plugin.Field{Key: "authToken", Type: plugin.FieldText, Label: "l", Help: "h"},
		plugin.Field{Key: "mode", Type: plugin.FieldEnum, Label: "l", Help: "h", Required: true, Default: "a"},
		plugin.Field{Key: "pw", Type: plugin.FieldSecret, Label: "l", Help: "h", Default: "x"},
	)
	joined := strings.Join(ManifestProblems(bad, plugin.KindDownloader), "\n")
	for _, want := range []string{"non-empty lower case", "kind", "version and name", "apiVersion", "delivery", "camelCase", "unknown type", "label and help", "duplicate key", "not flagged secret", "enum needs options", "cannot have a default", "secret fields cannot"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestManifestHelper(t *testing.T) {
	Manifest(t, goodManifest(), plugin.KindIndexer)
}

type fakeTester struct{ url string }

func (f fakeTester) Test(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	_, err := plugin.Fetch(http.DefaultClient, req, 10)
	return err
}

func TestErrorMappingAgainstSimplePlugin(t *testing.T) {
	ErrorMapping(t, func(url string) plugin.Tester { return fakeTester{url: url} })
}

func TestEnv(t *testing.T) {
	e := Env(http.DefaultClient)
	if e.HTTP() == nil || e.Now().Year() != 2026 {
		t.Fatal("env")
	}
}
