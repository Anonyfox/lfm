package check

import (
	"os"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/slop"
)

func intBound(n int) *int { return &n }

func TestCopyRequiresCounts(t *testing.T) {
	beacon := "<script src=\"beacon.js\"></script>"
	for _, tc := range []struct {
		name, raw string
		req       slop.Require
		want      bool
		actual    string
	}{
		{"after body", "<body></body>" + beacon, slop.Require{Snippet: beacon, Scope: "html", Before: "</body>", Exactly: intBound(1)}, true, "actual 0"},
		{"before body", "<body>" + beacon + "</BODY>", slop.Require{Snippet: beacon, Scope: "html", Before: "</body>", Exactly: intBound(1)}, false, ""},
		{"two", "<body>" + beacon + beacon + "</body>", slop.Require{Snippet: beacon, Scope: "html", Exactly: intBound(1)}, true, "actual 2"},
		{"head", "<head>" + beacon + "</head><body>" + beacon + "</body>", slop.Require{Snippet: beacon, Scope: "html", Before: "</head>", Exactly: intBound(1)}, false, ""},
		{"unicode before boundary", "<p>İ</p>" + beacon + "</body>", slop.Require{Snippet: beacon, Scope: "html", Before: "</body>", Exactly: intBound(1)}, false, ""},
		{"missing tag", beacon, slop.Require{Snippet: beacon, Scope: "html", Before: "</body>"}, false, ""},
		{"html case", "<p>Beacon</p>", slop.Require{Phrase: "beacon", Scope: "html"}, true, "actual 0"},
		{"html regex", beacon, slop.Require{Regex: "beacon[.]js", Scope: "html"}, false, ""},
		{"repeated copy", "<p>Privacy policy; privacy policy.</p>", slop.Require{Phrase: "Privacy policy", Exactly: intBound(1)}, true, "actual 2"},
		{"max", "<p>Privacy policy; privacy policy.</p>", slop.Require{Phrase: "Privacy policy", Max: intBound(1)}, true, "actual 2"},
		{"zero", "<p>Hello.</p>", slop.Require{Phrase: "Privacy policy", Exactly: intBound(0)}, false, ""},
		{"skip", "<p hidden>Privacy policy</p><script>Privacy policy</script>", slop.Require{Phrase: "Privacy policy"}, true, "actual 0"},
		{"metadata", "<head><title>Privacy policy</title></head><body></body>", slop.Require{Phrase: "Privacy policy"}, false, ""},
		{"boundaries", "<p>Privacy policies.</p>", slop.Require{Phrase: "policy"}, true, "actual 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Severity = "error"
			d, err := Parse([]byte(tc.raw), Options{File: "index.html", CopyRequires: []slop.Require{tc.req}})
			if err != nil {
				t.Fatal(err)
			}
			got := Run(d, DefaultRules(), map[string]Severity{"copy.required": Warning})
			var found []Diagnostic
			for _, v := range got.Diagnostics {
				if v.Rule == "copy.required" {
					found = append(found, v)
				}
			}
			if (len(found) > 0) != tc.want {
				t.Fatalf("findings: %+v", found)
			}
			if tc.want && (found[0].Severity != Warning || !strings.Contains(found[0].Message, tc.actual) || found[0].Hint == "") {
				t.Fatal(found)
			}
			off := Run(d, DefaultRules(), map[string]Severity{"copy.required": Off})
			for _, v := range off.Diagnostics {
				if v.Rule == "copy.required" {
					t.Fatal(v)
				}
			}
		})
	}
}

func TestMissingBeaconFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/copy_requires/missing-beacon.html")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(raw, Options{CopyRequires: []slop.Require{{Snippet: "static.cloudflareinsights.com/beacon.min.js", Scope: "html", Before: "</body>", Exactly: intBound(1), Severity: "error"}}})
	if err != nil {
		t.Fatal(err)
	}
	hits := slopRule("copy.required")(d)
	if len(hits) != 1 || !strings.Contains(hits[0].Message, "actual 0") {
		t.Fatal(hits)
	}
}

func FuzzCopyRequires(f *testing.F) {
	f.Add("\xff<script>beacon.js</script></body>", "beacon.js", "</body>")
	f.Add("<p>Privacy policy</p>", "Privacy policy", "")
	f.Add("<script src='beacon.js'></script>", "beacon.js", "</body>")
	f.Fuzz(func(t *testing.T, raw, phrase, before string) {
		if len(raw)+len(phrase)+len(before) > 32768 {
			t.Skip()
		}
		for _, scope := range []string{"copy", "html"} {
			r := slop.Require{Phrase: phrase, Before: before, Scope: scope, Severity: "error"}
			if _, err := r.Validate(); err != nil {
				continue
			}
			d, err := Parse([]byte(raw), Options{CopyRequires: []slop.Require{r}})
			if err != nil {
				return
			}
			slopRule("copy.required")(d)
		}
	})
}
