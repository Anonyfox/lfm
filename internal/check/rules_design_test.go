package check

import (
	"strings"
	"testing"
)

func designCount(t *testing.T, id, input string) []Diagnostic {
	t.Helper()
	return checkRule(t, id, "<!doctype html><html><head><title>t</title></head><body><main>"+input+"</main></body></html>")
}

func expectDesign(t *testing.T, id string, want int, input string) []Diagnostic {
	t.Helper()
	got := designCount(t, id, input)
	if len(got) != want {
		var msgs []string
		for _, d := range got {
			msgs = append(msgs, d.Message)
		}
		t.Fatalf("%s: want %d findings, got %d: %s", id, want, len(got), strings.Join(msgs, " | "))
	}
	return got
}

const eyebrowCSS = `<style>.k{text-transform:uppercase;letter-spacing:.14em;font-size:11px}</style>`

func TestDesignEyebrows(t *testing.T) {
	d := expectDesign(t, "design.eyebrows", 1, eyebrowCSS+`<p class="k">Our story</p><h2>Built for teams</h2>`)
	if !strings.Contains(d[0].Message, "Our story") || d[0].Reference == "" || !strings.Contains(d[0].Hint, "data-design-reason") {
		t.Fatalf("message must name the label, reference the guide and explain the escape: %+v", d[0])
	}
	expectDesign(t, "design.eyebrows", 0, eyebrowCSS+`<p class="k">Step 2 · 5 min</p><h2>Mix the clay</h2>`)
	expectDesign(t, "design.eyebrows", 1, eyebrowCSS+`<p class="k">200–1,000 m / Twilight zone</p><h2>Twilight zone</h2>`)
	expectDesign(t, "design.eyebrows", 0, eyebrowCSS+`<p class="k" data-design-reason="issue number in the masthead">Our story</p><h2>Built</h2>`)
	expectDesign(t, "design.eyebrows", 0, eyebrowCSS+`<p class="k">Our story</p><p>Not a heading follows.</p>`)
	expectDesign(t, "design.eyebrows", 0, `<p>Our story</p><h2>Plain lead-in is fine</h2>`)
	expectDesign(t, "design.eyebrows", 1, `<p class="text-xs uppercase tracking-widest">Features</p><h2>Everything you need</h2>`)
	expectDesign(t, "design.eyebrows", 1, `<p class="eyebrow">Features</p><h2>Everything you need</h2>`)
}

func TestDesignGradientText(t *testing.T) {
	expectDesign(t, "design.gradient-text", 1, `<style>.g{background:linear-gradient(90deg,#6366f1,#ec4899);-webkit-background-clip:text;background-clip:text;color:transparent}</style><h1 class="g">Ship faster</h1>`)
	expectDesign(t, "design.gradient-text", 1, `<h1 class="bg-gradient-to-r from-indigo-500 to-pink-500 bg-clip-text text-transparent">Ship</h1>`)
	expectDesign(t, "design.gradient-text", 0, `<style>.g{background:url(paper.webp);background-clip:text;color:transparent}</style><h1 class="g">Textured</h1>`)
	expectDesign(t, "design.gradient-text", 0, `<style>.g{background:linear-gradient(#fff,#eee)}</style><h1 class="g">Panel</h1>`)
}

func TestDesignPalette(t *testing.T) {
	expectDesign(t, "design.palette", 1, `<style>.cta{background:#6366f1}</style><a class="cta" href="#x">Go</a>`)
	expectDesign(t, "design.palette", 1, `<style>.cta{background:oklch(58.5% 0.233 277.117)}</style><a class="cta" href="#x">Go</a>`)
	expectDesign(t, "design.palette", 1, `<style>.h{background:linear-gradient(90deg,#7c3aed,#db2777)}</style><div class="h">x</div>`)
	expectDesign(t, "design.palette", 0, `<style>.foil{background:conic-gradient(#ff4fa3,#6fd6ff,#ffb12e,#9b7bff,#6fffc2)}</style><div class="foil">x</div>`)
	expectDesign(t, "design.palette", 0, `<style>.b{background:#1720d4;color:#fff}</style><div class="b">Klein-ish blue is not violet</div>`)
	expectDesign(t, "design.palette", 1, `<style>body{background:#f4f1ea}.a{color:#d97757}</style><p class="a">Warm</p>`)
	expectDesign(t, "design.palette", 0, `<style>body{background:#f4f1ea}</style><p>Paper alone is fine</p>`)
}

func TestDesignEmoji(t *testing.T) {
	expectDesign(t, "design.emoji", 1, `<h2>🚀 Launch faster</h2>`)
	d := expectDesign(t, "design.emoji", 1, `<button>✨ Generate</button>`)
	if !strings.Contains(d[0].Message, "Sparkle") {
		t.Fatalf("sparkle gets its own explanation: %s", d[0].Message)
	}
	expectDesign(t, "design.emoji", 1, `<ul><li>✅ Fast</li></ul>`)
	expectDesign(t, "design.emoji", 0, `<ul><li>Night Shift <span>✶</span></li></ul><button>✓ In</button><h2>Ratings ★★★</h2>`)
	expectDesign(t, "design.emoji", 0, `<p>Body copy may say 🙂 to a friend.</p>`)
}

func TestDesignCardsAndStats(t *testing.T) {
	card := `<article class="c"><svg width="24" height="24"></svg><h3>Fast</h3><p>Two lines.</p></article>`
	expectDesign(t, "design.cards", 1, `<div>`+strings.Repeat(card, 3)+`</div>`)
	plain := `<article class="c"><h3>Fast</h3><p>Two lines.</p></article>`
	expectDesign(t, "design.cards", 0, `<div>`+strings.Repeat(plain, 3)+`</div>`)
	expectDesign(t, "design.cards", 0, `<nav>`+strings.Repeat(card, 3)+`</nav>`)
	stat := func(n string) string {
		return `<div class="s"><strong>` + n + `</strong><span>happy users</span></div>`
	}
	expectDesign(t, "design.stats", 1, `<div>`+stat("10k+")+stat("99.9%")+stat("24/7")+`</div>`)
	expectDesign(t, "design.stats", 0, `<form><div>`+stat("10k+")+stat("99.9%")+stat("50+")+`</div></form>`)
	expectDesign(t, "design.stats", 0, `<div>`+stat("118")+stat("45")+stat("3")+`</div>`)
}

func TestDesignStripesGlassShadows(t *testing.T) {
	expectDesign(t, "design.stripes", 1, `<style>.n{border-left:4px solid #e11;padding:1rem;background:#fff}</style><div class="n">a</div><div class="n">b</div>`)
	expectDesign(t, "design.stripes", 0, `<style>blockquote{border-left:4px solid #e11;padding:1rem}</style><blockquote>a</blockquote><blockquote>b</blockquote>`)
	expectDesign(t, "design.stripes", 0, `<style>.n{border:1px solid #ccc;border-left:4px solid #e11;padding:1rem}</style><div class="n">a</div><div class="n">b</div>`)
	glass := `<style>.g{backdrop-filter:blur(12px)}</style>`
	expectDesign(t, "design.glass", 1, glass+`<nav class="g">n</nav><div class="g">a</div><div class="g">b</div>`)
	expectDesign(t, "design.glass", 0, glass+`<nav class="g">n</nav><div class="g">sheet</div>`)
	shadow := `<style>.c{box-shadow:0 10px 15px -3px rgb(0 0 0 / 0.1),0 4px 6px -4px rgb(0 0 0 / 0.1);border-radius:16px}</style>`
	expectDesign(t, "design.surfaces", 1, shadow+`<div class="c">a</div><div class="c">b</div><div class="c">c</div>`)
	expectDesign(t, "design.surfaces", 0, `<style>.c{box-shadow:0 30px 60px -30px #5a2408}</style><div class="c">a</div><div class="c">b</div><div class="c">c</div>`)
}

func TestDesignActionsChromeMono(t *testing.T) {
	btns := `<a href="#a" class="b">Start →</a><a href="#b" class="b">Read more →</a><button>Buy →</button><a href="#c">Plain</a>`
	expectDesign(t, "design.arrows", 1, btns)
	expectDesign(t, "design.arrows", 0, `<a href="https://x.example/" target="_blank">Docs ↗</a><a href="https://y.example/" target="_blank">Repo ↗</a><a href="#a">One</a><a href="#b">Two</a>`)
	expectDesign(t, "design.arrows", 0, `<p>Read the <a href="#a">notes →</a> before you start the experiment and then continue.</p><a href="#b">One</a><a href="#c">Two</a><a href="#d">Three</a>`)
	meta := `<p>Berlin · 2026 · Concept</p>`
	expectDesign(t, "design.chrome", 1, strings.Repeat(meta, 4))
	expectDesign(t, "design.chrome", 0, meta)
	dots := `<style>.r{background:#ff5f56}.y{background:#ffbd2e}.g{background:#27c93f}</style><div><span class="r"></span><span class="y"></span><span class="g"></span></div>`
	expectDesign(t, "design.chrome", 1, dots)
	suns := `<style>.s{background:#ffbd2e}</style><div><span class="s"></span><span class="s"></span><span class="s"></span></div>`
	expectDesign(t, "design.chrome", 0, suns)
	expectDesign(t, "design.chrome", 0, `<div class="window-controls"><button>← Previous</button><output>1 of 4</output><button>Next →</button></div>`)
	mono := `<style>.m{font-family:ui-monospace,monospace}</style>`
	expectDesign(t, "design.mono", 1, mono+strings.Repeat(`<p class="m">Field label</p>`, 8))
	expectDesign(t, "design.mono", 0, mono+strings.Repeat(`<pre class="m">npm install thing</pre>`, 8)+strings.Repeat(`<output class="m">25:00</output>`, 8))
	expectDesign(t, "design.mono", 0, mono+strings.Repeat(`<p class="m">Field label</p>`, 7))
}

func TestDesignHeroTypeProofSectionsMotion(t *testing.T) {
	pill := `<style>.p{border-radius:999px;border:1px solid #ddd;padding:2px 10px}h1{text-align:center}</style>`
	expectDesign(t, "design.hero", 2, pill+`<section><span class="p">✨ New: v2</span><h1>Build better</h1><p>Sub</p><a href="#a">Start</a><a href="#b">Demo</a></section>`)
	expectDesign(t, "design.hero", 0, `<section><p>Concept demo</p><h1>Before the alarm.</h1><a href="#a">Scroll</a></section>`)
	expectDesign(t, "design.type", 1, `<style>body{font-family:Inter,sans-serif}</style><h1>Hello world</h1><p>Body</p>`)
	expectDesign(t, "design.type", 0, `<style>body{font-family:Inter,sans-serif}h1{font-family:Anton,Impact}</style><h1>Hello world</h1>`)
	expectDesign(t, "design.type", 1, `<style>body,h1{font-family:Bricolage,sans-serif}em{font-family:'Instrument Serif',serif;font-style:italic}</style><h1>Build tools with <em>care</em> today</h1>`)
	expectDesign(t, "design.type", 0, `<style>h1{font-family:Bricolage}.hl{color:orange}</style><h1>Before <span class="hl">the alarm.</span><span class="visually-hidden"> First Light, a sunrise lamp</span></h1>`)
	expectDesign(t, "design.proof", 1, `<p>Trusted by 10,000+ teams</p>`)
	expectDesign(t, "design.proof", 0, `<p>Two nights, both rooms.</p>`)
	expectDesign(t, "design.proof", 1, `<img src="https://i.pravatar.cc/80?img=3" alt="Jane, CEO">`)
	expectDesign(t, "design.sections", 1, `<h2>How it works</h2><h2>What our customers say</h2><h2>Pricing</h2><h2>FAQ</h2>`)
	expectDesign(t, "design.sections", 0, `<h2>Choose your night.</h2><h2>Change rooms.</h2><h2>Two nights.</h2>`)
	expectDesign(t, "design.motion", 1, strings.Repeat(`<section data-aos="fade-up">x</section>`, 4))
	expectDesign(t, "design.motion", 0, strings.Repeat(`<section class="beat">x</section>`, 4))
	expectDesign(t, "design.glow", 1, `<style>.o{position:absolute;border-radius:50%;filter:blur(64px);background:#a855f7}</style><div class="o"></div><div class="o"></div>`)
	expectDesign(t, "design.glow", 0, `<style>.o{border-radius:50%;filter:blur(64px)}</style><div class="o" data-design-reason="the lamp's light is the product"></div><div class="o" data-design-reason="the lamp's light is the product"></div>`)
}

func TestDesignCombination(t *testing.T) {
	page := eyebrowCSS + `<style>.cta{background:#6366f1}body{font-family:Inter}</style><p class="k">Features</p><h1>Ship fast</h1><h2>🚀 Launch</h2><a class="cta" href="#a">Start →</a><a href="#b">Docs →</a><a href="#c">More →</a><a href="#d">x</a>`
	d := expectDesign(t, "design.defaults", 1, page)
	if !strings.Contains(d[0].Message, "combination") {
		t.Fatalf("combination message: %s", d[0].Message)
	}
	expectDesign(t, "design.defaults", 0, `<h1>Plain page</h1><p>Nothing stock here.</p>`)
	// weak signals alone stay quiet below five kinds
	expectDesign(t, "design.defaults", 0, `<style>h1{text-align:center}</style><h1>Centered</h1><a href="#a">A →</a><a href="#b">B →</a>`)
}

func TestDesignCombinationIgnoresRulesTurnedOff(t *testing.T) {
	page := "<!doctype html><html><head><title>t</title></head><body><main>" + eyebrowCSS +
		`<style>.cta{background:#6366f1}body{font-family:Inter}</style><p class="k">Features</p><h1>Ship fast</h1><h2>🚀 Launch</h2><a class="cta" href="#a">Start →</a><a href="#b">Docs →</a><a href="#c">More →</a><a href="#d">x</a>` +
		"</main></body></html>"
	off := map[string]Severity{"design.palette": Off, "design.eyebrows": Off, "design.emoji": Off, "design.arrows": Off, "design.type": Off}
	d, err := Parse([]byte(page), Options{Rules: off})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range DefaultRules() {
		if rule.ID == "design.defaults" {
			if got := Run(d, []Rule{rule}, off).Diagnostics; len(got) != 0 {
				t.Fatalf("signals of disabled rules still counted: %v", got[0].Message)
			}
			return
		}
	}
	t.Fatal("design.defaults missing")
}
