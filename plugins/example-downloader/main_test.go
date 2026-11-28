package main

import (
	"net/http/httptest"
	"testing"

	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

// The module runs natively under the SDK harness: the same operations the
// host calls inside the wasm module.
func TestModuleNatively(t *testing.T) {
	api := fakeapi.New("k")
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	h := plugintest.New(plugintest.Host{Settings: map[string]any{"url": srv.URL}, Secrets: map[string]string{"apiKey": "k"},
		Hosts: []string{"127.0.0.1"}})
	caps, err := h.Capabilities()
	if err != nil || caps.Kind != abi.KindDownloader || len(caps.Ops) < len(abi.DownloaderRequired)+10 {
		t.Fatalf("%+v %v", caps, err)
	}
	if err := h.Call(abi.OpTest, struct{}{}, nil); err != nil {
		t.Fatal(err)
	}
	var added abi.AddResponse
	if err := h.Call(abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{URL: "magnet:?x"}, Category: "tv"}, &added); err != nil || added.ID == "" {
		t.Fatalf("%+v %v", added, err)
	}
	var list abi.ListResponse
	if err := h.Call(abi.OpDLList, abi.ListRequest{Categories: []string{"tv"}}, &list); err != nil || len(list.Downloads) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	for op, req := range map[string]any{
		abi.OpDLPause: abi.IDRequest{ID: added.ID}, abi.OpDLResume: abi.IDRequest{ID: added.ID},
		abi.OpDLSetPriority: abi.PriorityRequest{ID: added.ID, Priority: "top"}, abi.OpDLEnsureCategory: abi.CategoryRequest{Category: "tv"},
		abi.OpDLSetPriorityLevel: abi.PriorityRequest{ID: added.ID, Priority: "high"}, abi.OpDLPauseAll: struct{}{},
		abi.OpDLResumeAll: struct{}{}, abi.OpDLSetSpeedLimits: abi.SpeedLimits{DownKiB: 5}, abi.OpDLSetAltSpeed: abi.Toggle{On: true},
		abi.OpDLRecheck: abi.IDRequest{ID: added.ID}, abi.OpDLReannounce: abi.IDRequest{ID: added.ID},
		abi.OpDLSetCategory: abi.CategoryRequest{ID: added.ID, Category: "movies"}, abi.OpDLFiles: abi.IDRequest{ID: added.ID},
		abi.OpDLTrackers: abi.IDRequest{ID: added.ID}, abi.OpDLStats: struct{}{}, abi.OpDLSpeedLimits: struct{}{},
		abi.OpDLAltSpeed: struct{}{}, abi.OpDLProtocols: struct{}{},
	} {
		if err := h.Call(op, req, nil); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
	var res abi.ActionResult
	if err := h.Call(abi.OpAction, abi.ActionRequest{ID: "test"}, &res); err != nil || res.Outcome != abi.OutcomeOK {
		t.Errorf("%+v %v", res, err)
	}
	if err := h.Call(abi.OpDLRemove, abi.RemoveRequest{ID: added.ID, DeleteFiles: true}, nil); err != nil || len(api.Deleted) != 1 {
		t.Errorf("remove: %v %v", err, api.Deleted)
	}
	if err := h.Call(abi.OpDLPause, abi.IDRequest{ID: added.ID}, nil); err == nil {
		t.Error("removed download still answers")
	}
}

func TestManifestIsValidJSON(t *testing.T) {
	if b, err := manifest.JSON(); err != nil || len(b) < 100 {
		t.Fatal(err)
	}
}
