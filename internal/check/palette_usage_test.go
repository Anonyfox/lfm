package check

import "testing"

func TestPaletteUsedCustomProperties(t *testing.T) {
	const tokens = `<style>:root{--bs-primary:#0d6efd;--bs-indigo:#6366f1;--alias:var(--bs-primary);--cycle:var(--cycle)} .absent{color:var(--bs-primary)}</style>`
	expectDesign(t, "design.palette", 0, tokens+`<p>Plain</p>`)
	expectDesign(t, "design.palette", 1, tokens+`<p style="color:var(--bs-primary)">Used</p>`)
	expectDesign(t, "design.palette", 1, tokens+`<style>p{color:var(--alias)}</style><p>Used indirectly</p>`)
	expectDesign(t, "design.palette", 0, tokens+`<style>p{color:var(--cycle)}</style><p>Cycle</p>`)
	expectDesign(t, "design.palette", 1, `<style>:root{--unused:#6366f1} p{color:var(--missing,#0d6efd)}</style><p>Fallback</p>`)
}
