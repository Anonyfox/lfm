package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceLocationMatchesDirectLookup(t *testing.T) {
	for _, raw := range []string{"", "a\nb\r\nc\rd", "é🐺\nend\n"} {
		indexed := NewSource("input", []byte(raw))
		for offset := -1; offset <= len(raw)+1; offset++ {
			if indexed.At(offset) != At("input", []byte(raw), offset) {
				t.Fatal(raw, offset, indexed.At(offset), At("input", []byte(raw), offset))
			}
		}
	}
}
func TestSummaryAndFormatting(t *testing.T) {
	a := Diagnostic{Rule: "a", Severity: Warning, Location: Location{File: "z", Line: 1, Column: 1}, Message: "empty", Hint: "fix", Related: []Location{{File: "lfm.json", Line: 2, Column: 3}}}
	b := Diagnostic{Rule: "b", Severity: Error, Location: Location{File: "a", Offset: 9}}
	c := Diagnostic{Rule: "c", Severity: Error, Location: Location{File: "a", Offset: 1}}
	s := Summarize([]Diagnostic{a, b, c})
	if s.Errors != 2 || s.Warnings != 1 || s.Diagnostics[0].Rule != "c" || len(HardOnly(s.Diagnostics)) != 2 {
		t.Fatal(s)
	}
	if !strings.Contains(a.String(), "Related: lfm.json:2:3") {
		t.Fatal(a.String())
	}
	raw, err := json.Marshal(s)
	if err != nil || !strings.Contains(string(raw), `"related"`) {
		t.Fatal(string(raw), err)
	}
}

func TestReadableReportIncludesConfigPathOnce(t *testing.T) {
	d := Diagnostic{Rule: "commerce.price", Severity: Error, Path: "data.commerce.offers.audit.price.amountMinor", Message: "Amount is invalid.", Hint: "Use an integer."}
	if strings.Count(d.String(), d.Path) != 1 {
		t.Fatal(d.String())
	}
	d.Message = d.Path + " is invalid."
	if strings.Count(d.String(), d.Path) != 1 {
		t.Fatal(d.String())
	}
}
