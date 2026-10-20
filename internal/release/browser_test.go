package release

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuleUnionPreservesOrderGroupsAndGlobals(t *testing.T) {
	t.Parallel()
	rules := []rule{{Key: "a", Text: "@layer base;"}, {Prefix: "@media (width>0){", Children: []rule{{Key: "b", Text: "p{color:red}"}, {Key: "c", Text: "aside{color:blue}"}}}, {Key: "d", Text: "@font-face{font-family:Demo}"}}
	got := render(rules, map[string]bool{"a": true, "b": true, "d": true})
	if got != "@layer base;@media (width>0){p{color:red}}@font-face{font-family:Demo}" {
		t.Fatal(got)
	}
}

func TestPreviewOnlyServesArtifactAndGET(t *testing.T) {
	t.Parallel()
	h := Handler([]byte("page"), map[string][]byte{"style.css": []byte("css")})
	for _, test := range []struct {
		method, path string
		status       int
	}{{"GET", "/", 200}, {"GET", "/style.css", 200}, {"GET", "/lfm.json", 404}, {"GET", "/../style.css", 404}, {"GET", "/node_modules/x", 404}, {"POST", "/", 405}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(test.method, test.path, nil))
		if r.Code != test.status {
			t.Fatal(test, r.Code)
		}
		if !strings.Contains(r.Header().Get("Content-Security-Policy"), "script-src 'none'") {
			t.Fatal("analysis can run page scripts")
		}
	}
}

func TestInstalledBrowserCriticalExtraction(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	page := []byte(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><style data-lfm-critical-source>body{margin:0}h1{color:rgb(10,20,30)}.below{color:red}@media(min-width:1000px){h1{font-size:45px}.below{color:blue}}.dynamic:hover{color:green}</style></head><body><h1>Visible</h1><div style="height:2400px"></div><p class="below">Below</p><script>document.body.textContent='MUST NOT EXECUTE'</script></body></html>`)
	result, err := Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "" || !strings.Contains(result.CSS, "h1") || strings.Contains(result.CSS, ".below") || !strings.Contains(result.CSS, ".dynamic:hover") {
		t.Fatalf("%+v", result)
	}
}

func TestInstalledBrowserPromotesOffscreenLayoutRules(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	page := []byte(`<!doctype html><html><head><style data-lfm-critical-source>body{margin:0}.below{padding:50px}</style></head><body><h1>Visible</h1><div style="height:2400px"></div><p class="below">Below</p></body></html>`)
	result, err := Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "" || result.Selection != "document" || !strings.Contains(result.CSS, ".below") {
		t.Fatalf("%+v", result)
	}
}
