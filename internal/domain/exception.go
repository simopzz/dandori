package domain

import "time"

// ExceptionKind is what an Exception on a Slot says happens instead of
// the Plan. Only a Meal Out or a skipped Slot can be logged ahead.
type ExceptionKind string

const (
	MealOut ExceptionKind = "meal-out"
	Skipped ExceptionKind = "skipped"
)

// ExceptionKinds lists every kind of Exception a Slot can be logged ahead
// with.
var ExceptionKinds = []ExceptionKind{MealOut, Skipped}

// Estimate is a Meal Out's own nutrition estimate, nutrient by nutrient:
// one left nil takes the Profile's default.
type Estimate struct {
	Kcal, ProteinG, CarbsG, FatG *int
}

// Over is the estimate with the nutrients it leaves out taken from def.
func (e Estimate) Over(def Nutrition) Nutrition {
	pick := func(own *int, d int) int {
		if own != nil {
			return *own
		}
		return d
	}
	return Nutrition{Kcal: pick(e.Kcal, def.Kcal), ProteinG: pick(e.ProteinG, def.ProteinG), CarbsG: pick(e.CarbsG, def.CarbsG), FatG: pick(e.FatG, def.FatG)}
}

// ToCome reports whether, at now, the Slot's time of day on its day, at
// times, is still to come: only then can it be logged ahead. Once past,
// its Exception stands as what happened.
func (s Slot) ToCome(now time.Time, times SlotTimes) bool {
	return WallClock(now).Before(s.On.Add(time.Duration(times[s.Kind]) * time.Minute))
}

// Exception is a Slot logged as a Meal Out or skipped. It belongs to its
// day and Slot kind, not to a Cycle.
type Exception struct {
	Slot Slot
	Kind ExceptionKind
	// Estimate is a Meal Out's own estimate; a skipped Slot has none.
	Estimate Estimate
}

// Nutrition is what the Exception's Slot eats, with mealOut the Profile's
// default Meal Out estimate: a skipped Slot eats nothing.
func (e Exception) Nutrition(mealOut Nutrition) Nutrition {
	if e.Kind != MealOut {
		return Nutrition{}
	}
	return e.Estimate.Over(mealOut)
}

// Describe is the Exception as a few words for the user, e.g. "Meal Out"
// or "skipped".
func (k ExceptionKind) Describe() string {
	if k == MealOut {
		return "Meal Out"
	}
	return "skipped"
}
