package compile

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/release"
)

func cardProject(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["socialImage"] = map[string]any{"src": "assets/share/card.html", "alt": "A comparison"}
	})
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, `content="assets/images/picture.svg"`, `content="{{.socialImage.src}}"`, 1))
	write(t, root, "assets/share/card.html", `<!doctype html><html><head><link rel="stylesheet" href="card.css"></head><body><h1>Compare</h1><img src="../images/picture.svg"></body></html>`)
	write(t, root, "assets/share/card.css", `body{margin:0;background:#123456;color:white}h1{font-size:80px}`)
	return root
}

func TestHTMLSocialCardMissingBrowser(t *testing.T) {
	t.Setenv("LFM_CHROME", filepath.Join(t.TempDir(), "missing"))
	r, err := Compile(context.Background(), cardProject(t), Options{})
	if err == nil {
		t.Fatal("missing Chrome succeeded")
	}
	if r.Errors() != 1 {
		t.Fatalf("expected one error: %+v", r.Validation)
	}
	var d = r.Validation.Diagnostics[0]
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "build.failed" {
			d = issue
		}
	}
	if d.Path != "socialImage.src" || d.Location.File != "lfm.json" || !strings.Contains(d.Message, "assets/share/card.html") || !strings.Contains(d.Hint, "LFM_CHROME") {
		t.Fatalf("%+v", d)
	}
}

func TestHTMLSocialCardRenderAndReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	root := cardProject(t)
	r, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v: %+v", err, r.Validation)
	}
	var card []byte
	for _, raw := range r.Artifact {
		if socialPNGStamp(raw) != "" || stampOf(raw) != "" {
			if info, _, e := image.DecodeConfig(bytes.NewReader(raw)); e == nil && info.Width == 1200 && info.Height == 630 {
				card = raw
			}
		}
	}
	if len(card) == 0 {
		t.Fatal("no 1200x630 social image", r.Files)
	}
	t.Setenv("LFM_CHROME", filepath.Join(t.TempDir(), "missing"))
	again, err := Compile(context.Background(), root, Options{})
	if err != nil {
		t.Fatalf("cached card needs Chrome: %v %+v", err, again.Validation)
	}
	write(t, root, "assets/share/card.css", `body{background:red}`)
	changed, err := Compile(context.Background(), root, Options{})
	if err == nil || changed.Errors() != 1 {
		t.Fatalf("changed asset reused: %v %+v", err, changed.Validation)
	}
	write(t, root, "assets/share/card.css", `body{margin:0;background:#123456;color:white}h1{font-size:80px}`)
	write(t, root, "assets/share/card.html", read(t, root, "assets/share/card.html")+"<!-- source changed -->")
	changed, err = Compile(context.Background(), root, Options{})
	if err == nil || changed.Errors() != 1 {
		t.Fatalf("changed card source reused: %v %+v", err, changed.Validation)
	}
}

func TestSocialCardKeyIncludesDocumentAndAssets(t *testing.T) {
	original := printKey("social-1200x630", []byte("card"), map[string][]byte{"font.woff2": []byte("font")})
	for _, key := range []string{printKey("social-1200x630", []byte("changed"), map[string][]byte{"font.woff2": []byte("font")}), printKey("social-1200x630", []byte("card"), map[string][]byte{"font.woff2": []byte("changed")})} {
		if key == original {
			t.Fatal("changed input reused key")
		}
	}
}

func TestSocialCardTimeoutFinding(t *testing.T) {
	d := socialCardFailure("assets/share/card.html", context.DeadlineExceeded)
	if d.Path != "socialImage.src" || !strings.Contains(d.Message, "card.html: did not render within 60 s") || !strings.Contains(d.Hint, "1200x630 PNG/JPEG") {
		t.Fatalf("%+v", d)
	}
}

func TestCardPNGStamp(t *testing.T) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 32)
	stamped := stampCardPNG(raw.Bytes(), key)
	if socialPNGStamp(stamped) != key {
		t.Fatal("missing PNG stamp")
	}
	if _, err := png.Decode(bytes.NewReader(stamped)); err != nil {
		t.Fatal(err)
	}
	stamped[len(stamped)-14] ^= 1
	if socialPNGStamp(stamped) != "" {
		t.Fatal("corrupt stamp accepted")
	}
}

func FuzzSocialPNGStamp(f *testing.F) {
	f.Add([]byte("not a PNG"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		key := socialPNGStamp(raw)
		if key != "" && len(key) != 32 {
			t.Fatal("invalid key")
		}
	})
}
