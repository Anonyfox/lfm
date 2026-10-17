// Package color parses CSS colors and compares them perceptually in OKLab,
// with exact WCAG 2 contrast ratios. It has no dependencies beyond math.
package color

import (
	"math"
	"strconv"
	"strings"
)

// Color is a color in OKLab with straight alpha in [0, 1].
type Color struct{ L, A, B, Alpha float64 }

// Chroma is the colorfulness in OKLCH.
func (c Color) Chroma() float64 { return math.Hypot(c.A, c.B) }

// Hue is the OKLCH hue in degrees, [0, 360).
func (c Color) Hue() float64 {
	h := math.Atan2(c.B, c.A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

// Distance is the Euclidean distance in OKLab, ignoring alpha. About 0.02 is
// a just-noticeable difference.
func (c Color) Distance(o Color) float64 {
	return math.Sqrt((c.L-o.L)*(c.L-o.L) + (c.A-o.A)*(c.A-o.A) + (c.B-o.B)*(c.B-o.B))
}

// Over composites c on an opaque backdrop. Blending happens in OKLab, which is
// close enough for the comparisons made here.
func (c Color) Over(backdrop Color) Color {
	if c.Alpha >= 1 {
		return c
	}
	a := c.Alpha
	return Color{c.L*a + backdrop.L*(1-a), c.A*a + backdrop.A*(1-a), c.B*a + backdrop.B*(1-a), 1}
}

// Luminance is the WCAG 2 relative luminance of the opaque color.
func (c Color) Luminance() float64 {
	r, g, b := c.linear()
	clamp := func(v float64) float64 { return math.Max(0, math.Min(1, v)) }
	return 0.2126*clamp(r) + 0.7152*clamp(g) + 0.0722*clamp(b)
}

// Contrast is the WCAG 2 contrast ratio between two opaque colors, 1 to 21.
func Contrast(a, b Color) float64 {
	x, y := a.Luminance(), b.Luminance()
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// Hex formats the opaque color as #rrggbb, clamping out-of-gamut values.
func (c Color) Hex() string {
	r, g, b := c.linear()
	byteOf := func(v float64) int {
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		}
		return int(math.Round(math.Max(0, math.Min(1, v)) * 255))
	}
	return "#" + hex2(byteOf(r)) + hex2(byteOf(g)) + hex2(byteOf(b))
}

func hex2(v int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&15]})
}

// linear converts to linear sRGB (the inverse of FromSRGB's matrices).
func (c Color) linear() (r, g, b float64) {
	l := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s := c.L - 0.0894841775*c.A - 1.2914855480*c.B
	l, m, s = l*l*l, m*m*m, s*s*s
	return 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s
}

// FromSRGB converts gamma-encoded sRGB channels in [0, 1].
func FromSRGB(r, g, b, alpha float64) Color {
	lin := func(c float64) float64 {
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b = lin(r), lin(g), lin(b)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return Color{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
		alpha,
	}
}

// ParseHex reads #rgb, #rgba, #rrggbb or #rrggbbaa.
func ParseHex(h string) (Color, bool) {
	h = strings.TrimPrefix(h, "#")
	if len(h) == 3 || len(h) == 4 {
		long := make([]byte, 0, 8)
		for i := 0; i < len(h); i++ {
			long = append(long, h[i], h[i])
		}
		h = string(long)
	}
	if len(h) != 6 && len(h) != 8 {
		return Color{}, false
	}
	n, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		return Color{}, false
	}
	alpha := 1.0
	if len(h) == 8 {
		alpha = float64(n&0xff) / 255
		n >>= 8
	}
	return FromSRGB(float64(n>>16&0xff)/255, float64(n>>8&0xff)/255, float64(n&0xff)/255, alpha), true
}

// Named CSS colors that matter for page design. Others are rare in authored
// styles and are ignored rather than guessed.
var named = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000", "blue": "#0000ff",
	"yellow": "#ffff00", "orange": "#ffa500", "purple": "#800080", "gray": "#808080", "grey": "#808080",
	"silver": "#c0c0c0", "navy": "#000080", "teal": "#008080", "maroon": "#800000", "olive": "#808000",
	"lime": "#00ff00", "aqua": "#00ffff", "fuchsia": "#ff00ff", "rebeccapurple": "#663399",
}

// Parse extracts every literal color from a CSS value: hex, rgb(a), hsl(a),
// oklch and the common named colors. Keywords such as currentColor,
// transparent and var() references are not colors and are skipped.
func Parse(v string) []Color {
	var out []Color
	lower := strings.ToLower(v)
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		switch {
		case c == '#':
			j := i + 1
			for j < len(lower) && isHex(lower[j]) {
				j++
			}
			if col, ok := ParseHex(lower[i:j]); ok {
				out = append(out, col)
			}
			i = j - 1
		case c >= 'a' && c <= 'z' && (i == 0 || !isIdent(lower[i-1])):
			j := i
			for j < len(lower) && isIdent(lower[j]) {
				j++
			}
			word := lower[i:j]
			if j < len(lower) && lower[j] == '(' {
				switch word {
				case "rgb", "rgba", "hsl", "hsla", "oklch", "url", "var", "env", "attr":
					end := strings.IndexByte(lower[j:], ')')
					if end < 0 {
						return out
					}
					if col, ok := function(word, lower[j+1:j+end]); ok {
						out = append(out, col)
					}
					i = j + end
				default: // gradients and other functions hold colors among their arguments
					i = j
				}
				continue
			}
			if h, ok := named[word]; ok {
				col, _ := ParseHex(h)
				out = append(out, col)
			}
			i = j - 1
		}
	}
	return out
}

func isHex(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' }
func isIdent(b byte) bool {
	return b == '-' || b == '_' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func function(name, args string) (Color, bool) {
	args = strings.NewReplacer(",", " ", "/", " ").Replace(args)
	f := strings.Fields(args)
	if len(f) < 3 {
		return Color{}, false
	}
	num := func(s string, percentScale float64) (float64, bool) {
		if p, ok := strings.CutSuffix(s, "%"); ok {
			v, err := strconv.ParseFloat(p, 64)
			return v / 100 * percentScale, err == nil
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "deg"), 64)
		return v, err == nil
	}
	alpha := 1.0
	if len(f) >= 4 {
		if a, ok := num(f[3], 1); ok {
			alpha = math.Max(0, math.Min(1, a))
		}
	}
	switch name {
	case "rgb", "rgba":
		r, ok1 := num(f[0], 255)
		g, ok2 := num(f[1], 255)
		b, ok3 := num(f[2], 255)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		return FromSRGB(r/255, g/255, b/255, alpha), true
	case "hsl", "hsla":
		h, ok1 := num(f[0], 1)
		s, ok2 := num(f[1], 100)
		l, ok3 := num(f[2], 100)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		s, l = s/100, l/100
		k := func(n float64) float64 { return math.Mod(n+h/30, 12) }
		a := s * math.Min(l, 1-l)
		ch := func(n float64) float64 { return l - a*math.Max(-1, math.Min(math.Min(k(n)-3, 9-k(n)), 1)) }
		return FromSRGB(ch(0), ch(8), ch(4), alpha), true
	case "oklch":
		L, ok1 := num(f[0], 1)
		C, ok2 := num(f[1], 0.4)
		H, ok3 := num(f[2], 1)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		if !strings.HasSuffix(f[0], "%") && L > 1 {
			L /= 100
		}
		return Color{L, C * math.Cos(H*math.Pi/180), C * math.Sin(H*math.Pi/180), alpha}, true
	}
	return Color{}, false
}
