package preflight

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRoot(t *testing.T) {
	cases := []struct {
		name             string
		id               Identity
		allow, container bool
		wantErr, warn    string
	}{
		{name: "non-root", id: Identity{UID: 1000, GID: 1000}},
		{name: "no numeric ids", id: Identity{UID: -1, GID: -1}},
		{name: "root native", id: Identity{}, wantErr: "refusing to run as root: run corsarr as a dedicated unprivileged user"},
		{name: "root container", id: Identity{}, container: true, wantErr: "inside a container"},
		{name: "root allowed", id: Identity{}, allow: true, warn: "--allow-root"},
		{name: "root allowed in container", id: Identity{}, allow: true, container: true, warn: "--allow-root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warn, err := Root(c.id, c.allow, c.container)
			if !matches(err, c.wantErr) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
			if !strings.Contains(warn, c.warn) || (c.warn == "") != (warn == "") {
				t.Fatalf("warning = %q, want %q", warn, c.warn)
			}
		})
	}
}

func matches(err error, want string) bool {
	if want == "" {
		return err == nil
	}
	return err != nil && strings.Contains(err.Error(), want)
}

func TestWritableCreatesMissingDirAndLeavesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := Writable("data dir", dir, Identity{UID: 1, GID: 2}, "hint"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("dir left %v (err %v)", entries, err)
	}
}

func TestWritableNamesIdentityAndPath(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := Writable("data dir", dir, Identity{UID: 12345, GID: 0}, "chown it or mount a writable volume")
	want := "data dir " + dir + " is not writable by uid 12345 gid 0: chown it or mount a writable volume"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v\nwant  %s", err, want)
	}
	// nothing was changed on the way: the mode is still read-only
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o555 {
		t.Fatalf("mode changed to %v", st.Mode().Perm())
	}
}

func TestWritableOtherErrorKeepsCause(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Writable("data dir", file, Identity{UID: 1, GID: 1}, "hint")
	if err == nil || !strings.Contains(err.Error(), "data dir "+file+" is not writable: ") {
		t.Fatalf("err = %v", err)
	}
}

func TestIdentityString(t *testing.T) {
	if got := (Identity{UID: -1, GID: -1}).String(); got != "the current user" {
		t.Fatal(got)
	}
	if got := (Identity{UID: 7, GID: 0}).String(); got != "uid 7 gid 0" {
		t.Fatal(got)
	}
}

func TestRunOrder(t *testing.T) {
	base := t.TempDir()
	o := Options{ID: Identity{UID: 1000, GID: 1000}, DataDir: filepath.Join(base, "data"), TempDir: filepath.Join(base, "tmp"), Umask: 0o022}
	old := SetUmask(0o022)
	t.Cleanup(func() { SetUmask(old) })
	if warn, err := Run(o); err != nil || warn != "" {
		t.Fatalf("Run = %q, %v", warn, err)
	}
	o.ID = Identity{}
	if _, err := Run(o); err == nil {
		t.Fatal("root accepted without --allow-root")
	}
	o.AllowRoot = true
	if warn, err := Run(o); err != nil || warn == "" {
		t.Fatalf("allowed root: %q, %v", warn, err)
	}
	file := filepath.Join(base, "file")
	_ = os.WriteFile(file, nil, 0o600)
	o.TempDir = file
	if _, err := Run(o); err == nil || !strings.Contains(err.Error(), "temp dir") {
		t.Fatalf("temp dir problem not reported: %v", err)
	}
	o.DataDir = file
	if _, err := Run(o); err == nil || !strings.Contains(err.Error(), "data dir") {
		t.Fatalf("data dir problem not reported first: %v", err)
	}
}

func TestUmaskApplies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no file creation mask")
	}
	old := SetUmask(0o002)
	t.Cleanup(func() { SetUmask(old) })
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(f); st.Mode().Perm() != 0o664 {
		t.Fatalf("mode %v, want 0664", st.Mode().Perm())
	}
	SetUmask(0o027)
	g := filepath.Join(t.TempDir(), "g")
	if err := os.WriteFile(g, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(g); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v, want 0640", st.Mode().Perm())
	}
}

func TestInContainer(t *testing.T) {
	none := func(string) bool { return false }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if InContainer(none, env(nil)) {
		t.Fatal("bare host detected as container")
	}
	if !InContainer(func(p string) bool { return p == "/run/.containerenv" }, env(nil)) {
		t.Fatal("podman marker missed")
	}
	if !InContainer(none, env(map[string]string{"KUBERNETES_SERVICE_HOST": "10.0.0.1"})) {
		t.Fatal("kubernetes missed")
	}
	if !InContainer(none, env(map[string]string{"container": "podman"})) {
		t.Fatal("container variable missed")
	}
	if FileExists(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("missing file exists")
	}
}
