package serve

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/compile"
)

func TestPublishReloadsOnlyWhenTheShownPageChanges(t *testing.T) {
	p := &preview{subscribers: map[chan state]bool{}, current: state{Path: "/"}}
	page := compile.Result{PageURL: "https://example.com/a/", Artifact: map[string][]byte{"index.html": []byte("one")}}
	p.publish(page, nil)
	if p.snapshot().Version != 1 || p.snapshot().Path != "/a/" {
		t.Fatalf("first publish: %+v", p.snapshot())
	}
	same := compile.Result{PageURL: page.PageURL, Artifact: map[string][]byte{"index.html": []byte("one")}}
	p.publish(same, nil)
	if p.snapshot().Version != 1 {
		t.Fatal("identical output triggered a reload")
	}
	p.publish(compile.Result{PageURL: page.PageURL}, errors.New("broken"))
	if v := p.snapshot(); v.Version != 2 || v.Problem == "" || string(v.Files["index.html"]) != "one" {
		t.Fatalf("failure must reload into the error page and keep the last good files: %+v", v)
	}
	p.publish(compile.Result{PageURL: page.PageURL}, errors.New("broken"))
	if p.snapshot().Version != 2 {
		t.Fatal("repeated identical failure triggered a reload")
	}
	p.publish(same, nil)
	if v := p.snapshot(); v.Version != 3 || v.Problem != "" {
		t.Fatalf("recovery must reload: %+v", v)
	}
}

func TestNewDependenciesJoinTheBaseline(t *testing.T) {
	root := t.TempDir()
	dep := filepath.Join(root, "node_modules", "pkg", "index.js")
	if err := os.MkdirAll(filepath.Dir(dep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dep, []byte("export default 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newMonitor(root)
	previous := m.snapshot()
	added := m.include([]string{dep})
	if len(added) == 0 || len(m.include([]string{dep})) != 0 {
		t.Fatalf("include must report each path once: %v", added)
	}
	for _, path := range added {
		if s, ok := m.stamp(path); ok {
			previous[path] = s
		}
	}
	if changed(previous, m.snapshot()) {
		t.Fatal("watching a new dependency looked like an edit")
	}
}

func TestPreviewsAreServedButNeverPublished(t *testing.T) {
	p := &preview{subscribers: map[chan state]bool{}, current: state{Path: "/"}, host: "127.0.0.1:8080"}
	p.publish(compile.Result{PageURL: "https://example.com/", Artifact: map[string][]byte{"index.html": []byte("page")},
		Previews: map[string][]byte{"k3x9m2q7w8e4r5t6y1u0/preview/a4-color.html": []byte("<p>edition</p>")}}, nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:8080/k3x9m2q7w8e4r5t6y1u0/preview/a4-color.html", nil))
	if rec.Code != 200 || rec.Body.String() != "<p>edition</p>" {
		t.Fatalf("preview: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:8080/a/b/c/d.html", nil))
	if rec.Code != 404 {
		t.Fatalf("deep path: %d", rec.Code)
	}
}
