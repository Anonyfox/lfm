package templatecheck

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
)

// HelperHelp enumerates the same functions used by parsing and execution.
func HelperHelp(custom template.FuncMap) string {
	descriptions := map[string]string{
		"dict":   `Build a map from string-key/value pairs. {{template "case" (dict "root" $ "case" .)}}`,
		"script": `Bundle a local script or installed package. {{script "./main.js"}}`,
		"style":  `Bundle a local stylesheet or installed package. {{style "./main.css"}}`,
		"eq":     `Compare values, including exact JSON numbers. {{if eq .data.count 2}}two{{end}}`,
		"ne":     `Test unequal values. {{if ne .data.count 0}}available{{end}}`,
		"lt":     `Test less than. {{if lt .data.count 10}}few{{end}}`,
		"le":     `Test less than or equal. {{if le .data.count 10}}few{{end}}`,
		"gt":     `Test greater than. {{if gt .data.count 0}}available{{end}}`,
		"ge":     `Test greater than or equal. {{if ge .data.count 10}}many{{end}}`,
	}
	var names []string
	for name := range assetFunctions(custom) {
		if !strings.HasPrefix(name, "__lfm") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out strings.Builder
	out.WriteString("Templates use Go html/template and its standard builtins (index, len, printf, and, or, not, etc.).\nAdditional helpers and numeric comparisons:\n\n")
	for _, name := range names {
		fmt.Fprintf(&out, "  %s — %s\n", name, descriptions[name])
	}
	out.WriteString("\n.urls is the named URL data map, not a function: {{.urls.checkout}}.\nPass multiple contexts with {{template \"case\" (dict \"root\" $ \"case\" .)}}.\nOptional paths: {{with .data.note}}{{.}}{{end}} or {{if .data.note}}{{.data.note}}{{end}}.\nA direct if/with field selection may be absent, null or empty; unguarded output must exist.\n")
	return out.String()
}
