// Command hostile is a test module that misbehaves on request: its test
// operation runs the attack named in the "attack" setting.
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
)

var (
	sink  []byte
	spins int
)

var attacks = map[string]func() error{
	"none": func() error { return nil },
	"host": func() error {
		_, err := plugin.Do(plugin.Request{Method: "GET", URL: "http://undeclared.example.net/"})
		return err
	},
	"declared": func() error {
		_, err := plugin.Do(plugin.Request{Method: "GET", URL: plugin.SettingString("target")})
		return err
	},
	"alloc": func() error {
		for range 64 {
			sink = append(sink, make([]byte, 16<<20)...)
		}
		return nil
	},
	"loop": func() error {
		for {
			spins++
		}
	},
	"sleep":  func() error { return plugin.Sleep(time.Hour) },
	"panic":  func() error { panic("handler bug") },
	"crash":  crash,
	"secret": func() error { _, err := plugin.Secret("other"); return err },
	"event":  func() error { return plugin.Emit("item.deleted", nil) },
	"emit":   func() error { return plugin.Emit("download.progress", map[string]any{"id": "x"}) },
	"spam": func() error {
		for i := range 1000 {
			plugin.Info("line", "n", i)
		}
		return nil
	},
	"stdout": func() error { fmt.Println("hello from stdout"); return nil },
	"clock": func() error {
		if plugin.Now().Year() < 2020 || time.Now().Year() < 2020 {
			return plugin.Errorf(abi.ClassServerError, "clock is wrong")
		}
		return nil
	},
}

func crash() error {
	done := make(chan struct{})
	go func() {
		var m map[string]int
		m["boom"] = 1
		close(done)
	}()
	<-done
	return nil
}

type dl struct{}

func (dl) Test() error {
	attack, ok := attacks[plugin.SettingString("attack")]
	if !ok {
		return plugin.Errorf(abi.ClassConfig, "unknown attack")
	}
	return attack()
}

func (dl) Protocols() []string { return []string{"torrent"} }

func (dl) Add(abi.AddRequest) (string, error) { return strings.Repeat("x", 2<<20), nil }

func (dl) List([]string) ([]abi.Snapshot, error) { return nil, nil }
func (dl) Remove(string, bool) error             { return nil }
func (dl) Pause(string) error                    { return nil }
func (dl) Resume(string) error                   { return nil }
func (dl) SetPriority(string, string) error      { return nil }
func (dl) EnsureCategory(string, string) error   { return nil }
func (dl) KeepsDataOnRemove() bool               { return true }
func (dl) Recheck(string) error                  { return plugin.Unsupported("recheck") }
func (dl) SetPriorityLevel(string, string) error { return plugin.NotFound("download") }

func init() { downloader.Serve(dl{}) }

func main() { plugin.Main(plugin.Manifest{}) }
