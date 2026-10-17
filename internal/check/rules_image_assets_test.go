package check

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/imaging"
)

func TestImageAssetBudgetsAndDensitySafety(t *testing.T) {
	base := imaging.Decision{Info: imaging.Info{Source: "assets/photo.jpg", Format: "jpeg", Raster: true, Width: 1200, Height: 800, Bytes: 40_000}, AuthoredWidth: 200, AuthoredHeight: 100, CompareSize: true}
	for _, test := range []struct {
		name   string
		change func(*imaging.Decision)
		want   string
	}{
		{"relative", func(*imaging.Decision) {}, "exceeds 2×"},
		{"responsive-density", func(d *imaging.Decision) { d.CompareSize = false }, ""},
		{"CSS-slot", func(d *imaging.Decision) { d.CompareSize = false }, ""},
		{"threshold", func(d *imaging.Decision) { d.Width = 400; d.Height = 200 }, ""},
		{"tiny-encoded", func(d *imaging.Decision) { d.Bytes = 100 }, ""},
		{"large-edge", func(d *imaging.Decision) { d.CompareSize = false; d.Width = 5000 }, "5000×800"},
		{"large-area", func(d *imaging.Decision) { d.CompareSize = false; d.Width = 4000; d.Height = 4000 }, "RGBA"},
		{"bytes", func(d *imaging.Decision) { d.CompareSize = false; d.Bytes = 600_000 }, "600000 bytes"},
		{"vector-no-pixel-warning", func(d *imaging.Decision) { d.Raster = false; d.Width = 100_000; d.Height = 100_000 }, ""},
		{"bad-metadata", func(d *imaging.Decision) { d.CompareSize = false; d.Issue = "bad orientation" }, "bad orientation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision := base
			test.change(&decision)
			doc, err := Parse([]byte(`<img src="./photo.jpg" width="200" height="100" alt="">`), Options{Images: []imaging.Decision{decision}})
			if err != nil {
				t.Fatal(err)
			}
			issues := imageAssetsRule(doc)
			if test.want == "" {
				if len(issues) != 0 {
					t.Fatal(issues)
				}
				return
			}
			if len(issues) != 1 || !strings.Contains(issues[0].Message, test.want) || issues[0].Path != "assets/photo.jpg" || issues[0].Location.File != "index.html" {
				t.Fatal(issues)
			}
		})
	}
}

func TestImageAssetRuleCanBePromotedOrDisabled(t *testing.T) {
	info := imaging.Decision{Info: imaging.Info{Source: "photo.png", Raster: true, Width: 6000, Height: 4000}}
	for _, level := range []Severity{Off, Warning, Error} {
		doc, err := Parse([]byte(`<img src="./photo.png" width="200" height="100" alt="">`), Options{Images: []imaging.Decision{info}})
		if err != nil {
			t.Fatal(err)
		}
		rules := []Rule{testRule("perf.image-assets", imageAssetsRule)}
		report := Run(doc, rules, map[string]Severity{"perf.image-assets": level})
		if level == Off && len(report.Diagnostics) != 0 || level == Warning && report.Warnings != 1 || level == Error && report.Errors != 1 {
			t.Fatal(report)
		}
	}
}
