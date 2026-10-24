package check

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type mediaSection struct {
	node    *html.Node
	heading string
	nodes   []*html.Node
}

// contentSections uses outermost sections inside main (or body). Without
// sections, h1/h2 boundaries group the main content in document order.
func contentSections(d *Document) []mediaSection {
	root := d.Root
	for _, n := range d.Elements {
		if n.Data == "main" {
			root = n
			break
		}
		if n.Data == "body" {
			root = n
		}
	}
	var out []mediaSection
	var walk func(*html.Node, int)
	walk = func(n *html.Node, active int) {
		if n.Type == html.ElementNode && (n.Data == "nav" || n.Data == "footer" || n.Data == "template") {
			return
		}
		if n.Type == html.ElementNode && n.Data == "section" && active < 0 {
			out = append(out, mediaSection{node: n})
			active = len(out) - 1
		}
		if active >= 0 {
			out[active].nodes = append(out[active].nodes, n)
			if out[active].heading == "" && (n.Data == "h1" || n.Data == "h2") {
				out[active].heading = Text(n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, active)
		}
	}
	walk(root, -1)
	if len(out) > 0 {
		return out
	}
	var flat func(*html.Node)
	flat = func(n *html.Node) {
		if n.Data == "nav" || n.Data == "footer" || n.Data == "template" {
			return
		}
		if n.Data == "h1" || n.Data == "h2" {
			if len(out) == 1 && out[0].heading == "" {
				out[0].node = n
				out[0].heading = Text(n)
			} else {
				out = append(out, mediaSection{node: n, heading: Text(n)})
			}
		}
		if len(out) == 0 {
			out = append(out, mediaSection{node: root})
		}
		out[len(out)-1].nodes = append(out[len(out)-1].nodes, n)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			flat(c)
		}
	}
	flat(root)
	return out
}

func mediaBackgrounds(d *Document, n *html.Node) []string {
	var refs []string
	m := d.styles()
	// background-image includes the winning shorthand. Do not count an earlier
	// background after a later background-image:none overrides it.
	value := m.value(n, "background-image")
	for _, match := range cssURL.FindAllStringSubmatch(value, -1) {
		ref := match[1]
		file := ""
		for _, r := range m.base[n] {
			for _, decl := range r.decls {
				if (decl.prop == "background" || decl.prop == "background-image" || strings.HasPrefix(decl.prop, "--")) && strings.Contains(decl.value, ref) {
					file = r.file
				}
			}
		}
		for _, decl := range m.inline[n] {
			if strings.Contains(decl.value, ref) {
				file = ""
			}
		}
		if file != "" {
			base, err := url.Parse(file)
			relative, e := url.Parse(ref)
			if err == nil && e == nil {
				ref = base.ResolveReference(relative).String()
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func sectionMedia(d *Document, n *html.Node) bool {
	switch n.Data {
	case "img", "picture", "video", "canvas":
		return true
	case "svg":
		w, h := sizeAttr(Attr(n, "width")), sizeAttr(Attr(n, "height"))
		if w == 0 || h == 0 || w > 48 || h > 48 {
			return true
		}
	}
	return len(mediaBackgrounds(d, n)) > 0
}

func sectionMediaRule(d *Document) []Diagnostic {
	immersive := d.Options.Concept != nil && d.Options.Concept.Intensity == "immersive"
	var issues []Diagnostic
	for _, s := range contentSections(d) {
		name := s.heading
		if name == "" {
			name = "Untitled section"
		}
		media, words := false, 0
		var first *html.Node
		flush := func() {
			if words > 300 {
				issues = append(issues, d.At(first, Warning, fmt.Sprintf("Section %q has a text wall of %d words in consecutive paragraphs.", name, words), "Break it with a subheading, list, table or a visual, or cut it."))
			}
			words, first = 0, nil
		}
		// Nodes are in document order. A breaking element's paragraphs do not
		// belong to the surrounding run.
		var blocked *html.Node
		for _, n := range s.nodes {
			if n.Type != html.ElementNode || !d.designVisible(n) {
				continue
			}
			hasMedia := sectionMedia(d, n)
			media = media || hasMedia
			if blocked != nil {
				inside := false
				for p := n.Parent; p != nil; p = p.Parent {
					if p == blocked {
						inside = true
						break
					}
				}
				if inside {
					continue
				}
				blocked = nil
			}
			breaker, container := hasMedia, false
			switch n.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "menu", "dl", "table", "figure", "blockquote", "details", "form", "picture", "video", "canvas":
				breaker, container = true, true
			case "svg":
				container = hasMedia
			}
			if breaker {
				flush()
				if container {
					blocked = n
				}
				continue
			}
			if n.Data == "p" {
				var text strings.Builder
				var walk func(*html.Node)
				walk = func(c *html.Node) {
					if !d.designVisible(c) {
						return
					}
					if c.Type == html.TextNode {
						text.WriteString(c.Data)
					}
					for child := c.FirstChild; child != nil; child = child.NextSibling {
						walk(child)
					}
				}
				walk(n)
				if first == nil {
					first = n
				}
				words += wordCount(text.String())
			}
		}
		flush()
		if immersive && !media {
			issues = append(issues, d.At(s.node, Warning, fmt.Sprintf("Immersive section %q has no media.", name), "Add the planned media for this section or lower Intensity in concept.md to balanced or restrained."))
		}
	}
	return issues
}

func eagerVideoRule(d *Document) []Diagnostic {
	first := map[*html.Node]bool{}
	if sections := contentSections(d); len(sections) > 0 {
		for _, n := range sections[0].nodes {
			first[n] = true
		}
	}
	var issues []Diagnostic
	for _, n := range d.Elements {
		if n.Data != "video" || d.inTemplate(n) {
			continue
		}
		auto := HasAttr(n, "autoplay")
		preload := strings.ToLower(Attr(n, "preload"))
		if !first[n] && (preload == "auto" || auto) {
			issues = append(issues, d.At(n, Warning, "Video below the first content section downloads at page load.", "Drop autoplay, set preload=\"none\" and a poster, and play it from an IntersectionObserver; see the implementation skill's video loop."))
		}
		// ux.media already reports missing muted.
		if auto && !HasAttr(n, "playsinline") {
			issues = append(issues, d.At(n, Warning, "Autoplay video lacks playsinline.", "Set playsinline so mobile Safari can play in the page; ux.media checks that autoplay starts muted."))
		}
	}
	return issues
}

// mediaBytes counts each known referenced file once, including responsive
// candidates and CSS backgrounds. Unknown remote assets are left out.
func mediaBytes(d *Document) (media, video, first int) {
	firstNodes := map[*html.Node]bool{}
	if sections := contentSections(d); len(sections) > 0 {
		for _, n := range sections[0].nodes {
			firstNodes[n] = true
		}
	}
	all, videos, paint := map[string]int{}, map[string]int{}, map[string]int{}
	add := func(ref string, isVideo, inFirst bool) {
		if ref == "" {
			return
		}
		raw, key, ok := sharingAsset(d, ref)
		if !ok {
			raw, ok = d.Options.Assets[ref]
			key = ref
		}
		size := len(raw)
		if known, exists := d.Options.AssetBytes[ref]; exists {
			size = known
			ok = known >= 0
			if key == "" {
				key = ref
			}
		}
		if !ok {
			return
		}
		if isVideo {
			videos[key] = size
		} else {
			all[key] = size
		}
		if inFirst {
			paint[key] = size
		}
	}
	for _, n := range d.Elements {
		if d.inTemplate(n) {
			continue
		}
		inFirst := firstNodes[n] && !strings.EqualFold(Attr(n, "loading"), "lazy")
		switch n.Data {
		case "img":
			add(Attr(n, "src"), false, inFirst)
		case "video":
			add(Attr(n, "poster"), false, firstNodes[n])
			add(Attr(n, "src"), true, inFirst && (HasAttr(n, "autoplay") || strings.EqualFold(Attr(n, "preload"), "auto")))
		case "source":
			if n.Parent != nil && n.Parent.Data == "video" {
				add(Attr(n, "src"), true, inFirst && (HasAttr(n.Parent, "autoplay") || strings.EqualFold(Attr(n.Parent, "preload"), "auto")))
			}
		}
		if n.Data == "img" || n.Data == "source" && n.Parent != nil && n.Parent.Data == "picture" {
			for _, part := range strings.Split(Attr(n, "srcset"), ",") {
				if fields := strings.Fields(part); len(fields) > 0 {
					add(fields[0], false, inFirst && !lazyPicture(n))
				}
			}
		}
		for _, ref := range mediaBackgrounds(d, n) {
			add(ref, false, inFirst)
		}
	}
	for _, n := range all {
		media += n
	}
	for _, n := range videos {
		video += n
	}
	for _, n := range paint {
		first += n
	}
	return
}

func lazyPicture(n *html.Node) bool {
	if n.Parent == nil || n.Parent.Data != "picture" {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Data == "img" && strings.EqualFold(Attr(c, "loading"), "lazy") {
			return true
		}
	}
	return false
}
