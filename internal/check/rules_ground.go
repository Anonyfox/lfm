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
			if designVisible(main) && paintedGround(m.value(main, "background-color")) {
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
	inspect := func(n *html.Node, model *styleModel, selector string, ground bool) {
		if n.Namespace != "" || !designVisible(n) {
			return
		}
		if ground {
			v := model.value(n, "background-color")
			// Gradient stops are images, not the background colour of a shorthand.
			for _, c := range color.Parse(withoutGradientImages(v)) {
				if color.WarmLightGround(c) {
					c = c.Over(color.FromSRGB(1, 1, 1, 1))
					out = append(out, d.At(n, Error, fmt.Sprintf("%s has prohibited warm-light ground %s (OKLCH %.3f %.4f %.1f°).", selector, c.Hex(), c.L, c.Chroma(), c.Hue()), groundHint))
				}
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
