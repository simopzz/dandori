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
// fill, none on a day no Plan covers.
type Day struct {
	On        time.Time
	Nutrition PortionNutrition
	Filled    int
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

// Aimed are the Targets' fields a Plan aims at, in the order they are shown.
var Aimed = []string{TargetKcal, TargetProtein, TargetFibre, TargetSugar}

// Of is n's value of the Target field target, and whether it is only at least that.
func (n PortionNutrition) Of(target string) (Amount, bool) {
	switch target {
	case TargetKcal:
		return n.Kcal, false
	case TargetProtein:
		return n.Protein, false
	case TargetFibre:
		return n.Fibre, n.FibreFloor
	}
	return n.Sugar, n.SugarFloor
}

// Verdict is how a value stands against its Target.
type Verdict int

const (
	Met Verdict = iota
	Missed
	// Unsure is a value unknown, or known only to be at least an amount that doesn't
	// settle it.
	Unsure
)

// Bounds are the least and the most target allows: calories within the tolerance either
// side, protein and fibre floors, sugar a ceiling.
func (t Targets) Bounds(target string) (lo, hi float64) {
	aim := float64(t.ByField()[target])
	switch target {
	case TargetKcal:
		return aim * float64(100-t.Tolerance) / 100, aim * float64(100+t.Tolerance) / 100
	case TargetSugar:
		return 0, aim
	}
	return aim, math.Inf(1)
}

// Judge is how n's value of target stands against t, rounded to whole as it is shown. One
// known only to be at least an amount is met only when that is already enough, missed
// only when that is already too much.
func (t Targets) Judge(target string, n PortionNutrition) Verdict {
	a, floor := n.Of(target)
	if !a.Known {
		return Unsure
	}
	v := math.Round(a.Value)
	lo, hi := t.Bounds(target)
	switch {
	case v > hi:
		return Missed
	case floor && math.IsInf(hi, 1) && v >= lo:
		return Met
	case floor:
		return Unsure
	case v < lo:
		return Missed
	}
	return Met
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
// Unused: Count Portions of Name left without a Slot in Location.
type Miss struct {
	Rule         Rule
	Target       string
	Value        int
	Floor, Under bool
	On           time.Time
	Name         string
	Location     StorageLocation
	Count        int
	Days         []time.Time
}

// targetNames name the Targets for the planner.
var targetNames = map[string]string{TargetKcal: "calories", TargetProtein: "protein", TargetFibre: "fibre", TargetSugar: "sugar"}

// Describe is m as the planner reads it, against t.
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
		return fmt.Sprintf("%s averages %s%d g a day against %d g", capitalised(targetNames[m.Target]), least, m.Value, t.ByField()[m.Target])
	case RuleDayOff:
		way := "over"
		if m.Under {
			way = "under"
		}
		return fmt.Sprintf("%s: %s%d%% %s %s", m.On.Format("Mon 2 Jan"), least, m.Value, way, targetNames[m.Target])
	case RuleBackToBack:
		days := make([]string, len(m.Days))
		for i, d := range m.Days {
			days[i] = d.Format("Mon 2 Jan")
		}
		return fmt.Sprintf("%s eaten back to back, %s", m.Name, strings.Join(days, " and "))
	}
	name := m.Name
	if m.Count > 1 {
		name = fmt.Sprintf("%s × %d", m.Name, m.Count)
	}
	return fmt.Sprintf("%s (%s) left unused", name, m.Location)
}

// capitalised is s with a capital first letter, s being ASCII.
func capitalised(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// Misses are the Targets t misses over days, each of whose Slots the Plan fills: the
// average against each Target, then each day more than maxOff off one in its direction.
// None when there is no day.
func (t Targets) Misses(days []Day) []Miss {
	if len(days) == 0 {
		return nil
	}
	var ms []Miss
	avg := Average(days)
	for _, target := range Aimed {
		if t.Judge(target, avg) == Missed {
			a, floor := avg.Of(target)
			ms = append(ms, Miss{Rule: RuleTargetAverage, Target: target, Value: int(math.Round(a.Value)), Floor: floor})
		}
	}
	for _, d := range days {
		for _, target := range Aimed {
			if m, off := t.off(target, d); off {
				ms = append(ms, m)
			}
		}
	}
	return ms
}

// off is how far d is off target past maxOff, and whether it is, or can be known to be:
// calories either way, protein and fibre under, sugar over.
func (t Targets) off(target string, d Day) (Miss, bool) {
	a, floor := d.Nutrition.Of(target)
	if !a.Known {
		return Miss{}, false
	}
	aim, v := float64(t.ByField()[target]), math.Round(a.Value)
	m := Miss{Rule: RuleDayOff, Target: target, On: d.On, Value: int(math.Round(math.Abs(v-aim) / aim * 100)), Floor: floor}
	switch {
	case v > aim*(1+maxOff) && target != TargetProtein && target != TargetFibre:
		return m, true
	case v < aim*(1-maxOff) && !floor && target != TargetSugar:
		m.Under = true
		return m, true
	}
	return Miss{}, false
}

// SoftMisses are the soft rules p breaks beside the Targets: Portions of one Prep Session
// eaten within two days of each other, and Portions left without a Slot, held being the
// Portion Lots the kitchen holds without one.
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
	unused := func(name string, location StorageLocation, n int) {
		if n == 0 {
			return
		}
		i := slices.IndexFunc(ms, func(m Miss) bool {
			return m.Rule == RuleUnusedPortion && m.Location == location && portionName(m.Name) == portionName(name)
		})
		if i < 0 {
			ms = append(ms, Miss{Rule: RuleUnusedPortion, Name: name, Location: location})
			i = len(ms) - 1
		}
		ms[i].Count += n
	}
	for _, lot := range held {
		left := 0
		for _, id := range lot.IDs {
			if !slices.ContainsFunc(p.Kept, func(k Kept) bool { return k.Portion == id }) {
				left++
			}
		}
		unused(lot.Name, lot.Location, left)
	}
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt == nil {
				unused(ps.Name(), pp.Location, 1)
			}
		}
	}
	return ms
}

// PlanDays are a Plan's days from its first Slot to last, beside the seven days before
// them: what earlier Plans put in Slots before first, earlier, and what the Plan puts in
// its own, planned. judged are the days it plans each of whose Slots it fills, which the
// Targets are judged over: a first day it fills only from lunch on is shown, not judged.
func PlanDays(earlier, planned []Eaten, first Slot, last time.Time) (trailing, days, judged []Day) {
	all := slices.Clone(planned)
	for _, e := range earlier {
		if e.At.Before(first) {
			all = append(all, e)
		}
	}
	trailing = Days(all, first.On.AddDate(0, 0, -7), first.On.AddDate(0, 0, -1))
	days = Days(all, first.On, last)
	judged = days
	if first.Kind != SlotKinds[0] && len(days) > 0 {
		judged = days[1:]
	}
	return trailing, days, judged
}
