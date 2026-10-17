package diagnostic

import (
	"strings"
	"testing"
)

func TestDynamicLighthouseOverrideSpecificity(t *testing.T) {
	issues := []Diagnostic{{Rule: "lighthouse.foo", Severity: Warning}, {Rule: "lighthouse.bar", Severity: Warning}, {Rule: "release.browser", Severity: Warning}}
	got := Apply(issues, map[string]Severity{"lighthouse.*": Off, "lighthouse.foo": Error, "release.browser": Off})
	if len(got) != 1 || got[0].Rule != "lighthouse.foo" || got[0].Severity != Error {
		t.Fatal(got)
	}
}

func TestSummarizeCollapsesRepeatsAtOnePlace(t *testing.T) {
	at := Location{File: "index.html.tmpl", Line: 4, Column: 2, Offset: 30}
	issues := []Diagnostic{
		{Rule: "template.bindings", Severity: Error, Location: at, Message: "Cannot read data.a[0].b from json.Number.", Hint: "h"},
		{Rule: "template.bindings", Severity: Error, Location: at, Message: "Cannot read data.a[1].b from json.Number.", Hint: "h"},
		{Rule: "template.bindings", Severity: Error, Location: at, Message: "Something else.", Hint: "h"},
		{Rule: "template.bindings", Severity: Error, Location: Location{File: "index.html.tmpl", Line: 9, Offset: 90}, Message: "Cannot read data.a[2].b from json.Number.", Hint: "h"},
	}
	s := Summarize(issues)
	if len(s.Diagnostics) != 3 || s.Errors != 3 || !strings.HasSuffix(s.Diagnostics[0].Message, "(2 times here)") && !strings.HasSuffix(s.Diagnostics[1].Message, "(2 times here)") {
		t.Fatalf("%+v", s)
	}
}
