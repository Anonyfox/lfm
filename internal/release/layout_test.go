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

func TestLayoutAuditFindsPinnedLayersOverText(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>
body{margin:0;font:18px/1.6 serif}
header{position:sticky;top:0;height:48px;background:#fff}
.stage{position:sticky;top:0;height:100vh;background:#234;z-index:2}
p{position:relative;z-index:1;margin:0 1rem 2rem}
.over p{z-index:3}
</style></head><body><header>Header bar passes over text</header><main><h1>Pinned</h1>`)
	b.WriteString(`<section><div class=stage></div>`)
	for i := 0; i < 12; i++ {
		b.WriteString(`<p>Text that scrolls under the pinned stage and disappears behind it.</p>`)
	}
	b.WriteString(`</section></main></body></html>`)
	r := AuditRuntime([]byte(b.String()), nil, path, "")
	found := false
	for _, f := range r.Layout.Findings {
		if f.Kind == "covered" && strings.Contains(f.Message, "div.stage covers the text of p") {
			found = true
		}
		if f.Kind == "covered" && strings.Contains(f.Message, "header") {
			t.Errorf("a thin header passing over text is expected: %s", f.Message)
		}
	}
	if !found {
		t.Fatalf("%+v", r.Layout.Findings)
	}
	over := strings.Replace(b.String(), "<section>", "<section class=over>", 1)
	for _, f := range AuditRuntime([]byte(over), nil, path, "").Layout.Findings {
		if f.Kind == "covered" {
			t.Errorf("text above the stage is not covered: %s", f.Message)
		}
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
	b.WriteString(`<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>body{margin:0 auto;max-width:40rem;padding:0 1rem;font:18px/1.6 serif}.v{white-space:nowrap}</style></head><body><main><h1>Plain page</h1><p><a href="#buy">See the offer</a></p>`)
	for i := 0; i < 40; i++ {
		b.WriteString(`<p>A paragraph of ordinary prose that keeps every frame legible, with a price of <span class=v>12 €</span> and 3 days of delivery.</p>`)
	}
	b.WriteString(`</main></body></html>`)
	r := AuditRuntime([]byte(b.String()), nil, path, "")
	if r.Layout == nil || len(r.Layout.Findings) != 0 {
		t.Fatalf("%+v", r.Layout)
	}
}

func TestLayoutAuditFirstScreenSmallTextAndContrast(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	page := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>
body{margin:0;font:16px/1.5 sans-serif;color:#111;background:#fff}
.hero{height:120vh;background:#234}
.faint{color:#aaa}
.photo{background:url(data:image/gif;base64,R0lGODlhAQABAAAAACw=) #000;color:#777}
svg text{font-size:8px}
</style></head><body><div class=hero></div><main><h1>Late heading</h1>
<p class=faint>Grey text on white.</p><p class=photo>Text over an image: unknown ground.</p>
<svg viewBox="0 0 1000 100" width="100%"><text x="10" y="50">0 m</text></svg>
<p><a href="#x">Act</a></p></main></body></html>`
	r := AuditRuntime([]byte(page), nil, path, "")
	var all []string
	for _, f := range r.Layout.Findings {
		all = append(all, f.Kind+": "+f.Message)
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"first screen shows no legible main heading", "first screen offers no link", "p.faint has 2.32:1 (#aaaaaa on #ffffff), needs 4.5:1", `text ("0 m") renders at`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "p.photo") {
		t.Errorf("text over an image has no certain ground:\n%s", joined)
	}
}

func TestLayoutAuditFindsMotionThatIgnoresReducedMotion(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	base := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>
body{margin:0;font:18px/1.6 serif}
@keyframes float{0%{transform:translateY(0)}50%{transform:translateY(-12px)}100%{transform:translateY(0)}}
.bob{width:80px;height:80px;background:#234;animation:float 3s infinite}
.fade{animation:fadein 2s infinite}@keyframes fadein{from{opacity:.5}to{opacity:1}}
REDUCE</style></head><body><main><h1>Motion</h1><p><a href="#x">Act</a></p><div class=bob></div><p class=fade>Only fades.</p><div id=js style="position:relative;width:40px;height:40px;background:#a00"></div></main>
<script>JS</script></body></html>`
	loop := `const el=document.getElementById("js");let t=0;(function f(){t+=4;el.style.left=(t%200)+"px";requestAnimationFrame(f)})()`
	page := strings.NewReplacer("REDUCE", "", "JS", loop).Replace(base)
	var motion string
	for _, f := range AuditRuntime([]byte(page), nil, path, "").Layout.Findings {
		if f.Kind == "motion" {
			motion = f.Message
		}
	}
	if !strings.Contains(motion, "div.bob (@keyframes float, looping)") || !strings.Contains(motion, "div#js (moved by a script)") || strings.Contains(motion, "fade") {
		t.Fatalf("motion finding: %q", motion)
	}
	quiet := strings.NewReplacer("REDUCE", "@media (prefers-reduced-motion:reduce){.bob{animation:none}}", "JS", "if(!matchMedia('(prefers-reduced-motion: reduce)').matches){"+loop+"}").Replace(base)
	for _, f := range AuditRuntime([]byte(quiet), nil, path, "").Layout.Findings {
		if f.Kind == "motion" {
			t.Fatalf("reduced page still reported: %s", f.Message)
		}
	}
}

func TestLayoutAuditComparesThePageWithoutScripts(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	head := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="></head><body><main>`
	scripted := head + `<h1 id=h style="opacity:0">Heading</h1><p><a href="#x">Act</a></p><div id=c></div></main><script>document.getElementById("h").style.opacity=1;document.getElementById("c").textContent="word ".repeat(200)</script></body></html>`
	var kinds []string
	for _, f := range AuditRuntime([]byte(scripted), nil, path, "").Layout.Findings {
		if f.Kind == "no-script" {
			kinds = append(kinds, f.Message)
		}
	}
	joined := strings.Join(kinds, "\n")
	if !strings.Contains(joined, "no legible main heading") || !strings.Contains(joined, "Without JavaScript the page shows") {
		t.Fatalf("scripted page: %q", joined)
	}
	static := head + `<style>@keyframes in{from{opacity:0}to{opacity:1}}h1{animation:in 3s both}</style><h1>Heading</h1><p><a href="#x">Act</a></p><p>` + strings.Repeat("word ", 200) + `</p></main><script>document.title="x"</script></body></html>`
	for _, f := range AuditRuntime([]byte(static), nil, path, "").Layout.Findings {
		if f.Kind == "no-script" {
			t.Fatalf("static page: %s", f.Message)
		}
	}
}

func TestLayoutAuditFindsFallbackFonts(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	font, err := os.ReadFile("../fontcheck/testdata/Go-Regular.woff2")
	if err != nil {
		t.Fatal(err)
	}
	page := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="><style>
@font-face{font-family:"Go Test";src:url(go.woff2) format("woff2");font-display:swap}
@font-face{font-family:"Gone";src:url(missing.woff2) format("woff2");font-display:swap}
body{font:18px/1.5 "Go Test",sans-serif}.gone{font-family:"Gone",serif}
</style></head><body><main><h1>Fonts</h1><p><a href="#x">Act</a></p>
<p class=ok>Plain Latin text, Grüße and €.</p><p class=cjk>日本語のテキスト</p><p class=emoji>Party 🎉</p><p class=gone>Lost face</p></main></body></html>`
	r := AuditRuntime([]byte(page), map[string][]byte{"go.woff2": font}, path, "")
	var fonts []string
	for _, f := range r.Layout.Findings {
		if f.Kind == "font" {
			fonts = append(fonts, f.Message)
		}
	}
	joined := strings.Join(fonts, "\n")
	if !strings.Contains(joined, `"Gone" failed to load`) || !strings.Contains(joined, `"Go Test" has no glyph for 日 本 語 の テ キ ス ト`) || !strings.Contains(joined, "p.cjk") {
		t.Fatalf("font findings:\n%s", joined)
	}
	if strings.Contains(joined, "p.ok") || strings.Contains(joined, "🎉") || strings.Count(joined, "no glyph") != 1 {
		t.Fatalf("covered text and emoji are fine:\n%s", joined)
	}
}
