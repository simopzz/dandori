package ingredients

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Handlers serve the Ingredients: as the side sheet's body, a fragment any page opens,
// and as a page of their own, for a browser without htmx.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// Show is the Ingredients page, confirming the Ingredient just added, if any.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, AddForm{Added: r.URL.Query().Get("added")}, false)
}

// ShowSheet is the side sheet's body.
func (h Handlers) ShowSheet(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, AddForm{}, true)
}

// Add adds an Ingredient. Posted from the side sheet by htmx, it answers with the sheet's
// body, so the page under it stays: the add form cleared once it is added, or as typed
// with why it was refused. A plain post lands on the Ingredients page.
func (h Handlers) Add(w http.ResponseWriter, r *http.Request) {
	ing, f := parseAddForm(r)
	fragment := httpx.IsHTMX(r)
	if len(f.Errs) > 0 {
		h.render(w, r, http.StatusUnprocessableEntity, f, fragment)
		return
	}
	err := h.Service.Add(r.Context(), web.LanguageOf(r.Context()), ing)
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		f.Errs["name"] = web.Tf(r.Context(), msgNameTaken, taken.Existing)
		h.render(w, r, http.StatusUnprocessableEntity, f, fragment)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add an Ingredient", err)
		return
	}
	if !fragment {
		http.Redirect(w, r, "/ingredients?added="+url.QueryEscape(ing.Name), http.StatusSeeOther)
		return
	}
	h.render(w, r, http.StatusOK, AddForm{Added: ing.Name}, true)
}

// render answers with f and the User's Ingredients, as the sheet's body when fragment is
// set, else as the Ingredients page.
func (h Handlers) render(w http.ResponseWriter, r *http.Request, status int, f AddForm, fragment bool) {
	ings, err := h.Service.List(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	if fragment {
		httpx.Render(w, r, h.Log, status, sheetBody(ings, f))
		return
	}
	httpx.Render(w, r, h.Log, status, page(h.Clock.Now(), ings, f))
}
