package printable

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestTemplatesAreBounded(t *testing.T) {
	if _, err := parseTemplate([]byte("<p>\n{{range 1000000000000}}{{end}}"), "en"); err == nil || !strings.Contains(err.Error(), "printable.html.tmpl:2") {
		t.Errorf("a huge range parsed: %v", err)
	}
	if _, err := parseTemplate([]byte("{{range 12}}{{.}}{{end}}"), "en"); err != nil {
		t.Errorf("a small range: %v", err)
	}
	tmpl, err := parseTemplate([]byte(`{{range (seq 3)}}{{mul 1000 1000}}{{end}}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := tmpl.Execute(io.Discard, nil); err == nil || !strings.Contains(err.Error(), "beyond") {
		t.Errorf("mul overflowed its bound: %v", err)
	}
	body := &limited{left: 10}
	if _, err := body.Write(make([]byte, 11)); err == nil {
		t.Error("the render cap let 11 bytes through 10")
	}
}

func TestRenderGivesUpOnAnEndlessTemplate(t *testing.T) {
	was := renderTime
	renderTime = 200 * time.Millisecond
	defer func() { renderTime = was }()
	tmpl, err := parseTemplate([]byte(`{{range 10000}}{{range 10000}}{{range 10000}}{{end}}{{end}}{{end}}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := render(tmpl, View{Language: "en"}, Plan{}, ""); err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("render: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("giving up took %s", time.Since(start))
	}
}
