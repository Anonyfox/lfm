package markup

import (
	"slices"
	"testing"
)

var fuzzSeeds = []string{
	"",
	"---\ntitle: x\n---\n# Heading\n\nText ==field==",
	"::: title\nA\nB\n:::\n::: columns 1 2\nL\n+++\nR\n:::",
	"::: form\nName: ==x==\n:::\n::: signature\nA\n:::",
	"::: center\n::: notes\n---\n:::\n:::",
	"# A\nline\n---\n# B\n\n::: notes\nsay\n:::",
	"::: columns\n",
	":::\n:::\n:::",
	"- [ ] a\n  - b\n    - c\n      - d",
	"| a | b |\n|--:|:-:|\n| 1 | x |",
	"```\ncode\n```\n<div>x</div>\n![i](a.png \"50%\")",
	"\ufeff---\r\ntitle: x\r\n---\r\nText\r\n",
	"\"quotes\" 'single' -- ... 5 kg § 14 1. Dezember",
	"> quote\n>\n> - item\n\n***\n\n___",
	"#### deep\n\n Setext\n===",
}

func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, src string, slides bool) {
		lines := Lines([]byte(src))
		o := Options{Lang: "de", Directives: []string{"title", "columns", "form", "signature", "center", "notes"}, Slides: slides}
		blocks, problems := Parse(lines, 0, o)
		for _, p := range problems {
			if p.Line < 1 || p.Line > len(lines) {
				t.Fatalf("problem %q at line %d of %d", p.Message, p.Line, len(lines))
			}
		}
		var walk func([]Block)
		walk = func(bs []Block) {
			for _, b := range bs {
				if b.Line < 1 || b.Line > len(lines) {
					t.Fatalf("%s block at line %d of %d", b.Kind, b.Line, len(lines))
				}
				if b.Kind == Columns && len(b.Widths) != len(b.Cols) {
					t.Fatalf("columns with %d widths for %d columns", len(b.Widths), len(b.Cols))
				}
				walk(b.Children)
				for _, c := range b.Cols {
					walk(c)
				}
			}
		}
		walk(blocks)
	})
}

func FuzzFrontMatter(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Add("---\na: 1\nb\n: c\n---")
	f.Add("---\n---")
	f.Fuzz(func(t *testing.T, src string) {
		lines := Lines([]byte(src))
		settings, start, problems := FrontMatter(lines)
		if start < 0 || start > len(lines) {
			t.Fatalf("body starts at %d of %d lines", start, len(lines))
		}
		for _, p := range problems {
			if p.Line < 1 || p.Line > len(lines) {
				t.Fatalf("problem at line %d of %d", p.Line, len(lines))
			}
		}
		keys := Keys(settings)
		if len(keys) != len(settings) {
			t.Fatalf("%d keys for %d settings", len(keys), len(settings))
		}
		if !slices.IsSortedFunc(keys, func(a, b string) int { return settings[a].Line - settings[b].Line }) {
			t.Fatalf("keys out of line order: %v", keys)
		}
		for k, s := range settings {
			if s.Line < 2 || s.Line >= start {
				t.Fatalf("setting %q at line %d, body at %d", k, s.Line, start)
			}
		}
	})
}
