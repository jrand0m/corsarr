package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/migrate"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// migrateFlags are the flags of `corsarr migrate plan|run`.
var migrateFlags = []string{"from", "url", "api-key-file", "api-key-env", "scope", "path-rewrite", "history-days", "activate",
	"exclude", "json", "write-config"}

const migrateUsage = `usage: corsarr migrate plan|run --from=sonarr|radarr|lidarr|prowlarr --url=http://host:port
       (--api-key-file=PATH | --api-key-env=NAME) [--scope=library,downloads,…] [--path-rewrite=/from:/to]
       [--history-days=N] [--activate] [--exclude=id,…] [--json] [--write-config=PATH]`

// migrate plans or runs a migration from one source against the database
// of the configured data directory; the server must not be running.
func (c cli) migrate(ctx context.Context, mode string) int {
	cfg, ok := c.load()
	if !ok {
		return 2
	}
	req, err := c.migrateRequest()
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		fmt.Fprintln(c.stderr, migrateUsage)
		return 2
	}
	st, err := store.Open(ctx, cfg.Data.Dir)
	if err != nil {
		fmt.Fprintln(c.stderr, "cannot open the database (stop a running corsarr first):", err)
		return 1
	}
	defer st.Close()
	svc := migrate.New(offlineDeps(st, cfg))
	progress := func(p migrate.Progress) {
		if p.Total == 0 {
			fmt.Fprintf(c.stderr, "%s: reading %s\n", p.Source, p.Step)
		}
	}
	var rep migrate.Report
	if mode == "run" {
		rep, err = svc.Apply(ctx, req, "", progress)
	} else {
		rep, err = svc.Plan(ctx, req, progress)
	}
	return c.migrateOutput(rep, err)
}

func offlineDeps(st *store.Store, cfg *config.Config) migrate.Deps {
	client := httpx.New(httpx.Options{Version: version, Default: httpx.Limits{Timeout: 60 * time.Second, Rate: 5, Burst: 2}})
	log := slog.New(slog.DiscardHandler)
	mgr := instances.New(plugin.Default, client, health.New(nil, nil, nil), log)
	mgr.Apply(cfg)
	return migrate.Deps{Store: st, HTTP: client, Config: func() *config.Config { return cfg }, Log: log,
		Downloaders: func() []migrate.Downloader {
			var out []migrate.Downloader
			for _, n := range instances.Of[plugin.Downloader](mgr, plugin.KindDownloader) {
				out = append(out, migrate.Downloader{Key: string(n.Key), List: n.Impl})
			}
			return out
		}}
}

// migrateRequest parses the flags once, at the boundary.
func (c cli) migrateRequest() (migrate.Request, error) {
	var req migrate.Request
	key, err := c.apiKey()
	if err != nil {
		return req, err
	}
	ep, err := migrate.ParseEndpoint(c.flags["from"], c.flags["url"], key)
	if err != nil {
		return req, err
	}
	req.Sources = []migrate.Endpoint{ep}
	req.Options, err = c.migrateOptions()
	return req, err
}

func (c cli) migrateOptions() (migrate.Options, error) {
	var o migrate.Options
	var err error
	if o.Scope, err = migrate.ParseScope(c.flags["scope"]); err != nil {
		return o, err
	}
	if o.Rewrite, err = migrate.ParseRewrite(c.flags["path-rewrite"]); err != nil {
		return o, err
	}
	if v := c.flags["history-days"]; v != "" {
		if o.HistoryDays, err = strconv.Atoi(v); err != nil || o.HistoryDays < 1 {
			return o, fmt.Errorf("--history-days must be a positive number of days")
		}
		o.Scope[migrate.CatHistory] = true
	}
	o.Activate = c.has("activate")
	o.Exclude = excludeSet(c.flags["exclude"])
	return o, nil
}

func excludeSet(list string) map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Split(list, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

// apiKey reads the source key from a file or an environment variable,
// never from the command line.
func (c cli) apiKey() (migrate.Secret, error) {
	if p := c.flags["api-key-file"]; p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return migrate.Secret{}, fmt.Errorf("--api-key-file: %w", err)
		}
		return migrate.NewSecret(string(b)), nil
	}
	if name := c.flags["api-key-env"]; name != "" {
		for _, kv := range c.in.Environ {
			if k, v, _ := strings.Cut(kv, "="); k == name {
				return migrate.NewSecret(v), nil
			}
		}
		return migrate.Secret{}, fmt.Errorf("--api-key-env: %s is not set", name)
	}
	return migrate.Secret{}, errors.New("give the source API key with --api-key-file or --api-key-env")
}

func (c cli) migrateOutput(rep migrate.Report, runErr error) int {
	if p := c.flags["write-config"]; p != "" && rep.Config != "" {
		if err := os.WriteFile(p, []byte(rep.Config), 0o600); err != nil {
			fmt.Fprintln(c.stderr, "--write-config:", err)
			return 1
		}
	}
	if c.has("json") {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printReport(c.stdout, rep)
	}
	if runErr != nil {
		fmt.Fprintln(c.stderr, "migration:", runErr)
		return 1
	}
	return 0
}

func printReport(w io.Writer, rep migrate.Report) {
	for _, s := range rep.Sources {
		fmt.Fprintf(w, "source %s %s version %s (supported %s)", s.App, s.URL, s.Version, s.Range)
		if s.Error != "" {
			fmt.Fprintf(w, ": %s", s.Error)
		}
		fmt.Fprintln(w)
	}
	for _, e := range rep.Entries {
		fmt.Fprintf(w, "%-15s %-11s %-28s %s — %s\n", e.Outcome, e.Category, e.ID, e.Name, e.Reason)
	}
	fmt.Fprintf(w, "%s: %v (plan %s)\n", rep.Mode, rep.Counts, rep.PlanHash)
	if rep.Config != "" {
		fmt.Fprintf(w, "\nconfig snippet (set %s):\n%s", strings.Join(rep.Env, ", "), rep.Config)
	}
}
