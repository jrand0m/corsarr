// Package preflight checks the process environment before corsarr opens
// anything: it refuses to run as root unless explicitly allowed, applies the
// file creation mask and verifies that the data and temp dirs are writable.
// It never changes ownership or permissions; when something is wrong the
// error names the uid, gid and path the deployer has to fix.
package preflight

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// Identity is the effective user and group of the process; both are -1 on
// platforms without numeric ids.
type Identity struct {
	UID, GID int
}

// Current returns the effective identity of this process.
func Current() Identity { return Identity{UID: os.Geteuid(), GID: os.Getegid()} }

func (id Identity) String() string {
	if id.UID < 0 {
		return "the current user"
	}
	return fmt.Sprintf("uid %d gid %d", id.UID, id.GID)
}

// Options is everything Run looks at.
type Options struct {
	ID        Identity
	DataDir   string
	TempDir   string
	Umask     int  // applied to every file and directory created from here on
	AllowRoot bool // the --allow-root flag
	Container bool // running inside a container (see InContainer)
}

// Run performs every check in order and returns a warning to log, or the
// first problem that must stop the start.
func Run(o Options) (warning string, err error) {
	warning, err = Root(o.ID, o.AllowRoot, o.Container)
	if err != nil {
		return "", err
	}
	SetUmask(o.Umask)
	if err := Writable("data dir", o.DataDir, o.ID, "chown it or mount a writable volume"); err != nil {
		return "", err
	}
	if err := Writable("temp dir", o.TempDir, o.ID, "mount a writable tmpfs there or set TMPDIR"); err != nil {
		return "", err
	}
	return warning, nil
}

// Root decides whether a process running as root may continue: only with
// --allow-root, and then with a warning.
func Root(id Identity, allowRoot, container bool) (warning string, err error) {
	switch {
	case id.UID != 0:
		return "", nil
	case allowRoot:
		return "running as root because --allow-root is set; a dedicated unprivileged user is safer", nil
	case container:
		return "", errors.New("refusing to run as root inside a container: start it with a non-root user (e.g. --user 65532:65532) or pass --allow-root")
	}
	return "", errors.New("refusing to run as root: run corsarr as a dedicated unprivileged user or pass --allow-root")
}

// Writable creates dir when it is missing (without changing anything that
// exists) and proves it accepts a new file. Permission and read-only file
// system errors become one actionable line: "<label> <dir> is not writable by
// uid N gid M: <hint>".
func Writable(label, dir string, id Identity, hint string) error {
	err := probe(dir)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS):
		return fmt.Errorf("%s %s is not writable by %s: %s", label, dir, id, hint)
	}
	return fmt.Errorf("%s %s is not writable: %w", label, dir, err)
}

func probe(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".corsarr-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	return errors.Join(f.Close(), os.Remove(name))
}

// containerMarkers are files container runtimes place in the root file system.
var containerMarkers = []string{"/.dockerenv", "/run/.containerenv"}

// InContainer reports whether the process looks containerised: a runtime
// marker file exists, or the runtime or orchestrator set its variable.
func InContainer(exists func(string) bool, getenv func(string) string) bool {
	for _, p := range containerMarkers {
		if exists(p) {
			return true
		}
	}
	return getenv("container") != "" || getenv("KUBERNETES_SERVICE_HOST") != ""
}

// FileExists is the exists function InContainer uses in production.
func FileExists(p string) bool { _, err := os.Stat(p); return err == nil }
