package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLighthouseParsingSeparatesScoresDiagnosticsAndManualChecks(t *testing.T) {
	t.Parallel()
	raw := `{"lighthouseVersion":"13.5.0","userAgent":"Chrome","categories":{"performance":{"score":1,"auditRefs":[{"id":"speed","weight":10},{"id":"size","weight":0}]},"accessibility":{"score":1},"best-practices":{"score":0.96,"auditRefs":[{"id":"console","weight":1}]},"seo":{"score":1}},"audits":{"speed":{"score":1},"console":{"title":"Browser errors","description":"Fix errors.","score":0,"details":{"items":[{"url":"http://local/favicon.ico"}]}},"size":{"title":"Transfer bytes","description":"Compress.","score":0.5},"keyboard":{"title":"Logical tab order","description":"Test it.","score":null,"scoreDisplayMode":"manual"},"cache-insight":{"title":"Use efficient cache lifetimes","description":"Cache.","score":0.5}}}`
	r, err := parseLighthouse([]byte(raw), "http://local")
	if err != nil {
		t.Fatal(err)
	}
	if *r.Scores["best-practices"] != 96 || len(r.Findings) != 2 || len(r.Manual) != 1 || len(r.Host) != 1 || r.Host[0].ID != "cache-insight" {
		t.Fatal(r)
	}
	if len(r.Findings[0].Categories) != 1 || r.Findings[0].Samples[0] != "url=/favicon.ico" || len(r.Findings[1].Categories) != 0 {
		t.Fatal(r.Findings)
	}
	for _, bad := range []string{`{}`, `{"runtimeError":{"message":"Navigation failed"}}`, strings.Replace(raw, `"score":0.96`, `"score":null`, 1)} {
		if _, err := parseLighthouse([]byte(bad), ""); err == nil {
			t.Fatal("accepted incomplete audit")
		}
	}
	tree := strings.Replace(raw, `"items":[{"url":"http://local/favicon.ico"}]`, `"items":{"root":{"url":"http://local/favicon.ico"}}`, 1)
	if r, err := parseLighthouse([]byte(tree), "http://local"); err != nil || r.Findings[0].Items != 1 {
		t.Fatal(r, err)
	}
}

func TestOwnedBrowserExposesPortForLighthouse(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserTestTimeout)
	defer cancel()
	profile, err := os.MkdirTemp("", "lfm-profile-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeProfile(profile); err != nil {
			t.Error(err)
		}
	})
	_, cleanup, err := startAuditBrowser(ctx, path, "http://127.0.0.1:1", profile)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	raw, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(strings.SplitN(string(raw), "\n", 2)[0])
	if err != nil || port < 1 || port > 65535 {
		t.Fatal(string(raw), err)
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/json/version")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
}

func TestAuditHandlerNeverForwardsOrMutates(t *testing.T) {
	t.Parallel()
	h := AuditHandler([]byte("page"), map[string][]byte{"app.js": []byte("code")}, "http://127.0.0.1:54321")
	for _, tc := range []struct {
		method, url string
		status      int
	}{
		{"GET", "http://127.0.0.1:54321/", 200}, {"GET", "http://127.0.0.1:54321/app.js", 200},
		{"GET", "http://127.0.0.1:54321/lfm.json", 404}, {"GET", "http://127.0.0.1:54321/../secret", 404},
		{"POST", "http://127.0.0.1:54321/", 403}, {"CONNECT", "http://remote.test/", 403}, {"GET", "http://remote.test/", 403},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.url, nil))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code)
		}
	}
}

func TestMissingCapabilitiesReportAllSetupRequirements(t *testing.T) {
	t.Setenv("LFM_CHROME", "/does-not-exist/chrome")
	t.Setenv("LFM_LIGHTHOUSE", "/does-not-exist/lighthouse")
	r := Audit(context.Background(), Target{Page: []byte("page")}, t.TempDir())
	// Only the missing browser warns; Lighthouse is optional, but the skip
	// still names how to set up both.
	if len(r.Diagnostics) != 1 || !strings.Contains(r.Lighthouse.Hint, "LFM_CHROME") || !strings.Contains(r.Lighthouse.Hint, "npm install") {
		t.Fatal(r)
	}
}

func TestRuntimeAuditExecutesJSAndBlocksOutsideTraffic(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	var calls atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer outside.Close()
	page := `<!doctype html><html><head><link rel="icon" href="data:image/png;base64,aA=="></head><body><script>fetch('` + outside.URL + `/',{method:'POST'}).catch(()=>{});console.error('probe failure');throw new Error('deliberate exception')</script></body></html>`
	r := AuditRuntime(context.Background(), Target{Page: []byte(page), Assets: nil}, path, "")
	if r.Status != "ok" {
		t.Fatal(r)
	}
	joined := strings.Join(r.Problems, " ")
	if !strings.Contains(joined, "deliberate exception") || !strings.Contains(joined, "probe failure") {
		t.Fatal(r.Problems)
	}
	if calls.Load() != 0 {
		t.Fatal("external mutation escaped audit proxy")
	}
}
