package indexers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// testTimeout bounds one connection test.
const testTimeout = 30 * time.Second

// TestResult is what a connection test found.
type TestResult struct {
	OK      bool         `json:"ok"`
	Class   string       `json:"class,omitempty"`
	Message string       `json:"message"`
	Caps    *plugin.Caps `json:"caps,omitempty"`
	Elapsed int64        `json:"elapsedMs"`
}

// Test checks an existing indexer and records the result in its health.
func (s *Service) Test(ctx context.Context, key domain.ObjectKey) (TestResult, error) {
	inst, ok := s.Merged(s.d.Config()).Instances[Section][key]
	if !ok {
		return TestResult{}, ErrNotFound
	}
	return s.test(ctx, inst, true), nil
}

// TestDraft checks unsaved settings without recording health.
func (s *Service) TestDraft(ctx context.Context, in Input) (TestResult, error) {
	reg, ok := s.d.Plugins.Get(in.Plugin)
	if !ok || reg.Manifest.Kind != plugin.KindIndexer {
		return TestResult{}, &InvalidError{Fields: []FieldError{{"plugin", "no indexer plugin " + in.Plugin + " in this build"}}}
	}
	var prev map[string]string
	if m, ok, _ := s.d.Repo.Get(ctx, in.Key); ok {
		prev = m.Secrets
	}
	vals, errs := Validate(reg.Manifest, in.Settings, prev)
	if len(errs) > 0 {
		return TestResult{}, &InvalidError{Fields: errs}
	}
	key := in.Key
	if key == "" {
		key = "draft"
	}
	inst := Instance(store.ManagedIndexer{Key: key, PluginID: in.Plugin, Enabled: true, Settings: vals.Settings, Secrets: vals.Secrets})
	return s.test(ctx, inst, false), nil
}

func (s *Service) test(ctx context.Context, inst config.Instance, record bool) TestResult {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	start := time.Now()
	res := s.run(ctx, inst, record)
	res.Elapsed = time.Since(start).Milliseconds()
	if record {
		o := health.Observation{OK: res.OK}
		if !res.OK {
			o = health.Observation{Class: res.Class, Message: res.Message}
		}
		s.d.Health.Report(ref(inst.Key), o)
	}
	return res
}

func (s *Service) run(ctx context.Context, inst config.Instance, record bool) (res TestResult) {
	defer func() {
		if p := recover(); p != nil { // a plugin bug fails its own test, never the process
			res = TestResult{Class: string(plugin.ClassServerError), Message: fmt.Sprint("plugin failed: ", p)}
		}
	}()
	ix, err := s.build(inst, record)
	if err == nil {
		err = ix.Test(ctx)
	}
	if err != nil {
		return failed(err)
	}
	caps, err := ix.Caps(ctx)
	if err != nil {
		return failed(err)
	}
	return TestResult{OK: true, Message: "connected", Caps: &caps}
}

func failed(err error) TestResult {
	class := string(plugin.ClassOf(err))
	if class == "" {
		class = string(plugin.ClassifyTransport(err))
		if errors.Is(err, context.Canceled) {
			class = string(plugin.ClassTimeout)
		}
	}
	return TestResult{Class: class, Message: err.Error()}
}

// build makes a throwaway instance of a plugin for a test. Saved instances use
// the client that reports their health; drafts use the plain client, so a
// typo never shows up as a failing indexer.
func (s *Service) build(inst config.Instance, record bool) (plugin.Indexer, error) {
	doer := s.d.Plain
	if record || doer == nil {
		doer = s.d.HTTP.For(ref(inst.Key))
	}
	reg, ok := s.d.Plugins.Get(inst.Type)
	if !ok {
		return nil, plugin.Errorf(plugin.ClassConfig, "no plugin %s in this build", inst.Type)
	}
	impl, err := reg.New(plugin.Instance{Key: inst.Key, Name: inst.Name, PluginID: inst.Type, Settings: inst.Settings,
		Env: env{http: doer, now: s.d.Now}})
	if err != nil {
		return nil, err
	}
	ix, ok := impl.(plugin.Indexer)
	if !ok {
		return nil, plugin.Errorf(plugin.ClassConfig, "plugin %s is not an indexer", inst.Type)
	}
	return ix, nil
}

type env struct {
	http plugin.HTTPDoer
	now  func() time.Time
}

func (e env) HTTP() plugin.HTTPDoer { return e.http }
func (e env) Now() time.Time        { return e.now() }
