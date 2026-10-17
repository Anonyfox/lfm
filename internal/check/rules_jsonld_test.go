package check

import (
	"bytes"
	"testing"
)

func TestJSONLDExactBytesAndIndividualRules(t *testing.T) {
	raw := []byte("<!doctype html><body>\n<script type=\"application/ld+json\">{\"@context\":\"https://schema.org\",\"@type\":42}</script></body>")
	original := bytes.Clone(raw)
	doc, err := Parse(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	issues := Run(doc, []Rule{{"jsonld.structure", "", jsonldRule("jsonld.structure")}}, nil).Diagnostics
	if len(issues) != 1 || issues[0].Location.Line != 2 || issues[0].Location.File != "index.html" || issues[0].Severity != Error {
		t.Fatal(issues)
	}
	if !bytes.Equal(raw, original) {
		t.Fatal("JSON-LD analysis mutated artifact")
	}
	if len(checkRule(t, "jsonld.structure", `<template><script type="application/ld+json">{"@type":42}</script></template>`)) != 0 {
		t.Fatal("inert JSON-LD analyzed")
	}
	if len(checkRule(t, "jsonld.block", `<script type="application/ld+json">{}</script><script type="application/LD+JSON">{}</script>`)) != 1 {
		t.Fatal("duplicate blocks missed")
	}
	if len(checkRule(t, "jsonld.structure", `<script type="application/ld+json">{"@context":"https://schema.org","@type":"WebPage","name":"Value"}</script>`)) != 0 {
		t.Fatal("valid JSON-LD rejected")
	}
}
