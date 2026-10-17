package stock

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/color"
)

func TestStockColorsAreRecognizedAndNeighboursAreNot(t *testing.T) {
	for _, value := range []string{"#6366f1", "#7c3aed", "oklch(58.5% 0.233 277.117)"} {
		c := color.Parse(value)[0]
		if _, ok := Nearest(c, Accents, 0.02); !ok {
			t.Errorf("%s not recognized as a stock accent", value)
		}
		if !Purpleish(c) && value != "#6366f1" && value != "oklch(58.5% 0.233 277.117)" {
			t.Errorf("%s should be purpleish", value)
		}
	}
	for _, value := range []string{"#2f6f4f", "#c2410c", "#1d4ed8", "#000000"} {
		c := color.Parse(value)[0]
		if n, ok := Nearest(c, Accents, 0.02); ok {
			t.Errorf("%s matched %s", value, n.Name)
		}
	}
	if len(WindowControls) != 3 {
		t.Fatal("window controls are red, yellow, green")
	}
}

func TestFamilies(t *testing.T) {
	if !GenericSans(`"Inter"`) || !GenericSans(" system-ui ") || GenericSans("Literata") {
		t.Fatal("generic sans")
	}
	if !Display("'Space Grotesk'") || Display("Mona Sans") {
		t.Fatal("display roster")
	}
	if !Mono("JetBrains Mono") || Mono("Inter") {
		t.Fatal("mono")
	}
}
