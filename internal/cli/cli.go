// Package cli implements the lfm command line: argument handling, the
// commands and how their results read in a terminal.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Anonyfox/lfm/internal/term"
)

// Build identifies the binary.
type Build struct{ Version, Commit, Date string }

// Env is everything a command touches outside the file system.
type Env struct {
	Stdout, Stderr io.Writer
	Build          Build
	Getenv         func(string) string // os.Getenv when nil
	Home           string              // the user's home directory; os.UserHomeDir when empty
}

func (e Env) getenv(key string) string {
	if e.Getenv == nil {
		return os.Getenv(key)
	}
	return e.Getenv(key)
}

func (e Env) home() (string, error) {
	if e.Home != "" {
		return e.Home, nil
	}
	return os.UserHomeDir()
}

type command struct {
	name, args, summary string
	flags               []flagSpec
	details             string
	run                 func(context.Context, Env, parsed) error
}

func commands() []command {
	return []command{
		initCommand, compileCommand, serveCommand, rulesCommand, skillsCommand, versionCommand,
	}
}

// Run executes args and returns the process exit code: 0 success, 1 failure,
// 2 a mistake in the command line, 130 interrupted.
func Run(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if len(args) > 2 {
			fmt.Fprintln(env.Stderr, "lfm: help takes at most one command")
			return 2
		}
		if len(args) > 1 {
			if c, ok := find(args[1]); ok {
				printCommandHelp(env.Stdout, c)
				return 0
			}
			fmt.Fprintf(env.Stderr, "lfm: unknown command %q\nRun 'lfm help' for the list.\n", args[1])
			return 2
		}
		printHelp(env.Stdout)
		return 0
	}
	if args[0] == "--version" {
		args[0] = "version"
	}
	c, ok := find(args[0])
	if !ok {
		fmt.Fprintf(env.Stderr, "lfm: unknown command %q\nRun 'lfm help' for the list.\n", args[0])
		return 2
	}
	p, err := parse(c.name, args[1:], c.flags)
	if err == nil && p.help {
		printCommandHelp(env.Stdout, c)
		return 0
	}
	if err == nil {
		err = c.run(ctx, env, p)
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintf(env.Stderr, "lfm: %s\nRun 'lfm help %s' for usage.\n", strings.TrimPrefix(err.Error(), errUsage.Error()+": "), c.name)
		return 2
	case errors.Is(err, errReported):
		return 1
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(env.Stderr, "lfm: interrupted")
		return 130
	default:
		fmt.Fprintf(env.Stderr, "lfm: %v\n", err)
		return 1
	}
}

// errReported means the command already explained the failure on stdout.
var errReported = errors.New("reported")

func find(name string) (command, bool) {
	for _, c := range commands() {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func printHelp(w io.Writer) {
	s := term.For(w)
	fmt.Fprintf(w, "%s compiles agent-built sales pages into fast, checked, static output.\n\n", s.Bold("lfm"))
	fmt.Fprintln(w, "Start a page:")
	fmt.Fprintln(w, "  lfm init my-page && cd my-page")
	fmt.Fprintln(w, "  lfm skills install    the research, design and implementation skills, where your agent loads them")
	fmt.Fprintln(w, "  lfm compile           after every change; it reports findings and the next step")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  lfm <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %s  %s\n", term.Pad(c.name, 8), c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'lfm help <command>' or 'lfm <command> -h' for details.")
}

func printCommandHelp(w io.Writer, c command) {
	fmt.Fprintf(w, "Usage: lfm %s", c.name)
	if c.args != "" {
		fmt.Fprintf(w, " %s", c.args)
	}
	fmt.Fprintf(w, "\n\n%s\n", c.summary)
	if c.details != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(c.details))
	}
	if len(c.flags) > 0 {
		fmt.Fprintln(w, "\nFlags:")
		for _, f := range c.flags {
			name := "--" + f.name
			if f.value {
				name += " <value>"
			}
			fmt.Fprintf(w, "  %s  %s\n", term.Pad(name, 18), f.usage)
		}
	}
}

var versionCommand = command{
	name:    "version",
	summary: "Print the version of this binary",
	run: func(_ context.Context, env Env, p parsed) error {
		if len(p.args) > 0 {
			return usageError("lfm version takes no arguments")
		}
		var about []string
		for _, v := range []string{env.Build.Commit, env.Build.Date} {
			if v != "" && v != "none" && v != "unknown" {
				about = append(about, v)
			}
		}
		line := "lfm " + env.Build.Version
		if len(about) > 0 {
			line += " (" + strings.Join(about, ", ") + ")"
		}
		fmt.Fprintln(env.Stdout, line)
		return nil
	},
}
