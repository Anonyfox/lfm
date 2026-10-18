// The prompt printer: the visitor types what they sell, the slip reprints the sentence for their
// agent and copies it. The text never comes from the URL: a crafted link could otherwise put
// someone else's instructions into the prompt a visitor pastes into their agent.

const data = JSON.parse(document.getElementById("product-data").textContent).data.prompt;

const form = document.getElementById("printer");
const input = document.getElementById("what");
const what = document.getElementById("prompt-what");
const prompt = document.getElementById("prompt");
const slip = document.getElementById("slip");
const copy = document.getElementById("copy");
const today = document.getElementById("today");

const copyLabel = copy.textContent;
let stampTimer = 0;

// Plain text only, one line, at most 80 characters; an empty field prints the example.
function clean(value) {
  const text = String(value || "")
    .replace(/[\u0000-\u001f\u007f]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 80);
  return text || data.example;
}

function sentence() {
  return prompt.textContent.replace(/\s+/g, " ").trim();
}

function print(value, animate) {
  what.textContent = clean(value);
  slip.classList.remove("stamped");
  copy.textContent = copyLabel;
  if (!animate) return;
  const lines = Math.max(1, Math.round(prompt.offsetHeight / parseFloat(getComputedStyle(prompt).lineHeight)));
  prompt.style.setProperty("--lines", String(lines));
  prompt.style.setProperty("--feed", `${lines * 80}ms`);
  slip.classList.remove("feeding");
  void slip.offsetWidth; // restart the feed when printing twice
  slip.classList.add("feeding");
}

form.addEventListener("submit", (event) => {
  event.preventDefault();
  print(input.value, true);
});

input.addEventListener("input", () => print(input.value, false));

copy.addEventListener("click", async () => {
  const text = sentence();
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const range = document.createRange();
    range.selectNodeContents(prompt);
    const selection = getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    copy.textContent = "Selected. Press copy, then paste it into your agent";
    return;
  }
  slip.classList.add("stamped");
  copy.textContent = "Copied. Paste it into your agent";
  clearTimeout(stampTimer);
  stampTimer = setTimeout(() => {
    slip.classList.remove("stamped");
    copy.textContent = copyLabel;
  }, 5000);
});

today.textContent = new Date().toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
