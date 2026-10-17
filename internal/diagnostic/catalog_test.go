package diagnostic

import "testing"

func TestDynamicLighthouseOverrideSpecificity(t *testing.T) {
	issues := []Diagnostic{{Rule: "lighthouse.foo", Severity: Warning}, {Rule: "lighthouse.bar", Severity: Warning}, {Rule: "release.browser", Severity: Warning}}
	got := Apply(issues, map[string]Severity{"lighthouse.*": Off, "lighthouse.foo": Error, "release.browser": Off})
	if len(got) != 1 || got[0].Rule != "lighthouse.foo" || got[0].Severity != Error {
		t.Fatal(got)
	}
}
