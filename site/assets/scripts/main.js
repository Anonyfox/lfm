// The prompt printer: the visitor types what they sell, the slip reprints their order and the
// sentence for their agent, and one button prints and copies it. The text never comes from the
// URL: a crafted link could otherwise put someone else's instructions into the prompt a visitor
// pastes into their agent.

const data = JSON.parse(document.getElementById("product-data").textContent).data.prompt;

const form = document.getElementById("printer");
const input = document.getElementById("what");
const slip = document.getElementById("slip");
const prompt = document.getElementById("prompt");
const copy = document.getElementById("copy");
const names = [document.getElementById("prompt-what"), document.getElementById("slip-what")];

const copyLabel = copy.textContent;
let resetTimer = 0;

// Plain text only, one line, at most 60 characters; an empty field prints the fallback.
function clean(value) {
  const text = String(value || "")
    .replace(/[\u0000-\u001f\u007f]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 60);
  return text || data.fallback;
}

function sentence() {
  return prompt.textContent.replace(/\s+/g, " ").trim();
}

function update() {
  const name = clean(input.value);
  for (const el of names) el.textContent = name;
  slip.classList.remove("stamped");
  copy.textContent = copyLabel;
}

// The slip feeds out line by line, like a receipt printer; reduced motion skips it in CSS.
function feed() {
  const lines = Math.max(1, Math.round(slip.offsetHeight / parseFloat(getComputedStyle(prompt).lineHeight)));
  slip.style.setProperty("--lines", String(lines));
  slip.style.setProperty("--feed", `${lines * 60}ms`);
  slip.classList.remove("feeding");
  void slip.offsetWidth; // restart the feed when printing twice
  slip.classList.add("feeding");
}

input.addEventListener("input", update);

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  update();
  feed();
  clearTimeout(resetTimer);
  try {
    await navigator.clipboard.writeText(sentence());
    slip.classList.add("stamped");
    copy.textContent = "Copied. Paste it into your agent";
  } catch {
    const range = document.createRange();
    range.selectNodeContents(prompt);
    const selection = getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    copy.textContent = "Selected. Copy it, then paste it into your agent";
  }
  resetTimer = setTimeout(update, 6000);
});

document.querySelector(".today").textContent = new Date().toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
