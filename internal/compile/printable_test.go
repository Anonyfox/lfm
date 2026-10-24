package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/testproject"
)

func TestPrintableIsPublishedBehindItsPath(t *testing.T) {
	t.Parallel()
	dir := printableProject(t)
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v\n%+v", err, result.Validation.Diagnostics)
	}
	for _, d := range result.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, "printable.") {
			t.Errorf("%s: %s", d.Rule, d.Message)
		}
	}
	pr := result.Printable
	if pr == nil || pr.Pages != 5 || len(pr.Editions) != 2 || !pr.Pending || len(pr.Channels) == 0 || !strings.HasSuffix(pr.URL, "/"+printablePath+"/") {
		t.Fatalf("printable summary %+v", pr)
	}
	out := filepath.Join(dir, "output", printablePath)
	if page, err := os.ReadFile(filepath.Join(out, "index.html")); err != nil || !bytes.Contains(page, []byte("being prepared")) {
		t.Fatalf("download page: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution", "printable", "stores", "etsy.md")); err != nil {
		t.Fatalf("store guides: %v", err)
	}
	if _, browserErr := release.BrowserPath(); browserErr != nil || testing.Short() {
		return
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || result.Printable.Pending || len(result.Printable.Files) != 2 || result.Printable.Images < 4 {
		t.Fatalf("release: %v, %+v", err, result.Printable)
	}
	for _, d := range result.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, "printable.") {
			t.Errorf("%s: %s", d.Rule, d.Message)
		}
	}
	pdf, err := os.ReadFile(filepath.Join(out, "maze-pack-a4.pdf"))
	if err != nil || pdfx.PageCount(pdf) != 5 || pdfx.StampOf(pdf) == "" {
		t.Fatalf("pdf: %v, %d pages", err, pdfx.PageCount(pdf))
	}
	cover, err := os.ReadFile(filepath.Join(dir, "distribution", "printable", "images", "01-cover.jpg"))
	if err != nil || len(cover) > 1<<20 {
		t.Fatalf("listing image: %v, %d bytes", err, len(cover))
	}
	// The next ordinary compile reuses the prints and images of the unchanged product.
	again, err := Compile(context.Background(), dir, Options{Publish: true})
	reused, _ := os.ReadFile(filepath.Join(out, "maze-pack-a4.pdf"))
	if err != nil || again.Printable.Pending || !bytes.Equal(reused, pdf) || again.Printable.Images != result.Printable.Images {
		t.Fatalf("prints not reused: %v, %+v", err, again.Printable)
	}
	// A page that clips into the margin is reported by the release.
	css := filepath.Join(dir, filepath.FromSlash(printable.Dir+"/printable.css"))
	os.WriteFile(css, []byte(".maze { width: 140mm; display: block; margin: 0 auto; }\n.page[data-role=key] .maze { width: 80mm; }\nh1 { margin-left: -8mm; }\n"), 0o644)
	result, _ = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	var layout []string
	for _, d := range result.Validation.Diagnostics {
		if d.Rule == printable.RuleLayout {
			layout = append(layout, d.Message)
		}
	}
	if len(layout) != 1 || !strings.Contains(layout[0], "Every edition: Page 1") {
		t.Fatalf("layout findings: %q", layout)
	}
}

func TestPageThatLeaksThePrintablePathFails(t *testing.T) {
	t.Parallel()
	dir := printableProject(t)
	tmpl, _ := os.ReadFile(filepath.Join(dir, "index.html.tmpl"))
	os.WriteFile(filepath.Join(dir, "index.html.tmpl"), bytes.Replace(tmpl, []byte("</main>"), []byte(`<p>`+printablePath+`</p></main>`), 1), 0o644)
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err == nil || !strings.Contains(rulesOf(result), printable.RuleDelivery) {
		t.Fatalf("leak not blocked: %v %s", err, rulesOf(result))
	}
}

func rulesOf(r Result) string {
	var out []string
	if r.Validation != nil {
		for _, d := range r.Validation.Diagnostics {
			out = append(out, d.Rule)
		}
	}
	return strings.Join(out, " ")
}

func findings(r Result, prefix string) []string {
	var out []string
	if r.Validation != nil {
		for _, d := range r.Validation.Diagnostics {
			if strings.HasPrefix(d.Rule, prefix) {
				out = append(out, d.Rule+": "+d.Message)
			}
		}
	}
	return out
}

func TestBundleSellsBookAndPrintableTogether(t *testing.T) {
	t.Parallel()
	dir := testproject.BookPrintableBundle(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	// A flat distribution/ from an earlier lfm is cleared away.
	os.MkdirAll(filepath.Join(dir, "distribution", "stores"), 0o755)
	os.WriteFile(filepath.Join(dir, "distribution", "README.md"), []byte("old"), 0o644)

	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v\n%v", err, findings(result, ""))
	}
	if f := append(findings(result, "bundle."), findings(result, "printable.")...); len(f) > 0 {
		t.Fatalf("findings: %v", f)
	}
	b := result.Bundle
	if b == nil || b.Sell != "page" || b.Ready || !strings.HasSuffix(b.URL, "/"+bundlePath+"/") || !result.Book.Complete || result.Printable.Sell != "bundle" {
		t.Fatalf("bundle %+v", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "output", bundlePath, "index.html")); err != nil {
		t.Fatalf("bundle download page: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "output", bookPath, "index.html")); err != nil {
		t.Fatalf("the book's own download page: %v", err)
	}
	for _, gone := range []string{"README.md", "stores", "printable"} {
		if _, err := os.Stat(filepath.Join(dir, "distribution", gone)); err == nil {
			t.Errorf("distribution/%s should be gone", gone)
		}
	}
	if _, browserErr := release.BrowserPath(); browserErr != nil || testing.Short() {
		return
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || !result.Bundle.Ready || len(result.Bundle.Channels) == 0 || result.Bundle.Images < 2 {
		t.Fatalf("release: %v, %+v; findings: %v", err, result.Bundle, findings(result, ""))
	}
	for _, want := range []string{"the-weekend-starter.epub", "maze-pack-a4.pdf", "maze-pack-letter.pdf"} {
		if _, err := os.Stat(filepath.Join(dir, "output", bundlePath, want)); err != nil {
			t.Errorf("bundle download lacks %s", want)
		}
	}
	// The book has no cover yet, so the bundle shows the printable's images.
	guide, err := os.ReadFile(filepath.Join(dir, "distribution", "bundle", "stores", "etsy.md"))
	if err != nil || !bytes.Contains(guide, []byte("maze-pack-a4.pdf")) || !bytes.Contains(guide, []byte("the-weekend-starter.epub")) || !bytes.Contains(guide, []byte("images/01-printable-cover.jpg")) {
		t.Fatalf("bundle etsy guide: %v\n%s", err, guide)
	}
	again, _ := Compile(context.Background(), dir, Options{Publish: true})
	if again.Bundle.Pending || again.Bundle.Images != result.Bundle.Images {
		t.Fatalf("bundle prints not reused: %+v", again.Bundle)
	}
}

func TestShopsModeLinksToTheShopInsteadOfADownloadPage(t *testing.T) {
	t.Parallel()
	dir := printableProject(t)
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"printable": map[string]any{"sell": "shops", "offer": "pack"}}
		c["urls"] = map[string]any{"pack": "https://buy.stripe.com/test_abc123"}
		c["data"] = map[string]any{"commerce": map[string]any{"offers": map[string]any{"pack": map[string]any{"sku": "pack", "name": "Maze Pack", "checkoutUrlKey": "pack"}}}}
	})
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %v", err, findings(result, ""))
	}
	if f := findings(result, "printable.config"); len(f) != 1 || !strings.Contains(f[0], "sells in shops, but its offer") {
		t.Fatalf("a shop sale linked to Stripe: %v", f)
	}
	if result.Printable.URL != "" {
		t.Fatal("a shop sale has a download URL")
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution", "printable", "stores", "etsy.md")); err != nil {
		t.Fatalf("shop packages: %v", err)
	}
	configure(t, dir, func(c map[string]any) {
		c["urls"] = map[string]any{"pack": "https://www.etsy.com/listing/123456/maze-pack"}
	})
	result, _ = Compile(context.Background(), dir, Options{Publish: true})
	if f := findings(result, "printable.config"); len(f) > 0 {
		t.Fatalf("a shop link was reported: %v", f)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "output"))
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("output/%s published for a shop sale", e.Name())
		}
	}
}

func TestBundleConfigErrors(t *testing.T) {
	t.Parallel()
	dir := printableProject(t)
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"printable": map[string]any{"sell": "bundle"}}
	})
	result, _ := Compile(context.Background(), dir, Options{Publish: true})
	if f := findings(result, "bundle.config"); len(f) != 1 || !strings.Contains(f[0], "no deliverables.bundle") {
		t.Fatalf("%v", f)
	}
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"printable": map[string]any{"sell": "bundle"}, "bundle": map[string]any{"name": "Kit", "sell": "shops"}}
	})
	result, _ = Compile(context.Background(), dir, Options{Publish: true})
	if f := findings(result, "bundle.config"); len(f) != 1 || !strings.Contains(f[0], "two or more") {
		t.Fatalf("%v", f)
	}
}
