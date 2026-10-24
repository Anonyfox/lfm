package check

import (
	"strings"
	"testing"
)

func TestRobotsHeader(t *testing.T) {
	for _, tc := range []struct {
		header          []string
		index, snippets int
	}{
		{[]string{"noindex"}, 1, 0}, {[]string{"googlebot: noindex, nosnippet, max-snippet: 10"}, 1, 1},
		{[]string{"otherbot: noindex, nosnippet"}, 0, 0}, {[]string{""}, 0, 0},
		{[]string{"max-snippet:0", "nosnippet"}, 0, 2}, {[]string{"bingbot: none"}, 2, 0},
	} {
		t.Run(strings.Join(tc.header, ";"), func(t *testing.T) {
			d, err := Parse([]byte(`<!doctype html><html><head></head><body></body></html>`), Options{RobotsHeader: tc.header, File: "pricing.html"})
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				issues []Diagnostic
				want   int
			}{{indexabilityRule(d), tc.index}, {snippetsRule(d), tc.snippets}} {
				if len(test.issues) != test.want {
					t.Fatalf("%+v want %d", test.issues, test.want)
				}
				for _, issue := range test.issues {
					if !strings.Contains(issue.Message, "X-Robots-Tag") || issue.Location.File != "pricing.html" {
						t.Fatal(issue)
					}
				}
			}
		})
	}
}
func TestProjectRuleMetadata(t *testing.T) {
	var ids []string
	for _, rule := range DefaultRules() {
		if rule.Project {
			ids = append(ids, rule.ID)
		}
	}
	if strings.Join(ids, ",") != "links.declared,jsonld.block,commerce.discovery,commerce.structured-data,concept.fidelity" {
		t.Fatal(ids)
	}
}
func FuzzRobotsHeader(f *testing.F) {
	for _, s := range []string{"googlebot: noindex, nosnippet", "max-snippet: 0", "other: none, googlebot: noindex", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := Parse([]byte(`<html><head></head><body></body></html>`), Options{RobotsHeader: []string{s}})
		if err != nil {
			t.Fatal(err)
		}
		_ = indexabilityRule(d)
		_ = snippetsRule(d)
	})
}
