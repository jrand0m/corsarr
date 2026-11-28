package wasm

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// pollGuest is a module with one page of exported memory and an exported
// poll(in, out, nsubs, nevents) that forwards to WASI poll_oneoff, the way a
// guest's runtime sleeps.
func pollGuest() []byte {
	sec := func(out []byte, id byte, body []byte) []byte {
		out = append(out, id)
		out = binary.AppendUvarint(out, uint64(len(body)))
		return append(out, body...)
	}
	str := func(out []byte, s string) []byte {
		out = binary.AppendUvarint(out, uint64(len(s)))
		return append(out, s...)
	}
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	out = sec(out, 1, []byte{1, 0x60, 4, 0x7f, 0x7f, 0x7f, 0x7f, 1, 0x7f})
	imp := str(str([]byte{1}, wasiModule), "poll_oneoff")
	out = sec(out, 2, append(imp, 0, 0))
	out = sec(out, 3, []byte{1, 0})
	out = sec(out, 5, []byte{1, 0, 1})
	exp := append(str([]byte{2}, "poll"), 0, 1)
	exp = append(str(exp, "memory"), 2, 0)
	out = sec(out, 7, exp)
	body := []byte{0, 0x20, 0, 0x20, 1, 0x20, 2, 0x20, 3, 0x10, 0, 0x0b}
	return sec(out, 10, append([]byte{1, byte(len(body))}, body...))
}

func newPollGuest(t *testing.T) api.Module {
	t.Helper()
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if err := instantiateWASI(ctx, rt); err != nil {
		t.Fatal(err)
	}
	mod, err := rt.Instantiate(ctx, pollGuest())
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func clockSub(userdata uint64, timeout time.Duration) []byte {
	sub := make([]byte, subSize)
	binary.LittleEndian.PutUint64(sub[0:], userdata)
	sub[8] = tagClock
	binary.LittleEndian.PutUint64(sub[24:], uint64(timeout))
	return sub
}

func fdSub(userdata uint64) []byte {
	sub := make([]byte, subSize)
	binary.LittleEndian.PutUint64(sub[0:], userdata)
	sub[8] = 1 // fd_read
	return sub
}

const (
	subsAt    = 1024
	eventsAt  = 4096
	neventsAt = 8192
	pageSize  = 65536
)

// poll writes subs into guest memory, calls poll_oneoff through the guest and
// returns the errno and how long the call took.
func poll(t *testing.T, mod api.Module, in, out, nsubs, nevents uint32, subs ...[]byte) (uint32, time.Duration) {
	t.Helper()
	at := in
	for _, s := range subs {
		if !mod.Memory().Write(at, s) {
			t.Fatalf("subscription at %d does not fit", at)
		}
		at += subSize
	}
	start := time.Now()
	res, err := mod.ExportedFunction("poll").Call(context.Background(), uint64(in), uint64(out), uint64(nsubs), uint64(nevents))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	return uint32(res[0]), time.Since(start)
}

type event struct {
	userdata uint64
	errno    uint16
	typ      byte
}

func pollResults(t *testing.T, mod api.Module, n uint32) []event {
	t.Helper()
	got, ok := mod.Memory().ReadUint32Le(neventsAt)
	if !ok || got != n {
		t.Fatalf("nevents = %d, want %d", got, n)
	}
	raw, _ := mod.Memory().Read(eventsAt, n*eventSize+eventSize)
	evs := make([]event, n)
	for i := range evs {
		ev := raw[i*eventSize:]
		evs[i] = event{binary.LittleEndian.Uint64(ev[0:]), binary.LittleEndian.Uint16(ev[8:]), ev[10]}
		for j := 11; j < eventSize; j++ {
			if ev[j] != 0 {
				t.Fatalf("event %d byte %d = %d, want 0", i, j, ev[j])
			}
		}
	}
	for j, b := range raw[n*eventSize:] {
		if b != 0 {
			t.Fatalf("byte %d after the last event = %d, want 0", j, b)
		}
	}
	return evs
}

func TestPollOneoffSleepsForAClock(t *testing.T) {
	mod := newPollGuest(t)
	errno, took := poll(t, mod, subsAt, eventsAt, 1, neventsAt, clockSub(0x0807060504030201, 60*time.Millisecond))
	if errno != errnoSuccess {
		t.Fatalf("errno = %d, want success", errno)
	}
	if took < 60*time.Millisecond {
		t.Fatalf("returned after %v, want a 60ms sleep", took)
	}
	evs := pollResults(t, mod, 1)
	if want := (event{0x0807060504030201, errnoSuccess, tagClock}); evs[0] != want {
		t.Fatalf("event = %+v, want %+v", evs[0], want)
	}
}

func TestPollOneoffAnswersEverySubscription(t *testing.T) {
	mod := newPollGuest(t)
	errno, took := poll(t, mod, subsAt, eventsAt, 3, neventsAt,
		clockSub(0x1112131415161718, time.Second), fdSub(0x2122232425262728), clockSub(0x3132333435363738, 2*time.Second))
	if errno != errnoSuccess {
		t.Fatalf("errno = %d, want success", errno)
	}
	if took > 500*time.Millisecond {
		t.Fatalf("took %v: a descriptor subscription must not wait for the clock", took)
	}
	want := []event{
		{0x1112131415161718, errnoSuccess, tagClock},
		{0x2122232425262728, errnoBadf, 1},
		{0x3132333435363738, errnoSuccess, tagClock},
	}
	for i, ev := range pollResults(t, mod, 3) {
		if ev != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, ev, want[i])
		}
	}
}

func TestPollOneoffDescriptorsOnlyDoNotSleep(t *testing.T) {
	mod := newPollGuest(t)
	errno, took := poll(t, mod, subsAt, eventsAt, 1, neventsAt, fdSub(7))
	if errno != errnoSuccess || took > 500*time.Millisecond {
		t.Fatalf("errno = %d after %v, want success at once", errno, took)
	}
	if evs := pollResults(t, mod, 1); evs[0] != (event{7, errnoBadf, 1}) {
		t.Fatalf("event = %+v", evs[0])
	}
}

func TestPollOneoffRejectsBadArguments(t *testing.T) {
	cases := map[string]struct {
		in, out, nsubs, nevents uint32
	}{
		"no subscriptions":               {subsAt, eventsAt, 0, neventsAt},
		"subscriptions past memory":      {pageSize - subSize, eventsAt, 2, neventsAt},
		"events past memory":             {subsAt, pageSize - eventSize, 2, neventsAt},
		"event count past memory":        {subsAt, eventsAt, 2, pageSize - 2},
		"subscriptions far past memory":  {pageSize, eventsAt, 1, neventsAt},
		"event count just past memory":   {subsAt, eventsAt, 1, pageSize},
		"events just past memory, 1 sub": {subsAt, pageSize, 1, neventsAt},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			mod := newPollGuest(t)
			for i := range uint32(2) {
				if c.in+i*subSize+subSize <= pageSize {
					mod.Memory().Write(c.in+i*subSize, fdSub(uint64(i+1)))
				}
			}
			if errno, _ := poll(t, mod, c.in, c.out, c.nsubs, c.nevents); errno != errnoInval {
				t.Fatalf("errno = %d, want EINVAL", errno)
			}
		})
	}
}
