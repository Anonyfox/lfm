package release

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiveTargetDoesNotListen(t *testing.T) {
	called := false
	target := Target{URL: "http://localhost:1234/pricing"}
	destination, proxy, closeTarget, err := target.serve(func(string, string) (net.Listener, error) { called = true; return nil, nil })
	if err != nil || called || destination != target.URL || proxy != "" {
		t.Fatalf("%s %s %v", destination, proxy, err)
	}
	closeTarget()
}
func TestNestedAuditHandler(t *testing.T) {
	origin := "http://localhost:1234"
	h := AuditHandler([]byte("page"), map[string][]byte{"_next/static/a.css": []byte("body{}"), "assets/x.png": []byte("image")}, origin)
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/_next/static/a.css", 200}, {"GET", "/assets/x.png", 200}, {"GET", "/assets/../assets/x.png", 404}, {"GET", "/%2e%2e/assets/x.png", 404}, {"GET", "/assets%5cx.png", 404}, {"GET", "/unknown", 404}, {"POST", "/assets/x.png", 403}} {
		r := httptest.NewRequest(tc.method, origin+tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
func TestCheckLiveBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser test")
	}
	browser, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Runtime</title></head><body><main><h1>Runtime</h1></main></body></html>`)
	}))
	defer s.Close()
	r := AuditRuntime(context.Background(), Target{URL: s.URL}, browser, "")
	if r.Status != "ok" {
		if strings.Contains(r.Reason, "Operation not permitted") || strings.Contains(r.Reason, "operation not permitted") {
			t.Skip("sandbox cannot start Chrome: " + r.Reason)
		}
		t.Fatalf("%+v", r)
	}
	if r.Layout == nil || len(r.Problems) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestLiveTargetToolRoot(t *testing.T) {
	target := Target{URL: "https://example.com/"}
	project := t.TempDir()
	if got := target.toolRoot(project); got != project {
		t.Fatal(got)
	}
	if got := target.toolRoot(target.URL); got != "." {
		t.Fatal(got)
	}
}
