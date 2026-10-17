package dataurl

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"strings"
	"testing"
)

func decode(t testing.TB, uri string) []byte {
	t.Helper()
	raw, _, err := Decode(uri)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSVGEncodingIsLosslessAndContextSafe(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><defs><linearGradient id="paint"><stop stop-color="#ffcc00"/></linearGradient></defs><path fill="url(#paint)" d="M0,0h40v40z"/><text xml:space="preserve">  café 🐺 &amp; 50% '+` + "`" + `\  </text></svg>`)
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	for _, source := range [][]byte{raw, all, {}, []byte("\xff\xfe<\x00s\x00v\x00g\x00")} {
		uri := SVG(source)
		if !bytes.Equal(decode(t, uri), source) {
			t.Fatal("encoded payload changed")
		}
		_, payload, _ := strings.Cut(uri, ",")
		if strings.ContainsAny(payload, "#,&\"'`<>\\(){} \t\r\n") {
			t.Fatal("unsafe character in SVG payload")
		}
	}
	uri, encoding := Encode(raw, "image/svg+xml")
	if encoding != "percent" || !bytes.Equal(decode(t, uri+"#paint"), raw) {
		t.Fatal("text SVG did not use lossless percent encoding", encoding)
	}
	base, encoding := Encode(all, "image/svg+xml")
	if encoding != "base64" || !bytes.Equal(decode(t, base), all) {
		t.Fatal("binary-heavy input should keep base64", encoding)
	}
}

func TestInlineEncodingChoosesCompressedCandidate(t *testing.T) {
	gzipSize := func(s string) int {
		var b bytes.Buffer
		w, _ := gzip.NewWriterLevel(&b, gzip.BestSpeed)
		w.Write([]byte(s))
		w.Close()
		return b.Len()
	}
	for _, raw := range [][]byte{[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`), []byte(strings.Repeat("#%'<>\"\x00\xff", 1000)), []byte(strings.Repeat(`<path fill="#abc" d="M1,2h3v4z"/>`, 300))} {
		text := SVG(raw)
		base := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(raw)
		got, _ := Encode(raw, "image/svg+xml")
		if gzipSize(got) > min(gzipSize(text), gzipSize(base)) {
			t.Fatal("selected the larger compressed candidate")
		}
		if !bytes.Equal(decode(t, got), raw) {
			t.Fatal("roundtrip")
		}
	}
	uri, encoding := Encode([]byte("binary"), "image/png")
	if encoding != "base64" || !bytes.Equal(decode(t, uri), []byte("binary")) {
		t.Fatal("non-SVG encoding changed")
	}
}

func FuzzEncodeRoundTrip(f *testing.F) {
	f.Add([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h1"/></svg>`), "image/svg+xml")
	f.Add([]byte{0, 0xff, '#', ',', '%'}, "image/png")
	f.Fuzz(func(t *testing.T, contents []byte, kind string) {
		if strings.ContainsAny(kind, ",;#%") || strings.TrimSpace(kind) != kind {
			return
		}
		uri, _ := Encode(contents, kind)
		got, gotKind, err := Decode(uri)
		if err != nil || !bytes.Equal(got, contents) || gotKind != kind {
			t.Fatalf("round trip %q: %v %q", kind, err, gotKind)
		}
	})
}
