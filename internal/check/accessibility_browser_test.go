package check

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Browser evidence for the focused naming/disabled/focus subset. Chrome is a
// test dependency only; normal build validation stays pure Go and offline.
func TestBrowserAccessibleNamesAndNativeFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := release.BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	raw := `<!doctype html><html lang=en><head><meta charset=utf-8><title>Accessibility fixture</title></head><body>
<span id=amount-label><img alt=Amount></span><input id=amount aria-labelledby=amount-label>
<span id=currency-label hidden>Currency</span><input id=currency aria-labelledby=currency-label>
<label for=correct>Wrong for nested<input id=unassociated></label><input id=correct aria-label=Actual>
<button id=action>Calculate</button><button id=combo role=combobox aria-expanded=false>Selected value</button>
<input id=blank-submit type=submit value=""><input id=default-submit type=submit>
<span id=empty-label></span><button id=empty-reference aria-labelledby=empty-label aria-label=Fallback>Visible</button>
<input id=reference-value value=Units><input id=embedded-reference aria-labelledby=reference-value>
<label for=author-label aria-label=Amount></label><input id=author-label>
<svg id=diagram role=img><title>Cost breakdown</title></svg>
<fieldset disabled><legend>Group <input id=legend aria-label=Legend></legend><input id=disabled aria-label=Disabled></fieldset>
<div aria-hidden=true><button id=hidden-control>Focusable but excluded</button></div>
<div inert><button id=inert-control>Inactive</button></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(raw))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), browserTestTimeout)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, stop := chromedp.NewExecAllocator(ctx, opts...)
	defer stop()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	if _, err := chromedp.Run(tab, chromedp.Navigate(server.URL)); err != nil {
		t.Fatal(err)
	}
	d, _ := Parse([]byte(raw), Options{})
	f := d.accessFacts()
	for _, id := range []string{"amount", "currency", "unassociated", "correct", "action", "combo", "blank-submit", "default-submit", "diagram", "empty-reference", "embedded-reference", "author-label"} {
		node, err := chromedp.Call(tab, runtime.Evaluate, runtime.EvaluateParams{Expression: "document.getElementById('" + id + "')"})
		if err != nil {
			t.Fatal(err)
		}
		fetch := false
		if node.Result == nil || node.Result.ObjectID == "" {
			t.Fatal("fixture element unavailable", id, node)
		}
		ax, err := chromedp.Call(tab, accessibility.GetPartialAXTree, accessibility.GetPartialAXTreeParams{ObjectID: node.Result.ObjectID, FetchRelatives: &fetch})
		if err != nil {
			t.Fatal(err)
		}
		name := ""
		if len(ax.Nodes) > 0 && ax.Nodes[0].Name != nil {
			if err := json.Unmarshal([]byte(ax.Nodes[0].Name.Value), &name); err != nil {
				t.Fatal(err)
			}
		}
		if id == "default-submit" && name != "" && d.controlName(f.ids[id]) != "" {
			continue
		} // UA default labels are localized.
		if got := d.controlName(f.ids[id]); got != name {
			t.Errorf("#%s: static name %q, Chromium %q", id, got, name)
		}
	}
	states, err := chromedp.Run(tab, chromedp.Evaluate[map[string]bool](`Object.fromEntries(['legend','disabled'].map(id=>[id,document.getElementById(id).matches(':disabled')]))`))
	if err != nil {
		t.Fatal(err)
	}
	for id, disabled := range states {
		if got := disabledHTML(f.ids[id]); got != disabled {
			t.Errorf("#%s disabled: static %v, browser %v", id, got, disabled)
		}
	}
	for _, tc := range []struct {
		id    string
		focus bool
	}{{"hidden-control", true}, {"inert-control", false}} {
		got, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(()=>{const el=document.getElementById('`+tc.id+`');el.focus();return document.activeElement===el})()`))
		if err != nil || got != tc.focus {
			t.Fatal(tc, got, err)
		}
	}
}
