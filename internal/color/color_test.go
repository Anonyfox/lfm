package color

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestContrastMatchesWCAGReferenceValues(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"#000", "#fff", 21},
		{"#777777", "#ffffff", 4.48},
		{"#767676", "#ffffff", 4.54},
		{"#0000ff", "#ffffff", 8.59},
		{"#ff0000", "#ffffff", 4.0},
		{"#fff", "#fff", 1},
	} {
		a, _ := ParseHex(tc.a)
		b, _ := ParseHex(tc.b)
		if got := Contrast(a, b); !near(got, tc.want, 0.01) {
			t.Errorf("Contrast(%s, %s) = %.3f, want %.2f", tc.a, tc.b, got, tc.want)
		}
		if Contrast(a, b) != Contrast(b, a) {
			t.Error("contrast must be symmetric")
		}
	}
}

func TestHexRoundTrip(t *testing.T) {
	for _, h := range []string{"#000000", "#ffffff", "#6366f1", "#d97757", "#123456", "#fedcba", "#808080"} {
		c, ok := ParseHex(h)
		if !ok || c.Hex() != h {
			t.Errorf("%s -> %s", h, c.Hex())
		}
	}
	for _, bad := range []string{"", "#", "#12", "#12345", "#gggggg", "#1234567"} {
		if _, ok := ParseHex(bad); ok {
			t.Errorf("ParseHex(%q) accepted", bad)
		}
	}
	short, _ := ParseHex("#f0a8")
	if !near(short.Alpha, 0x88/255.0, 1e-9) || short.Hex() != "#ff00aa" {
		t.Fatalf("#rgba: %+v %s", short, short.Hex())
	}
}

func TestParseFormats(t *testing.T) {
	white, _ := ParseHex("#fff")
	for value, want := range map[string]int{
		"#fff":                             1,
		"rgb(255, 255, 255)":               1,
		"rgb(255 255 255 / 50%)":           1,
		"hsl(0 0% 100%)":                   1,
		"oklch(100% 0 0)":                  1,
		"white":                            1,
		"1px solid white, 0 0 0 2px #000":  2,
		"url(white.png)":                   0,
		"var(--white)":                     0,
		"currentColor transparent inherit": 0,
		"linear-gradient(red, #00f 50%)":   2,
		"--off-white":                      0,
		"rgb(":                             0,
		"#12345":                           0,
	} {
		got := Parse(value)
		if len(got) != want {
			t.Errorf("Parse(%q) found %d colors, want %d", value, len(got), want)
			continue
		}
		if want == 1 && value != "rgb(255 255 255 / 50%)" && got[0].Distance(white) > 1e-3 {
			t.Errorf("Parse(%q) = %s, want white", value, got[0].Hex())
		}
	}
	half := Parse("rgb(255 255 255 / 50%)")
	if !near(half[0].Alpha, 0.5, 1e-9) {
		t.Fatal("alpha")
	}
}

func TestOverAndPerception(t *testing.T) {
	black, _ := ParseHex("#000")
	white, _ := ParseHex("#fff")
	grey := Color{white.L, white.A, white.B, 0.5}.Over(black)
	if !near(grey.L, 0.5, 1e-6) {
		t.Fatalf("over: %+v", grey)
	}
	indigo, _ := ParseHex("#6366f1")
	if h := indigo.Hue(); h < 260 || h > 290 || indigo.Chroma() < 0.15 {
		t.Fatalf("indigo hue %.1f chroma %.3f", h, indigo.Chroma())
	}
}

func FuzzParseIsTotal(f *testing.F) {
	f.Add("rgb(1 2 3 / 4%) #fff hsl(1 2 3) oklch(1 2 3) red")
	f.Add("((#")
	f.Fuzz(func(t *testing.T, v string) {
		for _, c := range Parse(v) {
			if math.IsNaN(c.L) || c.Alpha < 0 || c.Alpha > 1 {
				t.Fatalf("%q produced %+v", v, c)
			}
			if r := Contrast(c, c); !near(r, 1, 1e-9) {
				t.Fatalf("self contrast %v", r)
			}
		}
	})
}
