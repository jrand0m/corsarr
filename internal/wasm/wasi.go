package wasm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

// wasiModule is the WASI preview 1 import module. corsarr implements only what
// Go's wasip1 runtime touches, with no filesystem, sockets, arguments or
// environment: clocks and randomness come from the host, stdout and stderr go
// to the plugin log, sleeping honours the call deadline, and every other
// function answers "not supported".
const wasiModule = "wasi_snapshot_preview1"

// WASI errno values.
const (
	errnoSuccess = 0
	errnoBadf    = 8
	errnoInval   = 28
	errnoNosys   = 52
)

type sig struct{ params, results []api.ValueType }

type wasiFunc struct {
	sig
	fn api.GoModuleFunc
}

func i32s(n int) []api.ValueType {
	out := make([]api.ValueType, n)
	for i := range out {
		out[i] = api.ValueTypeI32
	}
	return out
}

var errnoResult = []api.ValueType{api.ValueTypeI32}

func wf(params int, fn api.GoModuleFunc) wasiFunc {
	return wasiFunc{sig{i32s(params), errnoResult}, fn}
}

func nosys(_ context.Context, _ api.Module, stack []uint64) { stack[0] = errnoNosys }

func badf(_ context.Context, _ api.Module, stack []uint64) { stack[0] = errnoBadf }

// wasiFuncs is the complete WASI surface a module may import.
var wasiFuncs = map[string]wasiFunc{
	"args_sizes_get":      wf(2, zeroSizes),
	"args_get":            wf(2, success),
	"environ_sizes_get":   wf(2, zeroSizes),
	"environ_get":         wf(2, success),
	"clock_time_get":      {sig{[]api.ValueType{api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeI32}, errnoResult}, clockTimeGet},
	"clock_res_get":       wf(2, clockResGet),
	"random_get":          wf(2, randomGet),
	"fd_write":            wf(4, fdWrite),
	"fd_read":             wf(4, badf),
	"fd_close":            wf(1, badf),
	"fd_fdstat_get":       wf(2, fdFdstatGet),
	"fd_fdstat_set_flags": wf(2, stdioOnly),
	"fd_prestat_get":      wf(2, badf),
	"fd_prestat_dir_name": wf(3, badf),
	"fd_seek":             {sig{[]api.ValueType{api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeI32, api.ValueTypeI32}, errnoResult}, badf},
	"fd_filestat_get":     wf(2, badf),
	"fd_sync":             wf(1, badf),
	"fd_readdir":          {sig{[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeI32}, errnoResult}, badf},
	"path_open": {sig{[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32,
		api.ValueTypeI64, api.ValueTypeI64, api.ValueTypeI32, api.ValueTypeI32}, errnoResult}, nosys},
	"path_filestat_get": wf(5, nosys),
	"poll_oneoff":       wf(4, pollOneoff),
	"sched_yield":       wf(0, success),
	"proc_exit":         {sig{i32s(1), nil}, procExit},
}

func success(_ context.Context, _ api.Module, stack []uint64) { stack[0] = errnoSuccess }

func zeroSizes(_ context.Context, mod api.Module, stack []uint64) {
	mem := mod.Memory()
	if !mem.WriteUint32Le(uint32(stack[0]), 0) || !mem.WriteUint32Le(uint32(stack[1]), 0) {
		stack[0] = errnoInval
		return
	}
	stack[0] = errnoSuccess
}

func clockTimeGet(ctx context.Context, mod api.Module, stack []uint64) {
	st := stateOf(ctx)
	var ns uint64
	switch uint32(stack[0]) {
	case 0: // realtime
		ns = uint64(st.now().UnixNano())
	default: // monotonic and CPU clocks: time since process start, never backwards
		ns = uint64(time.Since(processStart).Nanoseconds())
	}
	if !mod.Memory().WriteUint64Le(uint32(stack[2]), ns) {
		stack[0] = errnoInval
		return
	}
	stack[0] = errnoSuccess
}

var processStart = time.Now()

func clockResGet(_ context.Context, mod api.Module, stack []uint64) {
	if !mod.Memory().WriteUint64Le(uint32(stack[1]), 1000) {
		stack[0] = errnoInval
		return
	}
	stack[0] = errnoSuccess
}

func randomGet(_ context.Context, mod api.Module, stack []uint64) {
	buf, ok := mod.Memory().Read(uint32(stack[0]), uint32(stack[1]))
	if !ok {
		stack[0] = errnoInval
		return
	}
	_, _ = rand.Read(buf)
	stack[0] = errnoSuccess
}

// fdWrite sends stdout/stderr to the plugin log; other descriptors do not exist.
func fdWrite(ctx context.Context, mod api.Module, stack []uint64) {
	fd, iovs, n, written := uint32(stack[0]), uint32(stack[1]), uint32(stack[2]), uint32(stack[3])
	if fd != 1 && fd != 2 {
		stack[0] = errnoBadf
		return
	}
	mem := mod.Memory()
	total := uint32(0)
	for i := range n {
		data, ok := iov(mem, iovs+i*8)
		if !ok {
			stack[0] = errnoInval
			return
		}
		stateOf(ctx).output(fd, data)
		total += uint32(len(data))
	}
	stack[0] = errnoOf(mem.WriteUint32Le(written, total))
}

// iov reads one {ptr, len} I/O vector.
func iov(mem api.Memory, at uint32) ([]byte, bool) {
	ptr, ok1 := mem.ReadUint32Le(at)
	size, ok2 := mem.ReadUint32Le(at + 4)
	if !ok1 || !ok2 {
		return nil, false
	}
	return mem.Read(ptr, size)
}

func errnoOf(ok bool) uint64 {
	if ok {
		return errnoSuccess
	}
	return errnoInval
}

func fdFdstatGet(_ context.Context, mod api.Module, stack []uint64) {
	if uint32(stack[0]) > 2 {
		stack[0] = errnoBadf
		return
	}
	var stat [24]byte
	stat[0] = 2 // character device
	if !mod.Memory().Write(uint32(stack[1]), stat[:]) {
		stack[0] = errnoInval
		return
	}
	stack[0] = errnoSuccess
}

func stdioOnly(_ context.Context, _ api.Module, stack []uint64) {
	if uint32(stack[0]) > 2 {
		stack[0] = errnoBadf
		return
	}
	stack[0] = errnoSuccess
}

// Subscription and event layouts of poll_oneoff.
const (
	subSize      = 48
	eventSize    = 32
	tagClock     = 0
	flagAbstime  = 1
	maxPollSleep = time.Second
)

// pollOneoff implements sleeping: it waits for the earliest clock
// subscription (bounded by the call deadline) and fails descriptor
// subscriptions at once.
func pollOneoff(ctx context.Context, mod api.Module, stack []uint64) {
	in, out, nsubs, nevents := uint32(stack[0]), uint32(stack[1]), uint32(stack[2]), uint32(stack[3])
	mem := mod.Memory()
	subs, ok := mem.Read(in, nsubs*subSize)
	if !ok || nsubs == 0 {
		stack[0] = errnoInval
		return
	}
	wait, hasClock := earliest(subs, nsubs, stateOf(ctx).now())
	if hasClock && !hasFD(subs, nsubs) {
		sleepCtx(ctx, min(wait, maxPollSleep))
	}
	events := make([]byte, 0, nsubs*eventSize)
	for i := range nsubs {
		sub := subs[i*subSize : (i+1)*subSize]
		ev := make([]byte, eventSize)
		copy(ev[0:8], sub[0:8]) // userdata
		ev[10] = sub[8]         // type
		if sub[8] != tagClock {
			binary.LittleEndian.PutUint16(ev[8:], errnoBadf)
		}
		events = append(events, ev...)
	}
	if !mem.Write(out, events) || !mem.WriteUint32Le(nevents, nsubs) {
		stack[0] = errnoInval
		return
	}
	stack[0] = errnoSuccess
}

func earliest(subs []byte, n uint32, now time.Time) (time.Duration, bool) {
	best, found := time.Duration(1<<62), false
	for i := range n {
		sub := subs[i*subSize : (i+1)*subSize]
		if sub[8] != tagClock {
			continue
		}
		timeout := time.Duration(binary.LittleEndian.Uint64(sub[24:32]))
		if binary.LittleEndian.Uint16(sub[40:42])&flagAbstime != 0 {
			timeout = time.Duration(int64(timeout) - now.UnixNano())
		}
		best, found = min(best, max(timeout, 0)), true
	}
	return best, found
}

func hasFD(subs []byte, n uint32) bool {
	for i := range n {
		if subs[i*subSize+8] != tagClock {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

func procExit(ctx context.Context, mod api.Module, stack []uint64) {
	code := uint32(stack[0])
	_ = mod.CloseWithExitCode(ctx, code)
	panic(sys.NewExitError(code))
}

func instantiateWASI(ctx context.Context, rt wazero.Runtime) error {
	b := rt.NewHostModuleBuilder(wasiModule)
	for name, f := range wasiFuncs {
		b.NewFunctionBuilder().WithGoModuleFunction(f.fn, f.params, f.results).Export(name)
	}
	_, err := b.Instantiate(ctx)
	return err
}
