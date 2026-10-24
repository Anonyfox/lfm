package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
	"golang.org/x/text/language"
)

func metaNodes(d *Document, key, value string) []*html.Node {
	var nodes []*html.Node
	for _, n := range d.Elements {
		if n.Namespace == "" && n.Data == "meta" && !d.inTemplate(n) && strings.EqualFold(strings.TrimSpace(Attr(n, key)), value) {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func metadataRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	var titles []*html.Node
	for _, n := range d.activeNodes("title") {
		if n.Namespace == "" {
			titles = append(titles, n)
		}
	}
	if len(titles) == 0 || (len(titles) == 1 && strings.TrimSpace(Text(titles[0])) == "") {
		issues = append(issues, d.Whole(Warning, "Page title is missing or empty.", "Render the configured title in one <title> inside head."))
	}
	if len(titles) > 1 {
		issues = append(issues, d.At(titles[1], Error, "Multiple page titles.", "Keep exactly one title in head."))
	}
	for _, n := range titles {
		if n.Namespace == "" && !inHead(n) {
			issues = append(issues, d.At(n, Error, "Page title is outside head.", "Move the document title into head; SVG titles belong inside their SVG."))
		}
	}
	descriptions := metaNodes(d, "name", "description")
	if len(descriptions) == 0 || strings.TrimSpace(Attr(descriptions[0], "content")) == "" {
		issues = append(issues, d.Whole(Warning, "Meta description is missing or empty.", "Render the configured description in <meta name=\"description\" content=\"...\">."))
	}
	if len(descriptions) > 1 {
		issues = append(issues, d.At(descriptions[1], Warning, "Multiple meta descriptions.", "Keep one deliberate description."))
	}
	for _, n := range descriptions {
		if !inHead(n) {
			issues = append(issues, d.At(n, Warning, "Meta description is outside head.", "Move the description into head so metadata consumers can discover it reliably."))
		}
	}
	var canonicals []*html.Node
	for _, n := range d.activeNodes("link[rel]") {
		if n.Namespace == "" && containsWord(Attr(n, "rel"), "canonical") {
			canonicals = append(canonicals, n)
		}
	}
	if len(canonicals) == 0 {
		issues = append(issues, d.Whole(Error, "Canonical URL is missing.", "Render exactly one <link rel=\"canonical\" href=\"{{.url}}\"> in head; shared query state never changes lfm.json url."))
	}
	if len(canonicals) > 1 {
		issues = append(issues, d.At(canonicals[1], Error, "Multiple canonical URLs.", "Keep one canonical URL in head."))
	}
	for _, n := range canonicals {
		value, local := d.previewURL(Attr(n, "href"))
		if !publicPreviewURL(value, local) || (!local && !strings.HasPrefix(value, "https://")) || strings.ContainsAny(value, "?#") {
			issues = append(issues, d.metadataURLAt(n, value, Error, "Canonical URL must be a public absolute HTTPS URL without query, fragment or credentials.", "Render lfm.json url exactly, including its path/trailing slash; state/referral parameters belong only to shared links."))
		} else if !local && d.Options.URL != "" && value != d.Options.URL {
			issues = append(issues, d.metadataURLAt(n, value, Error, fmt.Sprintf("Canonical URL differs from configured target %q.", short(d.Options.URL)), "Use href=\"{{.url}}\" exactly; each static product/result/embed document owns its configured base canonical."))
		}
		if n.Parent == nil || n.Parent.Data != "head" {
			issues = append(issues, d.At(n, Error, "Canonical link is outside head.", "Put the canonical link inside head."))
		}
	}
	for _, n := range d.selectNodes("html") {
		value := Attr(n, "lang")
		if strings.TrimSpace(value) == "" {
			issues = append(issues, d.At(n, Warning, "Document language is missing.", "Set html lang from the configured language for browsers and assistive technology."))
		} else if _, err := language.Parse(value); err != nil || strings.Contains(value, "_") || value != strings.TrimSpace(value) {
			issues = append(issues, d.At(n, Warning, "Document language is not a recognized BCP 47 tag.", "Use a language tag such as en, de-DE or zh-Hant; underscores and language names are not HTML language tags."))
		}
	}
	return issues
}

func publicURL(value string) bool {
	return urlcheck.PublicURL(value)
}

func inHead(n *html.Node) bool {
	return n.Parent != nil && n.Parent.Namespace == "" && n.Parent.Data == "head"
}

func (d *Document) metadataURLAt(n *html.Node, value string, severity Severity, message, hint string) Diagnostic {
	issue := d.At(n, severity, message, hint)
	if d.Options.Registry != nil {
		if names := d.Options.Registry.Match(value); len(names) > 0 {
			issue.Path = names[0].Path
			issue.Related = []Location{names[0].Location}
		}
	}
	return issue
}

func containsWord(value, word string) bool {
	for _, part := range strings.Fields(strings.ToLower(value)) {
		if part == word {
			return true
		}
	}
	return false
}

// RobotsDirectives reads the indexing and link-following opt-outs on a live
// crawler meta tag. Both output generation and SEO checks use this policy.
// Unknown crawlers and directives, foreign markup and template content are inert.
func RobotsDirectives(n *html.Node) (noIndex, noFollow bool) {
	if n.Namespace != "" || n.Data != "meta" || htmlx.Dormant(n) {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(Attr(n, "name"))) {
	case "robots", "googlebot", "googlebot-news", "bingbot":
		for _, token := range strings.Fields(strings.ReplaceAll(strings.ToLower(Attr(n, "content")), ",", " ")) {
			noIndex = noIndex || token == "noindex" || token == "none"
			noFollow = noFollow || token == "nofollow" || token == "none"
		}
	}
	return noIndex, noFollow
}

func indexabilityRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, key := range []string{"robots", "googlebot", "googlebot-news", "bingbot"} {
		for _, n := range robotsNodes(d, key) {
			noIndex, noFollow := RobotsDirectives(n)
			if noIndex {
				issues = append(issues, d.At(n, Warning, robotsMessage(n, "Page explicitly requests no indexing."), "Remove noindex/none for public searchable products; keep it for intentional private or test pages."))
			}
			if noFollow {
				issues = append(issues, d.At(n, Warning, robotsMessage(n, "Page requests that crawlers not follow its links."), "Remove page-wide nofollow/none when linked products and supporting pages should be discoverable; retain deliberate restrictions."))
			}
		}
	}
	return issues
}

func headingsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	var ids map[string]*html.Node
	mainHeading, previous := false, 0
	for _, n := range d.activeNodes("h1, h2, h3, h4, h5, h6") {
		if n.Namespace != "" || discoveryHidden(n) {
			continue
		}
		level := int(n.Data[1] - '0')
		if level == 1 {
			mainHeading = true
		}
		if level > previous+1 {
			message := fmt.Sprintf("Heading hierarchy jumps from h%d to %s.", previous, n.Data)
			if previous == 0 {
				message = "Heading hierarchy starts at " + n.Data + "."
			}
			issues = append(issues, d.At(n, Warning, message, "Use levels for the content hierarchy and CSS for size; start with h1, then avoid skipped subsection levels."))
		}
		previous = level
		if strings.TrimSpace(visibleText(n)) == "" {
			if ids == nil {
				ids = map[string]*html.Node{}
				for _, node := range d.activeNodes("[id]") {
					ids[Attr(node, "id")] = node
				}
			}
			if strings.TrimSpace(accessibleName(n, ids, map[*html.Node]bool{})) == "" {
				issues = append(issues, d.At(n, Warning, "Heading has no readable content.", "Add meaningful heading text or an accessible image/ARIA name."))
			}
		}
	}
	if !mainHeading {
		issues = append(issues, d.Whole(Warning, "No visible main h1 heading.", "Give the page a readable main heading in initial HTML; hidden or template headings do not supply one."))
	}
	return issues
}

// robotsNodes applies header crawler groups with the same crawler policy as meta tags.
func robotsNodes(d *Document, key string) []*html.Node {
	nodes := metaNodes(d, "name", key)
	for _, line := range d.Options.RobotsHeader {
		crawler := "robots"
		var directives []string
		flush := func() {
			if crawler == key && len(directives) > 0 {
				nodes = append(nodes, &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "name", Val: key}, {Key: "content", Val: strings.Join(directives, ",")}, {Key: "data-header", Val: "X-Robots-Tag"}}})
			}
			directives = nil
		}
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			name, value, colon := strings.Cut(part, ":")
			name = strings.ToLower(strings.TrimSpace(name))
			// Valued directives are not crawler prefixes.
			if colon && name != "max-snippet" && name != "max-image-preview" && name != "max-video-preview" && name != "unavailable_after" {
				flush()
				crawler, part = name, strings.TrimSpace(value)
			}
			if part != "" {
				directives = append(directives, part)
			}
		}
		flush()
	}
	return nodes
}
func robotsMessage(n *html.Node, message string) string {
	if Attr(n, "data-header") != "" {
		return "X-Robots-Tag: " + message
	}
	return message
}
