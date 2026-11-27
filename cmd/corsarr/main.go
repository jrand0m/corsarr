// Command corsarr is the single-binary media automation server.
//
//	corsarr [serve] [--allow-root]          run the server (default); root is refused without --allow-root
//	corsarr healthcheck                     exit 0 when the running server is ready
//	corsarr config validate [--plan]        validate the active config source
//	corsarr config export [--format=yaml|env] [--full]
//	corsarr config schema                   print the JSON Schema
//	corsarr migrate plan|run --from=… --url=… --api-key-file=…   import an existing installation
//	corsarr admin reset-password [--username=…]   new admin password from stdin
//	corsarr version
//
// Configuration args (`--server.address=:8090`, `--configFile=…`) may follow
// any subcommand; exactly one configuration source is used per start.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/plugin"

	_ "github.com/jrand0m/corsarr/internal/plugins" // registers every built-in plugin
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Environ(), os.Stdout, os.Stderr))
}

// cli is one parsed invocation.
type cli struct {
	cmd     []string // e.g. ["config", "export"]
	flags   map[string]string
	cfgArgs []string
	in      config.Inputs
	stdout  io.Writer
	stderr  io.Writer
}

// commandFlags are subcommand flags, kept out of the config args.
var commandFlags = map[string]bool{"plan": true, "format": true, "full": true, "username": true, "allow-root": true}

func init() {
	for _, f := range migrateFlags {
		commandFlags[f] = true
	}
}

func parse(args, environ []string, stdout, stderr io.Writer) cli {
	c := cli{flags: map[string]string{}, stdout: stdout, stderr: stderr}
	for _, a := range args {
		name, val, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case !strings.HasPrefix(a, "-"):
			c.cmd = append(c.cmd, a)
		case commandFlags[name]:
			c.flags[name] = val
		default:
			c.cfgArgs = append(c.cfgArgs, a)
		}
	}
	c.in = config.OSInputs(c.cfgArgs)
	c.in.Environ = environ
	return c
}

// commands maps each subcommand to its handler.
var commands = map[string]func(c cli, ctx context.Context) int{
	"":                     func(c cli, ctx context.Context) int { return c.serve(ctx, nil) },
	"serve":                func(c cli, ctx context.Context) int { return c.serve(ctx, nil) },
	"healthcheck":          func(c cli, _ context.Context) int { return c.healthcheck() },
	"version":              func(c cli, _ context.Context) int { fmt.Fprintln(c.stdout, "corsarr", version); return 0 },
	"config validate":      func(c cli, _ context.Context) int { return c.validate() },
	"config export":        func(c cli, _ context.Context) int { return c.export() },
	"config schema":        func(c cli, _ context.Context) int { return c.schema() },
	"migrate plan":         func(c cli, ctx context.Context) int { return c.migrate(ctx, "plan") },
	"migrate run":          func(c cli, ctx context.Context) int { return c.migrate(ctx, "run") },
	"admin reset-password": func(c cli, ctx context.Context) int { return c.resetPassword(ctx) },
}

func run(ctx context.Context, args, environ []string, stdout, stderr io.Writer) int {
	c := parse(args, environ, stdout, stderr)
	name := strings.Join(c.cmd, " ")
	if h, ok := commands[name]; ok {
		return h(c, ctx)
	}
	fmt.Fprintf(stderr, "unknown command %q (serve | healthcheck | config validate|export|schema | migrate plan|run | admin reset-password | version)\n", name)
	return 2
}

func (c cli) schema() int {
	_, _ = c.stdout.Write(config.NewSchema(plugin.Default).JSONSchema())
	return 0
}

// load reads the active config source; errors are printed one per line.
func (c cli) load() (*config.Config, bool) {
	cfg, err := config.Load(c.in, plugin.Default)
	if err != nil {
		fmt.Fprintln(c.stderr, "configuration invalid:")
		fmt.Fprintln(c.stderr, err)
		return nil, false
	}
	return cfg, true
}
