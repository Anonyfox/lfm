// Command lfm compiles agent-built sales pages into fast, checked, static output.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/Anonyfox/lfm/internal/cli"
)

// Set at release time via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if info, ok := debug.ReadBuildInfo(); ok && version == "dev" {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version // go install, or go build in a checkout
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && commit == "none":
				commit = s.Value[:min(12, len(s.Value))]
			case s.Key == "vcs.time" && date == "unknown":
				date = s.Value
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// The first signal cancels the running command; restoring the default
	// handler lets a second one end the process at once.
	go func() { <-ctx.Done(); stop() }()
	code := cli.Run(ctx, cli.Env{Stdout: os.Stdout, Stderr: os.Stderr, Build: cli.Build{Version: version, Commit: commit, Date: date}}, os.Args[1:])
	stop()
	os.Exit(code)
}
