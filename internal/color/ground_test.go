package color

import "testing"

func TestWarmLightGround(t *testing.T) {
	for _, tc := range []struct {
		hex  string
		want bool
	}{
		{"#F4EFE6", true}, {"#F3E9DC", true}, {"#FAF0E6", true}, {"#F5E6D3", true}, {"#FBEEE6", true}, {"#F9ECE6", true},
		{"#f5f0e6", true}, {"#faf7f2", true}, {"#faf9f5", true}, {"#f4f1ea", true}, {"#fffbeb", true}, {"#ffedd5", true},
		{"#ffffff", false}, {"#fafafa", false}, {"#f5f5f5", false}, {"#f8f9fa", false}, {"#f3f4f6", false}, {"#f1f3f0", false}, {"#f9f9f7", false}, {"#fef3c7", false}, {"#121212", false}, {"#f5f0e600", false}, {"#f5f0e680", true},
	} {
		t.Run(tc.hex, func(t *testing.T) {
			c, ok := ParseHex(tc.hex)
			t.Logf("OKLCH %.4f %.4f %.2f°", c.L, c.Chroma(), c.Hue())
			if !ok || WarmLightGround(c) != tc.want {
				t.Fatalf("%s: %v (%f %f %f)", tc.hex, WarmLightGround(c), c.L, c.Chroma(), c.Hue())
			}
		})
	}
}
