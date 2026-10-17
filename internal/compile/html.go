package compile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func attr(node *html.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, item := range node.Attr {
		if item.Key == key {
			return item.Val
		}
	}
	return ""
}

func hasAttr(node *html.Node, key string) bool {
	for _, item := range node.Attr {
		if item.Key == key {
			return true
		}
	}
	return false
}

func setAttr(node *html.Node, key, value string) {
	for i := range node.Attr {
		if node.Attr[i].Key == key {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: key, Val: value})
}

func delAttr(node *html.Node, keys ...string) {
	kept := node.Attr[:0]
	for _, item := range node.Attr {
		remove := false
		for _, key := range keys {
			if item.Key == key {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, item)
		}
	}
	node.Attr = kept
}

func element(name, text string) *html.Node {
	node := &html.Node{Type: html.ElementNode, Data: name, DataAtom: atom.Lookup([]byte(name))}
	if text != "" {
		node.AppendChild(&html.Node{Type: html.TextNode, Data: text})
	}
	return node
}

func textContent(node *html.Node) string {
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(child.Data)
	}
	return text.String()
}

func setText(node *html.Node, value string) {
	for node.FirstChild != nil {
		node.RemoveChild(node.FirstChild)
	}
	node.AppendChild(&html.Node{Type: html.TextNode, Data: value})
}

func move(parent, node *html.Node) {
	if node.Parent != nil {
		node.Parent.RemoveChild(node)
	}
	parent.AppendChild(node)
}

func (b *builder) processHTML(doc *html.Node, data, jsonld []byte) error {
	renderedCanonical, err := renderedURLAttribute(b.config.URL)
	if err != nil {
		return fmt.Errorf("canonical URL attribute: %w", err)
	}
	var nodes []*html.Node
	bundles := &pageBundles{b: b}
	var owners []*html.Node
	inert := map[*html.Node]*pageBundles{}
	plan := func(node *html.Node) *pageBundles {
		for parent := node.Parent; parent != nil; parent = parent.Parent {
			if parent.Type == html.ElementNode && parent.Data == "template" && parent.Namespace == "" {
				if inert[parent] == nil {
					owners = append(owners, parent)
					inert[parent] = &pageBundles{b: b, prefix: fmt.Sprintf("template-%d-", len(owners))}
				}
				return inert[parent]
			}
		}
		return bundles
	}
	var head, body *html.Node
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		nodes = append(nodes, node)
		if node.Type == html.ElementNode {
			if node.Data == "head" {
				head = node
			}
			if node.Data == "body" {
				body = node
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(doc)
	if head == nil || body == nil {
		return fmt.Errorf("template must produce an HTML document")
	}
	consumedHelpers := map[string]bool{}
	for _, node := range nodes {
		if node.Type == html.CommentNode {
			if node.Parent != nil {
				node.Parent.RemoveChild(node)
			}
			continue
		}
		if node.Type != html.ElementNode {
			continue
		}
		if hasAttr(node, "data-lfm-helper") {
			id := attr(node, "data-lfm-helper")
			entry, ok := b.helpers[id]
			if !ok || node.Namespace != "" || node.Data != entry.kind || textContent(node) != "" || independentlyAddressable(node, "type", "data-lfm-helper") {
				return fmt.Errorf("invalid compiler asset marker; script/style helpers must occupy their own HTML element position")
			}
			consumedHelpers[id] = true
			plan(node).helper(node, entry)
			continue
		}
		if id := attr(node, "id"); id != "" {
			if id == "product-data" {
				return fmt.Errorf("id=product-data is reserved for build.browserData")
			}
		}
		if node.Data == "base" {
			return fmt.Errorf("<base> is unsupported: emitted assets are relative to index.html")
		}
		if node.Data == "meta" || node.Data == "script" {
			if err := b.sharingAssets(node); err != nil {
				return fmt.Errorf("sharing media in <%s>: %w", node.Data, err)
			}
		}
		switch node.Data {
		case "script":
			kind := strings.ToLower(strings.TrimSpace(attr(node, "type")))
			if kind != "" && kind != "module" && kind != "text/javascript" && kind != "application/javascript" {
				continue
			}
			if err := plan(node).script(node, kind == "module"); err != nil {
				return err
			}
			continue
		case "style":
			if err := plan(node).stylesheet(node); err != nil {
				return err
			}
			continue
		case "link":
			rel := strings.ToLower(attr(node, "rel"))
			if relToken(node, "canonical") && attr(node, "href") == renderedCanonical {
				setAttr(node, "href", b.config.URL)
			}
			if relToken(node, "discord:component-embed") {
				ref, err := b.resource(attr(node, "href"), b.root, true)
				if err != nil {
					return err
				}
				base, _ := url.Parse(b.config.URL)
				u, err := url.Parse(ref)
				if err != nil {
					return err
				}
				setAttr(node, "href", base.ResolveReference(u).String())
			}
			if stylesheetRel(rel) {
				if err := plan(node).stylesheet(node); err != nil {
					return err
				}
				continue
			}
			if relToken(node, "preload") || relToken(node, "modulepreload") || relToken(node, "prefetch") {
				// Reconcile against final packing and bundle filenames later.
				continue
			}
		case "meta":
			key := attr(node, "property")
			if key == "" {
				key = attr(node, "name")
			}
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "og:image" || key == "og:image:url" || key == "og:image:secure_url" || key == "twitter:image" || key == "twitter:image:src" || key == "snapchat:sticker" || key == "twitter:player:stream" || key == "og:video" || key == "og:video:url" || key == "og:video:secure_url" || key == "og:audio" || key == "og:audio:url" || key == "og:audio:secure_url" {
				if attr(node, "content") == "" {
					return fmt.Errorf("%s needs a media source", key)
				}
				ref, err := b.resource(attr(node, "content"), b.root, true)
				if err != nil {
					return err
				}
				base, _ := url.Parse(b.config.URL)
				u, err := url.Parse(ref)
				if err != nil {
					return err
				}
				u = base.ResolveReference(u)
				if u.Scheme != "https" && u.Scheme != "http" {
					return fmt.Errorf("%s needs a public HTTP(S) media URL", key)
				}
				setAttr(node, "content", u.String())
			}
		}
		for i := range node.Attr {
			item := &node.Attr[i]
			var err error
			switch item.Key {
			case "src", "poster":
				item.Val, err = b.resource(item.Val, b.root, false)
			case "data":
				if node.Data == "object" {
					item.Val, err = b.resource(item.Val, b.root, false)
				}
			case "srcset", "imagesrcset":
				item.Val, err = b.srcset(item.Val)
			case "href":
				icon := node.Data == "link" && (relToken(node, "icon") || relToken(node, "apple-touch-icon") || relToken(node, "mask-icon"))
				if icon || hasAttr(node, "download") || strings.HasPrefix(item.Val, "assets/") || strings.HasPrefix(item.Val, "./assets/") {
					fileIcon := icon
					if relToken(node, "icon") {
						if path, _, pathErr := b.assetPath(item.Val, b.root); pathErr == nil {
							relative, _ := filepath.Rel(b.root, path)
							if b.config.Build.Assets.Overrides[filepath.ToSlash(relative)] == "inline" {
								fileIcon = false
							}
						}
					}
					// Navigation, feeds and manifests need real URLs even when tiny;
					// browsers/crawlers cannot reliably navigate to inline data URLs.
					nativeLink := !icon && (node.Data == "a" || node.Data == "area" || node.Data == "link")
					item.Val, err = b.resource(item.Val, b.root, hasAttr(node, "download") || fileIcon || nativeLink)
				}
			case "style":
				var result compiled
				result, err = b.compile("", "x{"+item.Val+"}", true, false)
				if err == nil {
					item.Val = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(string(result.code)), "x{"), "}")
				}
			}
			if err != nil {
				return fmt.Errorf("<%s %s>: %w", node.Data, item.Key, err)
			}
		}
	}
	for id, entry := range b.helpers {
		if !consumedHelpers[id] {
			return fmt.Errorf("%s helper in %s was not rendered as an HTML element; place {{%s ...}} directly in HTML, outside attributes, scripts and styles", entry.kind, entry.origin, entry.kind)
		}
	}
	// Template content must stay dormant until the page explicitly instantiates
	// it. Bundling it into the active page would run its scripts/apply its CSS early.
	for _, owner := range owners {
		scripts, err := inert[owner].compileScripts()
		if err != nil {
			return err
		}
		if err := inert[owner].compileStyles(owner); err != nil {
			return err
		}
		for _, node := range scripts {
			move(owner, node)
		}
	}
	scripts, err := bundles.compileScripts()
	if err != nil {
		return err
	}
	if err := bundles.compileStyles(head); err != nil {
		return err
	}
	if len(b.config.Build.BrowserData) > 0 {
		node := element("script", string(data))
		setAttr(node, "type", "application/json")
		setAttr(node, "id", "product-data")
		body.AppendChild(node)
	}
	if len(jsonld) > 0 {
		node := element("script", string(jsonld))
		setAttr(node, "type", "application/ld+json")
		body.AppendChild(node)
	}
	for _, node := range scripts {
		move(body, node)
	}
	return nil
}
