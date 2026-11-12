package cli

import (
	"strings"
	"testing"
)

func TestBrowserJobsUsage(t *testing.T) {
	for _, value := range []string{"0", "-1", "no"} {
		code, _, errs := run(t, "compile", "--browser-jobs", value)
		if code != 2 || !strings.Contains(errs, "--browser-jobs") {
			t.Fatalf("%s: %d %s", value, code, errs)
		}
	}
}

func TestValidateImpliesRelease(t *testing.T) {
	t.Setenv("LFM_CHROME", "/missing/chrome")
	dir := examplePage(t)
	code, out, _ := run(t, "compile", dir, "--validate", "--json")
	if code != 1 || !strings.Contains(out, `"validated": true`) || !strings.Contains(out, "release.browser") {
		t.Fatalf("%d %s", code, out)
	}
}
