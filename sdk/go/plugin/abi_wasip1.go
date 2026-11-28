//go:build wasip1

package plugin

import "unsafe"

// Host imports. Each takes (ptr, len) of a JSON request and returns the
// length of the JSON answer, which take copies into a module buffer.

//go:wasmimport corsarr_v1 log
func importLog(ptr, n uint32) int32

//go:wasmimport corsarr_v1 http_do
func importHTTPDo(ptr, n uint32) int32

//go:wasmimport corsarr_v1 settings_get
func importSettingsGet(ptr, n uint32) int32

//go:wasmimport corsarr_v1 secret_get
func importSecretGet(ptr, n uint32) int32

//go:wasmimport corsarr_v1 now
func importNow(ptr, n uint32) int32

//go:wasmimport corsarr_v1 sleep
func importSleep(ptr, n uint32) int32

//go:wasmimport corsarr_v1 emit_event
func importEmitEvent(ptr, n uint32) int32

//go:wasmimport corsarr_v1 take
func importTake(ptr, n uint32) int32

var imports = map[string]func(ptr, n uint32) int32{
	"log": importLog, "http_do": importHTTPDo, "settings_get": importSettingsGet, "secret_get": importSecretGet,
	"now": importNow, "sleep": importSleep, "emit_event": importEmitEvent,
}

func addr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func rawHost(name string, req []byte) ([]byte, error) {
	fn := imports[name]
	n := fn(addr(req), uint32(len(req)))
	if n < 0 {
		return nil, Errorf("denied", "host refused the request to %s", name)
	}
	out := make([]byte, n)
	if n > 0 {
		importTake(addr(out), uint32(n))
	}
	return out, nil
}

// live keeps buffers handed to the host reachable until it frees them.
var live = map[uint32][]byte{}

func keep(b []byte) uint32 {
	if len(b) == 0 {
		b = make([]byte, 1)
	}
	p := addr(b)
	live[p] = b
	return p
}

//go:wasmexport corsarr_abi_v1
func abiMarker() {}

//go:wasmexport corsarr_alloc
func alloc(n uint32) uint32 { return keep(make([]byte, n)) }

//go:wasmexport corsarr_free
func free(p uint32) { delete(live, p) }

//go:wasmexport corsarr_call
func corsarrCall(opPtr, opLen, reqPtr, reqLen uint32) uint64 {
	buf := live[opPtr]
	if uint64(opLen)+uint64(reqLen) > uint64(len(buf)) || reqPtr != opPtr+opLen {
		out := envelope(nil, Errorf("parse", "request outside its buffer"))
		return uint64(keep(out))<<32 | uint64(len(out))
	}
	op := string(buf[:opLen])
	req := append([]byte(nil), buf[opLen:opLen+reqLen]...)
	out := dispatch(op, req)
	return uint64(keep(out))<<32 | uint64(len(out))
}

// Main does nothing inside the module; natively it prints or embeds the
// manifest.
func Main(Manifest) {}
