package domain

// CaloriesWithinTarget reports whether the calories eaten over a trailing
// seven days, Untracked Allowance included, are within ±5% of seven times
// the daily calorie Target.
func CaloriesWithinTarget(dailyTarget, eaten int) bool {
	weekly := 7 * dailyTarget
	diff := eaten - weekly
	if diff < 0 {
		diff = -diff
	}
	return diff*100 <= weekly*5
}
