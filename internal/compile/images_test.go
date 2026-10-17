package compile

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
)

func imagePNG(t testing.TB, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func imageProject(t *testing.T, body, styles string) string {
	t.Helper()
	root := fixture(t)
	write(t, root, "index.html.tmpl", `<!doctype html><html lang="{{.language}}"><head><link rel="canonical" href="{{.url}}"><title>{{.title}}</title><style>`+styles+`</style></head><body><h1>{{.title}}</h1>`+body+`</body></html>`)
	if err := os.WriteFile(filepath.Join(root, "assets/images/photo.png"), imagePNG(t, 12, 8), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestImageInferencePreservesAuthoredSizing(t *testing.T) {
	body := `<img id="plain" src="assets/images/photo.png" alt=""><img id="width" src="assets/images/photo.png" width="006" alt=""><img id="height" src="assets/images/photo.png" height="4" alt=""><img id="both" src="assets/images/photo.png" width="5" height="7" alt=""><img id="zero" src="assets/images/photo.png" width="0" alt=""><img id="empty" src="assets/images/photo.png" width="" alt=""><img id="relative" src="assets/images/photo.png" width="50%" alt=""><img id="fraction" src="assets/images/photo.png" width="5" alt=""><template><img id="dormant" src="assets/images/photo.png" alt=""></template>`
	root := imageProject(t, body, "")
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		id := htmlx.Attr(n, "id")
		want := map[string][2]string{"plain": {"12", "8"}, "width": {"006", "4"}, "height": {"6", "4"}, "both": {"5", "7"}, "zero": {"0", ""}, "empty": {"", ""}, "relative": {"50%", ""}, "fraction": {"5", ""}, "dormant": {"", ""}}
		if pair, ok := want[id]; ok {
			if htmlx.Attr(n, "width") != pair[0] || htmlx.Attr(n, "height") != pair[1] {
				t.Fatalf("%s got %q×%q, want %v", id, htmlx.Attr(n, "width"), htmlx.Attr(n, "height"), pair)
			}
			if id == "empty" && !htmlx.HasAttr(n, "width") {
				t.Fatal("empty deliberate attribute was removed")
			}
		}
	}
	if len(report.Images) != 9 || len(report.Images[0].Added) != 2 || len(report.Images[1].Added) != 1 || len(report.Images[3].Added) != 0 {
		t.Fatal(report.Images)
	}
	if report.Validation.Stats.HTMLBytes != len(read(t, root, "output/index.html")) {
		t.Fatal("not final-byte validation")
	}
}

func TestImageInferenceScreensCSSWithoutChangingIt(t *testing.T) {
	for _, test := range []struct {
		name, style, wrapper string
		infer                bool
	}{
		{"responsive", "img{width:100%;height:auto}", "", true},
		{"both-auto", "img{width:auto;height:auto}", "", true},
		{"width-only", "img{width:100%}", "", false},
		{"height-only", "img{height:30px}", "", false},
		{"max-width", "img{max-width:100%;height:auto}", "", false},
		{"ratio", "img{aspect-ratio:1/1}", "", false},
		{"grid-parent", "div{display:grid}", "div", false},
		{"flex-parent", "div{display:flex}", "div", false},
		{"variable-layout", "div{--layout:flex;display:var(--layout)}", "div", false},
		{"grid-with-auto-image", "div{display:grid}img{width:auto;height:auto}", "div", true},
		{"border-box", "*{box-sizing:border-box}img{padding:10px}", "", false},
		{"attribute-selector", "img[width]{color:red}", "", false},
		{"dynamic", "img:hover{width:50%}", "", false},
		{"conditional", "@media(min-width:500px){img{width:100%;height:auto}}", "", false},
		{"conditional-with-base", "img{width:100%;height:auto}@media(min-width:500px){img{height:10px}}", "", true},
		{"revert", "img{width:100%;height:auto}.image{height:revert}", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := `<img class="image" src="assets/images/photo.png" alt="">`
			if test.wrapper != "" {
				body = "<" + test.wrapper + ">" + body + "</" + test.wrapper + ">"
			}
			root := imageProject(t, body, test.style)
			report, err := Compile(context.Background(), root, Options{Publish: true})
			if err != nil {
				t.Fatal(err, report.Validation)
			}
			decision := report.Images[0]
			if (len(decision.Added) == 2) != test.infer {
				t.Fatalf("infer=%t: %+v", test.infer, decision)
			}
		})
	}
	root := imageProject(t, `<img src="assets/images/photo.png" style="width:100%;height:auto" alt="">`, "")
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil || len(report.Images[0].Added) != 2 {
		t.Fatal(err, report.Images)
	}
}

func TestImageInferenceHandlesInlineFilesSVGAndResponsiveSelection(t *testing.T) {
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imagePNG(t, 12, 8))
	body := `<img id="data" src="` + data + `" alt=""><img id="file" src="assets/images/photo.png?v=1" alt=""><img id="svg" src="fixed.svg" alt=""><img id="viewbox" src="ratio.svg" alt=""><img id="fragment" src="fixed.svg#crop" alt=""><img id="srcset" src="assets/images/photo.png" srcset="assets/images/photo.png 1x, assets/images/photo.png 2x" alt=""><picture><source type="image/png" srcset="assets/images/photo.png" width="20" height="30"><img id="picture" src="assets/images/photo.png" alt=""></picture>`
	root := imageProject(t, body, "")
	write(t, root, "fixed.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><view id="crop" viewBox="0 0 10 10"/></svg>`)
	write(t, root, "ratio.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 60"/>`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		switch htmlx.Attr(n, "id") {
		case "data", "file":
			if htmlx.Attr(n, "width") != "12" || htmlx.Attr(n, "height") != "8" {
				t.Fatal("raster inference", htmlx.Attr(n, "id"))
			}
		case "svg":
			if htmlx.Attr(n, "width") != "120" || htmlx.Attr(n, "height") != "60" {
				t.Fatal("SVG inference")
			}
		case "viewbox", "fragment", "srcset", "picture":
			if htmlx.HasAttr(n, "width") || htmlx.HasAttr(n, "height") {
				t.Fatalf("unsafe inference %s", htmlx.Attr(n, "id"))
			}
		}
		if n.Data == "source" && (htmlx.Attr(n, "width") != "20" || htmlx.Attr(n, "height") != "30") {
			t.Fatal("art direction changed")
		}
	}
	if len(report.Images[5].Candidates) != 2 || len(report.Images[6].Candidates) != 1 {
		t.Fatal("candidate attribution lost")
	}
	if !bytes.Equal([]byte(read(t, root, "output/photo.png")), imagePNG(t, 12, 8)) {
		t.Fatal("image bytes changed")
	}
}

func TestOversizedImageWarningsAreActionableAndDeduplicated(t *testing.T) {
	root := imageProject(t, `<img src="large.png" width="100" height="100" alt=""><img src="large.png" width="100" height="100" alt="">`, "")
	// Change only the valid IHDR and CRC. Header inspection must never allocate
	// a 20-megapixel pixel buffer or pretend to validate the compressed payload.
	raw := imagePNG(t, 12, 8)
	binary.BigEndian.PutUint32(raw[16:], 5000)
	binary.BigEndian.PutUint32(raw[20:], 4000)
	binary.BigEndian.PutUint32(raw[29:], crc32.ChecksumIEEE(raw[12:29]))
	if err := os.WriteFile(filepath.Join(root, "large.png"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	count := 0
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "perf.image-assets" {
			count++
			if !strings.Contains(issue.Message, "5000×4000") || !strings.Contains(issue.Message, "RGBA") || issue.Path != "large.png" || issue.Location.File != "index.html" {
				t.Fatal(issue)
			}
		}
	}
	if count != 1 {
		t.Fatalf("warnings=%d", count)
	}
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "img" && (htmlx.Attr(n, "width") != "100" || htmlx.Attr(n, "height") != "100") {
			t.Fatal("deliberate sizing changed")
		}
	}
}
