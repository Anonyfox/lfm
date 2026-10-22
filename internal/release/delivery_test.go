package release

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The audit serves text as hosts do, compressed when the browser takes
// it, and everything else as it is.
func TestAuditServesTextCompressedLikeAHost(t *testing.T) {
	t.Parallel()
	page := []byte("<!doctype html><title>t</title>" + strings.Repeat("<p>compressible text</p>", 400))
	css := []byte(strings.Repeat(".a{color:red}", 400))
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 4000))
	h := AuditHandler(page, map[string][]byte{"style.css": css, "i.png": png}, "http://example.test")
	get := func(path, accept string) *http.Response {
		r := httptest.NewRequest("GET", "http://example.test"+path, nil)
		if accept != "" {
			r.Header.Set("Accept-Encoding", accept)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Result()
	}
	for path, want := range map[string][]byte{"/": page, "/style.css": css} {
		res := get(path, "gzip, deflate, br, zstd")
		if res.Header.Get("Content-Encoding") != "gzip" || res.Header.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%s: %v", path, res.Header)
		}
		z, err := gzip.NewReader(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := io.ReadAll(z); !bytes.Equal(got, want) {
			t.Fatalf("%s does not decompress to the artifact", path)
		}
		if plain := get(path, ""); plain.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%s compressed for a client that takes no gzip", path)
		}
		if refused := get(path, "gzip;q=0, br"); refused.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%s compressed although gzip is refused", path)
		}
	}
	if res := get("/i.png", "gzip"); res.Header.Get("Content-Encoding") != "" {
		t.Fatal("an image was compressed again")
	}
}

func TestDeliveryCountsTheTextAHostCompresses(t *testing.T) {
	t.Parallel()
	page := []byte(strings.Repeat("<p>compressible text</p>", 4000))
	var tx texts
	d := tx.delivery(page, map[string][]byte{"style.css": []byte(strings.Repeat(".a{color:red}", 4000)), "font.woff2": make([]byte, 50000)})
	if d.TextBytes != len(page)+13*4000 || d.CompressedBytes <= 0 || d.CompressedBytes*10 > d.TextBytes {
		t.Fatalf("%+v", d)
	}
	want := int(float64(d.TextBytes-d.CompressedBytes) / mobileBytesPerSecond * 1000)
	if d.UncompressedMS != want || want < 500 {
		t.Fatalf("%+v, want %d ms", d, want)
	}
}
