package domain

// Earlier is what the Slots before from eat, as sessions' Portions fill
// them, from six days before c's first day on: all a trailing seven days
// of c can reach, whatever Cycle the Slots are in. They are earlier
// Cycles' Slots, and c's own that a Plan from from on keeps. An excepted
// Slot's Portion isn't eaten: only its Exception counts, as one of
// exceptions.
func Earlier(c Cycle, from Slot, sessions []PrepSession, exceptions []Exception) []DayNutrition {
	reach := c.Start.On.AddDate(0, 0, -6)
	byKey := excepted(exceptions)
	var days []DayNutrition
	for _, ps := range sessions {
		n := ps.NutritionPerPortion()
		for _, p := range ps.Portions {
			s := p.EatAt
			if s == nil || s.On.Before(reach) || !s.Before(from) {
				continue
			}
			if _, ok := byKey[s.key()]; ok {
				continue
			}
			days = append(days, DayNutrition{On: s.On, Nutrition: n})
		}
	}
	return days
}
