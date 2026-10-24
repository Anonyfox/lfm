package check

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Check portable preview metadata without requiring identical wording across
// listings or treating OG image arrays/structured properties as duplicates.
func socialRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	fields := map[string][]*html.Node{}
	for _, n := range d.activeNodes("meta") {
		if n.Namespace != "" {
			continue
		}
		key, attr := strings.ToLower(strings.TrimSpace(Attr(n, "property"))), "property"
		if !strings.HasPrefix(key, "og:") && !strings.HasPrefix(key, "twitter:") {
			key, attr = strings.ToLower(strings.TrimSpace(Attr(n, "name"))), "name"
		}
		if !strings.HasPrefix(key, "og:") && !strings.HasPrefix(key, "twitter:") {
			continue
		}
		fields[key] = append(fields[key], n)
		if strings.HasPrefix(key, "og:") && attr != "property" || strings.HasPrefix(key, "twitter:") && attr != "name" {
			issues = append(issues, d.At(n, Warning, key+" uses the wrong metadata attribute.", "Use property for Open Graph and name for Twitter/X card metadata."))
		}
		if !inHead(n) {
			issues = append(issues, d.At(n, Warning, key+" is outside head.", "Move social metadata into head; preview consumers may stop scanning before body content."))
		}
	}
	var missing []string
	for _, key := range []string{"og:title", "og:type", "og:url", "og:image"} {
		nodes := fields[key]
		if key == "og:image" {
			nodes = append(append([]*html.Node{}, nodes...), fields["og:image:url"]...)
		}
		present := false
		for _, n := range nodes {
			present = present || strings.TrimSpace(Attr(n, "content")) != ""
		}
		if !present {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		issues = append(issues, d.Whole(Warning, "Social preview is missing nonempty "+strings.Join(missing, ", ")+".", "Render the Open Graph core fields in head from config; use the canonical URL and a public image file URL."))
	}
	// Scalar fields should have one deliberate value. Do not apply this policy to
	// image/video/locale arrays or to properties scoped to each media root.
	for _, key := range []string{"og:title", "og:type", "og:url", "og:description", "twitter:card", "twitter:title", "twitter:description", "twitter:image", "twitter:image:src", "twitter:url"} {
		for i, n := range fields[key] {
			if i == 1 {
				issue := d.At(n, Warning, "Repeated scalar social field "+key+".", "Keep one deliberate value; consumers may choose different duplicates. Multiple Open Graph images remain supported.")
				issue.Related = []Location{d.At(fields[key][0], Warning, "", "").Location}
				issues = append(issues, issue)
			}
			if strings.TrimSpace(Attr(n, "content")) == "" && key != "og:title" && key != "og:type" && key != "og:url" {
				issues = append(issues, d.At(n, Warning, key+" is empty.", "Fill this field from config or remove an optional field to permit consumer fallback."))
			}
		}
	}
	canonical := d.Options.URL
	for _, n := range d.activeNodes("link[rel]") {
		if containsWord(Attr(n, "rel"), "canonical") && inHead(n) && publicURL(Attr(n, "href")) {
			canonical = Attr(n, "href")
			break
		}
	}
	_, analyzer := d.linkFacts()
	for _, key := range []string{"og:url", "twitter:url", "og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src"} {
		for _, n := range fields[key] {
			value, local := d.previewURL(Attr(n, "content"))
			if !publicPreviewURL(value, local) {
				issues = append(issues, d.metadataURLAt(n, value, Error, key+" must be an absolute HTTP(S) URL without credentials, fragment or a local/private host.", "Use a named public URL from lfm.json; social images must be deployed files, not data URLs. Verify live reachability after publication."))
			} else if !local && (key == "og:url" || key == "twitter:url") && canonical != "" {
				identity, _, _ := strings.Cut(value, "?")
				if analyzer.Key(identity) != analyzer.Key(canonical) {
					issues = append(issues, d.metadataURLAt(n, value, Warning, fmt.Sprintf("%s differs from page canonical %q.", key, short(canonical)), "Keep preview identity tied to this page; deliberate public-state query variants and different share wording/images are allowed."))
				}
			}
		}
	}
	return issues
}
