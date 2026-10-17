package diagnostic

import "strings"

// OperationalRules are emitted outside individual HTML/template checks.
// Lighthouse owns a versioned audit namespace, so its concrete IDs are dynamic.
func OperationalRules() []RuleInfo {
	return []RuleInfo{
		{
			ID:       "build.failed",
			Summary:  "Reading inputs, bundling assets or writing output completed without an error",
			Help:     "Reported when the compile stops on an operational error such as an unreadable file, a path outside the project or an esbuild failure. Nothing is published and the previous output/ stays in place. Fix the named file, path or bundler error and compile again.",
			Required: true,
		},
		{
			ID:      "assets.hint",
			Summary: "Resource hints that point at local files the bundled page never uses",
			Help:    "A preload, prefetch or similar hint names a local resource that no retained stylesheet, script or element references, so the compiler removed it. A stale hint costs visitors a wasted request. Reference the resource where it is actually consumed, or delete the hint from the template.",
		},
		{
			ID:      "bundle.warning",
			Summary: "Warnings that esbuild reported while bundling scripts and styles",
			Help:    "esbuild accepted the source but flagged something suspicious, such as a duplicate object key, a comparison that is always false or CSS it could not parse. These usually point at a real bug in the shipped script or stylesheet. Fix the code at the named source location; turn the rule off only for a warning you have reviewed and accept.",
		},
		{
			ID:      "html.compaction",
			Summary: "Safe HTML compaction skipped, so the serialized HTML ships unminified",
			Help:    "The compactor found input it could not shrink without risk and kept the serialized HTML byte for byte. The page is still correct and fully checked, only slightly larger. This indicates a compactor limitation, so report the input as an lfm bug.",
		},
		{
			ID:      "release.css",
			Summary: "The --release critical-CSS pass in headless Chrome finished",
			Help:    "With --release, lfm measures above-the-fold CSS in Chrome so the first paint does not wait for the full stylesheet. When that pass fails, the complete CSS is kept and the page still works, but first render is slower. Check that Chrome starts and the page renders, then compile with --release again.",
		},
		{
			ID:      "release.browser",
			Summary: "A Chrome browser was available for the --release checks",
			Help:    "--release needs Chrome or Chromium for critical CSS, the runtime probe and Lighthouse; when none is found or navigation fails, those checks are skipped and the page ships untested in a real browser. Install Chrome/Chromium on PATH or set LFM_CHROME to its executable. Set this rule to error if a release must never ship without the browser checks.",
		},
		{
			ID:      "release.runtime",
			Summary: "Uncaught exceptions, console errors and failed requests on first page load",
			Help:    "With --release, lfm loads the exact artifact in Chrome at phone size and records uncaught exceptions, console.error calls and HTTP responses of 400 or above. Each one is something visitors hit on arrival, such as a broken script or a missing generated file. Fix the named exception or asset reference; the probe covers initial navigation only, not later interactions.",
		},
		{
			ID:      "release.layout",
			Summary: "Every frame of the page is legible, nothing scrolls sideways and values stay whole",
			Help:    "With --release, lfm scrolls the page in Chrome by half a screen at a time at 390×844 (phone), 1280×680 and 1440×900, lets finite animations settle, and saves a screenshot of every frame. It warns when a stretch of the page shows no legible text or image (empty or faded scroll scenes, oversized spacing), when the page is wider than the screen, and when a number breaks onto a different line than its unit. These defects hide in chosen screenshots and are what visitors scroll through. Fix the named element, then read the saved frames top to bottom at every size.",
		},
		{
			ID:      "release.lighthouse",
			Summary: "Installed Lighthouse ran and produced a complete audit",
			Help:    "With --release, lfm runs an installed Lighthouse against the artifact; this warns when the executable is missing or the run fails, times out or returns incomplete JSON. Without it you get no performance, accessibility, best-practice or SEO scores for this build. Install it once with npm install -g lighthouse (or set LFM_LIGHTHOUSE) and check that the Node, Lighthouse and Chrome versions are compatible.",
		},
		{
			ID:      "lighthouse.*",
			Summary: "Findings from the installed Lighthouse audit, one rule per lighthouse.<audit-id>",
			Help:    "With --release, every Lighthouse audit that scores below 100%, errors or carries warnings becomes a warning named lighthouse.<audit-id>, with the audit's own guidance and sample elements. They measure one cold load on a simulated phone over a throttled mobile network, which is close to how many visitors first meet the page. Follow the audit's hint and the saved full report. Set \"lighthouse.*\" to change every audit's level at once; an exact lighthouse.<audit-id> override takes precedence.",
		},
		{
			ID:      "lighthouse.run",
			Summary: "Lighthouse run warnings and report or browser-profile cleanup problems",
			Help:    "Lighthouse completed but reported run warnings, or lfm could not save the full JSON report or remove the temporary Chrome profile. Scores may be less trustworthy and the detailed report may be missing. Read the warning, check the audit environment (disk space, temp directory, tool versions) and rerun --release.",
		},
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
