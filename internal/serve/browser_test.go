package serve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/chromedp"
)

func TestBrowserReloadsSourcesAndRecoversFromErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := release.BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	root := testProject(t)
	address, built := start(t, root)
	if first := nextBuild(t, built); first.err != nil {
		t.Fatal(first.err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, stop := chromedp.NewExecAllocator(ctx, opts...)
	defer stop()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	if _, err := chromedp.Run(tab, chromedp.Navigate(address)); err != nil {
		t.Fatal(err)
	}
	wait := func(expression string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			value, err := chromedp.Run(tab, chromedp.Evaluate[bool](expression))
			if err == nil && value {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("browser reload did not produce", expression)
	}
	wait(`document.querySelector('h1')?.textContent==='First' && window.marker==='first'`)
	raw, _ := os.ReadFile(filepath.Join(root, "lfm.json"))
	write(t, root, "lfm.json", strings.Replace(string(raw), "First", "Second", 1))
	nextBuild(t, built)
	wait(`document.querySelector('h1')?.textContent==='Second'`)
	write(t, root, "app.js", `window.marker="new-script"`)
	nextBuild(t, built)
	wait(`window.marker==='new-script'`)
	template, _ := os.ReadFile(filepath.Join(root, "index.html.tmpl"))
	write(t, root, "index.html.tmpl", `{{.missing}}`)
	nextBuild(t, built)
	wait(`document.querySelector('h1')?.textContent==='Build needs a fix'`)
	write(t, root, "index.html.tmpl", string(template))
	nextBuild(t, built)
	wait(`document.querySelector('h1')?.textContent==='Second' && window.marker==='new-script'`)
}
