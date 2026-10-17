package compile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"golang.org/x/net/html"
)

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
		if htmlx.HasAttr(node, "data-lfm-helper") {
			id := htmlx.Attr(node, "data-lfm-helper")
			entry, ok := b.helpers[id]
			if !ok || node.Namespace != "" || node.Data != entry.kind || htmlx.RawText(node) != "" || independentlyAddressable(node, "type", "data-lfm-helper") {
				return fmt.Errorf("invalid compiler asset marker; script/style helpers must occupy their own HTML element position")
			}
			consumedHelpers[id] = true
			plan(node).helper(node, entry)
			continue
		}
		if id := htmlx.Attr(node, "id"); id != "" {
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
			kind := strings.ToLower(strings.TrimSpace(htmlx.Attr(node, "type")))
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
			rel := strings.ToLower(htmlx.Attr(node, "rel"))
			if htmlx.Rel(node, "canonical") && htmlx.Attr(node, "href") == renderedCanonical {
				htmlx.SetAttr(node, "href", b.config.URL)
			}
			if htmlx.Rel(node, "discord:component-embed") {
				ref, err := b.resource(htmlx.Attr(node, "href"), b.root, true)
				if err != nil {
					return err
				}
				base, _ := url.Parse(b.config.URL)
				u, err := url.Parse(ref)
				if err != nil {
					return err
				}
				htmlx.SetAttr(node, "href", base.ResolveReference(u).String())
			}
			if stylesheetRel(rel) {
				if err := plan(node).stylesheet(node); err != nil {
					return err
				}
				continue
			}
			if htmlx.Rel(node, "preload") || htmlx.Rel(node, "modulepreload") || htmlx.Rel(node, "prefetch") {
				// Reconcile against final packing and bundle filenames later.
				continue
			}
		case "meta":
			key := htmlx.Attr(node, "property")
			if key == "" {
				key = htmlx.Attr(node, "name")
			}
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "og:image" || key == "og:image:url" || key == "og:image:secure_url" || key == "twitter:image" || key == "twitter:image:src" || key == "snapchat:sticker" || key == "twitter:player:stream" || key == "og:video" || key == "og:video:url" || key == "og:video:secure_url" || key == "og:audio" || key == "og:audio:url" || key == "og:audio:secure_url" {
				if htmlx.Attr(node, "content") == "" {
					return fmt.Errorf("%s needs a media source", key)
				}
				ref, err := b.resource(htmlx.Attr(node, "content"), b.root, true)
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
				htmlx.SetAttr(node, "content", u.String())
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
				icon := node.Data == "link" && (htmlx.Rel(node, "icon") || htmlx.Rel(node, "apple-touch-icon") || htmlx.Rel(node, "mask-icon"))
				if icon || htmlx.HasAttr(node, "download") || strings.HasPrefix(item.Val, "assets/") || strings.HasPrefix(item.Val, "./assets/") {
					fileIcon := icon
					if htmlx.Rel(node, "icon") {
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
					item.Val, err = b.resource(item.Val, b.root, htmlx.HasAttr(node, "download") || fileIcon || nativeLink)
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
			htmlx.Move(owner, node)
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
		node := htmlx.Element("script", string(data))
		htmlx.SetAttr(node, "type", "application/json")
		htmlx.SetAttr(node, "id", "product-data")
		body.AppendChild(node)
	}
	if len(jsonld) > 0 {
		node := htmlx.Element("script", string(jsonld))
		htmlx.SetAttr(node, "type", "application/ld+json")
		body.AppendChild(node)
	}
	for _, node := range scripts {
		htmlx.Move(body, node)
	}
	return nil
}
