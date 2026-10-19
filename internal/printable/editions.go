package printable

import (
	"fmt"
	"sort"
	"strings"
)

// Size is a page size in millimetres.
type Size struct {
	Key, Label    string
	Width, Height float64
	Digital       bool // for note apps on tablets and e-ink, not for paper
}

// Sizes are the page sizes lfm renders. tablet is 3:4, the shape of every
// e-ink tablet and the 13-inch iPad.
var Sizes = map[string]Size{
	"letter":      {Key: "letter", Label: "US Letter", Width: 215.9, Height: 279.4},
	"a4":          {Key: "a4", Label: "A4", Width: 210, Height: 297},
	"a5":          {Key: "a5", Label: "A5", Width: 148, Height: 210},
	"half-letter": {Key: "half-letter", Label: "Half Letter", Width: 139.7, Height: 215.9},
	"tablet":      {Key: "tablet", Label: "Tablet 3:4", Width: 157.5, Height: 210, Digital: true},
}

// SizeNames lists the size keys for messages.
func SizeNames() string {
	keys := make([]string, 0, len(Sizes))
	for k := range Sizes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// Safe is the margin printers may cut or skip, in millimetres: nothing that
// matters may reach into it.
const Safe = 6.35

// Edition is one rendering of the product: a size and one value per axis.
type Edition struct {
	Key     string // a4-color-monday
	Label   string // A4 · color · monday
	Size    Size
	Options map[string]string // axis name, lowercased -> value
}

// Is reports whether any option of the edition has value v, for templates:
// {{if .Edition.Is "ink-saver"}}.
func (e Edition) Is(v string) bool {
	for _, o := range e.Options {
		if o == strings.ToLower(v) {
			return true
		}
	}
	return false
}

// Option returns the value of an axis: {{.Edition.Option "Start"}}.
func (e Edition) Option(axis string) string { return e.Options[strings.ToLower(axis)] }

// Editions multiplies the sizes by every axis.
func (p Plan) Editions() []Edition {
	var out []Edition
	for _, key := range p.Sizes {
		size := Sizes[key]
		combos := []Edition{{Key: key, Label: size.Label, Size: size, Options: map[string]string{}}}
		for _, axis := range p.Axes {
			var next []Edition
			for _, c := range combos {
				for _, v := range axis.Values {
					opts := map[string]string{}
					for k, o := range c.Options {
						opts[k] = o
					}
					opts[strings.ToLower(axis.Name)] = v
					next = append(next, Edition{Key: c.Key + "-" + v, Label: c.Label + " · " + v, Size: size, Options: opts})
				}
			}
			combos = next
		}
		out = append(out, combos...)
	}
	return out
}

func mm(v float64) string { return fmt.Sprintf("%gmm", v) }
