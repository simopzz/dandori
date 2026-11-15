// The Recipe form: a refused form puts focus on its error summary, or else on the first
// new Ingredient to confirm.
(document.querySelector("#recipe-form .error-summary") ?? document.querySelector("#recipe-form [data-confirm]"))?.focus();
