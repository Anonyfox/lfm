package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestRuleUnionPreservesOrderGroupsAndGlobals(t *testing.T) {
	t.Parallel()
	rules := []rule{{Key: "a", Text: "@layer base;"}, {Prefix: "@media (width>0){", Children: []rule{{Key: "b", Text: "p{color:red}"}, {Key: "c", Text: "aside{color:blue}"}}}, {Key: "d", Text: "@font-face{font-family:Demo}"}}
	got := render(rules, map[string]bool{"a": true, "b": true, "d": true})
	if got != "@layer base;@media (width>0){p{color:red}}@font-face{font-family:Demo}" {
		t.Fatal(got)
	}
}

func TestPreviewOnlyServesArtifactAndGET(t *testing.T) {
	t.Parallel()
	h := Handler([]byte("page"), map[string][]byte{"style.css": []byte("css")})
	for _, test := range []struct {
		method, path string
		status       int
	}{{"GET", "/", 200}, {"GET", "/style.css", 200}, {"GET", "/lfm.json", 404}, {"GET", "/../style.css", 404}, {"GET", "/node_modules/x", 404}, {"POST", "/", 405}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(test.method, test.path, nil))
		if r.Code != test.status {
			t.Fatal(test, r.Code)
		}
		if !strings.Contains(r.Header().Get("Content-Security-Policy"), "script-src 'none'") {
			t.Fatal("analysis can run page scripts")
		}
	}
}

func TestInstalledBrowserCriticalExtraction(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	page := []byte(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><style data-lfm-critical-source>body{margin:0}h1{color:rgb(10,20,30)}.below{color:red}@media(min-width:1000px){h1{font-size:45px}.below{color:blue}}.dynamic:hover{color:green}</style></head><body><h1>Visible</h1><div style="height:2400px"></div><p class="below">Below</p><script>document.body.textContent='MUST NOT EXECUTE'</script></body></html>`)
	result, err := Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing is hovered on a first visit, so a hover rule waits for the full sheet.
	if result.Reason != "" || !strings.Contains(result.CSS, "h1") || strings.Contains(result.CSS, ".below") || strings.Contains(result.CSS, ".dynamic:hover") {
		t.Fatalf("%+v", result)
	}
}

func TestInstalledBrowserPromotesOffscreenLayoutRules(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	// Below the first screen, a rule changes only what no visitor sees yet.
	page := []byte(`<!doctype html><html><head><style data-lfm-critical-source>body{margin:0}.below{padding:50px}</style></head><body><h1>Visible</h1><div style="height:2400px"></div><p class="below">Below</p></body></html>`)
	result, err := Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "" || result.Selection != "viewport" || strings.Contains(result.CSS, ".below") {
		t.Fatalf("below the fold: %+v", result)
	}
	// Beside it, past the right edge, a rule sets the height of the row the
	// visible cell is in: dropping it would change the first screen, so it is
	// critical though its element is off the screen.
	page = []byte(`<!doctype html><html><head><style data-lfm-critical-source>body{margin:0;overflow-x:hidden}.row{display:grid;grid-template-columns:calc(100vw + 10px) 100vw}.tall{height:300px}</style></head><body><div class="row"><p style="background:#c00">Visible</p><p class="tall">Beside</p></div></body></html>`)
	result, err = Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "" || !strings.Contains(result.CSS, ".tall") {
		t.Fatalf("beside the screen: %+v", result)
	}
}

// TestInstalledBrowserSelectsBySubjectAndKeepsHiders holds the rules with
// pseudo-classes and pseudo-elements to the elements they style.
func TestInstalledBrowserSelectsBySubjectAndKeepsHiders(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	page := []byte(`<!doctype html><html><head><style data-lfm-critical-source>body{margin:0}:root{--ink:rgb(1,2,3)}` +
		`.led::after{content:"";display:inline-block;width:7px;height:7px;background:var(--ink)}.far::before{content:"far"}` +
		`a:hover{color:red}a:focus-visible{outline:2px solid}:target{color:blue}nav :is(.on,[aria-current])>.led::after{background:orange}` +
		`.menu{display:none}.gone{display:none}.skip{position:absolute;transform:translateY(-200%)}</style></head><body><a class="skip" href="#main">Skip to content</a><nav><a class="on" aria-current="page" href="#"><i class="led"></i>Home</a><ul class="menu"><li>Hidden menu</li></ul></nav>` +
		`<div style="height:2400px"></div><p class="far">Far</p><p class="gone">Gone below</p></body></html>`)
	result, err := Analyze(context.Background(), page, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{":root", ".led::after", "aria-current", ".menu", ".skip", ":target"} {
		if !strings.Contains(result.CSS, want) {
			t.Errorf("critical CSS lacks %s: %s", want, result.CSS)
		}
	}
	for _, unwanted := range []string{".far::before", "a:hover", ":focus-visible", ".gone"} {
		if strings.Contains(result.CSS, unwanted) {
			t.Errorf("critical CSS holds %s: %s", unwanted, result.CSS)
		}
	}
	if result.Reason != "" || result.Selection != "viewport" {
		t.Fatalf("%+v", result)
	}
}

// TestInstalledBrowserVerdictIsTheSameEveryTime analyzes a page whose first
// screen has auto margins and images still loading, which Chrome measured
// from an earlier layout now and then: every analysis must agree.
func TestInstalledBrowserVerdictIsTheSameEveryTime(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><style data-lfm-critical-source>body{margin:0}.wrap{width:min(100% - 2rem,60rem);margin-inline:auto;display:grid;gap:1rem;grid-template-columns:repeat(auto-fill,minmax(8rem,1fr))}img{width:100%;height:auto;display:block}.below{padding:3rem;border:1px solid}</style></head><body><div class="wrap">`)
	assets := map[string][]byte{}
	for i := range 24 {
		name := fmt.Sprintf("p%d.png", i)
		assets[name] = tile(i)
		fmt.Fprintf(&b, `<img src="%s" alt="" width="64" height="48">`, name)
	}
	b.WriteString(`</div><div style="height:3000px"></div><p class="below">Below</p></body></html>`)
	var first Result
	for run := range 4 {
		started := time.Now()
		result, err := Analyze(context.Background(), []byte(b.String()), assets)
		if err != nil {
			t.Fatalf("run %d after %s: %v", run, time.Since(started), err)
		}
		t.Logf("run %d completed in %s", run, time.Since(started))
		if result.Reason != "" || result.Selection != "viewport" {
			t.Fatalf("run %d: %+v", run, result)
		}
		if run == 0 {
			first = result
		} else if result.CSS != first.CSS {
			t.Fatalf("run %d chose other rules:\n%s\n%s", run, first.CSS, result.CSS)
		}
	}
}

func TestBrowserStepReportsStallsAndCancellationCause(t *testing.T) {
	t.Parallel()
	_, err := browserStep(context.Background(), "stalled command", time.Millisecond, func(ctx context.Context) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "stalled command: no completion within 1ms") {
		t.Fatal(err)
	}
	// A failure in the request handler must reach the foreground operation,
	// rather than becoming an unexplained cancellation or navigation timeout.
	cause := errors.New("handle release preview request: protocol failure")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	_, err = browserStep(ctx, "load release preview", time.Second, func(ctx context.Context) (bool, error) {
		return false, ctx.Err()
	})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "load release preview: handle release preview request") {
		t.Fatal(err)
	}
}

// Exercise actual navigation and DevTools promises, including image decode,
// which used to silently time out and allow measuring an unfinished screen.
// A stuck operation must fail by name without consuming the next operation's
// budget or killing Chrome when its step context ends.
func TestInstalledBrowserBoundsStalledOperations(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	browser, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	// Initialize with a lifetime context before using individual step contexts.
	if _, err := chromedp.Run(browser, chromedp.Evaluate[bool]("true")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, setup, script string }{
		{"fonts", `Object.defineProperty(document.fonts,'ready',{value:new Promise(()=>{})});`, settleFontsScript},
		{"images", `const i=document.createElement('img');Object.defineProperty(i,'complete',{value:false});i.decode=()=>new Promise(()=>{});document.body.append(i);`, settleImagesScript},
		{"animation frames", `window.requestAnimationFrame=()=>{};`, settleFramesScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := browserAction(browser, "reset test page", chromedp.Navigate("about:blank")); err != nil {
				t.Fatal(err)
			}
			if _, err := browserAction(browser, "install stalled promise", chromedp.Evaluate[bool](`(()=>{`+tc.setup+`return true})()`)); err != nil {
				t.Fatal(err)
			}
			_, err := browserStep(browser, tc.name, time.Second, func(ctx context.Context) (bool, error) {
				return chromedp.Run(ctx, chromedp.Evaluate[bool](tc.script, chromedp.EvalAwaitPromise))
			})
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), tc.name+": no completion within 1s") {
				t.Fatal(err)
			}
			if _, err := browserAction(browser, "check browser still responds", chromedp.Evaluate[bool]("true")); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("navigation", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/pending.png" {
				<-release
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><img src="/pending.png">`)
		}))
		defer server.Close()
		defer close(release)
		_, err := browserStep(browser, "load release preview", time.Second, func(ctx context.Context) (chromedp.Void, error) {
			return chromedp.Run(ctx, chromedp.Navigate(server.URL))
		})
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "load release preview: no completion within 1s") {
			t.Fatal(err)
		}
		if _, err := browserAction(browser, "check browser still responds", chromedp.Evaluate[bool]("true")); err != nil {
			t.Fatal(err)
		}
	})
}

func tile(seed int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = byte(seed*37 + i)
	}
	var out bytes.Buffer
	png.Encode(&out, img)
	return out.Bytes()
}
