package urlcheck

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// exactSpan reports whether raw[ref.Offset:ref.End] is the source text of ref.Value,
// for reference kinds that point at their value rather than at the containing tag.
func exactSpan(t *testing.T, raw string, ref Reference) {
	t.Helper()
	if ref.Offset < 0 || ref.End < ref.Offset || ref.End > len(raw) {
		t.Fatalf("bad source span %+v", ref)
	}
	span := raw[ref.Offset:ref.End]
	switch {
	case strings.HasPrefix(ref.Attribute, "JSON "):
		var value string
		if err := json.Unmarshal([]byte(span), &value); err != nil || value != ref.Value {
			t.Fatalf("JSON span %q does not encode %q (%v)", span, ref.Value, err)
		}
	case ref.Attribute == "CSS URL":
		if !strings.Contains(span, `\`) && span != ref.Value {
			t.Fatalf("CSS span %q is not %q", span, ref.Value)
		}
	case !strings.Contains(ref.Attribute, " ") && ref.Attribute != "ping" && ref.Attribute != "archive":
		if !strings.Contains(span, "&") && span != ref.Value {
			t.Fatalf("attribute span %q is not %q", span, ref.Value)
		}
	}
}

// Property: every reference's source span lies in the input and spells its value.
func FuzzReferenceSpans(f *testing.F) {
	for _, seed := range []string{
		`<a href="https://example.org/">go</a>`, `<img srcset="data:x,abc 1x, image 2x">`, `<meta http-equiv=refresh content="0; url='https://x.example/'">`,
		"<script type=application/ld+json>{\r\n\"url\": \"https://x.example/\"}</script>",
		"<style>\r\n.x{background:url( 'https://cdn.example/a.webp' )}</style>", `<svg><a xlink:href=#x>go</a></svg>`,
		`<link rel=stylesheet href='a.css'><div style="background:url(b.png)"></div>`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		for _, ref := range AllReferences([]byte(raw)) {
			exactSpan(t, raw, ref)
		}
	})
}

// Property: CSS and JSON references are offset-relative and spell their values.
func FuzzEmbeddedReferences(f *testing.F) {
	for _, seed := range []string{
		`@import "https://cdn.example/base.css"; .hero{background:URL("ht\74 ps://cdn.example/image.webp")}`,
		`.a{background-image:image-set("one.webp" 1x,url( two.webp ) 2x)}@namespace url(x);`,
		`{"url":"../other/","@id":"#organization","sameAs":["/profile/"],"appUri":"myapp:item/1"}`,
		"{\n  \"image\" :\t\"https://x.example/\\u0041\" , \"@context\":{\"url\":\"https://schema.org/url\"}}",
	} {
		f.Add(seed, 7)
	}
	f.Fuzz(func(t *testing.T, raw string, offset int) {
		if len(raw) > 8192 || offset < 0 || offset > 1<<20 {
			t.Skip()
		}
		for _, extract := range []func([]byte, int) []Reference{CSSReferences, JSONReferences} {
			base, shifted := extract([]byte(raw), 0), extract([]byte(raw), offset)
			if len(base) != len(shifted) {
				t.Fatal("offset changed the references found")
			}
			for i, ref := range base {
				exactSpan(t, raw, ref)
				ref.Offset, ref.End = ref.Offset+offset, ref.End+offset
				if ref != shifted[i] {
					t.Fatalf("offset %d: %+v, want %+v", offset, shifted[i], ref)
				}
			}
		}
	})
}

// Property: srcset candidates are nonempty, ordered, whitespace-free substrings at their offsets.
func FuzzCandidates(f *testing.F) {
	for _, seed := range []string{"data:x,abc 1x, image 2x", "a.png,b.png 2x", " a (1x, 2x), b,,", "x\f1x,\ty 100w", "data:image/svg+xml,<svg/>,"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		last := -1
		for _, c := range Candidates(value) {
			if c.URL == "" || c.Offset <= last || c.Offset+len(c.URL) > len(value) || value[c.Offset:c.Offset+len(c.URL)] != c.URL ||
				strings.ContainsAny(c.URL, " \t\r\n\f") || strings.HasSuffix(c.URL, ",") || strings.HasPrefix(c.URL, ",") {
				t.Fatalf("bad candidate %+v in %q", c, value)
			}
			last = c.Offset
		}
	})
}

// Property: a refresh destination is the substring at its offset, and an
// explicit unquoted url= value is returned verbatim.
func FuzzRefreshURL(f *testing.F) {
	for _, seed := range []string{"0; url=https://example.org/", "5", " 1.5 , URL = 'x' y", "0;ufo", ".;x", "3 url"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if url, offset, ok := RefreshURL(value); ok && (offset < 0 || offset+len(url) > len(value) || value[offset:offset+len(url)] != url) {
			t.Fatalf("%q: destination %q not at offset %d", value, url, offset)
		}
		if value == "" || strings.ContainsAny(value[:1], " \t\r\n\f'\"") {
			return
		}
		if url, offset, ok := RefreshURL("0; url=" + value); !ok || url != value || offset != len("0; url=") {
			t.Fatalf("explicit destination %q became %q at %d (%v)", value, url, offset, ok)
		}
	})
}

// Property: Inspect is deterministic and its per-analyzer cache never changes a result.
func FuzzInspectCache(f *testing.F) {
	for _, seed := range []string{"https://example.org/a?x=1", "../a b", "mailto:?subject=%ZZ", "javascript:alert(1)", "https://[::1]:99999/"} {
		f.Add(seed, "https://example.org/other")
	}
	f.Fuzz(func(t *testing.T, raw, other string) {
		if len(raw)+len(other) > 8192 {
			t.Skip()
		}
		warm := New("https://example.org/base/")
		warm.Inspect(other)
		cached, fresh := warm.Inspect(raw), New("https://example.org/base/").Inspect(raw)
		if !reflect.DeepEqual(cached.Findings, fresh.Findings) || (cached.URL == nil) != (fresh.URL == nil) || cached.URL != nil && cached.URL.Href(false) != fresh.URL.Href(false) {
			t.Fatalf("cached %+v, fresh %+v", cached, fresh)
		}
		if again := warm.Inspect(raw); !reflect.DeepEqual(again.Findings, cached.Findings) {
			t.Fatal("cache hit differs from first result")
		}
	})
}
