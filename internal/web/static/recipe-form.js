// The Recipe form: a refused form puts focus on its first invalid field, or else on the
// first new Ingredient to confirm.
(document.querySelector("#recipe-form [aria-invalid=true]") ?? document.querySelector("#recipe-form [data-confirm]"))?.focus();
