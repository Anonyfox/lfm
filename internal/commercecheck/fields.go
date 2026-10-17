package commercecheck

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// The optional customFields snapshot uses Stripe's native custom_fields object
// members. Unknown keys are retained; this is not a complete Stripe API schema.
func (c *checker) fields(raw any, path string) {
	fields, ok := raw.([]any)
	if !ok {
		c.add("commerce.fields", diagnostic.Error, path, "customFields must be an array.", "Copy the Payment Link's custom_fields array, or omit this optional snapshot.")
		return
	}
	if len(fields) > 3 {
		c.add("commerce.fields", diagnostic.Error, path, "Stripe supports at most three custom fields per checkout.", "Use Stripe's native email/name/address collection for those inputs; keep at most three delivery-critical custom fields.")
	}
	seen := map[string]bool{}
	for i, raw := range fields {
		child := fmt.Sprintf("%s[%d]", path, i)
		field := c.object(raw, child, "commerce.fields")
		if field == nil {
			continue
		}
		if key, valid := c.text(field["key"], child+".key", "commerce.fields", 200); valid {
			if !alphanumeric.MatchString(key) || seen[key] {
				c.add("commerce.fields", diagnostic.Error, child+".key", "Custom-field keys must be unique and alphanumeric.", "Use stable ASCII letters/digits, up to 200 characters; labels and keys have separate roles.")
			}
			seen[key] = true
		}
		label := c.object(field["label"], child+".label", "commerce.fields")
		if label != nil {
			c.text(label["custom"], child+".label.custom", "commerce.fields", 100)
			if label["type"] != "custom" {
				c.add("commerce.fields", diagnostic.Error, child+".label.type", "Custom-field label.type must be custom.", "Use Stripe's native label object: type=custom and nonempty custom text.")
			}
		}
		if optional, exists := field["optional"]; exists {
			if _, ok := optional.(bool); !ok {
				c.add("commerce.fields", diagnostic.Error, child+".optional", "Custom-field optional must be a JSON boolean.", "false requires an answer; omission defaults to false in Stripe.")
			}
		}
		kind, _ := field["type"].(string)
		switch kind {
		case "text", "numeric":
			if raw, exists := field[kind]; exists && raw != nil {
				c.fieldInput(raw, child+"."+kind, kind == "numeric")
			}
		case "dropdown":
			c.dropdown(field["dropdown"], child+".dropdown")
		default:
			c.add("commerce.fields", diagnostic.Error, child+".type", "Unsupported custom-field type.", "Stripe currently documents text, numeric and dropdown. Verify a newly supported type before overriding this check; file uploads/long briefs use the promised mailbox flow.")
		}
		for _, other := range []string{"text", "numeric", "dropdown"} {
			if value, exists := field[other]; exists && value != nil && other != kind {
				c.add("commerce.fields", diagnostic.Warning, child+"."+other, "Custom-field settings do not match its type.", "Keep the selected type's settings; remove a stale alternative configuration unless the provider requires it.")
			}
		}
	}
}

func (c *checker) fieldInput(raw any, path string, numeric bool) {
	input := c.object(raw, path, "commerce.fields")
	if input == nil {
		return
	}
	minimum, maximum := int64(0), int64(255)
	for _, bound := range []struct {
		name string
		out  *int64
	}{{"minimum_length", &minimum}, {"maximum_length", &maximum}} {
		if raw, exists := input[bound.name]; exists && raw != nil {
			value, valid := integer(raw)
			if !valid || value < 0 || value > 255 {
				c.add("commerce.fields", diagnostic.Error, path+"."+bound.name, "Custom-field length must be an integer between 0 and 255.", "Use character/digit counts supported by Stripe; this is a short checkout field, not a long brief.")
			} else {
				*bound.out = value
			}
		}
	}
	if minimum > maximum {
		c.add("commerce.fields", diagnostic.Error, path, "Custom-field minimum_length exceeds maximum_length.", "Keep ordered length bounds so an answer can satisfy them.")
	}
	if raw, exists := input["default_value"]; exists && raw != nil {
		value, ok := raw.(string)
		length := int64(utf8.RuneCountInString(value))
		if !ok || length > 255 || length > maximum || value != "" && length < minimum || numeric && strings.ContainsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
			c.add("commerce.fields", diagnostic.Error, path+".default_value", "Custom-field default does not satisfy its type/length bounds.", "Use a string within the declared bounds; numeric defaults contain only ASCII digits and may retain leading zeros. Empty prefill remains allowed.")
		}
	}
}

func (c *checker) dropdown(raw any, path string) {
	dropdown := c.object(raw, path, "commerce.fields")
	if dropdown == nil {
		return
	}
	options, ok := dropdown["options"].([]any)
	if !ok || len(options) < 1 || len(options) > 200 {
		c.add("commerce.fields", diagnostic.Error, path+".options", "Dropdown requires 1–200 options in an API-maintained Payment Link.", "Provide labelled options; the Dashboard editor supports at most 10, while the API supports up to 200.")
		return
	}
	seen := map[string]bool{}
	for i, raw := range options {
		child := fmt.Sprintf("%s.options[%d]", path, i)
		option := c.object(raw, child, "commerce.fields")
		if option == nil {
			continue
		}
		c.text(option["label"], child+".label", "commerce.fields", 100)
		if value, valid := c.text(option["value"], child+".value", "commerce.fields", 100); valid {
			if !alphanumeric.MatchString(value) || seen[value] {
				c.add("commerce.fields", diagnostic.Error, child+".value", "Dropdown option values must be unique and alphanumeric.", "Use stable ASCII letters/digits, at most 100 characters; labels may use natural language/emojis.")
			}
			seen[value] = true
		}
	}
	if raw, exists := dropdown["default_value"]; exists && raw != nil {
		value, ok := raw.(string)
		if !ok || !seen[value] {
			c.add("commerce.fields", diagnostic.Error, path+".default_value", "Dropdown default does not match an option value.", "Select an exact options[].value or omit the default.")
		}
	}
}
