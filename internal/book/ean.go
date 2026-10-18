package book

import (
	"fmt"
	"strings"
)

// EAN-13 digit patterns: L and G code the left half, chosen by the first
// digit; R codes the right half.
var (
	eanL      = [10]string{"0001101", "0011001", "0010011", "0111101", "0100011", "0110001", "0101111", "0111011", "0110111", "0001011"}
	eanG      = [10]string{"0100111", "0110011", "0011011", "0100001", "0011101", "0111001", "0000101", "0010001", "0001001", "0010111"}
	eanR      = [10]string{"1110010", "1100110", "1101100", "1000010", "1011100", "1001110", "1010000", "1000100", "1001000", "1110100"}
	eanParity = [10]string{"LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG", "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL"}
)

// barcodeSVG draws the EAN-13 barcode of an ISBN with the ISBN printed above
// and the digits below, sized for the back cover at 1.8 by 1 inch.
func barcodeSVG(isbn string) string {
	digits := strings.NewReplacer("-", "", " ", "").Replace(isbn)
	if !ValidISBN(digits) {
		return ""
	}
	d := func(i int) int { return int(digits[i] - '0') }
	bits := "101"
	for i := 1; i <= 6; i++ {
		if eanParity[d(0)][i-1] == 'L' {
			bits += eanL[d(i)]
		} else {
			bits += eanG[d(i)]
		}
	}
	bits += "01010"
	for i := 7; i <= 12; i++ {
		bits += eanR[d(i)]
	}
	bits += "101"
	// 95 modules plus quiet zones of 11 and 7 modules.
	const quietLeft, width, height, bar = 11, 113, 70, 50
	guard := func(i int) bool { return i < 3 || (i >= 45 && i < 50) || i >= 92 }
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="100%%" height="100%%" font-family="Helvetica, Arial, sans-serif"><rect width="%d" height="%d" fill="#fff"/>`, width, height, width, height)
	fmt.Fprintf(&b, `<text x="%d" y="8" font-size="6.5" text-anchor="middle">ISBN %s</text>`, quietLeft+47, isbn)
	for i, c := range bits {
		if c != '1' {
			continue
		}
		h := bar
		if guard(i) {
			h = bar + 5
		}
		fmt.Fprintf(&b, `<rect x="%d" y="11" width="1" height="%d" fill="#000"/>`, quietLeft+i, h)
	}
	fmt.Fprintf(&b, `<text x="%d" y="68" font-size="8" text-anchor="end">%s</text>`, quietLeft-2, digits[:1])
	fmt.Fprintf(&b, `<text x="%d" y="68" font-size="8" text-anchor="middle" letter-spacing="1.6">%s</text>`, quietLeft+24, digits[1:7])
	fmt.Fprintf(&b, `<text x="%d" y="68" font-size="8" text-anchor="middle" letter-spacing="1.6">%s</text>`, quietLeft+70, digits[7:])
	b.WriteString(`</svg>`)
	return b.String()
}
