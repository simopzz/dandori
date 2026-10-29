package planning

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

func exceptionsURL(id int64) string { return fmt.Sprintf("/cycles/%d/exceptions", id) }

// estimateFields are the form fields of a Meal Out's own estimate, by
// nutrient.
var estimateFields = []string{"estimate_kcal", "estimate_protein_g", "estimate_carbs_g", "estimate_fat_g"}

// ShowExceptions shows a Cycle's Exceptions page.
func (h Handlers) ShowExceptions(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	h.renderExceptions(w, r, id, nil, "", http.StatusOK)
}

// renderExceptions shows the Exceptions page of the Cycle with id, with
// rejected, a Slot's form as typed, and problem, why a change was
// refused, when set.
func (h Handlers) renderExceptions(w http.ResponseWriter, r *http.Request, id int64, rejected *ExceptionRow, problem string, status int) {
	ce, err := h.Service.Exceptions(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "load exceptions", err)
		return
	}
	page := ExceptionsPage{
		CycleID: id,
		Title:   "For the Cycle delivered " + ce.Cycle.Delivery.On.Format(longDate),
		Problem: problem,
	}
	for _, s := range ce.Slots {
		row := exceptionRow(id, s, ce.MealOut)
		if rejected != nil && rejected.ID == row.ID {
			row.Form = rejected.Form
		}
		date := s.Slot.On.Format(shortDate)
		if n := len(page.Days); n == 0 || page.Days[n-1].Date != date {
			page.Days = append(page.Days, ExceptionDay{Date: date})
		}
		day := &page.Days[len(page.Days)-1]
		day.Slots = append(day.Slots, row)
	}
	templ.Handler(ExceptionsView(page), templ.WithStatus(status)).ServeHTTP(w, r)
}

// renderExceptionRow answers htmx with the row of slot alone, so the page
// keeps its place (ADR 0009); rejected, when set, is its form as typed.
func (h Handlers) renderExceptionRow(w http.ResponseWriter, r *http.Request, id int64, slot domain.Slot, rejected *ExceptionRow, status int) {
	ce, err := h.Service.Exceptions(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "load exceptions", err)
		return
	}
	for _, s := range ce.Slots {
		if slotID(s.Slot) == slotID(slot) {
			row := exceptionRow(id, s, ce.MealOut)
			if rejected != nil {
				row.Form, row.Problem = rejected.Form, rejected.Problem
			}
			templ.Handler(exceptionRowView(row), templ.WithStatus(status)).ServeHTTP(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// exceptionRow is s, a Slot of the Cycle with id, as its row shows it,
// with mealOut the default Meal Out estimate.
func exceptionRow(id int64, s ExceptionSlot, mealOut domain.Nutrition) ExceptionRow {
	row := ExceptionRow{
		ID:      slotID(s.Slot),
		Action:  exceptionsURL(id),
		Day:     s.Slot.On.Format(dateLayout),
		Kind:    string(s.Slot.Kind),
		State:   "from the Plan",
		ToCome:  s.ToCome,
		Default: mealOut,
		Form:    ExceptionForm{Values: map[string]string{"exception": ""}},
	}
	if e := s.Exception; e != nil {
		row.State = describeException(*e, mealOut)
		row.Form.Values["exception"] = string(e.Kind)
		for i, own := range []*int{e.Estimate.Kcal, e.Estimate.ProteinG, e.Estimate.CarbsG, e.Estimate.FatG} {
			if own != nil {
				row.Form.Values[estimateFields[i]] = strconv.Itoa(*own)
			}
		}
	}
	return row
}

// describeException is what fills e's Slot as a row shows it, a Meal Out
// with its estimate over mealOut, the default.
func describeException(e domain.Exception, mealOut domain.Nutrition) string {
	if e.Kind != domain.MealOut {
		return e.Kind.Describe()
	}
	n := e.Nutrition(mealOut)
	return fmt.Sprintf("Meal Out · %d kcal · %d g protein", n.Kcal, n.ProteinG)
}

// LogException logs a Slot of a Cycle as a Meal Out or skipped, or back
// to what the Plan fills it with.
func (h Handlers) LogException(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slot, ok := readSlotField(r.PostForm)
	if !ok {
		http.Error(w, "bad slot", http.StatusBadRequest)
		return
	}
	e, problems := readException(r.PostForm, slot)
	rowID := slotID(slot)
	if len(problems) > 0 {
		rejected := &ExceptionRow{ID: rowID, Form: ExceptionForm{Values: typed(r.PostForm, append([]string{"exception"}, estimateFields...)...), Errors: problems}}
		if httpx.IsHTMX(r) {
			h.renderExceptionRow(w, r, id, slot, rejected, http.StatusUnprocessableEntity)
			return
		}
		h.renderExceptions(w, r, id, rejected, "", http.StatusUnprocessableEntity)
		return
	}
	switch err := h.Service.LogException(r.Context(), id, slot, e); {
	case errors.Is(err, ErrNoCycle):
		http.NotFound(w, r)
		return
	case errors.Is(err, ErrSlotUnavailable):
		const problem = "That Slot is past, or no longer one of this Cycle's: it can't be logged ahead."
		if httpx.IsHTMX(r) {
			h.renderExceptionRow(w, r, id, slot, &ExceptionRow{Problem: problem, Form: ExceptionForm{Values: map[string]string{}}}, http.StatusConflict)
			return
		}
		h.renderExceptions(w, r, id, nil, problem, http.StatusConflict)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "log exception", err)
		return
	}
	if httpx.IsHTMX(r) {
		h.renderExceptionRow(w, r, id, slot, nil, http.StatusOK)
		return
	}
	http.Redirect(w, r, exceptionsURL(id)+"#slot-"+rowID, http.StatusSeeOther)
}

// readSlotField reads the Slot a form logs, from its day and kind.
func readSlotField(form url.Values) (domain.Slot, bool) {
	on, err := time.Parse(dateLayout, form.Get("day"))
	kind := domain.SlotKind(form.Get("kind"))
	return domain.Slot{On: on, Kind: kind}, err == nil && slices.Contains(domain.SlotKinds, kind)
}

// readException reads what form logs slot as: an Exception, or nil for
// what the Plan fills it with, returning what is wrong keyed by form
// field name.
func readException(form url.Values, slot domain.Slot) (*domain.Exception, map[string]string) {
	problems := map[string]string{}
	kind := domain.ExceptionKind(form.Get("exception"))
	if kind == "" {
		return nil, problems
	}
	if !slices.Contains(domain.ExceptionKinds, kind) {
		problems["exception"] = "Must be from the Plan, Meal Out or skipped"
		return nil, problems
	}
	e := &domain.Exception{Slot: slot, Kind: kind}
	if kind != domain.MealOut {
		return e, problems
	}
	est := &e.Estimate
	for i, to := range []**int{&est.Kcal, &est.ProteinG, &est.CarbsG, &est.FatG} {
		name := estimateFields[i]
		raw := strings.TrimSpace(form.Get(name))
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		switch {
		case err != nil:
			problems[name] = "Must be a whole number"
		case n < 0:
			problems[name] = "Can't be negative"
		default:
			*to = &n
		}
	}
	return e, problems
}
