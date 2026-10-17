// Command lfm compiles agent-built sales pages into fast, checked, static output.
package main

import (
	"context"
	"os"
	"os/signal"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, cli.Env{Stdout: os.Stdout, Stderr: os.Stderr, Build: cli.Build{Version: version, Commit: commit, Date: date}}, os.Args[1:])
	stop()
	os.Exit(code)
}
