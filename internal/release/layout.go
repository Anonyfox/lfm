package release

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/dom"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

//go:embed layout.js
var layoutProbe string

// Screen is one screen size of the continuum review.
type Screen struct {
	Width, Height int
	Mobile        bool
}

func (v Screen) String() string { return fmt.Sprintf("%d×%d", v.Width, v.Height) }

// Screens are the sizes every page is reviewed at: a common phone, a short
// laptop screen and a desktop. Frames step by half a viewport, so every part of
// the page appears in two frames.
var Screens = []Screen{{390, 844, true}, {1280, 680, false}, {1440, 900, false}}

// maxRecheck bounds the elements rechecked in fine steps per screen.
const maxRecheck = 12

// maxFrames bounds one viewport's walk; longer pages are sampled to the end at
// a wider step.
const maxFrames = 48

// LayoutResult is what the frame walk found.
type LayoutResult struct {
	Frames   string          `json:"frames,omitempty"` // folder of the frame screenshots
	Count    int             `json:"count"`
	Previous bool            `json:"previous"`          // a former run left frames to compare with
	Changed  []string        `json:"changed,omitempty"` // frames that differ from the former run, without .jpg
	Findings []LayoutFinding `json:"findings,omitempty"`
}

// LayoutFinding is one layout defect at one viewport.
type LayoutFinding struct {
	Screen  string `json:"viewport"`
	Kind    string `json:"kind"` // overflow, empty, split
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// signaturesFile records what each screen size showed, as element hashes, to
// tell the next run which frames show something new.
const signaturesFile = "signatures.json"

// FramesPath is the screenshot folder of a project, in the system temp
// directory like the Lighthouse report, so the project tree stays clean.
func FramesPath(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(os.TempDir(), fmt.Sprintf("lfm-frames-%x", sum[:6]))
}

type frameState struct {
	Legible   bool     `json:"legible"`
	Faint     bool     `json:"faint"`
	Y         float64  `json:"y"`
	Signature []string `json:"signature"`
	Seen      []int    `json:"seen"`
	Covered   []struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
		By   string `json:"by"`
	} `json:"covered"`
}

type pageSize struct {
	Height, Width, VW, VH float64
}

type overflowState struct {
	Width    float64 `json:"width"`
	Culprits []struct {
		Selector string `json:"selector"`
		Right    int    `json:"right"`
		Left     int    `json:"left"`
	} `json:"culprits"`
}

type splitValue struct {
	Text, Selector string
}

// auditLayout walks the page at every viewport in an already open tab,
// writing one screenshot per frame into dir (emptied first; "" skips them).
func auditLayout(tab context.Context, url, dir string) (*LayoutResult, error) {
	r := &LayoutResult{Frames: dir}
	// what each screen showed in the former run, as element hashes
	before, after := map[string][]string{}, map[string][]string{}
	if dir != "" {
		if raw, err := os.ReadFile(filepath.Join(dir, signaturesFile)); err == nil && json.Unmarshal(raw, &before) == nil {
			r.Previous = len(before) > 0
		}
		defer func() {
			if raw, err := json.Marshal(after); err == nil && dir != "" {
				_ = os.WriteFile(filepath.Join(dir, signaturesFile), raw, 0o644)
			}
		}()
		if err := resetFrames(dir); err != nil {
			r.Frames = ""
			dir = ""
		}
	}
	// The screens are walked in parallel, each in its own tab of the browser,
	// and merged in screen order, so the result is the same as one walk after
	// the other. Reduced motion and scripts off get tabs of their own.
	results := make([]screenWalk, len(Screens))
	var moving []string
	var noScript []LayoutFinding
	var motionErr, scriptErr error
	var wg sync.WaitGroup
	for i, v := range Screens {
		wg.Go(func() {
			t, done := tab, func() {}
			if i > 0 {
				t, done = chromedp.NewContext(tab)
			}
			defer done()
			results[i], results[i].err = walkScreen(t, url, dir, v, before[v.String()], r.Previous)
			if i == len(Screens)-1 && results[i].err == nil {
				// fonts are read on the widest screen, where the most text shows
				results[i].fonts, results[i].fontsErr = fallbackFonts(t)
			}
		})
	}
	wg.Go(func() { moving, motionErr = reducedMotion(tab, url, Screens[0]) })
	wg.Go(func() {
		t, done := chromedp.NewContext(tab)
		defer done()
		noScript, scriptErr = withoutScripts(t, url, Screens[0])
	})
	wg.Wait()
	var sizes, contrasts perElement
	for i, v := range Screens {
		w := results[i]
		r.Findings = append(r.Findings, w.findings...)
		r.Count += w.count
		r.Changed = append(r.Changed, w.changed...)
		if len(w.after) > 0 {
			after[v.String()] = w.after
		}
		for _, o := range w.sizes {
			sizes.add(v, o.element, o.text)
		}
		for _, o := range w.contrasts {
			contrasts.add(v, o.element, o.text)
		}
		if w.err != nil {
			return r, w.err
		}
	}
	// a failed font report is reported and skipped, never a silent pass
	if last := results[len(results)-1]; last.fontsErr == nil {
		r.Findings = append(r.Findings, last.fonts...)
	} else {
		r.Findings = append(r.Findings, LayoutFinding{"", "incomplete", "The font check could not run: " + compact(last.fontsErr.Error(), 300), "Rerun --release; if it persists, report it with the page."})
	}
	if motionErr != nil {
		return r, motionErr
	} else if len(moving) > 0 {
		r.Findings = append(r.Findings, LayoutFinding{Screens[0].String(), "motion", fmt.Sprintf("With reduced motion requested, the page still moves at %s: %s.", Screens[0], strings.Join(moving, "; ")),
			"Inside @media (prefers-reduced-motion: reduce) stop loops and long moves (animation: none, or a short fade), and let scripts check matchMedia(\"(prefers-reduced-motion: reduce)\") before animating."})
	}
	if scriptErr != nil {
		return r, scriptErr
	}
	r.Findings = append(r.Findings, noScript...)
	r.Findings = append(r.Findings, sizes.findings("small", "Text too small to read: %s.", "Set text at 9 px or more where it renders, including SVG labels scaled down with their drawing; small print reads well from 12 px.")...)
	r.Findings = append(r.Findings, contrasts.findings("contrast", "Text below the WCAG contrast minimum: %s.", "Darken the text or lighten its ground until it reaches the ratio; large text (24 px, or 18.7 px bold) needs 3:1, other text 4.5:1.")...)
	return r, nil
}

// screenWalk is what one screen's walk found.
type screenWalk struct {
	findings         []LayoutFinding
	count            int
	changed, after   []string
	sizes, contrasts []elementNote
	fonts            []LayoutFinding
	err, fontsErr    error
}

type elementNote struct{ element, text string }

// walkScreen scrolls the page at screen v frame by frame. before holds the
// element hashes this screen showed in the former run.
func walkScreen(tab context.Context, url, dir string, v Screen, before []string, previous bool) (screenWalk, error) {
	var out screenWalk
	opts := []chromedp.EmulateViewportOption{}
	if v.Mobile {
		opts = append(opts, chromedp.EmulateMobile, chromedp.EmulateTouch)
	}
	if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(v.Width), int64(v.Height), opts...), chromedp.Navigate(url)); err != nil {
		return out, err
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
		return out, err
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](layoutProbe)); err != nil {
		return out, err
	}
	size, err := chromedp.Run(tab, chromedp.Evaluate[pageSize](`(()=>{const s=__lfmLayout.prepare();return {Height:s.height,Width:s.width,VW:s.vw,VH:s.vh}})()`))
	if err != nil {
		return out, err
	}
	step := v.Height / 2
	if last := int(size.Height) - v.Height; last > 0 && last/step+1 > maxFrames {
		step = last/(maxFrames-1) + 1
	}
	known := map[string]bool{}
	for _, h := range before {
		known[h] = true
	}
	var empty []frameState
	readable := map[int]bool{}
	type cover struct {
		text, by string
		from, to int
	}
	covered := map[int]*cover{}
	var order []int
	for i, y := 0, 0; ; i, y = i+1, y+step {
		f, err := chromedp.Run(tab, chromedp.Evaluate[frameState](fmt.Sprintf(`__lfmLayout.frame(%d)`, y), chromedp.EvalAwaitPromise))
		if err != nil {
			return out, err
		}
		if i == 0 {
			first, err := chromedp.Run(tab, chromedp.Evaluate[struct{ Heading, Control bool }](`(()=>{const f=__lfmLayout.firstScreen();return {Heading:f.heading,Control:f.control}})()`))
			if err != nil {
				return out, err
			}
			if !first.Heading {
				out.findings = append(out.findings, LayoutFinding{v.String(), "first-screen", fmt.Sprintf("At %s the first screen shows no legible main heading.", v), "Let the h1 that says what the page offers appear, readable, before any scrolling at every size."})
			}
			if !first.Control {
				out.findings = append(out.findings, LayoutFinding{v.String(), "first-screen", fmt.Sprintf("At %s the first screen offers no link, button or field.", v), "Give the first screen a way to act: the tool's first input, the answer link or the way to the offer."})
			}
		}
		if !f.Legible {
			empty = append(empty, f)
		}
		for _, id := range f.Seen {
			readable[id] = true
		}
		for _, c := range f.Covered {
			if covered[c.ID] == nil {
				covered[c.ID] = &cover{c.Text, c.By, int(f.Y), int(f.Y) + v.Height}
				order = append(order, c.ID)
			}
			covered[c.ID].to = int(f.Y) + v.Height
		}
		if dir != "" {
			quality, fast := int64(80), true
			shot, err := chromedp.Call(tab, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatJpeg, Quality: &quality, OptimizeForSpeed: &fast})
			if err == nil {
				name := fmt.Sprintf("%dx%d-%02d", v.Width, v.Height, i+1)
				if os.WriteFile(filepath.Join(dir, name+".jpg"), shot.Data, 0o644) == nil {
					out.count++
					out.after = append(out.after, f.Signature...)
					if previous && !subset(f.Signature, known) {
						out.changed = append(out.changed, name)
					}
				}
			}
		}
		// the last frame shows the bottom of the page; scrolling stops there
		if f.Y+float64(v.Height) >= size.Height-1 || y >= int(size.Height) {
			break
		}
	}
	out.findings = append(out.findings, emptyFindings(v, empty)...)
	// text passing under a pinned layer is ordinary scrolling; text that is
	// under it in every frame that shows it can never be read
	var suspects []int
	for _, id := range order {
		if !readable[id] && len(suspects) < maxRecheck {
			suspects = append(suspects, id)
		}
	}
	if len(suspects) > 0 {
		list, _ := json.Marshal(suspects)
		found, err := chromedp.Run(tab, chromedp.Evaluate[[]int](fmt.Sprintf(`__lfmLayout.recheck(%s)`, list), chromedp.EvalAwaitPromise))
		if err != nil {
			return out, err
		}
		for _, id := range found {
			readable[id] = true
		}
	}
	var hidden []*cover
	for _, id := range suspects {
		if !readable[id] {
			hidden = append(hidden, covered[id])
		}
	}
	if len(hidden) > 0 {
		h := hidden[0]
		msg := fmt.Sprintf("At %s %s covers the text of %s in every frame that shows it (page %d to %d px).", v, h.by, h.text, h.from, h.to)
		if len(hidden) > 1 {
			msg = fmt.Sprintf("At %s %s covers the text of %s and %d more elements in every frame that shows them (from page %d px).", v, h.by, h.text, len(hidden)-1, h.from)
		}
		out.findings = append(out.findings, LayoutFinding{v.String(), "covered", msg,
			"Give the text a zone the pinned element never overlaps at this size, shrink or unpin the element, or let the text scroll above it."})
	}
	over, err := chromedp.Run(tab, chromedp.Evaluate[overflowState](fmt.Sprintf(`__lfmLayout.overflow(%d)`, v.Width)))
	if err != nil {
		return out, err
	}
	splits, err := chromedp.Run(tab, chromedp.Evaluate[[]splitValue](`__lfmLayout.splits().map(s=>({Text:s.text,Selector:s.selector}))`))
	if err != nil {
		return out, err
	}
	if len(over.Culprits) > 0 || over.Width > float64(v.Width)+1 {
		var parts []string
		for _, c := range over.Culprits {
			parts = append(parts, fmt.Sprintf("%s (%d to %d px)", c.Selector, c.Left, c.Right))
		}
		msg := fmt.Sprintf("At %s the page scrolls sideways: %d px wide.", v, int(over.Width))
		if len(parts) > 0 {
			msg += " Widest: " + strings.Join(parts, ", ") + "."
		}
		out.findings = append(out.findings, LayoutFinding{v.String(), "overflow", msg, "Let the named element wrap, shrink or scroll inside itself; content must reflow at phone width without horizontal scrolling."})
	}
	for _, s := range splits {
		out.findings = append(out.findings, LayoutFinding{v.String(), "split", fmt.Sprintf("At %s %q breaks across two lines in %s.", v, s.Text, s.Selector), "Keep a number with its unit, or a short compound like E-Mail, on one line: a no-break space (&nbsp;) or non-breaking hyphen (U+2011) inside it, or white-space: nowrap on it."})
	}
	small, err := chromedp.Run(tab, chromedp.Evaluate[[]smallText](`__lfmLayout.small().map(s=>({Selector:s.selector,Size:s.size,Text:s.text}))`))
	if err != nil {
		return out, err
	}
	for _, t := range small {
		out.sizes = append(out.sizes, elementNote{t.Selector + " " + t.Text, fmt.Sprintf("%s (%q) renders at %.1f px", t.Selector, t.Text, t.Size)})
	}
	low, err := chromedp.Run(tab, chromedp.Evaluate[[]contrastIssue](`__lfmLayout.contrast().map(c=>({Selector:c.selector,Ratio:c.ratio,Need:c.need,FG:c.fg,BG:c.bg}))`))
	if err != nil {
		return out, err
	}
	for _, c := range low {
		out.contrasts = append(out.contrasts, elementNote{c.Selector, fmt.Sprintf("%s has %.2f:1 (%s on %s), needs %.1f:1", c.Selector, c.Ratio, c.FG, c.BG, c.Need)})
	}

	return out, nil
}

type smallText struct {
	Selector, Text string
	Size           float64
}

type contrastIssue struct {
	Selector, FG, BG string
	Ratio, Need      float64
}

// perElement merges per-screen observations of the same element, so a defect
// seen at every size is one finding naming the sizes.
type perElement struct {
	order   []string
	text    map[string]string
	screens map[string][]string
}

func (p *perElement) add(v Screen, element, text string) {
	if p.text == nil {
		p.text, p.screens = map[string]string{}, map[string][]string{}
	}
	if _, ok := p.text[element]; !ok {
		p.order = append(p.order, element)
		p.text[element] = text
	}
	if s := p.screens[element]; len(s) == 0 || s[len(s)-1] != v.String() {
		p.screens[element] = append(s, v.String())
	}
}

func (p *perElement) findings(kind, message, hint string) []LayoutFinding {
	if len(p.order) == 0 {
		return nil
	}
	var parts []string
	for i, e := range p.order {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("%d more", len(p.order)-3))
			break
		}
		parts = append(parts, p.text[e]+" at "+strings.Join(p.screens[e], ", "))
	}
	return []LayoutFinding{{p.screens[p.order[0]][0], kind, fmt.Sprintf(message, strings.Join(parts, "; ")), hint}}
}

// fallbackFonts reports web fonts that failed to load and characters a web
// font has no glyph for, which the browser then draws in another face. It
// asks the browser which font drew each character, so the answer is exact.
func fallbackFonts(tab context.Context) ([]LayoutFinding, error) {
	type glyph struct{ Family, Ch, Sample string }
	type report struct {
		Failed []struct{ Family, Sample string }
		Glyphs []glyph
	}
	found, err := chromedp.Run(tab, chromedp.Evaluate[report](`__lfmLayout.fonts().then(f=>({Failed:f.failed.map(x=>({Family:x.family,Sample:x.sample})),Glyphs:f.glyphs.map(g=>({Family:g.family,Ch:g.ch,Sample:g.sample}))}))`, chromedp.EvalAwaitPromise))
	if err != nil {
		return nil, err
	}
	var out []LayoutFinding
	for _, f := range found.Failed {
		out = append(out, LayoutFinding{"", "font", fmt.Sprintf("The web font %q failed to load; its text shows in a fallback face, e.g. %s.", f.Family, f.Sample), "Check the @font-face src path and format; the file must exist in the project and be WOFF2, WOFF or TrueType."})
	}
	if len(found.Glyphs) == 0 {
		return out, nil
	}
	if _, err := chromedp.Call(tab, dom.Enable, dom.EnableParams{}); err != nil {
		return out, err
	}
	if _, err := chromedp.Call(tab, css.Enable, cdp.Empty{}); err != nil {
		return out, err
	}
	doc, err := chromedp.Call(tab, dom.GetDocument, dom.GetDocumentParams{})
	if err != nil {
		return out, err
	}
	nodes, err := chromedp.Call(tab, dom.QuerySelectorAll, dom.QuerySelectorAllParams{NodeID: doc.Root.NodeID, Selector: "[data-lfm-glyph]"})
	if err != nil {
		return out, err
	}
	if len(nodes.NodeIDs) != len(found.Glyphs) {
		return out, fmt.Errorf("%d glyph probes, %d nodes", len(found.Glyphs), len(nodes.NodeIDs))
	}
	missing := map[string][]string{}
	sample := map[string]string{}
	var order []string
	for i, id := range nodes.NodeIDs {
		used, err := chromedp.Call(tab, css.GetPlatformFontsForNode, css.GetPlatformFontsForNodeParams{NodeID: id})
		if err != nil || len(used.Fonts) == 0 {
			continue
		}
		downloaded := false
		for _, f := range used.Fonts {
			downloaded = downloaded || f.IsCustomFont && f.GlyphCount > 0
		}
		if downloaded {
			continue
		}
		g := found.Glyphs[i]
		if _, ok := missing[g.Family]; !ok {
			order = append(order, g.Family)
			sample[g.Family] = g.Sample
		}
		missing[g.Family] = append(missing[g.Family], g.Ch)
	}
	for _, family := range order {
		chars := missing[family]
		them := "them"
		if len(chars) == 1 {
			them = "it"
		}
		if len(chars) > 12 {
			chars = append(chars[:12], "…")
		}
		out = append(out, LayoutFinding{"", "font", fmt.Sprintf("%q has no glyph for %s, so another face draws %s, e.g. in %s.", family, strings.Join(chars, " "), them, sample[family]), "Use a font file or subset that covers these characters (Latin Extended, quotes, currency, arrows), or set them in another declared face."})
	}
	return out, nil
}

// withoutScripts compares the page at screen v with JavaScript off against
// the scripted page: readers whose script fails, crawlers that do not run it
// and assistants that read the HTML see the first version.
func withoutScripts(tab context.Context, url string, v Screen) ([]LayoutFinding, error) {
	count := `for(const a of document.getAnimations()){try{if(Number.isFinite(a.effect.getComputedTiming().endTime))a.finish()}catch(e){}};const h=[...document.querySelectorAll("h1")].some(e=>{const r=e.getBoundingClientRect();return r.width>2&&r.height>2&&r.top<innerHeight&&getComputedStyle(e).visibility!=="hidden"&&parseFloat(getComputedStyle(e).opacity)>0.5});return {Words:(document.body.innerText.match(/\S+/g)||[]).length,Heading:h}`
	// timers never fire while scripts are off, so only the scripted page waits
	settled := "(async()=>{await document.fonts.ready;await new Promise(r=>setTimeout(r,250));" + count + "})()"
	immediate := "(()=>{" + count + "})()"
	type state struct {
		Words   int
		Heading bool
	}
	if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(v.Width), int64(v.Height), chromedp.EmulateMobile, chromedp.EmulateTouch), chromedp.Navigate(url)); err != nil {
		return nil, err
	}
	with, err := chromedp.Run(tab, chromedp.Evaluate[state](settled, chromedp.EvalAwaitPromise))
	if err != nil {
		return nil, err
	}
	if _, err := chromedp.Call(tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
		return nil, err
	}
	defer chromedp.Call(tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: false})
	if err := chromedp.Do(tab, chromedp.Navigate(url)); err != nil {
		return nil, err
	}
	without, err := chromedp.Run(tab, chromedp.Evaluate[state](immediate))
	if err != nil {
		return nil, err
	}
	var out []LayoutFinding
	if with.Heading && !without.Heading {
		out = append(out, LayoutFinding{v.String(), "no-script", fmt.Sprintf("Without JavaScript the first screen at %s shows no legible main heading.", v), "Render the heading visible in the HTML; let scripts enhance it, not reveal it."})
	}
	if with.Words >= 40 && without.Words*2 < with.Words {
		out = append(out, LayoutFinding{v.String(), "no-script", fmt.Sprintf("Without JavaScript the page shows %d words; with it, %d.", without.Words, with.Words), "Put the answer, the offer and the explanations in the HTML; let scripts compute and enhance. Crawlers, assistants and readers whose script fails see only the HTML."})
	}
	return out, nil
}

// reducedMotion loads the page at screen v with prefers-reduced-motion:
// reduce and lists what still moves in the first three frames.
func reducedMotion(parent context.Context, url string, v Screen) ([]string, error) {
	// Each of the three positions is sampled in its own tab, which first
	// passes the positions above it, as one tab scrolling down would.
	type moved struct{ Selector, How string }
	found := make([][]moved, 3)
	errs := make([]error, 3)
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Go(func() {
			tab, done := chromedp.NewContext(parent)
			defer done()
			reduce := []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}
			if _, err := chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: reduce}); err != nil {
				errs[i] = err
				return
			}
			if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(v.Width), int64(v.Height), chromedp.EmulateMobile, chromedp.EmulateTouch), chromedp.Navigate(url)); err != nil {
				errs[i] = err
				return
			}
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](layoutProbe)); err != nil {
				errs[i] = err
				return
			}
			script := "(async()=>{"
			for j := range i {
				script += fmt.Sprintf("await __lfmLayout.visit(%d);", j*v.Height/2)
			}
			script += fmt.Sprintf("return (await __lfmLayout.moving(%d)).map(m=>({Selector:m.selector,How:m.how}))})()", i*v.Height/2)
			found[i], errs[i] = chromedp.Run(tab, chromedp.Evaluate[[]moved](script, chromedp.EvalAwaitPromise))
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	var out []string
	for i := range 3 {
		if errs[i] != nil {
			return nil, errs[i]
		}
		for _, m := range found[i] {
			if item := m.Selector + " (" + m.How + ")"; !seen[item] && len(out) < 4 {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func subset(hashes []string, known map[string]bool) bool {
	for _, h := range hashes {
		if !known[h] {
			return false
		}
	}
	return true
}

// emptyFindings merges consecutive empty frames into ranges of the page.
func emptyFindings(v Screen, frames []frameState) []LayoutFinding {
	var out []LayoutFinding
	for i := 0; i < len(frames); {
		j := i
		for j+1 < len(frames) && frames[j+1].Y-frames[j].Y <= float64(v.Height) && frames[j+1].Faint == frames[i].Faint {
			j++
		}
		from, to := int(frames[i].Y), int(frames[j].Y)+v.Height
		what := "no legible text or image"
		if frames[i].Faint {
			what = "only faded content"
		}
		out = append(out, LayoutFinding{v.String(), "empty", fmt.Sprintf("At %s the page from %d to %d px shows %s.", v, from, to, what),
			"Every frame needs legible content or an obvious continuation: tighten the spacing of scroll scenes, let each item stay legible until the next arrives, fade whole units, keep a cue inside pinned parts."})
		i = j + 1
	}
	return out
}

func resetFrames(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
	for _, f := range old {
		os.Remove(f)
	}
	return nil
}
