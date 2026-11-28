package plugin

import (
	"context"
	"reflect"
	"testing"
)

func TestReplaceAndUnregister(t *testing.T) {
	r := NewRegistry()
	reg := Registration{Manifest: Manifest{ID: "m", Kind: KindDownloader, APIVersion: APIVersion, Version: "1"},
		New: func(Instance) (any, error) { return nil, nil }}
	if err := r.Replace(reg); err != nil {
		t.Fatal(err)
	}
	reg.Manifest.Version = "2"
	if err := r.Replace(reg); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get("m"); got.Manifest.Version != "2" {
		t.Errorf("version %s", got.Manifest.Version)
	}
	if err := r.Replace(Registration{}); err == nil {
		t.Error("empty registration accepted")
	}
	if old, ok := r.Unregister("m"); !ok || old.Manifest.Version != "2" {
		t.Error("unregister")
	}
	if _, ok := r.Unregister("m"); ok {
		t.Error("unregistered twice")
	}
}

type lister struct{}

func (lister) Files(context.Context, string) ([]FileEntry, error) { return nil, nil }
func (lister) Recheck(context.Context, string) error              { return nil }

type gated struct{ lister }

func (gated) Supports(t reflect.Type) bool { return t == reflect.TypeFor[FileLister]() }

func TestAs(t *testing.T) {
	if _, ok := As[FileLister](lister{}); !ok {
		t.Error("plain implementation")
	}
	if _, ok := As[Rechecker](lister{}); !ok {
		t.Error("plain implementation")
	}
	if _, ok := As[GlobalPauser](lister{}); ok {
		t.Error("missing method")
	}
	if _, ok := As[FileLister](gated{}); !ok {
		t.Error("supported capability")
	}
	if _, ok := As[Rechecker](gated{}); ok {
		t.Error("gate ignored")
	}
}
