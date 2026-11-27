package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Windows service mode:
//
//	corsarr service install [--account=…] [config args]   register corsarr with the Service Control Manager
//	corsarr service uninstall                              stop and remove the service (data is kept)
//	corsarr service start|stop|status                      control the installed service
//	corsarr service run                                    what the Service Control Manager starts
//
// On other systems every action prints that the mode is Windows-only; systemd
// and launchd units cover them.

const (
	serviceName        = "corsarr"
	serviceDisplayName = "corsarr"
	serviceDescription = "corsarr media automation server"
	// defaultServiceAccount is the per-service virtual account: no password, no
	// interactive logon, rights only where they are granted.
	defaultServiceAccount = `NT SERVICE\corsarr`
	serviceLogFile        = "corsarr.log"
	serviceLogMax         = 10 << 20 // rotate to corsarr.log.1 above this size at start
)

var serviceActions = []string{"install", "uninstall", "start", "stop", "status", "run"}

func init() {
	commandFlags["account"] = true
	for _, a := range serviceActions {
		commands["service "+a] = func(c cli, ctx context.Context) int { return c.service(ctx, a) }
	}
}

func serviceUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: corsarr service %s\n", strings.Join(serviceActions[:5], "|"))
	return 2
}

// servicePlan is what `service install` registers.
type servicePlan struct {
	Args    []string // arguments after the executable
	Account string
	Dirs    []string // directories the service account is given modify rights on
}

// planService builds the registration from the install invocation: config args
// given at install are kept verbatim for every start; without any, the data dir
// is <programData>\corsarr\data. The service account may always modify
// <programData>\corsarr and an explicit data dir.
func planService(programData, account string, cfgArgs []string) servicePlan {
	root := filepath.Join(programData, serviceName)
	p := servicePlan{Account: account, Dirs: []string{root}}
	if p.Account == "" {
		p.Account = defaultServiceAccount
	}
	p.Args = append([]string{"service", "run"}, cfgArgs...)
	if len(cfgArgs) == 0 {
		p.Args = append(p.Args, "--data.dir="+filepath.Join(root, "data"))
	}
	for _, a := range cfgArgs {
		if k, v, ok := strings.Cut(strings.TrimPrefix(a, "--"), "="); ok && strings.EqualFold(k, "data.dir") && v != root {
			p.Dirs = append(p.Dirs, v)
		}
	}
	return p
}

// underProfile reports whether exe lives in a user profile, where a service
// account usually cannot read it (per-user Scoop or winget installs).
func underProfile(exe, profile string) bool {
	if profile == "" {
		return false
	}
	rel, err := filepath.Rel(strings.ToLower(profile), strings.ToLower(exe))
	return err == nil && !strings.HasPrefix(rel, "..")
}

// logLevel classifies one JSON log line: 0 info, 1 warning, 2 error.
func logLevel(line string) int {
	switch {
	case strings.Contains(line, `"level":"ERROR"`):
		return 2
	case strings.Contains(line, `"level":"WARN"`):
		return 1
	}
	return 0
}

// lineWriter hands every complete line written to it to emit; a trailing
// partial line waits for the next write or Flush.
type lineWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	emit func(line string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(w.buf.Next(i + 1))
		if s := strings.TrimRight(line, "\r\n"); s != "" {
			w.emit(s)
		}
	}
}

// Flush emits a pending partial line.
func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s := strings.TrimRight(w.buf.String(), "\r\n"); s != "" {
		w.emit(s)
	}
	w.buf.Reset()
}

// openServiceLog opens <dir>\corsarr.log for appending, first moving a log
// above serviceLogMax to corsarr.log.1.
func openServiceLog(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, serviceLogFile)
	if st, err := os.Stat(p); err == nil && st.Size() > serviceLogMax {
		_ = os.Rename(p, p+".1")
	}
	return os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
}
