package check

import (
	"fmt"
	htmltext "html"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const discordPayloadLimit = 3000

func discordRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	var carriers []*html.Node
	for _, n := range d.Elements {
		if n.Namespace == "" && !inTemplate(n) && (n.Data == "script" && Attr(n, "id") == "discord:component-embed" || n.Data == "link" && containsWord(Attr(n, "rel"), "discord:component-embed")) {
			carriers = append(carriers, n)
		}
	}
	if len(carriers) > 1 {
		issue := d.At(carriers[1], Error, "Multiple Discord component-embed declarations.", "Choose one inline payload or linked JSON file; keep ordinary OG fallback alongside it.")
		issue.Related = []Location{d.At(carriers[0], Error, "", "").Location}
		issues = append(issues, issue)
	}
	for _, n := range carriers {
		if !inHead(n) {
			issues = append(issues, d.At(n, Error, "Discord component-embed declaration is outside head.", "Put the initial script/link in head; Discord does not execute page JavaScript."))
		}
		kind := strings.ToLower(strings.TrimSpace(Attr(n, "type")))
		if kind != "application/json" && kind != "application/vnd.discord.component-embed+json" {
			issues = append(issues, d.At(n, Error, "Discord component-embed has an unsupported type.", "Use type=application/json or application/vnd.discord.component-embed+json; do not make the payload executable."))
		}
		raw := []byte(Text(n))
		name := ""
		if n.Data == "link" {
			value := Attr(n, "href")
			if invalid := sharingURL(d, n, value, "Discord component JSON href", false, false); len(invalid) > 0 {
				issues = append(issues, invalid...)
				continue
			}
			_, analyzer := d.linkFacts()
			base, baseErr := analyzer.Resolve(d.Options.URL)
			u, err := analyzer.Resolve(value)
			if d.Options.URL != "" && baseErr == nil && err == nil && !relatedHosts(base.Hostname(), u.Hostname()) {
				issues = append(issues, d.At(n, Error, "Discord component JSON is on an unrelated host.", "Use the page's exact host, a parent domain or its subdomain; a sibling or suffix-lookalike domain does not qualify."))
			}
			var ok bool
			raw, name, ok = sharingAsset(d, value)
			if !ok {
				issues = append(issues, d.At(n, Warning, "Linked Discord component JSON was not inspected (not an emitted sidecar).", "Verify its exact JSON bytes, 3,000-byte limit and live preview with Discord's Embed Debugger; normal builds do not fetch endpoints."))
				continue
			}
		} else if HasAttr(n, "src") {
			issues = append(issues, d.At(n, Error, "Discord JSON script cannot use src as payload discovery.", "Use inline script text or a link rel=discord:component-embed with an absolute href."))
		}
		v := discordValidator{d: d, node: n, file: name}
		if len(raw) > discordPayloadLimit {
			v.add("payload", fmt.Sprintf("is %d bytes; maximum is %d including whitespace/escapes", len(raw), discordPayloadLimit))
		} else {
			object, err := sharingJSON(raw)
			if err != nil {
				v.add("payload", err.Error())
			} else {
				component, ok := object["component"].(map[string]any)
				if !ok || len(object) != 1 {
					v.add("payload", "requires exactly one top-level component object")
				}
				v.component(component, "component", 0)
				if v.count > 40 {
					v.add("payload", fmt.Sprintf("has %d components; maximum is 40 including the root", v.count))
				}
				if v.gallery > 10 {
					v.add("payload", fmt.Sprintf("has %d gallery items; maximum is 10 across the whole embed", v.gallery))
				}
			}
		}
		issues = append(issues, v.issues...)
		if v.buttons == 0 && len(v.issues) == 0 {
			issues = append(issues, d.At(n, Warning, "Discord component preview has no native link button.", "Consider a useful result/compare/try-your-own continuation. Markdown/media can also link; verify the actual clickable return path rather than adding a generic share plea."))
		}
	}
	return issues
}

func relatedHosts(a, b string) bool {
	a, b = strings.TrimSuffix(strings.ToLower(a), "."), strings.TrimSuffix(strings.ToLower(b), ".")
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

type discordValidator struct {
	d                       *Document
	node                    *html.Node
	file                    string
	issues                  []Diagnostic
	count, gallery, buttons int
}

func (v *discordValidator) add(path, problem string) {
	issue := v.d.At(v.node, Error, "Discord "+path+": "+problem+".", "Fix this JSON field against the supported component-embed schema; extra button keys invalidate the whole payload. Keep URLs named in lfm.json and retain OG fallback.")
	if v.file != "" {
		issue.Related = []Location{{File: v.file, Line: 1, Column: 1}}
	}
	v.issues = append(v.issues, issue)
}

func (v *discordValidator) destination(value, path string, limit int, navigation bool) {
	if registry := v.d.Options.Registry; registry != nil && len(registry.Match(value)) == 0 && value != htmltext.UnescapeString(value) && len(registry.Match(htmltext.UnescapeString(value))) > 0 {
		v.issues = append(v.issues, v.d.At(v.node, Error, "Discord "+path+" contains HTML entity escaping inside raw JSON.", "Use type=application/json for templated inline Discord JSON; Go's HTML template engine treats the vendor MIME as HTML context. Store raw & in config. Raw script JSON does not decode &amp; into &."))
	}
	if invalid := sharingURL(v.d, v.node, value, "Discord "+path, false, navigation); len(invalid) > 0 {
		v.issues = append(v.issues, invalid...)
	}
	if utf8.RuneCountInString(value) > limit {
		v.add(path, fmt.Sprintf("URL exceeds %d characters", limit))
	}
	// Inline JSON is already covered by links.declared. Linked JSON has its own
	// bytes and therefore needs registry coverage here, without duplicate findings.
	if v.file != "" && v.d.Options.Registry != nil && len(v.d.Options.Registry.Match(value)) == 0 {
		_, analyzer := v.d.linkFacts()
		if !v.d.generatedURL(value, analyzer) {
			v.add(path, "destination has no named config URL")
		}
	}
}

func (v *discordValidator) media(value any, path string) {
	media, ok := value.(map[string]any)
	if !ok {
		v.add(path, "requires a media object with url")
		return
	}
	v.destination(jsonString(media, "url"), path+".url", 2048, false)
}

func (v *discordValidator) children(object map[string]any, path string, min, max int, allowed map[int64]bool) {
	children, ok := object["components"].([]any)
	if !ok || len(children) < min || len(children) > max {
		v.add(path+".components", fmt.Sprintf("requires %d–%d children", min, max))
	}
	for i, child := range children {
		component, _ := child.(map[string]any)
		kind, ok := jsonInteger(component, "type")
		if ok && !allowed[kind] {
			v.add(fmt.Sprintf("%s.components[%d]", path, i), "component type is not allowed in this parent")
		}
		v.component(component, fmt.Sprintf("%s.components[%d]", path, i), 1)
	}
}

func (v *discordValidator) component(object map[string]any, path string, depth int) {
	if object == nil {
		v.add(path, "requires a component object")
		return
	}
	v.count++
	kind, ok := jsonInteger(object, "type")
	if !ok || depth == 0 && kind != 17 {
		v.add(path+".type", "requires an integer component type; the root must be Container 17")
		return
	}
	switch kind {
	case 17:
		if depth != 0 {
			v.add(path, "Container cannot contain another Container")
			return
		}
		v.children(object, path, 1, 39, map[int64]bool{1: true, 9: true, 10: true, 12: true, 14: true})
	case 1:
		v.children(object, path, 1, 5, map[int64]bool{2: true})
	case 9:
		v.children(object, path, 1, 3, map[int64]bool{10: true})
		accessory, _ := object["accessory"].(map[string]any)
		kind, ok := jsonInteger(accessory, "type")
		if !ok || kind != 2 && kind != 11 {
			v.add(path+".accessory", "requires a Button 2 or Thumbnail 11")
		}
		v.component(accessory, path+".accessory", depth+1)
	case 2:
		style, ok := jsonInteger(object, "style")
		if !ok || style != 5 {
			v.add(path+".style", "buttons must use link style 5")
		}
		allowed := map[string]bool{"id": true, "type": true, "url": true, "style": true, "label": true, "emoji": true, "disabled": true}
		var extra []string
		for key := range object {
			if !allowed[key] {
				extra = append(extra, key)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			v.add(path, "unsupported button keys: "+strings.Join(extra, ", "))
		}
		label := strings.TrimSpace(jsonString(object, "label"))
		if utf8.RuneCountInString(label) > 80 {
			v.add(path+".label", "maximum is 80 characters")
		}
		if value, exists := object["label"]; exists {
			if _, ok := value.(string); !ok {
				v.add(path+".label", "must be a string when supplied")
			}
		}
		if value, exists := object["disabled"]; exists {
			if _, ok := value.(bool); !ok {
				v.add(path+".disabled", "must be a boolean when supplied")
			}
		}
		emoji, _ := object["emoji"].(map[string]any)
		if label == "" && jsonString(emoji, "name") == "" && jsonString(emoji, "id") == "" {
			v.add(path, "button needs a nonempty label or emoji")
		}
		v.destination(jsonString(object, "url"), path+".url", 512, true)
		if disabled, _ := object["disabled"].(bool); !disabled {
			v.buttons++
		}
	case 10:
		if strings.TrimSpace(jsonString(object, "content")) == "" {
			v.add(path+".content", "Text Display requires nonempty text")
		}
	case 11:
		v.media(object["media"], path+".media")
	case 12:
		items, ok := object["items"].([]any)
		if !ok || len(items) == 0 {
			v.add(path+".items", "Media Gallery requires at least one item")
		}
		v.gallery += len(items)
		for i, item := range items {
			media, _ := item.(map[string]any)
			v.media(media["media"], fmt.Sprintf("%s.items[%d].media", path, i))
		}
	case 14:
		// Optional separator fields/defaults are intentionally left authored.
	default:
		v.add(path+".type", "unsupported component type for a link preview")
	}
}
