package compile

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
)

func helperHTML(script, style string) string {
	return strings.ReplaceAll(strings.ReplaceAll(fixtureHTML, `<script type="module" src="assets/scripts/app.js"></script>`, script), `<link rel="stylesheet" href="assets/styles/main.css">`, style)
}

func TestHelpersLocalPartialPackageAndRepeatedDeclarations(t *testing.T) {
	root := fixture(t)
	write(t, root, "features/shared.cjs", `window.events.push('shared');module.exports=7`)
	write(t, root, "features/main.ts", `import value from './shared.cjs';window.events.push('main:'+value)`)
	write(t, root, "partials/widgets/widget.js", `import value from '../../features/shared.cjs';window.events.push('widget:'+value)`)
	write(t, root, "partials/widgets/widget.css", `.widget{color:red}`)
	write(t, root, "partials/widget.html.tmpl", `{{define "widget"}}{{script "./widgets/widget.js"}}{{style "./widgets/widget.css"}}{{end}}`)
	write(t, root, "node_modules/demo/package.json", `{"name":"demo","exports":{".":"./index.cjs","./theme.css":"./theme.css"},"sideEffects":false}`)
	write(t, root, "node_modules/demo/index.cjs", `window.events.push('package');module.exports={}`)
	write(t, root, "node_modules/demo/theme.css", `.package{display:grid}`)
	write(t, root, "theme.css", `@import './base.css';.hero{color:blue}`)
	write(t, root, "base.css", `body{margin:0}`)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "features/main.ts"}}{{template "widget" .}}{{script "demo"}}{{script "./features/main.ts"}}{{script "demo"}}`, `{{style "theme.css"}}{{style "demo/theme.css"}}{{style "./theme.css"}}`))
	report, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	blobs := executableBlobs(t, root)
	if len(blobs) != 1 {
		t.Fatalf("scripts=%d", len(blobs))
	}
	result := runModule(t, blobs[0])
	if !reflect.DeepEqual(result["events"], []any{"shared", "main:7", "widget:7", "package"}) {
		t.Fatal(result)
	}
	page := read(t, root, "output/index.html")
	styles := 0
	for _, node := range parsePage(t, page) {
		if node.Data == "style" {
			styles++
			css := htmlx.RawText(node)
			for _, want := range []string{".widget", ".package", ".hero", "body"} {
				if !strings.Contains(css, want) {
					t.Fatal(css)
				}
			}
		}
	}
	if styles != 1 || strings.Contains(page, "data-lfm-helper") {
		t.Fatal("helpers were not reconciled", page)
	}
}

func TestHelperErrorsAggregatedBeforeStaging(t *testing.T) {
	root := fixture(t)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "missing-a.js"}}{{script "missing-b.js"}}`, `{{style ""}}`))
	report, err := Compile(root, Options{Publish: true})
	if err == nil || report.Validation == nil || report.Validation.Errors != 3 {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	assertClean(t, root)
	write(t, root, "index.html.tmpl", helperHTML(`{{if false}}{{script "missing.js"}}{{end}}{{script "assets/scripts/app.js"}}`, `{{style "assets/styles/main.css"}}`))
	report, err = Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "template.inactive" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing inactive helper advisory")
	}
}

func TestHelperContextsDormancyAndFileOverrides(t *testing.T) {
	root := fixture(t)
	write(t, root, "app.js", `window.helperApp=true`)
	write(t, root, "theme.css", `.helper{color:red}`)
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"].(map[string]any)["overrides"] = map[string]any{"app.js": "file", "theme.css": "file"}
	})
	write(t, root, "index.html.tmpl", helperHTML(`{{script "app.js"}}<template>{{script "app.js"}}{{style "theme.css"}}{{script "app.js"}}</template>`, `{{style "theme.css"}}`))
	report, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	if len(executableBlobs(t, root)) != 2 {
		t.Fatal("inert and active graphs were collapsed")
	}
	for _, name := range []string{"app.js", "theme.css", "template-1-app.js", "template-1-theme.css"} {
		if _, err := os.Stat(filepath.Join(root, "output", name)); err != nil {
			t.Fatal(err)
		}
	}
	before := read(t, root, "output/index.html")
	write(t, root, "index.html.tmpl", helperHTML(`<p title='{{script "app.js"}}'>Bad</p>`, `{{style "theme.css"}}`))
	if _, err := Compile(root, Options{Publish: true}); err == nil {
		t.Fatal("helper in attribute accepted")
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("failed helper replaced output")
	}
}

func TestCSSModulesImportedFromHelperEntry(t *testing.T) {
	root := fixture(t)
	write(t, root, "ui.module.css", `.card{color:rebeccapurple}.unused{color:green}`)
	write(t, root, "ui.js", `import classes from './ui.module.css';window.cssClass=classes.card;`)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "ui.js"}}`, `{{style "assets/styles/main.css"}}`))
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	result := runModule(t, executableBlobs(t, root)[0])
	if name, ok := result["cssClass"].(string); !ok || name == "card" || !strings.Contains(read(t, root, "output/index.html"), name) {
		t.Fatal(result)
	}
	write(t, root, "index.html.tmpl", helperHTML(`{{script "ui.js"}}`, `{{style "ui.module.css"}}`))
	if _, err := Compile(root, Options{Publish: true}); err == nil {
		t.Fatal("CSS module accepted without class mapping")
	}
	write(t, root, "index.html.tmpl", helperHTML(`{{script "ui.js"}}`, `<link rel="stylesheet" href="ui.module.css">`))
	if _, err := Compile(root, Options{Publish: true}); err == nil || !strings.Contains(err.Error(), "import .module.css") {
		t.Fatalf("native CSS module entry: %v", err)
	}
}

func TestIndependentCSSModuleGraphsCannotSilentlyCollide(t *testing.T) {
	root := fixture(t)
	write(t, root, "first.module.css", `.card{color:red}`)
	write(t, root, "second.module.css", `.card{color:blue}`)
	write(t, root, "first.js", `import css from './first.module.css';window.first=css.card`)
	write(t, root, "second.js", `import css from './second.module.css';window.second=css.card`)
	write(t, root, "index.html.tmpl", helperHTML(`{{script "first.js"}}{{script "second.js"}}`, `{{style "assets/styles/main.css"}}`))
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	result := runModule(t, executableBlobs(t, root)[0])
	if result["first"] == result["second"] {
		t.Fatal("CSS module classes collided in one graph")
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["assets"].(map[string]any)["overrides"] = map[string]any{"second.js": "file"}
	})
	if _, err := Compile(root, Options{Publish: true}); err == nil || !strings.Contains(err.Error(), "CSS Modules must share") {
		t.Fatalf("%v", err)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("unsafe CSS replaced output")
	}
}

func TestHelperPathsCannotEscapeOrReadGeneratedSources(t *testing.T) {
	for _, name := range []string{"../outside.js", "output/cached.js"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			if !strings.HasPrefix(name, "../") {
				write(t, root, name, `window.bad=true`)
			}
			write(t, root, "index.html.tmpl", helperHTML(fmt.Sprintf(`{{script %q}}`, name), `{{style "assets/styles/main.css"}}`))
			if _, err := Compile(root, Options{Publish: true}); err == nil {
				t.Fatal("forbidden source accepted")
			}
		})
	}
}

func TestHelperConfigBindingsLoopsAndPipelines(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["data"].(map[string]any)["scripts"] = []any{"assets/scripts/app.js", "assets/scripts/app.js"}
		c["data"].(map[string]any)["css"] = "assets/styles/main.css"
	})
	write(t, root, "index.html.tmpl", helperHTML(`{{range .data.scripts}}{{. | script}}{{end}}`, `{{style .data.css}}`))
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	if len(executableBlobs(t, root)) != 1 {
		t.Fatal("loop did not deduplicate helpers")
	}
	changeConfig(t, root, func(c map[string]any) { c["data"].(map[string]any)["css"] = 42 })
	report, err := Compile(root, Options{Publish: true})
	if err == nil || report.Validation == nil || report.Validation.Errors != 1 {
		t.Fatalf("%v %+v", err, report.Validation)
	}
}
