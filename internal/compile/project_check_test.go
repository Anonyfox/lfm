package compile

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
)

func TestProjectCheckOptions(t *testing.T) {
	root := fixture(t)
	write(t, root, "output/social.png", "own file")
	options, err := ProjectCheckOptions(root, check.Options{File: "live.html", URL: "https://example.com/tools/demo/", Assets: map[string][]byte{"app.js": []byte("page bytes")}})
	if err != nil {
		t.Fatal(err)
	}
	if options.File != "live.html" || string(options.Assets["app.js"]) != "page bytes" || !options.GeneratedAssets["app.js"] || !options.GeneratedAssets["social.png"] || options.Registry == nil || len(options.RenderedSource) == 0 {
		t.Fatalf("%+v", options)
	}
	for _, dir := range []string{root + "/missing", t.TempDir()} {
		if _, err := ProjectCheckOptions(dir, check.Options{}); err == nil {
			t.Fatal("accepted missing project", dir)
		}
	}
	write(t, root, "lfm.json", "{")
	if _, err := ProjectCheckOptions(root, check.Options{}); err == nil {
		t.Fatal("accepted malformed project")
	}
}
