package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/store"
)

func withStdin(t *testing.T, s string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(s)
	t.Cleanup(func() { stdin = old })
}

func TestAdminResetPassword(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Auth().SetAdmin(ctx, "owner", "old"); err != nil {
		t.Fatal(err)
	}

	withStdin(t, "correct horse battery\n")
	code, out, errs := cliRun(t, nil, "admin", "reset-password", "--data.dir="+dir)
	if code != 0 || !strings.Contains(out, "reset") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	if strings.Contains(out+errs, "correct horse") {
		t.Fatal("password echoed")
	}
	u, _, _ := st.Auth().Admin(ctx)
	if !api.VerifyPassword(u.PasswordHash, "correct horse battery") || u.Username != "owner" {
		t.Fatalf("stored admin %+v does not verify", u)
	}

	withStdin(t, "another password")
	if code, _, errs := cliRun(t, nil, "admin", "reset-password", "--username=boss", "--data.dir="+dir); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if u, _, _ := st.Auth().Admin(ctx); u.Username != "boss" || !api.VerifyPassword(u.PasswordHash, "another password") {
		t.Fatalf("rename %+v", u)
	}
}

func TestAdminResetPasswordRefusals(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cases := []struct {
		in, want string
		code     int
	}{
		{"", "stdin", 2},
		{"short\n", "at least", 2},
		{"long enough password\n", "web UI", 1}, // no admin yet
	}
	for _, c := range cases {
		withStdin(t, c.in)
		code, _, errs := cliRun(t, nil, "admin", "reset-password", "--data.dir="+dir)
		if code != c.code || !strings.Contains(errs, c.want) {
			t.Errorf("%q: %d %q", c.in, code, errs)
		}
	}
	withStdin(t, "   \n")
	if code, _, _ := cliRun(t, nil, "admin", "reset-password", "--username= ", "--data.dir="+dir); code != 2 {
		t.Errorf("blank password accepted: %d", code)
	}
}
