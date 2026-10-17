package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutAuditFindsEmptyFramesOverflowAndSplitValues(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	page := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>
body{margin:0;font:16px/1.5 sans-serif}
header{position:fixed;top:0;left:0;right:0;height:40px;background:#eee}
.gap{height:2600px}
.wide{width:900px;height:20px;background:#ccc}
.price{width:11ch;font:16px monospace}
.faded{opacity:.3}
@keyframes rise{from{opacity:0;transform:translateY(20px)}to{opacity:1;transform:none}}
.reveal{animation:rise 2s both}
</style></head><body><header>Site header</header>
<main><h1 class=reveal>A heading that animates in</h1><p>Some opening text that is legible.</p>
<div class=gap></div>
<p class=faded>Only faded text lives here.</p><div class=gap></div>
<p class=price>Price 1234 €</p>
<div class=wide></div>
<p>The end.</p></main></body></html>`
	dir := t.TempDir()
	r := AuditRuntime([]byte(page), nil, path, dir)
	if r.Status != "ok" || r.Layout == nil {
		t.Fatalf("%+v", r)
	}
	var phone []string
	for _, f := range r.Layout.Findings {
		if strings.HasPrefix(f.Message, "At 390×844") {
			phone = append(phone, f.Kind+": "+f.Message)
		}
	}
	joined := strings.Join(phone, "\n")
	for _, want := range []string{"empty: At 390×844 the page from", "shows only faded content", "overflow: At 390×844 the page scrolls sideways", "div.wide", `split: At 390×844 "1234 €"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "from 0 ") {
		t.Errorf("the opening frame is legible once its reveal settles:\n%s", joined)
	}
	shots, _ := filepath.Glob(filepath.Join(dir, "390x844-*.jpg"))
	if len(shots) < 10 || r.Layout.Count < len(shots) {
		t.Fatalf("frames: %d files, count %d", len(shots), r.Layout.Count)
	}
	if info, err := os.Stat(shots[0]); err != nil || info.Size() < 1000 {
		t.Fatal("first frame is not an image", err)
	}
}

func TestLayoutAuditKeepsCleanPagesQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>body{margin:0 auto;max-width:40rem;padding:0 1rem;font:18px/1.6 serif}.v{white-space:nowrap}</style></head><body><main><h1>Plain page</h1>`)
	for i := 0; i < 40; i++ {
		b.WriteString(`<p>A paragraph of ordinary prose that keeps every frame legible, with a price of <span class=v>12 €</span> and 3 days of delivery.</p>`)
	}
	b.WriteString(`</main></body></html>`)
	r := AuditRuntime([]byte(b.String()), nil, path, "")
	if r.Layout == nil || len(r.Layout.Findings) != 0 {
		t.Fatalf("%+v", r.Layout)
	}
}
