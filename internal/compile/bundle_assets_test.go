package compile

import (
	"strings"
	"testing"
)

func TestFileLoaderPreservesResourceSuffixes(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "file" })
	write(t, root, "assets/styles/main.css", `.hero{background:url('../images/picture.svg?v=1#shape')}`)
	write(t, root, "assets/scripts/extra.css", `.from-js{background:url('../images/picture.svg?v=2#other')}`)
	report, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	css := read(t, root, "output/bundle.css")
	if !strings.Contains(css, "picture.svg?v=1#shape") || !strings.Contains(css, "picture.svg?v=2#other") {
		t.Fatal(css)
	}
}

func TestFileLoaderEncodesAssetNames(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "file" })
	write(t, root, "assets/images/a b#c.webp", "asset")
	write(t, root, "assets/scripts/app.js", `import asset from "../images/a b#c.webp";window.asset=asset;`)
	report, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	result := runModule(t, executableBlobs(t, root)[0])
	if result["asset"] != "./a%20b%23c.webp" {
		t.Fatalf("asset URL not encoded: %#v", result["asset"])
	}
}
