package compile

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/chromedp"
)

func TestBrowserHTMLAndSVGEquivalence(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := release.BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="60" height="40" viewBox="0 0 60 40"><defs><linearGradient id="paint"><stop stop-color="#fc0"/><stop offset="1" stop-color="#60c"/></linearGradient></defs><view id="crop" viewBox="0 0 60 40"/><path fill="url(#paint)" d="M0,0h60v40H0z"/><text x="2" y="24" xml:space="preserve"> A  B </text></svg>`)
	base := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)
	percent, encoding := inlineData(svg, "image/svg+xml")
	if encoding != "percent" {
		t.Fatal(encoding)
	}
	source := `<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width"><title>  A  B &amp; C </title><style>body{margin:16px;font:16px monospace}.pre{white-space:break-spaces}.tiles{display:inline-block}.background{width:60px;height:40px;background-image:url("SVG_URI#crop")} [class="a  b"]{color:rgb(120,20,20)}</style></head><body>
<span class="tiles">A</span>  <span class="tiles">B</span>
<div class="pre">  preserved  spacing
and line break  </div><pre>

  preformatted  text
</pre><textarea id="field">

  editable  text
</textarea><label for="field" id="label">  Accessible  name  </label>
<div class="a  b" aria-labelledby="label" data-entity="&amp;#34; &quot; ' &lt;">  selector  target </div>
<img id="image" src="SVG_URI#crop" srcset="SVG_URI#crop 1x" width="60" height="40" alt="  Gradient  diagram  "><div class="background"></div>
<svg viewBox="0 0 80 30" width="80" height="30"><text xml:space="preserve" x="0" y="20"> A  B </text></svg>
<script>document.querySelector('[class="a  b"]').setAttribute('data-selected','yes');document.querySelector('.pre').style.whiteSpace='pre-wrap';const icon = new Image(60,40);icon.src="SVG_URI";icon.alt='JS image';document.body.append(icon);</script>
<script type="application/json" id="data"> { "spacing": "  a  b  ", "end": "\u003c/script>" } </script>
<template id="template"><p data-empty="">  inert  text  </p></template></body></html>`
	before := serializedHTML(t, strings.ReplaceAll(source, "SVG_URI", base))
	after, r := compactHTML(serializedHTML(t, strings.ReplaceAll(source, "SVG_URI", percent)))
	if r.Skipped != "" || r.SavedBytes == 0 {
		t.Fatal(r)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:")
		if req.URL.Path == "/after" {
			w.Write(after)
		} else {
			w.Write(before)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	encodedBase, _ := json.Marshal(base)
	encodedPercent, _ := json.Marshal(percent)
	// Normalize only the deliberately changed URI representation. Compare the
	// complete browser DOM (including template content), copied/rendered text,
	// form values, selectors, image decoding and native accessible-name inputs.
	observe := `(()=>{const norm=s=>s.replaceAll(` + string(encodedPercent) + `,` + string(encodedBase) + `);const tree=n=>[n.nodeType,n.namespaceURI,n.nodeName,n.nodeType===3||n.nodeType===8?norm(n.data):null,n.attributes?Array.from(n.attributes,a=>[a.namespaceURI,a.name,norm(a.value)]):null,Array.from(n.childNodes,tree),n instanceof HTMLTemplateElement?tree(n.content):null];const selection=getSelection();selection.selectAllChildren(document.body);const copied=selection.toString();selection.removeAllRanges();return JSON.stringify({tree:tree(document),copied,rendered:document.body.innerText,field:document.querySelector('#field').value,label:document.querySelector('#field').labels[0].textContent,selected:document.querySelector('[class="a  b"]').dataset.selected,whiteSpace:getComputedStyle(document.querySelector('.pre')).whiteSpace,images:Array.from(document.images,i=>[i.complete,i.naturalWidth,i.naturalHeight,i.alt])})})()`
	for _, width := range []int64{390, 1280} {
		if _, err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900)); err != nil {
			t.Fatal(err)
		}
		var dom string
		var screenshot []byte
		for _, route := range []string{"/before", "/after"} {
			if _, err := chromedp.Run(tab, chromedp.Navigate(server.URL+route)); err != nil {
				t.Fatal(err)
			}
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(async()=>{await document.fonts.ready;await Promise.all(Array.from(document.images,i=>i.decode()));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true})()`, chromedp.EvalAwaitPromise)); err != nil {
				t.Fatal(err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](observe))
			if err != nil {
				t.Fatal(err)
			}
			shot, err := chromedp.Run(tab, chromedp.CaptureScreenshot())
			if err != nil {
				t.Fatal(err)
			}
			if route == "/before" {
				dom, screenshot = got, shot
			} else {
				if got != dom {
					t.Fatalf("browser semantics changed at %dpx:\n%s\n%s", width, dom, got)
				}
				if !bytes.Equal(shot, screenshot) {
					t.Fatalf("rendering changed at %dpx", width)
				}
			}
		}
	}
}
