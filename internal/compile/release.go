package compile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/release"
	"golang.org/x/net/html"
)

type optionalReleaseError struct{ error }

func (b *builder) optimizeRelease(doc *html.Node) (*release.Result, error) {
	report := &release.Result{}
	if _, err := release.BrowserPath(); err != nil {
		report.Reason = "Critical CSS skipped: " + err.Error()
		return report, optionalReleaseError{err}
	}
	var sheets []*html.Node
	var blocked bool
	var scan func(*html.Node)
	scan = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if node.Data == "template" {
				return
			}
			if node.Data == "meta" && strings.EqualFold(htmlx.Attr(node, "http-equiv"), "content-security-policy") {
				blocked = true
			}
			if node.Data == "style" || (node.Data == "link" && stylesheetRel(htmlx.Attr(node, "rel"))) {
				sheets = append(sheets, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			scan(child)
		}
	}
	scan(doc)
	if blocked || len(b.external) > 0 || len(sheets) != 1 {
		report.Reason = "Critical splitting requires one ordinary local stylesheet, no external resources and no authored CSP; kept full CSS."
		return report, nil
	}
	node := sheets[0]
	if independentlyAddressable(node, "rel", "href", "type") || strings.Contains(htmlx.Attr(node, "rel"), "alternate") {
		report.Reason = "Stylesheet has independent loading/identity/security semantics; kept full CSS."
		return report, nil
	}
	full := []byte(htmlx.RawText(node))
	name := ""
	if node.Data == "link" {
		ref, err := url.Parse(htmlx.Attr(node, "href"))
		if err != nil {
			return report, err
		}
		name = strings.TrimPrefix(ref.Path, "./")
		file, ok := b.files[strings.ToLower(name)]
		if !ok {
			report.Reason = "Stylesheet is not a generated local asset; kept full CSS."
			return report, nil
		}
		full = file.contents
	}
	report.OriginalBytes = len(full)
	if len(full) < 8192 {
		report.Reason = "Stylesheet is below 8 KiB; splitting overhead is not justified."
		return report, nil
	}
	htmlx.SetAttr(node, "data-lfm-critical-source", "")
	defer htmlx.DelAttr(node, "data-lfm-critical-source")
	var preview bytes.Buffer
	if err := html.Render(&preview, doc); err != nil {
		return report, err
	}
	assets := map[string][]byte{}
	for _, file := range b.files {
		assets[file.name] = file.contents
	}
	observed, err := release.Analyze(preview.Bytes(), assets)
	if err != nil {
		return report, optionalReleaseError{fmt.Errorf("release CSS analysis: %w", err)}
	}
	observed.OriginalBytes = len(full)
	if observed.Reason != "" {
		return &observed, nil
	}
	critical := []byte(observed.CSS)
	observed.CriticalBytes = len(critical)
	if len(full)-len(critical) < 2048 || len(critical)*5 > len(full)*4 {
		observed.Reason = "Critical CSS does not remove at least 2 KiB and 20% from the blocking head; kept full CSS."
		return &observed, nil
	}
	if name == "" {
		hash := sha256.Sum256(full)
		name = fmt.Sprintf("styles-%x.css", hash[:6])
		if err := b.emit(name, "release-complete-css", full); err != nil {
			return report, err
		}
	}
	criticalNode := htmlx.Element("style", string(critical))
	link := htmlx.Element("link", "")
	htmlx.SetAttr(link, "rel", "stylesheet")
	htmlx.SetAttr(link, "href", fileURL(name))
	htmlx.SetAttr(link, "media", "print")
	htmlx.SetAttr(link, "onload", "this.media='all';this.onload=null")
	// A real native sheet is used when scripting is disabled. No executable
	// bundle, framework or custom CSS runtime is needed for stylesheet loading.
	fallbackLink := htmlx.Element("link", "")
	htmlx.SetAttr(fallbackLink, "rel", "stylesheet")
	htmlx.SetAttr(fallbackLink, "href", fileURL(name))
	var fallbackHTML bytes.Buffer
	if err := html.Render(&fallbackHTML, fallbackLink); err != nil {
		return report, err
	}
	fallback := htmlx.Element("noscript", fallbackHTML.String())
	parent := node.Parent
	parent.InsertBefore(criticalNode, node)
	parent.InsertBefore(link, node)
	parent.InsertBefore(fallback, node)
	parent.RemoveChild(node)
	observed.Applied = true
	observed.Stylesheet = name
	observed.CSS = ""
	return &observed, nil
}

func optionalReleaseFailure(err error) bool {
	var optional optionalReleaseError
	return errors.As(err, &optional)
}

// imageHints fills the loading and fetch-priority hints the author left out,
// from where the images render in Chrome: an image below the first screen at
// every reviewed size loads lazily, and the largest file-backed image of the
// phone's first screen is fetched first when no image has a priority yet.
// Authored hints are never changed; embedded images keep theirs, since their
// bytes arrive with the page anyway.
func (b *builder) imageHints(doc *html.Node, report *release.Result) {
	var images []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Namespace == "" {
			if n.Data == "template" {
				return
			}
			if n.Data == "img" {
				images = append(images, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(images) == 0 {
		return
	}
	var preview bytes.Buffer
	if err := html.Render(&preview, doc); err != nil {
		return
	}
	placements, err := release.ImagePlacement(preview.Bytes(), b.sidecars())
	if err != nil || len(placements) != len(images) {
		return
	}
	prioritized := false
	for k, img := range images {
		if placements[k].Src != htmlx.Attr(img, "src") {
			return // the browser saw other images; change nothing
		}
		prioritized = prioritized || htmlx.HasAttr(img, "fetchpriority")
	}
	var first *html.Node
	best := 0.0
	for k, img := range images {
		src := htmlx.Attr(img, "src")
		if strings.HasPrefix(strings.ToLower(src), "data:") || src == "" {
			continue
		}
		p := placements[k]
		if !p.FirstScreen && !htmlx.HasAttr(img, "loading") {
			htmlx.SetAttr(img, "loading", "lazy")
			report.LazyImages++
		}
		if p.PhoneArea > best && !strings.EqualFold(htmlx.Attr(img, "loading"), "lazy") {
			first, best = img, p.PhoneArea
		}
	}
	// a priority only helps an image large enough to be the largest paint
	if !prioritized && first != nil && best >= 100*100 {
		htmlx.SetAttr(first, "fetchpriority", "high")
		report.PriorityImage = htmlx.Attr(first, "src")
	}
}
