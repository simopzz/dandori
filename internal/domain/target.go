package domain

import (
	"errors"
	"strconv"
	"strings"
)

// OutOfRange is a whole number outside the range its field allows, such as a calorie
// tolerance outside 1–20%.
const OutOfRange Problem = "out of range"

// The bounds of the Targets: every Target is below MaxTarget, which no daily goal comes
// near, and the calorie tolerance is a percent from MinTolerance to MaxTolerance.
const (
	MaxTarget    = 10000
	MinTolerance = 1
	MaxTolerance = 20
)

// The Targets' fields, as typed.
const (
	TargetKcal      = "kcal"
	TargetProtein   = "protein"
	TargetFibre     = "fibre"
	TargetSugar     = "sugar"
	TargetTolerance = "tolerance"
)

// TargetFields are the Targets' fields, as typed, in the order ParseTargets reads them.
var TargetFields = []string{TargetKcal, TargetProtein, TargetFibre, TargetSugar, TargetTolerance}

// Targets are a User's daily goals: calories within Tolerance percent either side of
// Kcal, protein and fibre in grams as floors, sugar in grams as a ceiling.
type Targets struct {
	Kcal, Protein, Fibre, Sugar, Tolerance int
}

// ByField is t by its fields, as typed.
func (t Targets) ByField() map[string]int {
	return map[string]int{
		TargetKcal: t.Kcal, TargetProtein: t.Protein, TargetFibre: t.Fibre,
		TargetSugar: t.Sugar, TargetTolerance: t.Tolerance,
	}
}

// ParseTargets reads Targets as typed, by field, with a problem per field refused. Each
// field is checked on its own.
func ParseTargets(typed map[string]string) (Targets, map[string]Problem) {
	problems := map[string]Problem{}
	read := func(field string, check func(int) Problem) int {
		n, problem := parseWhole(typed[field])
		if problem == "" {
			problem = check(n)
		}
		if problem != "" {
			problems[field] = problem
		}
		return n
	}
	t := Targets{
		Kcal: read(TargetKcal, checkTarget), Protein: read(TargetProtein, checkTarget),
		Fibre: read(TargetFibre, checkTarget), Sugar: read(TargetSugar, checkTarget),
		Tolerance: read(TargetTolerance, checkTolerance),
	}
	return t, problems
}

func checkTarget(n int) Problem {
	switch {
	case n < 1:
		return NotAboveZero
	case n >= MaxTarget:
		return TooLarge
	}
	return ""
}

func checkTolerance(n int) Problem {
	if n < MinTolerance || n > MaxTolerance {
		return OutOfRange
	}
	return ""
}

// parseWhole reads a whole number as typed. One too large for an int reads as the
// largest, or the smallest, int, for the caller's range to refuse.
func parseWhole(typed string) (int, Problem) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return 0, Required
	}
	n, err := strconv.Atoi(typed)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, NotWhole
	}
	return n, ""
}
