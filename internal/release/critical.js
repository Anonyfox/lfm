(() => {
  const source = document.querySelector('[data-lfm-critical-source]');
  if (!source || !source.sheet) return {reason: 'The compiled stylesheet was not readable in Chromium.'};
  const elements = [...document.querySelectorAll('body *')];
  if (elements.length > 12000) return {reason: 'DOM exceeds the bounded critical-CSS observation budget; kept full CSS.'};
  const width = innerWidth, height = innerHeight;
  const boxes = new Map();
  const box = element => {
    let rect = boxes.get(element);
    if (!rect) boxes.set(element, rect = element.getBoundingClientRect());
    return rect;
  };
  const drawn = rect => rect.width || rect.height;
  const onScreen = rect => rect.bottom >= 0 && rect.right >= 0 && rect.top <= height && rect.left <= width;
  // What the first screen shows, and the elements a rule must style for it
  // to show the same: the shown ones, their ancestors, and the ones a rule
  // hides inside a shown box, which would appear without that rule.
  const shown = new Set([document.documentElement, document.body]);
  const styled = new Set(shown);
  let last = document.body; // the latest drawn element in document order
  for (const element of elements) {
    const rect = box(element);
    if (drawn(rect) && onScreen(rect)) {
      for (let parent = element; parent; parent = parent.parentElement) {
        shown.add(parent);
        styled.add(parent);
      }
    } else {
      // Hidden, or moved off the screen like a skip link, an element would
      // stand where the latest drawn one leaves off without its rules: after
      // it, or at its start when it is the element's container.
      const at = box(last);
      const top = last.contains(element) ? at.top : at.bottom;
      if (top >= 0 && top <= height && at.right >= 0 && at.left <= width) styled.add(element);
    }
    if (drawn(rect)) last = element;
  }
  // A pseudo-element styles its originating element; a rule for one counts
  // where that element is styled. States that cannot hold on a first visit
  // from a link (a target, a visited link, an autofilled field) stay in.
  const pseudoElement = /::?(?:before|after|first-line|first-letter)(?![\w-])|::[\w-]+(?:\([^()]*\))?/g;
  const unknowable = /:(?:target|target-within|visited|autofill|-webkit-autofill|user-valid|user-invalid|popover-open|modal|fullscreen|picture-in-picture|playing|paused|host|host-context)(?![\w-])/;
  function subject(selector) {
    return selector.replace(pseudoElement, (match, offset, text) => {
      const before = text.slice(0, offset).trimEnd();
      return before === '' || /[,>+~(]$/.test(before) || /\s$/.test(text.slice(0, offset)) ? '*' : '';
    });
  }
  function important(rule) {
    if (rule.cssRules?.length || unknowable.test(rule.selectorText)) return {critical:true, document:true};
    try {
      const matches = [...document.querySelectorAll(subject(rule.selectorText))];
      return {critical:matches.some(e => styled.has(e)), document:matches.length>0};
    }
    catch { return {critical:true, document:true}; }
  }
  function walk(rules, path) {
    return [...rules].map((rule, index) => {
      const key = path + '.' + index;
      if (rule instanceof CSSMediaRule || rule instanceof CSSSupportsRule) {
        return {key, prefix: (rule instanceof CSSMediaRule ? '@media ' : '@supports ') + rule.conditionText + '{', children: walk(rule.cssRules, key)};
      }
      // Keep layer/scope/container rules and all global definitions intact.
      // This preserves layer ordering, custom properties, fonts and keyframes.
      const keep = rule instanceof CSSStyleRule ? important(rule) : {critical:true, document:true};
      return {key, text: rule.cssText, ...keep};
    });
  }
  // A box that runs past the screen's edge has used sizes and positions set
  // by what lies beyond it, such as the page's length; its part on screen is
  // compared as its clipped box, its children and the pixels.
  const beyond = new Set(['width', 'height', 'inline-size', 'block-size', 'top', 'right', 'bottom', 'left',
    'inset-block-start', 'inset-block-end', 'inset-inline-start', 'inset-inline-end',
    'transform-origin', 'perspective-origin', 'grid-template-rows', 'grid-template-columns']);
  function computed(element, pseudo, clipped) {
    const style = getComputedStyle(element, pseudo);
    // Animation is frozen; every other computed value must match.
    return [...style].filter(key => !clipped || !beyond.has(key)).map(key => key + ':' + style.getPropertyValue(key)).join(';');
  }
  const page = document.documentElement;
  const order = new Map(elements.map((element, i) => [element, i]));
  const fingerprint = JSON.stringify([[width, height, page.clientWidth, page.scrollHeight > height]].concat([...shown].sort((a,b) => (order.get(a) ?? -1) - (order.get(b) ?? -1)).map(element => {
    const rect = box(element);
    const clipped = rect.left < 0 || rect.top < 0 || rect.right > width || rect.bottom > height;
    const edges = [Math.max(rect.left, 0), Math.max(rect.top, 0), Math.min(rect.right, width), Math.min(rect.bottom, height)];
    return [element.tagName, element.id, ...edges, computed(element, null, clipped), computed(element, '::before', clipped), computed(element, '::after', clipped)];
  })));
  try { return {rules: walk(source.sheet.cssRules, 'sheet'), fingerprint}; }
  catch { return {reason: 'Stylesheet contains unreadable external imports; kept full CSS.'}; }
})()
