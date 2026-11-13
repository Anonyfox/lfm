package compile

import (
	"bytes"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/release"
	"golang.org/x/net/html"
)

type cachedOptimization struct {
	Page    []byte
	Assets  map[string][]byte
	Result  *release.Result
	Notices []diagnostic.Diagnostic
	Origins map[string]string
}

// Cache the optimization inputs, before critical splitting or image hints, so
// the next identical compile does not need Chrome to reconstruct the final page.
func (b *builder) optimizeWithCache(doc *html.Node, result *Result, run func(*html.Node, *Result) error) error {
	var input bytes.Buffer
	if err := html.Render(&input, doc); err != nil {
		return err
	}
	assets := b.sidecars()
	public := b.public
	var err error
	if public == nil {
		public, err = b.publicFiles()
	}
	if err != nil {
		return err
	}
	for name, raw := range public {
		assets[name] = raw
	}
	key := release.CacheKey(b.ctx, release.Target{Page: input.Bytes(), Assets: assets, Tools: b.tools}, b.root)
	if key != "" {
		key += "-optimize-v2"
	}
	var entry cachedOptimization
	if date := release.ReadCache(key, &entry); !date.IsZero() && entry.Result != nil && len(entry.Page) > 0 {
		parsed, err := html.Parse(bytes.NewReader(entry.Page))
		if err == nil {
			replaceDocument(doc, parsed)
			for name, raw := range entry.Assets {
				index := strings.ToLower(name)
				old, ok := b.files[index]
				if !ok {
					old = emitted{name: name, origin: entry.Origins[name]}
				}
				old.contents = raw
				b.files[index] = old
			}
			result.Release = entry.Result
			b.notices = append(b.notices, entry.Notices...)
			return nil
		}
	}
	beforeAssets := b.sidecars()
	before := len(b.notices)
	if err := run(doc, result); err != nil {
		return err
	}
	var output bytes.Buffer
	if err := html.Render(&output, doc); err != nil {
		return err
	}
	// A cache hit continues from the parsed page, and parsing can reorder
	// attributes; continue from the same parse so both paths emit equal bytes.
	parsed, err := html.Parse(bytes.NewReader(output.Bytes()))
	if err != nil {
		return err
	}
	replaceDocument(doc, parsed)
	for _, notice := range b.notices[before:] {
		if notice.Severity == diagnostic.Error {
			return nil // Failed extraction must retry rather than cache a tool failure.
		}
	}
	if result.Release != nil {
		changed := map[string][]byte{}
		// Origins drive CSS/JS budget accounting as well as output bytes.
		origins := map[string]string{}
		for name, raw := range b.sidecars() {
			if previous, ok := beforeAssets[name]; !ok || !bytes.Equal(raw, previous) {
				changed[name] = raw
				origins[name] = b.files[strings.ToLower(name)].origin
			}
		}
		release.WriteCache(key, cachedOptimization{output.Bytes(), changed, result.Release, b.notices[before:], origins})
	}
	return nil
}

func replaceDocument(doc, parsed *html.Node) {
	*doc = *parsed
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		c.Parent = doc
	}
}
