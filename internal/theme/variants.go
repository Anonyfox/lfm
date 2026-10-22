package theme

// Variants are Looks that between them use every typesetting mechanism a
// concept decides: lfm's tests build every file format in each. The faces
// are the test fixtures' own; a product names its families.
var Variants = map[string]string{
	"base":            "Text font: Lora",
	"indented":        "Text font: Lora\nParagraphs: indented\nHeadings: regular\nText size: 11.5\nLine height: 1.35\nOrnament: ❧\nAccent: #7A2E2E",
	"hairline":        "Text font: Lora\nTables: hairline\nText size: 10\nLine height: 1.45\nHeading scale: 1.2\nAccent: #1F4FD1",
	"monochrome":      "Text font: Lora\nTables: hairline\nText size: 10\nLine height: 1.5\nHeading scale: 1.125",
	"accent-headings": "Text font: Lora\nHeadings: regular\nHeading color: accent\nText size: 11\nLine height: 1.45\nOrnament: ❦\nAccent: #9C4A2C",
	"grid":            "Text font: Lora\nTables: grid\nText size: 10\nLine height: 1.4\nHeading scale: 1.2\nOrnament: ■\nAccent: #2F4A6D",
}
