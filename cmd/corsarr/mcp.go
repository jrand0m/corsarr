package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/store"
)

// mcpStdio serves MCP on stdin/stdout for a local agent. With the data dir
// free it runs corsarr in this process; while a server holds the data dir it
// relays every message to that server's /mcp endpoint with the stored API
// key, so nothing new listens on the network either way.
func (c cli) mcpStdio(ctx context.Context) int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	if !cfg.MCP.Enabled {
		fmt.Fprintln(c.stderr, "mcp.enabled is false; enable it to serve assistants")
		return 1
	}
	k, err := startKernel(ctx, cfg, newLogger(c, cfg.Log.Level))
	if errors.Is(err, store.ErrLocked) {
		return c.relayMCP(ctx, cfg)
	}
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr:", err)
		return 1
	}
	defer k.close()
	return c.exit(k.app.MCP.ServeStdio(ctx, stdin, c.stdout))
}

func (c cli) exit(err error) int {
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr mcp:", err)
		return 1
	}
	return 0
}

// relayMCP forwards newline-delimited messages to the running server.
func (c cli) relayMCP(ctx context.Context, cfg *config.Config) int {
	key, err := store.ReadAPIKey(ctx, cfg.Data.Dir)
	if err != nil {
		return c.exit(err)
	}
	r := &relay{url: mcpURL(cfg.Server.Address, cfg.Server.BasePath), key: key, out: c.stdout,
		client: &http.Client{Timeout: 5 * time.Minute}}
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() && ctx.Err() == nil {
		if err := r.send(ctx, sc.Bytes()); err != nil {
			return c.exit(err)
		}
	}
	return c.exit(sc.Err())
}

// mcpURL is the server's endpoint, on loopback when it listens everywhere.
func mcpURL(addr, basePath string) string {
	return strings.TrimSuffix(healthURL(addr, basePath), "/api/v1/health") + "/mcp"
}

type relay struct {
	url, key, session string
	client            *http.Client
	out               io.Writer
}

func (r *relay) send(ctx context.Context, msg []byte) error {
	if len(bytes.TrimSpace(msg)) == 0 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if r.session != "" {
		req.Header.Set("Mcp-Session-Id", r.session)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach corsarr at %s: %w", r.url, err)
	}
	defer resp.Body.Close()
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		r.session = id
	}
	return r.copy(resp)
}

// copy writes the answer as one line; 202 (a notification) has none and an
// event stream carries it in its data lines.
func (r *relay) copy(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode == http.StatusAccepted || len(body) == 0 {
		return err
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		body = sseData(body)
	}
	_, err = r.out.Write(append(bytes.TrimSpace(body), '\n'))
	return err
}

func sseData(stream []byte) []byte {
	var out []byte
	for _, line := range bytes.Split(stream, []byte("\n")) {
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			out = append(out, bytes.TrimSpace(data)...)
		}
	}
	return out
}
