// Command lfm compiles agent-authored onepagers into standalone static output.
package main

import (
	"fmt"
	"io"
	"os"
)

// Set at release time via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `lfm — looking for money. Onepagers that sell, built by agents on rails.

Usage:
  lfm <command> [arguments]

Commands:
  version    Print build identity
  help       Show this help

More commands land as the compiler moves in.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "lfm %s (%s, %s)\n", version, commit, date)
		return 0
	default:
		fmt.Fprintf(stderr, "lfm: unknown command %q\nRun 'lfm help' for usage.\n", args[0])
		return 2
	}
}
