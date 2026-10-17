package compile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
)

func fontProject(t *testing.T, style, head string) string {
	t.Helper()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"] = map[string]any{"default": "auto"} })
	write(t, root, "index.html.tmpl", helperHTML("", head+`<style>`+style+`</style>`))
	return root
}

func realFont(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../fontcheck/testdata/Go-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFontPackingAndHintRepairsPreserveBytes(t *testing.T) {
	font := realFont(t)
	// An arbitrary filename still gets recognized after bundling; no new author syntax.
	root := fontProject(t, `@font-face{font-family:"Demo";src:local("Absent Demo"),url(./type.bin) format(truetype);font-display:optional;font-weight:400;unicode-range:U+0-FF}h1{font-family:Demo,serif}`, `<link rel="preload" as="font" href="type.bin">`)
	write(t, root, "type.bin", string(font))
	report, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	if len(report.Fonts) != 1 || report.Fonts[0].Format != "truetype" || report.FontHints != 1 {
		t.Fatal(report.Fonts, report.FontHints)
	}
	var output string
	for _, a := range report.Packing {
		if a.Source == "type.bin" {
			if a.Mode != "file" {
				t.Fatal(a)
			}
			output = a.Output
		}
	}
	if output == "" || !bytes.Equal([]byte(read(t, root, "output/"+output)), font) {
		t.Fatal("font binary changed")
	}
	page := read(t, root, "output/index.html")
	for _, n := range parsePage(t, page) {
		if n.Data == "link" && htmlx.Attr(n, "as") == "font" {
			if htmlx.Attr(n, "type") != "font/ttf" || !htmlx.HasAttr(n, "crossorigin") || htmlx.Attr(n, "href") != fileURL(output) {
				t.Fatal(n.Attr)
			}
		}
	}
	for _, want := range []string{"font-display:optional", "unicode-range:U+0-FF", `local("Absent Demo")`} {
		if !strings.Contains(page, want) {
			t.Fatal("authored font intent lost", want)
		}
	}
}

func TestFontAuthoredCredentialTypeAndInlinePolicyPreserved(t *testing.T) {
	root := fontProject(t, `@font-face{font-family:Demo;src:url(./a.ttf);font-display:block}`, `<link rel="preload" as="font" href="a.ttf" crossorigin="use-credentials" type="font/woff2">`)
	write(t, root, "a.ttf", string(realFont(t)))
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.FontHints != 0 {
		t.Fatal("authored hint overwritten")
	}
	page := read(t, root, "output/index.html")
	if !strings.Contains(page, `crossorigin=use-credentials`) || !strings.Contains(page, "type=font/woff2") {
		t.Fatal("authored hints changed")
	}
	found := false
	for _, d := range r.Validation.Diagnostics {
		if d.Rule == "perf.fonts" && strings.Contains(d.Message, "use-credentials") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing mismatch warning")
	}
	changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"] = map[string]any{"default": "inline"} })
	r, err = Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	page = read(t, root, "output/index.html")
	if !strings.Contains(page, "data:font/ttf;base64,") || strings.Contains(page, "as=font") {
		t.Fatal("inline pin/preload reconciliation failed")
	}
	if _, err := os.Stat(filepath.Join(root, "output/a.ttf")); !os.IsNotExist(err) {
		t.Fatal("stale font sidecar")
	}
}

func TestRemoteFontHintGetsCORSWithoutFetching(t *testing.T) {
	root := fontProject(t, `@font-face{font-family:Demo;src:url({{.urls.font}}) format(woff2);font-display:swap}`, `<link rel="preload" as="font" href="{{.urls.font}}">`)
	changeConfig(t, root, func(c map[string]any) { c["urls"] = map[string]any{"font": "https://example.invalid/demo.woff2"} })
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	if len(r.Fonts) != 0 || r.FontHints != 1 {
		t.Fatal(r.Fonts, r.FontHints)
	}
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "link" && htmlx.Attr(n, "as") == "font" {
			if !htmlx.HasAttr(n, "crossorigin") || htmlx.HasAttr(n, "type") {
				t.Fatal(n.Attr)
			}
		}
	}
}

func TestFontOpaqueSourceAndRenamedContainer(t *testing.T) {
	root := fontProject(t, `@font-face{font-family:Bad;src:url(./response.bin);font-display:swap}`, "")
	write(t, root, "response.bin", `<html>not a font</html>`)
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Fonts) != 1 || r.Fonts[0].Issue == "" {
		t.Fatal("opaque font source was not inspected", r.Fonts)
	}
	found := false
	for _, d := range r.Validation.Diagnostics {
		if d.Rule == "perf.font-assets" && d.Path == "response.bin" {
			found = true
		}
	}
	if !found {
		t.Fatal("source attribution lost")
	}
	// Small valid SFNT directory, padded past the font threshold. It falls below
	// the generic 24 KiB early-file threshold, so content recognition decides.
	raw := make([]byte, 8192)
	copy(raw, []byte{0, 1, 0, 0, 0, 1})
	copy(raw[12:], "name")
	raw[23] = 28
	raw[27] = 4
	write(t, root, "renamed.bin", string(raw))
	write(t, root, "index.html.tmpl", helperHTML("", `<style>@font-face{font-family:X;src:url(./renamed.bin);font-display:swap}</style><link rel="preload" as="font" href="renamed.bin"><link rel="preload" as="font" href="renamed.bin" crossorigin="anonymous">`))
	r, err = Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, a := range r.Packing {
		if a.Source == "renamed.bin" && a.Mode == "file" && strings.Contains(a.Reason, "font selection") {
			found = true
		}
	}
	if !found {
		t.Fatal(r.Packing)
	}
	hints := 0
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "link" && htmlx.Attr(n, "as") == "font" {
			hints++
		}
	}
	if hints != 1 {
		t.Fatal("equivalent CORS hints not deduplicated", hints)
	}
}
