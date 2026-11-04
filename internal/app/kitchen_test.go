package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// The scheduler beats every five minutes on the clock and brings every User's kitchen up
// to now, whether or not they have a page open: each one's Home shows the last beat,
// which a page opened in between leaves as it was.
func TestTheSchedulerBringsEveryUsersKitchenUpToNow(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.Invite("bob@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.DoesNotSee("Kitchen up to date")
	s.Open("/profile")
	s.SignsOut()
	s.SignsIn("bob@example.com")
	s.ChoosesLanguage("English")

	s.At("Mon 14:05")
	s.At("Mon 14:07")
	s.Open("/")
	s.Sees("Kitchen up to date as of 14:05")
	s.Open("/profile")
	s.SignsOut()
	s.SignsIn("ada@example.com")
	s.Sees("Kitchen up to date as of 14:05")

	s.At("Mon 14:10")
	s.Open("/")
	s.Sees("Kitchen up to date as of 14:10")
}

// A beat that fails for one User still brings the others up to now, and the failure is
// logged. The User it failed for sees the last beat that didn't, with its day.
func TestABeatFailingForOneUserStillBringsTheOthersUpToNow(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.Invite("bob@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/profile")
	s.SignsOut()
	s.SignsIn("bob@example.com")
	s.ChoosesLanguage("English")
	s.At("Mon 14:05")

	// Stands in for whatever could make Ada's beat fail: her kitchen refuses a new beat.
	s.Exec(t, `
		CREATE FUNCTION refuse_adas_beat() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN
			IF (SELECT email FROM "user" WHERE id = NEW.user_id) = 'ada@example.com' THEN
				RAISE EXCEPTION 'Ada''s kitchen refuses the beat';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER refuse_adas_beat BEFORE UPDATE ON kitchen FOR EACH ROW
			WHEN (NEW.beaten_at IS DISTINCT FROM OLD.beaten_at) EXECUTE FUNCTION refuse_adas_beat();`)
	s.At("Tue 14:05")

	s.Open("/")
	s.Sees("Kitchen up to date as of 14:05")
	s.Open("/profile")
	s.SignsOut()
	s.SignsIn("ada@example.com")
	s.Sees("Kitchen up to date as of Monday 2 November, 14:05")
	if logged := s.Logged(); !strings.Contains(logged, "kitchen beat") || !strings.Contains(logged, "Ada's kitchen refuses the beat") {
		t.Errorf("the log doesn't record Ada's failed beat:\n%s", logged)
	}
}

// Running the step again for the moment a beat brought the kitchen to changes nothing:
// a page opened then shows the beat as it was.
func TestTheStepRunAgainForTheSameMomentChangesNothing(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")

	s.At("Mon 14:05")
	s.Open("/")
	s.Sees("Kitchen up to date as of 14:05")
	s.Open("/")
	s.Sees("Kitchen up to date as of 14:05")
}
