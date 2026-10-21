package domain

// SlotKind is the eating occasion a Slot stands for.
type SlotKind string

const (
	Breakfast SlotKind = "breakfast"
	Lunch     SlotKind = "lunch"
	Dinner    SlotKind = "dinner"
	Snack     SlotKind = "snack"
)

// FilledByPortion reports whether Slots of this kind take a Portion.
// Breakfast and snack Slots take a Direct Meal instead.
func (k SlotKind) FilledByPortion() bool {
	return k == Lunch || k == Dinner
}
