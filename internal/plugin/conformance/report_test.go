package conformance

import "testing"

func TestRunRecords(t *testing.T) {
	ok := Run("ok", func(t T) { t.Helper(); t.Cleanup(func() {}) })
	if !ok.Passed || ok.Name != "ok" {
		t.Errorf("%+v", ok)
	}
	bad := Run("bad", func(t T) {
		t.Error("one")
		t.Errorf("two %d", 2)
		t.Fatalf("three")
		t.Error("never")
	})
	if bad.Passed || len(bad.Problems) != 3 || bad.Problems[1] != "two 2" {
		t.Errorf("%+v", bad)
	}
	if p := Run("panic", func(T) { panic("boom") }); p.Passed || p.Problems[0] != "panic: boom" {
		t.Errorf("%+v", p)
	}
	if f := Run("fatal", func(t T) { t.Fatal("x") }); f.Passed || f.Problems[0] != "x" {
		t.Errorf("%+v", f)
	}
}
