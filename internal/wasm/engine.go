// Package wasm runs plugin modules: WebAssembly (WASI preview 1, reactor
// build) loaded in-process by a pure-Go runtime. A module sees only the host
// ABI (log, http_do, settings_get, secret_get, now, sleep, emit_event) and a
// stub WASI layer with no filesystem, sockets, arguments or environment.
// Every plugin instance gets a small pool of module instances with a memory
// cap, a per-call deadline that interrupts a running call, size caps on every
// message and a concurrency cap; a trap or an exceeded limit fails only that
// call and recycles that module instance.
package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Limits are resource caps. Host limits are ceilings; a module's requested
// limits are clamped to them.
type Limits struct {
	MemoryMiB       int           // linear memory per module instance
	CallTimeout     time.Duration // wall time per call, interrupting a running module
	Concurrency     int           // parallel calls per plugin instance (module instances in the pool)
	MaxModuleBytes  int64         // module file size
	MaxMessageBytes int           // any single message or HTTP body crossing the boundary
}

// DefaultLimits are the host defaults.
func DefaultLimits() Limits {
	return Limits{MemoryMiB: 64, CallTimeout: 10 * time.Second, Concurrency: 4, MaxModuleBytes: 32 << 20, MaxMessageBytes: 8 << 20}
}

func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	return Limits{MemoryMiB: pos(l.MemoryMiB, d.MemoryMiB), CallTimeout: pos(l.CallTimeout, d.CallTimeout),
		Concurrency: pos(l.Concurrency, d.Concurrency), MaxModuleBytes: pos(l.MaxModuleBytes, d.MaxModuleBytes),
		MaxMessageBytes: pos(l.MaxMessageBytes, d.MaxMessageBytes)}
}

func pos[T int | int64 | time.Duration](v, def T) T {
	if v <= 0 {
		return def
	}
	return v
}

// clamp applies a module's requests below the host ceilings.
func (l Limits) clamp(req abi.Limits) Limits {
	out := l
	if req.MemoryMiB > 0 && req.MemoryMiB < l.MemoryMiB {
		out.MemoryMiB = req.MemoryMiB
	}
	if d := time.Duration(req.CallTimeoutMs) * time.Millisecond; d > 0 && d < l.CallTimeout {
		out.CallTimeout = d
	}
	if req.Concurrency > 0 && req.Concurrency < l.Concurrency {
		out.Concurrency = req.Concurrency
	}
	return out
}

// EventSink publishes events modules emit (events.Bus).
type EventSink interface {
	Publish(ev domain.Event) (domain.Event, error)
}

// Options configure an Engine.
type Options struct {
	Limits   Limits
	CacheDir string // compilation cache directory; "" keeps compiled code in memory only
	Log      *slog.Logger
	Events   EventSink
	Now      func() time.Time
	// Resolve looks up a host's addresses for the private-network check;
	// nil uses the system resolver.
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// Compiler selects the optimising compiler where the platform has one
	// (faster calls, slower first load); false uses the interpreter.
	Compiler bool
}

// Engine compiles and loads modules. It is safe for concurrent use.
type Engine struct {
	o      Options
	cache  wazero.CompilationCache
	log    atomic.Pointer[slog.Logger]
	events atomic.Pointer[sinkBox]
}

type sinkBox struct{ EventSink }

// SetLog replaces the logger module instances log through by default.
func (e *Engine) SetLog(l *slog.Logger) { e.log.Store(l) }

// SetEvents connects the event bus module events are published on.
func (e *Engine) SetEvents(s EventSink) { e.events.Store(&sinkBox{s}) }

func (e *Engine) logger() *slog.Logger { return e.log.Load() }

func (e *Engine) sink() EventSink {
	if b := e.events.Load(); b != nil {
		return b.EventSink
	}
	return nil
}

// NewEngine returns an engine; the compilation cache lives in o.CacheDir.
func NewEngine(o Options) (*Engine, error) {
	o.Limits = o.Limits.withDefaults()
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Resolve == nil {
		o.Resolve = systemResolve
	}
	cache := wazero.NewCompilationCache()
	if o.CacheDir != "" {
		c, err := wazero.NewCompilationCacheWithDir(o.CacheDir)
		if err != nil {
			return nil, fmt.Errorf("plugin compilation cache: %w", err)
		}
		cache = c
	}
	e := &Engine{o: o, cache: cache}
	e.SetLog(o.Log)
	if o.Events != nil {
		e.SetEvents(o.Events)
	}
	return e, nil
}

// Limits returns the host ceilings.
func (e *Engine) Limits() Limits { return e.o.Limits }

// Close releases the compilation cache.
func (e *Engine) Close(ctx context.Context) error { return e.cache.Close(ctx) }

// SHA256 is the hex digest modules are identified by.
func SHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadError is a refused module with the reason.
type LoadError struct{ Reason string }

func (e *LoadError) Error() string { return "module refused: " + e.Reason }

func refuse(format string, a ...any) error { return &LoadError{Reason: fmt.Sprintf(format, a...)} }

// Load verifies, compiles and probes a module: manifest, imports, exports,
// one start within the limits and a capabilities answer that matches the
// manifest's kind. sidecar is plugin.json when it is not embedded.
func (e *Engine) Load(ctx context.Context, module, sidecar []byte) (*Module, error) {
	if int64(len(module)) > e.o.Limits.MaxModuleBytes {
		return nil, refuse("module is %d bytes; the limit is %d", len(module), e.o.Limits.MaxModuleBytes)
	}
	raw, err := ManifestBytes(module, sidecar)
	if err != nil {
		return nil, err
	}
	man, err := ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	m := &Module{Manifest: man, SHA256: SHA256(module), Limits: e.o.Limits.clamp(man.Limits), eng: e,
		hosts: newHostRules(man.Permissions.HTTP.Hosts), spec: kinds[ManifestKind(man)]}
	if err := m.compile(ctx, module); err != nil {
		return nil, err
	}
	if err := m.probe(ctx); err != nil {
		_ = m.rt.Close(ctx)
		return nil, err
	}
	return m, nil
}

func (e *Engine) runtimeConfig(memMiB int) wazero.RuntimeConfig {
	cfg := wazero.NewRuntimeConfigInterpreter()
	if e.o.Compiler {
		cfg = wazero.NewRuntimeConfig()
	}
	return cfg.WithCompilationCache(e.cache).WithCloseOnContextDone(true).
		WithMemoryLimitPages(uint32(memMiB) * 16) // 64 KiB pages
}
