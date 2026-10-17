package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
)

const imageByteBudget = 512 * 1024
const imagePixelBudget = 12_000_000
const imageEdgeBudget = 4096

func imageAssetsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	nodes := d.selectNodes("img")
	seen := map[string]bool{}
	for index, decision := range d.Options.Images {
		if index >= len(nodes) {
			break
		}
		infos := append([]imaging.Info{decision.Info}, decision.Candidates...)
		for candidate, info := range infos {
			var problems []string
			if info.Issue != "" {
				problems = append(problems, info.Issue)
			}
			if info.Bytes > imageByteBudget {
				problems = append(problems, fmt.Sprintf("asset is %d bytes (advisory budget %d)", info.Bytes, imageByteBudget))
			}
			if info.Raster && (info.Width > imageEdgeBudget || info.Height > imageEdgeBudget || int64(info.Width)*int64(info.Height) > imagePixelBudget) {
				problems = append(problems, fmt.Sprintf("raster is %d×%d pixels (advisory edge %d px; area %d pixels); decoded RGBA can require about %.1f MiB", info.Width, info.Height, imageEdgeBudget, imagePixelBudget, float64(info.Width)*float64(info.Height)*4/(1024*1024)))
			}
			if candidate == 0 && decision.CompareSize && decision.AuthoredWidth > 0 && decision.AuthoredHeight > 0 && info.Bytes > 4096 && info.Raster && int64(info.Width) > 2*int64(decision.AuthoredWidth) && int64(info.Height) > 2*int64(decision.AuthoredHeight) {
				problems = append(problems, fmt.Sprintf("source %d×%d exceeds 2× the authored %d×%d box on both axes", info.Width, info.Height, decision.AuthoredWidth, decision.AuthoredHeight))
			}
			if len(problems) == 0 {
				continue
			}
			message := info.Source + ": " + strings.Join(problems, "; ") + "."
			if seen[message] {
				continue
			}
			seen[message] = true
			issue := d.At(nodes[index], Warning, message, "Resize/re-export raster sources for the largest real display size and intended pixel density; use responsive candidates when appropriate. This static budget does not measure the CSS viewport or change image bytes, cropping, animation or authored sizing.")
			issue.Path = info.Source
			issues = append(issues, issue)
		}
	}
	return issues
}
