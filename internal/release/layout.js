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
  // end, so a reveal shows its settled state; scroll-driven ones follow the scroll.
  // Looping ones are paused at their start, so frames show the same moment in
  // every run and compare across rounds.
  const finish = () => {
    for (const a of document.getAnimations()) {
      try {
        if (a.playState !== "running" || a.timeline !== document.timeline) continue;
        const end = a.effect && a.effect.getComputedTiming().endTime;
        if (Number.isFinite(end)) {
          a.finish();
        } else {
          a.pause();
          a.currentTime = 0;
        }
      } catch (_) {}
    }
  };

  // A pinned layer (fixed, or sticky and taller than a header) that sits on top
  // of legible text hides it. Thin bars are expected to pass over text while
  // scrolling and are left out.
  const pinnedLayer = (el) => {
    for (let e = el; e && e !== document.body; e = e.parentElement) {
      const cs = getComputedStyle(e);
      if ((cs.position === "fixed" || cs.position === "sticky") && e.getBoundingClientRect().height >= innerHeight * 0.2) return e;
    }
    return null;
  };
  // coverer returns the pinned layer painted over el at a point, or null.
  // elementsFromPoint lists the hit elements top first; everything before el
  // that is not one of its ancestors paints above it. Only an element's own
  // background or media content hides text; transparent boxes do not.
  const coverer = (el, x, y) => {
    for (const e of document.elementsFromPoint(x, y)) {
      if (e === el || el.contains(e)) return null;
      if (e.contains(el)) continue;
      const cs = getComputedStyle(e);
      const bg = cs.backgroundColor.match(/[\d.]+/g);
      const solid = /^(IMG|VIDEO|CANVAS|PICTURE|IFRAME)$/.test(e.tagName) || (bg && (bg.length < 4 || parseFloat(bg[3]) >= 0.5)) || /url\(|gradient\(/.test(cs.backgroundImage);
      if (solid && parseFloat(cs.opacity) >= 0.5) {
        const layer = pinnedLayer(e);
        if (layer && !layer.contains(el)) return layer;
      }
    }
    return null;
  };

  // Each text element gets a stable number for the whole walk, so the Go side
  // can tell text that is covered in one frame from text never readable at all.
  const ids = new WeakMap();
  let nextID = 0;
  const idOf = (el) => {
    if (!ids.has(el)) ids.set(el, ++nextID);
    return ids.get(el);
  };

  const elements = new Map(); // id → element, for the fine-grained recheck

  // readable reports whether a line of el is fully on screen and on top.
  const readable = (el) => {
    const range = document.createRange();
    range.selectNodeContents(el);
    for (const r of range.getClientRects()) {
      if (r.width < 4 || r.height < 4 || r.top < 0 || r.bottom > innerHeight || r.left < 0 || r.right > innerWidth) continue;
      if (!coverer(el, r.left + r.width / 2, r.top + r.height / 2)) return true;
    }
    return false;
  };

  // coverage lists the text elements fully on screen in this frame: the ones a
  // visitor can read, and the ones an opaque pinned layer sits on.
  const coverage = () => {
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    const range = document.createRange();
    const opacity = new Map();
    const seen = [], covered = [];
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const el = n.parentElement;
      if (!el || skipTags.has(el.tagName) || n.data.trim().length < 2 || pinnedLayer(el)) continue;
      const box = el.getBoundingClientRect();
      // visually hidden helper text (1px boxes) is for screen readers only
      if (box.bottom <= 0 || box.top >= innerHeight || box.width <= 2 || box.height <= 2) continue;
      if (opacityOf(el, opacity) < 0.5) continue;
      range.selectNodeContents(n);
      for (const r of range.getClientRects()) {
        if (r.width < 4 || r.height < 4 || r.top < 0 || r.bottom > innerHeight || r.left < 0 || r.right > innerWidth) continue;
        const layer = coverer(el, r.left + r.width / 2, r.top + r.height / 2);
        if (!layer) {
          seen.push(idOf(el));
          checkContrast(el, r);
        } else {
          elements.set(idOf(el), el);
          covered.push({ id: idOf(el), text: describe(el), by: describe(layer) });
        }
        break;
      }
    }
    return { seen, covered };
  };

  // Contrast is measured only where the ground is certain: the text's own and
  // ancestor backgrounds are flat colours, nothing else is painted between them
  // and the text, and no ancestor is translucent. Anything else is skipped.
  const lowContrast = new Map(); // element → finding, across frames
  // Computed colours come as rgb()/rgba() with 0–255 channels, or as
  // color(srgb r g b / a) with 0–1 channels (color-mix, relative colours).
  // Any other space (oklch, lab, display-p3) is left unmeasured.
  const rgba = (v) => {
    let scale = 1;
    if (v.startsWith("color(srgb ")) {
      scale = 255;
    } else if (!v.startsWith("rgb")) {
      return null;
    }
    const m = v.match(/-?[\d.]+(e-?\d+)?/g);
    if (!m || m.length < 3) return null;
    return [+m[0] * scale, +m[1] * scale, +m[2] * scale, m.length > 3 ? +m[3] : 1];
  };
  const over = (top, bottom) => {
    const a = top[3];
    return [0, 1, 2].map((i) => top[i] * a + bottom[i] * (1 - a)).concat(1);
  };
  const luminance = (c) => {
    const lin = (v) => ((v /= 255) <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4));
    return 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2]);
  };
  const hex = (c) => "#" + c.slice(0, 3).map((v) => Math.round(v).toString(16).padStart(2, "0")).join("");
  const ground = (el) => {
    const layers = [];
    for (let e = el; e; e = e.parentElement) {
      const cs = getComputedStyle(e);
      if (parseFloat(cs.opacity) < 1 || cs.backgroundImage !== "none" || cs.mixBlendMode !== "normal" || cs.filter !== "none" || cs.backdropFilter && cs.backdropFilter !== "none") return null;
      // a generated box with its own paint may sit behind the text unseen by hit tests
      for (const pseudo of ["::before", "::after"]) {
        const ps = getComputedStyle(e, pseudo);
        if (ps.content !== "none" && ps.content !== "normal" && (ps.backgroundImage !== "none" || (rgba(ps.backgroundColor) || [0, 0, 0, 0])[3] > 0)) return null;
      }
      const bg = rgba(cs.backgroundColor);
      if (bg && bg[3] > 0) {
        layers.push(bg);
        if (bg[3] >= 1) break;
      }
    }
    let c = [255, 255, 255, 1];
    for (let i = layers.length - 1; i >= 0; i--) c = over(layers[i], c);
    return c;
  };
  const checkContrast = (el, r) => {
    if (lowContrast.has(el)) return;
    // something other than an ancestor painted under the text makes the ground unknown
    for (const e of document.elementsFromPoint(r.left + r.width / 2, r.top + r.height / 2)) {
      if (e === el || el.contains(e)) continue;
      if (!e.contains(el)) return;
    }
    const cs = getComputedStyle(el);
    const fg = rgba(cs.color), bg = ground(el);
    if (!fg || !bg || cs.webkitTextFillColor && rgba(cs.webkitTextFillColor) && rgba(cs.webkitTextFillColor)[3] === 0) return;
    const text = over(fg, bg);
    const l1 = luminance(text), l2 = luminance(bg);
    const ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
    const size = parseFloat(cs.fontSize), weight = parseInt(cs.fontWeight, 10) || 400;
    const need = size >= 24 || (size >= 18.66 && weight >= 700) ? 3 : 4.5;
    if (ratio < need - 0.005) {
      lowContrast.set(el, { selector: describe(el), ratio: Math.floor(ratio * 100) / 100, need, fg: hex(text), bg: hex(bg) });
    }
  };

  // signature lists what a frame shows, one hash per element on screen: its
  // own text, colours, image source, form state and size, but not where it
  // sits vertically, so content that only moved down after an insertion keeps
  // its hash. Containers leave out their height, which follows their children.
  // The frame is read twice, two animation frames apart, and elements that
  // changed in between are left out: script-driven loops move on their own.
  const describeAll = () => {
    const round = (v) => Math.round(v / 4);
    const out = [];
    for (const el of document.body.getElementsByTagName("*")) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0 || r.bottom <= 0 || r.top >= innerHeight || r.right <= 0 || r.left >= innerWidth) continue;
      const cs = getComputedStyle(el);
      if (cs.visibility === "hidden" || cs.display === "none") continue;
      let own = "";
      for (const c of el.childNodes) if (c.nodeType === 3) own += c.data;
      own = own.trim();
      const leaf = own !== "" || el.children.length === 0;
      // the audit server's origin has a random port; it is not page content
      out.push([el.tagName, round(r.left), round(r.width), leaf ? round(r.height) : "", own, cs.color, cs.backgroundColor, cs.backgroundImage, Math.round(parseFloat(cs.opacity) * 10), cs.fontFamily, cs.fontSize, el.currentSrc || el.getAttribute("src") || "", el.value === undefined ? "" : String(el.value), el.checked ? "1" : "0"].join("|").split(location.origin).join(""));
    }
    return out;
  };
  const hash = (line) => {
    let h = 2166136261;
    for (let i = 0; i < line.length; i++) {
      h ^= line.charCodeAt(i);
      h = Math.imul(h, 16777619);
    }
    return (h >>> 0).toString(36);
  };
  const signature = async () => {
    // let script-driven intros finish: wait until two reads agree, at most ~2 s
    let first = describeAll();
    await settleFrames();
    let second = new Set(describeAll());
    for (let tries = 0; tries < 12 && (first.length !== second.size || first.some((l) => !second.has(l))); tries++) {
      await new Promise((r) => setTimeout(r, 150));
      first = [...second];
      await settleFrames();
      second = new Set(describeAll());
    }
    return [...new Set(first.filter((line) => second.has(line)).map(hash))];
  };

  const motionProps = /transform|translate|rotate|scale|offset|^left|^top|^right|^bottom|margin/i;

  window.__lfmLayout = {
    // fonts lists web fonts that failed to load, and sets every distinct
    // character of text in a web font into its own off-screen span with that
    // element's font list. The compiler then asks the browser which font drew
    // each span: a character no downloaded font drew is missing from the web
    // font. Whitespace and emoji are skipped; emoji always come from an emoji face.
    async fonts() {
      const clean = (f) => f.trim().replace(/^["']|["']$/g, "");
      const web = new Map();
      for (const f of document.fonts) {
        const family = clean(f.family);
        if (web.get(family.toLowerCase())?.status !== "loaded") web.set(family.toLowerCase(), { name: family, status: f.status });
      }
      const box = document.createElement("div");
      box.setAttribute("aria-hidden", "true");
      box.style.cssText = "position:absolute;left:-99999px;top:0;white-space:pre;font-size:32px";
      const failed = [], glyphs = [], seen = new Set();
      for (const el of document.body.getElementsByTagName("*")) {
        let own = "";
        for (const c of el.childNodes) if (c.nodeType === 3) own += c.data;
        if (!own.trim()) continue;
        const r = el.getBoundingClientRect();
        const cs = getComputedStyle(el);
        if (!r.width || !r.height || cs.visibility === "hidden" || cs.display === "none") continue;
        const family = clean(cs.fontFamily.split(",")[0]).toLowerCase();
        const w = web.get(family);
        if (!w) continue;
        const where = describe(el) + ' ("' + own.trim().replace(/\s+/g, " ").slice(0, 24) + '")';
        if (w.status === "error") {
          if (!seen.has(family)) failed.push({ family: w.name, sample: where });
          seen.add(family);
          continue;
        }
        for (const ch of own) {
          const key = family + "\u0000" + ch;
          if (!ch.trim() || seen.has(key) || /\p{Extended_Pictographic}/u.test(ch) || glyphs.length >= 600) continue;
          seen.add(key);
          const span = document.createElement("span");
          span.style.fontFamily = cs.fontFamily;
          span.style.fontWeight = cs.fontWeight;
          span.style.fontStyle = cs.fontStyle;
          span.textContent = ch;
          span.setAttribute("data-lfm-glyph", String(glyphs.length));
          box.append(span);
          glyphs.push({ family: w.name, ch, sample: where });
        }
      }
      document.body.append(box);
      void box.offsetHeight; // lay the spans out before the browser is asked about them
      await settleFrames();
      return { failed, glyphs };
    },

    // moving lists what still moves at scroll position y: CSS and Web
    // Animations that loop or run a second or longer and change a motion
    // property, and elements a script moves (their box changes between two
    // samples while the page stands still).
    async moving(y) {
      window.scrollTo({ top: y, left: 0, behavior: "instant" });
      await settleFrames();
      await new Promise((r) => setTimeout(r, 300));
      const out = new Map();
      for (const a of document.getAnimations()) {
        try {
          if (a.playState !== "running" || a.timeline !== document.timeline || !a.effect || !a.effect.target) continue;
          const timing = a.effect.getComputedTiming();
          const long = timing.iterations === Infinity || timing.activeDuration >= 1000;
          const meta = new Set(["offset", "computedOffset", "easing", "composite"]);
          const moves = a.effect.getKeyframes().some((k) => Object.keys(k).some((p) => !meta.has(p) && motionProps.test(p)));
          if (long && moves) out.set(a.effect.target, (a.animationName ? "@keyframes " + a.animationName : "animation") + (timing.iterations === Infinity ? ", looping" : ""));
        } catch (_) {}
      }
      // Continuous movement only: the box changes in two consecutive 200 ms
      // windows. A clock hand ticking once a second is an update, not motion,
      // and sampling it would make the result depend on timing.
      const sample = () => {
        const boxes = new Map();
        for (const el of document.body.getElementsByTagName("*")) {
          const r = el.getBoundingClientRect();
          if (r.width && r.height && r.bottom > 0 && r.top < innerHeight) boxes.set(el, r);
        }
        return boxes;
      };
      const moved = (a, b) => b && (Math.abs(a.left - b.left) > 2 || Math.abs(a.top - b.top) > 2);
      const first = sample();
      await new Promise((r) => setTimeout(r, 200));
      const second = sample();
      await new Promise((r) => setTimeout(r, 200));
      const third = sample();
      for (const [el, a] of first) {
        if (!moved(a, second.get(el)) || !moved(second.get(el), third.get(el))) continue;
        if (![...out.keys()].some((m) => m.contains(el) || el.contains(m))) out.set(el, "moved by a script");
      }
      return [...out].map(([el, how]) => ({ selector: describe(el), how }));
    },

    contrast() {
      return [...lowContrast.values()].sort((a, b) => a.ratio - b.ratio);
    },

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
      // the signature waits for script-driven intros to settle; measure after it
      const sig = await signature();
      return Object.assign(this.legibility(), coverage(), { signature: sig });
    },

    // recheck scrolls in small steps across the range where each element could
    // be on screen and returns the ids that are readable somewhere: text that
    // only passes under a pinned layer between two frames is fine.
    async recheck(list) {
      const found = [];
      for (const id of list) {
        const el = elements.get(id);
        if (!el) continue;
        const top = el.getBoundingClientRect().top + scrollY;
        const bottom = top + el.getBoundingClientRect().height;
        for (let y = Math.max(0, top - innerHeight); y <= bottom; y += Math.max(40, innerHeight / 10)) {
          window.scrollTo({ top: y, left: 0, behavior: "instant" });
          await new Promise((r) => requestAnimationFrame(r));
          if (readable(el)) {
            found.push(id);
            break;
          }
        }
      }
      return found;
    },

    legibility() {
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

    // firstScreen reports what the top of the page offers before any scroll:
    // whether the main heading is legible and whether any control is visible.
    firstScreen() {
      const opacity = new Map();
      const onScreen = (el) => {
        const r = el.getBoundingClientRect();
        return r.width > 2 && r.height > 2 && visibleArea(r) > 0 && opacityOf(el, opacity) >= 0.5;
      };
      const h1 = [...document.querySelectorAll("h1")].find((h) => onScreen(h));
      const controls = "a[href], button, input:not([type=hidden]), select, textarea, summary, [role=button], [tabindex='0']";
      const control = [...document.querySelectorAll(controls)].some((c) => onScreen(c));
      return { heading: !!h1, control };
    },

    // small lists text rendered under 9 CSS px: computed size times the
    // element's scale, which for SVG text is its screen transform.
    small() {
      const out = [], seen = new Set();
      const opacity = new Map();
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      for (let n = walker.nextNode(); n && out.length < 3; n = walker.nextNode()) {
        const el = n.parentElement;
        if (!el || seen.has(el) || skipTags.has(el.tagName) || n.data.trim().length < 2) continue;
        seen.add(el);
        const box = el.getBoundingClientRect();
        if (box.width <= 2 || box.height <= 2 || opacityOf(el, opacity) < 0.5) continue;
        let size = parseFloat(getComputedStyle(el).fontSize);
        if (el instanceof SVGElement && el.getScreenCTM) {
          const m = el.getScreenCTM();
          if (m) size *= Math.sqrt(Math.abs(m.a * m.d - m.b * m.c));
        }
        if (size > 0 && size < 9) out.push({ selector: describe(el), size: Math.round(size * 10) / 10, text: n.data.trim().replace(/\s+/g, " ").slice(0, 30) });
      }
      return out;
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
        // only overflow to the right scrolls; off-screen skip links sit left of 0
        if (r.width === 0 || r.right <= width + 1) continue;
        if (out.some((o) => o.el.contains(el)) || clipped(el)) continue;
        out.push({ el, right: Math.round(r.right), left: Math.round(r.left) });
      }
      out.sort((a, b) => b.right - a.right);
      return { width: root.scrollWidth, culprits: out.slice(0, 3).map((o) => ({ selector: describe(o.el), right: o.right, left: o.left })) };
    },

    // A number and its unit on different lines read as two values.
    splits() {
      const unit = /(?:[€$£¥]\s+\d[\d.,']*|\d[\d.,']*\s+(?:€|EUR|USD|CHF|GBP|%|‰|°C|°F|°|kg|g|mg|ml|cl|l|km|m|cm|mm|m²|m³|h|min|s|ms|kWh|kW|W|V|Hz|px|KB|MB|GB|TB|km\/h|mph|x|×|Std\.|Min\.))(?![\p{L}\p{N}])/gu;
      // short compounds (E-Mail, U-Bahn, T-Shirt) read as nonsense when the
      // line breaks at their hyphen
      const compound = /(?<![\p{L}\p{N}])[\p{L}\p{N}]{1,2}-\p{L}{2,}/gu;
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      const opacity = new Map();
      const out = [];
      for (let n = walker.nextNode(); n && out.length < 5; n = walker.nextNode()) {
        const el = n.parentElement;
        if (!el || skipTags.has(el.tagName) || opacityOf(el, opacity) < 0.15) continue;
        for (const m of [...n.data.matchAll(unit), ...n.data.matchAll(compound)]) {
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
