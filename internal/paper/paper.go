// Package paper holds the paper sizes editable files print on, and which a
// language prints on by default.
package paper

import "strings"

// Size is a page size in millimetres.
type Size struct {
	Key, Label    string
	Width, Height float64
}

// Office are the sizes documents and speaker notes print on.
var Office = map[string]Size{
	"a4":     {"a4", "A4", 210, 297},
	"letter": {"letter", "US Letter", 215.9, 279.4},
}

// For are the papers a language prints on: the configured ones, or Letter
// and A4 for American English and A4 elsewhere.
func For(lang string, configured []string) []string {
	if len(configured) > 0 {
		return configured
	}
	switch lang {
	case "en", "en-us", "en-ca", "es-us":
		return []string{"letter", "a4"}
	}
	return []string{"a4"}
}

// Label names a paper as buyers know it: US Letter, A4.
func Label(key string) string {
	if s, ok := Office[key]; ok {
		return s.Label
	}
	return strings.ToUpper(key)
}
