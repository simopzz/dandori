package domain

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// maxPortions bounds how many Portions are added at once: each is a row of its own.
const maxPortions = 99

// Portion is one ready-to-eat serving, with its values per Portion. Where it is and
// whether the kitchen still holds it are what its Movements add up to (ADR-0013).
type Portion struct {
	ID        int64
	Name      string
	Nutrition Nutrition
}

// NewPortions are Portions no Prep Session produced, as the User adds them.
type NewPortions struct {
	Name      string
	HowMany   int
	Location  StorageLocation
	Nutrition Nutrition
	Note      string
}

// TypedPortions are Portions as the User typed them, Nutrition by field.
type TypedPortions struct {
	Name, HowMany, Note string
	Location            StorageLocation
	Nutrition           map[string]string
}

// ParsePortions reads Portions as the User typed them, with why each field is refused,
// by field, if any is: they need a name, how many, a Storage Location and the values
// every food has.
func ParsePortions(typed TypedPortions) (NewPortions, map[string]Problem) {
	nutrition, problems := ParseNutrition(typed.Nutrition)
	name := strings.TrimSpace(typed.Name)
	if name == "" {
		problems["name"] = Required
	}
	howMany, problem := ParseHowMany(typed.HowMany)
	if problem != "" {
		problems["how_many"] = problem
	}
	if !slices.Contains(StorageLocations, typed.Location) {
		problems["location"] = Required
	}
	return NewPortions{
		Name: name, HowMany: howMany, Location: typed.Location, Nutrition: nutrition, Note: strings.TrimSpace(typed.Note),
	}, problems
}

// ParseHowMany reads how many Portions to add as the User typed it: a whole number of
// at least 1 and at most maxPortions.
func ParseHowMany(typed string) (int, Problem) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return 0, Required
	}
	n, err := strconv.Atoi(typed)
	switch {
	case err != nil || n < 1:
		return 0, NotWhole
	case n > maxPortions:
		return 0, TooLarge
	}
	return n, ""
}

// PortionLot is alike Portions the kitchen holds: the same name, ignoring case and
// surrounding space, the same nutritional values and the same Storage Location. Name is
// as its oldest Portion spells it.
type PortionLot struct {
	Name      string
	Nutrition Nutrition
	Location  StorageLocation
	// IDs are the Portions it holds, oldest first.
	IDs []int64
}

// PortionUnit is the Unit Portions are told in: whole numbers, as ParseCount and
// ParseMove read them, shown bare. No Ingredient has it.
const PortionUnit Unit = "portion"

// Count is how many Portions g holds.
func (g PortionLot) Count() int { return len(g.IDs) }

// held is how many Portions g holds, as a Quantity.
func (g PortionLot) held() Quantity { return portionCount(len(g.IDs)) }

// CheckFits says why more Portions alike can't join g, if they can't: a Portion Lot holds
// at most maxPortions.
func (g PortionLot) CheckFits(more int) Problem {
	if g.Count()+more > maxPortions {
		return TooManyAlike
	}
	return ""
}

// portionCount is n Portions as a Quantity.
func portionCount(n int) Quantity { return QuantityOf(float64(n)) }

// MoreAlike are the Portions a Count of stated adds to g, above what it holds.
func (g PortionLot) MoreAlike(stated Quantity) NewPortions {
	return NewPortions{Name: g.Name, HowMany: int((stated - g.held()).Number()), Location: g.Location, Nutrition: g.Nutrition}
}

// takes is the n Portions of g a Count down takes: those without a Slot, oldest first,
// then the latest Slot. slots has each one's Slot, nil until there are Slots.
func (g PortionLot) takes(n int, slots map[int64]time.Time) []int64 {
	order := slices.Clone(g.IDs)
	slices.SortStableFunc(order, func(a, b int64) int {
		slotA, slotB := slots[a], slots[b]
		if slotA.IsZero() != slotB.IsZero() {
			if slotA.IsZero() {
				return -1
			}
			return 1
		}
		return slotB.Compare(slotA)
	})
	return order[:n]
}

// ParsePortionCount reads a Count of a Lot as the User typed it: a whole number of at
// most maxPortions, with why each field is refused, by field, if any is.
func ParsePortionCount(location StorageLocation, stated string) (Quantity, map[string]Problem) {
	q, problems := ParseCount(PortionUnit, location, stated)
	if q > portionCount(maxPortions) {
		problems["stated"] = TooLarge
	}
	return q, problems
}

// LotKey identifies a Portion Lot, which has no id of its own: one Portion alike it, and
// its Storage Location.
type LotKey struct {
	Portion  int64
	Location StorageLocation
}

// LotOf is the Portion Lot key names, as where says the kitchen holds it: empty, and
// named as key's Portion is, if its Storage Location holds none. ok is false if portions,
// oldest first, has no such Portion.
func LotOf(portions []Portion, where map[int64]StorageLocation, key LotKey) (_ PortionLot, ok bool) {
	i := slices.IndexFunc(portions, func(p Portion) bool { return p.ID == key.Portion })
	if i < 0 {
		return PortionLot{}, false
	}
	return LotLike(portions, where, portions[i], key.Location), true
}

// LotLike is the Portion Lot alike the Portion like in location, as where says the
// kitchen holds it: empty, and named as like is, if location holds none.
func LotLike(portions []Portion, where map[int64]StorageLocation, like Portion, location StorageLocation) PortionLot {
	lot := PortionLot{Name: like.Name, Nutrition: like.Nutrition, Location: location}
	for _, p := range portions {
		if where[p.ID] == location && like.alike(p) {
			if lot.Count() == 0 {
				lot.Name = p.Name
			}
			lot.IDs = append(lot.IDs, p.ID)
		}
	}
	return lot
}

// alike says whether two Portions have the same name, ignoring case and surrounding
// space, and nutritional values.
func (p Portion) alike(other Portion) bool {
	return portionName(p.Name) == portionName(other.Name) && p.Nutrition == other.Nutrition
}

// portionName is the key two alike Portions' names share.
func portionName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// PortionLots is portions, oldest first, grouped where where says the kitchen holds
// them, by Storage Location and then by name. One where leaves out isn't held, so is in
// no Lot.
func PortionLots(portions []Portion, where map[int64]StorageLocation) []PortionLot {
	type alike struct {
		name      string
		nutrition Nutrition
		location  StorageLocation
	}
	var lots []PortionLot
	found := map[alike]int{}
	for _, p := range portions {
		location, held := where[p.ID]
		if !held {
			continue
		}
		key := alike{portionName(p.Name), p.Nutrition, location}
		i, seen := found[key]
		if !seen {
			i = len(lots)
			found[key] = i
			lots = append(lots, PortionLot{Name: p.Name, Nutrition: p.Nutrition, Location: location})
		}
		lots[i].IDs = append(lots[i].IDs, p.ID)
	}
	slices.SortStableFunc(lots, func(a, b PortionLot) int {
		return cmp.Or(
			cmp.Compare(slices.Index(StorageLocations, a.Location), slices.Index(StorageLocations, b.Location)),
			cmp.Compare(portionName(a.Name), portionName(b.Name)),
		)
	})
	return lots
}
