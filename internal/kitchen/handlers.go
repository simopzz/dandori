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

// stockCounted is the event that tells the Kitchen page to list its stock again.
const stockCounted = "stock-counted"

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

// ShowStock is what the Kitchen page lists alone, which it swaps in after each Count.
func (h Handlers) ShowStock(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	stock, err := h.Service.Stock(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, stockList(now, stock))
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
// with the sheet's body, so the sheet stays open: cleared with the Count confirmed,
// telling the page under it to list its stock again, or as typed with why it was refused.
// A plain post lands on the Count page.
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
	location := domain.StorageLocation(r.PostFormValue("location"))
	f := CountForm{
		Ingredient: &ing, Location: location,
		Stated: strings.TrimSpace(r.PostFormValue("stated")), Note: r.PostFormValue("note"),
		Errs: map[string]string{},
	}
	if !slices.Contains(domain.StorageLocations, location) {
		f.Errs["location"], f.Location = web.T(ctx, msgChooseWhere), ing.Usual
	}
	stated, problem := domain.ParseQuantity(ing.Unit, f.Stated)
	if problem != "" {
		f.Errs["stated"] = web.T(ctx, problemMessages[problem])
	}
	var saved int64
	if len(f.Errs) == 0 {
		saved, err = h.Service.Count(ctx, now, id, location, stated, f.Note)
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
	counted := url.Values{"counted": {strconv.FormatInt(saved, 10)}}
	if !httpx.IsHTMX(r) {
		http.Redirect(w, r, "/kitchen/count?"+counted.Encode(), http.StatusSeeOther)
		return
	}
	cleared, err := h.startCount(ctx, now, counted)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	w.Header().Set("HX-Trigger", stockCounted)
	h.answer(w, r, http.StatusOK, now, cleared)
}

// answer shows f as the sheet's body when htmx sent the request, else as the Count page.
func (h Handlers) answer(w http.ResponseWriter, r *http.Request, status int, now time.Time, f CountForm) {
	if httpx.IsHTMX(r) {
		httpx.Render(w, r, h.Log, status, countBody(f))
		return
	}
	httpx.Render(w, r, h.Log, status, countPage(now, f))
}

// startCount is the Count form as query asks for it: the Count counted confirmed; its
// Ingredient ingredient chosen, in its Storage Location location or else its usual one;
// or else the search for q. An Ingredient the User hasn't got is never chosen, nor is
// a Staple.
func (h Handlers) startCount(ctx context.Context, now time.Time, query url.Values) (CountForm, error) {
	f := CountForm{Query: strings.TrimSpace(query.Get("q"))}
	if id, err := strconv.ParseInt(query.Get("counted"), 10, 64); err == nil {
		if f.Counted, err = h.Service.FindCount(ctx, id); err != nil {
			return f, err
		}
	}
	if id, err := strconv.ParseInt(query.Get("ingredient"), 10, 64); err == nil {
		ing, ok, err := h.Service.ingredients.Find(ctx, id)
		if err != nil {
			return f, err
		}
		if ok && !ing.Staple {
			f.Ingredient, f.Location = &ing, ing.Usual
			if location := domain.StorageLocation(query.Get("location")); slices.Contains(domain.StorageLocations, location) {
				f.Location = location
			}
			f.Believed, err = h.Service.Believed(ctx, now, id)
			return f, err
		}
	}
	var err error
	f.Found, f.Staple, err = h.Service.Countable(ctx, web.LanguageOf(ctx), f.Query)
	return f, err
}
