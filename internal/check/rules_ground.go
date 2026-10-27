package check

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/color"
	"github.com/Anonyfox/lfm/internal/dataurl"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
)

const groundHint = "Use white, a cool or neutral grey, dark, or a saturated colour from the subject; remove paper, parchment and notebook decoration."

// groundRule uses the default desktop/light cascade, including inline styles.
// Browser geometry is not available in the final-document check pipeline.
func groundRule(d *Document) []Diagnostic {
	m := d.styles()
	var out []Diagnostic
	grounds := map[*html.Node]bool{}
	for _, n := range d.Elements {
		if n.Data == "html" || n.Data == "body" || n.Data == "main" {
			grounds[n] = true
		}
	}
	// A block wrapper occupies the available width unless constrained explicitly.
	for _, body := range d.activeNodes("body") {
		if paintedGround(m.value(body, "background-color")) {
			continue
		}
		mainPainted := false
		for _, main := range d.activeNodes("main") {
			if d.designVisible(main) && paintedGround(m.value(main, "background-color")) {
				mainPainted = true
				break
			}
		}
		if mainPainted {
			continue
		}
		for n := body.FirstChild; n != nil; n = n.NextSibling {
			if n.Type != html.ElementNode || n.Data != "div" && n.Data != "main" {
				continue
			}
			position := m.value(n, "position")
			if (position == "absolute" || position == "fixed") && !fullCover(m, n, grounds) {
				continue
			}
			display := m.value(n, "display")
			if (display == "" || display == "block" || display == "flex" || display == "grid" || display == "flow-root") && m.value(n, "max-width") == "" && (m.value(n, "width") == "" || m.value(n, "width") == "100%" || m.value(n, "width") == "100vw") {
				grounds[n] = true
			}
			break
		}
	}
	reported := map[string]bool{}
	inspect := func(n *html.Node, model *styleModel, selector string, ground bool) {
		if n.Namespace != "" || !d.designVisible(n) {
			return
		}
		// Cards, bands and panels count, but incidental or invisible paint does not.
		// Gradient stops are images, not the colour layer of a shorthand.
		for _, c := range color.Parse(withoutGradientImages(model.value(n, "background-color"))) {
			if !color.WarmLightGround(c) || groundHidden(model, n) || !ground && !fullCover(model, n, grounds) && insignificantGround(model, n) {
				continue
			}
			c = c.Over(color.FromSRGB(1, 1, 1, 1))
			if key := selector + " " + c.Hex(); !reported[key] {
				reported[key] = true
				out = append(out, d.At(n, Error, fmt.Sprintf("%s has prohibited warm-light background %s (OKLCH %.3f %.4f %.1f°).", selector, c.Hex(), c.L, c.Chroma(), c.Hue()), groundHint))
			}
		}
		if ground || fullCover(model, n, grounds) {
			image := model.value(n, "background-image")
			size := model.value(n, "background-size")
			if size == "" && strings.Contains(image, "/") {
				size = "shorthand"
			}
			if groundTexture(image, size, model.value(n, "background-repeat"), d.Options.Assets) {
				out = append(out, d.At(n, Error, selector+" has prohibited paper, parchment or notebook ground texture.", groundHint))
			}
		}
		for _, layer := range []string{"background", "mask", "-webkit-mask"} {
			image := layer + "-image"
			if layer == "background" {
				image = "background-image"
			}
			if paperEdge(model.value(n, image), model.value(n, layer+"-size"), model.value(n, layer+"-repeat")) {
				out = append(out, d.At(n, Error, selector+" has a prohibited torn, zigzag or perforated edge.", groundHint))
				break
			}
		}
	}
	for _, n := range d.Elements {
		inspect(n, m, elementLabel(n), grounds[n])
	}
	// Pseudo-elements have their own cascade but inherit custom properties from
	// their originating element. Never let their declarations alter its styles.
	for _, pseudo := range []string{"before", "after"} {
		pm := &styleModel{origin: m, base: map[*html.Node][]*cssRule{}, computed: map[*html.Node]map[string]string{}, inline: map[*html.Node][]cssDecl{}}
		for _, r := range m.rules {
			if r.pseudo != pseudo || r.alt || r.narrow || r.state != "" {
				continue
			}
			for _, n := range r.matches {
				pm.base[n] = append(pm.base[n], r)
			}
		}
		for _, n := range d.Elements {
			rules := pm.base[n]
			if len(rules) == 0 {
				continue
			}
			sort.SliceStable(rules, func(i, j int) bool {
				if rules[i].spec != rules[j].spec {
					return rules[i].spec.Less(rules[j].spec)
				}
				return rules[i].order < rules[j].order
			})

			inspect(n, pm, elementLabel(n)+"::"+pseudo, false)
		}
	}
	return out
}

// groundSurfaceArea is the smallest painted box (px²) that counts as a
// surface. Measured in Chrome at 1280x800 and 390x844 on the Dev News pages
// that 0.1.13 flagged: legend swatches render at 13x8 (102 px²) and marker
// lines at 2x40 to 2x50 (80-100 px²), while the smallest real cream surface,
// a one-line card in the prohibited fixtures, is 374x18 (6732 px²). 16x16 sits
// well clear of the markers and far below any card, chip or band.
const groundSurfaceArea = 256.0

// insignificantGround is deliberately conservative: unresolved dimensions never
// become zero. This filter applies only to colour, never textures or torn edges.
func insignificantGround(m *styleModel, n *html.Node) bool {
	w, wk := groundDimension(m, n, "width", "left", "right")
	h, hk := groundDimension(m, n, "height", "top", "bottom")
	position := m.value(n, "position")
	if position == "absolute" || position == "fixed" {
		for _, axis := range []struct {
			sides []string
			size  float64
			known bool
		}{
			{[]string{"left", "right"}, w, wk}, {[]string{"top", "bottom"}, h, hk},
		} {
			for _, side := range axis.sides {
				offset, ok := groundLength(m.value(n, side))
				if ok && offset < 0 && (axis.known && -offset >= axis.size || !axis.known && offset <= -300) {
					return true
				}
			}
		}
	}
	clip := strings.ReplaceAll(strings.Join(strings.Fields(m.value(n, "clip")), ""), ",", " ")
	if (position == "absolute" || position == "fixed") && strings.HasPrefix(clip, "rect(") && strings.HasSuffix(clip, ")") {
		values := strings.FieldsFunc(m.value(n, "clip")[5:len(m.value(n, "clip"))-1], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
		if len(values) == 4 {
			t, tk := groundLength(values[0])
			r, rk := groundLength(values[1])
			b, bk := groundLength(values[2])
			l, lk := groundLength(values[3])
			if tk && rk && bk && lk && (r <= l || b <= t) {
				return true
			}
		}
	}
	cp := strings.Join(strings.Fields(m.value(n, "clip-path")), "")
	if cp == "inset(50%)" || cp == "circle(0)" || cp == "circle(0px)" {
		return true
	}
	if wk && hk {
		return w*h < groundSurfaceArea
	}
	return wk && w <= 4 || hk && h <= 4
}

func groundHidden(m *styleModel, n *html.Node) bool {
	base := m
	if m.origin != nil {
		base = m.origin
	}
	// display and opacity suppress an entire subtree; visibility can be restored
	// by a descendant, so use its nearest explicit value instead.
	visibility := m.value(n, "visibility")
	start := n.Parent
	if m.origin != nil {
		start = n
	}
	if m.value(n, "display") == "none" || zeroGroundOpacity(m.value(n, "opacity")) {
		return true
	}
	for p := start; p != nil; p = p.Parent {
		if p.Type != html.ElementNode {
			continue
		}
		if base.value(p, "display") == "none" || zeroGroundOpacity(base.value(p, "opacity")) {
			return true
		}
		if visibility == "" {
			visibility = base.value(p, "visibility")
		}
	}
	if visibility == "hidden" || visibility == "collapse" {
		return true
	}
	return false
}

func zeroGroundOpacity(v string) bool {
	if v == "" {
		return false
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	return err == nil && f == 0
}

// Unlike lengthPx, this preserves the distinction between zero and unknown and
// does not approximate functions containing percentages or viewport units.
func groundLength(v string) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if f, err := strconv.ParseFloat(v, 64); err == nil && f == 0 {
		return 0, true
	}
	for _, unit := range []string{"px", "rem", "em", "pt"} {
		if strings.HasSuffix(v, unit) {
			f, err := strconv.ParseFloat(strings.TrimSuffix(v, unit), 64)
			scale := 1.0
			if unit == "em" || unit == "rem" {
				scale = 16
			} else if unit == "pt" {
				scale = 4.0 / 3
			}
			return f * scale, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
		}
	}
	return 0, false
}

func groundDimension(m *styleModel, n *html.Node, dimension, first, second string) (float64, bool) {
	size, known := groundLength(m.value(n, dimension))
	if !known || size < 0 {
		return 0, false
	}
	// min-size can enlarge a small declared width/height into a real panel.
	if v := m.value(n, "min-"+dimension); v != "" && v != "auto" {
		min, ok := groundLength(v)
		if !ok {
			return 0, false
		}
		size = math.Max(size, min)
	}
	padding := 0.0
	for _, side := range []string{first, second} {
		v := m.value(n, "padding-"+side)
		if v == "" {
			continue
		}
		p, ok := groundLength(v)
		if !ok || p < 0 {
			return 0, false
		}
		padding += p
	}
	if m.value(n, "box-sizing") == "border-box" {
		// CSS cannot shrink a border-box below its padding.
		size = math.Max(size, padding)
	} else {
		size += padding
	}
	return size, true
}

// paperEdge reports small gradient tiles repeated along one axis: diagonal
// hard-stop teeth (a torn or zigzag edge) or radial dots and scallops (a
// perforation or stamp edge). Both are the receipt and ticket motif of
// paper-style pages; dashed rules and broad bands are not matched.
func paperEdge(image, size, repeat string) bool {
	lower := strings.ToLower(image)
	if !strings.Contains(lower, "gradient(") {
		return false
	}
	sizes, repeats := splitSelectors(strings.ToLower(size)), splitSelectors(strings.ToLower(repeat))
	for i, layer := range splitSelectors(lower) {
		radial := strings.HasPrefix(layer, "radial-gradient(")
		if !radial && !strings.HasPrefix(layer, "linear-gradient(") {
			continue
		}
		end := closingParen(layer)
		if end < 0 {
			continue
		}
		args, rest := splitSelectors(layer[strings.IndexByte(layer, '(')+1:end]), layer[end+1:]
		tile, axis := "", rest
		if _, after, ok := strings.Cut(rest, "/"); ok {
			tile = after
		} else if i < len(sizes) {
			tile = sizes[i]
		}
		if i < len(repeats) {
			axis += " " + repeats[i]
		}
		if !oneAxisRepeat(axis) || !smallTile(tile) {
			continue
		}
		if radial || len(args) > 2 && diagonal(args[0]) && hardStop(args[1:]) {
			return true
		}
	}
	return false
}

func closingParen(s string) int {
	depth := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func oneAxisRepeat(v string) bool {
	if strings.Contains(v, "repeat-x") || strings.Contains(v, "repeat-y") {
		return true
	}
	f := strings.Fields(v)
	for i := 1; i < len(f); i++ {
		if f[i-1] == "repeat" && f[i] == "no-repeat" || f[i-1] == "no-repeat" && f[i] == "repeat" {
			return true
		}
	}
	return false
}

// smallTile is true when both tile dimensions are absolute lengths of at most 32 px.
func smallTile(v string) bool {
	f := strings.Fields(v)
	if len(f) == 0 {
		return false
	}
	if len(f) > 2 {
		f = f[:2]
	}
	for _, x := range f {
		if px := lengthPx(x); px <= 0 || px > 32 {
			return false
		}
	}
	return true
}

var diagonalAngle = regexp.MustCompile(`^(-?[0-9]*\.?[0-9]+)deg$`)

func diagonal(v string) bool {
	v = strings.Join(strings.Fields(v), " ")
	if m := diagonalAngle.FindStringSubmatch(v); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		return math.Abs(math.Mod(math.Abs(a), 90)-45) < 1
	}
	switch v {
	case "to top left", "to top right", "to bottom left", "to bottom right", "to left top", "to right top", "to left bottom", "to right bottom":
		return true
	}
	return false
}

// hardStop is true when a colour stop does not advance past the one before it
// in the same unit.
func hardStop(stops []string) bool {
	previous, unit := 0.0, ""
	for _, stop := range stops {
		f := strings.Fields(stop)
		if len(f) < 2 {
			continue
		}
		v, u := f[len(f)-1], "length"
		position := lengthPx(v)
		if strings.HasSuffix(v, "%") {
			p, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
			if err != nil {
				continue
			}
			position, u = p, "%"
		} else if position == 0 && v != "0" {
			continue
		}
		if v == "0" && unit != "" {
			u = unit
		}
		if u == unit && position <= previous {
			return true
		}
		previous, unit = position, u
	}
	return false
}

func fullCover(m *styleModel, n *html.Node, grounds map[*html.Node]bool) bool {
	p := m.value(n, "position")
	if p != "fixed" && p != "absolute" {
		return false
	}
	if p == "absolute" {
		parent, base := n.Parent, m
		if m.origin != nil {
			parent, base = n, m.origin
		}
		for ; parent != nil && parent.Type == html.ElementNode; parent = parent.Parent {
			position := base.value(parent, "position")
			if position != "" && position != "static" {
				if !grounds[parent] {
					return false
				}
				break
			}
		}
	}
	if v := m.value(n, "inset"); v == "0" || v == "0px" {
		return true
	}
	return (m.value(n, "width") == "100%" || m.value(n, "width") == "100vw") && (m.value(n, "height") == "100%" || m.value(n, "height") == "100vh") || m.value(n, "top") == "0" && m.value(n, "right") == "0" && m.value(n, "bottom") == "0" && m.value(n, "left") == "0"
}

func groundTexture(image, size, repeat string, assets map[string][]byte) bool {
	lower := strings.ToLower(image)
	decoded, err := url.PathUnescape(lower)
	if err == nil {
		lower = decoded
	}
	if strings.Contains(lower, "feturbulence") {
		return true
	}
	if strings.Contains(lower, "repeating-linear-gradient(") || strings.Contains(lower, "linear-gradient(") && size != "" && size != "auto" && !strings.Contains(repeat+lower, "no-repeat") {
		// Thin repeated stops distinguish rules from broad colour bands.
		if thinGroundLines(lower) {
			return true
		}
	}
	for _, reference := range urlcheck.CSSReferences([]byte(image), 0) {
		ref := reference.Value
		if strings.HasPrefix(ref, "data:") {
			if payload, _, err := dataurl.Decode(ref); err == nil && strings.Contains(strings.ToLower(string(payload)), "feturbulence") {
				return true
			}
			continue
		}
		name := strings.ToLower(path.Base(strings.SplitN(ref, "?", 2)[0]))
		for _, word := range []string{"paper", "parchment", "grain", "noise", "notebook"} {
			if strings.Contains(name, word) {
				return true
			}
		}
		if strings.Contains(strings.ToLower(string(assets[strings.TrimPrefix(ref, "./")])), "feturbulence") {
			return true
		}
	}
	return false
}

// withoutGradientImages leaves the colour layer of a background shorthand.
// The CSS lexer owns strings, escapes and function boundaries.
func withoutGradientImages(value string) string {
	lexer := css.NewLexer(parse.NewInputString(value))
	var out bytes.Buffer
	skip := 0
	for {
		kind, raw := lexer.Next()
		if kind == css.ErrorToken {
			break
		}
		if kind == css.FunctionToken || kind == css.LeftParenthesisToken {
			if skip > 0 {
				skip++
			} else if strings.HasSuffix(strings.ToLower(string(raw)), "gradient(") {
				skip = 1
				continue
			}
		}
		if skip > 0 {
			if kind == css.RightParenthesisToken {
				skip--
			}
			continue
		}
		out.Write(raw)
	}
	return out.String()
}

var groundStop = regexp.MustCompile(`([0-9]*\.?[0-9]+)(px|rem|em)\b`)

func thinGroundLines(value string) bool {
	stops := groundStop.FindAllStringSubmatch(value, -1)
	for i, stop := range stops {
		v, _ := strconv.ParseFloat(stop[1], 64)
		scale := 1.0
		if stop[2] != "px" {
			scale = 16
		}
		if v > 0 && v*scale <= 2 {
			return true
		}
		if i > 0 && stops[i-1][2] == stop[2] {
			previous, _ := strconv.ParseFloat(stops[i-1][1], 64)
			if diff := math.Abs(v-previous) * scale; diff > 0 && diff <= 2 {
				return true
			}
		}
	}
	return false
}

func paintedGround(value string) bool {
	for _, c := range color.Parse(withoutGradientImages(value)) {
		if c.Alpha > 0 {
			return true
		}
	}
	return false
}
