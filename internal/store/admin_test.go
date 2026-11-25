package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestResetAdminPasswordBesideALiveStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := open(t, dir) // the "server" keeps the store and its lock open
	a := s.Auth()
	if err := a.SetAdmin(ctx, "owner", "old-hash"); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateSession(ctx, "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := ResetAdminPassword(ctx, dir, "", "new-hash"); err != nil {
		t.Fatal(err)
	}
	u, ok, err := a.Admin(ctx)
	if err != nil || !ok || u.Username != "owner" || u.PasswordHash != "new-hash" {
		t.Fatalf("admin after reset: %+v %v %v", u, ok, err)
	}
	if valid, _ := a.SessionValid(ctx, "tok", time.Now()); valid {
		t.Fatal("session survived the reset")
	}

	if err := ResetAdminPassword(ctx, dir, "renamed", "newer-hash"); err != nil {
		t.Fatal(err)
	}
	if u, _, _ := a.Admin(ctx); u.Username != "renamed" || u.PasswordHash != "newer-hash" {
		t.Fatalf("rename: %+v", u)
	}
}

func TestResetAdminPasswordRefusals(t *testing.T) {
	ctx := context.Background()
	if err := ResetAdminPassword(ctx, filepath.Join(t.TempDir(), "missing"), "", "h"); err == nil {
		t.Fatal("reset without a database succeeded")
	}
	dir := t.TempDir()
	open(t, dir)
	if err := ResetAdminPassword(ctx, dir, "", "h"); !errors.Is(err, ErrNoAdmin) {
		t.Fatalf("reset before first run: %v", err)
	}
}
