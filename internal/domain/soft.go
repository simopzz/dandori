package domain

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Eaten is what a Plan puts in one Slot: the values of the Portion filling it.
type Eaten struct {
	At        Slot
	Nutrition PortionNutrition
}

// Eaten is what p puts in each Slot, kept being the Portions without a Slot it may put
// in them. One it puts in a Slot that isn't among kept is left out: HardRules says why.
func (p Plan) Eaten(kept []Portion) []Eaten {
	var out []Eaten
	for _, ps := range p.PrepSessions {
		n := ps.NutritionPerPortion()
		for _, pp := range ps.Portions {
			if pp.EatAt != nil {
				out = append(out, Eaten{At: *pp.EatAt, Nutrition: n})
			}
		}
	}
	for _, k := range p.Kept {
		if i := slices.IndexFunc(kept, func(portion Portion) bool { return portion.ID == k.Portion }); i >= 0 {
			out = append(out, Eaten{At: k.At, Nutrition: PortionNutrition{Nutrition: kept[i].Nutrition}})
		}
	}
	return out
}

// Day is what one day's Slots eat, as far as Plans fill them: Filled is how many they
// fill, none on a day no Plan covers. Partly is a Plan's first day it fills only from a
// later Slot on: shown, never judged against the Targets.
type Day struct {
	On        time.Time
	Nutrition PortionNutrition
	Filled    int
	Partly    bool
}

// Days is each day from first to last, at midnight, with what eaten puts in its Slots.
func Days(eaten []Eaten, first, last time.Time) []Day {
	var days []Day
	for on := first; !on.After(last); on = on.AddDate(0, 0, 1) {
		var ns []PortionNutrition
		for _, e := range eaten {
			if e.At.On.Format(time.DateOnly) == on.Format(time.DateOnly) {
				ns = append(ns, e.Nutrition)
			}
		}
		days = append(days, Day{On: on, Nutrition: sum(ns), Filled: len(ns)})
	}
	return days
}

// sum is ns added up: a value is known when any of ns knows it, and only at least that
// much when another doesn't know it or knows only at least so much. Unknown when none
// knows it, never 0. Calories and protein every food knows, so they are never floors.
func sum(ns []PortionNutrition) PortionNutrition {
	add := func(value func(PortionNutrition) (Amount, bool)) (Amount, bool) {
		var total float64
		var known, floor bool
		for _, n := range ns {
			a, f := value(n)
			if !a.Known {
				floor = true
				continue
			}
			known, total, floor = true, total+a.Value, floor || f
		}
		if !known {
			return Unknown, false
		}
		return Known(total), floor
	}
	var s PortionNutrition
	s.Kcal, _ = add(func(n PortionNutrition) (Amount, bool) { return n.Kcal, false })
	s.Protein, _ = add(func(n PortionNutrition) (Amount, bool) { return n.Protein, false })
	s.Carbs, _ = add(func(n PortionNutrition) (Amount, bool) { return n.Carbs, false })
	s.Fat, _ = add(func(n PortionNutrition) (Amount, bool) { return n.Fat, false })
	s.Fibre, s.FibreFloor = add(func(n PortionNutrition) (Amount, bool) { return n.Fibre, n.FibreFloor })
	s.Sugar, s.SugarFloor = add(func(n PortionNutrition) (Amount, bool) { return n.Sugar, n.SugarFloor })
	return s
}

// Average is days' nutrition averaged over them, a floor staying a floor: what a day
// doesn't know counts as at least none.
func Average(days []Day) PortionNutrition {
	ns := make([]PortionNutrition, len(days))
	for i, d := range days {
		ns[i] = d.Nutrition
	}
	avg := sum(ns)
	for _, a := range []*Amount{&avg.Kcal, &avg.Protein, &avg.Carbs, &avg.Fat, &avg.Fibre, &avg.Sugar} {
		a.Value /= float64(len(days))
	}
	return avg
}

// Of is n's value of target, and whether it is only at least that.
func (n PortionNutrition) Of(target Target) (Amount, bool) {
	switch target {
	case TargetKcal:
		return n.Kcal, false
	case TargetProtein:
		return n.Protein, false
	case TargetFibre:
		return n.Fibre, n.FibreFloor
	case TargetSugar:
		return n.Sugar, n.SugarFloor
	}
	panic("unknown Target " + target)
}

// Aim is t's daily goal for target.
func (t Targets) Aim(target Target) int {
	switch target {
	case TargetKcal:
		return t.Kcal
	case TargetProtein:
		return t.Protein
	case TargetFibre:
		return t.Fibre
	case TargetSugar:
		return t.Sugar
	}
	panic("unknown Target " + target)
}

// Bounds are the least and the most target allows a day: calories within the tolerance
// either side, protein and fibre floors with no most, sugar a ceiling with no least.
func (t Targets) Bounds(target Target) (lo, hi float64) {
	aim := float64(t.Aim(target))
	switch target {
	case TargetKcal:
		return aim * float64(100-t.Tolerance) / 100, aim * float64(100+t.Tolerance) / 100
	case TargetSugar:
		return 0, aim
	}
	return aim, math.Inf(1)
}

// Missed reports whether n's value of target misses t, rounded to whole as it is shown.
// One unknown never does, and one known only to be at least an amount does only when that
// is already too much.
func (t Targets) Missed(target Target, n PortionNutrition) bool {
	a, floor := n.Of(target)
	if !a.Known {
		return false
	}
	v := math.Round(a.Value)
	lo, hi := t.Bounds(target)
	return v > hi || (!floor && v < lo)
}

// maxOff is how far a planned day may be off a Target, as a fraction of it.
const maxOff = 0.25

// The soft rules: a Plan breaking one is still stored, the miss shown for the User to
// weigh (ADR-0018).
const (
	RuleTargetAverage Rule = "target-average"
	RuleDayOff        Rule = "day-off"
	RuleBackToBack    Rule = "back-to-back"
	RuleUnusedPortion Rule = "unused-portion"
)

// Miss is a soft rule a Plan breaks, and where. For the Targets' rules: the Target, and
// its average over the planned days as Value, or how far the day On is off it in percent,
// Under it or over, Floor when only at least that. Back to back: Name eaten on Days.
// Unused: Count Portions of Name carried into the Cycle left without a Slot in Location,
// of the Held there.
type Miss struct {
	Rule         Rule
	Target       Target
	Value        int
	Floor, Under bool
	On           time.Time
	Name         string
	Location     StorageLocation
	Count, Held  int
	Days         []time.Time
}

// Describe is m as the planner reads it, against t: the page says it in Messages, which
// the domain can't reach.
func (m Miss) Describe(t Targets) string {
	least := ""
	if m.Floor {
		least = "at least "
	}
	switch m.Rule {
	case RuleTargetAverage:
		if m.Target == TargetKcal {
			lo, hi := t.Bounds(TargetKcal)
			return fmt.Sprintf("Calories average %s%d a day against %.0f–%.0f", least, m.Value, lo, hi)
		}
		name := string(m.Target) // a Target's code is a plain English word
		return fmt.Sprintf("%s averages %s%d g a day against %d g", strings.ToUpper(name[:1])+name[1:], least, m.Value, t.Aim(m.Target))
	case RuleDayOff:
		way := "over"
		if m.Under {
			way = "under"
		}
		return fmt.Sprintf("%s: %s%d%% %s %s", m.On.Format(PlannerDay), least, m.Value, way, m.Target)
	case RuleBackToBack:
		days := make([]string, len(m.Days))
		for i, d := range m.Days {
			days[i] = d.Format(PlannerDay)
		}
		return fmt.Sprintf("%s eaten back to back, %s", m.Name, strings.Join(days, " and "))
	}
	return fmt.Sprintf("%s: %d of %d left unused, in the %s", m.Name, m.Count, m.Held, m.Location)
}

// PlannerDay is how the planner is told a day, as Slot.String tells it.
const PlannerDay = "Mon 2 Jan"

// Judged are the days of days the Targets are judged over: those not planned partly.
func Judged(days []Day) []Day {
	var judged []Day
	for _, d := range days {
		if !d.Partly {
			judged = append(judged, d)
		}
	}
	return judged
}

// Misses are the Targets t misses over the days judged of days: the average against each
// Target, then each day more than maxOff off one. None when no day is judged.
func (t Targets) Misses(days []Day) []Miss {
	judged := Judged(days)
	if len(judged) == 0 {
		return nil
	}
	var ms []Miss
	avg := Average(judged)
	for _, target := range Aimed {
		if t.Missed(target, avg) {
			a, floor := avg.Of(target)
			ms = append(ms, Miss{Rule: RuleTargetAverage, Target: target, Value: int(math.Round(a.Value)), Floor: floor})
		}
	}
	for _, d := range judged {
		for _, target := range Aimed {
			if m, off := t.DayOff(target, d); off {
				ms = append(ms, m)
			}
		}
	}
	return ms
}

// DayOff is how far d is off target past maxOff, and whether it is, or can be known to
// be: calories either way, protein and fibre under, sugar over. A day planned only
// partly never is.
func (t Targets) DayOff(target Target, d Day) (Miss, bool) {
	a, floor := d.Nutrition.Of(target)
	if !a.Known || d.Partly {
		return Miss{}, false
	}
	aim, v := float64(t.Aim(target)), math.Round(a.Value)
	lo, hi := t.Bounds(target)
	m := Miss{Rule: RuleDayOff, Target: target, On: d.On, Value: int(math.Round(math.Abs(v-aim) / aim * 100)), Floor: floor}
	switch {
	case !math.IsInf(hi, 1) && v > aim*(1+maxOff):
		return m, true
	case lo > 0 && !floor && v < aim*(1-maxOff):
		m.Under = true
		return m, true
	}
	return Miss{}, false
}

// SoftMisses are the soft rules p breaks beside the Targets: Portions of one Prep Session
// eaten within two days of each other, and Portions carried into the Cycle left without a
// Slot, held being the Portion Lots the kitchen holds without one. Those p makes with no
// Slot are no miss: they are MadeForAfter.
func (p Plan) SoftMisses(held []PortionLot) []Miss {
	var ms []Miss
	for _, ps := range p.PrepSessions {
		var days []time.Time
		for _, pp := range ps.Portions {
			if pp.EatAt != nil {
				days = append(days, pp.EatAt.On)
			}
		}
		slices.SortFunc(days, time.Time.Compare)
		var close []time.Time
		for i := 1; i < len(days); i++ {
			if math.Round(days[i].Sub(days[i-1]).Hours()/24) < 2 {
				for _, d := range days[i-1 : i+1] {
					if len(close) == 0 || !close[len(close)-1].Equal(d) {
						close = append(close, d)
					}
				}
			}
		}
		if len(close) > 0 {
			ms = append(ms, Miss{Rule: RuleBackToBack, Name: ps.Name(), Days: close})
		}
	}
	var unused []Miss
	for _, lot := range held {
		left := 0
		for _, id := range lot.IDs {
			if !slices.ContainsFunc(p.Kept, func(k Kept) bool { return k.Portion == id }) {
				left++
			}
		}
		i := slices.IndexFunc(unused, func(m Miss) bool {
			return m.Location == lot.Location && portionName(m.Name) == portionName(lot.Name)
		})
		if i < 0 {
			unused = append(unused, Miss{Rule: RuleUnusedPortion, Name: lot.Name, Location: lot.Location})
			i = len(unused) - 1
		}
		unused[i].Count += left
		unused[i].Held += len(lot.IDs)
	}
	return append(ms, slices.DeleteFunc(unused, func(m Miss) bool { return m.Count == 0 })...)
}

// MadeForAfter is how many Portions p's Prep Sessions make with no Slot, to eat after the
// Cycle, and their names, each once.
func (p Plan) MadeForAfter() (n int, names []string) {
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt == nil {
				n++
				if !slices.Contains(names, ps.Name()) {
					names = append(names, ps.Name())
				}
			}
		}
	}
	return n, names
}

// PlanDays are a Plan's days from its first Slot to last, beside the seven days before
// them: what earlier Plans put in Slots before first, earlier, and what the Plan puts in
// its own, planned. A first day it fills only from lunch on is Partly.
func PlanDays(earlier, planned []Eaten, first Slot, last time.Time) (trailing, days []Day) {
	all := slices.Clone(planned)
	for _, e := range earlier {
		if e.At.Before(first) {
			all = append(all, e)
		}
	}
	trailing = Days(all, first.On.AddDate(0, 0, -7), first.On.AddDate(0, 0, -1))
	days = Days(all, first.On, last)
	if first.Kind != SlotKinds[0] && len(days) > 0 {
		days[0].Partly = true
	}
	return trailing, days
}
