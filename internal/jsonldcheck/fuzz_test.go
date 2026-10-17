package jsonldcheck

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Property: Expand has a deterministic, time-bounded outcome, never needs its panic guard,
// and only a usable graph comes with no error findings; Check agrees with it.
func FuzzExpandOutcome(f *testing.F) {
	for _, raw := range []string{`{}`, `{"@context":"https://schema.org","@type":"Person","name":"Name"}`, `{"@context":{"x":"x"},"x":1}`, `[]`, `null`, `{"@reverse":[]}`,
		`{"@context":"http://schema.org/","@graph":[{"@id":"https://example.com/#one","@type":"Person","name":"One"},{"@id":"https://example.com/#two","knows":{"@id":"https://example.com/#one"}}]}`,
		`{"@context":{"ex":"https://example.com/"},"@id":"_:x","ex:list":{"@list":["a","b"]}}`, `{"@context":{"a":"b","b":"a"},"a":"cycle"}`, `{"@context":{"@protected":1,"a":"b"},"a":1}`, `{"@context":{"a":{"@id":"https://e.x/a","@container":["@index",1],"@protected":true}},"a":{"x":"y"}}`,
		`{"@context":{"r":{"@reverse":"https://e.x/r"}},"@reverse":{"https://e.x/p":{"@id":"z"}},"r":[{"@id":"y"}]}`, `{"@context":{"i":{"@id":"https://e.x/i","@container":"@index"}},"i":{"k":[{"@value":"v"}]}}`,
		`{"@context":{"@vocab":"https://example.com/","p":{"@type":"@id","@container":"@set"}},"p":["a","b"],"@included":[{"@id":"c"}]}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		start := time.Now()
		expanded, findings := Expand([]byte(raw), "https://example.com/")
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Expand took %v", elapsed)
		}
		// JSON-goLD defines context terms in map order, so with several invalid
		// terms the reported error code may vary; the outcome must not.
		again, repeated := Expand([]byte(raw), "https://example.com/")
		if !reflect.DeepEqual(expanded, again) || !reflect.DeepEqual(outcome(findings), outcome(repeated)) || !reflect.DeepEqual(outcome(findings), outcome(Check([]byte(raw), "https://example.com/"))) {
			t.Fatal("expansion is not deterministic")
		}
		for _, finding := range findings {
			if finding.Message == "JSON-LD processor could not safely analyze this document." {
				t.Fatal("JSON-GoLD panicked")
			}
			if expanded != nil && !finding.Warning {
				t.Fatalf("error finding with a usable graph: %+v", finding)
			}
		}
		if _, err := json.Marshal(expanded); err != nil {
			t.Fatal(err)
		}
	})
}

func outcome(findings []Finding) []string {
	var rules []string
	for _, finding := range findings {
		rules = append(rules, fmt.Sprint(finding.Rule, finding.Warning))
	}
	return rules
}
