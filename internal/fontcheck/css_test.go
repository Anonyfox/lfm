package fontcheck

import (
	"strings"
	"testing"
)

func TestFaces(t *testing.T) {
	raw := `/* @font-face{broken} */ .x{content:"@font-face"} @layer fonts {@supports(display:grid){@font-face{font-family:"Demo Serif";src:local("A, B"),url("font.woff2?q=1#face") format("woff2") tech(variations),url(font.ttf) format(opentype);font-display:swap;font-weight:100 900;unicode-range:U+0000-00ff,U+4??}}} @font-face{font-family:Icon;src:local(Icon);font-display:optional}`
	faces, err := Faces([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) != 2 {
		t.Fatalf("%+v", faces)
	}
	f := faces[0]
	if f.Family != `"Demo Serif"` || f.Display != "swap" || f.Weight != "100 900" || !f.Conditional || len(f.Sources) != 3 || len(f.Problems) != 0 {
		t.Fatalf("%+v", f)
	}
	if !f.Sources[0].Local || f.Sources[1].URL != "font.woff2?q=1#face" || f.Sources[1].Format != "woff2" || f.Sources[2].Format != "opentype" {
		t.Fatal(f.Sources)
	}
	if faces[1].Conditional || faces[1].Display != "optional" {
		t.Fatal(faces[1])
	}
}

func TestDescriptorFallbackAndEscapes(t *testing.T) {
	faces, err := Faces([]byte(`@font-face{font-family:X;src:url(a.woff2) format(woff2);src:url(b.woff2) format("woff2") tech(color-COLRv1);font-display:swap;font-display:made-up;unicode-range:U+ffffff;src:url("a\20 b.woff2");font-weight:900!important}`))
	if err != nil {
		t.Fatal(err)
	}
	f := faces[0]
	if f.Display != "swap" || len(f.Sources) != 3 || f.Sources[2].URL != "a b.woff2" || len(f.Problems) != 3 {
		t.Fatalf("%+v", f)
	}
	if f.Weight != "" {
		t.Fatal("invalid important descriptor became effective")
	}
}

func TestFamilyDisplayDefaultsAndLayers(t *testing.T) {
	s, err := ReadCSS([]byte(`@font-feature-values "Demo Serif",Second {font-display:optional;@styleset{nice:1}}@layer fonts{@font-face{font-family:Demo Serif;src:url(a)}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Faces) != 1 || s.Faces[0].Conditional || !s.FamilyDefaults["demo serif"] || !s.FamilyDefaults["second"] {
		t.Fatalf("%+v", s)
	}
}

func TestUnicodeRanges(t *testing.T) {
	for _, v := range []string{"U+0-10FFFF", "u+4??, U+00A0", "U+???", "U+10????", "U+000000"} {
		if !ValidUnicodeRange(v) {
			t.Error("valid", v)
		}
	}
	for _, v := range []string{"", "U+FFFFFF", "U+??????", "U+1?????", "U+1234567", "U+2-1", "U++12", "U+1?F", "U+?1", "U+1-?", "U+1,", "inherit", "U+1 U+2"} {
		if ValidUnicodeRange(v) {
			t.Error("invalid", v)
		}
	}
}

func FuzzFaces(f *testing.F) {
	for _, s := range []string{`@font-face{font-family:X;src:url(a) format(woff2);font-display:swap}`, `@layer{ @font-face{src:local("\")")}}`, `@font-face{unicode-range:U+10????}`, `@font-feature-values "X",Y{font-display:optional;@styleset{a:1}}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 32768 {
			t.Skip()
		}
		Faces([]byte(s))
		ValidUnicodeRange(s)
	})
}

func BenchmarkFaces(b *testing.B) {
	raw := []byte(strings.Repeat(`@font-face{font-family:Demo;src:local(Demo),url(a.woff2) format(woff2);font-display:swap;font-weight:100 900;unicode-range:U+0-FF}`, 64))
	for b.Loop() {
		Faces(raw)
	}
}
