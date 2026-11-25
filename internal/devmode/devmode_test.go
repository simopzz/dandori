package devmode_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/devmode"
)

// The dev server wakes up at the dev time: the clock stands there, and the
// scheduler's beat runs once, so what the app assumed up to then is
// recorded before the first request.
func TestTheDevServerWakesUpAtTheDevTime(t *testing.T) {
	at := time.Date(2026, time.November, 25, 8, 0, 0, 0, time.UTC)
	dev, err := devmode.New(at, 5*time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	var beats []time.Time
	dev.Clock.Every(t.Context(), 5*time.Minute, func(at time.Time) { beats = append(beats, at) })
	dev.Wake()
	if got := dev.Clock.Now(); !got.Equal(at) {
		t.Errorf("the clock stands at %s, want %s", got, at)
	}
	if len(beats) != 1 {
		t.Fatalf("beats = %v, want the one of the dev time", beats)
	}
}

// The dev routes are mounted under /_test/: the mailbox lists the links the
// fake Auth sent, the clock switch moves the clock, and the fake Auth
// answers where the app reaches it.
func TestTheDevRoutesAreMountedUnderTest(t *testing.T) {
	at := time.Date(2026, time.November, 25, 8, 0, 0, 0, time.UTC)
	dev, err := devmode.New(at, 5*time.Minute, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	dev.Listening("http://127.0.0.1:8080")
	srv := httptest.NewServer(dev.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the app was asked for %s", r.URL.Path)
	})))
	defer srv.Close()
	client := srv.Client()

	res, err := client.Get(srv.URL + "/_test/mail?email=ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var links []string
	if err := json.NewDecoder(res.Body).Decode(&links); err != nil {
		t.Fatalf("read the mailbox: %v", err)
	}
	res.Body.Close()
	if len(links) != 1 || !strings.HasPrefix(links[0], srv.URL+"/auth/confirm?") {
		t.Fatalf("the mailbox holds %v, want the invitation to the app", links)
	}

	res, err = client.PostForm(srv.URL+"/_test/clock", url.Values{"now": {at.Add(24 * time.Hour).Format(time.RFC3339)}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("the clock switch answered %d, want 204", res.StatusCode)
	}
	if got := dev.Clock.Now(); !got.Equal(at.Add(24 * time.Hour)) {
		t.Errorf("the clock stands at %s, want the day after", got)
	}

	res, err = client.Get(srv.URL + "/_test/supabase/auth/v1/.well-known/jwks.json")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("the fake Supabase Auth answered %d, want 200", res.StatusCode)
	}
}

// The clock switch posted beat=false moves the clock without beating, so a request is
// the first to bring the kitchen up to the new time. The beat it passed is skipped: the
// next one is at the next multiple.
func TestTheClockSwitchCanMoveWithoutBeating(t *testing.T) {
	at := time.Date(2026, time.November, 25, 8, 0, 0, 0, time.UTC)
	dev, err := devmode.New(at, 5*time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	dev.Wake()
	var beats []time.Time
	dev.Clock.Every(t.Context(), 5*time.Minute, func(at time.Time) { beats = append(beats, at) })
	move := func(now time.Time, form url.Values) {
		t.Helper()
		form.Set("now", now.Format(time.RFC3339))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/_test/clock", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		devmode.ClockSwitch(dev.Clock)(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("the clock switch answered %d, want 204", rec.Code)
		}
	}

	move(at.Add(10*time.Minute), url.Values{"beat": {"false"}})
	if got := dev.Clock.Now(); !got.Equal(at.Add(10*time.Minute)) || len(beats) != 0 {
		t.Fatalf("the clock stands at %s with beats %v, want 8:10 and none", got, beats)
	}
	move(at.Add(15*time.Minute), url.Values{})
	if len(beats) != 1 || !beats[0].Equal(at.Add(15*time.Minute)) {
		t.Errorf("beats = %v, want the one at 8:15", beats)
	}
}
