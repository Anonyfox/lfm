package ooxml

import "testing"

func TestOrderPutsChildrenInSchemaOrder(t *testing.T) {
	for in, want := range map[string]string{
		// pPr: spacing before ind, jc after both.
		`<w:p><w:pPr><w:jc w:val="center"/><w:ind w:left="1"/><w:spacing w:after="2"/></w:pPr></w:p>`: `<w:p><w:pPr><w:spacing w:after="2"/><w:ind w:left="1"/><w:jc w:val="center"/></w:pPr></w:p>`,
		// tblPr and its borders.
		`<w:tblPr><w:tblLayout w:type="fixed"/><w:tblBorders><w:bottom/><w:top/><w:left/></w:tblBorders><w:tblW w:w="1"/></w:tblPr>`: `<w:tblPr><w:tblW w:w="1"/><w:tblBorders><w:top/><w:left/><w:bottom/></w:tblBorders><w:tblLayout w:type="fixed"/></w:tblPr>`,
		// A doughnut series takes no invertIfNegative, prefixed or not.
		`<doughnutChart><ser><idx val="0"></idx><invertIfNegative val="0"></invertIfNegative><val></val></ser></doughnutChart>`: `<doughnutChart><ser><idx val="0"></idx><val></val></ser></doughnutChart>`,
		// SpreadsheetML fonts as Excel writes them; unknown parents stay as they are.
		`<font><color rgb="1"/><b/><sz val="10"/></font><other><z/><a/></other>`: `<font><b/><sz val="10"/><color rgb="1"/></font><other><z/><a/></other>`,
	} {
		if got := string(Order([]byte(in))); got != want {
			t.Errorf("Order(%s)\n got %s\nwant %s", in, got, want)
		}
	}
	broken := []byte(`<a><b></a>`)
	if got := Order(broken); string(got) != string(broken) {
		t.Error("a part that is not well-formed must come back unchanged")
	}
}
