// Package jsonldcheck delegates JSON-LD expansion to JSON-goLD. It never loads
// network or filesystem contexts and does not impose application-specific shapes.
package jsonldcheck

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/piprate/json-gold/ld"
)

type Finding struct {
	Rule, Message, Hint string
	Warning             bool
}

//go:embed schemaorg/context.jsonld
var schemaContext []byte

var contextOnce = sync.OnceValue(func() any {
	var document any
	if err := json.Unmarshal(schemaContext, &document); err != nil {
		panic("invalid embedded Schema.org context")
	}
	return document
})

// schemaTerms splits the bundled context once: string entries (@vocab and
// prefixes) always apply; object entries are term definitions.
var schemaTerms = sync.OnceValues(func() (prefixes map[string]any, terms map[string]any) {
	root, _ := contextOnce().(map[string]any)
	all, _ := root["@context"].(map[string]any)
	prefixes, terms = map[string]any{}, map[string]any{}
	for k, v := range all {
		if _, ok := v.(string); ok {
			prefixes[k] = v
		} else {
			terms[k] = v
		}
	}
	return prefixes, terms
})

// pruned returns the Schema.org context reduced to the term definitions the
// document can use: every object key and string value of the document that
// names a term, and the terms those definitions refer to, to a fixpoint.
// Expansion only consults definitions of terms that occur, so the result is
// the same as with the full context, while JSON-goLD no longer processes
// three thousand definitions per page.
func pruned(used map[string]bool) any {
	prefixes, terms := schemaTerms()
	context := make(map[string]any, len(prefixes)+len(used))
	for k, v := range prefixes {
		context[k] = v
	}
	queue := make([]string, 0, len(used))
	for k := range used {
		queue = append(queue, k)
	}
	for len(queue) > 0 {
		k := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		def, ok := terms[k]
		if !ok {
			continue
		}
		if _, done := context[k]; done {
			continue
		}
		context[k] = def
		eachString(def, func(s string) {
			if _, known := context[s]; !known {
				queue = append(queue, s)
			}
		})
	}
	return map[string]any{"@context": context}
}

// eachString calls f with every object key and string value in v.
func eachString(v any, f func(string)) {
	switch x := v.(type) {
	case string:
		f(x)
	case map[string]any:
		for k, item := range x {
			f(k)
			eachString(item, f)
		}
	case []any:
		for _, item := range x {
			eachString(item, f)
		}
	}
}

type offlineLoader struct {
	unavailable bool
	used        map[string]bool // keys and string values of the document
}

func (l *offlineLoader) LoadDocument(uri string) (*ld.RemoteDocument, error) {
	switch uri {
	case "http://schema.org", "https://schema.org", "http://schema.org/", "https://schema.org/",
		"http://schema.org/docs/jsonldcontext.json", "https://schema.org/docs/jsonldcontext.json",
		"http://schema.org/docs/jsonldcontext.jsonld", "https://schema.org/docs/jsonldcontext.jsonld":
		if l.used == nil {
			return &ld.RemoteDocument{DocumentURL: "https://schema.org/docs/jsonldcontext.jsonld", Document: ld.CloneDocument(contextOnce())}, nil
		}
		return &ld.RemoteDocument{DocumentURL: "https://schema.org/docs/jsonldcontext.jsonld", Document: pruned(l.used)}, nil
	}
	l.unavailable = true
	return nil, fmt.Errorf("context is unavailable offline")
}

// Check accepts exact script text. Serialization/expansion are separate: no
// normalization from the processor is ever written back into the shipped page.
func Check(raw []byte, base string) []Finding {
	_, findings := Expand(raw, base)
	return findings
}

// Expand exposes the same cached-by-caller offline expansion for focused
// semantic rules. An unavailable/invalid context yields no usable graph.
func Expand(raw []byte, base string) (expanded []any, findings []Finding) {
	return expand(raw, base, false)
}

// expand with fullContext processes the complete Schema.org context; tests use
// it to prove the pruned context expands identically.
func expand(raw []byte, base string, fullContext bool) (expanded []any, findings []Finding) {
	defer func() {
		if recover() != nil {
			expanded = nil
			findings = append(findings, Finding{Rule: "jsonld.structure", Message: "JSON-LD processor could not safely analyze this document.", Hint: "Reduce the document to a minimal example and report the processor limitation; no output was published."})
		}
	}()
	var input any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil || !json.Valid(raw) {
		return nil, nil // html.json owns invalid JSON syntax; avoid duplicate messages
	}
	switch input.(type) {
	case map[string]any, []any:
	default:
		return nil, []Finding{{Rule: "jsonld.structure", Message: "JSON-LD root must be an object or array.", Hint: "Describe node objects; omit the optional jsonld field to disable the block."}}
	}
	if !bounded(input, 0, new(int)) {
		return nil, []Finding{{Rule: "jsonld.structure", Message: "JSON-LD exceeds depth 128 or 100,000 values.", Hint: "Reduce nesting/data volume so expansion remains bounded."}}
	}
	if unexpandable(input) {
		return nil, []Finding{{Rule: "jsonld.structure", Message: "JSON-LD contains an empty property name or a malformed @protected/@container value.", Hint: "Name every property; @protected must be true or false and @container a keyword string or array of them."}}
	}
	used := map[string]bool{}
	eachString(input, func(s string) { used[s] = true })
	loader := &offlineLoader{used: used}
	if fullContext {
		loader.used = nil
	}
	options := ld.NewJsonLdOptions(base)
	options.DocumentLoader = loader
	options.Ordered = true  // stable diagnostic order, independent of Go map order
	options.SafeMode = true // detect property loss; retry spec-normal expansion below
	processor := ld.NewJsonLdProcessor()
	expanded, err := processor.Expand(input, options)
	var processing *ld.JsonLdError
	if errors.As(err, &processing) && processing.Code == ld.InvalidProperty {
		findings = append(findings, Finding{Rule: "jsonld.dropped", Warning: true, Message: "JSON-LD contains a property that expansion would discard.", Hint: "Check the active @context and property spelling. Null term mappings can intentionally discard properties; disable this advisory when deliberate."})
		options.SafeMode = false
		expanded, err = processor.Expand(input, options)
	}
	if loader.unavailable {
		return nil, append(findings, Finding{Rule: "jsonld.context", Warning: true, Message: "JSON-LD expansion was skipped: a remote context is not bundled.", Hint: "Builds never fetch contexts. Schema.org is bundled; inline other contexts to validate them locally. This is a coverage warning, not a claim that the context is invalid."})
	}
	if err != nil {
		code := "processing error"
		if errors.As(err, &processing) {
			code = string(processing.Code)
		}
		return nil, append(findings, Finding{Rule: "jsonld.structure", Message: "Invalid JSON-LD: " + code + ".", Hint: "Check JSON-LD keyword shapes, value/list objects and @context definitions; a valid JSON document can still be invalid JSON-LD."})
	}
	if len(expanded) == 0 {
		findings = append(findings, Finding{Rule: "jsonld.dropped", Warning: true, Message: "JSON-LD expands to no node data.", Hint: "Add meaningful node properties with a context or explicit IRIs, or omit the empty block."})
	}
	return expanded, findings
}

func bounded(value any, depth int, count *int) bool {
	*count += 1
	if depth > 128 || *count > 100000 {
		return false
	}
	switch v := value.(type) {
	case map[string]any:
		for _, item := range v {
			if !bounded(item, depth+1, count) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !bounded(item, depth+1, count) {
				return false
			}
		}
	}
	return true
}

// unexpandable rejects shapes JSON-goLD mishandles instead of reporting them:
// with no @type alias it reads an empty key as @type, and it type-asserts
// @protected and @container values without checking. Runs after bounded, so
// its recursion depth is limited.
func unexpandable(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "" || key == "@protected" && !isBool(item) || key == "@container" && !stringOrStrings(item) || unexpandable(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if unexpandable(item) {
				return true
			}
		}
	}
	return false
}

func isBool(value any) bool { _, ok := value.(bool); return ok }

func stringOrStrings(value any) bool {
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}
