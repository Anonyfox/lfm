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
	Findings []LayoutFinding `json:"findings,omitempty"`
}

// LayoutFinding is one layout defect at one viewport.
type LayoutFinding struct {
	Screen  string `json:"viewport"`
	Kind    string `json:"kind"` // overflow, empty, split
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// FramesPath is the screenshot folder of a project, in the system temp
// directory like the Lighthouse report, so the project tree stays clean.
func FramesPath(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(os.TempDir(), fmt.Sprintf("lfm-frames-%x", sum[:6]))
}

type frameState struct {
	Legible bool    `json:"legible"`
	Faint   bool    `json:"faint"`
	Y       float64 `json:"y"`
	Seen    []int   `json:"seen"`
	Covered []struct {
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
	if dir != "" {
		if err := resetFrames(dir); err != nil {
			r.Frames = ""
			dir = ""
		}
	}
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
					name := filepath.Join(dir, fmt.Sprintf("%dx%d-%02d.jpg", v.Width, v.Height, i+1))
					if os.WriteFile(name, shot.Data, 0o644) == nil {
						r.Count++
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
			r.Findings = append(r.Findings, LayoutFinding{v.String(), "split", fmt.Sprintf("At %s %q breaks across two lines in %s.", v, s.Text, s.Selector), "Keep a number and its unit together: a no-break space (&nbsp;) between them or white-space: nowrap on the value."})
		}
	}
	return r, nil
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
