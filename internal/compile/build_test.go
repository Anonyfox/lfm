package compile

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func write(t *testing.T, root, name, value string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	value, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func changeConfig(t *testing.T, root string, change func(map[string]any)) {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(read(t, root, "lfm.json")), &config); err != nil {
		t.Fatal(err)
	}
	change(config)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "lfm.json", string(raw))
}

const fixtureHTML = `<!doctype html><html lang="{{.language}}"><head>
<link rel="canonical" href="{{.url}}">
<title>{{.title}}</title><meta property="og:image" content="assets/images/picture.svg">
<link rel="stylesheet" href="assets/styles/main.css"></head><body>
<h1>{{.title}}</h1>{{if .data.enabled}}{{template "partials/answer.html.tmpl" .}}{{end}}
{{range .data.faq}}<details id="{{.id}}"><summary>{{.question}}</summary><p>{{.answer}}</p></details>{{end}}
<img src="assets/images/picture.svg" srcset="data:image/png;base64,aGVsbG8= 1x, assets/images/picture.svg 2x" alt="diagram">
<div style="background-image:url('assets/images/picture.svg')"></div>
<script type="module" src="assets/scripts/app.js"></script></body></html>`

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "lfm.json", `{
"schemaVersion":1,"url":"https://example.com/tools/demo/","language":"en",
"title":"A < B & café 🐺","description":"Example",
"build":{"browserData":["data.quiz"],"assets":{"default":"inline"}},
"data":{"enabled":true,"quiz":{"value":42,"prompt":"</script><script id=intruder>bad()</script>"},"unpublished":"PRIVATE_CONFIG_MARKER"}}
`)
	write(t, root, "index.html.tmpl", fixtureHTML)
	write(t, root, "partials/answer.html.tmpl", `<p id="explanation">A freely authored partial.</p>`)
	write(t, root, "data/faq.json", `[{"id":"faq-one","question":"Question?","answer":"A useful answer."}]`)
	write(t, root, "assets/styles/main.css", `@import "./theme.css"; @font-face {font-family:Example;src:url('../fonts/demo.woff2')} .hero{background:url('../images/picture.svg')}`)
	write(t, root, "assets/styles/theme.css", `p { color: #123456; }`)
	write(t, root, "assets/scripts/app.js", `import { answer } from './answer.js'; import picture from '../images/picture.svg'; import './extra.css'; window.fixture={answer,picture};`)
	write(t, root, "assets/scripts/answer.js", `export const answer = 42;`)
	write(t, root, "assets/scripts/extra.css", `.from-js { display: grid; }`)
	write(t, root, "assets/images/picture.svg", `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`)
	write(t, root, "assets/fonts/demo.woff2", "test-font-bytes")
	write(t, root, "assets/images/unused.svg", "must not be emitted")
	return root
}

func parsePage(t *testing.T, value string) []*html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(value))
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			nodes = append(nodes, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return nodes
}

func TestInlineBuild(t *testing.T) {
	root := fixture(t)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range report.Files {
		names = append(names, file.Name)
	}
	if !reflect.DeepEqual(names, []string{"favicon.png", "index.html", "picture.svg"}) {
		t.Fatalf("files: %v", names)
	}
	page := read(t, root, "output/index.html")
	if strings.Contains(page, "PRIVATE_CONFIG_MARKER") || strings.Contains(page, "{{") || strings.Contains(page, "ZgotmplZ") {
		t.Fatal("unselected config or unresolved template leaked")
	}
	if !strings.Contains(page, "A useful answer.") || !strings.Contains(page, "A freely authored partial.") {
		t.Fatal("dataset/partial not rendered")
	}
	var dataPosition, scriptPosition int
	for position, node := range parsePage(t, page) {
		if attr(node, "id") == "intruder" {
			t.Fatal("JSON escaped its script element")
		}
		if node.Data == "meta" && attr(node, "property") == "og:image" && attr(node, "content") != "https://example.com/tools/demo/picture.svg" {
			t.Fatal("incorrect social image URL")
		}
		if node.Data == "style" {
			if node.Parent.Data != "head" {
				t.Fatal("CSS outside head")
			}
			if strings.Contains(textContent(node), "@import") {
				t.Fatal("CSS import was not bundled")
			}
		}
		if node.Data == "img" {
			raw := decodeInline(t, attr(node, "src"))
			if string(raw) != read(t, root, "assets/images/picture.svg") {
				t.Fatal("image did not round-trip")
			}
			if !strings.Contains(attr(node, "srcset"), "data:image/png;base64,aGVsbG8= 1x") {
				t.Fatal("existing srcset data URL was damaged")
			}
		}
		if node.Data == "script" {
			if attr(node, "src") != "" {
				t.Fatal("script wasn't inlined")
			}
			if attr(node, "id") == "product-data" {
				dataPosition = position
				var payload map[string]map[string]map[string]any
				if err := json.Unmarshal([]byte(textContent(node)), &payload); err != nil {
					t.Fatal(err)
				}
				if payload["data"]["quiz"]["value"] != float64(42) {
					t.Fatal("runtime value lost its numeric type")
				}
			} else {
				scriptPosition = position
				if strings.Contains(textContent(node), "import ") {
					t.Fatal("module import not bundled")
				}
			}
		}
	}
	if dataPosition == 0 || scriptPosition <= dataPosition {
		t.Fatal("runtime data must precede scripts")
	}
	if !strings.Contains(page, "data:font/woff2;base64,") {
		t.Fatal("font not embedded")
	}
	info, err := os.Stat(filepath.Join(root, "output"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("output must be readable by the deployment server")
	}
	first := page
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	if read(t, root, "output/index.html") != first {
		t.Fatal("same sources produced different output")
	}
}

func TestFileModeAndCleanReplacement(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "file" })
	write(t, root, "output/stale.txt", "old")
	write(t, root, "output/old/nested.txt", "old")
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.js", "bundle.css", "demo.woff2", "favicon.png", "index.html", "picture.svg"}
	var got []string
	for _, file := range report.Files {
		got = append(got, file.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files: %v", got)
	}
	entries, err := os.ReadDir(filepath.Join(root, "output"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Fatal("stale files survived")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("output must be flat")
		}
	}
	css := read(t, root, "output/bundle.css")
	if !strings.Contains(css, "./demo.woff2") || !strings.Contains(css, "./picture.svg") {
		t.Fatal("CSS references not rewritten")
	}
	before := read(t, root, "output/index.html")
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "<h1>", "<h1 id=changed>", 1))
	if _, err := Run(root, false); err != nil {
		t.Fatal(err)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("check changed output")
	}
}

func TestFailuresPreserveOutput(t *testing.T) {
	cases := []struct {
		name, expected string
		change         func(*testing.T, string)
	}{
		{"missing value", "missing", func(t *testing.T, r string) { write(t, r, "index.html.tmpl", "{{.data.missing}}") }},
		{"unknown setting", "unknown field", func(t *testing.T, r string) { changeConfig(t, r, func(c map[string]any) { c["typo"] = true }) }},
		{"unknown runtime path", "unknown path", func(t *testing.T, r string) {
			changeConfig(t, r, func(c map[string]any) { c["build"].(map[string]any)["browserData"] = []string{"data.unknown"} })
		}},
		{"data collision", "both", func(t *testing.T, r string) {
			changeConfig(t, r, func(c map[string]any) { c["data"].(map[string]any)["faq"] = []any{} })
		}},
		{"missing image", "missing.svg", func(t *testing.T, r string) {
			write(t, r, "index.html.tmpl", `<img src="assets/images/missing.svg">`)
		}},
		{"JS syntax", "Expected", func(t *testing.T, r string) { write(t, r, "assets/scripts/app.js", "const =;") }},
		{"base element", "<base>", func(t *testing.T, r string) {
			write(t, r, "index.html.tmpl", `<base href="https://elsewhere.example/">`)
		}},
		{"reserved ID", "reserved", func(t *testing.T, r string) { write(t, r, "index.html.tmpl", `<div id="product-data"></div>`) }},
		{"duplicate ID", "duplicate HTML id", func(t *testing.T, r string) { write(t, r, "index.html.tmpl", `<p id="x"></p><p id="x"></p>`) }},
		{"filename collision", "collision", func(t *testing.T, r string) {
			write(t, r, "assets/a/icon.svg", "one")
			write(t, r, "assets/b/ICON.svg", "two")
			write(t, r, "index.html.tmpl", `<img src="assets/a/icon.svg"><img src="assets/b/ICON.svg">`)
			changeConfig(t, r, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "file" })
		}},
		{"reserved filename", "reserved", func(t *testing.T, r string) {
			write(t, r, "assets/index.html", "not a document")
			write(t, r, "index.html.tmpl", `<a href="assets/index.html">Download</a>`)
			changeConfig(t, r, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "file" })
		}},
		{"generated input", "outside output/", func(t *testing.T, r string) { write(t, r, "index.html.tmpl", `<img src="output/sentinel.txt">`) }},
		{"escape", "inside the workbench", func(t *testing.T, r string) { write(t, r, "index.html.tmpl", `<img src="../outside.svg">`) }},
		{"asset symlink", "symlink", func(t *testing.T, r string) {
			outside := t.TempDir()
			write(t, outside, "image.svg", "outside")
			if err := os.Symlink(filepath.Join(outside, "image.svg"), filepath.Join(r, "assets/images/link.svg")); err != nil {
				t.Skip(err)
			}
			write(t, r, "index.html.tmpl", `<img src="assets/images/link.svg">`)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			write(t, root, "output/sentinel.txt", "last good build")
			test.change(t, root)
			report, err := Run(root, true)
			message := ""
			if err != nil {
				message = err.Error()
			}
			if report.Validation != nil {
				for _, diagnostic := range report.Validation.Diagnostics {
					message += diagnostic.String()
				}
			}
			if err == nil || !strings.Contains(message, test.expected) {
				t.Fatalf("wanted %q, got %v", test.expected, err)
			}
			if read(t, root, "output/sentinel.txt") != "last good build" {
				t.Fatal("last build changed on failure")
			}
		})
	}
}

func TestOutputSymlinkAndBuildLock(t *testing.T) {
	root := fixture(t)
	outside := t.TempDir()
	write(t, outside, "keep.txt", "safe")
	if err := os.Symlink(outside, filepath.Join(root, "output")); err != nil {
		t.Skip(err)
	}
	if _, err := Run(root, true); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("got %v", err)
	}
	if read(t, outside, "keep.txt") != "safe" {
		t.Fatal("followed output symlink")
	}
	if err := os.Remove(filepath.Join(root, "output")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".micro/build.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, true); err == nil || !strings.Contains(err.Error(), "acquire") {
		t.Fatalf("got %v", err)
	}
}

func TestRemoteResourcesAndClassicGlobals(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"stylesheet": "https://example.org/theme.css", "image": "https://example.org/image.webp", "sdk": "https://example.org/sdk.js"}
	})
	write(t, root, "index.html.tmpl", `<!doctype html><html><head><link rel="canonical" href="{{.url}}"><link rel="stylesheet" href="{{.urls.stylesheet}}"></head><body>
<img src="{{.urls.image}}"><script src="{{.urls.sdk}}"></script>
<script>function publishedHandler() { return "</scr" + "ipt>"; }</script></body></html>`)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.External) != 3 {
		t.Fatalf("external assets: %v", report.External)
	}
	page := read(t, root, "output/index.html")
	if !strings.Contains(page, "function publishedHandler()") {
		t.Fatal("classic global scope was changed")
	}
	if strings.Contains(page, `return"</script>"`) {
		t.Fatal("script closing tag wasn't escaped")
	}
}

func TestExample(t *testing.T) {
	root := t.TempDir()
	// Copy authoring inputs only. Ignored browser/npm research and output can
	// be large; copying and then deleting them made local verification slow.
	source := os.DirFS("../../testdata/example")
	if err := fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".micro" || entry.Name() == "output" || entry.Name() == "node_modules" {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(root, filepath.FromSlash(path)), 0755)
		}
		raw, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), raw, 0644)
	}); err != nil {
		t.Fatal(err)
	}
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) != 3 {
		t.Fatalf("example files: %v", report.Files)
	}
	page := read(t, root, "output/index.html")
	if !strings.Contains(page, "1.500") && !strings.Contains(page, "defaultParticipants") {
		t.Fatal("calculator data missing")
	}
	if !bytes.Contains([]byte(page), []byte("data:image/svg+xml,")) && !bytes.Contains([]byte(page), []byte("data:image/svg+xml;base64,")) {
		t.Fatal("example diagram missing")
	}
}
