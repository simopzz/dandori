package planning

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Why the app refuses opening a Cycle.
const (
	refusedCycle    httpx.Refusal = "Cycle refused"
	refusedUpcoming httpx.Refusal = "a Cycle is already upcoming"
	refusedTargets  httpx.Refusal = "the Targets aren't set"
)

// cycleFields are the fields of opening a Cycle, in the form's order.
var cycleFields = []string{domain.CycleDelivery, domain.CycleFrom, domain.CycleTo, domain.CycleEnds}

// Handlers serve the Cycle page, opening a Cycle and dropping it.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// Show is the Cycle page: the upcoming Cycle, or that there's none.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	_, upcoming, err := h.Service.Now(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Cycles", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, cyclePage(now, upcoming))
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

// Open opens the Cycle posted, landing on the Cycle page, or answers with the form as
// typed, refused on each field or as a whole.
func (h Handlers) Open(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), h.Clock.Now()
	typed := map[string]string{}
	for _, field := range cycleFields {
		typed[field] = r.PostFormValue(field)
	}
	problems, err := h.Service.Open(ctx, web.LanguageOf(ctx), typed, now)
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
		web.Answer{Notice: web.T(ctx, msgOpened), RedirectTo: "/plan"}.Write(w, r, h.Log)
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
