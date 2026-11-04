// The prompt display: the visitor types what they sell, the readout shows their order and the
// sentence for their agent, and one button refreshes and copies it. The text never comes from the
// URL: a crafted link could otherwise put someone else's instructions into the prompt a visitor
// pastes into their agent.

const data = JSON.parse(document.getElementById("product-data").textContent).data.prompt;

const form = document.getElementById("maker");
const input = document.getElementById("what");
const readout = document.getElementById("readout");
const prompt = document.getElementById("prompt");
const copy = document.getElementById("copy");
const names = [document.getElementById("prompt-what"), document.getElementById("readout-what")];

const copyLabel = copy.textContent;
let resetTimer = 0;

// Plain text only, one line, at most 60 characters; an empty field shows the fallback.
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
  readout.classList.remove("copied");
  copy.textContent = copyLabel;
}

// The readout redraws line by line, like a till display refreshing; reduced motion skips it in CSS.
function refresh() {
  const lines = Math.max(1, Math.round(readout.offsetHeight / parseFloat(getComputedStyle(prompt).lineHeight)));
  readout.style.setProperty("--lines", String(lines));
  readout.style.setProperty("--refresh", `${lines * 60}ms`);
  readout.classList.remove("refreshing");
  void readout.offsetWidth; // restart the refresh when copying twice
  readout.classList.add("refreshing");
}

input.addEventListener("input", update);

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  update();
  refresh();
  clearTimeout(resetTimer);
  try {
    await navigator.clipboard.writeText(sentence());
    readout.classList.add("copied");
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

const wipe = document.getElementById("wipe");
wipe.addEventListener("input", () => {
  document.getElementById("comparison").style.setProperty("--wipe", `${wipe.value}%`);
  document.getElementById("wipe-value").textContent = `${wipe.value}% agent default`;
});
document.getElementById("replay").addEventListener("click", () => {
  const terminal = document.getElementById("terminal");
  const lines = terminal.querySelectorAll(".line, .command");
  for (const line of lines) line.style.animation = "none";
  void terminal.offsetWidth;
  for (const line of lines) line.style.animation = "";
});
