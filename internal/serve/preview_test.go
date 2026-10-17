package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/compile"
)

func write(t *testing.T, root, path, value string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}
func testProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "lfm.json", `{"schemaVersion":1,"url":"https://example.com/product/","language":"en","title":"First","description":"Description"}`)
	write(t, root, "index.html.tmpl", `<!doctype html><html lang=en><head><meta charset=utf-8><meta name=viewport content="width=device-width, initial-scale=1"><link rel=canonical href="{{.url}}"><title>{{.title}}</title></head><body><h1>{{.title}}</h1>{{script "app.js"}}</body></html>`)
	write(t, root, "app.js", `window.marker="first"`)
	return root
}

func TestMonitorAtomicSavesDependenciesAndIgnoredOutputs(t *testing.T) {
	root := testProject(t)
	m := newMonitor(root)
	before := m.snapshot()
	write(t, root, "output/index.html", "output")
	write(t, root, ".cache/report.json", "scratch")
	write(t, root, ".git/index", "git")
	write(t, root, "app.js.swp", "temporary")
	if changed(before, m.snapshot()) {
		t.Fatal("output/editor files triggered a rebuild")
	}
	write(t, root, "replacement.tmp", `window.marker="other"`)
	if err := os.Rename(filepath.Join(root, "replacement.tmp"), filepath.Join(root, "app.js")); err != nil {
		t.Fatal(err)
	}
	after := m.snapshot()
	if !changed(before, after) {
		t.Fatal("atomic save missed")
	}
	before = after
	write(t, root, "new/deep/data.json", "[]")
	if !changed(before, m.snapshot()) {
		t.Fatal("new nested source missed")
	}
	write(t, root, "node_modules/pkg/index.js", "export default 1")
	m.include([]string{filepath.Join(root, "node_modules/pkg/index.js")})
	before = m.snapshot()
	write(t, root, "node_modules/pkg/index.js", "export default 2")
	if !changed(before, m.snapshot()) {
		t.Fatal("resolved package edit missed")
	}
	before = m.snapshot()
	write(t, root, "node_modules/pkg/package.json", `{"type":"module"}`)
	if !changed(before, m.snapshot()) {
		t.Fatal("package resolution edit missed")
	}
	// Same-size edits with restored timestamps still change small authoring files.
	path := filepath.Join(root, "app.js")
	info, _ := os.Stat(path)
	before = m.snapshot()
	write(t, root, "app.js", `window.marker="third"`)
	os.Chtimes(path, info.ModTime(), info.ModTime())
	if !changed(before, m.snapshot()) {
		t.Fatal("same-size/timestamp source edit missed")
	}
}

type logURL struct{ ch chan string }

func (w logURL) Write(raw []byte) (int, error) {
	text := string(raw)
	if strings.HasPrefix(text, "serving http://") {
		address := strings.TrimSuffix(strings.Fields(text)[1], ",")
		select {
		case w.ch <- address:
		default:
		}
	}
	return len(raw), nil
}

type outcome struct {
	report compile.Result
	err    error
}

func start(t *testing.T, root string) (string, <-chan outcome) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	addresses := make(chan string, 1)
	built := make(chan outcome, 32)
	go func() {
		done <- Serve(ctx, root, Options{Port: 0, ExplicitPort: true, Log: logURL{addresses}, OnBuild: func(r compile.Result, e error) { built <- outcome{r, e} }})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("preview did not stop")
		}
	})
	select {
	case address := <-addresses:
		return address, built
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("preview did not start")
	}
	return "", nil
}
func fetch(t *testing.T, address string) (int, string) {
	t.Helper()
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(raw)
}
func nextBuild(t *testing.T, built <-chan outcome) outcome {
	t.Helper()
	select {
	case result := <-built:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("source change was not rebuilt")
	}
	return outcome{}
}

func TestServeReloadFailureRecoveryAndOutputIsolation(t *testing.T) {
	root := testProject(t)
	address, built := start(t, root)
	initial := nextBuild(t, built)
	if initial.err != nil {
		t.Fatal(initial.err)
	}
	code, page := fetch(t, address)
	if code != 200 || !strings.Contains(page, "First") || !strings.Contains(page, "/client.js?v=") {
		t.Fatal(code, page)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "output/index.html"))
	if bytes.Contains(raw, []byte("/__lfm_")) {
		t.Fatal("reload client leaked into deployable output")
	}
	prefix := strings.Split(page[strings.Index(page, "/__lfm_"):], "/client.js")[0]
	parsed, _ := http.NewRequest(http.MethodGet, address, nil)
	origin := "http://" + parsed.URL.Host
	for _, path := range []string{"/product/lfm.json", "/product/.cache/report.json", "/product/../lfm.json", "/elsewhere", "/product/sub/file"} {
		if code, _ := fetch(t, origin+path); code != 404 {
			t.Fatal("source/fallback exposed", path, code)
		}
	}
	response, err := http.Get(origin + prefix + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	states := make(chan state, 8)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if text := scanner.Text(); strings.HasPrefix(text, "data: ") {
				var s state
				if json.Unmarshal([]byte(strings.TrimPrefix(text, "data: ")), &s) == nil {
					states <- s
				}
			}
		}
	}()
	nextState := func() state {
		t.Helper()
		select {
		case s := <-states:
			return s
		case <-time.After(5 * time.Second):
			t.Fatal("reload event missing")
		}
		return state{}
	}
	first := nextState()
	write(t, root, "index.html.tmpl", `{{.missingValue}}`)
	failed := nextBuild(t, built)
	if failed.err == nil {
		t.Fatal("bad source accepted")
	}
	errorState := nextState()
	if errorState.Version <= first.Version {
		t.Fatal(errorState)
	}
	code, page = fetch(t, address)
	if code != 200 || !strings.Contains(page, "Build needs a fix") || !strings.Contains(page, "template.bindings") {
		t.Fatal(code, page)
	}
	latest, _ := os.ReadFile(filepath.Join(root, "output/index.html"))
	if !bytes.Equal(raw, latest) {
		t.Fatal("failed rebuild replaced output")
	}
	write(t, root, "index.html.tmpl", `<!doctype html><html lang=en><head><meta charset=utf-8><link rel=canonical href="{{.url}}"><title>{{.title}}</title></head><body><h1>Recovered</h1>{{script "app.js"}}</body></html>`)
	if result := nextBuild(t, built); result.err != nil {
		t.Fatal(result.err)
	}
	nextState()
	code, page = fetch(t, address)
	if code != 200 || !strings.Contains(page, "Recovered") {
		t.Fatal(code, page)
	}
	write(t, root, "app.js", `window.marker="updated"`)
	if result := nextBuild(t, built); result.err != nil {
		t.Fatal(result.err)
	}
	nextState()
	_, page = fetch(t, address)
	if !strings.Contains(page, "updated") {
		t.Fatal("script edit missed")
	}
	write(t, root, "output/unlisted.txt", "private")
	write(t, root, ".cache/arbitrary.txt", "scratch")
	select {
	case result := <-built:
		t.Fatal("generated output caused a rebuild loop", result)
	case <-time.After(500 * time.Millisecond):
	}
	if code, _ := fetch(t, origin+"/product/unlisted.txt"); code != 404 {
		t.Fatal("unlisted output file served")
	}
	request, _ := http.NewRequest(http.MethodPost, address, strings.NewReader("mutation"))
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatal(resp.StatusCode)
	}
}

func TestServeStartsWithBrokenSourceAndRecovers(t *testing.T) {
	root := testProject(t)
	write(t, root, "index.html.tmpl", `{{.missing}}`)
	address, built := start(t, root)
	if first := nextBuild(t, built); first.err == nil {
		t.Fatal("expected initial failure")
	}
	_, page := fetch(t, address)
	if !strings.Contains(page, "Build needs a fix") {
		t.Fatal(page)
	}
	write(t, root, "index.html.tmpl", `<!doctype html><html><head><link rel=canonical href="{{.url}}"><title>{{.title}}</title></head><body><h1>Ready</h1></body></html>`)
	if next := nextBuild(t, built); next.err != nil {
		t.Fatal(next.err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, page = fetch(t, address)
		if strings.Contains(page, "<h1>Ready</h1>") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("preview did not recover")
}

func TestReloadCSPDetectionPreservesPolicy(t *testing.T) {
	for _, tc := range []struct {
		policy  string
		blocked bool
	}{
		{"default-src 'self'", false}, {"script-src 'none'", true}, {"connect-src 'none'", true}, {"script-src", true}, {"script-src 'self';connect-src 'self'", false},
	} {
		raw := []byte(`<meta http-equiv="Content-Security-Policy" content="` + tc.policy + `">`)
		if got := reloadBlocked(raw); got != tc.blocked {
			t.Error(tc.policy, got)
		}
	}
}

func TestPreviewBusyPortPolicy(t *testing.T) {
	root := testProject(t)
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	_, number, _ := net.SplitHostPort(occupied.Addr().String())
	port, _ := strconv.Atoi(number)
	if err := Serve(context.Background(), root, Options{Port: port, ExplicitPort: true}); err == nil {
		t.Fatal("explicit occupied port silently changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addresses := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, root, Options{Port: port, Log: logURL{addresses}}) }()
	select {
	case address := <-addresses:
		if strings.Contains(address, occupied.Addr().String()) {
			t.Fatal("busy default port reused")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("default port fallback failed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("preview did not stop")
	}
}
