package kitchen

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// movementSaved is the event that tells the Kitchen page to list its stock again.
const movementSaved = "movement-saved"

// Handlers bring the signed-in User's kitchen up to now before every page, so a page is
// never wrong because the scheduler was late, and serve the Kitchen page.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// UpToNow brings the signed-in User's kitchen up to now, then serves next.
func (h Handlers) UpToNow(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.Service.BringUpToNow(r.Context(), h.Clock.Now()); err != nil {
			httpx.ServerError(w, r, h.Log, "bring the kitchen up to now", err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Show is the Kitchen page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	stock, err := h.Service.Stock(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	f, err := h.startCount(r.Context(), now, url.Values{})
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, page(now, stock, f))
}

// ShowStock is what the Kitchen page lists alone, which it swaps in after each
// Count or move.
func (h Handlers) ShowStock(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	stock, err := h.Service.Stock(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, stockList(now, stock))
}

// ShowMovements is a Stock Item's Movements as a page of their own, for a
// browser without htmx.
func (h Handlers) ShowMovements(w http.ResponseWriter, r *http.Request) {
	if item := h.stockItem(w, r); item != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, movementsPage(h.Clock.Now(), *item))
	}
}

// ShowMovementsSheet is the Movements sheet's body: a Stock Item's Movements.
func (h Handlers) ShowMovementsSheet(w http.ResponseWriter, r *http.Request) {
	if item := h.stockItem(w, r); item != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, movementsBody(h.Clock.Now(), *item, true))
	}
}

// stockItem is the Stock Item r's query asks for with its Movements. With no such
// Ingredient or Storage Location, or on a failure, it answers r itself and is nil.
func (h Handlers) stockItem(w http.ResponseWriter, r *http.Request) *StockItemMovements {
	id, err := strconv.ParseInt(r.URL.Query().Get("ingredient"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	location := domain.StorageLocation(r.URL.Query().Get("location"))
	item, err := h.Service.StockItemMovements(r.Context(), id, location)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load a Stock Item's Movements", err)
		return nil
	}
	if item == nil {
		http.NotFound(w, r)
	}
	return item
}

// ShowCount is the Count form as a page of its own, for a browser without htmx: what its
// query asks for, as startCount reads it.
func (h Handlers) ShowCount(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	f, err := h.startCount(r.Context(), now, r.URL.Query())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, countPage(now, f))
}

// ShowCountSheet is the Count sheet's body: what its query asks for.
func (h Handlers) ShowCountSheet(w http.ResponseWriter, r *http.Request) {
	f, err := h.startCount(r.Context(), h.Clock.Now(), r.URL.Query())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, countBody(f))
}

// ShowCountList is what a search for its query q finds to count, alone, which the search
// swaps in as the User types.
func (h Handlers) ShowCountList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	found, staple, err := h.Service.Countable(r.Context(), web.LanguageOf(r.Context()), q)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "search the Ingredients to count", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, foundList(found, staple, q))
}

// Count saves a Count at the moment it arrives. Posted from the sheet by htmx, it answers
// with the sheet's body, so the sheet stays open for the next one: cleared, the Count
// confirmed in a floating Notice, telling the page under it to list its stock again; or
// as typed with why it was refused. A plain post lands on the Count page, confirming it.
func (h Handlers) Count(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	id, err := strconv.ParseInt(r.PostFormValue("ingredient"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ing, ok, err := h.Service.ingredients.Find(ctx, id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "find the Ingredient to count", err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	location, stated, f := parseCountForm(r, ing)
	if len(f.Errs) == 0 {
		err = h.Service.Count(ctx, now, id, location, stated, f.Note)
		switch {
		case errors.Is(err, ErrStaple):
			f.Errs["stated"] = web.Tf(ctx, msgStaple, ing.Name)
		case err != nil:
			httpx.ServerError(w, r, h.Log, "save a Count", err)
			return
		}
	}
	if len(f.Errs) > 0 {
		if f.Believed, err = h.Service.Believed(ctx, now, id); err != nil {
			httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
			return
		}
		h.answer(w, r, http.StatusUnprocessableEntity, now, f)
		return
	}
	counted := web.Tf(ctx, msgCounted, ing.Name, quantity(ctx, stated, ing.Unit), web.T(ctx, inLocations[location]))
	if !httpx.IsHTMX(r) {
		web.Confirm(w, r, counted)
		http.Redirect(w, r, "/kitchen/count", http.StatusSeeOther)
		return
	}
	cleared, err := h.startCount(ctx, now, url.Values{})
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	w.Header().Set("HX-Trigger", movementSaved)
	httpx.Render(w, r, h.Log, http.StatusOK, web.Confirmed(countBody(cleared), counted))
}

// ShowMove is the form that moves a Stock Item as a page of its own, for a browser
// without htmx.
func (h Handlers) ShowMove(w http.ResponseWriter, r *http.Request) {
	if f := h.startMove(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, movePage(h.Clock.Now(), *f))
	}
}

// ShowMoveSheet is the Move sheet's body: the form that moves a Stock Item.
func (h Handlers) ShowMoveSheet(w http.ResponseWriter, r *http.Request) {
	if f := h.startMove(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, moveForm(*f))
	}
}

// startMove is the form that moves the Stock Item r's query asks for, the quantity all of
// it. Holding nothing it can't be moved; then, or on a failure, it answers r itself and
// is nil.
func (h Handlers) startMove(w http.ResponseWriter, r *http.Request) *MoveForm {
	id, err := strconv.ParseInt(r.URL.Query().Get("ingredient"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	ing, believed, ok, err := h.Service.StartCount(r.Context(), h.Clock.Now(), id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a move", err)
		return nil
	}
	from := domain.StorageLocation(r.URL.Query().Get("location"))
	if !ok || believed[from] <= 0 {
		http.NotFound(w, r)
		return nil
	}
	return &MoveForm{
		Ingredient: ing, From: from, Believed: believed,
		Quantity: strconv.FormatFloat(believed[from].Number(), 'f', -1, 64),
	}
}

// Move saves a move at the moment it arrives. Posted from the sheet by htmx, it answers
// with the move confirmed in a floating Notice, closing the sheet and telling the page
// under it to list its stock again; or with the form as typed with why it was refused. A
// plain post lands on the Kitchen page, confirming it.
func (h Handlers) Move(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	id, err := strconv.ParseInt(r.PostFormValue("ingredient"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	from := domain.StorageLocation(r.PostFormValue("location"))
	ing, ok, err := h.Service.ingredients.Find(ctx, id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "find the Ingredient to move", err)
		return
	}
	if !ok || !slices.Contains(domain.StorageLocations, from) {
		http.NotFound(w, r)
		return
	}
	moved, f := parseMoveForm(r, ing, from)
	if len(f.Errs) == 0 {
		var refused MoveRefused
		err = h.Service.Move(ctx, now, id, from, f.To, moved, f.Note)
		switch {
		case errors.As(err, &refused):
			f.Errs["quantity"] = refusal(ctx, refused, ing.Unit)
		case err != nil:
			httpx.ServerError(w, r, h.Log, "save a move", err)
			return
		}
	}
	if len(f.Errs) > 0 {
		if f.Believed, err = h.Service.Believed(ctx, now, id); err != nil {
			httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
			return
		}
		if httpx.IsHTMX(r) {
			httpx.Render(w, r, h.Log, http.StatusUnprocessableEntity, moveForm(f))
			return
		}
		httpx.Render(w, r, h.Log, http.StatusUnprocessableEntity, movePage(now, f))
		return
	}
	text := web.Tf(ctx, msgMoved, quantity(ctx, moved, ing.Unit), ing.Name, web.T(ctx, toLocations[f.To]))
	if !httpx.IsHTMX(r) {
		web.Confirm(w, r, text)
		http.Redirect(w, r, "/kitchen", http.StatusSeeOther)
		return
	}
	w.Header().Set("HX-Trigger", movementSaved+", "+web.CloseSheet)
	httpx.Render(w, r, h.Log, http.StatusOK, web.Confirmed(nil, text))
}

// CountNew answers an add the User made to count the Ingredient id: its Count form, in
// the sheet under the Ingredients sheet when htmx sent the add, else on the Count page.
func (h Handlers) CountNew(w http.ResponseWriter, r *http.Request, id int64) {
	chosen := url.Values{"ingredient": {strconv.FormatInt(id, 10)}}
	if !httpx.IsHTMX(r) {
		http.Redirect(w, r, "/kitchen/count?"+chosen.Encode(), http.StatusSeeOther)
		return
	}
	now := h.Clock.Now()
	f, err := h.startCount(r.Context(), now, chosen)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	w.Header().Set("HX-Retarget", "#"+web.SideSheetBody(countSheetID))
	w.Header().Set("HX-Reswap", "innerHTML")
	h.answer(w, r, http.StatusOK, now, f)
}

// answer shows f as the sheet's body when htmx sent the request, else as the Count page.
func (h Handlers) answer(w http.ResponseWriter, r *http.Request, status int, now time.Time, f CountForm) {
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, h.Log, status, countBody(f))
		return
	}
	httpx.Render(w, r, h.Log, status, countPage(now, f))
}

// startCount is the Count form as query asks for it: its Ingredient ingredient chosen, if
// Service.StartCount allows it, in its Storage Location location or else its usual one;
// or else the search for q.
func (h Handlers) startCount(ctx context.Context, now time.Time, query url.Values) (CountForm, error) {
	f := CountForm{Query: strings.TrimSpace(query.Get("q"))}
	if id, err := strconv.ParseInt(query.Get("ingredient"), 10, 64); err == nil {
		ing, believed, ok, err := h.Service.StartCount(ctx, now, id)
		if err != nil {
			return f, err
		}
		if ok {
			f.Ingredient, f.Location, f.Believed = &ing, ing.Usual, believed
			if location := domain.StorageLocation(query.Get("location")); slices.Contains(domain.StorageLocations, location) {
				f.Location = location
			}
			return f, nil
		}
	}
	var err error
	f.Found, f.Staple, err = h.Service.Countable(ctx, web.LanguageOf(ctx), f.Query)
	return f, err
}
