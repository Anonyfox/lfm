package diagnostic

import "strings"

// OperationalRules are emitted outside individual HTML/template checks.
// Lighthouse owns a versioned audit namespace, so its concrete IDs are dynamic.
func OperationalRules() []RuleInfo {
	return []RuleInfo{
		{ID: "build.failed", Description: "Input, bundling or output operation could not complete", Required: true},
		{ID: "assets.hint", Description: "Unused local resource hint removed during reconciliation"},
		{ID: "bundle.warning", Description: "Advisories returned by esbuild"},
		{ID: "html.compaction", Description: "Safe HTML compaction was skipped; original bytes retained"},
		{ID: "release.css", Description: "Optional critical-CSS browser pass could not finish"},
		{ID: "release.browser", Description: "Optional browser unavailable or navigation probe skipped"},
		{ID: "release.runtime", Description: "Browser startup exceptions and missing artifact resources"},
		{ID: "release.lighthouse", Description: "Installed Lighthouse unavailable or audit could not finish"},
		{ID: "lighthouse.*", Description: "Dynamic findings from the installed Lighthouse audit catalog"},
		{ID: "lighthouse.run", Description: "Lighthouse execution warnings and report cleanup problems"},
	}
}

func DynamicRule(id string) bool {
	name, ok := strings.CutPrefix(id, "lighthouse.")
	if !ok || name == "" {
		return false
	}
	if name == "*" {
		return true
	}
	for j, c := range name {
		if !(c >= 'a' && c <= 'z' || j > 0 && (c >= '0' && c <= '9' || c == '-')) {
			return false
		}
	}
	return true
}

func Level(id string, overrides map[string]Severity) Severity {
	if level := overrides[id]; level != "" {
		return level
	}
	if DynamicRule(id) {
		return overrides["lighthouse.*"]
	}
	return ""
}

// Apply makes lifecycle notices obey the same policy as normal quality rules.
// Required execution failures are never passed here as optional notices.
func Apply(issues []Diagnostic, overrides map[string]Severity) []Diagnostic {
	result := make([]Diagnostic, 0, len(issues))
	for _, issue := range issues {
		if level := Level(issue.Rule, overrides); level != "" {
			issue.Severity = level
		}
		if issue.Severity != Off {
			result = append(result, issue)
		}
	}
	return result
}
