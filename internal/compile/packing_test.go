package compile

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func autoFixture(t *testing.T) string {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { delete(c["build"].(map[string]any), "assets") })
	return root
}

func TestAutomaticPackingUsesRolesAndWholeDocument(t *testing.T) {
	root := autoFixture(t)
	for name, n := range map[string]int{"tiny.svg": 100, "large.png": 30000, "lazy.webp": 6000, "small.webp": 200, "font.woff2": 6000} {
		write(t, root, name, strings.Repeat("A", n))
	}
	write(t, root, "index.html.tmpl", helperHTML("", `<style>@font-face{font-family:Example;src:url('./font.woff2')}</style>`)+"")
	// Keep markup valid and put native image intent in the body.
	page := read(t, root, "index.html.tmpl")
	page = strings.Replace(page, "</body>", `<img src="tiny.svg" alt="tiny" width="10" height="10"><img src="large.png" alt="large" width="10" height="10"><img src="lazy.webp" loading="lazy" alt="lazy" width="10" height="10"><img src="small.webp" srcset="small.webp 1x, tiny.svg 2x" alt="responsive" width="10" height="10"></body>`, 1)
	write(t, root, "index.html.tmpl", page)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	got := map[string]AssetDecision{}
	for _, d := range report.Packing {
		got[d.Source] = d
	}
	for _, name := range []string{"tiny.svg", "large.png", "lazy.webp", "small.webp", "font.woff2"} {
		if got[name].Mode != "file" {
			t.Fatalf("%s: %+v", name, got[name])
		}
	}
	if got["tiny.svg"].Reason != "native responsive selection" {
		t.Fatal(got["tiny.svg"])
	}
	output := read(t, root, "output/index.html")
	if strings.Contains(output, "data:font/") || !strings.Contains(output, fileURL(got["font.woff2"].Output)) || !strings.Contains(output, `srcset="`+fileURL(got["small.webp"].Output)+` 1x, `+fileURL(got["tiny.svg"].Output)+` 2x"`) {
		t.Fatal(output)
	}
	if report.Cost.HTMLBytes != len(output) || report.Cost.SidecarBytes < 42000 || report.Cost.GzipBytes >= report.Cost.HTMLBytes {
		t.Fatal(report.Cost)
	}
	before := output
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("packing is nondeterministic")
	}
}

func TestPackingTargetAndPins(t *testing.T) {
	root := autoFixture(t)
	var imgs strings.Builder
	for i := 0; i < 35; i++ {
		name := string(rune('A'+i)) + ".png"
		write(t, root, name, strings.Repeat(string(rune('a'+i)), 7000))
		imgs.WriteString(`<img width="10" height="10" alt="image" src="` + name + `">`)
	}
	write(t, root, "index.html.tmpl", strings.Replace(helperHTML("", ""), "</body>", imgs.String()+"</body>", 1))
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	selected := 0
	for _, d := range report.Packing {
		if d.Reason == "whole HTML exceeds 256 KiB packing target" {
			selected++
		}
	}
	if selected == 0 || report.Cost.HTMLBytes > embeddedDocumentTarget+2048 {
		t.Fatal(selected, report.Cost)
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"] = map[string]any{"overrides": map[string]any{"A.png": "inline"}}
	})
	report, err = Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range report.Packing {
		if d.Source == "A.png" && (d.Mode != "inline" || !d.Pinned) {
			t.Fatal(d)
		}
	}
}

func TestPackingRewritesGeneratedCodeButPreservesInertAndVisibleData(t *testing.T) {
	root := autoFixture(t)
	raw := strings.Repeat("z", 16000)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(raw))
	changeConfig(t, root, func(c map[string]any) { c["urls"] = map[string]any{"original": uri} })
	write(t, root, "big.png", raw)
	write(t, root, "app.js", `import picture from './big.png';window.picture=picture`)
	write(t, root, "theme.css", `.hero{background:url('./big.png')}`)
	page := helperHTML(`{{script "app.js"}}`, `{{style "theme.css"}}`)
	page = strings.Replace(page, "</body>", `<p title="`+uri+`">`+uri+`</p><script type="application/json">{"original":"{{.urls.original}}"}</script></body>`, 1)
	write(t, root, "index.html.tmpl", page)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	output := read(t, root, "output/index.html")
	visible, inert := false, false
	for _, n := range parsePage(t, output) {
		if n.Data == "p" && attr(n, "title") == uri && textContent(n) == uri {
			visible = true
		}
		if n.Data == "script" && attr(n, "type") == "application/json" && attr(n, "id") == "" {
			var value map[string]string
			if err := json.Unmarshal([]byte(textContent(n)), &value); err != nil {
				t.Fatal(err)
			}
			inert = value["original"] == uri
		}
	}
	if !visible || !inert {
		t.Fatal("visible text, title or inert JSON was altered", visible, inert)
	}
	var code, css string
	for _, n := range parsePage(t, output) {
		if n.Data == "script" && executableScript(n) {
			code += textContent(n)
		}
		if n.Data == "style" {
			css += textContent(n)
		}
	}
	packedURL := ""
	for _, d := range report.Packing {
		if d.Source == "big.png" {
			packedURL = fileURL(d.Output)
		}
	}
	if !strings.Contains(code, `"`+packedURL+`"`) || !strings.Contains(code, "window.picture=") || strings.Contains(code, uri) || !strings.Contains(css, `url(`+packedURL+`)`) {
		t.Fatal("compiled resource URL not rewritten", code[:min(len(code), 300)], css[:min(len(css), 300)])
	}
	for _, d := range report.Packing {
		if d.Source == "big.png" && d.Occurrences != 2 {
			t.Fatal(report.Packing)
		}
	}
}

func TestIdenticalAssetAliasesHonorInlinePin(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "a.png", strings.Repeat("x", 16000))
	write(t, root, "b.png", read(t, root, "a.png"))
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"] = map[string]any{"overrides": map[string]any{"b.png": "inline"}}
	})
	write(t, root, "index.html.tmpl", strings.Replace(helperHTML("", ""), "</body>", `<img src="a.png" alt="a"><img src="b.png" alt="b"></body>`, 1))
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Packing[0].Pinned || report.Packing[0].Mode != "inline" || report.Packing[0].Occurrences != 2 {
		t.Fatal(report.Packing)
	}
}

func TestFaviconFallbackAndAuthoredIdentity(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "index.html.tmpl", helperHTML("", ""))
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	var icon *html.Node
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "link" && relToken(n, "icon") {
			icon = n
		}
	}
	if icon == nil {
		t.Fatal("no favicon")
	}
	if attr(icon, "href") != "./favicon.png" {
		t.Fatal(attr(icon, "href"))
	}
	raw := []byte(read(t, root, "output/favicon.png"))
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds().Dx() != 96 {
		t.Fatal(err)
	}
	write(t, root, "custom.png", string(raw))
	write(t, root, "index.html.tmpl", helperHTML("", `<link rel="shortcut icon" id="live-icon" href="custom.png">`))
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "link" && relToken(n, "icon") {
			count++
			if attr(n, "id") != "live-icon" {
				t.Fatal("authored icon replaced")
			}
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
}

func TestLocalHintsFollowCompiledFilesAndDropInlineResources(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "one.js", `window.one=1;export{}`)
	write(t, root, "two.js", `window.two=2;export{}`)
	write(t, root, "font.woff2", strings.Repeat("font", 2000))
	write(t, root, "tiny.png", "tiny")
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"] = map[string]any{"overrides": map[string]any{"one.js": "file", "two.js": "file"}}
	})
	head := `<link rel="preload" as="font" href="font.woff2"><link rel="preload" as="image" href="tiny.png"><link rel="modulepreload" href="one.js"><style>@font-face{font-family:Example;src:url('./font.woff2')}</style>`
	write(t, root, "index.html.tmpl", helperHTML(`{{script "one.js"}}{{script "two.js"}}`, head))
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	moduleHints, fonts := 0, 0
	fontOutput := ""
	for _, d := range report.Packing {
		if d.Source == "font.woff2" {
			fontOutput = fileURL(d.Output)
		}
	}
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data != "link" {
			continue
		}
		if relToken(n, "modulepreload") {
			moduleHints++
			if attr(n, "href") != "./bundle.js" {
				t.Fatal(attr(n, "href"))
			}
		}
		if attr(n, "as") == "font" {
			fonts++
			if attr(n, "href") != fontOutput || !hasAttr(n, "crossorigin") {
				t.Fatal(n.Attr)
			}
		}
		if attr(n, "as") == "image" {
			t.Fatal("unused/inline image hint survived")
		}
	}
	if moduleHints != 1 || fonts != 1 {
		t.Fatal(moduleHints, fonts)
	}
}

func TestAutomaticSidecarsEscapeCSSAndResolveBasenameCollisions(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "one/same(a).png", strings.Repeat("a", 30000))
	write(t, root, "two/same(a).png", strings.Repeat("b", 30000))
	write(t, root, "theme.css", `.one{background:url('./one/same(a).png')}.two{background:url('./two/same(a).png')}`)
	write(t, root, "index.html.tmpl", helperHTML("", `{{style "theme.css"}}`))
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	page := read(t, root, "output/index.html")
	if strings.Contains(page, "url(./same(a)") {
		t.Fatal("unsafe CSS URL")
	}
	names := map[string]bool{}
	for _, f := range report.Files {
		if strings.HasPrefix(f.Name, "same(a)") && strings.HasSuffix(f.Name, ".png") {
			names[f.Name] = true
		}
	}
	if len(names) != 2 {
		t.Fatal("automatic basename collision lost an asset", names)
	}
}

func TestAutomaticCodePackingPreservesNativeInlineBoundaries(t *testing.T) {
	root := autoFixture(t)
	large := strings.Repeat("abc", 20000)
	write(t, root, "app.js", `window.moduleValue="`+large+`"`)
	page := helperHTML(`{{script "app.js"}}<script>window.classicValue="`+large+`"</script>`, "")
	write(t, root, "index.html.tmpl", page)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	moduleFile, classicInline := false, false
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "script" {
			if attr(n, "type") == "module" && attr(n, "src") != "" {
				moduleFile = true
			}
			if strings.Contains(textContent(n), "window.classicValue=") && attr(n, "src") == "" {
				classicInline = true
			}
		}
	}
	if !moduleFile || !classicInline || report.Cost.ExternalJSBytes < 50000 {
		t.Fatal(moduleFile, classicInline, report.Cost)
	}
	found := false
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "perf.budgets" && strings.Contains(d.Message, "JavaScript") {
			found = true
		}
	}
	if !found {
		t.Fatal("file-backed code evaded total budget")
	}
}

func TestUnusedLargeAutomaticImportDoesNotEmitSidecar(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "unused.png", strings.Repeat("x", 100000))
	write(t, root, "app.js", `import unused from './unused.png';window.answer=42`)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "app.js"}}`, ""))
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Files {
		if strings.HasPrefix(f.Name, "unused") {
			t.Fatal("unused file-loader output survived", f)
		}
	}
	for _, d := range report.Packing {
		if d.Source == "unused.png" {
			t.Fatal("unused asset appeared in decisions", d)
		}
	}
}

func TestSmallLinkedDocumentKeepsNavigableFileURL(t *testing.T) {
	root := autoFixture(t)
	write(t, root, "assets/free.pdf", "small PDF fixture")
	write(t, root, "index.html.tmpl", strings.Replace(helperHTML("", ""), "</body>", `<a href="assets/free.pdf">Read the document</a></body>`, 1))
	_, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "a" && strings.Contains(textContent(n), "Read the document") {
			found = strings.HasPrefix(attr(n, "href"), "./free-") && !strings.HasPrefix(attr(n, "href"), "data:")
		}
	}
	if !found {
		t.Fatal("tiny linked document became an unnavigable data URL")
	}
}
