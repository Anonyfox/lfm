package compile

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

const printablePath = "p7m2x9q4w8e3r6t5y1u0"

func printableProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"printable": map[string]any{"path": printablePath, "version": "2027.1"}}
	})
	writePrintable(t, dir)
	return dir
}

// configure edits lfm.json.
func configure(t testing.TB, dir string, edit func(map[string]any)) {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(dir, "lfm.json"))
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	edit(c)
	raw, _ = json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "lfm.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writePrintable adds a small maze pack in deliverables/printable.
func writePrintable(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		printable.ConceptFile: "# Printable: Maze Pack\n\n## Buyer\nParents with a rainy afternoon.\n- Not for: toddlers.\n\n## Promise\nTwo mazes and their solutions, printed in a minute.\n\n" +
			"## Contents\n| Part | Pages | Job |\n| --- | --- | --- |\n| Cover | 1 | shows the pack and what is inside |\n| Guide | 1 | explains printing at actual size |\n| Mazes | 2 | two mazes to solve with a pencil |\n| Key | 1 | both solutions to check |\n\n" +
			"## Editions\n- Sizes: a4, letter\n\n## Look\nBody: system-ui, sans-serif\nHeadings: system-ui, sans-serif\nAccent: #2b6e5a\n",
		printable.TemplateFile: `{{- $a := maze 10 12 1 -}}{{- $b := maze 12 14 2 -}}
<section class="page" data-part="Cover" data-role="cover"><h1>{{.Name}}</h1></section>
<section class="page" data-part="Guide" data-role="guide"><p>Print at 100%. Solutions on page <span class="page-ref" data-ref="key"></span>.</p></section>
<section class="page" data-part="Mazes" data-role="puzzle">{{$a.SVG 10 false}}</section>
<section class="page" data-part="Mazes" data-role="puzzle">{{$b.SVG 10 false}}</section>
<section class="page" data-part="Key" data-role="key" id="key">{{$a.SVG 6 true}}{{$b.SVG 6 true}}</section>
`,
		printable.Dir + "/printable.css": ".maze { width: 140mm; display: block; margin: 0 auto; }\n.page[data-role=key] .maze { width: 80mm; }\n",
		printable.ListingFile:            "# Listing: Maze Pack\n\n## Title\nPrintable Maze Pack for Kids, A4 and Letter\n\n## Description\n" + strings.Repeat("Two calm mazes for a rainy afternoon, with solutions. ", 5) + "\n\n## Tags\n- maze printable\n\n## Price\n3.00 EUR\n\n## AI\nText: none\nImages: none\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

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

const bundlePath = "b7m2x9q4w8e3r6t5y1u0"

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
	dir := bookProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	writePrintable(t, dir)
	configure(t, dir, func(c map[string]any) {
		d := c["deliverables"].(map[string]any)
		d["printable"] = map[string]any{"sell": "bundle"}
		d["bundle"] = map[string]any{"name": "Starter Kit", "path": bundlePath}
	})
	os.MkdirAll(filepath.Join(dir, "deliverables", "bundle"), 0o755)
	os.WriteFile(filepath.Join(dir, "deliverables", "bundle", "listing.md"), []byte("# Listing: Starter Kit\n\n## Title\nSourdough Starter Kit: ebook and maze pack\n\n## Description\n"+strings.Repeat("The book and the pack for a rainy weekend with a starter. ", 5)+"\n\n## Tags\n- sourdough kit\n\n## Price\n9.00 EUR\n\n## AI\nText: none\nImages: none\n"), 0o644)
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
		t.Fatalf("release: %v, %+v", err, result.Bundle)
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
