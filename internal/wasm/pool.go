package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Binding is what one plugin instance gives its module: settings (secrets
// included; the module reads those only through secret_get), the shared
// outbound client scoped to the instance and a logger.
type Binding struct {
	Key      string
	Settings map[string]any
	HTTP     plugin.HTTPDoer
	Log      *slog.Logger
}

// degradeAfter consecutive limit or trap failures put a pool into a cool-down.
const (
	degradeAfter    = 3
	degradeCooldown = 30 * time.Second
	recycleAtShare  = 0.75 // recycle an instance whose memory passed this share of the cap
)

// Pool runs calls for one plugin instance on up to Limits.Concurrency module
// instances, created lazily. A wazero module instance is not safe for
// concurrent use, so each call borrows one instance exclusively.
type Pool struct {
	m   *Module
	b   Binding
	sem chan struct{}

	mu        sync.Mutex
	idle      []*guest
	closed    bool
	inflight  sync.WaitGroup
	failures  int
	coolUntil time.Time
}

func newPool(m *Module, b Binding) *Pool {
	if b.Log == nil {
		b.Log = m.eng.logger()
	}
	b.Log = b.Log.With("plugin", m.Manifest.ID, "instance", b.Key)
	return &Pool{m: m, b: b, sem: make(chan struct{}, m.Limits.Concurrency)}
}

// Module is the module version this pool runs.
func (p *Pool) Module() *Module { return p.m }

var errUnloaded = plugin.Errorf(plugin.ClassConfig, "plugin module is unloaded")

// Call runs op with req (marshalled to JSON) and decodes the result into resp
// (nil discards it). Module errors come back as classified plugin errors.
func (p *Pool) Call(ctx context.Context, op string, req, resp any) error {
	if err := p.enter(); err != nil {
		return err
	}
	defer p.inflight.Done()
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return plugin.Wrap(plugin.ClassTimeout, "waiting for a free module instance", ctx.Err())
	}
	defer func() { <-p.sem }()
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", op, err)
	}
	out, err := p.run(ctx, op, body)
	if err != nil {
		return err
	}
	return decodeEnvelope(out, resp)
}

func (p *Pool) enter() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errUnloaded
	}
	if now := p.m.eng.o.Now(); now.Before(p.coolUntil) {
		return plugin.Errorf(plugin.ClassServerError, "plugin module degraded after repeated failures; retrying after %s",
			p.coolUntil.Sub(now).Round(time.Second))
	}
	p.inflight.Add(1)
	return nil
}

// run executes one call on a borrowed instance under the call deadline.
func (p *Pool) run(ctx context.Context, op string, body []byte) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, p.m.Limits.CallTimeout)
	defer cancel()
	st := &callState{pool: p}
	g, err := p.borrow(cctx, st)
	if err != nil {
		return nil, p.fail(cctx, "start", err)
	}
	out, err := g.invoke(withState(cctx, st), op, body, p.m.Limits.MaxMessageBytes)
	if err != nil {
		_ = g.mod.Close(context.WithoutCancel(ctx))
		return nil, p.fail(cctx, op, err)
	}
	p.giveBack(ctx, g)
	return out, nil
}

func (p *Pool) borrow(ctx context.Context, st *callState) (*guest, error) {
	p.mu.Lock()
	if n := len(p.idle); n > 0 {
		g := p.idle[n-1]
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		return g, nil
	}
	p.mu.Unlock()
	return p.m.instantiate(ctx, st)
}

func (p *Pool) giveBack(ctx context.Context, g *guest) {
	p.mu.Lock()
	p.failures = 0
	keep := !p.closed && !g.mod.IsClosed() && float64(g.mod.Memory().Size()) < recycleAtShare*float64(p.m.Limits.MemoryMiB<<20)
	if keep {
		p.idle = append(p.idle, g)
	}
	p.mu.Unlock()
	if !keep {
		_ = g.mod.Close(context.WithoutCancel(ctx))
	}
}

// fail classifies a trap or limit and counts it toward degradation.
func (p *Pool) fail(ctx context.Context, op string, err error) error {
	out := classifyRun(ctx, op, err, p.m.Limits)
	if errors.Is(ctx.Err(), context.Canceled) {
		return out // the caller gave up; not the module's fault
	}
	p.mu.Lock()
	p.failures++
	if p.failures >= degradeAfter {
		p.coolUntil = p.m.eng.o.Now().Add(degradeCooldown)
		p.failures = 0
	}
	p.mu.Unlock()
	p.b.Log.Warn("plugin module call failed", "op", op, "err", out)
	return out
}

func classifyRun(ctx context.Context, op string, err error, l Limits) error {
	var exit *sys.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == sys.ExitCodeDeadlineExceeded,
		errors.Is(ctx.Err(), context.DeadlineExceeded):
		return plugin.Wrap(plugin.ClassTimeout, fmt.Sprintf("%s exceeded the %s call limit", op, l.CallTimeout), err)
	case errors.As(err, &exit) && exit.ExitCode() == sys.ExitCodeContextCanceled:
		return plugin.Wrap(plugin.ClassTimeout, op+" was cancelled", err)
	case errors.Is(err, errTooLarge):
		return plugin.Wrap(plugin.ClassParse, op+" answered more than the message size limit", err)
	case errors.Is(err, errNoMemory):
		return plugin.Wrap(plugin.ClassServerError, fmt.Sprintf("%s ran out of its %d MiB memory", op, l.MemoryMiB), err)
	}
	return plugin.Wrap(plugin.ClassServerError, op+" failed inside the module", err)
}

// Drain refuses new calls, waits for in-flight calls until ctx ends, then
// closes every instance (a call still running is interrupted).
func (p *Pool) Drain(ctx context.Context) {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	done := make(chan struct{})
	go func() { p.inflight.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	p.closeNow(ctx)
}

func (p *Pool) closeNow(ctx context.Context) {
	p.mu.Lock()
	p.closed = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, g := range idle {
		_ = g.mod.Close(context.WithoutCancel(ctx))
	}
	p.m.dropPool(p)
}

// Instances is the number of idle module instances (for status views).
func (p *Pool) Instances() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.idle)
}

// ── module errors ────────────────────────────────────────────────────────────

func decodeEnvelope(out []byte, resp any) error {
	var env abi.Envelope
	if err := json.Unmarshal(out, &env); err != nil {
		return plugin.Wrap(plugin.ClassParse, "module answered malformed JSON", err)
	}
	if env.Error != nil {
		return moduleError(env.Error)
	}
	if resp == nil || len(env.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Result, resp); err != nil {
		return plugin.Wrap(plugin.ClassParse, "module result does not match the operation", err)
	}
	return nil
}

var classes = map[string]plugin.FailureClass{
	abi.ClassAuth: plugin.ClassAuth, abi.ClassRateLimited: plugin.ClassRateLimited, abi.ClassTimeout: plugin.ClassTimeout,
	abi.ClassUnreachable: plugin.ClassUnreachable, abi.ClassParse: plugin.ClassParse, abi.ClassConfig: plugin.ClassConfig,
	abi.ClassPath: plugin.ClassPath, abi.ClassDenied: plugin.ClassConfig,
}

// moduleError maps a module error onto the host's errors; its message is
// cleaned to bounded plain text.
func moduleError(e *abi.Error) error {
	msg := CleanText(e.Message, 500)
	switch e.Class {
	case abi.ClassNotFound:
		return fmt.Errorf("%s: %w", msg, plugin.ErrNotFound)
	case abi.ClassUnsupported:
		return fmt.Errorf("%s: %w", msg, plugin.ErrUnsupported)
	}
	class, ok := classes[e.Class]
	if !ok {
		class = plugin.ClassServerError
	}
	return plugin.Errorf(class, "%s", msg)
}

// CleanText bounds module text and replaces control and format characters.
func CleanText(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "?") {
		if n == max {
			b.WriteString("…")
			break
		}
		if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zl, unicode.Zp) {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// ── one module instance ──────────────────────────────────────────────────────

type guest struct {
	mod               api.Module
	alloc, free, call api.Function
}

var (
	errTooLarge = errors.New("message too large")
	errNoMemory = errors.New("module could not allocate memory")
)

// invoke writes op and body into module memory, calls corsarr_call and copies
// the module's answer out.
func (g *guest) invoke(ctx context.Context, op string, body []byte, maxBytes int) ([]byte, error) {
	n := len(op) + len(body)
	if n > maxBytes {
		return nil, errTooLarge
	}
	res, err := g.alloc.Call(ctx, uint64(n))
	if err != nil {
		return nil, err
	}
	ptr := uint32(res[0])
	if ptr == 0 || !g.mod.Memory().Write(ptr, append([]byte(op), body...)) {
		return nil, errNoMemory
	}
	res, err = g.call.Call(ctx, uint64(ptr), uint64(len(op)), uint64(ptr)+uint64(len(op)), uint64(len(body)))
	if err != nil {
		return nil, err
	}
	if _, err := g.free.Call(ctx, uint64(ptr)); err != nil {
		return nil, err
	}
	return g.readResult(ctx, res[0], maxBytes)
}

func (g *guest) readResult(ctx context.Context, packed uint64, maxBytes int) ([]byte, error) {
	rp, rl := uint32(packed>>32), uint32(packed)
	if int(rl) > maxBytes {
		return nil, errTooLarge
	}
	data, ok := g.mod.Memory().Read(rp, rl)
	if !ok {
		return nil, fmt.Errorf("module answered outside its memory")
	}
	out := append([]byte(nil), data...)
	if _, err := g.free.Call(ctx, uint64(rp)); err != nil {
		return nil, err
	}
	return out, nil
}
