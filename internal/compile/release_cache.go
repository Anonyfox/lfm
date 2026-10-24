package compile

import (
	"bytes"
	"strings"

	"github.com/Anonyfox/lfm/internal/release"
	"golang.org/x/net/html"
)

type cachedOptimization struct {
	Page   []byte
	Assets map[string][]byte
	Result *release.Result
}

// Cache the optimization inputs, before critical splitting or image hints, so
// the next identical compile does not need Chrome to reconstruct the final page.
func (b *builder) optimizeCached(doc *html.Node, result *Result) error {
	return b.optimizeWithCache(doc, result, b.optimize)
}

func (b *builder) optimizeWithCache(doc *html.Node, result *Result, run func(*html.Node, *Result) error) error {
	var input bytes.Buffer
	if err := html.Render(&input, doc); err != nil {
		return err
	}
	assets := b.sidecars()
	public, err := b.publicFiles()
	if err != nil {
		return err
	}
	for name, raw := range public {
		assets[name] = raw
	}
	key := release.CacheKey(b.ctx, release.Target{Page: input.Bytes(), Assets: assets}, b.root)
	if key != "" {
		key += "-optimize"
	}
	var entry cachedOptimization
	if date := release.ReadCache(key, &entry); !date.IsZero() && entry.Result != nil && len(entry.Page) > 0 {
		parsed, err := html.Parse(bytes.NewReader(entry.Page))
		if err == nil {
			*doc = *parsed
			for c := doc.FirstChild; c != nil; c = c.NextSibling {
				c.Parent = doc
			}
			for name, raw := range entry.Assets {
				index := strings.ToLower(name)
				old, ok := b.files[index]
				if !ok {
					old = emitted{name: name, origin: "release cache"}
				}
				old.contents = raw
				b.files[index] = old
			}
			result.Release = entry.Result
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
	if result.Release != nil && len(b.notices) == before && !strings.HasPrefix(result.Release.Reason, "Critical CSS skipped:") {
		changed := map[string][]byte{}
		for name, raw := range b.sidecars() {
			if previous, ok := beforeAssets[name]; !ok || !bytes.Equal(raw, previous) {
				changed[name] = raw
			}
		}
		release.WriteCache(key, cachedOptimization{output.Bytes(), changed, result.Release})
	}
	return nil
}
