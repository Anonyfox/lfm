package compile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/testproject"
)

func criticalFixture(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	var css strings.Builder
	css.WriteString(`body{margin:0}h1{color:rebeccapurple}`)
	for i := 0; i < 900; i++ {
		fmt.Fprintf(&css, ".unseen-%d{color:rgb(%d,40,80);padding:10px}", i, i%255)
	}
	write(t, root, "theme.css", css.String())
	write(t, root, "index.html.tmpl", helperHTML(`{{script "assets/scripts/app.js"}}`, `{{style "theme.css"}}`))
	write(t, root, "assets/scripts/app.js", `window.result=42`)
	return root
}

func releaseBrowser(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
}

func loggingTool(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LFM_TEST_TOOL_LOG\"\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlainReleaseCriticalCSSColdWarmAndValidation(t *testing.T) {
	releaseBrowser(t)
	root := criticalFixture(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("LFM_TEST_TOOL_LOG", log)
	t.Setenv("LFM_LIGHTHOUSE", loggingTool(t, "echo fixture-version"))
	result, err := Compile(context.Background(), root, Options{Release: true, Publish: true})
	if err != nil || !result.Published || result.Validated {
		t.Fatalf("%v %+v", err, result)
	}
	if result.Release == nil || !result.Release.Applied {
		t.Fatalf("no critical split: %+v", result.Release)
	}
	page := read(t, root, "output/index.html")
	full := read(t, root, "output/"+result.Release.Stylesheet)
	critical, loader := false, false
	for _, n := range parsePage(t, page) {
		if n.Data == "style" && len(htmlx.RawText(n)) > 0 && len(htmlx.RawText(n)) < len(full) {
			critical = true
		}
		if n.Data == "link" && htmlx.Attr(n, "media") == "print" && htmlx.Attr(n, "onload") != "" {
			loader = true
		}
	}
	if !critical || !loader {
		t.Fatal("missing critical CSS or deferred full stylesheet")
	}
	for _, d := range result.Validation.Diagnostics {
		if d.Rule == "release.validate" {
			t.Fatal(d)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(raw)) {
		if line != "--version" {
			t.Fatalf("Lighthouse launched: %q", raw)
		}
	}
	// Preserve the real version so the optimization cache key stays identical.
	chrome, err := release.BrowserPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", loggingTool(t, "if [ \"$1\" = --version ]; then exec '"+chrome+"' --version; fi\nexit 1"))
	if err := os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	warm, err := Compile(context.Background(), root, Options{Release: true, Publish: true})
	if err != nil || !warm.Published {
		t.Fatalf("%v %+v", err, warm.Validation)
	}
	if page != read(t, root, "output/index.html") {
		t.Fatal("warm output differs")
	}
	raw, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" && line != "--version" {
			t.Fatalf("warm Chrome launched: %q", raw)
		}
	}
	t.Setenv("LFM_CHROME", chrome)
	if err := os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	validated, _ := Compile(context.Background(), root, Options{Validate: true})
	if !validated.Validated {
		t.Fatal("validation mode missing")
	}
	raw, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	launched := false
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" && line != "--version" {
			launched = true
		}
	}
	if !launched {
		t.Fatalf("validation did not start Lighthouse: %q", raw)
	}
}

func TestPlainReleasePrintsPrintable(t *testing.T) {
	releaseBrowser(t)
	root := testproject.Printable(t)
	result, err := Compile(context.Background(), root, Options{Release: true, Publish: true})
	if err != nil || !result.Published {
		t.Fatalf("%v %+v", err, result.Validation)
	}
	found := false
	for _, a := range result.Artifact {
		if strings.HasPrefix(string(a), "%PDF-") {
			found = true
		}
	}
	// PDFs may be packed inside delivery ZIPs; also inspect published files.
	_ = filepath.WalkDir(filepath.Join(root, "output"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".pdf") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("no printable PDF")
	}
	for _, d := range result.Validation.Diagnostics {
		if d.Rule == "release.pending" {
			t.Fatal(d)
		}
	}
}

func TestReleaseCriticalCSSFailurePreservesOutput(t *testing.T) {
	for _, validate := range []bool{false, true} {
		t.Run(fmt.Sprint(validate), func(t *testing.T) {
			root := criticalFixture(t)
			if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
				t.Fatal(err)
			}
			before := read(t, root, "output/index.html")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LFM_TEST_TOOL_LOG", filepath.Join(t.TempDir(), "calls"))
			t.Setenv("LFM_CHROME", loggingTool(t, "exit 1"))
			result, err := Compile(context.Background(), root, Options{Release: true, Validate: validate, Publish: true})
			if err == nil || result.Published {
				t.Fatalf("failed extraction published: %v", err)
			}
			count := 0
			for _, d := range result.Validation.Diagnostics {
				if d.Rule == "release.css" && d.Severity == "error" {
					count++
				}
			}
			if count != 1 || result.Errors() != 1 {
				t.Fatalf("expected one CSS error: %+v", result.Validation)
			}
			if before != read(t, root, "output/index.html") {
				t.Fatal("failed release replaced output")
			}
		})
	}
}
