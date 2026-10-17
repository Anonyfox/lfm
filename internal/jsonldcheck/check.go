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

type offlineLoader struct{ unavailable bool }

func (l *offlineLoader) LoadDocument(uri string) (*ld.RemoteDocument, error) {
	switch uri {
	case "http://schema.org", "https://schema.org", "http://schema.org/", "https://schema.org/",
		"http://schema.org/docs/jsonldcontext.json", "https://schema.org/docs/jsonldcontext.json",
		"http://schema.org/docs/jsonldcontext.jsonld", "https://schema.org/docs/jsonldcontext.jsonld":
		return &ld.RemoteDocument{DocumentURL: "https://schema.org/docs/jsonldcontext.jsonld", Document: ld.CloneDocument(contextOnce())}, nil
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
	loader := &offlineLoader{}
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
