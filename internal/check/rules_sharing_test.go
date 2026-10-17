package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/urlcheck"
)

const validPlayer = `<head><meta name=twitter:card content=player><meta name=twitter:player content="https://example.com/play/?seed=42#result"><meta name=twitter:player:width content=640><meta name=twitter:player:height content=480><meta name=twitter:image content="https://example.com/poster.png"><meta property=og:title content="Try it"><meta property=og:description content="An interactive result"></head>`
const validDiscord = `{"component":{"type":17,"components":[{"type":9,"components":[{"type":10,"content":"Compare venues"}],"accessory":{"type":11,"media":{"url":"https://example.com/venue.webp"}}},{"type":1,"components":[{"type":2,"style":5,"label":"Compare","url":"https://example.com/?seed=42#result"}]}]}}`
const validFarcaster = `{"version":"1","imageUrl":"https://example.com/card.png","button":{"title":"Try your result","action":{"type":"launch_miniapp","url":"https://example.com/?seed=42#result"}}}`

func discordHTML(payload string) string {
	return `<head><script id="discord:component-embed" type="application/json">` + payload + `</script></head>`
}

func farcasterHTML(payload string) string {
	return `<head><meta name="fc:miniapp" content="` + html.EscapeString(payload) + `"></head>`
}

func TestExactParameterFreeCanonical(t *testing.T) {
	for _, tc := range []struct{ name, href, message string }{
		{"exact", "https://example.com/product/", ""},
		{"query", "https://example.com/product/?seed=1", "without query"},
		{"bare query", "https://example.com/product/?", "without query"},
		{"fragment", "https://example.com/product/#result", "without query"},
		{"bare fragment", "https://example.com/product/#", "without query"},
		{"other product", "https://example.com/", "differs from configured"},
		{"slash", "https://example.com/product", "differs from configured"},
		{"case normalization", "https://EXAMPLE.com/product/", "differs from configured"},
		{"port normalization", "https://example.com:443/product/", "differs from configured"},
		{"http", "http://example.com/product/", "absolute HTTPS"},
		{"relative", "/product/", "absolute HTTPS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `<html lang=en><head><title>T</title><meta name=description content=D><link rel=canonical href="` + tc.href + `"></head></html>`
			d, _ := Parse([]byte(raw), Options{URL: "https://example.com/product/"})
			issues := metadataRule(d)
			if tc.message == "" && len(issues) != 0 || tc.message != "" && !hasMessage(issues, Error, tc.message) {
				t.Fatal(tc, issues)
			}
		})
	}
	for _, raw := range []string{`<head></head>`, `<head></head><body><template><link rel=canonical href="https://example.com/"></template></body>`} {
		if issues := checkRule(t, "seo.metadata", raw); !hasMessage(issues, Error, "Canonical URL is missing") {
			t.Fatal(issues)
		}
	}
}

func TestPlayerContractAndFallback(t *testing.T) {
	if issues := checkRule(t, "sharing.x-player", validPlayer); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ raw, message string }{
		{strings.Replace(validPlayer, `<meta name=twitter:player:height content=480>`, "", 1), "missing nonempty twitter:player:height"},
		{strings.Replace(validPlayer, "content=640", "content=640px", 1), "positive integer"},
		{strings.Replace(validPlayer, "https://example.com/play/", "http://example.com/play/", 1), "public absolute HTTPS"},
		{strings.Replace(validPlayer, "https://example.com/poster.png", "data:image/png;base64,YQ==", 1), "public absolute HTTPS"},
	} {
		if issues := checkRule(t, "sharing.x-player", tc.raw); !hasMessage(issues, Error, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	if issues := checkRule(t, "sharing.x-player", `<meta name=twitter:player content="https://example.com/">`); !hasMessage(issues, Warning, "without twitter:card=player") {
		t.Fatal(issues)
	}
	if issues := checkRule(t, "sharing.x-player", `<meta name=twitter:card content=summary_large_image>`); len(issues) != 0 {
		t.Fatal("unused player integration warned", issues)
	}
}

func TestSharingDiscoveryPlacementAndScalarScope(t *testing.T) {
	for _, tc := range []struct {
		raw, message string
		level        Severity
	}{
		{`<meta name=twitter:player content="https://example.com/"><meta name=twitter:player content="https://example.com/other/">`, "Repeated scalar", Error},
		{`<body><meta name=fediverse:creator content="@fox@example.com"></body>`, "outside head", Warning},
		{`<meta property=fc:miniapp content="{}">`, "wrong metadata attribute", Warning},
		{`<meta name=twitter:label1 content=Price>`, "pair", Warning},
		{`<meta property=og:video:width content=-20>`, "positive integer", Error},
		{`<meta property=og:video content="blob:foo">`, "public absolute HTTP(S)", Error},
		{`<meta http-equiv=Content-Security-Policy content="frame-ancestors https://x.com">`, "cannot enable framing", Warning},
		{`<link rel=alternate type=application/json+oembed href="data:application/json,{}">`, "public absolute HTTP(S)", Error},
		{`<head><style>` + strings.Repeat(" ", 33*1024) + `</style><meta property=og:title content=T></head>`, "first 32 KiB", Warning},
	} {
		if issues := checkRule(t, "sharing.discovery", tc.raw); !hasMessage(issues, tc.level, tc.message) {
			t.Fatal(tc.message, issues)
		}
	}
	valid := `<head><meta property=og:image content="https://example.com/a.png"><meta property=og:image:width content=400><meta property=og:image content="https://example.com/b.png"><meta property=og:image:width content=600><meta name=twitter:label1 content=Price><meta name=twitter:data1 content="€49"><meta name=fediverse:creator content="@fox@example.com"></head><body><template><meta name=fc:miniapp content=bad></template><svg><title>Foreign title</title></svg></body>`
	if issues := checkRule(t, "sharing.discovery", valid); len(issues) != 0 {
		t.Fatal(issues)
	}
}

func TestSharingImageDescriptorScopeAndMismatch(t *testing.T) {
	valid := `<head><meta property=og:image content="https://example.com/card.png"><meta property=og:image:alt content=""><meta property=og:image:width content=600><meta property=og:image:height content=400><meta property=og:image content="https://example.com/other.png"><meta property=og:image:alt content=Other><meta property=og:image:width content=800><meta property=og:image:height content=600><meta name=twitter:image content="https://example.com/card.png"><meta name=twitter:image:alt content=Result></head>`
	d, _ := Parse([]byte(valid), Options{URL: "https://example.com/", Assets: map[string][]byte{"card.png": sharingPNG(t, 600, 400)}})
	if issues := sharingMediaRule(d); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ raw, message string }{
		{strings.Replace(valid, "content=600", "content=500", 1), "declares 500; emitted image is 600"},
		{strings.Replace(valid, `<meta property=og:image:alt content="">`, "", 1), "missing og:image:alt"},
		{strings.Replace(valid, `<meta name=twitter:image:alt content=Result>`, "", 1), "no twitter:image:alt"},
	} {
		candidate, _ := Parse([]byte(tc.raw), d.Options)
		if issues := sharingMediaRule(candidate); !hasMessage(issues, Warning, tc.message) {
			t.Fatal(tc.message, issues)
		}
	}
	if issues := checkRule(t, "sharing.discovery", `<meta http-equiv=Content-Security-Policy content="script-src 'nonce-frame-ancestors-test'">`); len(issues) != 0 {
		t.Fatal("nonce mistaken for a framing directive", issues)
	}
}

func TestDiscordSupportedComponentsAndFailures(t *testing.T) {
	if issues := checkRule(t, "sharing.discord", discordHTML(validDiscord)); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ payload, message string }{
		{strings.Replace(validDiscord, `"style":5`, `"style":1`, 1), "link style 5"},
		{strings.Replace(validDiscord, `"style":5`, `"style":5,"custom_id":"buy"`, 1), "unsupported button keys: custom_id"},
		{strings.Replace(validDiscord, `"label":"Compare"`, `"label":""`, 1), "label or emoji"},
		{strings.Replace(validDiscord, `https://example.com/venue.webp`, `data:image/png;base64,YQ==`, 1), "public absolute HTTP(S)"},
		{strings.Replace(validDiscord, `"type":17`, `"type":17,"type":10`, 1), "duplicate JSON key"},
		{`{"component":{"type":17,"components":[{"type":17,"components":[]}]}}`, "another Container"},
		{`{"component":{"type":17,"components":[{"type":99}]}}`, "unsupported component type"},
		{`{"component":{"type":17,"components":[]},"other":{}}`, "exactly one top-level"},
		{`[]`, "JSON object"},
		{`{"component":`, "invalid JSON"},
	} {
		if issues := checkRule(t, "sharing.discord", discordHTML(tc.payload)); !hasMessage(issues, Error, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	integer := strings.Replace(validDiscord, `"type":17`, `"type":17.0`, 1)
	if issues := checkRule(t, "sharing.discord", discordHTML(integer)); len(issues) != 0 {
		t.Fatal("JSON integer value rejected", issues)
	}
	if issues := checkRule(t, "sharing.discord", `<template>`+discordHTML(`{"bad":true}`)+`</template>`); len(issues) != 0 {
		t.Fatal("inert declaration validated", issues)
	}
}

func TestDiscordByteAndWholeEmbedLimits(t *testing.T) {
	for _, size := range []int{3000, 3001} {
		payload := validDiscord + strings.Repeat(" ", size-len(validDiscord))
		issues := checkRule(t, "sharing.discord", discordHTML(payload))
		if got := hasMessage(issues, Error, "maximum is 3000"); got != (size == 3001) {
			t.Fatal(size, issues)
		}
	}
	// Count encoded bytes, not decoded characters (\u003c is six bytes).
	payload := strings.Replace(validDiscord, "Compare venues", strings.Repeat(`\u003c`, 500), 1)
	if issues := checkRule(t, "sharing.discord", discordHTML(payload)); !hasMessage(issues, Error, "maximum is 3000") {
		t.Fatal(issues)
	}
	text := `{"type":10,"content":"A"}`
	payload = `{"component":{"type":17,"components":[` + strings.TrimSuffix(strings.Repeat(text+",", 40), ",") + `]}}`
	if issues := checkRule(t, "sharing.discord", discordHTML(payload)); !hasMessage(issues, Error, "maximum is 40 including the root") {
		t.Fatal(issues)
	}
	item := `{"media":{"url":"https://example.com/a.png"}}`
	gallery := `{"type":12,"items":[` + strings.TrimSuffix(strings.Repeat(item+",", 6), ",") + `]}`
	payload = `{"component":{"type":17,"components":[` + gallery + "," + gallery + `]}}`
	if issues := checkRule(t, "sharing.discord", discordHTML(payload)); !hasMessage(issues, Error, "maximum is 10 across") {
		t.Fatal(issues)
	}
}

func TestLinkedDiscordBytesHostAndReturnAdvisory(t *testing.T) {
	link := `<head><link rel=discord:component-embed type=application/json href="https://example.com/preview.json"></head>`
	d, _ := Parse([]byte(link), Options{URL: "https://example.com/", Assets: map[string][]byte{"preview.json": []byte(validDiscord)}})
	if issues := discordRule(d); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, host := range []string{"cdn.example.com", "example.com"} {
		d, _ := Parse([]byte(strings.Replace(link, "https://example.com/preview", "https://"+host+"/preview", 1)), Options{URL: "https://example.com/"})
		if issues := discordRule(d); hasMessage(issues, Error, "unrelated host") || !hasMessage(issues, Warning, "not inspected") {
			t.Fatal(host, issues)
		}
	}
	for _, host := range []string{"example.com.evil.org", "notexample.com", "sibling.example.com"} {
		d, _ := Parse([]byte(strings.Replace(link, "https://example.com/preview", "https://"+host+"/preview", 1)), Options{URL: "https://app.example.com/"})
		if issues := discordRule(d); !hasMessage(issues, Error, "unrelated host") {
			t.Fatal(host, issues)
		}
	}
	if issues := checkRule(t, "sharing.discord", discordHTML(`{"component":{"type":17,"components":[{"type":10,"content":"Result"}]}}`)); !hasMessage(issues, Warning, "no native link button") {
		t.Fatal(issues)
	}
}

func TestSharingBrowserURLRepairsNeverPanic(t *testing.T) {
	for _, value := range []string{"https://example.com/%zz?state=1", "https://example.com/?q=%", "https://bücher.de/über/?q=🦊"} {
		raw := strings.Replace(completeSocial, `content="https://example.com/"`, `content="`+value+`"`, 1)
		d, _ := Parse([]byte(raw), Options{URL: "https://example.com/"})
		_ = socialRule(d)
		link := `<head><link rel=discord:component-embed type=application/json href="` + value + `"></head>`
		d, _ = Parse([]byte(link), Options{URL: "https://example.com/"})
		_ = discordRule(d)
	}
}

func sharingPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestFarcasterContractAndEmittedImage(t *testing.T) {
	raw := []byte(farcasterHTML(validFarcaster))
	d, _ := Parse(raw, Options{URL: "https://example.com/", Assets: map[string][]byte{"card.png": sharingPNG(t, 600, 400)}})
	if issues := farcasterRule(d); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ payload, message string }{
		{strings.Replace(validFarcaster, `"version":"1"`, `"version":1`, 1), "version must be the string"},
		{strings.Replace(validFarcaster, `"title":"Try your result"`, `"title":""`, 1), "button.title"},
		{strings.Replace(validFarcaster, `"launch_miniapp"`, `"link"`, 1), "button.action.type"},
		{strings.Replace(validFarcaster, `https://example.com/card.png`, "blob:bad", 1), "public absolute HTTPS"},
		{strings.Replace(validFarcaster, `"version":"1"`, `"version":"1","version":"1"`, 1), "duplicate JSON key"},
	} {
		if issues := checkRule(t, "sharing.farcaster", farcasterHTML(tc.payload)); !hasMessage(issues, Error, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	d.Options.Assets["card.png"] = sharingPNG(t, 400, 400)
	if issues := farcasterRule(d); !hasMessage(issues, Error, "require 3:2") {
		t.Fatal(issues)
	}
	// Omitting action.url is valid: the host supplies the shared page/query.
	payload := strings.Replace(validFarcaster, `,"url":"https://example.com/?seed=42#result"`, "", 1)
	if issues := checkRule(t, "sharing.farcaster", farcasterHTML(payload)); hasMessage(issues, Error, "url") || !hasMessage(issues, Warning, "not inspected") {
		t.Fatal(issues)
	}
	if issues := checkRule(t, "sharing.farcaster", `<meta name=fc:frame content=vNext>`); len(issues) != 0 {
		t.Fatal("legacy Frames v1 misclassified", issues)
	}
}

func TestSnapchatLocalLimitsAndRemoteEvidence(t *testing.T) {
	raw := []byte(`<head><meta property=snapchat:sticker content="https://example.com/sticker.png"></head>`)
	d, _ := Parse(raw, Options{URL: "https://example.com/", Assets: map[string][]byte{"sticker.png": sharingPNG(t, 400, 400)}})
	if issues := snapchatRule(d); len(issues) != 0 {
		t.Fatal(issues)
	}
	d.Options.Assets["sticker.png"] = sharingPNG(t, 399, 400)
	if issues := snapchatRule(d); !hasMessage(issues, Error, "400×400") {
		t.Fatal(issues)
	}
	d.Options.Assets["sticker.png"] = append(sharingPNG(t, 400, 400), bytes.Repeat([]byte{' '}, 1_000_000)...)
	if issues := snapchatRule(d); !hasMessage(issues, Error, "<1 MB") {
		t.Fatal(issues)
	}
	d.Options.Assets = nil
	if issues := snapchatRule(d); !hasMessage(issues, Warning, "not inspected") {
		t.Fatal(issues)
	}
	if issues := checkRule(t, "sharing.snapchat", `<meta property=snapchat:sticker content="data:image/png;base64,YQ==">`); !hasMessage(issues, Error, "public absolute") {
		t.Fatal(issues)
	}
}

func TestSharingURLRegistryEscapesLocationsAndOverrides(t *testing.T) {
	declarations := []urlcheck.Declaration{{Path: "url", Value: "https://example.com/"}, {Path: "urls.card", Value: "https://example.com/card.png"}, {Path: "urls.result", Value: "https://example.com/?seed=42#result"}}
	raw := []byte("<head>\n" + strings.TrimSuffix(strings.TrimPrefix(farcasterHTML(validFarcaster), "<head>"), "</head>") + "</head>")
	d, _ := Parse(raw, Options{URL: "https://example.com/", Registry: urlcheck.NewRegistry("https://example.com/", declarations)})
	if issues := linksDeclaredRule(d); len(issues) != 0 {
		t.Fatal("JSON content attribute was treated as one URL", issues)
	}
	d.Options.Registry = urlcheck.NewRegistry("https://example.com/", declarations[:1])
	// linkFacts caches the analyzer, but the registry lookup remains current.
	issues := linksDeclaredRule(d)
	if len(issues) != 2 || issues[0].Location.Line != 2 {
		t.Fatal("nested URL declaration/position missing", issues)
	}
	before := bytes.Clone(raw)
	first := Run(d, DefaultRules(), nil)
	if !reflect.DeepEqual(first, Run(d, DefaultRules(), nil)) || !bytes.Equal(raw, before) {
		t.Fatal("sharing analysis mutated bytes or was nondeterministic")
	}
	override := map[string]Severity{"sharing.farcaster": Off}
	if report := Run(d, []Rule{testRule("sharing.farcaster", farcasterRule)}, override); len(report.Diagnostics) != 0 {
		t.Fatal(report)
	}
	for _, issue := range first.Diagnostics {
		if strings.HasPrefix(issue.Rule, "sharing.") && (issue.Hint == "" || issue.Location.File != "index.html") {
			t.Fatal(issue)
		}
	}
}

func BenchmarkAnalyzeSharing(b *testing.B) {
	raw := []byte(`<!doctype html><html lang=en><head><meta charset=utf-8><title>Result</title><link rel=canonical href="https://example.com/">` + strings.TrimSuffix(strings.TrimPrefix(validPlayer, "<head>"), "</head>") + strings.TrimSuffix(strings.TrimPrefix(discordHTML(validDiscord), "<head>"), "</head>") + strings.TrimSuffix(strings.TrimPrefix(farcasterHTML(validFarcaster), "<head>"), "</head>") + `</head><body><main><h1>Result</h1><p>A useful result.</p></main></body></html>`)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_, _ = Analyze(raw, Options{URL: "https://example.com/"})
	}
}

func FuzzSharingPayloads(f *testing.F) {
	for _, raw := range []string{validDiscord, validFarcaster, `{"component":{"type":17,"components":null}}`, `{"version":"1","version":"1"}`, `null`, `{"a":[[[[]]]]}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		if len(payload) > 20_000 {
			t.Skip()
		}
		for _, input := range []string{discordHTML(payload), farcasterHTML(payload)} {
			d, err := Parse([]byte(input), Options{})
			if err != nil {
				t.Fatal(err)
			}
			_ = discordRule(d)
			_ = farcasterRule(d)
		}
	})
}

func TestSharingJSONRejectsAmbiguityWithoutLeakingPayload(t *testing.T) {
	for _, input := range []string{`{"a":1,"\u0061":2}`, `{} {}`, `{"a":]}`, `"text"`, `{"a":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`} {
		if _, err := sharingJSON([]byte(input)); err == nil {
			t.Fatal("accepted ambiguous/malformed JSON", input)
		}
	}
	object, err := sharingJSON([]byte(`{"type":17e0,"text":"< & 🐺","array":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := jsonInteger(object, "type"); !ok || n != 17 {
		t.Fatal(n, ok)
	}
	encoded, _ := json.Marshal(object)
	if !json.Valid(encoded) || jsonString(object, "text") != "< & 🐺" {
		t.Fatal(fmt.Sprint(object))
	}
}
