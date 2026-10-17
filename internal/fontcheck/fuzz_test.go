package fontcheck

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"os"
	"reflect"
	"testing"
)

func addFontSeeds(f *testing.F) {
	f.Add(woffFixture(false))
	f.Add(woffFixture(true))
	for _, name := range []string{"testdata/Go-Regular.ttf", "testdata/Go-Regular.woff2"} {
		if raw, err := os.ReadFile(name); err == nil && len(raw) <= 64*1024 {
			f.Add(raw)
		}
	}
	for _, raw := range []string{"ttcf\x00\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10", "OTTO", "true", "\x00\x01\x00\x00\x00\x01", "wOF"} {
		f.Add([]byte(raw))
	}
}

// Property: Inspect never mutates its input and reports exactly the sniffed
// Kind, which depends only on the four-byte signature and always has a MIME type.
func FuzzKind(f *testing.F) {
	addFontSeeds(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 256*1024 {
			t.Skip()
		}
		before := bytes.Clone(b)
		info := Inspect(b)
		kind := Kind(b)
		if !bytes.Equal(b, before) || info.Format != kind || info.Bytes != len(b) {
			t.Fatalf("Inspect %+v disagrees with Kind %q or mutated input", info, kind)
		}
		if (MIME(kind) == "") != (kind == "") || len(b) >= 4 && Kind(b[:4]) != kind {
			t.Fatalf("kind %q is not signature-only or lacks a MIME type", kind)
		}
		if kind == "" && info.Issue == "" {
			t.Fatal("unrecognized font without an issue")
		}
	})
}

// Property: a base64 or percent-encoded data URL inspects exactly like its bytes.
func FuzzDataMatchesInspect(f *testing.F) {
	addFontSeeds(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 64*1024 {
			t.Skip()
		}
		want := Inspect(b)
		for _, ref := range []string{"data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b) + "#face", "data:font/ttf," + url.PathEscape(string(b))} {
			if got := Data(ref); got != want {
				t.Fatalf("Data %+v, Inspect %+v", got, want)
			}
		}
	})
}

// Property: ReadCSS is deterministic, leaves its input untouched, agrees with
// Faces and reports faces in source order at in-bounds offsets.
func FuzzReadCSS(f *testing.F) {
	for _, s := range []string{
		`/* @font-face{broken} */ .x{content:"@font-face"} @layer fonts {@supports(display:grid){@font-face{font-family:"Demo Serif";src:local("A, B"),url("font.woff2?q=1#face") format("woff2") tech(variations),url(font.ttf) format(opentype);font-display:swap;font-weight:100 900;unicode-range:U+0000-00ff,U+4??}}} @font-face{font-family:Icon;src:local(Icon);font-display:optional}`,
		`@font-face{font-family:X;src:url(a) format(woff2);font-display:swap}`, `@layer{ @font-face{src:local("\")")}}`, `@font-face{unicode-range:U+10????}`,
		`@font-feature-values "X",Y{font-display:optional;@styleset{a:1}}`, `@font-face{font-family:A!important;src:url(x)}@font-face{`, `@media print{@font-face{src:url( "a\"b" )}}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 32768 {
			t.Skip()
		}
		raw := []byte(s)
		sheet, err := ReadCSS(raw)
		again, err2 := ReadCSS(raw)
		faces, _ := Faces(raw)
		if string(raw) != s || !reflect.DeepEqual(sheet, again) || (err == nil) != (err2 == nil) || !reflect.DeepEqual(sheet.Faces, faces) {
			t.Fatal("ReadCSS is not deterministic or mutated its input")
		}
		last := 0
		for _, face := range sheet.Faces {
			if face.Offset < last || face.Offset > len(raw) {
				t.Fatalf("face offset %d out of order or bounds", face.Offset)
			}
			last = face.Offset
		}
	})
}
