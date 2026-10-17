// The compiler supplies selected config branches as JSON before this script.
(() => {
  const source = document.getElementById("product-data");
  const form = document.querySelector("[data-calculator]");
  if (!source || !form) return;

  const { calculator } = JSON.parse(source.textContent).data;
  const money = new Intl.NumberFormat(calculator.locale, {
    style: "currency",
    currency: calculator.currency
  });
  const result = form.querySelector("[data-result]");
  const totalOutput = form.querySelector("[data-total]");
  const perPersonOutput = form.querySelector("[data-per-person]");
  form.querySelector("fieldset").disabled = false;

  function update() {
    result.hidden = !form.checkValidity();
    if (result.hidden) return;

    const participants = form.elements.namedItem("participants").valueAsNumber;
    const roomCost = form.elements.namedItem("roomCost").valueAsNumber;
    const catering = form.elements.namedItem("cateringPerPerson").valueAsNumber;
    const total = roomCost + participants * catering;
    totalOutput.textContent = money.format(total);
    perPersonOutput.textContent = money.format(total / participants);
  }

  form.addEventListener("input", update);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    update();
  });
  update();
})();
