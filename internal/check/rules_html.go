package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/tdewolff/parse/v2"
	htmllex "github.com/tdewolff/parse/v2/html"
	"golang.org/x/net/html"
)

var voidTags = words("area base br col embed hr img input link meta param source track wbr")
var optionalEnds = words("html head body p li dt dd rt rp optgroup option colgroup thead tbody tfoot tr td th")
var implicitContainers = words("html head body tbody colgroup")
var headTags = words("title meta link script style base noscript template")

func words(value string) map[string]bool {
	m := map[string]bool{}
	for _, word := range strings.Fields(value) {
		m[word] = true
	}
	return m
}

// Compare library-parsed structure with original token events. This catches
// dropped/reparented elements and required end-tag repairs without reimplementing
// the HTML tree builder. Legal optional end tags/implied containers are normalized.
// It is intentionally a repair detector, not a complete WHATWG conformance checker.
func syntaxRule(d *Document) []Diagnostic {
	issues := syntaxIssues(d.Raw, "index.html", d.Root, d.Tokens)
	if len(d.Options.RenderedSource) > 0 && !bytes.Equal(d.Raw, d.Options.RenderedSource) {
		issues = append(issues, syntaxIssues(d.Options.RenderedSource, "rendered-template.html", nil, nil)...)
	}
	return issues
}

func syntaxIssues(raw []byte, file string, root *html.Node, tokens []Token) []Diagnostic {
	var issues []Diagnostic
	lines := lineStarts(raw)
	add := func(offset int, message, hint string) {
		issues = append(issues, Diagnostic{Severity: Error, Location: location(file, lines, offset), Message: message, Hint: hint})
	}
	if !validUTF8(raw) {
		add(0, "HTML contains invalid UTF-8 or a NUL byte.", "Write UTF-8 text and remove NUL bytes before compilation.")
	}
	if tokens == nil {
		var err error
		tokens, err = tokenize(raw)
		if err != nil {
			add(0, err.Error(), "Fix malformed HTML tokenization.")
			return issues
		}
	}
	foreign := 0
	inHead := false
	headTemplate := 0
	containers := map[string]int{}
	issues = append(issues, attributeIssues(raw, file, tokens)...)
	for _, token := range tokens {
		name := token.Value.Data
		switch token.Type {
		case html.ErrorToken:
			add(token.Offset, "Incomplete HTML token at end of document.", "Close the tag and any quoted attribute value.")
		case html.CommentToken:
			part := raw[token.Offset:token.End]
			if foreign > 0 && bytes.HasPrefix(part, []byte("<![CDATA[")) && bytes.HasSuffix(part, []byte("]]>")) {
				continue
			}
			if !bytes.HasPrefix(part, []byte("<!--")) || !bytes.HasSuffix(part, []byte("-->")) {
				add(token.Offset, "Malformed HTML comment or declaration.", "Use <!-- ... --> for comments and <!doctype html> for the doctype.")
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			if name == "html" || name == "head" || name == "body" {
				containers[name]++
				if containers[name] > 1 {
					add(token.Offset, fmt.Sprintf("Multiple <%s> start tags.", name), "Produce one complete document; browsers silently discard duplicate document containers.")
				}
			}
			if name == "head" {
				inHead = true
			}
			if inHead && name == "template" {
				headTemplate++
			}
			if inHead && name != "head" && headTemplate == 0 && !headTags[name] {
				add(token.Offset, fmt.Sprintf("<%s> is not permitted in <head>; browsers may move later metadata into the body.", name), "Keep only metadata elements in head; put visible content in body.")
			}
			if name == "svg" || name == "math" {
				foreign++
			}
			if token.Type == html.SelfClosingTagToken && foreign == 0 && !voidTags[name] {
				add(token.Offset, fmt.Sprintf("HTML <%s/> does not self-close.", name), "Use an explicit end tag; slash syntax only self-closes foreign SVG/MathML elements.")
			}
			if token.Type == html.SelfClosingTagToken && (name == "svg" || name == "math") {
				foreign--
			}
		case html.EndTagToken:
			if name == "head" {
				inHead = false
			}
			if name == "template" && headTemplate > 0 {
				headTemplate--
			}
			if (name == "svg" || name == "math") && foreign > 0 {
				foreign--
			}
		}
	}
	if root == nil {
		var err error
		root, err = html.Parse(bytes.NewReader(raw))
		if err != nil {
			add(0, err.Error(), "Fix document structure.")
			return issues
		}
	}
	var normalized bytes.Buffer
	if err := html.Render(&normalized, root); err != nil {
		add(0, err.Error(), "Fix document structure.")
		return issues
	}
	repairedTokens, _ := tokenize(normalized.Bytes())
	before, after := structuralEvents(tokens), structuralEvents(repairedTokens)
	for i := 0; i < max(len(before), len(after)); i++ {
		if i < len(before) && i < len(after) && before[i].value == after[i].value {
			continue
		}
		offset := len(raw)
		found := "end of document"
		if i < len(before) {
			offset, found = before[i].offset, before[i].value
		}
		expected := "end of document"
		if i < len(after) {
			expected = after[i].value
		}
		add(offset, fmt.Sprintf("HTML parser repairs the structure near %s (browser structure has %s).", short(found), short(expected)), "Check nesting and required closing tags here. Valid optional end tags are allowed; browser repairs are not silently accepted.")
		break // One precise repair boundary is more useful than a cascade of guesses.
	}
	return issues
}

// x/net/html discards duplicate attributes during tokenization. The established
// parse/v2 lexer retains every attribute. Give it a copy: lexers may lowercase
// their backing input; analysis must never mutate the bytes destined for disk.
func attributeIssues(raw []byte, file string, tokens []Token) []Diagnostic {
	lines := lineStarts(raw)
	var issues []Diagnostic
	add := func(offset int, message, hint string) {
		issues = append(issues, Diagnostic{Severity: Error, Location: location(file, lines, offset), Message: message, Hint: hint})
	}
	for _, token := range tokens {
		if token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken && token.Type != html.EndTagToken {
			continue
		}
		part := bytes.Clone(raw[token.Offset:token.End])
		// parse/v2 treats complete SVG/MathML as an opaque XML token. Lex each
		// x/net-delimited start tag individually, masking these names (same byte
		// length), so foreign attributes receive the same raw checks.
		if token.Type != html.EndTagToken && (token.Value.Data == "svg" || token.Value.Data == "math") {
			for i := 1; i <= len(token.Value.Data); i++ {
				part[i] = 'x'
			}
		}
		input := parse.NewInputBytes(part)
		lexer := htmllex.NewLexer(input)
		seen := map[string]bool{}
		for {
			offset := token.Offset + input.Offset()
			kind, data := lexer.Next()
			if kind == htmllex.ErrorToken {
				break
			}
			if kind == htmllex.EndTagToken && !strings.EqualFold(strings.TrimSpace(string(lexer.Text())), token.Value.Data) {
				add(offset, "Invalid attributes or slash on an end tag.", "End tags contain only the element name, for example </div>.")
			}
			if kind != htmllex.AttributeToken {
				continue
			}
			key := strings.ToLower(string(lexer.AttrKey()))
			if len(data) > 0 && !strings.ContainsRune(" \t\n\r\f", rune(data[0])) {
				add(offset, "Missing whitespace before an attribute.", "Separate attributes with whitespace, including after quoted values.")
			}
			if strings.ContainsAny(key, "\"'<>=`/") || key == "" {
				add(offset, "Invalid character in an attribute name.", "Use a valid attribute name and quote its value.")
			}
			value := lexer.AttrVal()
			if value != nil && (len(value) == 0 || value[0] != 39 && value[0] != 34 && bytes.ContainsAny(value, "\"'=<`")) {
				add(offset, "Missing or invalid unquoted attribute value.", "Quote the complete attribute value; escape any literal quote inside it.")
			}
			if seen[key] {
				issues = append(issues, Diagnostic{Severity: Error, Location: location(file, lines, offset), Message: fmt.Sprintf("Duplicate attribute %q on <%s>.", key, token.Value.Data), Hint: "Keep one value for each attribute; duplicate values are silently discarded by browsers."})
			}
			seen[key] = true
		}
	}
	return issues
}

type structuralEvent struct {
	value  string
	offset int
}

func structuralEvents(tokens []Token) []structuralEvent {
	var events []structuralEvent
	foreign := 0
	for _, t := range tokens {
		name := t.Value.Data
		switch t.Type {
		case html.StartTagToken, html.SelfClosingTagToken:
			if name == "svg" || name == "math" {
				foreign++
			}
			if !implicitContainers[name] {
				events = append(events, structuralEvent{"<" + name + ">", t.Offset})
				if t.Type == html.SelfClosingTagToken && !voidTags[name] {
					events = append(events, structuralEvent{"</" + name + ">", t.Offset})
				}
			}
			if t.Type == html.SelfClosingTagToken && (name == "svg" || name == "math") {
				foreign--
			}
		case html.EndTagToken:
			if !optionalEnds[name] {
				events = append(events, structuralEvent{"</" + name + ">", t.Offset})
			}
			if (name == "svg" || name == "math") && foreign > 0 {
				foreign--
			}
		case html.TextToken:
			// Foreign CDATA is tokenized differently without the tree builder's
			// namespace feedback. Delegate foreign text handling to the DOM parser.
			if foreign > 0 {
				continue
			}
			if text := strings.TrimSpace(t.Value.Data); text != "" {
				events = append(events, structuralEvent{"text:" + text, t.Offset})
			}
		}
	}
	return events
}

func short(value string) string {
	if len(value) > 80 {
		return value[:80] + "…"
	}
	return value
}

func documentRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	doctypes := 0
	for _, token := range d.Tokens {
		if token.Type == html.DoctypeToken {
			doctypes++
			if token.Value.Data != "html" || len(token.Value.Attr) != 0 {
				issues = append(issues, d.Whole(Error, "Use the HTML5 doctype.", "Start the document with <!doctype html>; legacy public/system doctypes are unsupported."))
			}
		}
	}
	if doctypes != 1 {
		issues = append(issues, d.Whole(Error, fmt.Sprintf("Expected one HTML5 doctype, found %d.", doctypes), "Start the document with <!doctype html>."))
	}
	// The parser always yields exactly one html, head and body; duplicate start
	// tags in the source are reported by html.syntax from the token stream.
	charsets := d.activeNodes("meta[charset]")
	if len(charsets) == 0 {
		issues = append(issues, d.Whole(Warning, "No explicit UTF-8 encoding declaration.", "Put <meta charset=\"utf-8\"> first in head; the server must also serve UTF-8."))
	}
	if len(charsets) > 1 {
		issues = append(issues, d.At(charsets[1], Error, "Multiple character encoding declarations.", "Keep a single UTF-8 declaration."))
	}
	for _, n := range charsets {
		if !strings.EqualFold(strings.TrimSpace(Attr(n, "charset")), "utf-8") {
			issues = append(issues, d.At(n, Error, "Generated HTML must declare UTF-8.", "Use <meta charset=\"utf-8\">."))
		}
		for _, t := range d.Tokens {
			if t.Offset == d.positions[n] && t.End > 1024 {
				issues = append(issues, d.At(n, Error, "Encoding declaration ends after byte 1,024.", "Move the complete charset declaration to the start of head."))
				break
			}
		}
	}
	return issues
}

func contentRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	allowed := map[string]map[string]bool{
		"head": headTags, "ul": words("li script template"), "ol": words("li script template"),
		"table": words("caption colgroup thead tbody tfoot tr script template"),
		"thead": words("tr script template"), "tbody": words("tr script template"), "tfoot": words("tr script template"),
		"tr": words("td th script template"), "colgroup": words("col template"),
		"optgroup": words("option script template"), "picture": words("source img script template"),
	}
	parents := map[string]map[string]bool{
		"li": words("ul ol menu"), "dt": words("dl div"), "dd": words("dl div"),
		"tr": words("table thead tbody tfoot"), "td": words("tr"), "th": words("tr"),
		"caption": words("table"), "colgroup": words("table"), "col": words("colgroup"),
		"thead": words("table"), "tbody": words("table"), "tfoot": words("table"),
		"summary": words("details"), "legend": words("fieldset"), "source": words("picture audio video"), "track": words("audio video"),
	}
	phrasingParents := words("p h1 h2 h3 h4 h5 h6 pre span strong em b i u small sub sup s mark q cite code samp kbd var abbr time data label button")
	nonPhrasing := words("address article aside blockquote details dialog div dl fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hgroup hr main menu nav ol p pre search section table ul")
	for _, n := range d.Elements {
		if n.Namespace != "" {
			continue
		}
		if set := allowed[n.Data]; set != nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && !set[c.Data] {
					issues = append(issues, d.At(c, Error, fmt.Sprintf("<%s> is not a permitted direct child of <%s>.", c.Data, n.Data), "Move this element to a valid container."))
				}
				if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
					issues = append(issues, d.At(n, Error, fmt.Sprintf("Unexpected text directly inside <%s>.", n.Data), "Wrap text in the appropriate child element."))
				}
			}
		}
		if set := parents[n.Data]; set != nil && n.Parent != nil && !set[n.Parent.Data] {
			issues = append(issues, d.At(n, Error, fmt.Sprintf("<%s> is not permitted inside <%s>.", n.Data, n.Parent.Data), "Use the element's required parent container."))
		}
		for parent := n.Parent; parent != nil; parent = parent.Parent {
			if parent.Data == "template" {
				break
			}
			if parent.Type != html.ElementNode || parent.Namespace != "" {
				continue
			}
			invalid := (n.Data == "form" && parent.Data == "form") || (n.Data == "label" && parent.Data == "label")
			invalid = invalid || (parent.Data == "a" && (n.Data == "a" || interactive(n))) || (parent.Data == "button" && interactive(n))
			if invalid {
				issues = append(issues, d.At(n, Error, fmt.Sprintf("<%s> cannot be nested inside <%s>.", n.Data, parent.Data), "Make interactive controls siblings rather than placing one inside another."))
				break
			}
			if nonPhrasing[n.Data] && phrasingParents[parent.Data] {
				issues = append(issues, d.At(n, Error, fmt.Sprintf("<%s> is not phrasing content permitted inside <%s>.", n.Data, parent.Data), "Move block content outside this text/heading/control container."))
				break
			}
		}
	}
	return issues
}

func interactive(n *html.Node) bool {
	if n.Data == "a" {
		return HasAttr(n, "href")
	}
	if n.Data == "input" {
		return !strings.EqualFold(Attr(n, "type"), "hidden")
	}
	if n.Data == "audio" || n.Data == "video" {
		return HasAttr(n, "controls")
	}
	return slices.Contains([]string{"button", "select", "textarea", "details", "embed", "iframe"}, n.Data) || HasAttr(n, "tabindex")
}

func idsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	seen := map[string]bool{}
	for _, n := range d.activeNodes("[id]") {
		id := Attr(n, "id")
		if id == "" || strings.ContainsAny(id, " \t\r\n\f") {
			issues = append(issues, d.At(n, Error, "ID is empty or contains ASCII whitespace.", "Use a nonempty ID without spaces."))
		}
		if seen[id] {
			issues = append(issues, d.At(n, Error, fmt.Sprintf("duplicate HTML id %q", short(id)), "Assign unique IDs and update any matching references."))
		}
		seen[id] = true
	}
	return issues
}

func referencesRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	ids := map[string]*html.Node{}
	for _, n := range d.activeNodes("[id]") {
		ids[Attr(n, "id")] = n
	}
	for _, n := range d.Elements {
		if inTemplate(n) {
			continue
		}
		for _, key := range []string{"aria-labelledby", "aria-describedby", "aria-controls", "aria-owns", "aria-activedescendant", "aria-details", "aria-errormessage", "aria-flowto", "headers", "list", "form", "for", "popovertarget", "commandfor"} {
			if !nativeReferenceApplies(n, key) {
				continue
			}
			value := Attr(n, key)
			if value == "" {
				continue
			}
			refs := strings.Fields(value)
			if key == "for" && n.Data == "label" || key == "form" || key == "list" || key == "popovertarget" || key == "commandfor" {
				refs = []string{value}
			}
			for _, id := range refs {
				if ids[id] == nil {
					issues = append(issues, d.At(n, Error, fmt.Sprintf("%s references missing ID %q.", key, short(id)), "Add the target element or update/remove this reference."))
				}
			}
		}
		if n.Namespace == "" && (n.Data == "a" || n.Data == "area") {
			value := Attr(n, "href")
			if strings.HasPrefix(value, "#") && value != "#" {
				id, err := fragmentID(value[1:])
				if err != nil || (id != "" && !strings.EqualFold(id, "top") && d.fragmentTarget(id) == nil) {
					issues = append(issues, d.At(n, Warning, fmt.Sprintf("Fragment link %q has no matching target.", short(value)), "Add the target ID or correct the fragment; script-created targets cannot be verified here."))
				}
			}
		}
	}
	return issues
}

func jsonRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.selectNodes("script[type]") {
		kind := strings.ToLower(strings.TrimSpace(Attr(n, "type")))
		if kind == "application/ld+json" || kind == "application/json" || kind == "importmap" || kind == "speculationrules" {
			if !json.Valid([]byte(Text(n))) {
				issues = append(issues, d.At(n, Error, "Embedded JSON is invalid.", "Use valid JSON with no comments, trailing commas or literal HTML closing tags."))
			}
		}
	}
	return issues
}
