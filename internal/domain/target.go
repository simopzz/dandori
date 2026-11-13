package domain

import (
	"errors"
	"strconv"
	"strings"
)

// OutOfRange is a whole number outside the range its field allows, such as a calorie
// tolerance outside 1–20%.
const OutOfRange Problem = "out of range"

// DefaultTolerance is the calorie tolerance, in percent either side of the calorie
// Target, of a User who hasn't set their own.
const DefaultTolerance = 5

// maxTarget bounds every Target: no daily goal comes near it.
const maxTarget = 10000

// Targets are a User's daily goals: calories within Tolerance percent either side of
// Kcal, protein and fibre in grams as floors, sugar in grams as a ceiling.
type Targets struct {
	Kcal, Protein, Fibre, Sugar, Tolerance int
}

// ParseTargets reads Targets as typed, by field (kcal, protein, fibre, sugar and
// tolerance), with a problem per field refused. Each field is checked on its own.
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
		Kcal: read("kcal", checkTarget), Protein: read("protein", checkTarget),
		Fibre: read("fibre", checkTarget), Sugar: read("sugar", checkTarget),
		Tolerance: read("tolerance", checkTolerance),
	}
	return t, problems
}

func checkTarget(n int) Problem {
	switch {
	case n < 1:
		return NotAboveZero
	case n >= maxTarget:
		return TooLarge
	}
	return ""
}

func checkTolerance(n int) Problem {
	if n < 1 || n > 20 {
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
