package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestPluginListEntriesAreChecked(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.MustRegister(plugin.Registration{New: func(plugin.Instance) (any, error) { return nil, nil }, Manifest: plugin.Manifest{
		ID: "qbittorrent", Kind: plugin.KindDownloader, APIVersion: plugin.APIVersion, Name: "qBittorrent",
		Settings: []plugin.Field{
			{Key: "url", Type: plugin.FieldURL, Required: true},
			{Key: "pathMap", Type: plugin.FieldList, Check: plugin.CheckPathRule},
		},
	}})
	file := `version: 1
downloadClients:
  qbit:
    type: qbittorrent
    url: http://q
    pathMap:
      - /downloads=/data/downloads
      - not-a-mapping
`
	_, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file}), reg)
	var errs Errors
	if !errors.As(err, &errs) || len(errs) != 1 || errs[0].Path != "downloadClients.qbit.pathMap.1" ||
		!strings.Contains(err.Error(), "remote=local") {
		t.Fatalf("want one error on the bad entry, got %v", err)
	}
	ok := strings.Replace(file, "      - not-a-mapping\n", "", 1)
	cfg, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": ok}), reg)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Instances["downloadClients"]["qbit"].Settings.Strings("pathMap"); len(got) != 1 || got[0] != "/downloads=/data/downloads" {
		t.Fatalf("pathMap %v", got)
	}
}
