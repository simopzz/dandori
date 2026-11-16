package planning

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/web"
)

// Why the app refuses opening a Cycle.
const (
	refusedCycle    httpx.Refusal = "Cycle refused"
	refusedUpcoming httpx.Refusal = "a Cycle is already upcoming"
	refusedTargets  httpx.Refusal = "the Targets aren't set"
	refusedBusy     httpx.Refusal = "another planning Run is going or a Plan Proposal is pending"
)

// cycleFields are the fields of opening a Cycle, in the form's order.
var cycleFields = []string{domain.CycleDelivery, domain.CycleFrom, domain.CycleTo, domain.CycleEnds}

// Handlers serve the Cycle page, opening a Cycle, dropping it, and the Kitchen Check
// that starts planning it.
type Handlers struct {
	Service *Service
	Kitchen *kitchen.Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// Show is the Cycle page: the upcoming Cycle with how planning it stands, or that
// there's none.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	_, upcoming, err := h.Service.Now(ctx, now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Cycles", err)
		return
	}
	var p Planning
	if upcoming != nil {
		if p, err = h.Service.PlanningOf(ctx, upcoming.ID, now.Location()); err != nil {
			httpx.ServerError(w, r, h.Log, "load how planning the Cycle stands", err)
			return
		}
	}
	httpx.Render(w, r, h.Log, http.StatusOK, cyclePage(now, upcoming, p))
}

// ShowCheck is the Kitchen Check before planning the upcoming Cycle in the path.
func (h Handlers) ShowCheck(w http.ResponseWriter, r *http.Request) {
	h.check(w, r, http.StatusOK, nil)
}

// check answers with the Kitchen Check of the upcoming Cycle in the path, with status,
// saying why planning is refused, if it is. A Cycle not upcoming is unknown.
func (h Handlers) check(w http.ResponseWriter, r *http.Request, status int, refusal error) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx, now := r.Context(), h.Clock.Now()
	c, err := h.Service.Upcoming(ctx, id, now)
	if errors.Is(err, ErrNoCycle) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Cycle to plan", err)
		return
	}
	contents, err := h.Kitchen.Contents(ctx, now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "add up the Kitchen Ledger", err)
		return
	}
	httpx.Render(w, r, h.Log, status, checkPage(now, c, contents, refusal))
}

// Plan confirms the Kitchen Check of the upcoming Cycle in the path and starts its
// planning Run, landing on the Run's page and its live Trace, or answers with the
// Kitchen Check saying why it can't.
func (h Handlers) Plan(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	runID, err := h.Service.StartPlanning(r.Context(), id, h.Clock.Now())
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
	case errors.Is(err, ErrBusy):
		httpx.Refused(r, refusedBusy, nil)
		h.check(w, r, http.StatusConflict, err)
	case err != nil:
		httpx.ServerError(w, r, h.Log, "start planning the Cycle", err)
	default:
		web.Answer{RedirectTo: fmt.Sprintf("/runs/%d", runID)}.Write(w, r, h.Log)
	}
}

// ShowOpen is the form opening a Cycle, saying first why it would be refused, if it
// would be.
func (h Handlers) ShowOpen(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	refusal := h.Service.Refusal(r.Context(), now)
	if refusal != nil && !errors.Is(refusal, ErrUpcoming) && !errors.Is(refusal, ErrNoTargets) {
		httpx.ServerError(w, r, h.Log, "check the User can open a Cycle", refusal)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, openPage(now, OpenForm{Refusal: refusal}))
}

// Open opens the Cycle posted, landing on its Kitchen Check, or answers with the form as
// typed, refused on each field or as a whole.
func (h Handlers) Open(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	typed := map[string]string{}
	for _, field := range cycleFields {
		typed[field] = r.PostFormValue(field)
	}
	id, problems, err := h.Service.Open(ctx, web.LanguageOf(ctx), typed, now)
	form := OpenForm{Values: typed, Errs: map[string]string{}, Refusal: err}
	switch {
	case errors.Is(err, ErrUpcoming):
		httpx.Refused(r, refusedUpcoming, nil)
	case errors.Is(err, ErrNoTargets):
		httpx.Refused(r, refusedTargets, nil)
	case err != nil:
		httpx.ServerError(w, r, h.Log, "open a Cycle", err)
		return
	case len(problems) == 0:
		web.Answer{Notice: web.T(ctx, msgOpened), RedirectTo: string(checkURL(id))}.Write(w, r, h.Log)
		return
	default:
		for field, problem := range problems {
			form.Errs[field] = web.T(ctx, web.ProblemMessage(problem, cycleProblems))
		}
		httpx.Refused(r, refusedCycle, form.Errs)
	}
	web.Answer{Status: http.StatusUnprocessableEntity, Page: openPage(now, form)}.Write(w, r, h.Log)
}

// ShowDrop is the step asking before dropping the upcoming Cycle in the path, as a page.
func (h Handlers) ShowDrop(w http.ResponseWriter, r *http.Request) { h.showDrop(w, r, false) }

// ShowDropSheet is ShowDrop as the body of the sheet over the Cycle page.
func (h Handlers) ShowDropSheet(w http.ResponseWriter, r *http.Request) { h.showDrop(w, r, true) }

// showDrop answers with the step asking before dropping the Cycle in the path: as its
// sheet's body when sheet, else as a page. A Cycle the User has none upcoming of
// is unknown.
func (h Handlers) showDrop(w http.ResponseWriter, r *http.Request, sheet bool) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	now := h.Clock.Now()
	c, err := h.Service.Upcoming(r.Context(), id, now)
	if errors.Is(err, ErrNoCycle) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Cycle to drop", err)
		return
	}
	if sheet {
		httpx.Render(w, r, h.Log, http.StatusOK, dropStep(c, ""))
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, dropPage(now, c))
}

// Drop drops the upcoming Cycle in the path, landing on the Cycle page.
func (h Handlers) Drop(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	err := h.Service.Drop(r.Context(), id, h.Clock.Now())
	if errors.Is(err, ErrNoCycle) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "drop a Cycle", err)
		return
	}
	web.Answer{Notice: web.T(r.Context(), msgDropped), RedirectTo: "/plan"}.Write(w, r, h.Log)
}
