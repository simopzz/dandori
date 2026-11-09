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
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/web"
)

// movementSaved is the event that tells the Kitchen page to list its stock again.
const movementSaved = "movement-saved"

// Handlers bring the signed-in User's kitchen up to now before every page, so a page is
// never wrong because the scheduler was late, and serve the Kitchen page.
type Handlers struct {
	Service *Service
	Ledger  *ledger.Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// UpToNow brings the signed-in User's kitchen up to now, then serves next.
func (h Handlers) UpToNow(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.Ledger.BringUpToNow(r.Context(), h.Clock.Now()); err != nil {
			httpx.ServerError(w, r, h.Log, "bring the kitchen up to now", err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Show is the Kitchen page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	contents, err := h.Service.Contents(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	f, err := h.startCount(r.Context(), now, url.Values{})
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a Count", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, page(now, contents, f))
}

// ShowContents is what the Kitchen page lists alone, which it swaps in after each
// write to the Kitchen Ledger.
func (h Handlers) ShowContents(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	contents, err := h.Service.Contents(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, contentsList(now, contents))
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
	rec, err := h.Service.Count(ctx, now, id, domain.StorageLocation(r.PostFormValue("location")), r.PostFormValue("stated"), r.PostFormValue("note"))
	switch {
	case err != nil && !errors.Is(err, ErrStaple):
		httpx.ServerError(w, r, h.Log, "save a Count", err)
		return
	case rec == nil:
		http.NotFound(w, r)
		return
	}
	ing := rec.Ingredient
	f := postedCountForm(r, ing, rec.Problems)
	if errors.Is(err, ErrStaple) {
		f.Errs["stated"] = web.Tf(ctx, msgStaple, ing.Name)
	}
	if len(f.Errs) > 0 {
		f.Believed = rec.Held
		web.Answer{Status: http.StatusUnprocessableEntity, Page: countPage(now, f), Fragment: countBody(f)}.Write(w, r, h.Log)
		return
	}
	a := web.Answer{
		Trigger: movementSaved, RedirectTo: "/kitchen/count",
		Notice: web.Tf(ctx, msgCounted, ing.Name, web.Quantity(ctx, rec.Movement.Stated, ing.Unit), web.T(ctx, web.InLocations[rec.Movement.Key.Location])),
	}
	if !a.Redirects(r) {
		cleared, err := h.startCount(ctx, now, nil)
		if err != nil {
			httpx.ServerError(w, r, h.Log, "start a Count", err)
			return
		}
		a.Fragment = countBody(cleared)
	}
	a.Write(w, r, h.Log)
}

// ShowPortions is the Add Portions form as a page of its own, for a browser without htmx.
func (h Handlers) ShowPortions(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, portionsPage(h.Clock.Now(), PortionsForm{}))
}

// ShowPortionsSheet is the Add Portions sheet's body.
func (h Handlers) ShowPortionsSheet(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, portionsForm(PortionsForm{}))
}

// AddPortions adds the Portions posted, at the moment they arrive. Posted from the sheet
// by htmx, it answers with them confirmed in a floating Notice, closing the sheet and
// telling the page under it to list its stock again; or with the form as typed with why
// it was refused. A plain post lands on the Kitchen page, confirming it.
func (h Handlers) AddPortions(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	typed := postedPortions(r)
	p, problems, err := h.Service.AddPortions(ctx, now, typed)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add Portions", err)
		return
	}
	if len(problems) > 0 {
		f := refusedPortionsForm(ctx, typed, problems)
		web.Answer{Status: http.StatusUnprocessableEntity, Page: portionsPage(now, f), Fragment: portionsForm(f)}.Write(w, r, h.Log)
		return
	}
	web.Answer{
		Trigger: movementSaved + ", " + web.CloseSheet, RedirectTo: "/kitchen",
		Notice: web.Tf(ctx, msgAddedPortions, p.Name, p.HowMany, web.T(ctx, toLocations[p.Location])),
	}.Write(w, r, h.Log)
}

// askedLot reads which Portion Lot a request asks for: one Portion of it and its Storage
// Location. With neither, it answers r itself and ok is false.
func askedLot(w http.ResponseWriter, r *http.Request, portion, location string) (_ domain.LotKey, ok bool) {
	id, err := strconv.ParseInt(portion, 10, 64)
	key := domain.LotKey{Portion: id, Location: domain.StorageLocation(location)}
	if err != nil || !slices.Contains(domain.StorageLocations, key.Location) {
		http.NotFound(w, r)
		return domain.LotKey{}, false
	}
	return key, true
}

// lotMovements is the Portion Lot r's query asks for with its Movements. With no such
// Portion, or on a failure, it answers r itself and is nil.
func (h Handlers) lotMovements(w http.ResponseWriter, r *http.Request) *LotMovements {
	key, ok := askedLot(w, r, r.URL.Query().Get("portion"), r.URL.Query().Get("location"))
	if !ok {
		return nil
	}
	item, err := h.Service.LotMovements(r.Context(), h.Clock.Now(), key)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load a Portion Lot's Movements", err)
		return nil
	}
	if item == nil {
		http.NotFound(w, r)
	}
	return item
}

// ShowLotMovements is a Portion Lot's Movements as a page of their own, for a browser
// without htmx.
func (h Handlers) ShowLotMovements(w http.ResponseWriter, r *http.Request) {
	if item := h.lotMovements(w, r); item != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, lotMovementsPage(h.Clock.Now(), *item))
	}
}

// ShowLotMovementsSheet is the Movements sheet's body: a Portion Lot's Movements.
func (h Handlers) ShowLotMovementsSheet(w http.ResponseWriter, r *http.Request) {
	if item := h.lotMovements(w, r); item != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, lotMovementsBody(h.Clock.Now(), *item))
	}
}

// startLotForm is the form for the Portion Lot r's query asks for. Holding none, it can't
// be counted or moved; then, or on a failure, it answers r itself and is nil.
func (h Handlers) startLotForm(w http.ResponseWriter, r *http.Request) *PortionLotForm {
	key, ok := askedLot(w, r, r.URL.Query().Get("portion"), r.URL.Query().Get("location"))
	if !ok {
		return nil
	}
	lot, err := h.Service.PortionLot(r.Context(), h.Clock.Now(), key)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a form for a Portion Lot", err)
		return nil
	}
	if lot == nil || lot.Count() == 0 {
		http.NotFound(w, r)
		return nil
	}
	return &PortionLotForm{Portion: key.Portion, Lot: *lot}
}

// ShowPortionCount is the form that counts a Portion Lot as a page of its own, for a
// browser without htmx.
func (h Handlers) ShowPortionCount(w http.ResponseWriter, r *http.Request) {
	if f := h.startLotForm(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, portionCountPage(h.Clock.Now(), *f))
	}
}

// ShowPortionCountSheet is the Count sheet's body: the form that counts a Portion Lot.
func (h Handlers) ShowPortionCountSheet(w http.ResponseWriter, r *http.Request) {
	if f := h.startLotForm(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, portionCountForm(*f))
	}
}

// CountPortions saves a Count of a Portion Lot at the moment it arrives. Posted from the
// sheet by htmx, it answers with the Count confirmed in a floating Notice, closing the
// sheet and telling the page under it to list its stock again; or with the form as typed
// with why it was refused. A plain post lands on the Kitchen page, confirming it. One
// that recorded nothing confirms nothing.
func (h Handlers) CountPortions(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	key, ok := askedLot(w, r, r.PostFormValue("portion"), r.PostFormValue("location"))
	if !ok {
		return
	}
	rec, err := h.Service.CountPortions(ctx, now, key, r.PostFormValue("stated"), r.PostFormValue("note"))
	switch {
	case err != nil:
		httpx.ServerError(w, r, h.Log, "save a Count of Portions", err)
		return
	case rec == nil:
		http.NotFound(w, r)
		return
	}
	if problem, refused := rec.Problems["stated"]; refused {
		f := postedLotForm(r, key.Portion, rec.Lot, "stated")
		f.Errs["stated"] = web.T(ctx, web.ProblemMessage(problem, portionCountProblemMessages))
		web.Answer{Status: http.StatusUnprocessableEntity, Page: portionCountPage(now, f), Fragment: portionCountForm(f)}.Write(w, r, h.Log)
		return
	}
	var notice string
	if len(rec.Movement.Portions) > 0 {
		notice = web.Tf(ctx, msgCounted, rec.Lot.Name, web.Quantity(ctx, rec.Movement.Stated, domain.PortionUnit), web.T(ctx, web.InLocations[key.Location]))
	}
	web.Answer{Trigger: movementSaved + ", " + web.CloseSheet, RedirectTo: "/kitchen", Notice: notice}.Write(w, r, h.Log)
}

// startLotMove is the form that moves the Portion Lot r's query asks for, all of it to
// start with, or nil as startLotForm is.
func (h Handlers) startLotMove(w http.ResponseWriter, r *http.Request) *PortionLotForm {
	f := h.startLotForm(w, r)
	if f != nil {
		f.Typed = strconv.Itoa(f.Lot.Count())
	}
	return f
}

// ShowPortionMove is the form that moves a Portion Lot as a page of its own, for a
// browser without htmx.
func (h Handlers) ShowPortionMove(w http.ResponseWriter, r *http.Request) {
	if f := h.startLotMove(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, portionMovePage(h.Clock.Now(), *f))
	}
}

// ShowPortionMoveSheet is the Move sheet's body: the form that moves a Portion Lot.
func (h Handlers) ShowPortionMoveSheet(w http.ResponseWriter, r *http.Request) {
	if f := h.startLotMove(w, r); f != nil {
		httpx.Render(w, r, h.Log, http.StatusOK, portionMoveForm(*f))
	}
}

// MovePortions saves a move of a Portion Lot at the moment it arrives, answering as
// Move does.
func (h Handlers) MovePortions(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	key, ok := askedLot(w, r, r.PostFormValue("portion"), r.PostFormValue("location"))
	if !ok {
		return
	}
	rec, err := h.Service.MovePortions(ctx, now, key, domain.StorageLocation(r.PostFormValue("to")), r.PostFormValue("quantity"), r.PostFormValue("note"))
	switch {
	case err != nil:
		httpx.ServerError(w, r, h.Log, "save a move of Portions", err)
		return
	case rec == nil:
		http.NotFound(w, r)
		return
	}
	if len(rec.Problems) > 0 {
		f := postedLotForm(r, key.Portion, rec.Lot, "quantity")
		f.Errs = moveErrs(ctx, rec.Problems, web.Tf(ctx, msgPortionsMoreThanHeld, rec.Lot.Count()), portionMoveProblemMessages)
		if f.Errs["to"] != "" {
			f.To = ""
		}
		web.Answer{Status: http.StatusUnprocessableEntity, Page: portionMovePage(now, f), Fragment: portionMoveForm(f)}.Write(w, r, h.Log)
		return
	}
	web.Answer{
		Trigger: movementSaved + ", " + web.CloseSheet, RedirectTo: "/kitchen",
		Notice: web.Tf(ctx, msgMovedPortions, rec.Lot.Name, len(rec.Movement.Portions), web.T(ctx, toLocations[rec.Movement.To])),
	}.Write(w, r, h.Log)
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
	holding, err := h.Service.Holding(r.Context(), h.Clock.Now(), id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "start a move", err)
		return nil
	}
	from := domain.StorageLocation(r.URL.Query().Get("location"))
	if holding == nil || holding.Held[from] <= 0 {
		http.NotFound(w, r)
		return nil
	}
	return &MoveForm{
		Ingredient: holding.Ingredient, From: from, Believed: holding.Held,
		Quantity: strconv.FormatFloat(holding.Held[from].Number(), 'f', -1, 64),
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
	if !slices.Contains(domain.StorageLocations, from) {
		http.NotFound(w, r)
		return
	}
	rec, err := h.Service.Move(ctx, now, id, from, domain.StorageLocation(r.PostFormValue("to")), r.PostFormValue("quantity"), r.PostFormValue("note"))
	switch {
	case err != nil:
		httpx.ServerError(w, r, h.Log, "save a move", err)
		return
	case rec == nil:
		http.NotFound(w, r)
		return
	}
	ing := rec.Ingredient
	if len(rec.Problems) > 0 {
		f := postedMoveForm(r, ing, from, rec.Held, rec.Problems)
		web.Answer{Status: http.StatusUnprocessableEntity, Page: movePage(now, f), Fragment: moveForm(f)}.Write(w, r, h.Log)
		return
	}
	web.Answer{
		Trigger: movementSaved + ", " + web.CloseSheet, RedirectTo: "/kitchen",
		Notice: web.Tf(ctx, msgMoved, web.Quantity(ctx, rec.Movement.Change, ing.Unit), ing.Name, web.T(ctx, toLocations[rec.Movement.To])),
	}.Write(w, r, h.Log)
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
	web.Answer{Fragment: countBody(f)}.Write(w, r, h.Log)
}

// startCount is the Count form as query asks for it: its Ingredient ingredient chosen, if
// the signed-in User has it and it is no Staple, in its Storage Location location or else
// its usual one; or else the search for q.
func (h Handlers) startCount(ctx context.Context, now time.Time, query url.Values) (CountForm, error) {
	f := CountForm{Query: strings.TrimSpace(query.Get("q"))}
	if id, err := strconv.ParseInt(query.Get("ingredient"), 10, 64); err == nil {
		holding, err := h.Service.Holding(ctx, now, id)
		if err != nil {
			return f, err
		}
		if holding != nil && !holding.Ingredient.Staple {
			ing := holding.Ingredient
			f.Ingredient, f.Location, f.Believed = &ing, ing.Usual, holding.Held
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
