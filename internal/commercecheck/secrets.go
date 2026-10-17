package commercecheck

import (
	"bytes"
	"fmt"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/templatecheck"
)

// Secret recognizes only concrete Stripe secret-key shapes, never publishable
// keys, public object IDs, customer field names or generic words like token.
func Secret(value string) bool { return SecretOffset([]byte(value)) >= 0 }

func SecretOffset(raw []byte) int {
	first := -1
	for _, prefix := range []string{"sk_live_", "sk_test_", "rk_live_", "rk_test_", "whsec_"} {
		at := 0
		for at < len(raw) {
			found := bytes.Index(raw[at:], []byte(prefix))
			if found < 0 {
				break
			}
			at += found
			end := at + len(prefix)
			for end < len(raw) && (raw[end] >= 'A' && raw[end] <= 'Z' || raw[end] >= 'a' && raw[end] <= 'z' || raw[end] >= '0' && raw[end] <= '9') {
				end++
			}
			if end-at-len(prefix) >= 16 {
				if first < 0 || at < first {
					first = at
				}
				break
			}
			at = end
		}
	}
	return first
}

func ConfigSecrets(values map[string]any, sources templatecheck.Sources) []diagnostic.Diagnostic {
	var issues []diagnostic.Diagnostic
	var walk func(any, string)
	walk = func(value any, path string) {
		if text, ok := value.(string); ok && Secret(text) {
			at, _ := sources.Nearest(path)
			if Secret(path) {
				path = "(redacted config path)"
			}
			issues = append(issues, diagnostic.Diagnostic{Rule: "commerce.secrets", Severity: diagnostic.Error, Location: at, Path: path, Message: "Config contains a Stripe secret-key-shaped value.", Hint: "Keep Stripe API/webhook secrets in the operator environment; public offer config needs only public object IDs and hosted URLs. The value is intentionally redacted."})
		}
		switch object := value.(type) {
		case map[string]any:
			for _, key := range keys(object) {
				walk(object[key], templatecheck.Child(path, key))
			}
		case []any:
			for i, item := range object {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(values, "")
	return issues
}
