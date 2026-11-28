package wasm

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// KindSpec describes how modules of one extension point plug in: the
// operations they must answer, the events they may emit and the adapter that
// makes a module look like a built-in plugin of that kind. Adding a kind
// means adding one entry here and its adapter.
type KindSpec struct {
	Kind     plugin.Kind
	Required []string
	Events   map[string]bool
	New      func(h *Handle) any
}

var kinds = map[string]KindSpec{
	abi.KindDownloader: {
		Kind:     plugin.KindDownloader,
		Required: abi.DownloaderRequired,
		Events:   map[string]bool{"download.progress": true, "download.state_changed": true},
		New:      func(h *Handle) any { return &Downloader{h: h} },
	},
}

// KindNames lists the kinds modules may implement.
func KindNames() []string {
	out := make([]string, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Spec returns the kind spec for a manifest kind.
func Spec(kind string) (KindSpec, bool) {
	s, ok := kinds[kind]
	return s, ok
}

// Handle is a plugin instance's link to its current pool. Replacing a module
// swaps the pool underneath without rebuilding the instance; the old pool
// drains in the background.
type Handle struct {
	pool atomic.Pointer[Pool]
	mu   sync.Mutex
	gone bool
	// onClose runs once when the instance is closed (the manager forgets it).
	onClose func(*Handle)
}

// SetOnClose sets what runs once when the instance is closed.
func (h *Handle) SetOnClose(fn func(*Handle)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onClose = fn
}

// NewHandle wraps a pool.
func NewHandle(p *Pool) *Handle {
	h := &Handle{}
	h.pool.Store(p)
	return h
}

// Adapter builds the kind's plugin implementation around the handle.
func (s KindSpec) Adapter(h *Handle) any { return s.New(h) }

// Pool is the current pool.
func (h *Handle) Pool() *Pool { return h.pool.Load() }

// Swap installs a new pool and returns the previous one; a closed instance
// refuses (ok is false) and the caller keeps p.
func (h *Handle) Swap(p *Pool) (old *Pool, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gone {
		return nil, false
	}
	return h.pool.Swap(p), true
}

// Call runs an operation on the current pool.
func (h *Handle) Call(ctx context.Context, op string, req, resp any) error {
	return h.pool.Load().Call(ctx, op, req, resp)
}

func (h *Handle) has(op string) bool { return h.pool.Load().m.Has(op) }

func (h *Handle) flag(f string) bool { return h.pool.Load().m.HasFlag(f) }

// drainTimeout bounds how long a closed instance waits for in-flight calls.
const drainTimeout = 15 * time.Second

// Close drains the instance's pool; the registry calls it when the instance
// is dropped.
func (h *Handle) Close() error {
	h.mu.Lock()
	if h.gone {
		h.mu.Unlock()
		return nil
	}
	h.gone = true
	fn := h.onClose
	h.mu.Unlock()
	if fn != nil {
		fn(h)
	}
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	h.pool.Load().Drain(ctx)
	return nil
}

// Test runs the module's self-test.
func (h *Handle) Test(ctx context.Context) error { return h.Call(ctx, abi.OpTest, struct{}{}, nil) }

// Handled exposes the handle behind an adapter.
type Handled interface{ ModuleHandle() *Handle }
