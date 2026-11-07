package ingredients

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
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

// Saved is the event an edit triggers, with HX-Trigger, so a page showing Stock Items
// lists them again: marking a Staple counts its stock to nothing, and a rename shows.
const Saved = "ingredient-saved"

// Show is the Ingredients page, searched for its query q, or for the Ingredient just
// picked, confirming it. Its query name starts the add form,
// and counting says it is to count what is added.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	f, q := AddForm{Name: query.Get("name"), Counting: query.Get("counting") != "", Picked: query.Get("picked")}, query.Get("q")
	if f.Picked != "" {
		q = f.Picked
	}
	h.render(w, r, http.StatusOK, sheetView{Add: f, Q: q}, false)
}

// ShowSheet is the side sheet's body, searched for the Ingredient just picked, if any.
// Its query name starts the add form, and counting says it is to count what is added.
func (h Handlers) ShowSheet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	picked := query.Get("picked")
	h.render(w, r, http.StatusOK, sheetView{Add: AddForm{Name: query.Get("name"), Counting: query.Get("counting") != "", Picked: picked}, Q: picked}, true)
}

// ShowList is the list of the User's Ingredients alone, searched for its query q, which
// the search swaps in as the User types.
func (h Handlers) ShowList(w http.ResponseWriter, r *http.Request) {
	view := sheetView{Q: r.URL.Query().Get("q")}
	ings, withMovements, err := h.Service.Listing(r.Context(), view.Q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "search the Ingredients", err)
		return
	}
	view.WithMovements = withMovements
	httpx.Render(w, r, h.Log, http.StatusOK, list(ings, view))
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
		h.render(w, r, http.StatusUnprocessableEntity, sheetView{Add: f}, fragment)
		return
	}
	confirmed := r.PostFormValue("confirm") == f.Name
	id, err := h.Service.Add(r.Context(), web.LanguageOf(r.Context()), ing, confirmed)
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		f.Errs["name"] = web.Tf(r.Context(), msgNameTaken, taken.Existing)
		h.render(w, r, http.StatusUnprocessableEntity, sheetView{Add: f}, fragment)
		return
	}
	if suggested, ok := errors.AsType[*SuggestedError](err); ok {
		f.Suggested = suggested.Closest
		h.render(w, r, http.StatusUnprocessableEntity, sheetView{Add: f}, fragment)
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
	h.render(w, r, http.StatusOK, sheetView{Add: AddForm{Added: ing.Name}}, true)
}

// Edit edits the Ingredient id. Posted from the side sheet by htmx, it answers with the
// sheet's body, searched as it was: the edit confirmed and its row closed, or the row
// open as typed with why it was refused. A plain post lands on the Ingredients page. A
// new name close to an Ingredient's is refused with that one suggested, unless the
// suggestion was already shown for this name, which confirms it. Marked a Staple, the
// confirmation says what stock was counted to nothing, and a page under the sheet lists
// its Stock Items again.
func (h Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ing, f := parseAddForm(r)
	view := sheetView{Edit: EditForm{ID: id, AddForm: f}, Q: r.PostFormValue("q")}
	fragment := httpx.IsHTMX(r)
	if len(f.Errs) > 0 {
		h.render(w, r, http.StatusUnprocessableEntity, view, fragment)
		return
	}
	counted, err := h.Service.Edit(ctx, web.LanguageOf(ctx), h.Clock.Now(), id, ing, r.PostFormValue("confirm") == f.Name)
	if taken, ok := errors.AsType[*NameTakenError](err); ok {
		view.Edit.Errs["name"] = web.Tf(ctx, msgNameTaken, taken.Existing)
		h.render(w, r, http.StatusUnprocessableEntity, view, fragment)
		return
	}
	if suggested, ok := errors.AsType[*SuggestedError](err); ok {
		view.Edit.Suggested = suggested.Closest
		h.render(w, r, http.StatusUnprocessableEntity, view, fragment)
		return
	}
	if fixed, ok := errors.AsType[*UnitFixedError](err); ok {
		view.Edit.Errs["unit"] = web.Tf(ctx, msgUnitFixed, web.T(ctx, recordedIn[fixed.Unit]))
		h.render(w, r, http.StatusUnprocessableEntity, view, fragment)
		return
	}
	if errors.Is(err, ErrNoIngredient) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "edit an Ingredient", err)
		return
	}
	notice := web.Tf(ctx, msgSaved, ing.Name)
	if len(counted) > 0 {
		notice = stapleNotice(ctx, ing, counted)
	}
	if fragment {
		w.Header().Set("HX-Trigger", Saved)
	}
	h.done(w, r, sheetView{Q: view.Q, Notice: notice}, fragment)
}

// stapleNotice confirms ing marked a Staple, with each of its Stock Items counted, in
// counted, to nothing: one by its quantity alone, several each with its Storage Location.
func stapleNotice(ctx context.Context, ing domain.Ingredient, counted []domain.Movement) string {
	first := web.Quantity(ctx, counted[0].Believed, ing.Unit)
	if len(counted) == 1 {
		return web.Tf(ctx, msgStapleCounted, web.Tf(ctx, msgQuantityOf, first, ing.Name))
	}
	parts := []string{web.Tf(ctx, msgQuantityOfIn, first, ing.Name, web.T(ctx, web.InLocations[counted[0].Key.Location]))}
	for _, c := range counted[1:] {
		parts = append(parts, web.Tf(ctx, msgQuantityIn, web.Quantity(ctx, c.Believed, ing.Unit), web.T(ctx, web.InLocations[c.Key.Location])))
	}
	last := len(parts) - 1
	listed := web.Tf(ctx, msgAnd, strings.Join(parts[:last], ", "), parts[last])
	return web.Tf(ctx, msgStapleCounted, listed)
}

// Delete deletes the Ingredient id, answering as Edit does. One Movements refer to is
// refused, its row open with why.
func (h Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q, fragment := r.PostFormValue("q"), httpx.IsHTMX(r)
	name, err := h.Service.Delete(ctx, id)
	if errors.Is(err, ErrHasMovements) {
		view := sheetView{Edit: EditForm{ID: id, DeleteErr: web.T(ctx, msgHasMovements)}, Q: q}
		h.render(w, r, http.StatusUnprocessableEntity, view, fragment)
		return
	}
	if errors.Is(err, ErrNoIngredient) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "delete an Ingredient", err)
		return
	}
	h.done(w, r, sheetView{Q: q, Notice: web.Tf(ctx, msgDeleted, name)}, fragment)
}

// done answers a write view confirms: as the sheet's body when fragment is set, else by
// redirecting to the Ingredients page searched as it was.
func (h Handlers) done(w http.ResponseWriter, r *http.Request, view sheetView, fragment bool) {
	if fragment {
		h.render(w, r, http.StatusOK, view, true)
		return
	}
	to := "/ingredients"
	if view.Q != "" {
		to += "?q=" + url.QueryEscape(view.Q)
	}
	web.RedirectWithNotice(w, r, to, view.Notice)
}

// sheetView is what the Ingredients show: the add form, the Ingredient being edited, the
// search, which Ingredients have Movements, and a Notice confirming what was done, when
// something was.
type sheetView struct {
	Add  AddForm
	Edit EditForm
	Q    string
	// WithMovements is the ids of the Ingredients some Movement refers to.
	WithMovements map[int64]bool
	// Notice confirms a write. Without one, Add's Added is confirmed, or else its
	// Picked, which only an Ingredient of the User's of that name is.
	Notice string
}

// render answers with view, its list the User's Ingredients a search for its Q finds, as
// the sheet's body when fragment is set, else as the Ingredients page.
func (h Handlers) render(w http.ResponseWriter, r *http.Request, status int, view sheetView, fragment bool) {
	ings, withMovements, err := h.Service.Listing(r.Context(), view.Q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Ingredients", err)
		return
	}
	view.WithMovements = withMovements
	f := &view.Add
	if !slices.ContainsFunc(ings, func(ing domain.Ingredient) bool { return ing.Name == f.Picked }) {
		f.Picked = ""
	}
	notice := view.Notice
	switch {
	case notice != "":
	case f.Added != "":
		notice = web.Tf(r.Context(), msgAdded, f.Added)
	case f.Picked != "":
		notice = web.Tf(r.Context(), msgPicked, f.Picked)
	}
	if fragment {
		body := sheetBody(ings, view)
		if notice != "" {
			body = web.FragmentWithNotice(body, notice)
		}
		httpx.Render(w, r, h.Log, status, body)
		return
	}
	if notice != "" {
		r = r.WithContext(web.WithNotice(r.Context(), notice))
	}
	httpx.Render(w, r, h.Log, status, page(h.Clock.Now(), ings, view))
}
