package compile

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/dataurl"
)

func decodeInline(t testing.TB, uri string) []byte {
	t.Helper()
	uri, _, _ = strings.Cut(uri, "#")
	header, payload, ok := strings.Cut(uri, ",")
	if !ok {
		t.Fatal("missing data URL separator")
	}
	if strings.HasSuffix(header, ";base64") {
		raw, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	raw, err := url.PathUnescape(payload)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(raw)
}

func TestSVGEncodingSurvivesBundlingAndPacking(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	raw := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><defs><linearGradient id="paint"><stop stop-color="#f00"/></linearGradient></defs><path fill="url(#paint)" d="M0,0h20v20z"/><text xml:space="preserve">  café 🐺 &amp; 50% </text></svg>`
	write(t, root, "assets/images/picture.svg", raw)
	write(t, root, "assets/styles/main.css", `.hero{background:url('../images/picture.svg#paint')}`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	page := read(t, root, "output/index.html")
	var decision *AssetDecision
	for i := range report.Packing {
		if report.Packing[i].Source == "assets/images/picture.svg" {
			decision = &report.Packing[i]
		}
	}
	if decision == nil || decision.Encoding != "percent" || decision.Occurrences != 5 {
		t.Fatalf("decision: %+v", decision)
	}
	for _, embed := range check.InspectEmbeds([]byte(page), nil) {
		if embed.MIME == "image/svg+xml" {
			if !bytes.Equal(decodeInline(t, page[embed.Offset:embed.Offset+embed.Bytes]), []byte(raw)) {
				t.Fatal("bundled payload changed")
			}
		}
	}
	for _, e := range report.Validation.Stats.Embeds {
		if e.MIME == "image/svg+xml" && (e.Count != 5 || len(e.Sources) != 1 || e.Sources[0] != "assets/images/picture.svg") {
			t.Fatalf("lost source/occurrences: %+v", e)
		}
	}
	if !strings.Contains(page, "%23paint") {
		t.Fatal("internal fragment was not encoded")
	}
	if !strings.Contains(page, "#paint") {
		t.Fatal("external URL fragment lost")
	}
	if report.Compaction == nil || report.Compaction.Skipped != "" || report.Validation.Stats.HTMLBytes != len(page) || report.Cost.HTMLBytes != len(page) {
		t.Fatal("gate did not inspect final compact bytes")
	}
}

func TestSVGSidecarRewritePreservesFragments(t *testing.T) {
	t.Parallel()
	root := autoFixture(t)
	// Each occurrence reaches the holistic planner; together they cross 32 KiB.
	raw := `<svg xmlns="http://www.w3.org/2000/svg"><!--` + strings.Repeat("many repeated source bytes ", 300) + `--><view id="crop" viewBox="0 0 10 10"/><path d="M0 0h10v10z"/></svg>`
	write(t, root, "assets/images/picture.svg", raw)
	write(t, root, "index.html.tmpl", `<!doctype html><html lang="{{.language}}"><head><link rel="canonical" href="{{.url}}"><title>{{.title}}</title><style>.a{background:url('assets/images/picture.svg#crop')}</style></head><body><h1>{{.title}}</h1>`+strings.Repeat(`<img src="assets/images/picture.svg#crop" alt="diagram">`, 5)+`</body></html>`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	page := read(t, root, "output/index.html")
	var found bool
	for _, d := range report.Packing {
		if d.Source == "assets/images/picture.svg" {
			found = true
			if d.Mode != "file" || d.Occurrences != 6 || d.Encoding != "" || strings.Count(page, fileURL(d.Output)+"#crop") != 6 {
				t.Fatalf("rewrite: %+v", d)
			}
			if read(t, root, "output/"+d.Output) != raw {
				t.Fatal("sidecar changed")
			}
		}
	}
	if !found || strings.Contains(page, "data:image/svg+xml") {
		t.Fatal("SVG planner lost token")
	}
}

func BenchmarkSVGEncoding(b *testing.B) {
	raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat(`<path fill="#abcdef" d="M1,2h3v4z"/>`, 500) + `</svg>`)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		dataurl.Encode(raw, "image/svg+xml")
	}
}
