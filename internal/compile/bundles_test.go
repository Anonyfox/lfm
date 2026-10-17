package compile

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
)

func executableBlobs(t *testing.T, root string) []string {
	t.Helper()
	var blobs []string
	for _, node := range parsePage(t, read(t, root, "output/index.html")) {
		kind := htmlx.Attr(node, "type")
		if node.Data != "script" || kind != "" && kind != "module" {
			continue
		}
		if path := htmlx.Attr(node, "src"); path != "" {
			blobs = append(blobs, read(t, root, "output/"+strings.TrimPrefix(path, "./")))
		} else {
			blobs = append(blobs, htmlx.RawText(node))
		}
	}
	return blobs
}

// Optional runtime verification; Go-only builds/tests remain usable without Node.
func runModule(t *testing.T, source string) map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for optional bundled-JS execution check")
	}
	url := "data:text/javascript;base64," + base64.StdEncoding.EncodeToString([]byte(source))
	program := "globalThis.window={events:[]};await import(" + strconvJSON(url) + ");console.log(JSON.stringify(window));"
	output, err := exec.Command(node, "--input-type=module", "-e", program).CombinedOutput()
	if err != nil {
		t.Fatalf("bundle runtime: %v\n%s", err, output)
	}
	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err, string(output))
	}
	return result
}

func strconvJSON(value string) string { raw, _ := json.Marshal(value); return string(raw) }

func TestPackagesVendorFormatsAndTreeShaking(t *testing.T) {
	for _, mode := range []string{"inline", "file"} {
		t.Run(mode, func(t *testing.T) {
			root := fixture(t)
			changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = mode })
			write(t, root, "package.json", `{"private":true,"sideEffects":false,"dependencies":{"fixture-pkg":"1.0.0"}}`)
			write(t, root, "node_modules/fixture-pkg/package.json", `{"name":"fixture-pkg","version":"1.0.0","browser":{"fs":false},"sideEffects":["*.css"],"exports":{".":{"browser":{"import":"./browser.mjs","require":"./browser.cjs"},"default":"./node.cjs"},"./styles.css":"./styles.css"}}`)
			write(t, root, "node_modules/fixture-pkg/browser.mjs", `export const value="browser-esm"; export function dead(){return "UNUSED_FUNCTION_MARKER"} export const mode=process.env.NODE_ENV;`)
			write(t, root, "node_modules/fixture-pkg/browser.cjs", `const fs=require("fs");module.exports={value:Object.keys(fs).length===0?"browser-cjs":"wrong-browser-map"};`)
			write(t, root, "node_modules/fixture-pkg/node.cjs", `throw new Error("NODE_BRANCH_MUST_NOT_SHIP");`)
			write(t, root, "node_modules/fixture-pkg/styles.css", `@font-face{font-family:Package;src:url('./font.woff2')} .package{background:url('./icon.svg')}`)
			write(t, root, "node_modules/fixture-pkg/font.woff2", "package-font")
			write(t, root, "node_modules/fixture-pkg/icon.svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
			write(t, root, "assets/scripts/vendor/old.cjs", `const pkg=require("fixture-pkg");module.exports=()=>pkg.value;`)
			write(t, root, "assets/scripts/vendor/modern.mjs", `export default "vendor-esm";`)
			write(t, root, "assets/scripts/vendor/typed.ts", `export const number:number=7;`)
			write(t, root, "assets/images/unused.webp", "UNUSED_ASSET_MARKER")
			write(t, root, "assets/scripts/app.js", `import {value,mode} from "fixture-pkg"; import old from "./vendor/old.cjs"; import modern from "./vendor/modern.mjs"; import {number} from "./vendor/typed.ts"; import unused from "../images/unused.webp"; import "fixture-pkg/styles.css"; window.result=[value,old(),modern,number,mode];`)
			report, err := Compile(context.Background(), root, Options{Publish: true})
			if err != nil {
				t.Fatalf("%v %+v", err, report.Validation)
			}
			blobs := executableBlobs(t, root)
			if len(blobs) != 1 || strings.Contains(blobs[0], "UNUSED_FUNCTION_MARKER") || strings.Contains(blobs[0], "UNUSED_ASSET_MARKER") || strings.Contains(blobs[0], "NODE_BRANCH_MUST_NOT_SHIP") {
				t.Fatal(blobs)
			}
			for _, file := range report.Files {
				if file.Name == "unused.webp" {
					t.Fatal("tree-shaken asset emitted")
				}
			}
			styles := 0
			for _, node := range parsePage(t, read(t, root, "output/index.html")) {
				if node.Data == "style" || node.Data == "link" && htmlx.Attr(node, "rel") == "stylesheet" {
					styles++
				}
			}
			if styles != 1 {
				t.Fatalf("wanted one CSS bundle, got %d", styles)
			}
			if mode == "file" {
				css := read(t, root, "output/bundle.css")
				if !strings.Contains(css, "./font.woff2") || !strings.Contains(css, "./icon.svg") {
					t.Fatal(css)
				}
			}
			result := runModule(t, blobs[0])
			if !reflect.DeepEqual(result["result"], []any{"browser-esm", "browser-cjs", "vendor-esm", float64(7), "production"}) {
				t.Fatal(result)
			}
		})
	}
}

func TestMultipleEntriesShareOneGraph(t *testing.T) {
	root := fixture(t)
	write(t, root, "assets/scripts/shared.mjs", `window.events.push("shared"); export const value=3;`)
	write(t, root, "assets/scripts/app.js", `import {value} from "./shared.mjs";window.events.push("first:"+value);`)
	write(t, root, "assets/scripts/second.cjs", `const {value}=require("./shared.mjs");window.events.push("second:"+value);`)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<script src="assets/scripts/second.cjs"></script></body>`, 1))
	if report, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	blobs := executableBlobs(t, root)
	if len(blobs) != 1 {
		t.Fatal(len(blobs))
	}
	result := runModule(t, blobs[0])
	if !reflect.DeepEqual(result["events"], []any{"shared", "first:3", "second:3"}) {
		t.Fatal(result)
	}
}

func TestUntypedModuleEntriesWork(t *testing.T) {
	for _, source := range []string{`import {answer} from "./answer.js";window.answer=answer;`, `const {answer}=require("./answer.js");window.answer=answer;`} {
		root := fixture(t)
		write(t, root, "assets/scripts/app.js", source)
		write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, `type="module" src="assets/scripts/app.js"`, `src="assets/scripts/app.js"`, 1))
		if report, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
			t.Fatalf("%v %+v", err, report.Validation)
		}
		result := runModule(t, executableBlobs(t, root)[0])
		if result["answer"] != float64(42) {
			t.Fatal(result)
		}
	}
}

func TestCSSMediaOrderAndNativeBoundaries(t *testing.T) {
	root := fixture(t)
	write(t, root, "assets/styles/second.css", `.hero{color:blue}.never-in-static-html{display:block}`)
	write(t, root, "assets/styles/alternate.css", `.hero{color:green}`)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</head>", `<style>.hero{color:red}</style><link rel="stylesheet" href="assets/styles/second.css" media="print"><link rel="alternate stylesheet" title="Green" href="assets/styles/alternate.css"><link id="switchable" rel="stylesheet" href="assets/styles/second.css" media="screen" disabled></head>`, 1))
	if report, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	var css string
	alternate := false
	switchable := false
	for _, node := range parsePage(t, read(t, root, "output/index.html")) {
		if node.Data == "style" {
			css += htmlx.RawText(node)
		}
		if htmlx.Attr(node, "rel") == "alternate stylesheet" && htmlx.Attr(node, "title") == "Green" && htmlx.Attr(node, "href") != "" {
			alternate = true
		}
		if htmlx.Attr(node, "id") == "switchable" {
			switchable = node.Data == "link" && htmlx.HasAttr(node, "disabled") && htmlx.Attr(node, "media") == "screen" && htmlx.Attr(node, "href") != ""
		}
	}
	if !alternate || !switchable || !strings.Contains(css, "@media print") || !strings.Contains(css, ".never-in-static-html") || strings.Index(css, "color:red") > strings.Index(css, "color:#00f") {
		t.Fatal(css, alternate)
	}
}

func TestDeferredClassicKeepsNativeSource(t *testing.T) {
	root := fixture(t)
	write(t, root, "assets/scripts/global.js", `function globalHandler(){return 42}`)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, `<script type="module" src="assets/scripts/app.js"></script>`, `<script defer src="assets/scripts/global.js"></script>`, 1))
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range parsePage(t, read(t, root, "output/index.html")) {
		if node.Data == "script" && htmlx.HasAttr(node, "defer") && htmlx.Attr(node, "src") != "" && htmlx.Attr(node, "type") == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("defer lost its native loading behavior")
	}
}

func TestPackageFailurePreservesOutput(t *testing.T) {
	root := fixture(t)
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	write(t, root, "assets/scripts/app.js", `import value from "not-installed-package";window.value=value;`)
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err == nil || !strings.Contains(err.Error(), "npm ci") {
		t.Fatal(err)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("package failure replaced output")
	}
	outside := t.TempDir()
	write(t, outside, "index.js", `window.bad=true`)
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "node_modules", "escape")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "assets/scripts/app.js", `import "escape";`)
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err == nil {
		t.Fatal("package symlink escaped the project")
	}
}

func TestLegacyUMDAndClassicGlobalsRemainAvailable(t *testing.T) {
	root := fixture(t)
	write(t, root, "package.json", `{"type":"module"}`)
	write(t, root, "assets/scripts/legacy.js", `(function(root,factory){if(typeof module==="object" && module.exports)module.exports=factory();else root.Legacy=factory()})(window,function(){return {answer:42}});`)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, `<script type="module" src="assets/scripts/app.js"></script>`, `<script data-lfm-classic src="assets/scripts/legacy.js"></script><script>function publishedHandler(){return window.Legacy.answer}window.result=publishedHandler()</script>`, 1))
	if report, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	page := read(t, root, "output/index.html")
	if strings.Contains(page, "data-lfm-classic") {
		t.Fatal("build annotation leaked")
	}
	blobs := executableBlobs(t, root)
	if len(blobs) != 2 {
		t.Fatal("classic globals merged unsafely")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for optional classic script execution check")
	}
	program := `const vm=require("node:vm");const window={};const context=vm.createContext({window});`
	for _, blob := range blobs {
		program += "vm.runInContext(" + strconvJSON(blob) + ",context);"
	}
	program += `console.log(JSON.stringify({result:window.result,globalHandler:vm.runInContext("typeof publishedHandler",context)}));`
	output, err := exec.Command(node, "-e", program).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(output))
	}
	if !strings.Contains(string(output), `"result":42`) || !strings.Contains(string(output), `"globalHandler":"function"`) {
		t.Fatal(string(output))
	}
}

func TestHTMLTemplateBundlesStayDormant(t *testing.T) {
	root := fixture(t)
	write(t, root, "assets/scripts/dormant.js", `window.dormant=true;`)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<template id="result"><style>.dormant{color:red}</style><div class="dormant">Future</div><script type="module" src="assets/scripts/dormant.js"></script></template></body>`, 1))
	if report, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	styles, scripts := 0, 0
	for _, node := range parsePage(t, read(t, root, "output/index.html")) {
		if node.Data == "style" && strings.Contains(htmlx.RawText(node), ".dormant") {
			styles++
			if node.Parent.Data != "template" {
				t.Fatal("dormant styles activated")
			}
		}
		if node.Data == "script" && strings.Contains(htmlx.RawText(node), "dormant") {
			scripts++
			if node.Parent.Data != "template" {
				t.Fatal("dormant script activated")
			}
		}
	}
	if styles != 1 || scripts != 1 {
		t.Fatal(styles, scripts)
	}
}
