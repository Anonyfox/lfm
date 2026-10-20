package release

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

type RuntimeResult struct {
	Status     string        `json:"status"`
	Reason     string        `json:"reason,omitempty"`
	Hint       string        `json:"hint,omitempty"`
	Chrome     string        `json:"chrome,omitempty"`
	Scope      string        `json:"scope,omitempty"`
	Problems   []string      `json:"problems,omitempty"`
	Layout     *LayoutResult `json:"layout,omitempty"`
	DurationMS int64         `json:"durationMs"`
}

// A cheap real-JS navigation probe also runs when Lighthouse isn't installed.
// It never guesses which business controls to click or submits forms/orders.
// The same tab then walks the page frame by frame at every review screen
// (auditLayout); frames is the screenshot folder, "" for none.
func AuditRuntime(ctx context.Context, page []byte, assets map[string][]byte, path, frames string) *RuntimeResult {
	start := time.Now()
	r := &RuntimeResult{Status: "skipped", Scope: "390x844 initial navigation with application JS; 250 ms settle; no user journeys or simulated mobile performance scores; external traffic/mutations blocked."}
	defer func() { r.DurationMS = time.Since(start).Milliseconds() }()
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	server.Config.Handler = AuditHandler(page, assets, origin)
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	fail := func(err error) *RuntimeResult {
		r.Reason = compact(err.Error(), 500)
		r.Hint = BrowserSetup + " Check OS sandbox support and page startup time."
		return r
	}
	tab, cleanup, err := startAuditBrowser(ctx, path, server.URL, "")
	if err != nil {
		return fail(err)
	}
	defer cleanup()
	version, err := chromedp.CallBrowser(tab, browser.GetVersion, cdp.Empty{})
	if err != nil {
		return fail(err)
	}
	r.Chrome = version.Product
	var mu sync.Mutex
	problems := map[string]int{}
	add := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		problems[compact(strings.ReplaceAll(s, server.URL, ""), 500)]++
	}
	exceptions := chromedp.Events(tab, runtime.ExceptionThrown)
	responses := chromedp.Events(tab, network.ResponseReceived)
	console := chromedp.Events(tab, runtime.ConsoleAPICalled)
	go func() {
		for e, err := range exceptions {
			if err != nil {
				return
			}
			d := e.ExceptionDetails
			message := d.Text
			if d.Exception != nil && d.Exception.Description != "" {
				message = d.Exception.Description
			}
			add(fmt.Sprintf("%s:%d:%d: %s", d.URL, d.LineNumber+1, d.ColumnNumber+1, message))
		}
	}()
	go func() {
		for e, err := range responses {
			if err != nil {
				return
			}
			if e.Response.Status >= 400 {
				add(fmt.Sprintf("HTTP %d %s", e.Response.Status, e.Response.URL))
			}
		}
	}()
	go func() {
		for e, err := range console {
			if err != nil {
				return
			}
			if string(e.Type) == "error" {
				var parts []string
				for _, arg := range e.Args {
					if arg.Description != "" {
						parts = append(parts, arg.Description)
					} else {
						parts = append(parts, string(arg.Value))
					}
				}
				add("console.error: " + strings.Join(parts, " "))
			}
		}
	}()
	if _, err := chromedp.Call(tab, network.Enable, network.EnableParams{}); err != nil {
		return fail(err)
	}
	if _, err := chromedp.Call(tab, runtime.Enable, cdp.Empty{}); err != nil {
		return fail(err)
	}
	if _, err := chromedp.Run(tab, chromedp.EmulateViewport(390, 844)); err != nil {
		return fail(err)
	}
	if _, err := chromedp.Run(tab, chromedp.Navigate(server.URL+"/")); err != nil {
		return fail(err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(async()=>{await document.fonts.ready;await new Promise(r=>setTimeout(r,250));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true})()`, chromedp.EvalAwaitPromise)); err != nil {
		return fail(err)
	}
	mu.Lock()
	for problem, count := range problems {
		if count > 1 {
			problem += fmt.Sprintf(" (×%d)", count)
		}
		r.Problems = append(r.Problems, problem)
	}
	mu.Unlock()
	sort.Strings(r.Problems)
	r.Status = "ok"
	// Problems are taken before the frame walk reloads the page at other sizes.
	layout, err := auditLayout(tab, server.URL+"/", frames)
	r.Layout = layout
	if err != nil {
		layout.Findings = append(layout.Findings, LayoutFinding{Kind: "incomplete", Message: "The frame walk stopped early: " + compact(err.Error(), 300), Hint: "Check that the page loads and scrolls in Chrome; rerun --release."})
	}

	return r
}

func startAuditBrowser(ctx context.Context, path, proxy, profile string) (context.Context, func(), error) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path), chromedp.ProxyServer(proxy), chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("disable-quic", true), chromedp.Flag("disable-component-update", true))
	if profile != "" {
		opts = append(opts, chromedp.UserDataDir(profile), chromedp.Flag("remote-debugging-port", "0"), remote.WebSocket)
	}
	alloc, closeAlloc := chromedp.NewExecAllocator(ctx, opts...)
	tab, closeTab := chromedp.NewContext(alloc)
	cleanup := func() { closeTab(); closeAlloc() }
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool]("true")); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return tab, cleanup, nil
}
