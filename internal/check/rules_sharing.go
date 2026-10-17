package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
)

func sharingDiscoveryRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	late := false
	// Scalar platform fields; OG media arrays remain owned by seo.social.
	for _, key := range []string{"twitter:player", "twitter:player:width", "twitter:player:height", "twitter:player:stream", "twitter:player:stream:content_type", "snapchat:sticker", "snapchat:app_id", "fediverse:creator", "fc:miniapp", "fc:frame", "twitter:label1", "twitter:data1", "twitter:label2", "twitter:data2"} {
		nodes := platformMeta(d, key)
		if len(nodes) > 1 {
			issue := d.At(nodes[1], Error, "Repeated scalar sharing field "+key+".", "Keep one deliberate value; the selected platform can otherwise choose conflicting declarations.")
			issue.Related = []Location{d.At(nodes[0], Error, "", "").Location}
			issues = append(issues, issue)
		}
		for _, n := range nodes {
			if !inHead(n) && !strings.HasPrefix(key, "twitter:") {
				issues = append(issues, d.At(n, Warning, key+" is outside head.", "Put the platform declaration in initial head HTML; crawler previews do not execute page JS."))
			}
			attribute := "name"
			if strings.HasPrefix(key, "snapchat:") {
				attribute = "property"
			}
			if !strings.EqualFold(strings.TrimSpace(Attr(n, attribute)), key) && !strings.HasPrefix(key, "twitter:") {
				issues = append(issues, d.At(n, Warning, key+" uses the wrong metadata attribute.", "Use "+attribute+" for this platform field."))
			}
			if strings.TrimSpace(Attr(n, "content")) == "" {
				issues = append(issues, d.At(n, Error, key+" is empty.", "Supply its configured value or remove the unused optional declaration."))
			}
		}
	}
	for _, n := range d.Elements {
		if n.Namespace != "" || inTemplate(n) {
			continue
		}
		if n.Data == "meta" {
			key := strings.ToLower(strings.TrimSpace(Attr(n, "property")))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(Attr(n, "name")))
			}
			if !late && (strings.HasPrefix(key, "og:") || strings.HasPrefix(key, "twitter:") || strings.HasPrefix(key, "fc:") || strings.HasPrefix(key, "snapchat:")) && d.positions[n] >= 32*1024 {
				issues = append(issues, d.At(n, Warning, "Sharing metadata begins/continues beyond the first 32 KiB of HTML.", "Move crawler tags/payloads ahead of bulky inline CSS/assets. Slack has been observed fetching a bounded prefix; 32 KiB is an advisory, not a universal crawler limit."))
				late = true
			}
			if strings.EqualFold(Attr(n, "http-equiv"), "Content-Security-Policy") {
				for _, directive := range strings.Split(strings.ToLower(Attr(n, "content")), ";") {
					if fields := strings.Fields(directive); len(fields) > 0 && fields[0] == "frame-ancestors" {
						issues = append(issues, d.At(n, Warning, "CSP frame-ancestors in a meta tag cannot enable framing.", "Set frame-ancestors in the player route's HTTP response header and resolve conflicting X-Frame-Options; verify actual deployed headers/ancestor chain."))
						break
					}
				}
			}
		}
		if n.Data == "link" && containsWord(Attr(n, "rel"), "alternate") && (Attr(n, "type") == "application/json+oembed" || Attr(n, "type") == "text/xml+oembed") {
			issues = append(issues, sharingURL(d, n, Attr(n, "href"), "oEmbed discovery", false, false)...)
			if !inHead(n) {
				issues = append(issues, d.At(n, Warning, "oEmbed discovery link is outside head.", "Place rel=alternate with its oEmbed MIME type in initial head; consumer allowlists/acceptance need deployment review."))
			}
		}
	}
	for _, key := range []string{"og:video", "og:video:url", "og:video:secure_url", "og:audio", "og:audio:url", "og:audio:secure_url", "twitter:player:stream"} {
		for _, n := range platformMeta(d, key) {
			issues = append(issues, sharingURL(d, n, Attr(n, "content"), key, strings.HasSuffix(key, ":secure_url") || key == "twitter:player:stream", false)...)
		}
	}
	for _, key := range []string{"og:image:width", "og:image:height", "og:video:width", "og:video:height"} {
		for _, n := range platformMeta(d, key) {
			if imaging.PositiveAttribute(Attr(n, "content")) == 0 {
				issues = append(issues, d.At(n, Error, key+" must be a positive integer pixel dimension.", "Use the actual media dimension as digits, without px, decimals or percentages; omit an unknown optional hint."))
			}
		}
	}
	for _, pair := range [][2]string{{"twitter:label1", "twitter:data1"}, {"twitter:label2", "twitter:data2"}} {
		if (metaValue(d, pair[0]) != "") != (metaValue(d, pair[1]) != "") {
			issues = append(issues, d.Whole(Warning, "Slack preview pair "+pair[0]+" / "+pair[1]+" is incomplete.", "Supply both a useful label and value, or remove the pair; this legacy Slack enhancement is not an X Product Card."))
		}
	}
	for _, n := range platformMeta(d, "twitter:card") {
		value := Attr(n, "content")
		if value != "" && value != "summary" && value != "summary_large_image" && value != "player" && value != "app" {
			issues = append(issues, d.At(n, Warning, "Unrecognized or legacy X card type "+short(value)+".", "Use a current summary, summary_large_image, player or app card; Slack label/data pairs do not require the obsolete product card. Verify any newer platform type before retaining it."))
		}
	}
	return issues
}

func xPlayerRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	card := strings.TrimSpace(metaValue(d, "twitter:card"))
	if card != "player" {
		if nodes := platformMeta(d, "twitter:player"); len(nodes) > 0 {
			issues = append(issues, d.At(nodes[0], Warning, "twitter:player is declared without twitter:card=player.", "Select the player card explicitly when intended; verify actual X clients and retain a poster/direct-link fallback."))
		}
		return issues
	}
	carrier := platformMeta(d, "twitter:card")[0]
	for _, key := range []string{"twitter:player", "twitter:player:width", "twitter:player:height", "twitter:image"} {
		nodes := platformMeta(d, key)
		if len(nodes) == 0 || strings.TrimSpace(Attr(nodes[0], "content")) == "" {
			issues = append(issues, d.At(carrier, Error, "X player card is missing nonempty "+key+".", "Declare a public HTTPS player, positive pixel width/height and public twitter:image poster in head. Set framing headers on the player route; query state must be in the declared player target."))
			continue
		}
		n := nodes[0]
		value := Attr(n, "content")
		if key == "twitter:player" || key == "twitter:image" {
			issues = append(issues, sharingURL(d, n, value, key, true, key == "twitter:player")...)
		} else if imaging.PositiveAttribute(value) == 0 {
			issues = append(issues, d.At(n, Error, key+" must be a positive integer pixel dimension.", "Use digits without px/decimals; declared aspect ratio does not establish the physical mobile viewport."))
		}
	}
	for _, field := range []string{"title", "description"} {
		if strings.TrimSpace(metaValue(d, "twitter:"+field)) == "" && strings.TrimSpace(metaValue(d, "og:"+field)) == "" {
			issues = append(issues, d.At(carrier, Warning, "X player has no "+field+" fallback text.", "Provide useful twitter:"+field+" or its Open Graph fallback for clients that show a poster/link instead of the player."))
		}
	}
	return issues
}

func snapchatRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range platformMeta(d, "snapchat:sticker") {
		value := Attr(n, "content")
		if invalid := sharingURL(d, n, value, "snapchat:sticker", false, false); len(invalid) > 0 {
			issues = append(issues, invalid...)
			continue
		}
		issues = append(issues, sharingImage(d, n, value, "Snapchat sticker", func(info imaging.Info) string {
			var problems []string
			if info.Format != "" && info.Format != "png" && info.Format != "webp" {
				problems = append(problems, "use PNG for static / WebP for animated stickers; GIF is unsupported")
			}
			if info.Bytes >= 1_000_000 {
				problems = append(problems, fmt.Sprintf("%d bytes exceeds the <1 MB sticker budget", info.Bytes))
			}
			if info.Width > 0 && info.Height > 0 && (info.Width != 400 || info.Height != 400) {
				problems = append(problems, fmt.Sprintf("%d×%d; sticker must be 400×400 px", info.Width, info.Height))
			}
			return strings.Join(problems, "; ")
		})...)
	}
	return issues
}
