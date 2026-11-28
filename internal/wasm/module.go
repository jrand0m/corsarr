package wasm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Module is one verified, compiled module version.
type Module struct {
	Manifest abi.Manifest
	SHA256   string
	Limits   Limits
	Caps     abi.Capabilities
	// StartTime is how long the probe instance took to start.
	StartTime time.Duration

	eng      *Engine
	spec     KindSpec
	hosts    hostRules
	rt       wazero.Runtime
	compiled wazero.CompiledModule

	mu     sync.Mutex
	pools  map[*Pool]bool
	closed bool
}

// compileTimeout bounds the first compilation of a large module.
const compileTimeout = 2 * time.Minute

func (m *Module) compile(ctx context.Context, module []byte) error {
	m.rt = wazero.NewRuntimeWithConfig(ctx, m.eng.runtimeConfig(m.Limits.MemoryMiB))
	cctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	compiled, err := m.rt.CompileModule(cctx, module)
	if err != nil {
		_ = m.rt.Close(ctx)
		return refuse("not a valid WebAssembly module: %v", err)
	}
	m.compiled = compiled
	if err := checkLinks(compiled); err != nil {
		_ = m.rt.Close(ctx)
		return err
	}
	if err := instantiateHost(ctx, m.rt); err != nil {
		_ = m.rt.Close(ctx)
		return err
	}
	return nil
}

// checkLinks refuses any import beyond the host ABI and the WASI subset, and
// any module that lacks the ABI exports.
func checkLinks(c wazero.CompiledModule) error {
	if len(c.ImportedMemories()) > 0 {
		return refuse("imports a memory; modules must define their own")
	}
	for _, d := range c.ImportedFunctions() {
		mod, name, _ := d.Import()
		sig, ok := allowedImport(mod, name)
		if !ok {
			return refuse("imports %s.%s, which this corsarr does not provide", mod, name)
		}
		if !slices.Equal(sig.params, d.ParamTypes()) || !slices.Equal(sig.results, d.ResultTypes()) {
			return refuse("imports %s.%s with the wrong signature", mod, name)
		}
	}
	return checkExports(c.ExportedFunctions())
}

func checkExports(ex map[string]api.FunctionDefinition) error {
	if err := checkMarker(ex); err != nil {
		return err
	}
	if _, ok := ex[abi.ExportInit]; !ok {
		if _, cmd := ex[abi.ExportStart]; cmd {
			return refuse("built as a command; build it as a reactor with -buildmode=c-shared")
		}
		return refuse("missing the %s export", abi.ExportInit)
	}
	return checkSignatures(ex)
}

func checkMarker(ex map[string]api.FunctionDefinition) error {
	if _, ok := ex[abi.ExportMarker]; ok {
		return nil
	}
	for name := range ex {
		if v, found := strings.CutPrefix(name, abi.ExportMarkerBase); found {
			return refuse("module speaks ABI v%s, this corsarr supports v%d", v, abi.Version)
		}
	}
	return refuse("not a corsarr plugin module (no %s export)", abi.ExportMarker)
}

func checkSignatures(ex map[string]api.FunctionDefinition) error {
	want := map[string][2][]api.ValueType{
		abi.ExportAlloc: {{api.ValueTypeI32}, {api.ValueTypeI32}},
		abi.ExportFree:  {{api.ValueTypeI32}, nil},
		abi.ExportCall:  {{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, {api.ValueTypeI64}},
	}
	for name, sig := range want {
		d, ok := ex[name]
		if !ok || !slices.Equal(d.ParamTypes(), sig[0]) || !slices.Equal(d.ResultTypes(), sig[1]) {
			return refuse("missing or mistyped export %s", name)
		}
	}
	return nil
}

// probe starts one instance with no settings and asks for capabilities.
func (m *Module) probe(ctx context.Context) error {
	p := m.newPool(Binding{Key: "probe"})
	defer p.closeNow(ctx)
	start := time.Now()
	var caps abi.Capabilities
	if err := p.Call(ctx, abi.OpCapabilities, struct{}{}, &caps); err != nil {
		return refuse("capabilities call failed: %v", err)
	}
	m.StartTime = time.Since(start)
	if err := m.spec.checkCaps(m.Manifest, caps); err != nil {
		return err
	}
	m.Caps = caps
	return nil
}

func (s KindSpec) checkCaps(man abi.Manifest, caps abi.Capabilities) error {
	if caps.ABI != abi.Version {
		return refuse("module reports ABI v%d, this corsarr supports v%d", caps.ABI, abi.Version)
	}
	if caps.Kind != ManifestKind(man) {
		return refuse("module implements %q but its manifest says %q", caps.Kind, ManifestKind(man))
	}
	for _, op := range s.Required {
		if !slices.Contains(caps.Ops, op) {
			return refuse("module does not implement the required operation %s", op)
		}
	}
	return nil
}

// Has reports whether the module implements op.
func (m *Module) Has(op string) bool { return slices.Contains(m.Caps.Ops, op) }

// HasFlag reports whether the module declared a capability flag.
func (m *Module) HasFlag(flag string) bool { return slices.Contains(m.Caps.Flags, flag) }

// Kind is the module's extension point.
func (m *Module) Kind() string { return ManifestKind(m.Manifest) }

// NewPool returns the module-instance pool for one plugin instance.
func (m *Module) NewPool(b Binding) (*Pool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, refuse("module %s %s is unloaded", m.Manifest.ID, m.Manifest.Version)
	}
	p := m.newPoolLocked(b)
	return p, nil
}

func (m *Module) newPool(b Binding) *Pool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newPoolLocked(b)
}

func (m *Module) newPoolLocked(b Binding) *Pool {
	if m.pools == nil {
		m.pools = map[*Pool]bool{}
	}
	p := newPool(m, b)
	m.pools[p] = true
	return p
}

func (m *Module) dropPool(p *Pool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pools, p)
}

// Close drains every pool (in-flight calls finish or hit ctx) and frees the
// compiled code.
func (m *Module) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	pools := make([]*Pool, 0, len(m.pools))
	for p := range m.pools {
		pools = append(pools, p)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range pools {
		wg.Add(1)
		go func() { defer wg.Done(); p.Drain(ctx) }()
	}
	wg.Wait()
	return m.rt.Close(context.WithoutCancel(ctx))
}

// instantiate creates and initialises one module instance.
func (m *Module) instantiate(ctx context.Context, st *callState) (*guest, error) {
	cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions()
	mod, err := m.rt.InstantiateModule(withState(ctx, st), m.compiled, cfg)
	if err != nil {
		return nil, err
	}
	g := &guest{mod: mod, alloc: mod.ExportedFunction(abi.ExportAlloc), free: mod.ExportedFunction(abi.ExportFree),
		call: mod.ExportedFunction(abi.ExportCall)}
	if _, err := mod.ExportedFunction(abi.ExportInit).Call(withState(ctx, st)); err != nil {
		_ = mod.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return g, nil
}

// String names the module for logs.
func (m *Module) String() string { return m.Manifest.ID + "@" + m.Manifest.Version }

// Action runs a settings action on a short-lived pool bound to the form's
// current values (b.Settings), so the module tests what the owner typed
// before it is saved. The result text is cleaned to bounded plain text.
func (m *Module) Action(ctx context.Context, b Binding, id string) (abi.ActionResult, error) {
	var res abi.ActionResult
	if !schema.HasAction(m.Manifest.Settings, id) || !m.Has(abi.OpAction) {
		return res, fmt.Errorf("action %q: %w", id, plugin.ErrUnsupported)
	}
	if ps := schema.Validate(m.Manifest.Settings, b.Settings); len(ps) > 0 {
		return res, ps
	}
	p, err := m.NewPool(b)
	if err != nil {
		return res, err
	}
	defer p.closeNow(ctx)
	req := abi.ActionRequest{ID: id, Values: schema.ModuleValues(m.Manifest.Settings, b.Settings)}
	if err := p.Call(ctx, abi.OpAction, req, &res); err != nil {
		return res, err
	}
	if !slices.Contains([]string{abi.OutcomeOK, abi.OutcomeWarn, abi.OutcomeFail}, res.Outcome) {
		res.Outcome = abi.OutcomeFail
	}
	res.Text = CleanText(res.Text, 500)
	return res, nil
}
