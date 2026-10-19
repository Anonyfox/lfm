// Package ooxml puts Office Open XML parts into the order the schemas
// require. Word and Excel refuse a file whose elements come out of order
// ("unreadable content") where LibreOffice and Pages read it silently, so
// every part lfm writes or takes from a library passes through Order.
package ooxml

import (
	"bytes"
	"slices"
	"strings"
)

// sequences are the schema orders (ECMA-376 transitional) of the elements
// lfm and its libraries write, by parent; "grandparent/parent" narrows a
// rule. A child a strict parent does not list is dropped, as the schema
// forbids it there.
var sequences = map[string][]string{
	// WordprocessingML.
	"w:pPr": {"w:pStyle", "w:keepNext", "w:keepLines", "w:pageBreakBefore", "w:framePr", "w:widowControl", "w:numPr", "w:suppressLineNumbers", "w:pBdr", "w:shd", "w:tabs",
		"w:suppressAutoHyphens", "w:kinsoku", "w:wordWrap", "w:overflowPunct", "w:topLinePunct", "w:autoSpaceDE", "w:autoSpaceDN", "w:bidi", "w:adjustRightInd", "w:snapToGrid",
		"w:spacing", "w:ind", "w:contextualSpacing", "w:mirrorIndents", "w:suppressOverlap", "w:jc", "w:textDirection", "w:textAlignment", "w:textboxTightWrap", "w:outlineLvl",
		"w:divId", "w:cnfStyle", "w:rPr", "w:sectPr", "w:pPrChange"},
	"w:rPr": {"w:rStyle", "w:rFonts", "w:b", "w:bCs", "w:i", "w:iCs", "w:caps", "w:smallCaps", "w:strike", "w:dstrike", "w:outline", "w:shadow", "w:emboss", "w:imprint",
		"w:noProof", "w:snapToGrid", "w:vanish", "w:webHidden", "w:color", "w:spacing", "w:w", "w:kern", "w:position", "w:sz", "w:szCs", "w:highlight", "w:u", "w:effect",
		"w:bdr", "w:shd", "w:fitText", "w:vertAlign", "w:rtl", "w:cs", "w:em", "w:lang", "w:eastAsianLayout", "w:specVanish", "w:oMath"},
	"w:tblPr": {"w:tblStyle", "w:tblpPr", "w:tblOverlap", "w:bidiVisual", "w:tblStyleRowBandSize", "w:tblStyleColBandSize", "w:tblW", "w:jc", "w:tblCellSpacing", "w:tblInd",
		"w:tblBorders", "w:shd", "w:tblLayout", "w:tblCellMar", "w:tblLook", "w:tblCaption", "w:tblDescription"},
	"w:tcPr":       {"w:cnfStyle", "w:tcW", "w:gridSpan", "w:hMerge", "w:vMerge", "w:tcBorders", "w:shd", "w:noWrap", "w:tcMar", "w:textDirection", "w:tcFitText", "w:vAlign", "w:hideMark"},
	"w:tblBorders": {"w:top", "w:start", "w:left", "w:bottom", "w:end", "w:right", "w:insideH", "w:insideV"},
	"w:tcBorders":  {"w:top", "w:start", "w:left", "w:bottom", "w:end", "w:right", "w:insideH", "w:insideV", "w:tl2br", "w:tr2bl"},
	"w:pBdr":       {"w:top", "w:left", "w:bottom", "w:right", "w:between", "w:bar"},
	"w:tblCellMar": {"w:top", "w:start", "w:left", "w:bottom", "w:end", "w:right"},
	"w:tcMar":      {"w:top", "w:start", "w:left", "w:bottom", "w:end", "w:right"},
	"w:sectPr": {"w:headerReference", "w:footerReference", "w:footnotePr", "w:endnotePr", "w:type", "w:pgSz", "w:pgMar", "w:paperSrc", "w:pgBorders", "w:lnNumType",
		"w:pgNumType", "w:cols", "w:formProt", "w:vAlign", "w:noEndnote", "w:titlePg", "w:textDirection", "w:bidi", "w:rtlGutter", "w:docGrid", "w:printerSettings"},
	"w:style": {"w:name", "w:aliases", "w:basedOn", "w:next", "w:link", "w:autoRedefine", "w:hidden", "w:uiPriority", "w:semiHidden", "w:unhideWhenUsed", "w:qFormat",
		"w:locked", "w:personal", "w:personalCompose", "w:personalReply", "w:rsid", "w:pPr", "w:rPr", "w:tblPr", "w:trPr", "w:tcPr", "w:tblStylePr"},
	"w:lvl": {"w:start", "w:numFmt", "w:lvlRestart", "w:pStyle", "w:isLgl", "w:suff", "w:lvlText", "w:lvlPicBulletId", "w:legacy", "w:lvlJc", "w:pPr", "w:rPr"},
	"w:settings": {"w:writeProtection", "w:view", "w:zoom", "w:removePersonalInformation", "w:removeDateAndTime", "w:doNotDisplayPageBoundaries", "w:displayBackgroundShape",
		"w:printPostScriptOverText", "w:printFractionalCharacterWidth", "w:printFormsData", "w:embedTrueTypeFonts", "w:embedSystemFonts", "w:saveSubsetFonts",
		"w:saveFormsData", "w:mirrorMargins", "w:alignBordersAndEdges", "w:bordersDoNotSurroundHeader", "w:bordersDoNotSurroundFooter", "w:gutterAtTop",
		"w:hideSpellingErrors", "w:hideGrammaticalErrors", "w:activeWritingStyle", "w:proofState", "w:formsDesign", "w:attachedTemplate", "w:linkStyles",
		"w:stylePaneFormatFilter", "w:stylePaneSortMethod", "w:documentType", "w:mailMerge", "w:revisionView", "w:trackRevisions", "w:doNotTrackMoves",
		"w:doNotTrackFormatting", "w:documentProtection", "w:autoFormatOverride", "w:styleLockTheme", "w:styleLockQFSet", "w:defaultTabStop", "w:autoHyphenation",
		"w:consecutiveHyphenLimit", "w:hyphenationZone", "w:doNotHyphenateCaps", "w:showEnvelope", "w:summaryLength", "w:clickAndTypeStyle", "w:defaultTableStyle",
		"w:evenAndOddHeaders", "w:bookFoldRevPrinting", "w:bookFoldPrinting", "w:bookFoldPrintingSheets", "w:drawingGridHorizontalSpacing", "w:drawingGridVerticalSpacing",
		"w:displayHorizontalDrawingGridEvery", "w:displayVerticalDrawingGridEvery", "w:doNotUseMarginsForDrawingGridOrigin", "w:drawingGridHorizontalOrigin",
		"w:drawingGridVerticalOrigin", "w:doNotShadeFormData", "w:noPunctuationKerning", "w:characterSpacingControl", "w:printTwoOnOne", "w:strictFirstAndLastChars",
		"w:noLineBreaksAfter", "w:noLineBreaksBefore", "w:savePreviewPicture", "w:doNotValidateAgainstSchema", "w:saveInvalidXml", "w:ignoreMixedContent",
		"w:alwaysShowPlaceholderText", "w:doNotDemarcateInvalidXml", "w:saveXmlDataOnly", "w:useXSLTWhenSaving", "w:saveThroughXslt", "w:showXMLTags",
		"w:alwaysMergeEmptyNamespace", "w:updateFields", "w:hdrShapeDefaults", "w:footnotePr", "w:endnotePr", "w:compat", "w:docVars", "w:rsids", "m:mathPr",
		"w:attachedSchema", "w:themeFontLang", "w:clrSchemeMapping", "w:doNotIncludeSubdocsInStats", "w:doNotAutoCompressPictures", "w:forceUpgrade", "w:captions",
		"w:readModeInkLockDown", "w:smartTagType", "sl:schemaLibrary", "w:shapeDefaults", "w:doNotEmbedSmartTags", "w:decimalSymbol", "w:listSeparator"},
	// SpreadsheetML fonts, as Excel writes them.
	"font": {"b", "i", "strike", "condense", "extend", "outline", "shadow", "u", "vertAlign", "sz", "color", "name", "family", "charset", "scheme"},
	// Chart series per chart type.
	"c:barChart/c:ser":      {"c:idx", "c:order", "c:tx", "c:spPr", "c:invertIfNegative", "c:pictureOptions", "c:dPt", "c:dLbls", "c:trendline", "c:errBars", "c:cat", "c:val", "c:shape", "c:extLst"},
	"c:pieChart/c:ser":      {"c:idx", "c:order", "c:tx", "c:spPr", "c:explosion", "c:dPt", "c:dLbls", "c:cat", "c:val", "c:extLst"},
	"c:doughnutChart/c:ser": {"c:idx", "c:order", "c:tx", "c:spPr", "c:explosion", "c:dPt", "c:dLbls", "c:cat", "c:val", "c:extLst"},
	"c:lineChart/c:ser":     {"c:idx", "c:order", "c:tx", "c:spPr", "c:marker", "c:dPt", "c:dLbls", "c:trendline", "c:errBars", "c:cat", "c:val", "c:smooth", "c:extLst"},
}

// strict parents drop the children their sequence does not list.
var strict = map[string]bool{"c:barChart/c:ser": true, "c:pieChart/c:ser": true, "c:doughnutChart/c:ser": true, "c:lineChart/c:ser": true}

// Charts are written with the chart namespace as a prefix or as the
// default namespace (excelize); the rules hold for both.
func init() {
	for key, seq := range sequences {
		if !strings.HasPrefix(key, "c:") {
			continue
		}
		bare := make([]string, len(seq))
		for i, name := range seq {
			bare[i] = strings.TrimPrefix(name, "c:")
		}
		k := strings.ReplaceAll(key, "c:", "")
		sequences[k] = bare
		strict[k] = strict[key]
	}
}

// node is an element: its raw start tag, children and the raw end tag;
// text and other markup are kept as raw nodes.
type node struct {
	name, start, end string
	raw              string // for text, declarations and comments
	children         []*node
}

// Order returns the part with every element's children in schema order.
// Parts that are not well-formed come back unchanged.
func Order(xml []byte) []byte {
	root, ok := parse(string(xml))
	if !ok {
		return xml
	}
	var b strings.Builder
	b.Grow(len(xml))
	for _, n := range root.children {
		reorder(n, "")
		emit(&b, n)
	}
	out := []byte(b.String())
	if bytes.Equal(out, xml) {
		return xml
	}
	return out
}

func reorder(n *node, parent string) {
	if n.name == "" {
		return
	}
	for _, c := range n.children {
		reorder(c, n.name)
	}
	seq, key := sequences[parent+"/"+n.name], parent+"/"+n.name
	if seq == nil {
		seq, key = sequences[n.name], n.name
	}
	if seq == nil {
		return
	}
	rank := func(c *node) int {
		if c.name == "" {
			return -1 // whitespace stays where it is relative to nothing
		}
		if i := slices.Index(seq, c.name); i >= 0 {
			return i
		}
		return len(seq)
	}
	kept := n.children[:0:0]
	for _, c := range n.children {
		if strict[key] && c.name != "" && !slices.Contains(seq, c.name) {
			continue
		}
		kept = append(kept, c)
	}
	slices.SortStableFunc(kept, func(a, b *node) int { return rank(a) - rank(b) })
	n.children = kept
}

func emit(b *strings.Builder, n *node) {
	if n.name == "" {
		b.WriteString(n.raw)
		return
	}
	b.WriteString(n.start)
	if n.end == "" {
		return // self-closing
	}
	for _, c := range n.children {
		emit(b, c)
	}
	b.WriteString(n.end)
}

// parse reads the simple XML Office parts are: elements, attributes, text,
// a declaration. CDATA and DTDs do not occur there.
func parse(s string) (*node, bool) {
	root := &node{name: "#root"}
	stack := []*node{root}
	for i := 0; i < len(s); {
		top := stack[len(stack)-1]
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				j = len(s) - i
			}
			top.children = append(top.children, &node{raw: s[i : i+j]})
			i += j
			continue
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			return nil, false
		}
		tag := s[i : i+j+1]
		i += j + 1
		switch {
		case strings.HasPrefix(tag, "<?") || strings.HasPrefix(tag, "<!"):
			top.children = append(top.children, &node{raw: tag})
		case strings.HasPrefix(tag, "</"):
			if len(stack) < 2 || top.name != strings.TrimSpace(tag[2:len(tag)-1]) {
				return nil, false
			}
			top.end = tag
			stack = stack[:len(stack)-1]
		default:
			name := tag[1 : len(tag)-1]
			if k := strings.IndexAny(name, " \t\r\n/"); k >= 0 {
				name = name[:k]
			}
			n := &node{name: name, start: tag}
			top.children = append(top.children, n)
			if !strings.HasSuffix(tag, "/>") {
				stack = append(stack, n)
			}
		}
	}
	return root, len(stack) == 1
}
