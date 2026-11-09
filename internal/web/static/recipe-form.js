// The Recipe form: "add" buttons add an empty row from its template in place, Ingredient
// fields being numbered by row, and a refused form puts focus on its first invalid field,
// or else on the first new Ingredient to confirm.
for (const button of document.querySelectorAll("#recipe-form [data-add]")) {
  button.addEventListener("click", (event) => {
    event.preventDefault();
    const list = document.getElementById(button.dataset.add);
    const item = document.getElementById(button.dataset.add + "-template").content.firstElementChild.cloneNode(true);
    const n = list.children.length;
    for (const field of item.querySelectorAll("[name]")) {
      field.name = field.name.replace(/_\d+$/, "_" + n);
    }
    list.append(item);
    htmx.process(item);
    item.querySelector("input, textarea").focus();
  });
}
(document.querySelector("#recipe-form [aria-invalid=true]") ?? document.querySelector("#recipe-form [data-confirm]"))?.focus();
