package compile

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/htmlx"
)

func TestCanonicalFinalGateAndConfigBase(t *testing.T) {
	root := fixture(t)
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"state": "https://example.com/tools/demo/?seed=42", "other": "https://example.com/other/"}
	})
	for _, page := range []string{
		strings.Replace(fixtureHTML, `<link rel="canonical" href="{{.url}}">`, "", 1),
		strings.Replace(fixtureHTML, `href="{{.url}}"`, `href="{{.urls.state}}"`, 1),
		strings.Replace(fixtureHTML, `href="{{.url}}"`, `href="{{.urls.other}}"`, 1),
		strings.Replace(fixtureHTML, "</head>", `<link rel=canonical href="{{.url}}"></head>`, 1),
	} {
		write(t, root, "index.html.tmpl", page)
		r, err := Compile(root, Options{Publish: true})
		if err == nil || r.Published || r.Validation == nil || read(t, root, "output/index.html") != before {
			t.Fatal("canonical failure replaced output", err)
		}
		found := false
		for _, issue := range r.Validation.Diagnostics {
			if issue.Rule == "seo.metadata" && issue.Severity == check.Error && issue.Hint != "" {
				found = true
			}
		}
		if !found {
			t.Fatal(r.Validation)
		}
	}
	write(t, root, "index.html.tmpl", fixtureHTML)
	for _, base := range []string{"https://example.com/tools/demo/?", "https://example.com/tools/demo/#", "https://example.com/tools/demo/?state=1", "http://example.com/tools/demo/"} {
		changeConfig(t, root, func(c map[string]any) { c["url"] = base })
		if r, err := Compile(root, Options{Publish: true}); err == nil || r.Published || r.Validation.Stage != "template" || read(t, root, "output/index.html") != before {
			t.Fatal("invalid configured base reached publication", base, err)
		}
	}
}

func writeSharingPNG(t *testing.T, root, name string, width, height int) {
	t.Helper()
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	write(t, root, name, raw.String())
}

const sharingHead = `<meta name="twitter:card" content="player">
<meta name="twitter:player" content="{{.urls.player}}">
<meta name="twitter:player:width" content="640"><meta name="twitter:player:height" content="480">
<meta name="twitter:image" content="card.png">
<meta name="twitter:image:alt" content="Your comparison result">
<meta name="twitter:title" content="Try the calculator"><meta name="twitter:description" content="Compare your result">
<meta property="snapchat:sticker" content="sticker.png">
<meta name="fc:miniapp" content='{"version":"1","imageUrl":"{{.urls.card}}","button":{"title":"Compare","action":{"type":"launch_miniapp","url":"{{.urls.result}}"}}}'>
<script id="discord:component-embed" type="application/json">{"component":{"type":17,"components":[{"type":10,"content":"[Compare your result]({{.urls.result}})"},{"type":1,"components":[{"type":2,"style":5,"label":"Compare","url":"{{.urls.result}}"}]}]}}</script>`

func sharingWorkbench(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	writeSharingPNG(t, root, "card.png", 600, 400)
	writeSharingPNG(t, root, "sticker.png", 400, 400)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{
			"player": "https://example.com/play/?seed=42#result",
			"card":   "https://example.com/tools/demo/card.png",
			"result": "https://example.com/tools/demo/?seed=42&ref=card#result",
		}
	})
	page := strings.Replace(fixtureHTML, `<meta property="og:image" content="assets/images/picture.svg">`, `<meta property="og:image" content="card.png"><meta property="og:image:width" content="600"><meta property="og:image:height" content="400"><meta property="og:image:alt" content="Your comparison result">`, 1)
	write(t, root, "index.html.tmpl", strings.Replace(page, "</head>", sharingHead+"</head>", 1))
	return root
}

func TestSharingFinalBytesSidecarsAndFailureAggregation(t *testing.T) {
	root := sharingWorkbench(t)
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	for _, issue := range r.Validation.Diagnostics {
		if strings.HasPrefix(issue.Rule, "sharing.") {
			t.Fatal("complete static integration warned", issue)
		}
	}
	before := read(t, root, "output/index.html")
	for _, node := range parsePage(t, before) {
		if node.Data == "meta" && (htmlx.Attr(node, "property") == "snapchat:sticker" || htmlx.Attr(node, "name") == "twitter:image") && !strings.HasPrefix(htmlx.Attr(node, "content"), "https://example.com/tools/demo/") {
			t.Fatal("platform media not emitted with public URLs", node.Attr)
		}
		if node.Data == "script" && htmlx.Attr(node, "id") == "discord:component-embed" && (node.Parent == nil || node.Parent.Data != "head") {
			t.Fatal("component JSON was moved with executable scripts")
		}
	}
	writeSharingPNG(t, root, "sticker.png", 200, 200)
	page := read(t, root, "index.html.tmpl")
	page = strings.Replace(page, `content="640"`, `content="0"`, 1)
	page = strings.Replace(page, `"style":5`, `"style":1`, 1)
	page = strings.Replace(page, `"version":"1"`, `"version":1`, 1)
	write(t, root, "index.html.tmpl", page)
	r, err = Compile(root, Options{Publish: true})
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("sharing rejection replaced output", err)
	}
	for _, rule := range []string{"sharing.x-player", "sharing.discord", "sharing.farcaster", "sharing.snapchat"} {
		found := false
		for _, issue := range r.Validation.Diagnostics {
			if issue.Rule == rule && issue.Severity == check.Error && issue.Hint != "" && issue.Location.File == "index.html" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing aggregated platform finding", rule, r.Validation)
		}
	}
	// Same existing rule overrides, with no additional syntax or CLI flags.
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"sharing.x-player": "off", "sharing.discord": "warn", "sharing.farcaster": "off", "sharing.snapchat": "off"}}
	})
	if r, err = Compile(root, Options{Publish: true}); err != nil || !r.Published {
		t.Fatal("existing overrides did not control sharing findings", err, r.Validation)
	}
}

func TestVendorDiscordTemplateEscapingHasActionableFeedback(t *testing.T) {
	root := sharingWorkbench(t)
	page := strings.Replace(read(t, root, "index.html.tmpl"), `id="discord:component-embed" type="application/json"`, `id="discord:component-embed" type="application/vnd.discord.component-embed+json"`, 1)
	write(t, root, "index.html.tmpl", page)
	r, err := Compile(root, Options{})
	if err == nil {
		t.Fatal("raw JSON URL entity escaping passed")
	}
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "sharing.discord" && strings.Contains(issue.Hint, "type=application/json") {
			return
		}
	}
	t.Fatal("missing precise JSON template repair", r.Validation)
}

func TestLinkedDiscordFileIsInspectedWithoutFetch(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"result": "https://example.com/tools/demo/?seed=42"}
	})
	write(t, root, "assets/preview.json", `{"component":{"type":17,"components":[{"type":1,"components":[{"type":2,"style":5,"label":"Try","url":"https://example.com/tools/demo/?seed=42"}]}]}}`)
	page := strings.Replace(fixtureHTML, "</head>", `<link rel="discord:component-embed" type="application/json" href="assets/preview.json"></head>`, 1)
	write(t, root, "index.html.tmpl", page)
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "sharing.discord" {
			t.Fatal(issue)
		}
	}
	before := read(t, root, "output/index.html")
	write(t, root, "assets/preview.json", `{"component":{},"component":{}}`)
	r, err = Compile(root, Options{Publish: true})
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("linked JSON was not gated", err)
	}
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "sharing.discord" && len(issue.Related) > 0 && strings.HasSuffix(issue.Related[0].File, ".json") {
			return
		}
	}
	t.Fatal("linked JSON source missing", r.Validation)
}

func TestStructuredSharingMediaFollowsAutomaticSidecarNames(t *testing.T) {
	root := sharingWorkbench(t)
	changeConfig(t, root, func(c map[string]any) {
		delete(c["build"].(map[string]any), "assets")
		c["urls"].(map[string]any)["card"] = "card.png"
	})
	page := read(t, root, "index.html.tmpl")
	page = strings.Replace(page, `{"type":10,"content":"[Compare`, `{"type":12,"items":[{"media":{"url":"{{.urls.card}}"}}]},{"type":10,"content":"[Compare`, 1)
	write(t, root, "index.html.tmpl", page)
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	for _, issue := range r.Validation.Diagnostics {
		if strings.HasPrefix(issue.Rule, "sharing.") {
			t.Fatal(issue)
		}
	}
	card := ""
	for _, n := range parsePage(t, read(t, root, "output/index.html")) {
		if n.Data == "meta" && htmlx.Attr(n, "name") == "fc:miniapp" {
			var object map[string]any
			if err := json.Unmarshal([]byte(htmlx.Attr(n, "content")), &object); err != nil {
				t.Fatal(err)
			}
			card, _ = object["imageUrl"].(string)
		}
	}
	if !strings.HasPrefix(card, "https://example.com/tools/demo/card-") || !strings.HasSuffix(card, ".png") {
		t.Fatal("structured metadata did not follow automatic packing", card)
	}
}
