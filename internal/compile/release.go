package compile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

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
			if node.Data == "meta" && strings.EqualFold(attr(node, "http-equiv"), "content-security-policy") {
				blocked = true
			}
			if node.Data == "style" || (node.Data == "link" && stylesheetRel(attr(node, "rel"))) {
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
	if independentlyAddressable(node, "rel", "href", "type") || strings.Contains(attr(node, "rel"), "alternate") {
		report.Reason = "Stylesheet has independent loading/identity/security semantics; kept full CSS."
		return report, nil
	}
	full := []byte(textContent(node))
	name := ""
	if node.Data == "link" {
		ref, err := url.Parse(attr(node, "href"))
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
	setAttr(node, "data-lfm-critical-source", "")
	defer delAttr(node, "data-lfm-critical-source")
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
	criticalNode := element("style", string(critical))
	link := element("link", "")
	setAttr(link, "rel", "stylesheet")
	setAttr(link, "href", fileURL(name))
	setAttr(link, "media", "print")
	setAttr(link, "onload", "this.media='all';this.onload=null")
	// A real native sheet is used when scripting is disabled. No executable
	// bundle, framework or custom CSS runtime is needed for stylesheet loading.
	fallbackLink := element("link", "")
	setAttr(fallbackLink, "rel", "stylesheet")
	setAttr(fallbackLink, "href", fileURL(name))
	var fallbackHTML bytes.Buffer
	if err := html.Render(&fallbackHTML, fallbackLink); err != nil {
		return report, err
	}
	fallback := element("noscript", fallbackHTML.String())
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
