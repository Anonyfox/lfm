package compile

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

const testBookSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 175"><circle cx="50" cy="87" r="30" fill="teal"/></svg>`

func TestBookSVGWithoutChrome(t *testing.T) {
	t.Setenv("LFM_CHROME", filepath.Join(t.TempDir(), "missing-chrome"))
	r := &book.Report{Sources: map[string][]byte{"images/a.svg": []byte(testBookSVG + "\n"), "images/b.svg": []byte(testBookSVG + "\n")}}
	st := newStock(context.Background(), project.Project{Root: t.TempDir()}, false)
	defer st.close()
	count := 0
	images := bookImages(r, st, func(_ diagnostic.Severity, rule, message, hint string) {
		count++
		if rule != book.RuleImages || message != "The EPUB keeps SVG, which Kindle's Enhanced Typesetting does not support." || hint == "" {
			t.Fatal(rule, message, hint)
		}
	})
	if count != 1 || len(images) != 0 {
		t.Fatal(count, images)
	}
}

func TestBookSVGRasterAndCache(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	r := &book.Report{Sources: map[string][]byte{"images/a.svg": []byte(testBookSVG)}}
	st := newStock(context.Background(), project.Project{Root: t.TempDir()}, false)
	defer st.close()
	warn := func(_ diagnostic.Severity, rule, message, hint string) { t.Fatal(rule, message, hint) }
	images := bookImages(r, st, warn)
	data := images["images/a.svg"]
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 1600 || im.Bounds().Dy() != 2800 {
		t.Fatal(im.Bounds())
	}
	red, green, blue, _ := im.At(0, 0).RGBA()
	if red != 65535 || green != 65535 || blue != 65535 {
		t.Fatal("SVG ground is not white")
	}
	cached := newStock(context.Background(), project.Project{Root: t.TempDir()}, false)
	cached.started = true
	cached.startErr = errors.New("browser must not start on a cache hit")
	again := bookImages(r, cached, warn)
	if !bytes.Equal(again["images/a.svg"], data) {
		t.Fatal("cached rendition differs")
	}
	r.Sources["images/a.svg"] = bytes.ReplaceAll([]byte(testBookSVG), []byte("teal"), []byte("red"))
	count := 0
	changed := bookImages(r, cached, func(_ diagnostic.Severity, _, _, _ string) { count++ })
	if len(changed) != 0 || count != 1 {
		t.Fatal("source changes did not invalidate cache")
	}
}
