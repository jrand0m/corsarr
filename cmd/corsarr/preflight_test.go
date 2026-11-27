package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServeRefusesUnwritableDataDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "data")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	var stderr bytes.Buffer
	c := parse([]string{"--data.dir=" + dir, "--server.address=127.0.0.1:0"}, []string{"HOME=" + t.TempDir()}, &bytes.Buffer{}, &stderr)
	if code := c.serve(context.Background(), nil); code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, stderr.String())
	}
	want := fmt.Sprintf("corsarr: data dir %s is not writable by uid %d gid %d: chown it or mount a writable volume\n",
		dir, os.Geteuid(), os.Getegid())
	if stderr.String() != want {
		t.Fatalf("stderr %q\nwant   %q", stderr.String(), want)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("wrote into the refused dir: %v", entries)
	}
}

func TestAllowRootIsACommandFlag(t *testing.T) {
	c := parse([]string{"serve", "--allow-root", "--data.dir=/x"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if !c.has("allow-root") || strings.Join(c.cfgArgs, " ") != "--data.dir=/x" {
		t.Fatalf("flags %v cfgArgs %v", c.flags, c.cfgArgs)
	}
}
