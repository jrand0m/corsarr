package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// callState is the per-call context host functions see.
type callState struct {
	pool *Pool

	mu      sync.Mutex
	pending []byte
	logs    int
	out     [3]bytes.Buffer // partial stdout/stderr lines
}

type stateKey struct{}

func withState(ctx context.Context, st *callState) context.Context {
	return context.WithValue(ctx, stateKey{}, st)
}

// stateOf never returns nil: host functions called outside a call (which the
// runtime does not do) get an inert state.
func stateOf(ctx context.Context) *callState {
	if st, ok := ctx.Value(stateKey{}).(*callState); ok {
		return st
	}
	return &callState{}
}

func (st *callState) now() time.Time {
	if st.pool == nil {
		return time.Now()
	}
	return st.pool.m.eng.o.Now()
}

// Per-call caps on host-function use.
const (
	maxLogLines   = 200
	maxLogMessage = 2048
	maxEventBytes = 16 << 10
)

func (st *callState) logger() *slog.Logger {
	if st.pool == nil {
		return slog.New(slog.DiscardHandler)
	}
	return st.pool.b.Log
}

// output collects stdout/stderr and logs complete lines.
func (st *callState) output(fd uint32, data []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	buf := &st.out[fd]
	buf.Write(data)
	for {
		line, err := buf.ReadBytes('\n')
		if err != nil {
			buf.Reset()
			if len(line) > maxLogMessage {
				line = line[:maxLogMessage]
			}
			buf.Write(line)
			return
		}
		st.logLine(slog.LevelInfo, string(bytes.TrimRight(line, "\n")), nil, "stream", map[uint32]string{1: "stdout", 2: "stderr"}[fd])
	}
}

func (st *callState) logLine(level slog.Level, msg string, attrs map[string]any, extra ...any) {
	if st.logs >= maxLogLines {
		return
	}
	st.logs++
	args := append([]any{}, extra...)
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		args = append(args, CleanText(k, 64), attrs[k])
	}
	st.logger().Log(context.Background(), level, CleanText(msg, maxLogMessage), args...)
}

// ── host functions ───────────────────────────────────────────────────────────

type handler func(ctx context.Context, st *callState, req []byte) (any, error)

// denied is a refused host call; the module sees class "denied".
type denied struct{ msg string }

func (d denied) Error() string { return d.msg }

func deny(format string, a ...any) error { return denied{fmt.Sprintf(format, a...)} }

var hostFuncs = map[string]handler{
	abi.HostLog:         hostLog,
	abi.HostHTTPDo:      hostHTTP,
	abi.HostSettingsGet: hostSettings,
	abi.HostSecretGet:   hostSecret,
	abi.HostNow:         hostNow,
	abi.HostSleep:       hostSleep,
	abi.HostEmitEvent:   hostEmit,
}

var hostSig = sig{i32s(2), []api.ValueType{api.ValueTypeI32}}

// allowedImport is the complete import surface of a module.
func allowedImport(module, name string) (sig, bool) {
	switch module {
	case abi.HostModule:
		if _, ok := hostFuncs[name]; ok || name == abi.HostTake {
			return hostSig, true
		}
	case wasiModule:
		if f, ok := wasiFuncs[name]; ok {
			return f.sig, true
		}
	}
	return sig{}, false
}

func instantiateHost(ctx context.Context, rt wazero.Runtime) error {
	if err := instantiateWASI(ctx, rt); err != nil {
		return err
	}
	b := rt.NewHostModuleBuilder(abi.HostModule)
	for name, h := range hostFuncs {
		b.NewFunctionBuilder().WithGoModuleFunction(hostCall(h), hostSig.params, hostSig.results).Export(name)
	}
	b.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(hostTake), hostSig.params, hostSig.results).Export(abi.HostTake)
	_, err := b.Instantiate(ctx)
	return err
}

// hostCall reads the request, runs the handler and parks the JSON envelope
// for take; it returns the envelope length (-1 when the request is unreadable).
func hostCall(h handler) api.GoModuleFunc {
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		st := stateOf(ctx)
		limit := 1 << 20
		if st.pool != nil {
			limit = st.pool.m.Limits.MaxMessageBytes
		}
		ptr, n := uint32(stack[0]), uint32(stack[1])
		req, ok := mod.Memory().Read(ptr, n)
		if !ok || int(n) > limit {
			stack[0] = api.EncodeI32(-1)
			return
		}
		res, err := h(ctx, st, append([]byte(nil), req...))
		env := envelope(res, err)
		st.mu.Lock()
		st.pending = env
		st.mu.Unlock()
		stack[0] = uint64(len(env))
	}
}

func envelope(res any, err error) []byte {
	var env abi.Envelope
	if err != nil {
		env.Error = &abi.Error{Class: errClass(err), Message: err.Error()}
	} else if res != nil {
		env.Result, _ = json.Marshal(res)
	}
	b, _ := json.Marshal(env)
	return b
}

func errClass(err error) string {
	var d denied
	var blocked httpx.ErrEgressBlocked
	if errors.As(err, &d) || errors.As(err, &blocked) {
		return abi.ClassDenied
	}
	if c := classOfHTTP(err); c != "" {
		return c
	}
	return abi.ClassServerError
}

func hostTake(ctx context.Context, mod api.Module, stack []uint64) {
	st := stateOf(ctx)
	st.mu.Lock()
	data := st.pending
	st.pending = nil
	st.mu.Unlock()
	n := min(uint32(len(data)), uint32(stack[1]))
	if !mod.Memory().Write(uint32(stack[0]), data[:n]) {
		stack[0] = api.EncodeI32(-1)
		return
	}
	stack[0] = uint64(n)
}

func decodeReq(req []byte, v any) error {
	if err := json.Unmarshal(req, v); err != nil {
		return deny("malformed request: %v", err)
	}
	return nil
}

var logLevels = map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}

func hostLog(_ context.Context, st *callState, req []byte) (any, error) {
	var r abi.LogRequest
	if err := decodeReq(req, &r); err != nil {
		return nil, err
	}
	level, ok := logLevels[r.Level]
	if !ok {
		level = slog.LevelInfo
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.logLine(level, r.Message, r.Attrs)
	return nil, nil
}

func hostSettings(_ context.Context, st *callState, _ []byte) (any, error) {
	if st.pool == nil {
		return abi.SettingsResponse{Values: map[string]any{}}, nil
	}
	return abi.SettingsResponse{Values: schema.ModuleValues(st.pool.m.Manifest.Settings, st.pool.b.Settings)}, nil
}

func hostSecret(_ context.Context, st *callState, req []byte) (any, error) {
	var r abi.SecretRequest
	if err := decodeReq(req, &r); err != nil {
		return nil, err
	}
	if st.pool == nil || !slices.Contains(st.pool.m.Manifest.Permissions.Secrets, r.Name) {
		return nil, deny("secret %q is not declared in permissions.secrets", r.Name)
	}
	v, _ := st.pool.b.Settings[r.Name].(string)
	return abi.SecretResponse{Value: v}, nil
}

func hostNow(_ context.Context, st *callState, _ []byte) (any, error) {
	return abi.NowResponse{UnixNano: st.now().UnixNano()}, nil
}

func hostSleep(ctx context.Context, _ *callState, req []byte) (any, error) {
	var r abi.SleepRequest
	if err := decodeReq(req, &r); err != nil {
		return nil, err
	}
	sleepCtx(ctx, time.Duration(max(r.Millis, 0))*time.Millisecond)
	if err := ctx.Err(); err != nil {
		return nil, deny("sleep interrupted by the call deadline")
	}
	return nil, nil
}

func hostEmit(_ context.Context, st *callState, req []byte) (any, error) {
	var r abi.EventRequest
	if len(req) > maxEventBytes {
		return nil, deny("event is larger than %d bytes", maxEventBytes)
	}
	if err := decodeReq(req, &r); err != nil {
		return nil, err
	}
	if st.pool == nil || !st.pool.m.spec.Events[r.Type] {
		return nil, deny("event type %q is not allowed for this kind", r.Type)
	}
	sink := st.pool.m.eng.sink()
	if sink == nil {
		return nil, nil
	}
	_, err := sink.Publish(domain.Event{Type: domain.EventType(r.Type), Actor: "plugin:" + st.pool.m.Manifest.ID, Data: r.Data})
	return nil, err
}
