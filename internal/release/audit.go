package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

const LighthouseSetup = "Install Node >=22.19 and run npm install -g lighthouse@13.5.0 once; ensure lighthouse is in PATH (or set LFM_LIGHTHOUSE to its executable). Builds never install/download tools."
const BrowserSetup = "Install Chrome/Chromium or a Chromium headless shell in PATH (Debian: apt install chromium), or set LFM_CHROME to its executable; retry --release."

type AuditResult struct {
	Runtime     *RuntimeResult          `json:"runtime,omitempty"`
	Lighthouse  *LighthouseResult       `json:"lighthouse"`
	Diagnostics []diagnostic.Diagnostic `json:"-"`
}
type LighthouseResult struct {
	Status     string            `json:"status"`
	Reason     string            `json:"reason,omitempty"`
	Hint       string            `json:"hint,omitempty"`
	Version    string            `json:"version,omitempty"`
	Chrome     string            `json:"chrome,omitempty"`
	Scope      string            `json:"scope"`
	Report     string            `json:"report,omitempty"`
	DurationMS int64             `json:"durationMs"`
	Scores     map[string]*int   `json:"scores,omitempty"`
	Metrics    map[string]string `json:"metrics,omitempty"`
	Findings   []AuditFinding    `json:"findings,omitempty"`
	Manual     []AuditFinding    `json:"manualChecks,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}
type AuditFinding struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Hint       string   `json:"hint"`
	Value      string   `json:"value,omitempty"`
	Categories []string `json:"scoredCategories,omitempty"`
	Score      *float64 `json:"score,omitempty"`
	Samples    []string `json:"samples,omitempty"`
	Items      int      `json:"detailItems,omitempty"`
}

func Audit(page []byte, assets map[string][]byte, root string) *AuditResult {
	r := &AuditResult{}
	warn := func(rule, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	browser, err := BrowserPath()
	_, toolErr := lighthousePath(root)
	if err != nil {
		r.Runtime = &RuntimeResult{Status: "skipped", Reason: err.Error(), Hint: BrowserSetup}
		r.Lighthouse = &LighthouseResult{Status: "skipped", Reason: "browser unavailable", Hint: BrowserSetup, Scope: auditScope}
		warn("release.browser", "Critical CSS, runtime and Lighthouse browser features skipped: "+err.Error(), BrowserSetup)
		if toolErr != nil {
			r.Lighthouse.Reason = "Chrome and Lighthouse unavailable"
			r.Lighthouse.Hint = BrowserSetup + " " + LighthouseSetup
			warn("release.lighthouse", "Lighthouse executable also unavailable: "+toolErr.Error(), LighthouseSetup)
		}
		return r
	}
	r.Runtime = AuditRuntime(page, assets, browser, FramesPath(root))
	if r.Runtime.Status != "ok" {
		warn("release.browser", "Runtime navigation check skipped: "+r.Runtime.Reason, r.Runtime.Hint)
	}
	for _, problem := range r.Runtime.Problems {
		warn("release.runtime", problem, "Fix the named browser exception or missing generated asset; this check observes initial navigation only.")
	}
	if l := r.Runtime.Layout; l != nil {
		for _, f := range l.Findings {
			hint := f.Hint
			if l.Count > 0 {
				hint += " Frames: " + l.Frames
			}
			warn("release.layout", f.Message, hint)
		}
	}
	if r.Runtime.Status != "ok" && r.Runtime.Chrome == "" {
		r.Lighthouse = &LighthouseResult{Status: "skipped", Reason: "Chrome could not start for navigation", Hint: r.Runtime.Hint, Scope: auditScope}
		if toolErr != nil {
			r.Lighthouse.Hint += " " + LighthouseSetup
			warn("release.lighthouse", "Lighthouse executable also unavailable: "+toolErr.Error(), LighthouseSetup)
		}
		return r
	}
	r.Lighthouse = runLighthouse(page, assets, browser, root)
	if r.Lighthouse.Status != "ok" {
		warn("release.lighthouse", "Lighthouse skipped: "+r.Lighthouse.Reason, r.Lighthouse.Hint)
	}
	r.Diagnostics = append(r.Diagnostics, LighthouseDiagnostics(r.Lighthouse)...)
	return r
}

func LighthouseDiagnostics(lh *LighthouseResult) []diagnostic.Diagnostic {
	if lh == nil {
		return nil
	}
	var result []diagnostic.Diagnostic
	for _, f := range lh.Findings {
		message := f.Title
		if f.Value != "" {
			message += ": " + f.Value
		}
		if len(f.Categories) > 0 {
			message += " (scored: " + strings.Join(f.Categories, ", ") + ")"
		} else {
			message += " (diagnostic)"
		}
		hint := f.Hint
		for _, sample := range f.Samples {
			hint += "\n  " + sample
		}
		if f.Items > len(f.Samples) {
			hint += fmt.Sprintf("\n  %d detail rows; full audit: %s", f.Items, lh.Report)
		}
		result = append(result, diagnostic.Diagnostic{Rule: "lighthouse." + f.ID, Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	for _, warning := range lh.Warnings {
		result = append(result, diagnostic.Diagnostic{Rule: "lighthouse.run", Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: warning, Hint: "Review the installed audit's environment and saved report."})
	}
	return result
}

const auditScope = "One fresh mobile Lighthouse navigation; simulated mobile CPU/network; exact artifact on local HTTP; external traffic and non-GET requests blocked. Delivery/TLS/CDN and interaction journeys are not measured."

func lighthousePath(root string) (string, error) {
	if path := os.Getenv("LFM_LIGHTHOUSE"); path != "" {
		return exec.LookPath(path)
	}
	if path, err := exec.LookPath("lighthouse"); err == nil {
		return path, nil
	}
	for _, name := range []string{"lighthouse", "lighthouse.cmd"} {
		path := filepath.Join(root, "node_modules", ".bin", name)
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("installed Lighthouse executable not found")
}

func runLighthouse(page []byte, assets map[string][]byte, browser, root string) *LighthouseResult {
	start := time.Now()
	r := &LighthouseResult{Status: "skipped", Scope: auditScope}
	defer func() { r.DurationMS = time.Since(start).Milliseconds() }()
	tool, err := lighthousePath(root)
	if err != nil {
		r.Reason, r.Hint = err.Error(), LighthouseSetup
		return r
	}
	// Chrome's proxy sends ALL HTTP(S), including loopback, through this deny-by-
	// default artifact server. It never forwards requests. HTTPS CONNECT, outside
	// origins, mutations and WebSocket upgrades are rejected. No user profile.
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	server.Config.Handler = AuditHandler(page, assets, origin)
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	profile, err := os.MkdirTemp("", "lfm-lighthouse-chrome-")
	if err != nil {
		r.Reason = err.Error()
		r.Hint = "Ensure the system temporary directory is writable."
		return r
	}
	defer func() {
		if err := os.RemoveAll(profile); err != nil {
			r.Warnings = append(r.Warnings, "Could not remove release browser profile "+profile+": "+err.Error())
		}
	}()
	_, closeBrowser, err := startAuditBrowser(ctx, browser, server.URL, profile)
	if err != nil {
		r.Reason = err.Error()
		r.Hint = BrowserSetup
		return r
	}
	defer closeBrowser()
	active, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	port, portErr := strconv.Atoi(strings.SplitN(string(active), "\n", 2)[0])
	if err != nil || portErr != nil || port < 1 || port > 65535 {
		r.Reason = "Chrome did not expose a valid local DevTools port"
		r.Hint = BrowserSetup
		return r
	}
	// Attach Lighthouse to OUR Chrome. Even a killed/timed-out Node process
	// cannot orphan a Chrome child/profile: Go owns and closes both.
	cmd := exec.CommandContext(ctx, tool, server.URL+"/", "--quiet", "--output=json", "--output-path=stdout", "--only-categories=performance,accessibility,best-practices,seo", "--form-factor=mobile", "--throttling-method=simulate", "--port="+strconv.Itoa(port), "--no-enable-error-reporting")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CHROME_PATH="+browser)
	var stdout cappedBuffer
	stdout.Limit = 32 * 1024 * 1024
	var stderr cappedBuffer
	stderr.Limit = 4096
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		r.Reason = compact(err.Error()+": "+stderr.String(), 600)
		if ctx.Err() != nil {
			r.Reason = "45 s audit deadline exceeded"
		}
		r.Hint = "Check compatible Node/Lighthouse/Chrome versions and OS browser sandbox support. " + LighthouseSetup
		return r
	}
	if stdout.Truncated {
		r.Reason = "Lighthouse JSON exceeded the 32 MiB report limit"
		r.Hint = "Reduce repetitive audit detail or inspect the page's DOM/assets; rerun --release."
		return r
	}
	parsed, parseErr := parseLighthouse(stdout.Bytes(), server.URL)
	// Persist raw detail in the system temp directory, one file per project.
	// Atomic rename replaces an existing symlink rather than following it.
	report := ReportPath(root)
	file, err := os.CreateTemp(filepath.Dir(report), "lfm-lighthouse-*.json")
	if err == nil {
		name := file.Name()
		defer os.Remove(name)
		_, err = file.Write(stdout.Bytes())
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, report)
		}
	}
	if parsed != nil {
		*r = *parsed
	}
	if err != nil {
		r.Warnings = append(r.Warnings, "Full report could not be saved: "+err.Error())
	} else {
		r.Report = report
	}
	if parseErr != nil {
		r.Reason = parseErr.Error()
		r.Hint = "Inspect the saved audit's runtimeError/details and compatible tool versions. " + LighthouseSetup
	}
	return r
}

// Only immutable in-memory artifact bytes are served, without invented delivery
// compression/cache/TLS advantages. Also suitable as Chrome's non-forwarding proxy.
func AuditHandler(page []byte, assets map[string][]byte, origin string) http.Handler {
	want, _ := url.Parse(origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Host != want.Host || r.URL.IsAbs() && (r.URL.Scheme != "http" || r.URL.Host != want.Host) || r.Header.Get("Upgrade") != "" {
			http.Error(w, "audit permits only GETs to the generated artifact", http.StatusForbidden)
			return
		}
		// Fresh profiles/Lighthouse reset provide cold runs. A synthetic no-store
		// header here would itself disable bfcache and create misleading findings.
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == filepath.Base(name) && !strings.ContainsAny(name, `/\`) {
			if content, ok := assets[name]; ok {
				http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
				return
			}
		}
		http.NotFound(w, r)
	})
}

type cappedBuffer struct {
	bytes.Buffer
	Limit     int
	Truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	keep := min(n, max(0, b.Limit-b.Len()))
	_, _ = b.Buffer.Write(p[:keep])
	b.Truncated = b.Truncated || keep < n
	return n, nil
}

// compact collapses whitespace and cuts s to limit runes, marking the cut.
func compact(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if runes := []rune(s); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}

type lhAudit struct {
	Title, Description, DisplayValue, ScoreDisplayMode, ErrorMessage string
	Score                                                            *float64
	Details                                                          json.RawMessage
	Warnings                                                         []string
}
type lhReport struct {
	LighthouseVersion string
	UserAgent         string
	RuntimeError      *struct{ Message string }
	RunWarnings       []string
	ConfigSettings    struct{ FormFactor, ThrottlingMethod string }
	Categories        map[string]struct {
		Score     *float64
		AuditRefs []struct {
			ID     string
			Weight float64
		}
	}
	Audits map[string]lhAudit
}

func parseLighthouse(raw []byte, origin string) (*LighthouseResult, error) {
	var data lhReport
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("invalid Lighthouse JSON: %w", err)
	}
	if data.RuntimeError != nil {
		return nil, fmt.Errorf("navigation failed in Lighthouse: %s", data.RuntimeError.Message)
	}
	if data.LighthouseVersion == "" || len(data.Categories) == 0 || len(data.Audits) == 0 {
		return nil, fmt.Errorf("the Lighthouse JSON lacks version, categories or audits")
	}
	r := &LighthouseResult{Status: "ok", Version: data.LighthouseVersion, Chrome: data.UserAgent, Scope: auditScope, Scores: map[string]*int{}, Metrics: map[string]string{}, Warnings: data.RunWarnings}
	owners := map[string][]string{}
	for _, name := range []string{"performance", "accessibility", "best-practices", "seo"} {
		category, ok := data.Categories[name]
		if !ok || category.Score == nil {
			return nil, fmt.Errorf("the Lighthouse category %s has no score; the audit is incomplete", name)
		}
		if math.IsNaN(*category.Score) || *category.Score < 0 || *category.Score > 1 {
			return nil, fmt.Errorf("invalid Lighthouse category score: %s", name)
		}
		score := int(math.Round(*category.Score * 100))
		r.Scores[name] = &score
		for _, ref := range category.AuditRefs {
			if ref.Weight > 0 {
				owners[ref.ID] = append(owners[ref.ID], name)
			}
		}
	}
	for _, id := range []string{"first-contentful-paint", "largest-contentful-paint", "total-blocking-time", "cumulative-layout-shift", "speed-index"} {
		if a, ok := data.Audits[id]; ok {
			r.Metrics[id] = a.DisplayValue
		}
	}
	var ids []string
	for id := range data.Audits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := data.Audits[id]
		manual := a.ScoreDisplayMode == "manual"
		if !manual && a.ErrorMessage == "" && len(a.Warnings) == 0 && (a.Score == nil || *a.Score >= 1) {
			continue
		}
		items := auditItems(a.Details)
		f := AuditFinding{ID: id, Title: a.Title, Hint: a.Description, Value: a.DisplayValue, Categories: owners[id], Score: a.Score, Items: len(items)}
		if a.ErrorMessage != "" {
			f.Value = a.ErrorMessage
		}
		if len(a.Warnings) > 0 {
			f.Hint += " Warnings: " + strings.Join(a.Warnings, " ")
			if a.Score != nil && *a.Score >= 1 {
				f.Categories = nil
			}
		}
		for _, item := range items {
			if sample := detailSample(item, origin); sample != "" && len(f.Samples) < 3 {
				f.Samples = append(f.Samples, sample)
			}
		}
		if manual {
			r.Manual = append(r.Manual, f)
		} else {
			r.Findings = append(r.Findings, f)
		}
	}
	return r, nil
}

func detailSample(raw json.RawMessage, origin string) string {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	var parts []string
	for _, key := range []string{"url", "sourceURL", "node", "source", "location", "reason", "failureType", "protocolReason", "frameUrl", "message", "errorMessage", "description", "label", "wastedBytes", "wastedMs", "totalBytes"} {
		v, ok := value[key]
		if !ok {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			for _, field := range []string{"selector", "snippet", "url", "line", "column", "lineNumber", "columnNumber"} {
				if item, ok := nested[field]; ok {
					parts = append(parts, fmt.Sprint(item))
				}
			}
		} else {
			parts = append(parts, key+"="+fmt.Sprint(v))
		}
	}
	s := strings.ReplaceAll(strings.Join(parts, "; "), origin, "")
	if i := strings.Index(s, "data:"); i >= 0 {
		s = s[:i] + "[embedded resource; full detail in report]"
	}
	return compact(s, 240)
}

// Details are a tagged union: tables contain arrays, request trees use objects.
// Unknown detail kinds remain available in the complete scratch report.
func auditItems(raw json.RawMessage) []json.RawMessage {
	var details struct{ Items json.RawMessage }
	if json.Unmarshal(raw, &details) != nil {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(details.Items, &list) == nil {
		return list
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(details.Items, &object) != nil {
		return nil
	}
	var keys []string
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		list = append(list, object[key])
	}
	return list
}

// ReportPath is where the complete Lighthouse JSON of a project is kept: the
// system temp directory, outside the project and its deployable output.
func ReportPath(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(os.TempDir(), fmt.Sprintf("lfm-lighthouse-%x.json", sum[:6]))
}
