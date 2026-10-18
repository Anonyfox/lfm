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
	var sizes, contrasts perElement
	for _, v := range Screens {
		opts := []chromedp.EmulateViewportOption{}
		if v.Mobile {
			opts = append(opts, chromedp.EmulateMobile, chromedp.EmulateTouch)
		}
		if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(v.Width), int64(v.Height), opts...), chromedp.Navigate(url)); err != nil {
			return r, err
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
			return r, err
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](layoutProbe)); err != nil {
			return r, err
		}
		size, err := chromedp.Run(tab, chromedp.Evaluate[pageSize](`(()=>{const s=__lfmLayout.prepare();return {Height:s.height,Width:s.width,VW:s.vw,VH:s.vh}})()`))
		if err != nil {
			return r, err
		}
		step := v.Height / 2
		if last := int(size.Height) - v.Height; last > 0 && last/step+1 > maxFrames {
			step = last/(maxFrames-1) + 1
		}
		known := map[string]bool{}
		for _, h := range before[v.String()] {
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
				return r, err
			}
			if i == 0 {
				first, err := chromedp.Run(tab, chromedp.Evaluate[struct{ Heading, Control bool }](`(()=>{const f=__lfmLayout.firstScreen();return {Heading:f.heading,Control:f.control}})()`))
				if err != nil {
					return r, err
				}
				if !first.Heading {
					r.Findings = append(r.Findings, LayoutFinding{v.String(), "first-screen", fmt.Sprintf("At %s the first screen shows no legible main heading.", v), "Let the h1 that says what the page offers appear, readable, before any scrolling at every size."})
				}
				if !first.Control {
					r.Findings = append(r.Findings, LayoutFinding{v.String(), "first-screen", fmt.Sprintf("At %s the first screen offers no link, button or field.", v), "Give the first screen a way to act: the tool's first input, the answer link or the way to the offer."})
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
						r.Count++
						after[v.String()] = append(after[v.String()], f.Signature...)
						if r.Previous && !subset(f.Signature, known) {
							r.Changed = append(r.Changed, name)
						}
					}
				}
			}
			// the last frame shows the bottom of the page; scrolling stops there
			if f.Y+float64(v.Height) >= size.Height-1 || y >= int(size.Height) {
				break
			}
		}
		r.Findings = append(r.Findings, emptyFindings(v, empty)...)
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
				return r, err
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
			r.Findings = append(r.Findings, LayoutFinding{v.String(), "covered", msg,
				"Give the text a zone the pinned element never overlaps at this size, shrink or unpin the element, or let the text scroll above it."})
		}
		over, err := chromedp.Run(tab, chromedp.Evaluate[overflowState](fmt.Sprintf(`__lfmLayout.overflow(%d)`, v.Width)))
		if err != nil {
			return r, err
		}
		splits, err := chromedp.Run(tab, chromedp.Evaluate[[]splitValue](`__lfmLayout.splits().map(s=>({Text:s.text,Selector:s.selector}))`))
		if err != nil {
			return r, err
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
			r.Findings = append(r.Findings, LayoutFinding{v.String(), "overflow", msg, "Let the named element wrap, shrink or scroll inside itself; content must reflow at phone width without horizontal scrolling."})
		}
		for _, s := range splits {
			r.Findings = append(r.Findings, LayoutFinding{v.String(), "split", fmt.Sprintf("At %s %q breaks across two lines in %s.", v, s.Text, s.Selector), "Keep a number with its unit, or a short compound like E-Mail, on one line: a no-break space (&nbsp;) or non-breaking hyphen (U+2011) inside it, or white-space: nowrap on it."})
		}
		small, err := chromedp.Run(tab, chromedp.Evaluate[[]smallText](`__lfmLayout.small().map(s=>({Selector:s.selector,Size:s.size,Text:s.text}))`))
		if err != nil {
			return r, err
		}
		for _, t := range small {
			sizes.add(v, t.Selector+" "+t.Text, fmt.Sprintf("%s (%q) renders at %.1f px", t.Selector, t.Text, t.Size))
		}
		low, err := chromedp.Run(tab, chromedp.Evaluate[[]contrastIssue](`__lfmLayout.contrast().map(c=>({Selector:c.selector,Ratio:c.ratio,Need:c.need,FG:c.fg,BG:c.bg}))`))
		if err != nil {
			return r, err
		}
		for _, c := range low {
			contrasts.add(v, c.Selector, fmt.Sprintf("%s has %.2f:1 (%s on %s), needs %.1f:1", c.Selector, c.Ratio, c.FG, c.BG, c.Need))
		}
	}
	if moving, err := reducedMotion(tab, url, Screens[0]); err != nil {
		return r, err
	} else if len(moving) > 0 {
		r.Findings = append(r.Findings, LayoutFinding{Screens[0].String(), "motion", fmt.Sprintf("With reduced motion requested, the page still moves at %s: %s.", Screens[0], strings.Join(moving, "; ")),
			"Inside @media (prefers-reduced-motion: reduce) stop loops and long moves (animation: none, or a short fade), and let scripts check matchMedia(\"(prefers-reduced-motion: reduce)\") before animating."})
	}
	// scripts off last: toggling script execution leaves later loads unlike a visitor's
	if found, err := withoutScripts(tab, url, Screens[0]); err != nil {
		return r, err
	} else {
		r.Findings = append(r.Findings, found...)
	}
	r.Findings = append(r.Findings, sizes.findings("small", "Text too small to read: %s.", "Set text at 9 px or more where it renders, including SVG labels scaled down with their drawing; small print reads well from 12 px.")...)
	r.Findings = append(r.Findings, contrasts.findings("contrast", "Text below the WCAG contrast minimum: %s.", "Darken the text or lighten its ground until it reaches the ratio; large text (24 px, or 18.7 px bold) needs 3:1, other text 4.5:1.")...)
	return r, nil
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
func reducedMotion(tab context.Context, url string, v Screen) ([]string, error) {
	reduce := []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}
	if _, err := chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: reduce}); err != nil {
		return nil, err
	}
	defer chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: ""}}})
	if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(v.Width), int64(v.Height), chromedp.EmulateMobile, chromedp.EmulateTouch), chromedp.Navigate(url)); err != nil {
		return nil, err
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](layoutProbe)); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; i < 3; i++ {
		found, err := chromedp.Run(tab, chromedp.Evaluate[[]struct{ Selector, How string }](fmt.Sprintf(`__lfmLayout.moving(%d).then(l=>l.map(m=>({Selector:m.selector,How:m.how})))`, i*v.Height/2), chromedp.EvalAwaitPromise))
		if err != nil {
			return nil, err
		}
		for _, m := range found {
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
