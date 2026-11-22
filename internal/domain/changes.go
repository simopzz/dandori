package domain

import "slices"

// Planned are p's Planned Portions as Portions, by PlannedID: those a Plan Proposal
// replacing p from a later Slot may put in Slots again.
func (p Plan) Planned() []Portion {
	var out []Portion
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.ID != 0 {
				out = append(out, Portion{ID: PlannedID(pp.ID), Name: ps.Name(), Nutrition: ps.NutritionPerPortion().Nutrition})
			}
		}
	}
	return out
}

// Changes are what p, a Plan Proposal's Plan, changes of current, the Plan it replaces
// from the Slot from on: each Slot from from on the two fill with other food, by its
// name and values per Portion, in order, and the index of each of p's Prep Sessions
// current has none alike, at the same Slot, preparing the same at the same size or
// quantity, its Portions kept in the same Storage Locations and eaten at the same Slots.
// portions name the Portions without a Slot either puts in Slots.
func (p Plan) Changes(current Plan, from Slot, portions []Portion) (slots []Slot, sessions []int) {
	mine, theirs := p.fills(portions), current.fills(portions)
	for key, f := range mine {
		if !f.at.Before(from) && theirs[key] != f {
			slots = append(slots, f.at)
		}
	}
	for key, f := range theirs {
		if _, ok := mine[key]; !ok && !f.at.Before(from) {
			slots = append(slots, f.at)
		}
	}
	slices.SortFunc(slots, func(a, b Slot) int {
		if a.Before(b) {
			return -1
		}
		return 1
	})
	for i, ps := range p.PrepSessions {
		if !slices.ContainsFunc(current.PrepSessions, func(o PrepSession) bool { return alike(ps, o) }) {
			sessions = append(sessions, i)
		}
	}
	return slots, sessions
}

// SplitChanges are the indexes of `p`'s freezer splits `current` has none alike of, the
// same Ingredient at the same quantity, and whether `p` changes `current`'s freezer
// splits at all: one of them is changed, or `current` has one `p` has none alike of.
func (p Plan) SplitChanges(current Plan) (rows []int, changed bool) {
	alike := func(a RecipeIngredient) func(RecipeIngredient) bool {
		return func(b RecipeIngredient) bool { return a.Ingredient.ID == b.Ingredient.ID && a.Quantity == b.Quantity }
	}
	for i, fs := range p.FreezerSplits {
		if !slices.ContainsFunc(current.FreezerSplits, alike(fs)) {
			rows = append(rows, i)
		}
	}
	dropped := slices.ContainsFunc(current.FreezerSplits, func(fs RecipeIngredient) bool {
		return !slices.ContainsFunc(p.FreezerSplits, alike(fs))
	})
	return rows, len(rows) > 0 || dropped
}

// fill is what fills a Slot at: a Portion's name and values.
type fill struct {
	at        Slot
	name      string
	nutrition Nutrition
}

// fills is what p puts in each Slot, by Slot.key, portions naming the Portions without a
// Slot it puts in them.
func (p Plan) fills(portions []Portion) map[string]fill {
	out := map[string]fill{}
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt != nil {
				out[pp.EatAt.key()] = fill{at: *pp.EatAt, name: ps.Name(), nutrition: ps.NutritionPerPortion().Nutrition}
			}
		}
	}
	for _, k := range p.Kept {
		if i := slices.IndexFunc(portions, func(portion Portion) bool { return portion.ID == k.Portion }); i >= 0 {
			out[k.At.key()] = fill{at: k.At, name: portions[i].Name, nutrition: portions[i].Nutrition}
		}
	}
	return out
}

// alike reports whether a and b prepare the same at the same Slot: the same food at the
// same size or quantity, their Portions kept in the same Storage Locations and eaten at
// the same Slots, in whatever order.
func alike(a, b PrepSession) bool {
	return a.At.Equal(b.At) && a.Name() == b.Name() && a.Size == b.Size && a.Quantity == b.Quantity &&
		slices.Equal(portionKeys(a), portionKeys(b))
}

// portionKeys are where each of ps's Portions is kept and eaten, sorted.
func portionKeys(ps PrepSession) []string {
	keys := make([]string, len(ps.Portions))
	for i, pp := range ps.Portions {
		keys[i] = string(pp.Location)
		if pp.EatAt != nil {
			keys[i] += " " + pp.EatAt.key()
		}
	}
	slices.Sort(keys)
	return keys
}
