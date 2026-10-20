package release

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestScreensCoverPhoneLaptopAndDesktop(t *testing.T) {
	t.Parallel()
	var names []string
	for _, s := range Screens {
		names = append(names, s.String())
	}
	if strings.Join(names, " ") != "390×844 1280×680 1440×900" || !Screens[0].Mobile || Screens[1].Mobile || Screens[2].Mobile {
		t.Fatalf("%+v", Screens)
	}
}

func TestEmptyFramesMergeIntoRanges(t *testing.T) {
	t.Parallel()
	v := Screen{390, 800, true}
	frames := []frameState{{Y: 0}, {Y: 400}, {Y: 800}, {Y: 2400}, {Y: 2800, Faint: true}, {Y: 3200, Faint: true}}
	var got []string
	for _, f := range emptyFindings(v, frames) {
		if f.Kind != "empty" || f.Screen != "390×800" || f.Hint == "" {
			t.Fatalf("%+v", f)
		}
		got = append(got, f.Message)
	}
	want := []string{
		"At 390×800 the page from 0 to 1600 px shows no legible text or image.",
		"At 390×800 the page from 2400 to 3200 px shows no legible text or image.",
		"At 390×800 the page from 2800 to 4000 px shows only faded content.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if emptyFindings(v, nil) != nil {
		t.Fatal("no empty frames, yet findings")
	}
}

func TestPerElementMergesScreensAndCutsTheList(t *testing.T) {
	t.Parallel()
	var p perElement
	if p.findings("small", "Small: %s.", "h") != nil {
		t.Fatal("nothing observed, yet a finding")
	}
	phone, laptop := Screens[0], Screens[1]
	p.add(phone, "p.a", "p.a at 8 px")
	p.add(phone, "p.a", "p.a again")
	p.add(laptop, "p.a", "p.a on the laptop")
	p.add(laptop, "p.b", "p.b at 7 px")
	p.add(laptop, "p.c", "p.c at 6 px")
	p.add(laptop, "p.d", "p.d at 5 px")
	p.add(laptop, "p.e", "p.e at 5 px")
	got := p.findings("small", "Small: %s.", "hint")
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	want := "Small: p.a at 8 px at 390×844, 1280×680; p.b at 7 px at 1280×680; p.c at 6 px at 1280×680; 2 more."
	if got[0].Message != want || got[0].Kind != "small" || got[0].Screen != "390×844" || got[0].Hint != "hint" {
		t.Fatalf("%+v\nwant %q", got[0], want)
	}
}

func TestSubset(t *testing.T) {
	t.Parallel()
	known := map[string]bool{"a": true, "b": true}
	if !subset(nil, known) || !subset([]string{"a", "b", "a"}, known) || subset([]string{"a", "c"}, known) {
		t.Fatal("subset")
	}
}

func TestResetFramesClearsOnlyOldFrames(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "frames")
	if err := resetFrames(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"390x844-01.jpg", "390x844-02.jpg", signaturesFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := resetFrames(dir); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 1 || left[0].Name() != signaturesFile {
		t.Fatalf("left %v", left)
	}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o644)
	if resetFrames(filepath.Join(file, "frames")) == nil {
		t.Fatal("a folder under a file")
	}
	// Frames that cannot be cleared would mix with the new run's.
	os.WriteFile(filepath.Join(dir, "390x844-01.jpg"), []byte("x"), 0o644)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if os.Remove(filepath.Join(dir, "390x844-01.jpg")) == nil {
		t.Skip("the folder stays writable, as for root")
	}
	if resetFrames(dir) == nil {
		t.Fatal("stale frames were kept silently")
	}
}

func TestFramesAndReportsLiveInTempPerProject(t *testing.T) {
	t.Parallel()
	a, b := FramesPath("/projects/a"), FramesPath("/projects/b")
	if a == b || a != FramesPath("/projects/a") || filepath.Dir(a) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(a), "lfm-frames-") {
		t.Fatalf("%q %q", a, b)
	}
	if ReportPath("/projects/a") == a || !strings.HasSuffix(ReportPath("/projects/a"), ".json") {
		t.Fatal("report and frames share a path")
	}
}

func TestRemoveProfileDeletesTheFolder(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "profile")
	os.MkdirAll(filepath.Join(dir, "Default", "Cache"), 0o755)
	os.WriteFile(filepath.Join(dir, "Default", "Cache", "x"), []byte("x"), 0o644)
	if err := removeProfile(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("profile still there")
	}
	if err := removeProfile(dir); err != nil {
		t.Fatal("removing a gone profile fails:", err)
	}
}

func TestCapitalize(t *testing.T) {
	t.Parallel()
	if capitalize("at phone width") != "At phone width" || capitalize("") != "" {
		t.Fatal(capitalize("at phone width"))
	}
}

func TestCompactCollapsesWhitespace(t *testing.T) {
	t.Parallel()
	if got := compact("a\n\tb   c", 100); got != "a b c" {
		t.Fatalf("%q", got)
	}
	if got := compact("", 3); got != "" {
		t.Fatalf("%q", got)
	}
}

func TestCollectSelectsCriticalAndDocumentRules(t *testing.T) {
	t.Parallel()
	rules := []rule{
		{Key: "a", Critical: true},
		{Key: "b", Document: true},
		{Prefix: "@media x{", Children: []rule{{Key: "c", Critical: true}, {Key: "d", Document: true}, {Key: "e"}}},
	}
	viewport, document := map[string]bool{}, map[string]bool{}
	collect(rules, viewport, false)
	collect(rules, document, true)
	if len(viewport) != 2 || !viewport["a"] || !viewport["c"] {
		t.Fatalf("viewport %v", viewport)
	}
	if len(document) != 4 || document["e"] {
		t.Fatalf("document %v", document)
	}
	if got := render([]rule{{Prefix: "@media x{", Children: []rule{{Key: "e", Text: "p{}"}}}}, map[string]bool{}); got != "" {
		t.Fatalf("an empty group was kept: %q", got)
	}
}

func pngOf(t *testing.T, w, h int, paint func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, paint(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSamePixelsToleratesRasterNoiseOnly(t *testing.T) {
	t.Parallel()
	grey := func(v uint8) func(x, y int) color.Color {
		return func(x, y int) color.Color { return color.RGBA{v, v, v, 255} }
	}
	base := pngOf(t, 20, 20, grey(100))
	if !samePixels(base, base) {
		t.Fatal("identical bytes")
	}
	if !samePixels(base, pngOf(t, 20, 20, grey(103))) {
		t.Fatal("a shift of 3 levels is noise")
	}
	if samePixels(base, pngOf(t, 20, 20, grey(130))) {
		t.Fatal("a shift of 30 levels is a change")
	}
	// 4 of 400 pixels off by 10 is at the 1% limit; 5 is over it
	spots := func(n int) []byte {
		return pngOf(t, 20, 20, func(x, y int) color.Color {
			if y == 0 && x < n {
				return color.RGBA{110, 100, 100, 255}
			}
			return color.RGBA{100, 100, 100, 255}
		})
	}
	if !samePixels(base, spots(4)) || samePixels(base, spots(5)) {
		t.Fatal("the 1% noise limit")
	}
	if samePixels(base, pngOf(t, 20, 21, grey(100))) {
		t.Fatal("different sizes")
	}
	if samePixels([]byte("not a png"), base) || samePixels(base, []byte("not a png")) {
		t.Fatal("garbage")
	}
}

func TestHandlerTypesAssets(t *testing.T) {
	t.Parallel()
	h := Handler([]byte("<p>page"), map[string][]byte{"font.woff2": {1}, "style.css": []byte("p{}")})
	for path, want := range map[string]string{"/": "text/html; charset=utf-8", "/index.html": "text/html; charset=utf-8", "/style.css": "text/css; charset=utf-8", "/font.woff2": "font/woff2"} {
		w := httpGet(h, path)
		if w.Code != 200 || w.Header().Get("Content-Type") != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %q", path, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

func TestAuditHandlerServesAssetsAndRefusesUpgrades(t *testing.T) {
	origin := "http://127.0.0.1:54321"
	h := AuditHandler([]byte("page"), map[string][]byte{"app.js": []byte("code")}, origin)
	w := httpGet(h, origin+"/index.html")
	if w.Code != 200 || w.Body.String() != "page" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httpGet(h, origin+"/app.js")
	if w.Code != 200 || w.Body.String() != "code" || w.Header().Get("Cache-Control") != "" {
		t.Fatal(w.Code, w.Header())
	}
	r := newRequest("GET", origin+"/", map[string]string{"Upgrade": "websocket"})
	w = serve(h, r)
	if w.Code != 403 {
		t.Fatal("websocket upgrade", w.Code)
	}
	if w := serve(h, newRequest("GET", "https://127.0.0.1:54321/", nil)); w.Code != 403 {
		t.Fatal("https", w.Code)
	}
}

func TestLighthouseParsingReadsMetricsErrorsWarningsAndSamples(t *testing.T) {
	t.Parallel()
	report := map[string]any{
		"lighthouseVersion": "13.5.0", "userAgent": "HeadlessChrome/140",
		"runWarnings": []string{"The page loaded too slowly."},
		"categories": map[string]any{
			"performance":    map[string]any{"score": 0.874, "auditRefs": []any{map[string]any{"id": "largest-contentful-paint", "weight": 25}, map[string]any{"id": "render-blocking", "weight": 1}}},
			"accessibility":  map[string]any{"score": 1, "auditRefs": []any{map[string]any{"id": "color-contrast", "weight": 7}, map[string]any{"id": "render-blocking", "weight": 1}}},
			"best-practices": map[string]any{"score": 1},
			"seo":            map[string]any{"score": 0},
		},
		"audits": map[string]any{
			"first-contentful-paint":   map[string]any{"score": 1, "displayValue": "0.8 s"},
			"largest-contentful-paint": map[string]any{"score": 0.6, "displayValue": "3.1 s"},
			"render-blocking": map[string]any{"title": "Render-blocking", "description": "Inline it.", "score": 0.5, "displayValue": "Est savings of 300 ms",
				"details": map[string]any{"items": []any{
					map[string]any{"url": "http://local/style.css", "wastedMs": 300},
					map[string]any{"url": "data:font/woff2;base64,AAAA"},
					map[string]any{"node": map[string]any{"selector": "body > p", "snippet": "<p>"}},
					map[string]any{"url": "http://local/4.css"},
				}}},
			"color-contrast": map[string]any{"title": "Contrast", "description": "Darken.", "score": 1, "warnings": []string{"Some nodes were skipped."}},
			"font-display":   map[string]any{"title": "Font display", "description": "Swap.", "score": nil, "errorMessage": "Audit crashed"},
			"passing":        map[string]any{"title": "Fine", "score": 1},
			"informative":    map[string]any{"title": "Info", "score": nil},
			"uses-http2":     map[string]any{"title": "HTTP/2", "score": 0},
		},
	}
	raw, _ := json.Marshal(report)
	r, err := parseLighthouse(raw, "http://local")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "ok" || r.Version != "13.5.0" || r.Chrome != "HeadlessChrome/140" || r.Scope == "" {
		t.Fatalf("%+v", r)
	}
	if *r.Scores["performance"] != 87 || *r.Scores["accessibility"] != 100 || *r.Scores["seo"] != 0 {
		t.Fatalf("scores %v %v %v", *r.Scores["performance"], *r.Scores["accessibility"], *r.Scores["seo"])
	}
	if r.Metrics["first-contentful-paint"] != "0.8 s" || r.Metrics["largest-contentful-paint"] != "3.1 s" || len(r.Metrics) != 2 {
		t.Fatalf("metrics %v", r.Metrics)
	}
	if !slices.Equal(r.Warnings, []string{"The page loaded too slowly."}) || len(r.Host) != 1 || r.Host[0].ID != "uses-http2" || len(r.Manual) != 0 {
		t.Fatalf("%+v", r)
	}
	byID := map[string]AuditFinding{}
	var ids []string
	for _, f := range r.Findings {
		byID[f.ID] = f
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "color-contrast,font-display,largest-contentful-paint,render-blocking" {
		t.Fatalf("findings in id order: %q", ids)
	}
	blocking := byID["render-blocking"]
	if blocking.Items != 4 || !slices.Equal(blocking.Categories, []string{"performance", "accessibility"}) {
		t.Fatalf("%+v", blocking)
	}
	if !slices.Equal(blocking.Samples, []string{"url=/style.css; wastedMs=300", "url=[embedded resource; full detail in report]", "body > p; <p>"}) {
		t.Fatalf("samples %q", blocking.Samples)
	}
	if c := byID["color-contrast"]; c.Categories != nil || !strings.HasSuffix(c.Hint, "Warnings: Some nodes were skipped.") {
		t.Fatalf("a passing audit with warnings counts as a diagnostic: %+v", c)
	}
	if f := byID["font-display"]; f.Value != "Audit crashed" {
		t.Fatalf("%+v", f)
	}
}

func TestLighthouseParsingRefusesBrokenReports(t *testing.T) {
	t.Parallel()
	good := `{"lighthouseVersion":"13","categories":{"performance":{"score":1},"accessibility":{"score":1},"best-practices":{"score":1},"seo":{"score":1}},"audits":{"a":{"score":1}}}`
	if _, err := parseLighthouse([]byte(good), ""); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{
		"not JSON":        "lighthouse crashed",
		"runtime error":   `{"runtimeError":{"message":"NO_FCP"}}`,
		"no audits":       strings.Replace(good, `"audits":{"a":{"score":1}}`, `"audits":{}`, 1),
		"category absent": strings.Replace(good, `"seo":{"score":1}`, `"pwa":{"score":1}`, 1),
		"score above one": strings.Replace(good, `"seo":{"score":1}`, `"seo":{"score":1.5}`, 1),
		"negative score":  strings.Replace(good, `"seo":{"score":1}`, `"seo":{"score":-0.1}`, 1),
	} {
		if _, err := parseLighthouse([]byte(bad), ""); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := parseLighthouse([]byte(`{"runtimeError":{"message":"NO_FCP"}}`), ""); err == nil || !strings.Contains(err.Error(), "NO_FCP") {
		t.Fatal("the runtime error is not named:", err)
	}
}

func TestAuditItemsReadTablesAndTrees(t *testing.T) {
	t.Parallel()
	if got := auditItems(json.RawMessage(`{"items":[{"a":1},{"b":2}]}`)); len(got) != 2 {
		t.Fatal(got)
	}
	tree := auditItems(json.RawMessage(`{"items":{"z":{"n":2},"a":{"n":1}}}`))
	if len(tree) != 2 || string(tree[0]) != `{"n":1}` {
		t.Fatalf("tree items in key order: %s", tree)
	}
	for _, raw := range []string{``, `null`, `{"items":3}`, `{"type":"opportunity"}`, `[1,2]`} {
		if got := auditItems(json.RawMessage(raw)); len(got) != 0 {
			t.Errorf("%q: %s", raw, got)
		}
	}
}

func TestDetailSampleNamesWhatMatters(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		`{"url":"http://o/a.js","wastedBytes":1200,"ignored":"x"}`:      "url=/a.js; wastedBytes=1200",
		`{"source":{"url":"http://o/b.js","line":3,"column":7}}`:        "/b.js; 3; 7",
		`{"node":{"selector":"h1","snippet":"<h1>"},"label":"Heading"}`: "h1; <h1>; label=Heading",
		`{"description":"` + strings.Repeat("long ", 80) + `"}`:         "description=" + strings.TrimSpace(strings.Repeat("long ", 80))[:228] + "…",
		`{"url":"data:image/png;base64,iVBOR","totalBytes":5}`:          "url=[embedded resource; full detail in report]",
		`not json`:    "",
		`{"other":1}`: "",
		`{"reason":"Blocked","failureType":"Not actionable","protocolReason":"none"}`: "reason=Blocked; failureType=Not actionable; protocolReason=none",
	} {
		if got := detailSample(json.RawMessage(raw), "http://o"); got != want {
			t.Errorf("detailSample(%s)\n got %q\nwant %q", raw, got, want)
		}
	}
}

func TestLighthouseDiagnosticsMarkUnscoredFindings(t *testing.T) {
	t.Parallel()
	got := LighthouseDiagnostics(&LighthouseResult{Report: "r.json", Findings: []AuditFinding{{ID: "dom-size", Title: "Avoid an excessive DOM size", Hint: "Fewer nodes.", Samples: []string{"body"}, Items: 1}}})
	if len(got) != 1 || got[0].Message != "Avoid an excessive DOM size (diagnostic)" || got[0].Hint != "Fewer nodes.\n  body" || got[0].Location.File != "index.html" {
		t.Fatalf("%+v", got)
	}
	if len(LighthouseDiagnostics(&LighthouseResult{})) != 0 {
		t.Fatal("an empty result yields diagnostics")
	}
}

func TestMissingLighthouseIsNotAFailure(t *testing.T) {
	t.Setenv("LFM_LIGHTHOUSE", filepath.Join(t.TempDir(), "missing"))
	r := runLighthouse([]byte("page"), nil, "/no/chrome", t.TempDir())
	if r.Status != "skipped" || !r.Missing || r.Hint != LighthouseSetup || r.Scope != auditScope {
		t.Fatalf("%+v", r)
	}
	if len(LighthouseDiagnostics(r)) != 0 {
		t.Fatal("a missing Lighthouse warns")
	}
}

func TestMissingBrowserWithLighthouseInstalledNamesOnlyTheBrowser(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "lighthouse")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", "/does-not-exist/chrome")
	t.Setenv("LFM_LIGHTHOUSE", tool)
	r := Audit([]byte("page"), nil, t.TempDir())
	if r.Runtime.Status != "skipped" || r.Lighthouse.Status != "skipped" || r.Lighthouse.Hint != BrowserSetup || r.Lighthouse.Reason != "browser unavailable" {
		t.Fatalf("%+v %+v", r.Runtime, r.Lighthouse)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Rule != "release.browser" {
		t.Fatalf("%+v", r.Diagnostics)
	}
}

func TestBrowserlessFeaturesFailWithoutChrome(t *testing.T) {
	t.Setenv("LFM_CHROME", "/does-not-exist/chrome")
	if _, err := NewPrinter(); err == nil || !strings.Contains(err.Error(), "LFM_CHROME") {
		t.Fatal("printer without Chrome:", err)
	}
	if _, err := Analyze([]byte("page"), nil); err == nil {
		t.Fatal("critical CSS without Chrome")
	}
	if _, err := ImagePlacement([]byte("page"), nil); err == nil {
		t.Fatal("image placement without Chrome")
	}
}

func newRequest(method, url string, header map[string]string) *http.Request {
	r := httptest.NewRequest(method, url, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func httpGet(h http.Handler, url string) *httptest.ResponseRecorder {
	return serve(h, newRequest("GET", url, nil))
}
