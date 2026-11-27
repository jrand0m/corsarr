package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyKeyAndReadingTheKeyBesideALiveStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if _, err := ReadAPIKey(ctx, dir); err == nil {
		t.Fatal("no data dir yet")
	}
	st, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := Open(ctx, dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("a second open is refused as locked: %v", err)
	}
	if _, err := ReadAPIKey(ctx, dir); err == nil {
		t.Fatal("no key yet")
	}
	_ = st.Auth().SetAPIKey(ctx, "full-key")
	if k, err := ReadAPIKey(ctx, dir); err != nil || k != "full-key" {
		t.Fatal(k, err)
	}
	a := st.Auth()
	if _, ok, err := a.ReadOnlyAPIKey(ctx); ok || err != nil {
		t.Fatal("none issued")
	}
	_ = a.SetReadOnlyAPIKey(ctx, "ro-1")
	_ = a.SetReadOnlyAPIKey(ctx, "ro-2")
	if k, ok, _ := a.ReadOnlyAPIKey(ctx); !ok || k != "ro-2" {
		t.Fatal(k)
	}
	_ = a.SetReadOnlyAPIKey(ctx, "")
	if _, ok, _ := a.ReadOnlyAPIKey(ctx); ok {
		t.Fatal("revoked")
	}
	if err := os.WriteFile(filepath.Join(dir, KeyFile), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAPIKey(ctx, dir); err == nil {
		t.Fatal("a broken secret key")
	}
}
