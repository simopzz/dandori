package planning

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Plan page and the Plan Proposal pages.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

// dateLayout is how a Cycle's dates are typed; longDate and shortDate how
// they are shown.
const (
	dateLayout = time.DateOnly
	longDate   = "Mon 2 Jan 2006"
	shortDate  = "Mon 2 Jan"
)

// Show shows the Plan page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, PlanPage{}, http.StatusOK)
}

// render shows the Plan page p, filled in with the Cycles, keeping a
// rejected open or change form p carries.
func (h Handlers) render(w http.ResponseWriter, r *http.Request, p PlanPage, status int) {
	o, err := h.Service.Overview(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load plan", err)
		return
	}
	change := CycleForm{}
	if p.Upcoming != nil {
		change = p.Upcoming.Change
	}
	n := 1
	if o.Current != nil {
		p.Current = cycleView(*o.Current, n)
		if o.Previous != nil {
			p.Current.PreviousID = o.Previous.ID
		}
		n += 3
		if o.PlanNext {
			p.PlanNext = o.Current.Cycle.EndsOn.Format(longDate)
		}
	}
	p.Upcoming = nil
	if o.Upcoming != nil {
		p.Upcoming = cycleView(*o.Upcoming, n)
		p.Upcoming.Change = deliveryValues(o.Upcoming.Cycle.Delivery)
		if change.Values != nil {
			p.Upcoming.Change = change
		}
		n += 3
	}
	p.BetweenNumber = max(n, 2)
	for _, es := range o.Between {
		state, more := describeException(*es.Exception, o.MealOut)
		if more != "" {
			state += " · " + more
		}
		p.Between = append(p.Between, BetweenRow{
			ID:    slotID(es.Slot),
			Slot:  es.Slot.String(),
			State: state,
			Day:   es.Slot.On.Format(dateLayout),
			Kind:  string(es.Slot.Kind),
		})
	}
	templ.Handler(Page(p), templ.WithStatus(status)).ServeHTTP(w, r)
}

func cycleView(cp CyclePlan, number int) *CycleView {
	c := cp.Cycle
	v := &CycleView{
		ID:                   c.ID,
		Number:               number,
		Delivery:             describeDelivery(c.Delivery, longDate),
		Ordered:              c.Delivery.Ordered,
		Starts:               c.Start.String(),
		Ends:                 c.EndsOn.Format(longDate),
		NeedsProposalBecause: c.NeedsProposalBecause,
	}
	if cp.Plan != nil {
		v.HasPlan, v.Calendar = true, calendarOf(*cp.Plan)
	}
	if cp.Planning.CanPlan {
		v.PlanAction = planURL(c.ID)
	}
	if run := cp.Planning.Run; run != nil {
		switch {
		case run.Running():
			v.PlanningRun = runURL(run.ID)
		case cp.Planning.CanPlan && run.Failure() != "":
			v.PlanningRun, v.PlanningFailed = runURL(run.ID), run.Failure()
		}
	}
	for _, pr := range cp.Proposals {
		v.Proposals = append(v.Proposals, ProposalRow{ID: pr.ID, Title: proposalTitle(pr.Origin), Status: pr.Status})
	}
	return v
}

// describeDelivery is d's date, in layout, and window, e.g. "Thu 12 Nov,
// 18:00–20:00".
func describeDelivery(d domain.Delivery, layout string) string {
	return d.On.Format(layout) + ", " + d.From.String() + "–" + d.To.String()
}

// deliveryValues are d as the change-the-Delivery form shows it.
func deliveryValues(d domain.Delivery) CycleForm {
	ordered := ""
	if d.Ordered {
		ordered = "yes"
	}
	return CycleForm{Values: map[string]string{
		"delivery_on": d.On.Format(dateLayout),
		"window_from": d.From.String(),
		"window_to":   d.To.String(),
		"ordered":     ordered,
	}}
}

// typed is what was typed in form's fields names, to show a rejected form
// back as it was.
func typed(form url.Values, names ...string) map[string]string {
	values := map[string]string{}
	for _, name := range names {
		values[name] = form.Get(name)
	}
	return values
}

// cycleProblems turns err, from opening a Cycle or changing its Delivery,
// into what is wrong keyed by form field name, adding to problems. It
// returns false for an error that isn't the user's to fix.
func cycleProblems(err error, problems map[string]string) bool {
	var invalid *InvalidError
	var overlap *OverlapError
	switch {
	case errors.As(err, &invalid):
		if msg := invalid.Fields["EndsOn"]; msg != "" {
			problems["ends_on"] = msg
		}
		if msg := invalid.Fields["Delivery.To"]; msg != "" {
			problems["window_to"] = msg
		}
	case errors.As(err, &overlap):
		problems["delivery_on"] = "Must be after the current Cycle's delivery on " + overlap.Current.Format(longDate)
	case err != nil:
		return false
	}
	return true
}

// OpenCycle opens a Cycle.
func (h Handlers) OpenCycle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	d, problems := readDelivery(r.PostForm)
	endsOn := domain.DefaultCycleEnd(d.On)
	if ends := strings.TrimSpace(r.PostForm.Get("ends_on")); ends != "" {
		if t, err := time.Parse(dateLayout, ends); err != nil {
			problems["ends_on"] = "Must be a date"
		} else {
			endsOn = t
		}
	}
	var id int64
	if len(problems) == 0 {
		var err error
		switch id, err = h.Service.OpenCycle(r.Context(), d, endsOn); {
		case errors.Is(err, ErrCycleUpcoming):
			h.render(w, r, PlanPage{Problem: "A Cycle is already upcoming: change its Delivery instead."}, http.StatusConflict)
			return
		case !cycleProblems(err, problems):
			httpx.ServerError(w, h.Log, "open cycle", err)
			return
		}
	}
	if len(problems) > 0 {
		values := typed(r.PostForm, "delivery_on", "window_from", "window_to", "ends_on")
		h.render(w, r, PlanPage{Open: CycleForm{Values: values, Errors: problems}}, http.StatusUnprocessableEntity)
		return
	}
	// Logging the Exceptions already known is the first step of planning.
	http.Redirect(w, r, exceptionsURL(id), http.StatusSeeOther)
}

// ChangeDelivery moves the upcoming Cycle's Delivery or marks it ordered.
func (h Handlers) ChangeDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	d, problems := readDelivery(r.PostForm)
	d.Ordered = r.PostForm.Get("ordered") == "yes"
	if len(problems) == 0 {
		switch err := h.Service.ChangeDelivery(r.Context(), id, d); {
		case errors.Is(err, ErrNotUpcoming):
			h.render(w, r, PlanPage{Problem: "That Cycle's Delivery has arrived, or the Cycle was dropped: it can no longer be changed."}, http.StatusConflict)
			return
		case !cycleProblems(err, problems):
			httpx.ServerError(w, h.Log, "change delivery", err)
			return
		}
	}
	if len(problems) > 0 {
		values := typed(r.PostForm, "delivery_on", "window_from", "window_to", "ordered")
		h.render(w, r, PlanPage{Upcoming: &CycleView{Change: CycleForm{Values: values, Errors: problems}}}, http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/plan", http.StatusSeeOther)
}

// Drop drops the upcoming Cycle.
func (h Handlers) Drop(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	switch err := h.Service.Drop(r.Context(), id); {
	case errors.Is(err, ErrNotUpcoming):
		h.render(w, r, PlanPage{Problem: "That Cycle's Delivery has arrived, or the Cycle was already dropped."}, http.StatusConflict)
	case errors.Is(err, ErrDeliveryOrdered):
		h.render(w, r, PlanPage{Problem: "The Delivery was ordered, so its Cycle can't be dropped."}, http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "drop cycle", err)
	default:
		http.Redirect(w, r, "/plan", http.StatusSeeOther)
	}
}

// RemoveExceptionBetweenCycles removes the Exception of a Slot still to
// come that no Cycle holds.
func (h Handlers) RemoveExceptionBetweenCycles(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slot, ok := readSlotField(r.PostForm)
	if !ok {
		http.Error(w, "bad slot", http.StatusBadRequest)
		return
	}
	switch err := h.Service.RemoveExceptionBetweenCycles(r.Context(), slot); {
	case errors.Is(err, ErrSlotUnavailable):
		h.render(w, r, PlanPage{Problem: "A Cycle holds that Slot now: change it on that Cycle's Exceptions page."}, http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "remove exception", err)
	default:
		http.Redirect(w, r, "/plan", http.StatusSeeOther)
	}
}

// readDelivery reads a Delivery's date and window from form, returning
// what is wrong with them keyed by form field name.
func readDelivery(form url.Values) (domain.Delivery, map[string]string) {
	problems := map[string]string{}
	var d domain.Delivery
	if on := strings.TrimSpace(form.Get("delivery_on")); on == "" {
		problems["delivery_on"] = "Required"
	} else if t, err := time.Parse(dateLayout, on); err != nil {
		problems["delivery_on"] = "Must be a date"
	} else {
		d.On = t
	}
	for name, to := range map[string]*domain.TimeOfDay{"window_from": &d.From, "window_to": &d.To} {
		raw := strings.TrimSpace(form.Get(name))
		if raw == "" {
			problems[name] = "Required"
			continue
		}
		t, err := domain.ParseTimeOfDay(raw)
		if err != nil {
			problems[name] = "Must be a time"
			continue
		}
		*to = t
	}
	return d, problems
}

// ShowProposal shows a Plan Proposal.
func (h Handlers) ShowProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	h.renderProposal(w, r, id, "", http.StatusOK)
}

// renderProposal shows the Plan Proposal with id, with problem, why the
// user's decision was refused, if there is one.
func (h Handlers) renderProposal(w http.ResponseWriter, r *http.Request, id int64, problem string, status int) {
	p, err := h.Service.Proposal(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "load plan proposal", err)
		return
	}
	page := ProposalPage{
		ID:         p.ID,
		Title:      proposalTitle(p.Origin) + " for the Cycle delivered " + p.Cycle.Delivery.On.Format(longDate),
		Status:     p.Status,
		Note:       p.Note,
		Unreadable: p.Unreadable,
		Problem:    problem,
	}
	if p.Unreadable == "" {
		page.Calendar, page.Sessions = calendarOf(p.Check), prepSessionRows(p.Check.Plan)
	}
	if p.RunID != 0 {
		page.Run = runURL(p.RunID)
	}
	if p.ReplanID != 0 {
		page.Replan = runURL(p.ReplanID)
	}
	for _, d := range p.Drafts {
		row := DraftRow{Name: d.Recipe.Name, Link: fmt.Sprintf("/plan-proposals/%d/recipe-drafts/%d", p.ID, d.ID)}
		if d.RecipeID != 0 {
			row.Accepted, row.Link = true, fmt.Sprintf("/recipes/%d", d.RecipeID)
		}
		page.Drafts = append(page.Drafts, row)
	}
	for _, v := range p.Violations {
		page.Violations = append(page.Violations, v.Message)
	}
	templ.Handler(ProposalView(page), templ.WithStatus(status)).ServeHTTP(w, r)
}

// Approve approves a Plan Proposal, making it its Cycle's Plan.
func (h Handlers) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var unreadable *UnreadableError
	switch err := h.Service.Approve(r.Context(), id); {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, ErrDecided):
		h.renderProposal(w, r, id, "This Plan Proposal was already decided.", http.StatusConflict)
	case errors.Is(err, ErrCycleClosed):
		h.renderProposal(w, r, id, "This Plan Proposal's Cycle has closed: a later Delivery started the next one.", http.StatusConflict)
	case errors.Is(err, ErrStartsInPast):
		h.renderProposal(w, r, id, "This Plan Proposal starts in the past, so it was not approved: what was prepared and eaten since stays.", http.StatusConflict)
	case errors.Is(err, ErrFillsExceptedSlot):
		h.renderProposal(w, r, id, "This Plan Proposal fills a Slot logged as a Meal Out or skipped, so it was not approved: ask for a new one.", http.StatusConflict)
	case errors.As(err, &unreadable):
		h.renderProposal(w, r, id, "This Plan Proposal can't be read, so it was not approved: "+unreadable.Reason+".", http.StatusConflict)
	case errors.Is(err, ErrInapplicable):
		h.renderProposal(w, r, id, "This Plan Proposal no longer fits what is kept, so it was not approved; the Plan is unchanged.", http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "approve plan proposal", err)
	default:
		http.Redirect(w, r, "/plan", http.StatusSeeOther)
	}
}

// Plan starts a planning Run for a Cycle, and sends the user to it.
func (h Handlers) Plan(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	run, err := h.Service.StartPlanning(r.Context(), id)
	var cannot *CannotPlanError
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
	case errors.As(err, &cannot) && cannot.RunID != 0:
		// Planned twice, as by a double tap: follow the Run already going.
		http.Redirect(w, r, runURL(cannot.RunID), http.StatusSeeOther)
	case errors.As(err, &cannot):
		h.render(w, r, PlanPage{Problem: "That Cycle can't be planned: " + cannot.Reason + "."}, http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "start planning", err)
	default:
		http.Redirect(w, r, runURL(run), http.StatusSeeOther)
	}
}

// planURL is where a Cycle's planning Run is started.
func planURL(id int64) string { return fmt.Sprintf("/cycles/%d/plan", id) }

// runURL is a Run's page, with its Trace.
func runURL(id int64) string { return fmt.Sprintf("/runs/%d", id) }

// Reject rejects a Plan Proposal, keeping the user's note.
func (h Handlers) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	switch _, err := h.Service.Reject(r.Context(), id, strings.TrimSpace(r.PostForm.Get("note"))); {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, ErrDecided):
		h.renderProposal(w, r, id, "This Plan Proposal was already decided.", http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "reject plan proposal", err)
	default:
		http.Redirect(w, r, fmt.Sprintf("/plan-proposals/%d", id), http.StatusSeeOther)
	}
}

func proposalTitle(origin string) string {
	if origin == "substitution" {
		return "Substitution Plan Proposal"
	}
	return "Initial Plan Proposal"
}

func formatNutrition(n domain.Nutrition) string {
	return fmt.Sprintf("%d kcal · %d g protein · %d g carbs · %d g fat", n.Kcal, n.ProteinG, n.CarbsG, n.FatG)
}

// calendarOf is every day of c's Cycle with its nutrition and what fills
// its Slots.
func calendarOf(c domain.PlanCheck) []CalendarDay {
	slots := map[string][]SlotRow{}
	add := func(s domain.Slot, name string) {
		day := s.On.Format(dateLayout)
		slots[day] = append(slots[day], SlotRow{Kind: string(s.Kind), What: name})
	}
	for _, ps := range c.Plan.PrepSessions {
		for _, p := range ps.Portions {
			if p.EatAt != nil {
				add(*p.EatAt, ps.Name())
			}
		}
	}
	kept := map[int64]string{}
	for _, k := range c.Kept {
		kept[k.ID] = k.Name
	}
	for _, k := range c.Plan.Kept {
		name, ok := kept[k.PortionID]
		if !ok {
			name = fmt.Sprintf("Portion %d", k.PortionID)
		}
		add(k.Slot, name)
	}
	// An excepted Slot shows its Exception, and what the Plan puts there
	// instead, uneaten.
	for _, e := range c.Exceptions {
		if !c.Cycle.Contains(e.Slot) {
			continue
		}
		rows := slots[e.Slot.On.Format(dateLayout)]
		i := slices.IndexFunc(rows, func(r SlotRow) bool { return r.Kind == string(e.Slot.Kind) })
		if i < 0 {
			add(e.Slot, e.Kind.Describe())
			continue
		}
		rows[i].What = e.Kind.Describe() + ", not " + rows[i].What
	}

	var days []CalendarDay
	for _, d := range c.Days() {
		day := CalendarDay{Date: d.On.Format(shortDate), Eaten: formatNutrition(d.Eaten)}
		if t := d.TrailingSeven; t != nil {
			day.Trailing = fmt.Sprintf("%d kcal · %d g protein", t.Kcal, t.ProteinG)
		}
		day.Slots = slots[d.On.Format(dateLayout)]
		slices.SortStableFunc(day.Slots, func(a, b SlotRow) int {
			return slices.Index(domain.SlotKinds, domain.SlotKind(a.Kind)) - slices.Index(domain.SlotKinds, domain.SlotKind(b.Kind))
		})
		days = append(days, day)
	}
	return days
}

func prepSessionRows(plan domain.Plan) []PrepSessionRow {
	sessions := slices.Clone(plan.PrepSessions)
	slices.SortStableFunc(sessions, func(a, b domain.PrepSession) int {
		switch {
		case a.At.Before(b.At):
			return -1
		case b.At.Before(a.At):
			return 1
		}
		return 0
	})
	var rows []PrepSessionRow
	for _, ps := range sessions {
		var locations []string
		for _, p := range ps.Portions {
			locations = append(locations, string(p.Location))
		}
		portions := fmt.Sprintf("%d Portions", len(ps.Portions))
		if len(ps.Portions) == 1 {
			portions = "1 Portion"
		}
		rows = append(rows, PrepSessionRow{What: ps.Name(), At: ps.At.String(), Portions: portions, Locations: strings.Join(locations, ", ")})
	}
	return rows
}
