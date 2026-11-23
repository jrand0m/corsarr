//go:build windows

package api

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// rusageRSS is the current working set, the Windows counterpart of RSS.
func rusageRSS(string) int64 {
	var c processMemoryCounters
	c.cb = uint32(unsafe.Sizeof(c))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	if r == 0 {
		return 0
	}
	return int64(c.workingSetSize)
}
