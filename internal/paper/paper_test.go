package paper

import (
	"slices"
	"testing"
)

func TestForDefaultsByLanguage(t *testing.T) {
	if got := For("en", nil); !slices.Equal(got, []string{"letter", "a4"}) {
		t.Fatalf("en %v", got)
	}
	if got := For("de", nil); !slices.Equal(got, []string{"a4"}) {
		t.Fatalf("de %v", got)
	}
	if got := For("en", []string{"a4"}); !slices.Equal(got, []string{"a4"}) {
		t.Fatalf("configured %v", got)
	}
}

func TestLabel(t *testing.T) {
	if Label("letter") != "US Letter" || Label("a4") != "A4" || Label("a5") != "A5" {
		t.Fatal(Label("letter"), Label("a4"), Label("a5"))
	}
}
