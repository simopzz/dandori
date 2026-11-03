package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/clock"
)

// A deploy starts the new binary on the database as the last one left it:
// the migrations a PR added are applied before the app serves.
func TestStartupMigratesTheDatabaseBeforeServing(t *testing.T) {
	h := apptest.Start(t, apptest.WithUnmigratedDatabase())
	h.Invite("ada@example.com")

	// Signing in creates the User, in the table a migration adds.
	if res := h.SignIn(t, "ada@example.com"); res.StatusCode != http.StatusOK || res.Path != "/" {
		t.Fatalf("signing in = %d at %s, want 200 at /:\n%s", res.StatusCode, res.Path, res.Body)
	}
	// 00001 enables pgvector: no page reads a vector yet.
	h.Exec(t, "SELECT '[1,2,3]'::vector")
}

// The app does not start on a database it can't reach: Docker Compose
// restarts it, and a deploy that never comes up healthy fails.
func TestStartupFailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(context.Background(), app.Config{
		DatabaseURL: "postgres://postgres:postgres@127.0.0.1:1/postgres?sslmode=disable",
		Clock:       clock.Real(time.UTC),
	}, log)
	if err == nil {
		a.Close()
		t.Fatal("app.New started on an unreachable database")
	}
}

func TestHealthReportsTheDatabase(t *testing.T) {
	h := apptest.Start(t)

	res := h.Get(t, "/healthz")
	if res.StatusCode != http.StatusOK || res.Body != `{"database":"ok"}`+"\n" {
		t.Errorf("GET /healthz = %d %q, want 200 and the database ok", res.StatusCode, res.Body)
	}
}
