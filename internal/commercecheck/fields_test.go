package commercecheck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

const validDropdown = `{"key":"language","label":{"type":"custom","custom":"Language 🐺"},"type":"dropdown","optional":false,"dropdown":{"options":[{"label":"Deutsch","value":"de"},{"label":"English","value":"en"}],"default_value":"de"}}`

func fieldConfig(fields string) string {
	return `{"urls":{"buy":"https://buy.stripe.com/ABC"},"data":{"commerce":{"offers":{"a":{"sku":"a","name":"A","checkoutUrlKey":"buy","stripe":{"customFields":` + fields + `}}}}}}`
}

func TestCustomFieldLimitsShapesAndDefaults(t *testing.T) {
	for _, tc := range []struct{ input, fragment string }{
		{`null`, "must be an array"},
		{`[null]`, "object"},
		{`[` + strings.Replace(validDropdown, `"key":"language"`, `"key":"bad_key"`, 1) + `]`, "alphanumeric"},
		{`[` + strings.Replace(validDropdown, `"optional":false`, `"optional":"false"`, 1) + `]`, "JSON boolean"},
		{`[` + strings.Replace(validDropdown, `"type":"dropdown"`, `"type":"upload"`, 1) + `]`, "Unsupported custom-field"},
		{`[` + strings.Replace(validDropdown, `"type":"custom"`, `"type":"label"`, 1) + `]`, "label.type"},
		{`[` + strings.Replace(validDropdown, `"default_value":"de"`, `"default_value":"fr"`, 1) + `]`, "does not match"},
		{`[` + strings.Replace(validDropdown, `"value":"en"`, `"value":"de"`, 1) + `]`, "unique and alphanumeric"},
		{`[` + strings.Replace(validDropdown, `"value":"en"`, `"value":"en-GB"`, 1) + `]`, "alphanumeric"},
		{`[` + strings.Replace(validDropdown, `"custom":"Language 🐺"`, `"custom":"`+strings.Repeat("🐺", 101)+`"`, 1) + `]`, "100 characters"},
		{`[` + validDropdown + `,` + validDropdown + `]`, "unique and alphanumeric"},
	} {
		_, issues := checkConfig(t, fieldConfig(tc.input))
		if !found(issues, "commerce.fields", diagnostic.Error, tc.fragment) {
			t.Fatal(tc, issues)
		}
	}
	for _, kind := range []string{"text", "numeric"} {
		for _, tc := range []struct{ spec, fragment string }{
			{`{"minimum_length":10,"maximum_length":2}`, "exceeds"},
			{`{"maximum_length":256}`, "between 0 and 255"},
			{`{"minimum_length":-1}`, "between 0 and 255"},
			{`{"maximum_length":1.5}`, "integer"},
			{`{"maximum_length":2,"default_value":"123"}`, "default"},
			{`{"minimum_length":3,"default_value":"12"}`, "default"},
			{`{"default_value":123}`, "default"},
		} {
			field := `{"key":"brief","type":"` + kind + `","label":{"type":"custom","custom":"Brief"},"` + kind + `":` + tc.spec + `}`
			_, issues := checkConfig(t, fieldConfig(`[`+field+`]`))
			if !found(issues, "commerce.fields", diagnostic.Error, tc.fragment) {
				t.Fatal(kind, tc, issues)
			}
		}
	}
	field := `{"key":"code","type":"numeric","label":{"type":"custom","custom":"Code"},"numeric":{"default_value":"00123"}}`
	for _, raw := range []string{`[]`, `[` + validDropdown + `]`, `[` + field + `]`, strings.Replace(`[`+field+`]`, "00123", "", 1)} {
		if _, issues := checkConfig(t, fieldConfig(raw)); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	_, issues := checkConfig(t, fieldConfig(strings.Replace(`[`+field+`]`, "00123", "123a", 1)))
	if !found(issues, "commerce.fields", diagnostic.Error, "default") {
		t.Fatal(issues)
	}
}

func TestCustomFieldCountsAndUnicodeBoundaries(t *testing.T) {
	var field map[string]any
	json.Unmarshal([]byte(validDropdown), &field)
	for _, count := range []int{3, 4} {
		var fields []map[string]any
		for i := 0; i < count; i++ {
			copy := map[string]any{}
			for k, v := range field {
				copy[k] = v
			}
			copy["key"] = string(rune('a' + i))
			fields = append(fields, copy)
		}
		raw, _ := json.Marshal(fields)
		_, issues := checkConfig(t, fieldConfig(string(raw)))
		if found(issues, "commerce.fields", diagnostic.Error, "at most three") != (count == 4) {
			t.Fatal(count, issues)
		}
	}
	for _, count := range []int{0, 200, 201} {
		var options []map[string]any
		for i := 0; i < count; i++ {
			options = append(options, map[string]any{"label": "Choice", "value": string(rune('a'+i/26)) + string(rune('a'+i%26))})
		}
		field["dropdown"] = map[string]any{"options": options}
		raw, _ := json.Marshal([]any{field})
		_, issues := checkConfig(t, fieldConfig(string(raw)))
		if found(issues, "commerce.fields", diagnostic.Error, "1–200") != (count != 200) {
			t.Fatal(count, issues)
		}
	}
	_, issues := checkConfig(t, fieldConfig(`[`+strings.Replace(validDropdown, `Language 🐺`, strings.Repeat("🐺", 100), 1)+`]`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
}
