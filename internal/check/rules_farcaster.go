package check

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/imaging"
)

func farcasterRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, key := range []string{"fc:miniapp", "fc:frame"} {
		for _, n := range platformMeta(d, key) {
			// Legacy Frames v1 use fc:frame=vNext, a different contract. Do not
			// mistake a supported older protocol for malformed Mini App JSON.
			if key == "fc:frame" && strings.TrimSpace(Attr(n, "content")) == "vNext" {
				continue
			}
			object, err := sharingJSON([]byte(Attr(n, "content")))
			if err != nil {
				issues = append(issues, d.At(n, Error, key+": "+err.Error()+".", "Supply one escaped MiniAppEmbed JSON object in the meta content attribute; no executable JS or templated JSON keys."))
				continue
			}
			if jsonString(object, "version") != "1" {
				issues = append(issues, d.At(n, Error, key+" version must be the string \"1\".", "Use the MiniAppEmbed version string, not a number or a legacy Frames response."))
			}
			image := jsonString(object, "imageUrl")
			if invalid := sharingURL(d, n, image, key+" imageUrl", true, false); len(invalid) > 0 {
				issues = append(issues, invalid...)
			} else {
				if utf8.RuneCountInString(image) > 1024 {
					issues = append(issues, d.At(n, Error, key+" imageUrl exceeds 1024 characters.", "Use a compact public image URL; keep product state in the launch URL rather than an oversized image address."))
				}
				issues = append(issues, sharingImage(d, n, image, "Farcaster embed", func(info imaging.Info) string {
					var problems []string
					if info.Format != "" && info.Format != "png" && info.Format != "jpeg" && info.Format != "gif" && info.Format != "webp" {
						problems = append(problems, "use PNG/JPEG/GIF/WebP; SVG preview support is not production compatibility")
					}
					if info.Bytes >= 10_000_000 {
						problems = append(problems, fmt.Sprintf("%d bytes exceeds the <10 MB image budget", info.Bytes))
					}
					if info.Width > 0 && info.Height > 0 && (info.Width < 600 || info.Height < 400 || info.Width > 3000 || info.Height > 2000 || int64(info.Width)*2 != int64(info.Height)*3) {
						problems = append(problems, fmt.Sprintf("%d×%d; require 3:2 within 600×400–3000×2000 px", info.Width, info.Height))
					}
					return strings.Join(problems, "; ")
				})...)
			}
			button, _ := object["button"].(map[string]any)
			if strings.TrimSpace(jsonString(button, "title")) == "" {
				issues = append(issues, d.At(n, Error, key+" button.title is missing or empty.", "Give the launch button a useful action label; keep unknown/new optional fields intact."))
			}
			action, _ := button["action"].(map[string]any)
			switch jsonString(action, "type") {
			case "launch_miniapp", "launch_frame":
			default:
				issues = append(issues, d.At(n, Error, key+" button.action.type must be launch_miniapp or launch_frame.", "Use the documented launch action; an ordinary link/transaction is a different protocol."))
			}
			for _, field := range []string{"url", "splashImageUrl"} {
				if _, exists := action[field]; exists {
					issues = append(issues, sharingURL(d, n, jsonString(action, field), key+" button.action."+field, true, field == "url")...)
				}
			}
			// An omitted action URL deliberately inherits the shared page/query.
			// Manifest ownership, ready(), host permissions and query restoration
			// cannot be certified by this static payload check.
		}
	}
	return issues
}
