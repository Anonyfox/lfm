package jsonldcheck

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExpansion(t *testing.T) {
	for _, document := range []string{
		`{"@context":"https://schema.org","@type":"WebPage","name":"Title"}`,
		`{"@context":"http://schema.org/","@graph":[{"@id":"https://example.com/#one","@type":"Person","name":"One"},{"@id":"https://example.com/#two","knows":{"@id":"https://example.com/#one"}}]}`,
		`[{"@context":{"ex":"https://example.com/","label":"ex:label"},"@type":"ex:Custom","label":"One"}]`,
		`{"@context":"https://schema.org","@type":["Book","Product"],"name":"Book","author":"Plain text is allowed"}`,
		`{"@context":"https://schema.org","@type":"NotARealType","unknownProperty":"Not vocabulary checked"}`,
		`{"@context":"https://schema.org","@type":"WebPage","identifier":9007199254740993}`,
		`{"@context":"https://schema.org","@type":"WebPage","name":{"@value":"Hello","@language":"en"}}`,
		`{"@context":{"ex":"https://example.com/"},"@id":"_:x","ex:list":{"@list":["a","b"]}}`,
	} {
		if findings := Check([]byte(document), "https://example.com/"); len(findings) != 0 {
			t.Fatalf("%s: %+v", document, findings)
		}
	}
}

func TestStructuralErrors(t *testing.T) {
	for _, document := range []string{
		`42`, `"not a node"`,
		`{"@context":"https://schema.org","@type":42,"name":"Bad"}`,
		`{"@context":"https://schema.org","@id":{},"name":"Bad"}`,
		`{"@context":123,"name":"Bad"}`,
		`{"@context":"https://schema.org","name":{"@value":{},"@type":"Text"}}`,
		`{"@context":"https://schema.org","name":{"@value":"x","@language":"en","@type":"Text"}}`,
		`{"@context":{"a":"b","b":"a"},"a":"cycle"}`,
	} {
		findings := Check([]byte(document), "https://example.com/")
		found := false
		for _, issue := range findings {
			found = found || issue.Rule == "jsonld.structure" && !issue.Warning
		}
		if !found {
			t.Fatalf("missed %s: %+v", document, findings)
		}
	}
}

func TestOfflineContextsAndDroppedData(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"@context":{"@vocab":"https://example.com/"}}`)
	}))
	defer server.Close()
	if findings := Check([]byte(fmt.Sprintf(`{"@context":%q,"name":"No fetch"}`, server.URL)), ""); len(findings) != 1 || findings[0].Rule != "jsonld.context" || requests.Load() != 0 {
		t.Fatalf("offline check fetched a context: %d %+v", requests.Load(), findings)
	}
	for _, context := range []string{"https://not-bundled.example/context", "file:///tmp/secret", "../context.json", "https://schema.org/?untrusted"} {
		findings := Check([]byte(fmt.Sprintf(`{"@context":%q,"name":"Value"}`, context)), "https://example.com/")
		if len(findings) != 1 || findings[0].Rule != "jsonld.context" || !findings[0].Warning {
			t.Fatalf("%s: %+v", context, findings)
		}
	}
	for _, context := range []string{"http://schema.org", "https://schema.org/", "https://schema.org/docs/jsonldcontext.json", "http://schema.org/docs/jsonldcontext.jsonld"} {
		if findings := Check([]byte(fmt.Sprintf(`{"@context":%q,"name":"Value"}`, context)), ""); len(findings) != 0 {
			t.Fatal(findings)
		}
	}
	if findings := Check([]byte(`{"name":"Dropped without a context"}`), ""); len(findings) == 0 || findings[0].Rule != "jsonld.dropped" {
		t.Fatal(findings)
	}
	if findings := Check([]byte(`{}`), ""); len(findings) != 1 || findings[0].Rule != "jsonld.dropped" {
		t.Fatal(findings)
	}
	if findings := Check([]byte(`{"name":}`), ""); len(findings) != 0 {
		t.Fatal("duplicate JSON syntax diagnostic")
	}
	deep := strings.Repeat(`{"https://example.com/child":`, 130) + `"leaf"` + strings.Repeat(`}`, 130)
	if findings := Check([]byte(deep), ""); len(findings) != 1 || findings[0].Rule != "jsonld.structure" {
		t.Fatal(findings)
	}
}

func TestPinnedContextIntegrity(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(schemaContext)); got != "65d0c5d17def9b171b7697bb0929eed17aa32f19af539e15302b6c20479b8930" {
		t.Fatal("context snapshot changed without provenance update", got)
	}
}

func TestParallelExpansion(t *testing.T) {
	for i := 0; i < 16; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			if findings := Check([]byte(`{"@context":"https://schema.org","@type":"Person","name":"Name"}`), ""); len(findings) != 0 {
				t.Fatal(findings)
			}
		})
	}
}

func BenchmarkSchemaExpansion(b *testing.B) {
	raw := []byte(`{"@context":"https://schema.org","@type":"WebPage","url":"https://example.com/","name":"Example","description":"Description","inLanguage":"en"}`)
	for b.Loop() {
		if findings := Check(raw, "https://example.com/"); len(findings) != 0 {
			b.Fatal(findings)
		}
	}
}

func FuzzExpansion(f *testing.F) {
	for _, raw := range []string{`{}`, `{"@context":"https://schema.org","@type":"Person","name":"Name"}`, `{"@context":{"x":"x"},"x":1}`, `[]`, `null`, `{"@reverse":[]}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 16384 {
			t.Skip()
		}
		Check([]byte(raw), "https://example.com/")
	})
}

func TestPrunedContextExpandsLikeTheFullContext(t *testing.T) {
	docs := []string{
		`{"@context":"https://schema.org","@type":"Product","name":"Kit","offers":{"@type":"Offer","price":"9.00","priceCurrency":"EUR","availability":"https://schema.org/InStock","url":"https://example.com/"},"image":"https://example.com/a.png","sameAs":["https://x.com/a"]}`,
		`{"@context":"https://schema.org","@graph":[{"@type":"Organization","@id":"#org","name":"A","logo":"logo.png"},{"@type":"WebPage","url":"https://example.com/","publisher":{"@id":"#org"},"datePublished":"2026-10-18","inLanguage":"en"}]}`,
		`{"@context":["https://schema.org",{"x":"http://example.org/x"}],"@type":"Event","name":"N","startDate":"2026-11-13T19:00","eventAttendanceMode":"OfflineEventAttendanceMode","location":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Hamburg"}},"x":"y"}`,
		`{"@context":"https://schema.org","@type":["Service","HowTo"],"name":"Plan","step":[{"@type":"HowToStep","text":"Mark the hours"}],"potentialAction":{"@type":"BuyAction","target":"https://buy.stripe.com/x"},"schema:description":"compact IRI","additionalType":"Product"}`,
		`{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[{"@type":"Question","name":"Q?","acceptedAnswer":{"@type":"Answer","text":"A"}}],"dateModified":"2026-10-18","about":{"@type":"Thing","name":"birthDate"}}`,
	}
	for i, doc := range docs {
		full, fullFindings := expand([]byte(doc), "https://example.com/", true)
		pruned, prunedFindings := expand([]byte(doc), "https://example.com/", false)
		a, _ := json.Marshal(full)
		b, _ := json.Marshal(pruned)
		if string(a) != string(b) || len(fullFindings) != len(prunedFindings) {
			t.Errorf("doc %d differs:\nfull   %s %v\npruned %s %v", i, a, fullFindings, b, prunedFindings)
		}
	}
}

func FuzzPrunedContextExpandsLikeTheFullContext(f *testing.F) {
	f.Add(`{"@context":"https://schema.org","@type":"Product","name":"Kit","offers":{"price":"1"}}`)
	f.Add(`{"@context":"https://schema.org","@type":"Person","birthDate":"2000-01-01","knows":"Thing"}`)
	f.Fuzz(func(t *testing.T, doc string) {
		full, fullFindings := expand([]byte(doc), "https://example.com/", true)
		pruned, prunedFindings := expand([]byte(doc), "https://example.com/", false)
		a, _ := json.Marshal(full)
		b, _ := json.Marshal(pruned)
		if string(a) != string(b) || len(fullFindings) != len(prunedFindings) {
			t.Fatalf("differs for %s:\n%s\n%s", doc, a, b)
		}
	})
}
