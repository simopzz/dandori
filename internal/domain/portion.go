package domain

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
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

// PortionGroup is alike Portions the kitchen holds: the same name, ignoring case and
// surrounding space, the same nutritional values and the same Storage Location. Name is
// as its oldest Portion spells it.
type PortionGroup struct {
	Name      string
	Nutrition Nutrition
	Location  StorageLocation
	Portions  int
}

// portionName is the key two alike Portions' names share.
func portionName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// GroupPortions is portions, oldest first, grouped where where says the kitchen holds
// them, by Storage Location and then by name. One where leaves out isn't held, so is in
// no group.
func GroupPortions(portions []Portion, where map[int64]StorageLocation) []PortionGroup {
	type alike struct {
		name      string
		nutrition Nutrition
		location  StorageLocation
	}
	var groups []PortionGroup
	found := map[alike]int{}
	for _, p := range portions {
		location, held := where[p.ID]
		if !held {
			continue
		}
		key := alike{portionName(p.Name), p.Nutrition, location}
		i, seen := found[key]
		if !seen {
			i = len(groups)
			found[key] = i
			groups = append(groups, PortionGroup{Name: p.Name, Nutrition: p.Nutrition, Location: location})
		}
		groups[i].Portions++
	}
	slices.SortStableFunc(groups, func(a, b PortionGroup) int {
		return cmp.Or(
			cmp.Compare(slices.Index(StorageLocations, a.Location), slices.Index(StorageLocations, b.Location)),
			cmp.Compare(portionName(a.Name), portionName(b.Name)),
		)
	})
	return groups
}
