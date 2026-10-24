package check

import (
	"golang.org/x/net/html"
	"strings"
	"testing"
)

func TestUsePhraseInflections(t *testing.T) {
	for _, tc := range []struct {
		lang, text, phrase string
		want               bool
	}{
		{"en", "It remembers your work", "remember", true},
		{"en-GB", "She watches", "watch", true},
		{"en", "They remembered", "remember", true},
		{"en", "Remembering matters", "remember", true},
		{"en", "Keep remembering", "keep remember", true},
		{"en", "Saving matters", "save", true},
		{"en", "Studies matter", "study", true},
		{"en", "Keep running", "keep run", true},
		{"en", "We planned ahead", "plan", true},
		{"en", "", "", false},
		{"de", "remembers", "remember", false},
		{"de", "remember", "remember", true},
		{"en", "unremembered", "remember", false},
		{"en", "rememberable", "remember", false},
	} {
		d, err := Parse([]byte(`<html lang="`+tc.lang+`"><body><p>`+tc.text+`</p></body></html>`), Options{Concept: &ConceptFacts{Use: []string{tc.phrase}}})
		if err != nil {
			t.Fatal(err)
		}
		issues := conceptFidelityRule(d)
		if tc.lang == "de" && tc.text == "remembers" && (len(issues) != 1 || !strings.Contains(issues[0].Hint, `Near match: "remembers"`)) {
			t.Fatal(issues)
		}
		got := len(issues) == 0
		if got != tc.want {
			t.Errorf("%+v: matched %v", tc, got)
		}
	}
}

func TestPlaceholderFileNamesAndCode(t *testing.T) {
	for _, tc := range []struct {
		html string
		want int
	}{
		{`<p>Read TODO.md and path/FIXME.txt.</p>`, 0},
		{`<p>Read <code>TODO</code> and <code>FIXME</code>.</p>`, 0},
		{`<p>TODO: write this.</p>`, 1},
		{`<p>Read TODO.md. TODO: write this.</p>`, 1},
	} {
		if got := len(checkRule(t, "copy.placeholder", tc.html)); got != tc.want {
			t.Errorf("%s: %d", tc.html, got)
		}
	}
}

func TestClientShellGroupsOnlyExplainedFindings(t *testing.T) {
	shell := `<!doctype html><html lang=en><head><meta name=robots content=noindex></head><body><div id=mount></div><script type=module src="/app.js"></script></body></html>`
	report, err := Analyze([]byte(shell), Options{})
	if err != nil {
		t.Fatal(err)
	}
	grouped, independent := false, false
	for _, issue := range report.Diagnostics {
		if shellExplains(issue) {
			t.Errorf("ungrouped: %+v", issue)
		}
		if issue.Rule == "html.client-shell" {
			grouped = true
			if issue.Severity != Error || !strings.Contains(issue.Hint, "Canonical URL") || !strings.Contains(issue.Hint, "h1") || !strings.Contains(issue.Hint, "main-content") || !strings.Contains(issue.Hint, "og:title") {
				t.Errorf("%+v", issue)
			}
		}
		if issue.Rule == "seo.indexability" {
			independent = true
		}
	}
	if !grouped || !independent {
		t.Fatalf("%+v", report.Diagnostics)
	}
	for _, raw := range []string{
		strings.Replace(shell, `<div id=mount></div>`, `<main id=mount><h1>Library docs</h1><p>Read the API reference.</p></main>`, 1),
		strings.Replace(shell, `type=module`, `defer`, 1),
		strings.Replace(shell, `<div id=mount></div>`, `<div></div>`, 1),
		strings.Replace(shell, `<div id=mount></div>`, `<div id=mount></div><pre><code>go run example.org/reference with several arguments</code></pre>`, 1),
		strings.Replace(shell, `<div id=mount></div>`, `<div id=mount></div><blockquote>A readable quotation supplies initial page content.</blockquote>`, 1),
	} {
		d, err := Parse([]byte(raw), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(clientShellRule(d)) != 0 {
			t.Errorf("false shell: %s", raw)
		}
	}
}

func TestClientShellLoadingText(t *testing.T) {
	d, err := Parse([]byte(`<body><div id=mount></div>Loading…<script type=module></script></body>`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clientShellRule(d)) != 1 {
		t.Fatal("loading shell was missed")
	}
}

func FuzzClientShell(f *testing.F) {
	f.Add(`<body><div id=root></div><script type=module></script>`)
	f.Fuzz(func(t *testing.T, raw string) {
		d, err := Parse([]byte(raw), Options{})
		if err == nil {
			clientShellRule(d)
		}
	})
}

func FuzzUsePhrase(f *testing.F) {
	f.Add("It remembers", "remember", "en")
	f.Add("geht er nicht mehr auf", "geht nicht mehr auf", "de")
	f.Fuzz(func(t *testing.T, text, phrase, lang string) {
		d, err := Parse([]byte(`<html></html>`), Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range d.Elements {
			if n.Data == "html" {
				n.Attr = append(n.Attr, html.Attribute{Key: "lang", Val: lang})
			}
		}
		usePhrase(d, text, phrase)
	})
}
