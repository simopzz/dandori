package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
)

// Tool reads the Profile as an agent tool, for the planner: what every
// Plan is checked against.
func (s *Service) Tool() agent.Tool {
	return agent.Tool{
		Name:        "get_profile",
		Description: "Read the user's Profile: the daily Targets, the Untracked Allowance reserved each day before Slots are filled, the exclusions and allergies, the default Meal Out estimate and the time of day of each kind of Slot.",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		Run: func(ctx context.Context, _ agent.Trace, _ json.RawMessage) (string, error) {
			p, found, err := s.Get(ctx)
			if err != nil {
				return "", err
			}
			if !found {
				return "", agent.Stop(errors.New("the Profile isn't set up yet: save it before planning"))
			}
			return describe(p), nil
		},
	}
}

// describe is p as the model reads it.
func describe(p domain.Profile) string {
	list := func(terms []string) string {
		if len(terms) == 0 {
			return "none"
		}
		return strings.Join(terms, ", ")
	}
	var times []string
	for _, k := range domain.SlotKinds {
		times = append(times, fmt.Sprintf("%s %s", k, p.SlotTimes[k]))
	}
	return fmt.Sprintf(`Targets, a day: %s. Over any trailing seven days calories must be within ±5%% of seven days' Target and protein at least seven days' Target.
Untracked Allowance, a day: %s.
Exclusions: %s.
Allergies: %s.
Default Meal Out estimate: %s.
Slot times: %s.`,
		p.Targets.Describe(), p.UntrackedAllowance.Describe(), list(p.Exclusions), list(p.Allergies), p.MealOut.Describe(), strings.Join(times, ", "))
}
