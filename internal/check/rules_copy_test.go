package check

import (
	"crypto/sha256"
	"fmt"
	"html"
	"os"
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
		"nicht nur twice":   {`<p>Nicht nur schön, sondern auch nützlich.</p><p>Nicht nur Basilikum, sondern Minze.</p>`, "more like it"},
		"nicht nur once":    {`<p>Nicht nur schön, sondern auch nützlich.</p>`, ""},
		"mühelos twice":     {`<p>Mühelos backen.</p><p>Ein müheloser Start.</p>`, "more like it"},
		"mühelos once":      {`<p>Mühelos backen.</p>`, ""},
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

func TestPlaceholders(t *testing.T) {
	for name, tc := range map[string]struct {
		head, body string
		want       string
	}{
		"lorem":         {"", `<p>Lorem ipsum dolor sit amet.</p>`, "Lorem ipsum"},
		"todo marker":   {"", `<h2>TODO: price</h2>`, "TODO"},
		"todo word":     {"", `<p>Your todo list, sorted.</p>`, ""},
		"your here":     {"", `<h1>Your headline here</h1>`, "Your headline here"},
		"insert":        {"", `<p>Call us at [insert phone].</p>`, "[insert phone]"},
		"alt":           {"", `<img src=a.png alt="TBD">`, "alternative text"},
		"meta":          {`<meta name=description content="Lorem ipsum">`, "", "metadata"},
		"quote exempt":  {"", `<blockquote>TODO was all he wrote</blockquote>`, ""},
		"starter text":  {`<meta property="og:image:alt" content="Placeholder preview: replace it.">`, "", "Placeholder preview"},
		"plain content": {"", `<p>Basil wants six hours of sun.</p>`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := runOn(t, placeholderRule, tc.head, tc.body, Options{StarterTexts: []string{"Placeholder preview: replace it."}})
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
	if got := runOn(t, placeholderRule, "", "", Options{URL: "https://example.com/herbs/", StarterTexts: []string{"x"}}); len(got) != 1 || !strings.Contains(got[0].Message, "still the placeholder") {
		t.Fatalf("starter url: %+v", got)
	}
	img := []byte("starter-png")
	got := runOn(t, placeholderRule, "", "", Options{Assets: map[string][]byte{"share-1.png": img}, StarterImages: [][32]byte{sha256.Sum256(img)}})
	if len(got) != 1 || !strings.Contains(got[0].Message, "share-1.png is still the starter") {
		t.Fatalf("%+v", got)
	}
}

func TestHiddenStart(t *testing.T) {
	for name, tc := range map[string]struct {
		css, body string
		want      string
	}{
		"visible":             {`h1{color:#000}`, `<h1>Hi</h1>`, ""},
		"opacity zero":        {`h1{opacity:0}`, `<h1>Hi</h1>`, "opacity: 0"},
		"container hidden":    {`.hero{opacity:0}`, `<div class=hero><h1>Hi</h1></div>`, "container"},
		"fade in":             {`@keyframes in{from{opacity:0;transform:translateY(8px)}to{opacity:1}}h1{animation:in .6s both}`, `<h1>Hi</h1>`, "animation in starts invisible"},
		"fade from half":      {`@keyframes in{from{opacity:.5}to{opacity:1}}h1{animation:in .6s}`, `<h1>Hi</h1>`, ""},
		"visibility":          {`main{visibility:hidden}`, `<main><h1>Hi</h1></main>`, "visibility: hidden"},
		"hover only":          {`h1:hover{opacity:0}`, `<h1>Hi</h1>`, ""},
		"reduced only":        {`@media (prefers-reduced-motion:reduce){h1{opacity:0}}`, `<h1>Hi</h1>`, ""},
		"reason":              {`.hero{opacity:0}`, `<div class=hero data-design-reason="curtain reveal is the product"><h1>Hi</h1></div>`, ""},
		"hidden h1 skipped":   {`h1{opacity:0}`, `<h1 hidden>Old</h1>`, ""},
		"percentage keyframe": {`@keyframes in{0%,40%{opacity:0}100%{opacity:1}}h1{animation-name:in}`, `<h1>Hi</h1>`, "starts invisible"},
	} {
		t.Run(name, func(t *testing.T) {
			got := runOn(t, hiddenStartRule, "<style>"+tc.css+"</style>", tc.body, Options{})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected: %+v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, tc.want) {
				t.Fatalf("want %q, got %+v", tc.want, got)
			}
		})
	}
}

func TestAltText(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"described":   {`<img src=a.png alt="Basil on a sunny rail">`, ""},
		"decorative":  {`<img src=a.png alt="">`, ""},
		"file name":   {`<img src=a.png alt="IMG_2041.jpg">`, "says nothing"},
		"camera name": {`<img src=a.png alt="DSC 0042">`, "says nothing"},
		"generic":     {`<img src=a.png alt="Photo">`, "says nothing"},
		"german":      {`<img src=a.png alt="Bild">`, "says nothing"},
		"shared":      {`<img src=a.png alt="Herb"><img src=b.png alt="Herb">`, "2 different images share"},
		"same image":  {`<img src=a.png alt="Herb"><img src=a.png alt="Herb">`, ""},
		"word in alt": {`<img src=a.png alt="Photo of a balcony at 7 am">`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := runOn(t, imageAltRule, "", tc.body, Options{})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected: %+v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, tc.want) {
				t.Fatalf("want %q, got %+v", tc.want, got)
			}
		})
	}
}

func TestGenericActions(t *testing.T) {
	for body, want := range map[string]bool{
		`<a href="#x">Learn more</a>`:                     true,
		`<a href="#x">Learn more →</a>`:                   true,
		`<button type=button>Mehr erfahren</button>`:      true,
		`<input type=submit value="Submit">`:              true,
		`<a href="#x">Learn more about the rota</a>`:      false,
		`<a href="#x">Buy the guide for 12 €</a>`:         false,
		`<a href="#x" aria-label="See the plan">More</a>`: false,
		`<a href="#x" hidden>Learn more</a>`:              false,
	} {
		if got := runOn(t, actionsRule, "", body, Options{}); (len(got) > 0) != want {
			t.Errorf("%s: %+v", body, got)
		}
	}
}

// Copy typed with curly apostrophes, as typographers and models write it,
// gets the findings of the same copy with straight ones.
func TestCopyTellsReadEitherApostrophe(t *testing.T) {
	for _, body := range []string{
		`<p>In today's fast-paced world, planning is hard.</p>`,
		`<p>Not just a diet, but a rota.</p><p>Not only a course, but a kit.</p>`,
		`<p>Let's dive in and unlock your garden's full potential.</p>`,
		`<p>Whether you're a beginner or a pro, you'll love it.</p>`,
		`<p>Don't just plan. It's time to take your herbs to the next level.</p>`,
	} {
		straight := runOn(t, copyTellsRule, "", body, Options{})
		curly := runOn(t, copyTellsRule, "", strings.ReplaceAll(body, "'", "’"), Options{})
		if len(straight) == 0 || len(curly) != len(straight) {
			t.Errorf("%s: %d findings straight, %d curly", body, len(straight), len(curly))
			continue
		}
		for i := range straight {
			if straight[i].Message != Straight(curly[i].Message) {
				t.Errorf("%s:\n straight %s\n curly    %s", body, straight[i].Message, curly[i].Message)
			}
		}
	}
}

// A page raises its own budget instead of turning perf.budgets off.
func TestBudgetsFollowThePage(t *testing.T) {
	css := "<style>" + strings.Repeat(".a{color:red}", 2000) + "</style>"
	got := runOn(t, budgetsRule, css, "<p>x</p>", Options{})
	if len(got) != 1 || !strings.Contains(got[0].Message, "over this page's budget of 20.0 KiB") || !strings.Contains(got[0].Hint, "build.validation.budgets.cssKiB") {
		t.Fatalf("default budget: %+v", got)
	}
	if got := runOn(t, budgetsRule, css, "<p>x</p>", Options{Budgets: Budgets{CSSKiB: 40}}); len(got) != 0 {
		t.Fatalf("raised budget still warns: %+v", got)
	}
	if got := runOn(t, budgetsRule, css, "<p>x</p>", Options{Budgets: Budgets{CSSKiB: 10}}); len(got) != 1 {
		t.Fatalf("lowered budget: %+v", got)
	}
	// A release's inline copy of the first-screen rules is counted once.
	if got := runOn(t, budgetsRule, css, "<p>x</p>", Options{ExternalCSSBytes: 20 << 10, CriticalCSSBytes: 20 << 10, Budgets: Budgets{CSSKiB: 40}}); len(got) != 0 {
		t.Fatalf("the critical copy counted twice: %+v", got)
	}
}

func TestMachinePassagesHaveLocationsAndExcerpts(t *testing.T) {
	raw, err := os.ReadFile("../aitext/testdata/generated/opus-01.md")
	if err != nil {
		t.Fatal(err)
	}
	var paragraphs []string
	for _, block := range strings.Split(string(raw), "\n\n") {
		if len(strings.Fields(block)) >= minMachineParagraph {
			paragraphs = append(paragraphs, strings.Join(strings.Fields(block), " "))
		}
	}
	var body strings.Builder
	for _, paragraph := range paragraphs {
		body.WriteString("\n<p>" + html.EscapeString(paragraph) + "</p>")
	}
	issues := runOn(t, copyMachineRule, "", body.String(), Options{File: "output/index.html"})
	if len(issues) != 1 {
		t.Fatalf("expected a generated fixture finding: %+v", issues)
	}
	issue := issues[0]
	if len(issue.Related) == 0 || len(issue.Related) > 3 || !strings.Contains(issue.Message, "window signals:") {
		t.Fatalf("%+v", issue)
	}
	for _, at := range issue.Related {
		if at.File != "output/index.html" || at.Line < 2 || !strings.Contains(issue.Message, fmt.Sprintf("%s:%d", at.File, at.Line)) {
			t.Fatalf("%+v", issue)
		}
	}
	found := false
	for _, paragraph := range paragraphs {
		found = found || strings.Contains(issue.Message, preview(paragraph))
	}
	if !found {
		t.Fatal("missing excerpts")
	}
	if got := runOn(t, copyMachineRule, "", "<p>Short factual copy.</p>", Options{}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestMachineExcludesTables(t *testing.T) {
	raw, err := os.ReadFile("../aitext/testdata/generated/opus-01.md")
	if err != nil {
		t.Fatal(err)
	}
	body := "<p>" + strings.ReplaceAll(html.EscapeString(string(raw)), "\n\n", "</p><p>") + "</p>"
	if len(runOn(t, copyMachineRule, "", body, Options{})) != 1 {
		t.Fatal("fixture must trigger")
	}
	for _, wrap := range []struct{ open, close string }{{"<table><tr><td>", "</td></tr></table>"}, {`<div role="table">`, "</div>"}, {`<div role="grid">`, "</div>"}} {
		if got := runOn(t, copyMachineRule, "", wrap.open+body+wrap.close, Options{}); len(got) != 0 {
			t.Fatalf("%s: %+v", wrap.open, got)
		}
	}
}
