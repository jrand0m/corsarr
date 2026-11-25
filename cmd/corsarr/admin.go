package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/store"
)

// stdin is where reset-password reads the new password; tests replace it.
var stdin io.Reader = os.Stdin

// resetPassword sets a new admin password, read from the first line of
// stdin so it never shows in the process list or shell history, and signs
// every session out. A running server picks it up without a restart.
func (c cli) resetPassword(ctx context.Context) int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	pw, err := readPasswordLine(stdin)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return 2
	}
	if len(pw) < api.MinPasswordLen {
		fmt.Fprintf(c.stderr, "password must have at least %d characters\n", api.MinPasswordLen)
		return 2
	}
	hash, err := api.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return 1
	}
	return c.reportReset(store.ResetAdminPassword(ctx, cfg.Data.Dir, strings.TrimSpace(c.flags["username"]), hash))
}

func (c cli) reportReset(err error) int {
	switch {
	case errors.Is(err, store.ErrNoAdmin):
		fmt.Fprintln(c.stderr, "no admin user yet: open the web UI to set the first password")
		return 1
	case err != nil:
		fmt.Fprintln(c.stderr, "reset failed:", err)
		return 1
	}
	fmt.Fprintln(c.stdout, "admin password reset; every session is signed out")
	return 0
}

// readPasswordLine returns the first line of r without its line ending.
func readPasswordLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", errors.New(`no password on stdin: pipe it in, e.g. printf '%s\n' "$NEW_PASSWORD" | corsarr admin reset-password`)
	}
	return line, nil
}
