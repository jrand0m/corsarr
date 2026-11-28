// Command example-downloader is a complete download-client plugin module for
// the small JSON API in ./fakeapi. Build it with
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o example.wasm ./plugins/example-downloader
//	go run ./plugins/example-downloader -embed example.wasm -o example.wasm
//
// and check it with `corsarr plugin test example.wasm`.
package main

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
)

// manifest is plugin.json.
var manifest = plugin.Manifest{
	ID: "exampledl", Name: "Example download client", Version: "1.0.0", Kind: abi.KindDownloader, ABIVersion: abi.Version,
	Description: "Talks to the example JSON download API ⬇\uFE0F. A starting point for your own client.",
	Author:      "corsarr", License: "MIT",
	Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:url"}}, Secrets: []string{"apiKey"}},
	Limits:      abi.Limits{MemoryMiB: 32, CallTimeoutMs: 10000, Concurrency: 2},
	Settings: func() abi.Schema {
		s := plugin.Schema(
			plugin.String("url", "Address").Pattern(abi.PatternURLHTTP).Required().
				Describe("Where the client's API answers, for example http://10.0.0.10:8080."),
			plugin.SecretField("apiKey", "API key 🔑").Describe("Sent as X-Api-Key; leave empty when the client has none."),
			plugin.Duration("timeout", "Request timeout").Between("1s", "60s").Default("10s"),
			plugin.Map("pathMap", "Path mapping").Describe("Client path = corsarr path, for clients on another machine.").
				Entries(10, abi.TextRule{Pattern: abi.PatternPathAbsolute}, abi.TextRule{Pattern: abi.PatternPathAbsolute}),
		)
		s.Groups = []abi.Group{plugin.Group("Connection", "url", "apiKey", "timeout"), plugin.Group("Paths", "pathMap")}
		s.Actions = []abi.Action{plugin.Action("test", "Test connection ✅")}
		return s
	}(),
}

// client implements the downloader core and every optional capability.
type client struct{}

func api() (plugin.Client, error) {
	key, err := plugin.Secret("apiKey")
	if err != nil {
		return plugin.Client{}, err
	}
	c := plugin.Client{Base: plugin.SettingString("url")}
	if c.Base == "" {
		return c, plugin.Errorf(abi.ClassConfig, "the address is not set")
	}
	if key != "" {
		c.Headers = map[string][]string{"X-Api-Key": {key}}
	}
	return c, nil
}

func do(method, path string, query url.Values, body, out any) error {
	c, err := api()
	if err != nil {
		return err
	}
	return c.JSON(method, path, query, body, out)
}

func (client) Test() error { return do("GET", "/api/v1/version", nil, nil, nil) }

func (client) Protocols() []string { return []string{"torrent"} }

func (client) Add(r abi.AddRequest) (string, error) {
	source := r.Payload.URL
	if len(r.Payload.Content) > 0 {
		source = base64.StdEncoding.EncodeToString(r.Payload.Content)
	}
	var out struct {
		ID string `json:"id"`
	}
	err := do("POST", "/api/v1/downloads", nil, map[string]any{"source": source, "name": r.Payload.Name,
		"category": r.Category, "savePath": r.SavePath, "paused": r.Paused}, &out)
	return out.ID, err
}

type download struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	SavePath string `json:"savePath"`
	State    string `json:"state"`
	Size     int64  `json:"size"`
	Done     int64  `json:"done"`
}

var states = map[string]string{"queued": "queued", "downloading": "downloading", "paused": "queued", "done": "completed"}

func (client) List(categories []string) ([]abi.Snapshot, error) {
	var list []download
	if err := do("GET", "/api/v1/downloads", url.Values{"category": categories}, nil, &list); err != nil {
		return nil, err
	}
	out := make([]abi.Snapshot, 0, len(list))
	for _, d := range list {
		progress := 0.0
		if d.Size > 0 {
			progress = float64(d.Done) / float64(d.Size)
		}
		out = append(out, abi.Snapshot{ID: d.ID, Name: d.Name, Category: d.Category, State: states[d.State],
			Progress: progress, Size: d.Size, Downloaded: d.Done, SavePath: d.SavePath, LastActivityAt: plugin.Now().Unix()})
	}
	return out, nil
}

func (client) Remove(id string, deleteFiles bool) error {
	return do("DELETE", "/api/v1/downloads/"+url.PathEscape(id), url.Values{"deleteFiles": {strconv.FormatBool(deleteFiles)}}, nil, nil)
}

func action(id, name string, body any) error {
	return do("POST", "/api/v1/downloads/"+url.PathEscape(id)+"/"+name, nil, body, nil)
}

func (client) Pause(id string) error  { return action(id, "pause", nil) }
func (client) Resume(id string) error { return action(id, "resume", nil) }
func (client) SetPriority(id, move string) error {
	return action(id, "priority", map[string]string{"move": move})
}

func (client) EnsureCategory(category, savePath string) error {
	return do("PUT", "/api/v1/categories/"+url.PathEscape(category), nil, map[string]string{"savePath": savePath}, nil)
}

func (client) Files(id string) ([]abi.File, error) {
	var files []abi.File
	err := do("GET", "/api/v1/downloads/"+url.PathEscape(id)+"/files", nil, nil, &files)
	return files, err
}

func (client) SetPriorityLevel(id, level string) error {
	return action(id, "level", map[string]string{"level": level})
}

func (client) PauseAll() error  { return do("POST", "/api/v1/pause-all", nil, nil, nil) }
func (client) ResumeAll() error { return do("POST", "/api/v1/resume-all", nil, nil, nil) }

type limits struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
	Alt  bool  `json:"alt"`
}

func getLimits() (limits, error) {
	var l limits
	err := do("GET", "/api/v1/limits", nil, nil, &l)
	return l, err
}

func (client) SpeedLimits() (abi.SpeedLimits, error) {
	l, err := getLimits()
	return abi.SpeedLimits{DownKiB: l.Down, UpKiB: l.Up}, err
}

func (client) SetSpeedLimits(s abi.SpeedLimits) error {
	l, err := getLimits()
	if err != nil {
		return err
	}
	l.Down, l.Up = s.DownKiB, s.UpKiB
	return do("PUT", "/api/v1/limits", nil, l, nil)
}

func (client) AltSpeed() (bool, error) {
	l, err := getLimits()
	return l.Alt, err
}

func (client) SetAltSpeed(on bool) error {
	l, err := getLimits()
	if err != nil {
		return err
	}
	l.Alt = on
	return do("PUT", "/api/v1/limits", nil, l, nil)
}

func (client) Recheck(id string) error    { return action(id, "recheck", nil) }
func (client) Reannounce(id string) error { return action(id, "reannounce", nil) }
func (client) SetCategory(id, category string) error {
	return action(id, "category", map[string]string{"category": category})
}

func (client) Stats() (abi.Stats, error) {
	var s struct {
		Down   int64  `json:"down"`
		Up     int64  `json:"up"`
		Free   int64  `json:"free"`
		Paused bool   `json:"paused"`
		Limits limits `json:"limits"`
	}
	if err := do("GET", "/api/v1/stats", nil, nil, &s); err != nil {
		return abi.Stats{}, err
	}
	return abi.Stats{DownSpeed: s.Down, UpSpeed: s.Up, FreeSpace: s.Free, Paused: s.Paused,
		Limits: abi.SpeedLimits{DownKiB: s.Limits.Down, UpKiB: s.Limits.Up}, AltSpeed: s.Limits.Alt}, nil
}

func (client) Trackers(id string) ([]abi.Tracker, error) {
	var list []abi.Tracker
	err := do("GET", "/api/v1/downloads/"+url.PathEscape(id)+"/trackers", nil, nil, &list)
	return list, err
}

func init() {
	downloader.Serve(client{})
	plugin.OnAction(func(id string, _ map[string]any) (string, string) {
		start := time.Now()
		if err := (client{}).Test(); err != nil {
			return abi.OutcomeFail, "Could not reach the client: " + err.Error()
		}
		return abi.OutcomeOK, "Connected in " + time.Since(start).Round(time.Millisecond).String() + " ✅"
	})
}

func main() { plugin.Main(manifest) }
