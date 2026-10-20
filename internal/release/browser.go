// Package release performs optional, isolated browser work outside the fast build.
package release

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"image/png"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/evanw/esbuild/pkg/api"
)

//go:embed critical.js
var criticalJS string

type Viewport struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

var viewports = [...]Viewport{{390, 844}, {768, 1024}, {1440, 900}}

type Result struct {
	Applied       bool         `json:"applied"`
	Reason        string       `json:"reason,omitempty"`
	OriginalBytes int          `json:"originalCSSBytes"`
	CriticalBytes int          `json:"criticalCSSBytes"`
	Stylesheet    string       `json:"stylesheet,omitempty"`
	Selection     string       `json:"selection,omitempty"`
	Viewports     []Viewport   `json:"viewports,omitempty"`
	CSS           string       `json:"-"`
	Audit         *AuditResult `json:"audit,omitempty"`
	LazyImages    int          `json:"lazyImages,omitempty"`    // images the compiler set to load lazily
	PriorityImage string       `json:"priorityImage,omitempty"` // image the compiler set to fetch first
}

type rule struct {
	Key, Text, Prefix  string
	Children           []rule
	Critical, Document bool
}
type observation struct {
	Rules       []rule
	Fingerprint string
	Reason      string
}

func BrowserPath() (string, error) {
	if path := os.Getenv("LFM_CHROME"); path != "" {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", fmt.Errorf("LFM_CHROME: %w", err)
		}
		return found, nil
	}
	for _, path := range browserCandidates() {
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("--release needs installed Chrome/Chromium; install it or set LFM_CHROME to its executable (ordinary builds need neither browser nor Node)")
}

// browserCandidates lists Chromium-based browsers in order of preference:
// headless builds first, then regular installs on Linux and macOS.
func browserCandidates() []string {
	candidates := []string{
		"chromium-headless-shell", "chrome-headless-shell", "headless_shell",
		"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome",
		"microsoft-edge", "brave-browser", "/snap/bin/chromium",
	}
	for _, app := range []string{"Google Chrome", "Chromium", "Microsoft Edge", "Brave Browser"} {
		inApplications := filepath.Join("/Applications", app+".app", "Contents", "MacOS", app)
		candidates = append(candidates, inApplications)
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, inApplications))
		}
	}
	return candidates
}

// Analyze observes static first paint at several widths, unions visible rules,
// then verifies both critical-only and critical+full rendering at every width.
// The original full CSS is never purged. Ambiguous/unstable pages retain it inline.
func Analyze(ctx context.Context, page []byte, assets map[string][]byte) (Result, error) {
	var result Result
	path, err := BrowserPath()
	if err != nil {
		return result, err
	}
	server := httptest.NewServer(Handler(page, assets))
	defer server.Close()
	ctx, timeout := context.WithTimeout(ctx, 45*time.Second)
	defer timeout()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path), chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-component-update", true), chromedp.Flag("no-first-run", true))
	alloc, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browser, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	// The preview CSP blocks application scripts. Keep the engine enabled for
	// DevTools promises, loaded-font checks and rendering-frame synchronization.
	if _, err = chromedp.Run(browser, chromedp.Evaluate[bool]("true")); err != nil {
		return result, fmt.Errorf("start release browser: %w", err)
	}
	events := chromedp.Events(browser, fetch.RequestPaused)
	go func() {
		for ev, err := range events {
			if err != nil {
				return
			}
			if strings.HasPrefix(ev.Request.URL, server.URL+"/") && ev.Request.Method == "GET" {
				_, _ = chromedp.Call(browser, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
			} else {
				_, _ = chromedp.Call(browser, fetch.FailRequest, fetch.FailRequestParams{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
			}
		}
	}()
	if _, err = chromedp.Call(browser, fetch.Enable, fetch.EnableParams{}); err != nil {
		return result, err
	}
	selected := map[string]bool{}
	static := map[string]bool{}
	var reference observation
	var fingerprints []string
	var screenshots [][]byte
	for _, viewport := range viewports {
		if err := load(browser, server.URL, viewport); err != nil {
			return result, err
		}
		observed, err := chromedp.Run(browser, chromedp.Evaluate[observation](criticalJS))
		if err != nil {
			return result, fmt.Errorf("observe critical CSS: %w", err)
		}
		if observed.Reason != "" {
			result.Reason = observed.Reason
			return result, nil
		}
		collect(observed.Rules, selected, false)
		collect(observed.Rules, static, true)
		reference = observed
		fingerprints = append(fingerprints, observed.Fingerprint)
		shot, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
		if err != nil {
			return result, err
		}
		screenshots = append(screenshots, shot)
	}
	// Off-screen layout rules can change ancestor geometry. If the minimal
	// viewport set fails, promote every statically matched rule and verify again.
	for attempt, selection := range []map[string]bool{selected, static} {
		critical := render(reference.Rules, selection)
		// The rules come from the browser's own serialization. Syntax
		// minification would rewrite equivalent values (a gradient's 0px first
		// stop) and the exact style comparison below would reject the result.
		minified := api.Transform(critical, api.TransformOptions{Loader: api.LoaderCSS, MinifyWhitespace: true, Charset: api.CharsetUTF8, Target: api.ES2020, LogLevel: api.LogLevelSilent})
		if len(minified.Errors) > 0 {
			return result, fmt.Errorf("critical CSS minification: %s", api.FormatMessages(minified.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage}))
		}
		critical = string(minified.Code)
		matched, err := verify(browser, server.URL, critical, fingerprints, screenshots)
		if err != nil {
			return result, err
		}
		if matched {
			result.CSS, result.Viewports = critical, append([]Viewport(nil), viewports[:]...)
			result.CriticalBytes = len(critical)
			result.Selection = "viewport"
			if attempt > 0 {
				result.Selection = "document"
			}
			return result, nil
		}
	}
	result.Reason = "Critical CSS changed first-screen styles/layout or pixels; retained the complete blocking stylesheet."
	return result, nil
}

func verify(browser context.Context, url, critical string, fingerprints []string, screenshots [][]byte) (bool, error) {
	encoded, _ := json.Marshal(critical)
	for i, viewport := range viewports {
		if err := load(browser, url, viewport); err != nil {
			return false, err
		}
		// Insert critical rules immediately before their complete original sheet.
		if _, err := chromedp.Run(browser, chromedp.Evaluate[bool](`(()=>{const original=document.querySelector('[data-lfm-critical-source]');const style=document.createElement('style');style.textContent=`+string(encoded)+`;original.before(style);return true})()`)); err != nil {
			return false, err
		}
		for _, criticalOnly := range []bool{false, true} {
			if criticalOnly {
				if _, err := chromedp.Run(browser, chromedp.Evaluate[bool](`(()=>{document.querySelector('[data-lfm-critical-source]').sheet.disabled=true;return true})()`)); err != nil {
					return false, err
				}
			}
			if err := settle(browser); err != nil {
				return false, err
			}
			observed, err := chromedp.Run(browser, chromedp.Evaluate[observation](criticalJS))
			if err != nil {
				return false, err
			}
			shot, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
			if err != nil {
				return false, err
			}
			if observed.Fingerprint != fingerprints[i] || !samePixels(shot, screenshots[i]) {
				return false, nil
			}
		}
	}
	return true, nil
}

func collect(rules []rule, selected map[string]bool, document bool) {
	for _, rule := range rules {
		if rule.Critical || document && rule.Document {
			selected[rule.Key] = true
		}
		collect(rule.Children, selected, document)
	}
}
func render(rules []rule, selected map[string]bool) string {
	var out strings.Builder
	for _, rule := range rules {
		if rule.Prefix != "" {
			if child := render(rule.Children, selected); child != "" {
				out.WriteString(rule.Prefix)
				out.WriteString(child)
				out.WriteByte('}')
			}
		} else if selected[rule.Key] {
			out.WriteString(rule.Text)
		}
	}
	return out.String()
}

func load(ctx context.Context, url string, viewport Viewport) error {
	if _, err := chromedp.Run(ctx, chromedp.EmulateViewport(viewport.Width, viewport.Height)); err != nil {
		return err
	}
	if _, err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		return fmt.Errorf("release preview: %w", err)
	}
	if err := settle(ctx); err != nil {
		return fmt.Errorf("settle release preview: %w", err)
	}
	return nil
}
func settle(ctx context.Context) error {
	_, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(async()=>{await document.fonts.ready;let freeze=document.querySelector('[data-lfm-freeze]');if(!freeze){freeze=document.createElement('style');freeze.setAttribute('data-lfm-freeze','');freeze.textContent='*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}';document.head.append(freeze)}await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true})()`, chromedp.EvalAwaitPromise))
	return err
}

// Handler exposes only the in-memory artifact, never project sources.
func Handler(page []byte, assets map[string][]byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'unsafe-inline' data:; img-src 'self' data:; font-src 'self' data:; script-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == filepath.Base(name) && !strings.ContainsAny(name, `/\`) {
			if body, ok := assets[name]; ok {
				w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(name)))
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, r)
	})
}

// samePixels reports whether two screenshots show the same image. The style
// fingerprint compared beside it is exact, so this guards what styles cannot
// show; two loads of one page still differ by rasterization noise of a few
// levels (gradients, image scaling, text antialiasing). A screenshot passes
// when no channel differs by more than 24 levels and fewer than 1% of pixels
// differ by more than 3; a missing rule changes far more than that.
func samePixels(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	x, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return false
	}
	y, err := png.Decode(bytes.NewReader(b))
	if err != nil || x.Bounds() != y.Bounds() {
		return false
	}
	diff := func(p, q uint32) uint32 {
		p, q = p>>8, q>>8
		if p > q {
			return p - q
		}
		return q - p
	}
	r := x.Bounds()
	noisy, limit := 0, r.Dx()*r.Dy()/100
	for py := r.Min.Y; py < r.Max.Y; py++ {
		for px := r.Min.X; px < r.Max.X; px++ {
			r1, g1, b1, a1 := x.At(px, py).RGBA()
			r2, g2, b2, a2 := y.At(px, py).RGBA()
			d := max(diff(r1, r2), diff(g1, g2), diff(b1, b2), diff(a1, a2))
			if d > 24 {
				return false
			}
			if d > 3 {
				if noisy++; noisy > limit {
					return false
				}
			}
		}
	}
	return true
}
