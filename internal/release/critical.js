(() => {
  const source = document.querySelector('[data-lfm-critical-source]');
  if (!source || !source.sheet) return {reason: 'The compiled stylesheet was not readable in Chromium.'};
  const elements = [...document.querySelectorAll('body *')];
  if (elements.length > 12000) return {reason: 'DOM exceeds the bounded critical-CSS observation budget; kept full CSS.'};
  const visible = new Set([document.documentElement, document.body]);
  for (const element of elements) {
    const rect = element.getBoundingClientRect();
    if (rect.bottom >= 0 && rect.right >= 0 && rect.top <= innerHeight && rect.left <= innerWidth && (rect.width || rect.height)) {
      for (let parent = element; parent; parent = parent.parentElement) visible.add(parent);
    }
  }
  function important(rule) {
    // Dynamic/pseudo states and nested style rules are retained conservatively.
    if (rule.selectorText.includes(':') || rule.cssRules?.length) return {critical:true, document:true};
    try {
      const matches = [...document.querySelectorAll(rule.selectorText)];
      return {critical:matches.some(e => visible.has(e)), document:matches.length>0};
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
  function computed(element, pseudo) {
    const style = getComputedStyle(element, pseudo);
    // Animation is deliberately frozen; all other computed values must match.
    return [...style].map(key => key + ':' + style.getPropertyValue(key)).join(';');
  }
  const fingerprint = JSON.stringify([...visible].sort((a,b) => elements.indexOf(a) - elements.indexOf(b)).map(element => {
    const rect = element.getBoundingClientRect();
    return [element.tagName, element.id, rect.x, rect.y, rect.width, rect.height, computed(element), computed(element, '::before'), computed(element, '::after')];
  }));
  try { return {rules: walk(source.sheet.cssRules, 'sheet'), fingerprint}; }
  catch { return {reason: 'Stylesheet contains unreadable external imports; kept full CSS.'}; }
})()
