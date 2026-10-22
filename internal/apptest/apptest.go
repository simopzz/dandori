// Package apptest starts MealOps in-process over HTTP against a real,
// freshly migrated Postgres + pgvector database of its own, for tests at
// the app seam.
//
// MEALOPS_TEST_DATABASE_URL must point at a disposable Postgres server with
// pgvector, as a superuser (`mise run testdb`). pgtestdb migrates a template
// database once per set of migrations and clones it for every test.
package apptest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver pgtestdb connects with
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/simopzz/mealops/internal/app"
	"github.com/simopzz/mealops/internal/db"
)

// Harness is one running app with its own database.
type Harness struct {
	URL    string
	Client *http.Client

	adminURL string
	database string
}

// Option customises the app a test starts.
type Option func(*settings)

type settings struct {
	ports       app.Ports
	databaseCut bool
}

// WithPorts lets a test swap outbound ports for fakes.
func WithPorts(configure func(*app.Ports)) Option {
	return func(s *settings) { configure(&s.ports) }
}

// WithDatabaseCut starts the app while its database is unreachable.
func WithDatabaseCut() Option {
	return func(s *settings) { s.databaseCut = true }
}

// Start boots the app against a new database cloned from a migrated
// template. Everything is torn down when the test ends; a failed test's
// database is kept for inspection.
func Start(t testing.TB, opts ...Option) *Harness {
	t.Helper()
	var s settings
	for _, opt := range opts {
		opt(&s)
	}

	adminURL := os.Getenv("MEALOPS_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("MEALOPS_TEST_DATABASE_URL is not set; run tests through mise and start the test database with `mise run testdb`")
	}
	admin, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse MEALOPS_TEST_DATABASE_URL: %v", err)
	}
	password, _ := admin.User.Password()
	port := admin.Port()
	if port == "" {
		port = "5432"
	}
	instance := pgtestdb.Custom(t, pgtestdb.Config{
		DriverName:                "pgx",
		Host:                      admin.Hostname(),
		Port:                      port,
		User:                      admin.User.Username(),
		Password:                  password,
		Database:                  strings.TrimPrefix(admin.Path, "/"),
		Options:                   admin.RawQuery, // keeps sslmode and friends from the URL
		ForceTerminateConnections: true,
		// Migrate and connect as the admin role, like the app's `postgres`
		// role on Supabase: enabling pgvector needs more than a plain role.
		TestRole: &pgtestdb.Role{Username: admin.User.Username(), Password: password},
	}, goosemigrator.New("migrations", goosemigrator.WithFS(db.Migrations())))

	h := &Harness{adminURL: adminURL, database: instance.Database}
	if s.databaseCut {
		h.CutDatabase(t)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	a, err := app.New(context.Background(), app.Config{DatabaseURL: instance.URL()}, s.ports, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.Close()
		a.Close()
	})
	h.URL, h.Client = srv.URL, srv.Client()
	return h
}

// CutDatabase makes the database unreachable, as when Supabase pauses the
// project: open connections drop and new ones are refused.
func (h *Harness) CutDatabase(t testing.TB) {
	t.Helper()
	h.setAllowConnections(t, false)
	h.admin(t, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", h.database)
}

// RestoreDatabase makes the database reachable again.
func (h *Harness) RestoreDatabase(t testing.TB) {
	t.Helper()
	h.setAllowConnections(t, true)
}

func (h *Harness) setAllowConnections(t testing.TB, allow bool) {
	t.Helper()
	h.admin(t, fmt.Sprintf("ALTER DATABASE %s ALLOW_CONNECTIONS %t", pgx.Identifier{h.database}.Sanitize(), allow))
}

// admin runs sql on the test database server as the superuser.
func (h *Harness) admin(t testing.TB, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, h.adminURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Response is a fully read HTTP response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       string
	// Method is the method of the request that got this response: GET when
	// a POST was redirected.
	Method string
}

// Get requests path from the app.
func (h *Harness) Get(t testing.TB, path string) Response {
	t.Helper()
	res, err := h.Client.Get(h.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("GET %s: read body: %v", path, err)
	}
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body)}
}

// PostForm submits form to path, as a browser does without HTMX, following
// any redirect.
func (h *Harness) PostForm(t testing.TB, path string, form url.Values) Response {
	t.Helper()
	return h.postForm(t, path, form, false)
}

// HTMXPostForm submits form to path as HTMX does, marked with HX-Request.
func (h *Harness) HTMXPostForm(t testing.TB, path string, form url.Values) Response {
	t.Helper()
	return h.postForm(t, path, form, true)
}

func (h *Harness) postForm(t testing.TB, path string, form url.Values, htmx bool) Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	res, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("POST %s: read body: %v", path, err)
	}
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body), Method: res.Request.Method}
}
