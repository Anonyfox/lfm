package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = Run(context.Background(), Env{Stdout: &out, Stderr: &errs, Build: Build{Version: "1.2.3", Commit: "abc", Date: "today"}}, args)
	return code, out.String(), errs.String()
}

func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		code, out, _ := run(t, args...)
		if code != 0 || !strings.Contains(out, "Commands:") || !strings.Contains(out, "compile") {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
	code, out, _ := run(t, "help", "serve")
	if code != 0 || !strings.Contains(out, "--port") || !strings.Contains(out, "--open") {
		t.Fatalf("command help: %d %q", code, out)
	}
	code, out, _ = run(t, "compile", "--help")
	if code != 0 || !strings.Contains(out, "--release") {
		t.Fatalf("flag help: %d %q", code, out)
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		if code, out, _ := run(t, args...); code != 0 || out != "lfm 1.2.3 (abc, today)\n" {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"bogus"}, `unknown command "bogus"`},
		{[]string{"help", "bogus"}, `unknown command "bogus"`},
		{[]string{"compile", "--bogus"}, "unknown flag --bogus"},
		{[]string{"compile", "-x"}, "unknown flag -x"},
		{[]string{"compile", "a", "b"}, "takes one directory"},
		{[]string{"compile", "--release=yes"}, "--release takes no value"},
		{[]string{"serve", "--port"}, "--port needs a value"},
		{[]string{"serve", "--port", "http"}, "--port needs a number"},
		{[]string{"serve", "--open", "--open"}, "--open given twice"},
		{[]string{"version", "x"}, "takes no arguments"},
	} {
		code, out, errs := run(t, tc.args...)
		if code != 2 || out != "" || !strings.Contains(errs, tc.want) {
			t.Errorf("%v: code %d, stdout %q, stderr %q", tc.args, code, out, errs)
		}
	}
}

func TestInitThenCompile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tea-shop")
	code, out, errs := run(t, "init", dir)
	if code != 0 || !strings.Contains(out, "created") || !strings.Contains(out, "lfm.json") || !strings.Contains(out, "lfm serve --open") {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, _, errs := run(t, "init", dir); code != 1 || !strings.Contains(errs, "already contains") {
		t.Fatalf("second init: %d %q", code, errs)
	}
	code, out, errs = run(t, "compile", dir)
	if code != 0 || !strings.Contains(out, "✓ tea-shop compiled → output/") || strings.Contains(out, "warning") || errs != "" {
		t.Fatalf("compile: %d %q %q", code, out, errs)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("colored output written to a buffer")
	}
}

func TestFailedCompileExplainsAndKeepsOutput(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := run(t, "init", dir); code != 0 {
		t.Fatal(errs)
	}
	if code, _, _ := run(t, "compile", dir); code != 0 {
		t.Fatal("clean compile failed")
	}
	before, _ := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if err := os.WriteFile(filepath.Join(dir, "index.html.tmpl"), []byte(`<!doctype html><html lang="{{.language}}"><head><title>{{.missing}}</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, "compile", dir)
	if code != 1 || !strings.Contains(out, "error") || !strings.Contains(out, "failed") || !strings.Contains(out, "output/ unchanged") {
		t.Fatalf("failed compile: %d %q", code, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed compile replaced the output")
	}
}

func TestRulesListsExplainsAndRejects(t *testing.T) {
	code, out, _ := run(t, "rules")
	if code != 0 || !strings.Contains(out, "design.eyebrows") || !strings.Contains(out, "build.failed") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "design")
	if code != 0 || !strings.Contains(out, "design.eyebrows") || strings.Contains(out, "seo.metadata") {
		t.Fatalf("family: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "design.eyebrows")
	if code != 0 || !strings.Contains(out, `"design.eyebrows": "off"`) {
		t.Fatalf("explain: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "build.failed")
	if code != 0 || !strings.Contains(out, "Required") {
		t.Fatalf("required: %d %q", code, out)
	}
	if code, _, errs := run(t, "rules", "nope"); code != 1 || !strings.Contains(errs, `no rule or family named "nope"`) {
		t.Fatalf("unknown: %d %q", code, errs)
	}
}

func TestSkillsIsAnnouncedNotImplemented(t *testing.T) {
	code, out, _ := run(t, "skills")
	if code != 0 || !strings.Contains(out, "not published yet") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestParse(t *testing.T) {
	specs := []flagSpec{{name: "release"}, {name: "port", value: true}}
	p, err := parse("x", []string{"site", "--port", "9000", "--release", "--", "--literal"}, specs)
	if err != nil || p.flags["port"] != "9000" || !p.has("release") || strings.Join(p.args, " ") != "site --literal" {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parse("x", []string{"--port=0", "-"}, specs)
	if err != nil || p.flags["port"] != "0" || len(p.args) != 1 || p.args[0] != "-" {
		t.Fatalf("%+v %v", p, err)
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add("site --port 9000 --release")
	f.Add("-- --x = --port=")
	specs := []flagSpec{{name: "release"}, {name: "port", value: true}}
	f.Fuzz(func(t *testing.T, line string) {
		p, err := parse("x", strings.Fields(line), specs)
		if err == nil {
			for name := range p.flags {
				if _, ok := lookup(specs, name); !ok {
					t.Fatalf("accepted undeclared flag %q", name)
				}
			}
		}
	})
}
