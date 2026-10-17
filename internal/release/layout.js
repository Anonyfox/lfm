// Layout probe for lfm --release. It reads the rendered page the way a visitor
// sees it while scrolling: what is legible in each frame, whether the page
// scrolls sideways, and whether a number breaks away from its unit. It never
// clicks, types or submits anything.
(() => {
  const skipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "TEMPLATE", "TITLE"]);
  const media = "img, video, canvas, svg, picture, iframe, object, embed";
  let backgrounds = [];

  const describe = (el) => {
    let s = el.tagName.toLowerCase();
    if (el.id) return s + "#" + el.id;
    const cls = typeof el.className === "string" ? el.className.trim().split(/\s+/).filter(Boolean).slice(0, 2) : [];
    return cls.length ? s + "." + cls.join(".") : s;
  };

  // opacity multiplies down the tree; hidden or undisplayed ancestors make it 0.
  const opacityOf = (el, cache) => {
    if (!el || el.nodeType !== 1) return 1;
    if (cache.has(el)) return cache.get(el);
    const cs = getComputedStyle(el);
    let o = cs.display === "none" || cs.visibility === "hidden" || cs.contentVisibility === "hidden" ? 0 : parseFloat(cs.opacity);
    if (o > 0) o *= opacityOf(el.parentElement, cache);
    cache.set(el, o);
    return o;
  };

  // Fixed bars and thin sticky headers sit in every frame; they never make a
  // frame worth reading. Tall sticky stages are content and stay.
  const chrome = (el, cache) => {
    for (let e = el; e && e !== document.body; e = e.parentElement) {
      if (cache.has(e)) return cache.get(e);
      const pos = getComputedStyle(e).position;
      if (pos === "fixed" || (pos === "sticky" && e.getBoundingClientRect().height < innerHeight * 0.2)) {
        cache.set(e, true);
        return true;
      }
    }
    return false;
  };

  const visibleArea = (r) => {
    const w = Math.min(r.right, innerWidth) - Math.max(r.left, 0);
    const h = Math.min(r.bottom, innerHeight) - Math.max(r.top, 0);
    return w > 0 && h > 0 ? w * h : 0;
  };

  const settleFrames = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));

  // Jump finite animations and transitions on the document timeline to their
  // end, so a reveal shows its settled state; looping and scroll-driven ones run.
  const finish = () => {
    for (const a of document.getAnimations()) {
      try {
        if (a.playState !== "running" || a.timeline !== document.timeline) continue;
        const end = a.effect && a.effect.getComputedTiming().endTime;
        if (Number.isFinite(end)) a.finish();
      } catch (_) {}
    }
  };

  window.__lfmLayout = {
    prepare() {
      backgrounds = [];
      for (const el of document.body.querySelectorAll("*")) {
        if (/url\(/.test(getComputedStyle(el).backgroundImage)) backgrounds.push(el);
      }
      const root = document.scrollingElement || document.documentElement;
      return { height: root.scrollHeight, width: root.scrollWidth, vw: innerWidth, vh: innerHeight };
    },

    async frame(y) {
      window.scrollTo({ top: y, left: 0, behavior: "instant" });
      await settleFrames();
      await new Promise((r) => setTimeout(r, 50));
      finish();
      await settleFrames();
      const opacity = new Map(), pinned = new Map();
      let faint = false;
      const judge = (el) => {
        if (chrome(el, pinned)) return false;
        const o = opacityOf(el, opacity);
        if (o >= 0.5) return true;
        if (o >= 0.15) faint = true;
        return false;
      };
      for (const el of document.body.querySelectorAll(media)) {
        if (el.ownerSVGElement) continue;
        const r = el.getBoundingClientRect();
        if (r.width >= 64 && r.height >= 64 && visibleArea(r) >= 48 * 48 && judge(el)) return { legible: true, y: scrollY };
      }
      for (const el of backgrounds) {
        if (visibleArea(el.getBoundingClientRect()) >= 0.25 * innerWidth * innerHeight && judge(el)) return { legible: true, y: scrollY };
      }
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        const el = n.parentElement;
        if (!el || skipTags.has(el.tagName) || n.data.trim().length < 2) continue;
        const box = el.getBoundingClientRect();
        if (box.bottom <= 0 || box.top >= innerHeight || box.right <= 0 || box.left >= innerWidth) continue;
        range.selectNodeContents(n);
        let shown = false;
        for (const r of range.getClientRects()) {
          if (visibleArea(r) >= 16) { shown = true; break; }
        }
        if (shown && judge(el)) return { legible: true, y: scrollY };
      }
      return { legible: false, faint, y: scrollY };
    },

    // width is the screen's; a phone browser zooms out a page wider than it,
    // which widens innerWidth too.
    overflow(width) {
      const root = document.scrollingElement || document.documentElement;
      if (root.scrollWidth <= width + 1) return { width: root.scrollWidth, culprits: [] };
      const clipped = (el) => {
        for (let e = el.parentElement; e && e !== document.body; e = e.parentElement) {
          if (getComputedStyle(e).overflowX !== "visible") return true;
        }
        return false;
      };
      const out = [];
      for (const el of document.body.querySelectorAll("*")) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 || (r.right <= width + 1 && r.left >= -1)) continue;
        if (out.some((o) => o.el.contains(el)) || clipped(el)) continue;
        out.push({ el, right: Math.round(r.right), left: Math.round(r.left) });
      }
      out.sort((a, b) => b.right - a.right);
      return { width: root.scrollWidth, culprits: out.slice(0, 3).map((o) => ({ selector: describe(o.el), right: o.right, left: o.left })) };
    },

    // A number and its unit on different lines read as two values.
    splits() {
      const unit = /(?:[€$£¥]\s+\d[\d.,']*|\d[\d.,']*\s+(?:€|EUR|USD|CHF|GBP|%|‰|°C|°F|°|kg|g|mg|ml|cl|l|km|m|cm|mm|m²|m³|h|min|s|ms|kWh|kW|W|V|Hz|px|KB|MB|GB|TB|km\/h|mph|x|×|Std\.|Min\.))(?![\p{L}\p{N}])/gu;
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      const opacity = new Map();
      const out = [];
      for (let n = walker.nextNode(); n && out.length < 5; n = walker.nextNode()) {
        const el = n.parentElement;
        if (!el || skipTags.has(el.tagName) || opacityOf(el, opacity) < 0.15) continue;
        for (const m of n.data.matchAll(unit)) {
          range.setStart(n, m.index);
          range.setEnd(n, m.index + m[0].length);
          const tops = [...range.getClientRects()].filter((r) => r.width > 0).map((r) => r.top);
          if (tops.length > 1 && Math.max(...tops) - Math.min(...tops) > 2) {
            out.push({ text: m[0].replace(/\s+/g, " "), selector: describe(el) });
            break;
          }
        }
      }
      return out;
    },
  };
  return true;
})();
