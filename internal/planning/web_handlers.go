package planning

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/recipes"
	"github.com/simopzz/dandori/internal/runs"
	"github.com/simopzz/dandori/internal/web"
)

// Why the app refuses opening, planning or deciding on a Cycle.
const (
	refusedCycle    httpx.Refusal = "Cycle refused"
	refusedUpcoming httpx.Refusal = "a Cycle is already upcoming"
	refusedTargets  httpx.Refusal = "the Targets aren't set"
	refusedBusy     httpx.Refusal = "another planning Run is going or a Plan Proposal is pending"
	refusedPast     httpx.Refusal = "the Plan Proposal's first Slot has passed"
	refusedPlanned  httpx.Refusal = "the Cycle already has a Plan"
	refusedNoneLeft httpx.Refusal = "no Slot of the Cycle is left to plan"
	refusedClash    httpx.Refusal = "a Recipe Draft clashes with the Recipe Library"
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

// RunPage is what the Runs pages tell of a planning Run: a succeeded one left a Plan
// Proposal, which the Cycle page shows.
func (h Handlers) RunPage() runs.KindPage {
	return runs.KindPage{
		Kind:        planningKind,
		Tab:         web.PlanTab,
		About:       h.about,
		Name:        msgPlanningKind,
		ResultLabel: msgPlanProposal,
		ResultURL:   func(context.Context, int64) (string, error) { return "/plan", nil },
		Reasons:     planningReasons,
	}
}

// about is what a planning Run's page tells of its Cycle, while it is open: its name with
// a link back, and after a failure, if the Run is the Cycle's latest, Plan again.
func (h Handlers) about(ctx context.Context, run runs.Run) (templ.Component, error) {
	c, latest, err := h.Service.CycleOfRun(ctx, run.ID, h.Clock.Now())
	if errors.Is(err, ErrNoCycle) {
		return nil, nil
	}
	return runAbout(c, run.Status == runs.Failed && latest), err
}

// Show is the Cycle page: the upcoming Cycle, or the current one when none is, with how
// planning it stands, or that there's none.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	h.cycle(w, r, http.StatusOK, nil)
}

// cycle answers with the Cycle page, with status, saying why a decision or planning was
// refused, if it was, with any Planning Note posted still typed.
func (h Handlers) cycle(w http.ResponseWriter, r *http.Request, status int, refusal error) {
	ctx, now := r.Context(), h.Clock.Now()
	current, upcoming, err := h.Service.Now(ctx, now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the User's Cycles", err)
		return
	}
	v := cycleView{Cycle: cmp.Or(upcoming, current), Upcoming: upcoming != nil, Refusal: refusal, Note: r.PostFormValue("note")}
	if v.Cycle != nil {
		if v.Planning, err = h.Service.PlanningOf(ctx, v.Cycle.ID, now.Location()); err != nil {
			httpx.ServerError(w, r, h.Log, "load how planning the Cycle stands", err)
			return
		}
		if v.Planning.Failed != nil {
			v.Failure = h.RunPage().Failure(ctx, *v.Planning.Failed)
		}
	}
	httpx.Render(w, r, h.Log, status, cyclePage(now, v))
}

// Approve approves the Plan Proposal in the path, landing on the Cycle page and its
// Plan, or answers with the Cycle page saying why it can't be.
func (h Handlers) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	err := h.Service.Approve(r.Context(), id, h.Clock.Now())
	if err != nil {
		h.refused(w, r, "approve a Plan Proposal", err)
		return
	}
	web.Answer{Notice: web.T(r.Context(), msgApproved), RedirectTo: "/plan"}.Write(w, r, h.Log)
}

// Reject rejects the Plan Proposal in the path: with a Planning Note it lands on the
// planning Run it starts, without one on the Cycle page. It answers with the Cycle page
// saying why it can't.
func (h Handlers) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	runID, err := h.Service.Reject(r.Context(), id, r.PostFormValue("note"), h.Clock.Now())
	switch {
	case err != nil:
		h.refused(w, r, "reject a Plan Proposal", err)
	case runID != 0:
		web.Answer{RedirectTo: fmt.Sprintf("/runs/%d", runID)}.Write(w, r, h.Log)
	default:
		web.Answer{Notice: web.T(r.Context(), msgRejected), RedirectTo: "/plan"}.Write(w, r, h.Log)
	}
}

// PlanAgain starts another planning Run of the Cycle in the path, told the Planning Note
// posted, if any, landing on it, or answers with the Cycle page saying why it can't.
func (h Handlers) PlanAgain(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	runID, err := h.Service.PlanAgain(r.Context(), id, r.PostFormValue("note"), h.Clock.Now())
	if err != nil {
		h.refused(w, r, "plan a Cycle again", err)
		return
	}
	web.Answer{RedirectTo: fmt.Sprintf("/runs/%d", runID)}.Write(w, r, h.Log)
}

// refused answers a decision or a replan that failed with err, doing what: unknown when
// there's nothing to decide or plan, the Cycle page saying why when a rule refused it.
func (h Handlers) refused(w http.ResponseWriter, r *http.Request, what string, err error) {
	var refusal httpx.Refusal
	switch {
	case errors.Is(err, ErrNoProposal), errors.Is(err, ErrNoCycle), errors.Is(err, ErrNoFailure):
		http.NotFound(w, r)
		return
	case errors.Is(err, ErrStartsInThePast):
		refusal = refusedPast
	case errors.Is(err, ErrBusy):
		refusal = refusedBusy
	case errors.Is(err, ErrPlanned):
		refusal = refusedPlanned
	case errors.Is(err, ErrNothingLeft):
		refusal = refusedNoneLeft
	case clashed(err):
		refusal = refusedClash
	default:
		httpx.ServerError(w, r, h.Log, what, err)
		return
	}
	httpx.Refused(r, refusal, nil)
	h.cycle(w, r, http.StatusUnprocessableEntity, err)
}

// ShowDraft is the pending Plan Proposal's Recipe Draft in the path, as a page.
func (h Handlers) ShowDraft(w http.ResponseWriter, r *http.Request) { h.showDraft(w, r, false) }

// ShowDraftSheet is ShowDraft as the body of the sheet over the Cycle page.
func (h Handlers) ShowDraftSheet(w http.ResponseWriter, r *http.Request) { h.showDraft(w, r, true) }

// showDraft answers with the Recipe Draft in the path: as its sheet's body when sheet,
// else as a page. One the User's pending Plan Proposal doesn't prepare is unknown.
func (h Handlers) showDraft(w http.ResponseWriter, r *http.Request, sheet bool) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	rec, err := h.Service.Drafted(r.Context(), id)
	if errors.Is(err, ErrNoProposal) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load a Recipe Draft", err)
		return
	}
	if sheet {
		httpx.Render(w, r, h.Log, http.StatusOK, recipes.Planned(rec))
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, draftPage(h.Clock.Now(), rec))
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
	c, err := h.Service.Find(ctx, id, now)
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
	case errors.Is(err, ErrBusy):
		httpx.Refused(r, refusedBusy, nil)
		h.check(w, r, http.StatusUnprocessableEntity, err)
	case err != nil:
		h.refused(w, r, "start planning the Cycle", err)
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
