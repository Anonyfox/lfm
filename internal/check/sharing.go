package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
	"golang.org/x/net/html"
)

func platformMeta(d *Document, key string) []*html.Node {
	var nodes []*html.Node
	for _, n := range d.Elements {
		if n.Namespace == "" && n.Data == "meta" && !d.inTemplate(n) && (strings.EqualFold(strings.TrimSpace(Attr(n, "name")), key) || strings.EqualFold(strings.TrimSpace(Attr(n, "property")), key)) {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func metaValue(d *Document, key string) string {
	nodes := platformMeta(d, key)
	if len(nodes) == 0 {
		return ""
	}
	return Attr(nodes[0], "content")
}

// Navigation may deliberately contain an anchor; externally fetched media may not.
// URL syntax/publicness is established locally, never DNS or host acceptance.
func sharingURL(d *Document, n *html.Node, value, label string, https, navigation bool) []Diagnostic {
	checked := value
	if navigation {
		checked, _, _ = strings.Cut(value, "#")
	}
	if !publicURL(checked) || https && !strings.HasPrefix(value, "https://") {
		protocol := "HTTP(S)"
		if https {
			protocol = "HTTPS"
		}
		return []Diagnostic{d.metadataURLAt(n, value, Error, label+" needs a public absolute "+protocol+" URL.", "Use a named lfm.json URL without credentials/private hosts; fetched media needs a public file URL, never data:/blob:. Keep public state in the actual launch/return target.")}
	}
	return nil
}

// Asset lookup uses the exact emitted output, not arbitrary workbench files or
// remote requests. Queries can version a static sidecar; fragments do not change it.
func sharingAsset(d *Document, value string) ([]byte, string, bool) {
	_, analyzer := d.linkFacts()
	u, err := analyzer.Resolve(value)
	base, baseErr := analyzer.Resolve(".")
	if err != nil || baseErr != nil || u.Scheme() != base.Scheme() || u.Host() != base.Host() {
		return nil, "", false
	}
	path, err := url.PathUnescape(u.Pathname())
	directory, dirErr := url.PathUnescape(base.Pathname())
	if err != nil || dirErr != nil || !strings.HasPrefix(path, directory) {
		return nil, "", false
	}
	name := strings.TrimPrefix(path, directory)
	raw, ok := d.Options.Assets[name]
	return raw, name, ok
}

func sharingImage(d *Document, n *html.Node, value, label string, check func(imaging.Info) string) []Diagnostic {
	raw, name, ok := sharingAsset(d, value)
	if !ok {
		return []Diagnostic{d.At(n, Warning, label+" media bytes were not inspected (not an emitted sidecar).", "Verify the deployed file's format, dimensions and byte limit in the target client; normal builds never fetch remote media.")}
	}
	info := imaging.Inspect(raw)
	if problem := check(info); problem != "" {
		return []Diagnostic{d.At(n, Error, label+" "+name+": "+problem, "Re-export the source for this platform and rebuild; this check reads bounded metadata, not rendered pixels or transparency/animation.")}
	}
	if info.Issue != "" || info.Unknown != "" {
		return []Diagnostic{d.At(n, Warning, label+" "+name+": dimensions could not be verified.", "Re-export with normalized supported metadata or inspect the actual platform preview; no pixels are decoded.")}
	}
	return nil
}

// DecodeSharingJSON decodes a sharing payload. Known schemas need shallow
// objects. Reject ambiguous duplicate keys instead of
// silently applying encoding/json's last-key-wins behavior. The depth bound is
// above every supported schema and keeps malformed payload work bounded.
// DecodeSharingJSON also lets asset packing inspect a platform payload without
// repairing ambiguous JSON before the final quality gate sees it.
func DecodeSharingJSON(raw []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 64 {
			return nil, fmt.Errorf("nesting exceeds 64 levels; flatten the platform payload")
		}
		token, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON near byte %d", d.InputOffset())
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return token, nil
		}
		if delim == '{' {
			object := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, fmt.Errorf("invalid JSON object near byte %d", d.InputOffset())
				}
				name, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("object key must be a string")
				}
				if _, exists := object[name]; exists {
					return nil, fmt.Errorf("duplicate JSON key %q", short(name))
				}
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				object[name] = value
			}
			if _, err := d.Token(); err != nil {
				return nil, fmt.Errorf("unclosed JSON object near byte %d", d.InputOffset())
			}
			return object, nil
		}
		if delim == '[' {
			var array []any
			for d.More() {
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if _, err := d.Token(); err != nil {
				return nil, fmt.Errorf("unclosed JSON array near byte %d", d.InputOffset())
			}
			return array, nil
		}
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("expected one JSON value")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("payload must be a JSON object")
	}
	return object, nil
}

func sharingJSON(raw []byte) (map[string]any, error) { return DecodeSharingJSON(raw) }

func jsonString(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func jsonInteger(object map[string]any, key string) (int64, bool) {
	number, ok := object[key].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err == nil {
		return value, true
	}
	// JSON has one number type: 17.0 and 17e0 are also integer values. The
	// inspected schema discriminators are small; never round large integers.
	f, err := number.Float64()
	if err == nil && !math.IsInf(f, 0) && f == math.Trunc(f) && f >= -1<<31 && f <= 1<<31-1 {
		return int64(f), true
	}
	return 0, false
}
