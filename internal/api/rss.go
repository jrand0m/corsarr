package api

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// rssBytes is the resident set size: current RSS from /proc on Linux, the
// platform's own figure elsewhere (peak RSS on BSD/macOS, working set on
// Windows).
func rssBytes() int64 {
	if b, err := os.ReadFile("/proc/self/statm"); err == nil {
		return statmRSS(string(b), int64(os.Getpagesize()))
	}
	return rusageRSS(runtime.GOOS)
}

// statmRSS parses /proc/self/statm (second field = resident pages).
func statmRSS(statm string, pageSize int64) int64 {
	f := strings.Fields(statm)
	if len(f) < 2 {
		return 0
	}
	pages, _ := strconv.ParseInt(f[1], 10, 64)
	return pages * pageSize
}
