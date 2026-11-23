package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
)

func (c cli) validate() int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	fmt.Fprintf(c.stdout, "configuration valid (source: %s%s)\n", cfg.Source.Kind, pathSuffix(cfg.Source.Path))
	for _, w := range cfg.Warnings {
		fmt.Fprintln(c.stdout, "warning:", w)
	}
	if _, plan := c.flags["plan"]; plan {
		printPlan(c, cfg)
	}
	return 0
}

func pathSuffix(p string) string {
	if p == "" {
		return ""
	}
	return " " + p
}

// printPlan lists the desired managed objects. Diffing against the database
// (create/update/prune) belongs to the config apply pipeline, which is not built yet.
func printPlan(c cli, cfg *config.Config) {
	fmt.Fprintln(c.stdout, "desired managed objects:")
	sections := make([]string, 0, len(cfg.Instances))
	for s := range cfg.Instances {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	for _, s := range sections {
		keys := make([]string, 0, len(cfg.Instances[s]))
		for k, inst := range cfg.Instances[s] {
			keys = append(keys, fmt.Sprintf("  %s.%s (%s)", s, k, inst.Type))
		}
		sort.Strings(keys)
		fmt.Fprint(c.stdout, joinLines(keys))
	}
	for i, f := range cfg.RootFolders {
		fmt.Fprintf(c.stdout, "  rootFolders.%d %s (%s)\n", i, f.Path, f.Kind)
	}
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func (c cli) export() int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	opts := config.ExportOptions{Full: c.has("full")}
	switch c.flags["format"] {
	case "", "yaml":
		_, _ = c.stdout.Write(cfg.ExportYAML(opts))
	case "env":
		_, _ = c.stdout.Write(cfg.ExportEnv(opts))
	default:
		fmt.Fprintln(c.stderr, "--format must be yaml or env")
		return 2
	}
	return 0
}

func (c cli) has(flag string) bool { _, ok := c.flags[flag]; return ok }

// healthcheck asks the running server on the configured address; it is the
// container health check (distroless has no curl).
func (c cli) healthcheck() int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	url := healthURL(cfg.Server.Address, cfg.Server.BasePath)
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(c.stderr, "unhealthy:", err)
		return 1
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(c.stderr, "unhealthy:", resp.Status)
		return 1
	}
	return 0
}

// healthURL targets loopback when the server listens on all interfaces.
func healthURL(addr, basePath string) string {
	host, port, _ := net.SplitHostPort(addr) // validated by config
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + basePath + "/api/v1/health"
}
