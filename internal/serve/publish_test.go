package serve

import (
	"errors"
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
