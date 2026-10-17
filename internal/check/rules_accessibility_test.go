package check

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestAccessibleNativeAndCustomNames(t *testing.T) {
	for _, raw := range []string{
		`<div role=button tabindex=0>Calculate</div>`,
		`<div role="unknown button" tabindex=0><img alt=Calculate></div>`,
		`<div role=switch tabindex=0 aria-checked=false aria-label=Notifications></div>`,
		`<div role=textbox tabindex=0 aria-labelledby=label></div><span id=label hidden>Amount</span>`,
		`<dialog aria-labelledby=title><h2 id=title>Review</h2><button>Close</button></dialog>`,
		`<iframe title="Venue map"></iframe>`,
		`<label for=p>Completion</label><progress id=p max=100 value=30></progress>`,
		`<details><summary>What is included?</summary>Answer</details>`,
		`<span id=label><img alt=Amount></span><input aria-labelledby=label>`,
		`<span id=label aria-label=Amount></span><input aria-labelledby=label>`,
		`<input id=source aria-label=Source value=Units><input aria-labelledby=source>`,
		`<label for=target aria-label=Amount></label><input id=target>`,
		`<svg role=img><title>Cost breakdown</title></svg>`,
		`<area href=x alt=Venue>`, `<template><button></button><img></template>`,
		`<label for=one>Amount<input id=one></label>`,
		`<span id=hidden hidden>Amount <span hidden>in euros</span></span><input aria-labelledby=hidden>`,
	} {
		if issues := checkRule(t, "a11y.controls", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{
		`<div role=button tabindex=0></div>`, `<div role=textbox tabindex=0>Current value</div>`, `<input type=submit value="">`,
		`<button role=combobox>Selected value</button>`, `<iframe></iframe>`, `<dialog><h2>Unassociated title</h2></dialog>`,
		`<details><summary> </summary>Answer</details>`, `<svg role=img></svg>`,
		`<label for=other>Wrong label<input id=target></label><input id=other aria-label=Other>`,
		`<span id=label><script>not a label</script></span><input aria-labelledby=label>`,
	} {
		if issues := checkRule(t, "a11y.controls", raw); len(issues) != 1 {
			t.Fatal(raw, issues)
		}
	}
	if issues := checkRule(t, "a11y.images", `<template><img></template><img alt="">`); len(issues) != 0 {
		t.Fatal(issues)
	}
}

func TestNativeRelationships(t *testing.T) {
	for _, raw := range []string{
		`<label for=x>Amount</label><input id=x>`, `<label>Amount<input></label>`,
		`<label for=x>Custom</label><custom-input id=x></custom-input>`,
		`<input form=f list=options><form id=f></form><datalist id=options></datalist>`,
		`<button popovertarget=p>Open</button><div id=p popover>Info</div>`,
		`<table><tr><th id=h scope=col>Price</th><td headers=h>49</td></tr></table>`,
		`<label>Amount<input><template><input></template></label>`,
		`<button commandfor=p command=--custom>Go</button><div id=p>Target</div>`,
	} {
		if issues := checkRule(t, "a11y.relationships", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{
		`<label for=x>Amount</label><div id=x></div>`, `<label for=x>Hidden</label><input id=x type=hidden>`,
		`<label>Two<input><select></select></label>`,
		`<label for=elsewhere>Wrong<input id=inside></label><input id=elsewhere>`,
		`<input form=x><div id=x></div>`, `<input list=x><div id=x></div>`,
		`<button popovertarget=x>Open</button><div id=x></div>`,
		`<table><tr><td id=x>49</td><td headers=x>Price</td></tr></table>`,
		`<table><tr><th id=x>Elsewhere</th></tr></table><table><tr><td headers=x>49</td></tr></table>`,
	} {
		issues := checkRule(t, "a11y.relationships", raw)
		if len(issues) != 1 || issues[0].Severity != Error || issues[0].Hint == "" {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{`<button popovertarget=missing>Open</button>`, `<button commandfor=missing command=show-modal>Open</button>`, `<input aria-errormessage=missing>`, `<div aria-details=missing></div>`, `<div aria-flowto=missing></div>`} {
		if issues := checkRule(t, "html.references", raw); len(issues) != 1 {
			t.Fatal(raw, issues)
		}
	}
}

func TestFocusOrderAndHiddenCandidates(t *testing.T) {
	for _, raw := range []string{
		`<button tabindex=0>Go</button><h2 tabindex=-1>Section</h2>`,
		`<div role=button tabindex=-1>Roving/programmatic</div>`,
		`<div role=button aria-disabled=true>Unavailable action</div>`,
		`<div aria-hidden=true hidden><button>Inactive</button></div>`,
		`<div aria-hidden=true inert><a href=x>Inactive</a></div>`,
		`<div aria-hidden=true><button disabled>Disabled</button></div>`,
		`<fieldset disabled aria-hidden=true><legend>Group</legend><input></fieldset>`,
		`<dialog aria-hidden=true><button>Closed</button></dialog>`,
		`<details><summary>Open</summary><div aria-hidden=true><button>Closed</button></div></details>`,
		`<div aria-hidden=true><button tabindex=-1>Not in tab order</button></div>`,
		`<div onclick="delegate(event)"><button>Actual control</button></div>`,
		`<label onclick="track()">Amount<input></label>`,
		`<div role=listbox tabindex=0 aria-activedescendant=choice><div id=choice role=radio aria-checked=false>Choice</div></div>`,
		`<template><button tabindex=3 autofocus></button></template>`,
		`<input autofocus><dialog><button autofocus>OK</button></dialog><div popover><input autofocus></div>`,
	} {
		if issues := checkRule(t, "a11y.focus", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct{ raw, message string }{
		{`<button tabindex=2>Go</button>`, "Positive tabindex"},
		{`<button tabindex=1.5>Go</button>`, "not a valid integer"},
		{`<div aria-hidden=true><a href=x>Go</a></div>`, "inside aria-hidden"},
		{`<div aria-hidden=true><div aria-hidden=false><button>Go</button></div></div>`, "inside aria-hidden"},
		{`<fieldset disabled aria-hidden=true><legend><button>Enabled legend control</button></legend><input></fieldset>`, "inside aria-hidden"},
		{`<div role=button>Go</div>`, "no static focus entry"},
		{`<span onclick="go()">Go</span>`, "non-semantic control"},
		{`<input autofocus><button autofocus>Go</button>`, "Multiple autofocus"},
	} {
		issues := checkRule(t, "a11y.focus", tc.raw)
		if len(issues) != 1 || !strings.Contains(issues[0].Message, tc.message) || issues[0].Hint == "" {
			t.Fatal(tc, issues)
		}
	}
}

func TestFocusedARIAContracts(t *testing.T) {
	for _, raw := range []string{
		`<div aria-live=polite aria-atomic=true aria-expanded=false></div>`,
		`<div role=checkbox aria-checked=mixed></div>`, `<input type=checkbox role=switch>`,
		`<input type=range role=slider>`, `<select role=combobox></select>`,
		`<div role=combobox aria-expanded=false></div>`,
		`<div role=slider aria-valuenow=50></div>`, `<div role=spinbutton></div>`,
		`<div role=progressbar aria-label=Loading></div>`, // Indeterminate is meaningful.
		`<div aria-current=chapter aria-invalid=custom-error aria-live="" aria-future=custom></div>`,
		`<template><div role=slider aria-valuenow=bad></div></template>`,
	} {
		if issues := checkRule(t, "a11y.aria", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct{ raw, message string }{
		{`<div aria-live=loud></div>`, "Invalid token"},
		{`<div aria-disabled=disabled></div>`, "Invalid token"},
		{`<div role=checkbox></div>`, "checked state"},
		{`<div role=switch aria-checked=mixed></div>`, "mixed checked state"},
		{`<div role=radio aria-checked=undefined></div>`, "checked state"},
		{`<div role=slider></div>`, "current numeric value"},
		{`<div role=combobox></div>`, "expanded/collapsed"},
		{`<div role=combobox aria-expanded=true></div>`, "controlled popup"},
		{`<div aria-activedescendant="one two"></div>`, "one active element"},
		{`<div aria-valuenow=NaN></div>`, "finite decimal"},
		{`<div aria-valuenow=0x1p2></div>`, "finite decimal"},
		{`<div aria-valuemin=10 aria-valuemax=2></div>`, "minimum exceeds"},
		{`<div role=slider aria-valuenow=101></div>`, "outside its declared/default range"},
		{`<div role=slider aria-valuemin="" aria-valuenow=-1></div>`, "outside its declared/default range"},
	} {
		if issues := checkRule(t, "a11y.aria", tc.raw); len(issues) != 1 || !strings.Contains(issues[0].Message, tc.message) {
			t.Fatal(tc, issues)
		}
	}
}

func TestFormIntentAndNumericInputs(t *testing.T) {
	for _, raw := range []string{
		`<form><button type=button>Calculate</button><button type=submit>Send</button></form>`,
		`<button>Outside a form</button>`, `<form><button commandfor=d command=show-modal>Open</button></form><dialog id=d></dialog>`,
		`<input type=number min=-1.5 max=1e3 step=.1 value=0>`, `<input type=number step=any value="">`,
		`<input type=range value=50>`, `<input type=range min=10 max=20 value=15>`,
		`<template><form><button>Inert</button></form></template>`,
	} {
		if issues := checkRule(t, "ux.forms", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct{ raw, message string }{
		{`<form><button>Next</button></form>`, "implicit submit"},
		{`<button form=f>Next</button><form id=f></form>`, "implicit submit"},
		{`<form><button type=buton>Next</button></form>`, "unrecognized"},
		{`<input type=number min=5 max=1>`, "minimum exceeds"},
		{`<input type=number min=1 value=0>`, "starts outside"},
		{`<input type=range value=101>`, "starts outside"},
		{`<input type=number step=0>`, "positive step"},
		{`<input type=number value="1,5">`, "HTML number"},
		{`<input type=number min=NaN>`, "HTML number"},
		{`<input type=number value=+1>`, "HTML number"},
	} {
		if issues := checkRule(t, "ux.forms", tc.raw); len(issues) != 1 || !strings.Contains(issues[0].Message, tc.message) {
			t.Fatal(tc, issues)
		}
	}
}

func TestNativeDisclosureAndMedia(t *testing.T) {
	for _, raw := range []string{
		`<details><!-- first element --><summary>Question</summary><p>Answer</p></details>`,
		`<button commandfor=d command=show-modal>Open</button><dialog id=d></dialog>`,
		`<button commandfor=p command=toggle-popover>Open</button><div id=p popover></div>`,
		`<button commandfor=p command=--custom>Open</button><div id=p></div>`,
		`<template><details><summary>One</summary><summary>Two</summary></details></template>`,
	} {
		if issues := checkRule(t, "ux.disclosures", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct {
		raw, message string
		level        Severity
	}{
		{`<details>Answer</details>`, "no authored summary", Warning},
		{`<details><p>Answer</p><summary>Question</summary></details>`, "not the first element", Error},
		{`<details><summary>One</summary><summary>Two</summary></details>`, "multiple summary", Error},
		{`<button commandfor=p command=show-modal>Open</button><div id=p></div>`, "wrong kind", Error},
	} {
		issues := checkRule(t, "ux.disclosures", tc.raw)
		if len(issues) != 1 || issues[0].Severity != tc.level || !strings.Contains(issues[0].Message, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	for _, raw := range []string{`<video autoplay muted loop></video>`, `<video controls></video>`, `<audio></audio>`, `<template><audio autoplay></audio></template>`} {
		if issues := checkRule(t, "ux.media", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{`<audio autoplay controls></audio>`, `<video autoplay></video>`, `<audio autoplay="false"></audio>`} {
		if issues := checkRule(t, "ux.media", raw); len(issues) != 1 {
			t.Fatal(raw, issues)
		}
	}
}

func TestAccessibilityReportLocationsImmutabilityAndOverrides(t *testing.T) {
	raw := []byte("<main>\n<div aria-hidden=true><button>Go</button></div>\n<label for=x>Amount</label><div id=x></div>\n</main>")
	d, _ := Parse(raw, Options{})
	before := bytes.Clone(raw)
	rules := []Rule{testRule("a11y.focus", focusRule), testRule("a11y.relationships", relationshipsRule)}
	first := Run(d, rules, nil)
	if first.Errors != 1 || first.Warnings != 1 || !reflect.DeepEqual(first, Run(d, rules, nil)) || !bytes.Equal(before, raw) {
		t.Fatal(first)
	}
	for _, issue := range first.Diagnostics {
		if issue.Location.File != "index.html" || issue.Location.Line < 2 || issue.Hint == "" {
			t.Fatal(issue)
		}
	}
	if first.Diagnostics[1].Location.Line != 2 || len(first.Diagnostics[1].Related) != 1 {
		t.Fatal(first)
	}
	if r := Run(d, rules, map[string]Severity{"a11y.focus": Off, "a11y.relationships": Warning}); r.Errors != 0 || r.Warnings != 1 {
		t.Fatal(r)
	}
}

func BenchmarkAnalyzeAccessibleForms(b *testing.B) {
	var body strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&body, `<fieldset><legend>Scenario %d</legend><label for=n%d>Guests</label><input id=n%d type=number min=1 max=1000 value=20><button type=button>Calculate</button><details><summary>Method</summary><p>Static answer</p></details></fieldset>`, i, i, i)
	}
	raw := []byte(`<!doctype html><html lang=en>` + completeSocial + `<body><main><h1>Calculator</h1><form>` + body.String() + `</form></main></body></html>`)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_, _ = Analyze(raw, Options{URL: "https://example.com/"})
	}
}
