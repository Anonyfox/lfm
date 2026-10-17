package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRulesExplainsOneRule(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"links.declared", `"links.declared": "off"`},
		{"build.failed", "Required: yes."},
		{"lighthouse.uses-long-cache-ttl", `"lighthouse.uses-long-cache-ttl": "off"`},
	} {
		var out bytes.Buffer
		if err := printRules(&out, tc.id); err != nil {
			t.Fatal(tc.id, err)
		}
		text := out.String()
		if !strings.HasPrefix(text, tc.id+"\n") || !strings.Contains(text, tc.want) || strings.Count(text, "\n\n") < 2 {
			t.Errorf("%s:\n%s", tc.id, text)
		}
	}
	var out bytes.Buffer
	if err := printRules(&out, "made.up"); err == nil {
		t.Fatal("unknown rule accepted")
	}
}
