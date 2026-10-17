package urlcheck

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/tdewolff/parse/v2"
	htmllex "github.com/tdewolff/parse/v2/html"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	htmltext "html"
)

type Reference struct {
	Tag, Attribute, Value string
	Offset, End           int
	Resource              bool
}

func URLAttribute(tag, attribute string, attrs map[string]string) (bool, bool) {
	switch attribute {
	case "href", "xlink:href":
		return true, tag != "a" && tag != "area" && (tag != "link" || resourceRel(attrs["rel"]))
	case "src", "poster", "background", "manifest", "srcset", "imagesrcset":
		return true, true
	case "data":
		return tag == "object", true
	case "action", "formaction", "cite", "longdesc", "ping", "archive":
		return true, false
	case "content":
		key := strings.ToLower(attrs["property"])
		if key == "" {
			key = strings.ToLower(attrs["name"])
		}
		if tag == "meta" {
			if strings.EqualFold(attrs["http-equiv"], "refresh") {
				return true, false
			}
			switch key {
			case "fc:miniapp", "fc:frame":
				return true, false // JSON metadata; extract its URL fields below.
			case "og:url", "og:image", "og:image:url", "og:image:secure_url", "og:video", "og:video:url", "og:video:secure_url", "og:audio", "og:audio:url", "og:audio:secure_url", "twitter:url", "twitter:image", "twitter:image:src", "twitter:player", "twitter:player:stream", "snapchat:sticker":
				return true, key != "og:url" && key != "twitter:url"
			}
		}
	}
	return false, false
}
func resourceRel(rel string) bool {
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		switch token {
		case "stylesheet", "icon", "apple-touch-icon", "preload", "modulepreload", "prefetch", "preconnect", "dns-prefetch", "manifest", "discord:component-embed":
			return true
		}
	}
	return false
}

// References uses HTML tokenizers, not a URL regex. Attribute values are decoded
// exactly once. Every offset refers to the original byte slice; lex a clone because
// parse/v2 normalizes attribute names in its backing buffer.
func References(raw []byte) []Reference {
	refs, _ := scan(raw, false)
	return refs
}

// scan reads every URL reference of a document in one tokenizer pass:
// attribute URLs always, and with all also style attributes, <style> text and
// JSON script text. The second list holds those extra references, kept apart
// so AllReferences returns them after the attribute references as before.
func scan(raw []byte, all bool) (refs, extra []Reference) {
	z := html.NewTokenizer(bytes.NewReader(raw))
	offset := 0
	jsonText, styleText, platformText := false, false, false
	for {
		kind := z.Next()
		start := offset
		offset += len(z.Raw())
		if kind == html.ErrorToken {
			break
		}
		if all {
			// Raw, not Text: Text normalizes newlines, which would shift every offset.
			if kind == html.TextToken && jsonText {
				if platformText {
					extra = append(extra, platformJSONReferences(z.Raw(), start)...)
				} else {
					extra = append(extra, JSONReferences(z.Raw(), start)...)
				}
			}
			if kind == html.TextToken && styleText {
				extra = append(extra, CSSReferences(z.Raw(), start)...)
			}
			if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
				jsonText, styleText = false, false
			}
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		// Tags are lexed once more with positions. The lexer appends to its input,
		// so it gets a copy; its attributes also give the policy values, so no
		// attribute is copied out of the tokenizer.
		name, _ := z.TagName()
		tag := atom.Lookup(name).String()
		if tag == "" {
			tag = string(name)
		}
		part := bytes.Clone(z.Raw())
		if tag == "svg" || tag == "math" {
			for i := 1; i <= len(tag) && i < len(part); i++ {
				part[i] = 'x'
			}
		}
		attributes := tagAttributes(part)
		attrs := map[string]string{}
		for _, a := range attributes {
			if _, seen := attrs[a.key]; !seen && policyAttribute[a.key] {
				attrs[a.key] = a.text()
			}
		}
		if all {
			platformText = false
			jsonText = false
			styleText = tag == "style"
			if tag == "script" {
				if attrs["id"] == "discord:component-embed" {
					platformText = true
				}
				if value, ok := attrs["type"]; ok {
					value = strings.ToLower(strings.TrimSpace(value))
					jsonText = value == "application/ld+json" || value == "application/json" || value == "application/vnd.discord.component-embed+json"
				}
			}
			if value, ok := attrs["style"]; ok { // Attribute positions are tag-relative after entity decoding.
				for _, ref := range CSSReferences([]byte(value), start) {
					ref.Tag = tag
					ref.Attribute = "style CSS URL"
					ref.Offset = start
					ref.End = offset
					extra = append(extra, ref)
				}
			}
		}
		for _, a := range attributes {
			attribute := a.key
			yes, resource := URLAttribute(tag, attribute, attrs)
			if !yes {
				continue
			}
			value := a.value
			position := start + a.end - len(value)
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
				value = value[1 : len(value)-1]
				position++
			}
			text := htmltext.UnescapeString(string(value))
			ref := Reference{Tag: tag, Attribute: attribute, Value: text, Offset: position, End: position + len(value), Resource: resource}
			switch attribute {
			case "content":
				if strings.EqualFold(attrs["http-equiv"], "refresh") {
					value, offset, ok := RefreshURL(text)
					if ok {
						ref.Value = value
						ref.Offset += offset
						ref.End = ref.Offset + len(value)
						refs = append(refs, ref)
					}
				} else if key := strings.ToLower(strings.TrimSpace(attrs["name"])); key == "fc:miniapp" || key == "fc:frame" || strings.EqualFold(attrs["property"], "fc:miniapp") || strings.EqualFold(attrs["property"], "fc:frame") {
					// Attribute entity decoding changes internal byte positions. Keep
					// the containing attribute's accurate source location for each URL.
					for _, nested := range platformJSONReferences([]byte(text), 0) {
						nested.Tag, nested.Offset, nested.End = tag, position, ref.End
						nested.Attribute = "platform " + nested.Attribute
						refs = append(refs, nested)
					}
				} else {
					refs = append(refs, ref)
				}
			case "srcset", "imagesrcset":
				for _, candidate := range Candidates(text) {
					copy := ref
					copy.Value = candidate.URL
					copy.Offset += candidate.Offset
					copy.End = copy.Offset + len(candidate.URL)
					refs = append(refs, copy)
				}
			case "ping", "archive":
				for _, value := range strings.FieldsFunc(text, func(ch rune) bool { return strings.ContainsRune(" \t\r\n\f", ch) }) {
					copy := ref
					copy.Value = value
					refs = append(refs, copy)
				}
			default:
				refs = append(refs, ref)
			}
		}
	}
	return refs, extra
}

// policyAttribute names the attributes whose values decide how a tag's URLs
// are read; only these are decoded into strings.
var policyAttribute = map[string]bool{"rel": true, "name": true, "property": true, "http-equiv": true, "id": true, "type": true, "style": true}

type tagAttribute struct {
	key   string // lowercase
	value []byte // raw, with quotes
	end   int    // offset after the value within the tag
}

func (a tagAttribute) text() string {
	v := a.value
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		v = v[1 : len(v)-1]
	}
	return htmltext.UnescapeString(string(v))
}

// tagAttributes lexes one tag and returns its attributes in source order.
func tagAttributes(part []byte) []tagAttribute {
	input := parse.NewInputBytes(part)
	lexer := htmllex.NewLexer(input)
	var out []tagAttribute
	for {
		kind, _ := lexer.Next()
		if kind == htmllex.ErrorToken {
			return out
		}
		if kind == htmllex.AttributeToken {
			out = append(out, tagAttribute{strings.ToLower(string(lexer.AttrKey())), lexer.AttrVal(), input.Offset()})
		}
	}
}

type Candidate struct {
	URL, Descriptor string
	Offset          int
}

// Candidates follows HTML's URL/descriptor token boundaries, including commas
// inside data URLs and parenthesized descriptors. Descriptor conformance is left
// to a future image rule; all candidate destinations are inspected independently.
func Candidates(value string) []Candidate {
	var result []Candidate
	space := func(ch byte) bool { return strings.ContainsRune(" \t\r\n\f", rune(ch)) }
	for i := 0; i < len(value); {
		for i < len(value) && (space(value[i]) || value[i] == ',') {
			i++
		}
		if i == len(value) {
			break
		}
		start := i
		for i < len(value) && !space(value[i]) {
			i++
		}
		url := value[start:i]
		descriptor := ""
		if strings.HasSuffix(url, ",") {
			url = strings.TrimRight(url, ",")
		} else {
			begin := i
			depth := 0
			for i < len(value) {
				if value[i] == ',' && depth == 0 {
					break
				}
				if value[i] == '(' {
					depth++
				}
				if value[i] == ')' && depth > 0 {
					depth--
				}
				i++
			}
			descriptor = strings.TrimSpace(value[begin:i])
		}
		if url != "" {
			result = append(result, Candidate{url, descriptor, start})
		}
	}
	return result
}

// JSONReferences finds URL-like JSON string values. Vocabulary identifiers are
// intentionally excluded: @context/@type are schemas, not managed destinations.
// Useful for JSON-LD; malformed JSON belongs to the existing HTML JSON rule.
func JSONReferences(raw []byte, offset int) []Reference {
	return jsonReferences(raw, offset, jsonURLLike)
}

// Platform component text can contain Markdown links or prose mentioning URLs.
// Inspect structural destinations, not entire content strings as if they were URLs.
func platformJSONReferences(raw []byte, offset int) []Reference {
	return jsonReferences(raw, offset, func(key, value string) bool {
		switch strings.ToLower(key) {
		case "url", "imageurl", "splashimageurl":
			return value != ""
		}
		return false
	})
}

func jsonReferences(raw []byte, offset int, isURL func(string, string) bool) []Reference {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var refs []Reference
	var walk func(string, bool) error
	walk = func(key string, vocab bool) error {
		vocab = vocab || key == "@context" || key == "@type" || key == "@vocab"
		// InputOffset stops before the separator preceding this value.
		start := int(decoder.InputOffset())
		for start < len(raw) && strings.IndexByte(" \t\r\n,:", raw[start]) >= 0 {
			start++
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if text, ok := token.(string); ok {
			if !vocab && isURL(key, text) {
				// JSON identifiers/destinations are metadata, not resources the browser
				// automatically fetches. In particular @id fragments are not SVG/CSS
				// resource references, and HTTP entity IRIs are not mixed content.
				refs = append(refs, Reference{Tag: "script", Attribute: "JSON " + key, Value: text, Offset: offset + start, End: offset + int(decoder.InputOffset())})
			}
			return nil
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delimiter == '{' {
			for decoder.More() {
				name, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := walk(name.(string), vocab); err != nil {
					return err
				}
			}
		} else if delimiter == '[' {
			for decoder.More() {
				if err := walk(key, vocab); err != nil {
					return err
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk("", false); err != nil {
		return nil
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Offset < refs[j].Offset })
	return refs
}
func LooksURL(value string) bool { return Scheme(value) != "" || strings.HasPrefix(value, "//") }

func Apply(issues []diagnostic.Diagnostic, rules map[string]diagnostic.Severity) []diagnostic.Diagnostic {
	filtered := make([]diagnostic.Diagnostic, 0, len(issues))
	for _, d := range issues {
		if level := rules[d.Rule]; level != "" {
			d.Severity = level
		}
		if d.Severity != diagnostic.Off {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// AllReferences includes URL-like values in embedded JSON/JSON-LD in addition
// to URL-bearing HTML attributes. It never inspects executable JS as HTML.
func AllReferences(raw []byte) []Reference {
	refs, extra := scan(raw, true)
	return append(refs, extra...)
}

func jsonURLLike(key, value string) bool {
	key = strings.ToLower(key)
	if key == "@id" && strings.HasPrefix(value, "_:") {
		return false
	} // JSON-LD blank-node identifier
	scheme := Scheme(value)
	if strings.HasPrefix(value, "//") || strings.Contains(value, "://") || scheme != "" && knownSchemes[scheme] {
		return true
	}
	return value != "" && (strings.HasSuffix(key, "url") || strings.HasSuffix(key, "uri") || strings.HasSuffix(key, "href") || key == "@id" || key == "sameas" || key == "image" || key == "action")
}

// VisibleURLLiterals is an advisory for duplicated URL copy. Code samples,
// executable/structured scripts, styles and comments are intentionally excluded.
func VisibleURLLiterals(raw []byte) []int {
	z := html.NewTokenizer(bytes.NewReader(raw))
	offset, ignored := 0, 0
	ignoredTags := map[string]bool{"script": true, "style": true, "pre": true, "code": true, "samp": true, "kbd": true, "textarea": true}
	var positions []int
	for {
		kind := z.Next()
		start := offset
		offset += len(z.Raw())
		if kind == html.ErrorToken {
			break
		}
		if kind == html.StartTagToken {
			if ignoredTags[z.Token().Data] {
				ignored++
			}
		}
		if kind == html.EndTagToken {
			if ignoredTags[z.Token().Data] && ignored > 0 {
				ignored--
			}
		}
		if kind != html.TextToken || ignored > 0 {
			continue
		}
		for _, word := range strings.Fields(strings.ToLower(string(z.Text()))) {
			if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") || strings.HasPrefix(word, "www.") || strings.HasPrefix(word, "mailto:") {
				positions = append(positions, start)
				break
			}
		}
	}
	return positions
}

// RefreshURL extracts the destination using HTML's declarative refresh token
// boundaries (ASCII delay/separators, optional URL= and optional quotes).
// A delay-only refresh has no explicit destination to register.
func RefreshURL(value string) (string, int, bool) {
	space := func(ch byte) bool { return strings.ContainsRune(" \t\r\n\f", rune(ch)) }
	i := 0
	for i < len(value) && space(value[i]) {
		i++
	}
	start := i
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
	}
	if i == start && (i == len(value) || value[i] != '.') {
		return "", 0, false
	}
	for i < len(value) && (value[i] >= '0' && value[i] <= '9' || value[i] == '.') {
		i++
	}
	if i == len(value) {
		return "", 0, false
	}
	if !space(value[i]) && value[i] != ';' && value[i] != ',' {
		return "", 0, false
	}
	for i < len(value) && space(value[i]) {
		i++
	}
	if i < len(value) && (value[i] == ';' || value[i] == ',') {
		i++
	}
	for i < len(value) && space(value[i]) {
		i++
	}
	if i == len(value) {
		return "", 0, false
	}
	start = i
	// Failed URL= recognition leaves the entire string as the destination, as HTML
	// specifies; it must not silently turn a URL starting with 'u' into another path.
	if i+3 <= len(value) && strings.EqualFold(value[i:i+3], "url") {
		next := i + 3
		for next < len(value) && space(value[next]) {
			next++
		}
		if next < len(value) && value[next] == '=' {
			i = next + 1
			for i < len(value) && space(value[i]) {
				i++
			}
			start = i
		}
	}
	end := len(value)
	if start < end && (value[start] == '"' || value[start] == '\'') {
		quote := value[start]
		start++
		if at := strings.IndexByte(value[start:], quote); at >= 0 {
			end = start + at
		}
	}
	return value[start:end], start, true
}
