package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestServiceCommandOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service mode is real on Windows")
	}
	for _, a := range serviceActions {
		code, _, errs := cliRun(t, nil, "service", a)
		if code != 2 || !strings.Contains(errs, "service mode is only available on Windows") {
			t.Fatalf("%s: %d %q", a, code, errs)
		}
	}
	if code, _, errs := cliRun(t, nil, "service"); code != 2 || !strings.Contains(errs, "usage: corsarr service install|") {
		t.Fatalf("%d %q", code, errs)
	}
	if code, _, errs := cliRun(t, nil, "service", "install", "--account=LocalService"); code != 2 || strings.Contains(errs, "unknown command") {
		t.Fatalf("--account must be a command flag: %d %q", code, errs)
	}
}

func TestPlanService(t *testing.T) {
	pd := filepath.Join("C", "ProgramData")
	root := filepath.Join(pd, "corsarr")
	got := planService(pd, "", nil)
	want := servicePlan{
		Args:    []string{"service", "run", "--data.dir=" + filepath.Join(root, "data")},
		Account: defaultServiceAccount, Dirs: []string{root},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default plan %+v", got)
	}
	got = planService(pd, "NT AUTHORITY\\LocalService", []string{"--configFile=" + filepath.Join(root, "corsarr.yml"), "--data.dir=D:\\corsarr"})
	if got.Account != "NT AUTHORITY\\LocalService" || len(got.Args) != 4 || got.Args[2] != "--configFile="+filepath.Join(root, "corsarr.yml") {
		t.Fatalf("explicit args %+v", got)
	}
	if !reflect.DeepEqual(got.Dirs, []string{root, "D:\\corsarr"}) {
		t.Fatalf("dirs %v", got.Dirs)
	}
	if got := planService(pd, "", []string{"--data.dir=" + root}); len(got.Dirs) != 1 {
		t.Fatalf("data dir equal to the root is granted once: %v", got.Dirs)
	}
}

func TestUnderProfile(t *testing.T) {
	home := filepath.Join("Users", "me")
	for exe, want := range map[string]bool{
		filepath.Join("users", "ME", "scoop", "corsarr.exe"): true,
		filepath.Join("Program Files", "corsarr.exe"):        false,
	} {
		if underProfile(exe, home) != want {
			t.Fatalf("%s: want %v", exe, want)
		}
	}
	if underProfile("x", "") {
		t.Fatal("no profile, no warning")
	}
}

func TestLogLevel(t *testing.T) {
	for line, want := range map[string]int{
		`{"level":"ERROR","msg":"x"}`: 2, `{"level":"WARN","msg":"x"}`: 1,
		`{"level":"INFO","msg":"x"}`: 0, "plain": 0,
	} {
		if logLevel(line) != want {
			t.Fatalf("%s: want %d", line, want)
		}
	}
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := &lineWriter{emit: func(s string) { got = append(got, s) }}
	_, _ = w.Write([]byte("one\r\ntw"))
	_, _ = w.Write([]byte("o\n\nthr"))
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("%q", got)
	}
	w.Flush()
	w.Flush()
	if !reflect.DeepEqual(got, []string{"one", "two", "thr"}) {
		t.Fatalf("%q", got)
	}
}

func TestOpenServiceLogRotates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	f, err := openServiceLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(strings.Repeat("x", serviceLogMax+1))
	_ = f.Close()
	f, err = openServiceLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if st, err := os.Stat(filepath.Join(dir, serviceLogFile+".1")); err != nil || st.Size() <= serviceLogMax {
		t.Fatalf("rotated log: %v", err)
	}
	if st, _ := os.Stat(filepath.Join(dir, serviceLogFile)); st.Size() != 0 {
		t.Fatal("new log not empty")
	}
	if _, err := openServiceLog(filepath.Join(dir, serviceLogFile, "sub")); err == nil {
		t.Fatal("a file in the way must fail")
	}
}
