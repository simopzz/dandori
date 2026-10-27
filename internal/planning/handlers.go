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
	h.render(w, r, CycleForm{}, http.StatusOK)
}

func (h Handlers) render(w http.ResponseWriter, r *http.Request, open CycleForm, status int) {
	o, err := h.Service.Overview(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load plan", err)
		return
	}
	p := PlanPage{Open: open}
	if o.Cycle != nil {
		p.Cycle = &CycleView{Delivered: o.Cycle.DeliveryOn.Format(longDate), Ends: o.Cycle.EndsOn.Format(longDate)}
	}
	if o.Plan != nil {
		p.HasPlan, p.Calendar = true, calendarOf(*o.Plan)
	}
	for _, pr := range o.Proposals {
		p.Proposals = append(p.Proposals, ProposalRow{ID: pr.ID, Title: proposalTitle(pr.Origin), Status: pr.Status})
	}
	templ.Handler(Page(p), templ.WithStatus(status)).ServeHTTP(w, r)
}

// OpenCycle opens a Cycle.
func (h Handlers) OpenCycle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	c, problems := readCycle(r.PostForm)
	if len(problems) == 0 {
		var invalid *InvalidError
		var overlap *OverlapError
		switch err := h.Service.OpenCycle(r.Context(), c); {
		case errors.As(err, &invalid):
			if msg := invalid.Fields["EndsOn"]; msg != "" {
				problems["ends_on"] = msg
			}
		case errors.As(err, &overlap):
			problems["delivery_on"] = "Must be after the current Cycle's delivery on " + overlap.Current.Format(longDate)
		case err != nil:
			httpx.ServerError(w, h.Log, "open cycle", err)
			return
		}
	}
	if len(problems) > 0 {
		values := map[string]string{"delivery_on": r.PostForm.Get("delivery_on"), "ends_on": r.PostForm.Get("ends_on")}
		h.render(w, r, CycleForm{Values: values, Errors: problems}, http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/plan", http.StatusSeeOther)
}

// readCycle reads the open-a-Cycle form, returning the Cycle and what is
// wrong with it keyed by form field name. The end date defaults to ten
// days after delivery.
func readCycle(form url.Values) (domain.Cycle, map[string]string) {
	problems := map[string]string{}
	var c domain.Cycle
	delivery := strings.TrimSpace(form.Get("delivery_on"))
	if delivery == "" {
		problems["delivery_on"] = "Required"
	} else if d, err := time.Parse(dateLayout, delivery); err != nil {
		problems["delivery_on"] = "Must be a date"
	} else {
		c.DeliveryOn = d
		c.EndsOn = domain.DefaultCycleEnd(d)
	}
	if ends := strings.TrimSpace(form.Get("ends_on")); ends != "" {
		if d, err := time.Parse(dateLayout, ends); err != nil {
			problems["ends_on"] = "Must be a date"
		} else {
			c.EndsOn = d
		}
	}
	return c, problems
}

// ShowProposal shows a Plan Proposal.
func (h Handlers) ShowProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
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
		ID:       p.ID,
		Title:    proposalTitle(p.Origin) + " for the Cycle delivered " + p.Cycle.DeliveryOn.Format(longDate),
		Status:     p.Status,
		Note:       p.Note,
		Unreadable: p.Unreadable,
	}
	if p.Unreadable == "" {
		page.Calendar, page.Sessions = calendarOf(p.Check), prepSessionRows(p.Check.Plan)
	}
	for _, v := range p.Violations {
		page.Violations = append(page.Violations, v.Message)
	}
	templ.Handler(ProposalView(page)).ServeHTTP(w, r)
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
		http.Error(w, "This Plan Proposal was already decided.", http.StatusConflict)
	case errors.Is(err, ErrCycleClosed):
		http.Error(w, "This Plan Proposal's Cycle has closed: a later delivery started the next one.", http.StatusConflict)
	case errors.Is(err, ErrStartsInPast):
		http.Error(w, "This Plan Proposal starts in the past, so it was not approved: what was prepared and eaten since stays.", http.StatusConflict)
	case errors.As(err, &unreadable):
		http.Error(w, "This Plan Proposal can't be read, so it was not approved: "+unreadable.Reason+".", http.StatusConflict)
	case errors.Is(err, ErrInapplicable):
		http.Error(w, "This Plan Proposal no longer fits what is kept, so it was not approved; the Plan is unchanged.", http.StatusConflict)
	case err != nil:
		httpx.ServerError(w, h.Log, "approve plan proposal", err)
	default:
		http.Redirect(w, r, "/plan", http.StatusSeeOther)
	}
}

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
	switch err := h.Service.Reject(r.Context(), id, strings.TrimSpace(r.PostForm.Get("note"))); {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, ErrDecided):
		http.Error(w, "This Plan Proposal was already decided.", http.StatusConflict)
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
