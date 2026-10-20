package compile

import (
	"reflect"
	"strings"
	"testing"
)

// TestUnitSummariesSpeakOneLanguage holds the --json summaries of the
// deliverables to one shape: a count or a flag is always there, since 0 and
// false say something, and what every unit has goes by the same key.
func TestUnitSummariesSpeakOneLanguage(t *testing.T) {
	shared := map[string]string{ // Go field -> JSON key
		"Title": "name", "Name": "name", "Sell": "sell", "URL": "url", "Files": "files", "Listing": "listing",
		"Channels": "channels", "Images": "images", "Pending": "pending", "Layout": "layout", "Concept": "concept",
	}
	units := reflect.TypeFor[Result]()
	for _, unit := range []string{"Book", "Printable", "Spreadsheet", "Document", "Deck", "Bundle"} {
		f, ok := units.FieldByName(unit)
		if !ok {
			t.Fatalf("Result has no %s", unit)
		}
		summary := f.Type.Elem()
		for i := range summary.NumField() {
			field := summary.Field(i)
			key, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if key == "-" {
				continue
			}
			switch field.Type.Kind() {
			case reflect.Bool, reflect.Int:
				if strings.Contains(options, "omitempty") {
					t.Errorf("%s.%s (%s) is left out when 0 or false", unit, field.Name, key)
				}
			}
			if want, ok := shared[field.Name]; ok && key != want {
				t.Errorf("%s.%s is %q in JSON; every unit calls it %q", unit, field.Name, key, want)
			}
		}
	}
}
