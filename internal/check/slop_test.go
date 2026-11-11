package check

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/slop"
)

func TestCopySkipAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		html  string
		count int
	}{
		{`<p>As an AI language model, I cannot answer.</p>`, 1},
		{`<div data-slop-skip><p>As an AI language model, I cannot answer.</p></div>`, 0},
		{`<p><span data-slop-skip>As an AI language model</span> the report follows.</p>`, 0},
		{`<blockquote>As an AI language model</blockquote>`, 0},
		{`<title>As an AI language model</title>`, 1},
	} {
		raw := "<html lang=en><head></head><body>" + tc.html + "</body></html>"
		if strings.HasPrefix(tc.html, "<title>") {
			raw = "<html lang=en><head>" + tc.html + "</head><body></body></html>"
		}
		d, err := Parse([]byte(raw), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if got := slopRule("copy.internal-leak")(d); len(got) != tc.count {
			t.Errorf("%s: %+v", tc.html, got)
		}
	}
}

func TestCopyText(t *testing.T) {
	for _, tc := range []struct {
		name, source, lang string
		md                 bool
		allow              []slop.Allow
		levels             map[string]Severity
		rule               string
		line               int
		severity           Severity
	}{
		{name: "front matter", source: "---\nlang: de\n---\nKulinarische Vielfalt.", md: true, rule: "copy.food-filler", line: 4, severity: Warning},
		{name: "override", source: "---\nlang: en\n---\nKulinarische Vielfalt.", lang: "de", md: true, rule: "copy.food-filler", line: 4, severity: Warning},
		{name: "inline markup", source: "# **Kulinarische** Vielfalt", lang: "de", md: true, rule: "copy.food-filler", line: 1, severity: Warning},
		{name: "plain", source: "Facts.\nAs an AI language model, I cannot.", rule: "copy.internal-leak", line: 2, severity: Error},
		{name: "code", source: "```\nAs an AI language model\n```", md: true},
		{name: "inline code", source: "Use `as an AI language model` here.", md: true},
		{name: "quote", source: "> As an AI language model", md: true},
		{name: "phrase allowance", source: "Kulinarische Vielfalt.", lang: "de", allow: []slop.Allow{{Rule: "copy.food-filler", Phrase: "Kulinarische Vielfalt", Reason: "Registered name"}}},
		{name: "off", source: "Kulinarische Vielfalt.", lang: "de", levels: map[string]Severity{"copy.food-filler": Off}},
		{name: "severity", source: "Kulinarische Vielfalt.", lang: "de", levels: map[string]Severity{"copy.food-filler": Error}, rule: "copy.food-filler", line: 1, severity: Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := CopyText([]byte(tc.source), tc.md, Options{File: "copy.md", Language: tc.lang, CopyAllow: tc.allow, Rules: tc.levels})
			if tc.rule == "" {
				if len(r.Diagnostics) != 0 {
					t.Fatal(r)
				}
				return
			}
			if len(r.Diagnostics) != 1 || r.Diagnostics[0].Rule != tc.rule || r.Diagnostics[0].Location.Line != tc.line || r.Diagnostics[0].Severity != tc.severity {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func FuzzCopyText(f *testing.F) {
	f.Add("---\nlang: de\n---\n# Kulinarische Vielfalt")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 32768 {
			t.Skip()
		}
		for _, md := range []bool{false, true} {
			r := CopyText([]byte(s), md, Options{})
			for _, d := range r.Diagnostics {
				if !strings.HasPrefix(d.Rule, "copy.") || d.Location.Line < 1 {
					t.Fatal(d)
				}
			}
		}
	})
}

func TestCopySentenceDatedRecords(t *testing.T) {
	const summary = "Security fixes to the command and several networking packages."
	for _, tc := range []struct {
		name, prefix string
		want         int
	}{
		{"dated record", `<time datetime="2026-10-08">8 Oct</time>`, 0},
		{"prose", "", 1},
		{"hidden date", `<time hidden datetime="2026-10-08">8 Oct</time>`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte("<body><ul>"+strings.Repeat("<li>"+tc.prefix+summary+"</li>", 2)+"</ul></body>"), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got := slopRule("copy.duplicate-sentence")(d); len(got) != tc.want {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestCopyBanCTAScope(t *testing.T) {
	for _, tc := range []struct {
		name, selector, html string
		want                 int
	}{
		{"broad policy reproduces", "", `<div class="ctabox"><h2>Mietpreisbremse-Rechner</h2><a href="/rechner/">Mehr erfahren</a></div>`, 1},
		{"calculator", ".travel", `<div class="ctabox"><h2>Mietpreisbremse-Rechner</h2><a href="/rechner/">Mehr erfahren</a></div>`, 0},
		{"travel", ".travel", `<section class="travel"><p><a href="/reise/">Mehr erfahren</a></p></section>`, 1},
		{"inline and selector list", "a[href='/reise/'], .travel", `<p><a href="/rechner/">Mehr erfahren</a> <a href="/reise/">Mehr <em>erfahren</em></a></p>`, 1},
		{"hidden", ".travel", `<div hidden><p class="travel">Mehr erfahren</p></div>`, 0},
		{"metadata", ".travel", `<meta name="description" content="Mehr erfahren">`, 0},
		{"block boundary", ".travel", `<section class="travel"><p>Mehr</p><p>erfahren</p></section>`, 0},
		{"shared marker keeps ban", ".travel", `<p class="travel" data-lfm-shared>Mehr erfahren</p>`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.html), Options{CopyBans: []slop.Ban{{Regex: "mehr erfahren|jetzt buchen", Selector: tc.selector, Severity: "error"}}})
			if err != nil {
				t.Fatal(err)
			}
			got := slopRule("copy.banned")(d)
			if len(got) != tc.want {
				t.Fatalf("got %+v, want %d", got, tc.want)
			}
		})
	}
}

func FuzzCopyBanScope(f *testing.F) {
	f.Add(`<main class="travel"><p>Mehr <em>erfahren</em></p></main>`, ".travel")
	f.Add(`<p>Mehr erfahren</p>`, "[")
	f.Fuzz(func(t *testing.T, raw, selector string) {
		if len(raw)+len(selector) > 32768 {
			t.Skip()
		}
		ban := slop.Ban{Phrase: "mehr erfahren", Selector: selector, Severity: "error"}
		if _, err := ban.Validate(); err != nil {
			return
		}
		d, err := Parse([]byte(raw), Options{CopyBans: []slop.Ban{ban}})
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range slopRule("copy.banned")(d) {
			if hit.Location.Line < 1 || hit.Hint == "" {
				t.Fatal(hit)
			}
		}
	})
}
