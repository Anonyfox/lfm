package check

import (
	"strings"
	"testing"
)

func TestPreviewURLJudgments(t *testing.T) {
	for _, host := range []string{"localhost:8000", "127.2.3.4:8000", "[::1]:8000"} {
		local := "http://" + host
		raw := `<html><head><link rel=canonical href="https://example.org/path/"><meta property=og:url content="` + local + `/path/"><meta property=og:image content="` + local + `/image.jpg"></head><body><img src="` + local + `/image.jpg"></body></html>`
		for _, served := range []string{local + "/path/", "https://example.org/path/"} {
			d, err := Parse([]byte(raw), Options{URL: "https://example.org/path/", ServedURL: served})
			if err != nil {
				t.Fatal(err)
			}
			issues := append(socialRule(d), linksSyntaxRule(d)...)
			issues = append(issues, linksBehaviorRule(d)...)
			var bad int
			for _, issue := range issues {
				if strings.Contains(issue.Message, "local/private") || strings.Contains(issue.Message, "loopback") || strings.Contains(issue.Message, "HTTP resource") || strings.Contains(issue.Message, "differs from page canonical") {
					bad++
				}
			}
			if served == local+"/path/" && bad != 0 || served != local+"/path/" && bad == 0 {
				t.Fatalf("%s: %+v", served, issues)
			}
		}
		for _, canonical := range []string{local + "/path/", "https://example.org/path/"} {
			d, _ := Parse([]byte(`<head><link rel=canonical href="`+canonical+`"></head>`), Options{URL: canonical, ServedURL: local + "/path/"})
			for _, issue := range metadataRule(d) {
				if strings.Contains(issue.Message, "Canonical URL must") {
					t.Fatal(issue)
				}
			}
		}
	}
}

func FuzzPreviewURL(f *testing.F) {
	f.Add("http://localhost:8000/a", "https://example.org/a", "http://localhost:8000/a")
	f.Fuzz(func(t *testing.T, served, canonical, raw string) {
		d := &Document{Options: Options{ServedURL: served, URL: canonical}}
		d.previewURL(raw)
	})
}

func TestPreviewKeepsURLShapeAndExternalHTTPFindings(t *testing.T) {
	d, _ := Parse([]byte(`<head><link rel=canonical href="http://localhost:8000/path/?x=1"><meta property=og:image content="http://user:secret@localhost:8000/image#fragment"></head><body><img src="http://remote.example/image"></body>`), Options{URL: "https://example.org/path/", ServedURL: "http://localhost:8000/path/"})
	for _, tc := range []struct {
		issues []Diagnostic
		want   string
	}{{metadataRule(d), "Canonical URL must"}, {socialRule(d), "og:image must"}, {linksBehaviorRule(d), "HTTP resource"}} {
		found := false
		for _, issue := range tc.issues {
			found = found || strings.Contains(issue.Message, tc.want)
		}
		if !found {
			t.Fatalf("missing %s: %+v", tc.want, tc.issues)
		}
	}
}

func TestPreviewWithoutPublicCanonical(t *testing.T) {
	const local = "http://localhost:8000"
	raw := []byte(`<head><link rel=canonical href="` + local + `/path/"><meta property=og:url content="` + local + `/path/"><meta property=og:image content="` + local + `/image"></head><body><img src="` + local + `/image"><a href="http://localhost:8000/ok">Local</a></body>`)
	for _, identity := range []string{"", local + "/path/"} {
		d, _ := Parse(raw, Options{URL: identity, ServedURL: local + "/path/"})
		issues := append(metadataRule(d), socialRule(d)...)
		issues = append(issues, linksBehaviorRule(d)...)
		for _, issue := range issues {
			if strings.Contains(issue.Message, "Canonical URL must") || strings.Contains(issue.Message, "local/private") || strings.Contains(issue.Message, "HTTP resource") {
				t.Fatal(issue)
			}
		}
		for _, issue := range linksSyntaxRule(d) {
			if strings.Contains(issue.Message, "loopback host") {
				t.Fatal(issue)
			}
		}
		// Deferring the host must not suppress other syntax findings.
		broken, _ := Parse([]byte(strings.ReplaceAll(string(raw), "/ok", "/%zz")), d.Options)
		if len(linksSyntaxRule(broken)) == 0 {
			t.Fatal("malformed percent escape was suppressed")
		}
	}
}
