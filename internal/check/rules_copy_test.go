package check

import (
	"strings"
	"testing"
)

func runOn(t *testing.T, rule func(*Document) []Diagnostic, head, body string, options Options) []Diagnostic {
	t.Helper()
	d, err := Parse([]byte("<!doctype html><html lang=en><head><title>t</title>"+head+"</head><body>"+body+"</body></html>"), options)
	if err != nil {
		t.Fatal(err)
	}
	return rule(d)
}

func TestCopyTells(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string // empty: no finding
	}{
		"opener":            {`<p>In today's fast-paced world, planning is hard.</p>`, "stock phrase"},
		"curly apostrophe":  {`<p>In today’s digital age we plan.</p>`, "stock phrase"},
		"unlock":            {`<h2>Unlock the full potential of your garden</h2>`, "stock phrase"},
		"german opener":     {`<p>Im digitalen Zeitalter planen alle online.</p>`, "stock phrase"},
		"german dive":       {`<p>Tauchen Sie ein in die Welt der Kräuter.</p>`, "stock phrase"},
		"german beginner":   {`<p>Ob Anfänger oder Profi: hier wächst alles.</p>`, "stock phrase"},
		"not just twice":    {`<p>Not just a planner, but a habit.</p><p>Not only fast, but kind.</p>`, "more like it"},
		"not just once":     {`<p>Not just a planner, but a habit.</p>`, ""},
		"its not its twice": {`<p>It's not a diet. It's a rota.</p><p>This is not a course, this is a kit.</p>`, "more like it"},
		"nicht nur twice":   {`<p>Nicht nur schön, sondern auch nützlich.</p><p>Nicht nur Basilikum, sondern Minze.</p>`, ""},
		"nicht nur thrice":  {`<p>Nicht nur schön, sondern auch nützlich.</p><p>Nicht nur Basilikum, sondern Minze.</p><p>Nicht nur heute, sondern immer.</p>`, "more like it"},
		"no x no y just":    {`<p>No fluff. No jargon. Just results.</p>`, "stock phrase"},
		"seamless knitting": {`<p>Seamless socks knitted in the round.</p>`, ""},
		"seamless slop":     {`<p>Enjoy a seamless experience from start to finish.</p>`, "stock phrase"},
		"plain delve word":  {`<p>Delve is a word we never use.</p>`, ""},
		"quote exempt":      {`<blockquote>This app was a game changer for my nights</blockquote>`, ""},
		"q exempt":          {`<p>One nurse wrote <q>look no further</q> in her review.</p>`, ""},
		"reason exempt":     {`<p data-design-reason="parody of ad copy">Look no further!</p>`, ""},
		"hidden exempt":     {`<p hidden>Look no further</p><template><p>Look no further</p></template>`, ""},
		"no join blocks":    {`<li>say goodbye</li><li>to winter</li>`, ""},
		"inline join":       {`<p>Say <em>goodbye</em> to dead basil.</p>`, "stock phrase"},
		"repeat":            {`<p>Look no further.</p><p>Look no further!</p>`, "appears 2 times"},
		"dashes":            {`<p>Basil — the king — wilts. Mint — the rebel — spreads. Rosemary — patient.</p>`, "em dashes"},
		"dash ranges":       {`<p>13—14 Nov, 19—24 h, 2—3 pots, 4 — 5 days, rooms 1—2.</p>`, ""},
		"few dashes long":   {`<p>Basil — the king — wilts. ` + strings.Repeat("word ", 400) + `Mint — spreads — fast.</p>`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := runOn(t, copyTellsRule, "", tc.body, Options{})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected: %+v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, tc.want) {
				t.Fatalf("want one finding with %q, got %+v", tc.want, got)
			}
		})
	}
}

func TestReducedMotion(t *testing.T) {
	floatKF := `@keyframes float{0%{transform:translateY(0)}50%{transform:translateY(-8px)}}`
	for name, tc := range map[string]struct {
		css, script string
		want        bool
	}{
		"moving keyframes":      {floatKF + `.hero img{animation:float 6s ease-in-out infinite}`, "", true},
		"opacity keyframes":     {`@keyframes fade{from{opacity:0}to{opacity:1}}.a{animation:fade .3s}`, "", false},
		"handled in css":        {floatKF + `.a{animation:float 6s}@media (prefers-reduced-motion:reduce){.a{animation:none}}`, "", false},
		"only no-preference":    {`@media (prefers-reduced-motion:no-preference){` + floatKF + `.a{animation:float 6s}}`, "", false},
		"handled in js":         {floatKF + `.a{animation:float 6s}`, `matchMedia("(prefers-reduced-motion: reduce)")`, false},
		"defined but unused":    {floatKF, "", false},
		"smooth scroll":         {`html{scroll-behavior:smooth}`, "", true},
		"scroll timeline":       {`.a{animation-timeline:view()}`, "", true},
		"hover transition":      {`.btn{transition:transform .2s}`, "", false},
		"slow reveal":           {`.reveal{transition:opacity .6s,transform .6s}`, "", true},
		"slow color transition": {`.a{transition:color 1s}`, "", false},
		"split longhands":       {`.a{transition-property:transform;transition-duration:800ms}`, "", true},
		"scripted transforms":   {``, `el.animate([{transform:"scale(1)"},{transform:"scale(2)"}],600)`, true},
	} {
		t.Run(name, func(t *testing.T) {
			head := "<style>" + tc.css + "</style>"
			body := "<div class=a></div>"
			if tc.script != "" {
				body += "<script>" + tc.script + "</script>"
			}
			got := runOn(t, reducedMotionRule, head, body, Options{})
			if (len(got) > 0) != tc.want {
				t.Fatalf("want finding %v, got %+v", tc.want, got)
			}
		})
	}
	got := runOn(t, reducedMotionRule, `<link rel=stylesheet href=./a.css>`, `<script type=module src=./a.js></script>`, Options{
		Stylesheets: map[string][]byte{"./a.css": []byte(floatKF + `.a{animation:2s float infinite}`)},
		Assets:      map[string][]byte{"a.js": []byte(`if(matchMedia("(prefers-reduced-motion: reduce)").matches){}`)},
	})
	if len(got) != 0 {
		t.Fatalf("linked sheet and script: %+v", got)
	}
}

func TestLinkPurpose(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want bool
	}{
		"two buys":         {`<a href="https://buy.stripe.com/a">Buy</a><a href="https://buy.stripe.com/b">Buy</a>`, true},
		"case and spaces":  {`<a href="/a">Learn  more</a><a href="/b">learn more</a>`, true},
		"same destination": {`<a href="/kit">Get the kit</a><a href="/kit">Get the kit</a>`, false},
		"distinct labels":  {`<a href="/a" aria-label="Buy the starter kit">Buy</a><a href="/b" aria-label="Buy the full plan">Buy</a>`, false},
		"hidden one":       {`<a href="/a">Buy</a><a href="/b" hidden>Buy</a>`, false},
		"template one":     {`<a href="/a">Buy</a><template><a href="/b">Buy</a></template>`, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := runOn(t, linkPurposeRule, "", tc.body, Options{})
			if (len(got) > 0) != tc.want {
				t.Fatalf("want finding %v, got %+v", tc.want, got)
			}
		})
	}
}
