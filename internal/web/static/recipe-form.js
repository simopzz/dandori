// Adds an empty Ingredient row or step to a Recipe form from its template.
// Ingredient fields are numbered by row, so a new row takes the next number.
for (const button of document.querySelectorAll("[data-add]")) {
  button.addEventListener("click", () => {
    const list = document.getElementById(button.dataset.add);
    const template = document.getElementById(button.dataset.add + "-template");
    const item = template.content.firstElementChild.cloneNode(true);
    const n = list.children.length;
    for (const field of item.querySelectorAll("[name]")) {
      field.name = field.name.replace(/_\d+$/, "_" + n);
    }
    list.append(item);
    item.querySelector("input:not([type=hidden]), textarea").focus();
  });
}
