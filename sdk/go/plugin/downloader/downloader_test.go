package downloader_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

// core implements only Downloader and records what each operation received.
type core struct{ calls []string }

func (c *core) rec(s string) error {
	c.calls = append(c.calls, s)
	if strings.HasSuffix(s, " fail") {
		return errors.New("boom")
	}
	return nil
}

func (c *core) Test() error         { return c.rec("test") }
func (c *core) Protocols() []string { return []string{"torrent"} }
func (c *core) Add(r abi.AddRequest) (string, error) {
	return "id-" + r.Payload.URL, c.rec("add " + r.Category)
}
func (c *core) List(cats []string) ([]abi.Snapshot, error) {
	return []abi.Snapshot{{ID: "a", Category: cats[0]}}, c.rec("list")
}
func (c *core) Remove(id string, del bool) error {
	if del {
		return c.rec("remove+data " + id)
	}
	return c.rec("remove " + id)
}
func (c *core) Pause(id string) error             { return c.rec("pause " + id) }
func (c *core) Resume(id string) error            { return c.rec("resume " + id) }
func (c *core) SetPriority(id, move string) error { return c.rec("priority " + id + " " + move) }
func (c *core) EnsureCategory(cat, path string) error {
	return c.rec("category " + cat + " " + path)
}

// full implements every optional capability as well.
type full struct {
	core
	keeps bool
	alt   bool
	lim   abi.SpeedLimits
}

func (f *full) Files(id string) ([]abi.File, error) {
	return []abi.File{{Path: id + "/f"}}, f.rec("files " + id)
}
func (f *full) SetPriorityLevel(id, lvl string) error { return f.rec("level " + id + " " + lvl) }
func (f *full) PauseAll() error                       { return f.rec("pause all") }
func (f *full) ResumeAll() error                      { return f.rec("resume all") }
func (f *full) SpeedLimits() (abi.SpeedLimits, error) { return f.lim, nil }
func (f *full) SetSpeedLimits(l abi.SpeedLimits) error {
	f.lim = l
	return nil
}
func (f *full) AltSpeed() (bool, error) { return f.alt, nil }
func (f *full) SetAltSpeed(on bool) error {
	f.alt = on
	return nil
}
func (f *full) Recheck(id string) error          { return f.rec("recheck " + id) }
func (f *full) Reannounce(id string) error       { return f.rec("reannounce " + id) }
func (f *full) SetCategory(id, cat string) error { return f.rec("set category " + id + " " + cat) }
func (f *full) Stats() (abi.Stats, error)        { return abi.Stats{DownSpeed: 7}, nil }
func (f *full) KeepsDataOnRemove() bool          { return f.keeps }
func (f *full) Trackers(id string) ([]abi.Tracker, error) {
	return []abi.Tracker{{URL: "udp://" + id}}, f.rec("trackers " + id)
}

var optionalOps = []string{
	abi.OpDLFiles, abi.OpDLSetPriorityLevel, abi.OpDLPauseAll, abi.OpDLResumeAll, abi.OpDLSpeedLimits,
	abi.OpDLSetSpeedLimits, abi.OpDLAltSpeed, abi.OpDLSetAltSpeed, abi.OpDLRecheck, abi.OpDLReannounce,
	abi.OpDLSetCategory, abi.OpDLStats, abi.OpDLTrackers,
}

// Operations register into one process-wide table, so the steps run in order:
// a core-only client first, then one with every capability.
func TestServeRegistersWhatTheClientImplements(t *testing.T) {
	h := plugintest.New(plugintest.Host{})

	t.Run("core only", func(t *testing.T) {
		c := &core{}
		downloader.Serve(c)
		caps, err := h.Capabilities()
		if err != nil || caps.Kind != abi.KindDownloader || len(caps.Flags) != 0 {
			t.Fatalf("%+v %v", caps, err)
		}
		for _, op := range abi.DownloaderRequired {
			if !slices.Contains(caps.Ops, op) {
				t.Errorf("required %s missing", op)
			}
		}
		for _, op := range optionalOps {
			if slices.Contains(caps.Ops, op) {
				t.Errorf("optional %s offered by a client without it", op)
			}
		}
		var p abi.ProtocolsResponse
		var added abi.AddResponse
		var list abi.ListResponse
		steps := []error{
			h.Call(abi.OpTest, struct{}{}, nil),
			h.Call(abi.OpDLProtocols, struct{}{}, &p),
			h.Call(abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{URL: "m"}, Category: "tv"}, &added),
			h.Call(abi.OpDLList, abi.ListRequest{Categories: []string{"tv"}}, &list),
			h.Call(abi.OpDLRemove, abi.RemoveRequest{ID: "a", DeleteFiles: true}, nil),
			h.Call(abi.OpDLRemove, abi.RemoveRequest{ID: "b"}, nil),
			h.Call(abi.OpDLPause, abi.IDRequest{ID: "a"}, nil),
			h.Call(abi.OpDLResume, abi.IDRequest{ID: "a"}, nil),
			h.Call(abi.OpDLSetPriority, abi.PriorityRequest{ID: "a", Priority: "top"}, nil),
			h.Call(abi.OpDLEnsureCategory, abi.CategoryRequest{Category: "tv", SavePath: "/d"}, nil),
		}
		if err := errors.Join(steps...); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.Protocols, []string{"torrent"}) || added.ID != "id-m" || len(list.Downloads) != 1 || list.Downloads[0].Category != "tv" {
			t.Errorf("results: %+v %+v %+v", p, added, list)
		}
		want := []string{"test", "add tv", "list", "remove+data a", "remove b", "pause a", "resume a", "priority a top", "category tv /d"}
		if !reflect.DeepEqual(c.calls, want) {
			t.Errorf("calls %v\nwant  %v", c.calls, want)
		}
		if err := h.Call(abi.OpDLPause, abi.IDRequest{ID: "fail"}, nil); err == nil {
			t.Error("a client error reaches the caller")
		}
	})

	t.Run("every capability", func(t *testing.T) {
		f := &full{keeps: true}
		downloader.Serve(f)
		caps, err := h.Capabilities()
		if err != nil || !slices.Contains(caps.Flags, abi.FlagRemoveKeepsData) {
			t.Fatalf("%+v %v", caps, err)
		}
		for _, op := range optionalOps {
			if !slices.Contains(caps.Ops, op) {
				t.Errorf("optional %s missing", op)
			}
		}
		var files abi.FilesResponse
		var tr abi.TrackersResponse
		var st abi.Stats
		var lim abi.SpeedLimits
		var alt abi.Toggle
		steps := []error{
			h.Call(abi.OpDLFiles, abi.IDRequest{ID: "a"}, &files),
			h.Call(abi.OpDLSetPriorityLevel, abi.PriorityRequest{ID: "a", Priority: "high"}, nil),
			h.Call(abi.OpDLPauseAll, struct{}{}, nil),
			h.Call(abi.OpDLResumeAll, struct{}{}, nil),
			h.Call(abi.OpDLSetSpeedLimits, abi.SpeedLimits{DownKiB: 5, UpKiB: 6}, nil),
			h.Call(abi.OpDLSpeedLimits, struct{}{}, &lim),
			h.Call(abi.OpDLSetAltSpeed, abi.Toggle{On: true}, nil),
			h.Call(abi.OpDLAltSpeed, struct{}{}, &alt),
			h.Call(abi.OpDLRecheck, abi.IDRequest{ID: "a"}, nil),
			h.Call(abi.OpDLReannounce, abi.IDRequest{ID: "a"}, nil),
			h.Call(abi.OpDLSetCategory, abi.CategoryRequest{ID: "a", Category: "movies"}, nil),
			h.Call(abi.OpDLStats, struct{}{}, &st),
			h.Call(abi.OpDLTrackers, abi.IDRequest{ID: "a"}, &tr),
		}
		if err := errors.Join(steps...); err != nil {
			t.Fatal(err)
		}
		if len(files.Files) != 1 || files.Files[0].Path != "a/f" || len(tr.Trackers) != 1 || tr.Trackers[0].URL != "udp://a" ||
			st.DownSpeed != 7 || lim != (abi.SpeedLimits{DownKiB: 5, UpKiB: 6}) || !alt.On {
			t.Errorf("results: %+v %+v %+v %+v %+v", files, tr, st, lim, alt)
		}
		want := []string{"files a", "level a high", "pause all", "resume all", "recheck a", "reannounce a", "set category a movies", "trackers a"}
		if !reflect.DeepEqual(f.calls, want) {
			t.Errorf("calls %v\nwant  %v", f.calls, want)
		}
	})
}
