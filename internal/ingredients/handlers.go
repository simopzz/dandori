package ingredients

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Handlers serve the Ingredients: as the side sheet's body, a fragment any page opens,
// and as a page of their own, for a browser without htmx.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
	// CountNew answers an add the User made to count the Ingredient id, which app wires
	// to the kitchen, since that feature depends on this one.
	CountNew func(w http.ResponseWriter, r *http.Request, id int64)
}

// Show is the Ingredients page, searched for its query q, or for the Ingredient just
// picked, confirming it. Its query name starts the add form,
// and counting says it is to count what is added.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	f, q := AddForm{Name: query.Get("name"), Counting: query.Get("counting") != "", Picked: query.Get("picked")}, query.Get("q")
	if f.Picked != "" {
		q = f.Picked
	}
	h.render(w, r, http.StatusOK, f, q, false)
}

// ShowSheet is the side sheet's body, searched for the Ingredient just picked, if any.
// Its query name starts the add form, and counting says it is to count what is added.
func (h Handlers) ShowSheet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	picked := query.Get("picked")
	h.render(w, r, http.StatusOK, AddForm{Name: query.Get("name"), Counting: query.Get("counting") != "", Picked: picked}, picked, true)
}

// ShowList is the list of the User's Ingredients alone, searched for its query q, which
// the search swaps in as the User types.
func (h Handlers) ShowList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ings, err := h.Service.Search(r.Context(), q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "search the Ingredients", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, list(ings, q))
}

// ShowSuggestion is what the add form says of its query name as the User leaves it,
// without waiting for Add: the Ingredient it is close to, if any.
func (h Handlers) ShowSuggestion(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	closest, err := h.Service.Suggest(r.Context(), web.LanguageOf(r.Context()), name)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "suggest an Ingredient", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, suggestion(name, closest))
}

// Add adds an Ingredient. Posted from the side sheet by htmx, it answers with the sheet's
// body, so the page under it stays: the add form cleared once it is added, or as typed
// with why it was refused. A plain post lands on the Ingredients page. A name close to an
// Ingredient's is refused with that one suggested, unless the suggestion was already
// shown for this name, which confirms it: pressing Add past one means the name typed. One
// added to count, unless it is a Staple, which is never counted, goes on to its Count.
func (h Handlers) Add(w http.ResponseWriter, r *http.Request) {
	ing, f := parseAddForm(r)
	fragment := httpx.IsHTMX(r)
	if len(f.Errs) > 0 {
		h.render(w, r, http.StatusUnprocessableEntity, f, "", fragment)
		return
	}
	confirmed := r.PostFormValue("confirm") == f.Name
	id, err := h.Service.Add(r.Context(), web.LanguageOf(r.Context()), ing, confirmed)
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		f.Errs["name"] = web.Tf(r.Context(), msgNameTaken, taken.Existing)
		h.render(w, r, http.StatusUnprocessableEntity, f, "", fragment)
		return
	}
	if suggested, ok := errors.AsType[*SuggestedError](err); ok {
		f.Suggested = suggested.Closest
		h.render(w, r, http.StatusUnprocessableEntity, f, "", fragment)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add an Ingredient", err)
		return
	}
	if f.Counting && !ing.Staple {
		h.CountNew(w, r, id)
		return
	}
	if !fragment {
		web.RedirectWithNotice(w, r, "/ingredients", web.Tf(r.Context(), msgAdded, ing.Name))
		return
	}
	h.render(w, r, http.StatusOK, AddForm{Added: ing.Name}, "", true)
}

// render answers with f and the User's Ingredients a search for q finds, as the sheet's
// body when fragment is set, else as the Ingredients page. A floating Notice confirms
// f's Added, or else its Picked, which only an Ingredient of the User's of that name is.
func (h Handlers) render(w http.ResponseWriter, r *http.Request, status int, f AddForm, q string, fragment bool) {
	ings, err := h.Service.Search(r.Context(), q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	if !slices.ContainsFunc(ings, func(ing domain.Ingredient) bool { return ing.Name == f.Picked }) {
		f.Picked = ""
	}
	var notice string
	switch {
	case f.Added != "":
		notice = web.Tf(r.Context(), msgAdded, f.Added)
	case f.Picked != "":
		notice = web.Tf(r.Context(), msgPicked, f.Picked)
	}
	if fragment {
		body := sheetBody(ings, f, q)
		if notice != "" {
			body = web.FragmentWithNotice(body, notice)
		}
		httpx.Render(w, r, h.Log, status, body)
		return
	}
	if notice != "" {
		r = r.WithContext(web.WithNotice(r.Context(), notice))
	}
	httpx.Render(w, r, h.Log, status, page(h.Clock.Now(), ings, f, q))
}
