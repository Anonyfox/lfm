package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{nil, 0, "Usage:", ""},
		{[]string{"help"}, 0, "Usage:", ""},
		{[]string{"--help"}, 0, "Usage:", ""},
		{[]string{"version"}, 0, "lfm dev", ""},
		{[]string{"--version"}, 0, "lfm dev", ""},
		{[]string{"nope"}, 2, "", `unknown command "nope"`},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if code := run(c.args, &out, &errOut); code != c.code {
			t.Errorf("run(%q) exit = %d, want %d", c.args, code, c.code)
		}
		if !strings.Contains(out.String(), c.stdout) {
			t.Errorf("run(%q) stdout = %q, want substring %q", c.args, out.String(), c.stdout)
		}
		if !strings.Contains(errOut.String(), c.stderr) {
			t.Errorf("run(%q) stderr = %q, want substring %q", c.args, errOut.String(), c.stderr)
		}
	}
}
