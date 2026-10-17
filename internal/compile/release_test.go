package compile

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func TestReleaseMissingBrowserWarnsAndPublishesNormalArtifact(t *testing.T) {
	root := fixture(t)
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	t.Setenv("LFM_CHROME", "/missing/chromium")
	report, err := RunWithOptions(root, RunOptions{Publish: true, Release: true})
	if err != nil || !report.Published || report.Validation.Errors != 0 {
		t.Fatal(err, report.Validation)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("unavailable optional feature changed the normal artifact")
	}
	if report.Release.Audit.Lighthouse.Status != "skipped" || report.Release.Audit.Runtime.Status != "skipped" {
		t.Fatal(report.Release.Audit)
	}
	found := false
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "release.browser" && d.Severity == "warn" && strings.Contains(d.Hint, "LFM_CHROME") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing actionable warning", report.Validation)
	}
}

func TestReleaseBuildValidatedWithFullCSSFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	root := fixture(t)
	var css strings.Builder
	css.WriteString(`body{margin:0}h1{color:rebeccapurple}`)
	for i := 0; i < 900; i++ {
		fmt.Fprintf(&css, ".unseen-%d{color:rgb(%d,40,80);padding:10px}", i, i%255)
	}
	css.WriteString(`.dynamic:hover{color:green}`)
	write(t, root, "theme.css", css.String())
	write(t, root, "index.html.tmpl", helperHTML(`{{script "assets/scripts/app.js"}}`, `{{style "theme.css"}}`))
	// Avoid a second CSS graph extracted from the JS fixture.
	write(t, root, "assets/scripts/app.js", `window.result=42`)
	report, err := RunWithOptions(root, RunOptions{Publish: true, Release: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	if report.Release == nil || !report.Release.Applied {
		t.Fatalf("%+v", report.Release)
	}
	if report.Compaction == nil || report.Compaction.Skipped != "" || report.Compaction.SavedBytes == 0 {
		t.Fatalf("release HTML was not safely compacted: %+v", report.Compaction)
	}
	page := read(t, root, "output/index.html")
	hasPrintLoader := false
	for _, n := range parsePage(t, page) {
		if n.Data == "link" && attr(n, "media") == "print" && attr(n, "onload") != "" {
			hasPrintLoader = true
		}
	}
	if !strings.Contains(page, "<noscript>") || !hasPrintLoader || !strings.Contains(page, ".dynamic:hover") || strings.Contains(page, ".unseen-899") {
		t.Fatal(page)
	}
	full := read(t, root, "output/"+report.Release.Stylesheet)
	if !strings.Contains(full, ".unseen-899") {
		t.Fatal("deferred CSS was purged")
	}
	if report.Validation.Errors != 0 {
		t.Fatal(report.Validation)
	}
	// Exercise the exact emitted loader and its genuine no-JavaScript fallback.
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(root, "output"))))
	defer server.Close()
	path, _ := release.BrowserPath()
	for _, scripts := range []bool{true, false} {
		t.Run(fmt.Sprintf("scripts=%t", scripts), func(t *testing.T) {
			deadline, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
			opts = append(opts, chromedp.ExecPath(path))
			alloc, stop := chromedp.NewExecAllocator(deadline, opts...)
			defer stop()
			browser, closeBrowser := chromedp.NewContext(alloc)
			defer closeBrowser()
			if !scripts {
				if _, err := chromedp.Call(browser, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := chromedp.Run(browser, chromedp.Navigate(server.URL)); err != nil {
				t.Fatal(err)
			}
			if scripts {
				if _, err := chromedp.Run(browser, chromedp.Poll[bool](`document.querySelector('link[onload]').media==='all'`, chromedp.WithPollingTimeout(5*time.Second))); err != nil {
					t.Fatal(err)
				}
			}
			padding, err := chromedp.Run(browser, chromedp.Evaluate[string](`(()=>{const p=document.createElement('p');p.className='unseen-899';document.body.append(p);return getComputedStyle(p).paddingTop})()`))
			if err != nil || padding != "10px" {
				t.Fatalf("deferred state: %q %v", padding, err)
			}
		})
	}
}

func TestReleaseReusesFileModeStylesheetWithEscapedName(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	root := fixture(t)
	var css strings.Builder
	css.WriteString("h1{color:blue}")
	for n := 0; n < 400; n++ {
		fmt.Fprintf(&css, ".unused-%d{padding:%dpx;color:rgb(%d,40,80)}", n, 5+n%20, n%255)
	}
	write(t, root, "theme&sheet.css", css.String())
	write(t, root, "assets/scripts/app.js", `window.result=42`)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "assets/scripts/app.js"}}`, `{{style "theme&sheet.css"}}`))
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"].(map[string]any)["overrides"] = map[string]any{"theme&sheet.css": "file"}
	})
	report, err := RunWithOptions(root, RunOptions{Publish: true, Release: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	if report.Release == nil || !report.Release.Applied || report.Release.Stylesheet != "theme&sheet.css" {
		t.Fatalf("%+v", report.Release)
	}
	count := 0
	for _, file := range report.Files {
		if strings.HasSuffix(file.Name, ".css") {
			count++
		}
	}
	hasSheet := false
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "link" && attr(n, "href") == "./theme&sheet.css" {
			hasSheet = true
		}
	}
	if count != 1 || !hasSheet {
		t.Fatal("stylesheet was not reused/escaped correctly")
	}
}
