// Package apptest starts Dandori in-process over HTTP against a real,
// freshly migrated Postgres + pgvector database of its own, for tests at
// the app seam.
//
// DANDORI_TEST_DATABASE_URL must point at a disposable Postgres server with
// pgvector, as a superuser (`mise run testdb`). pgtestdb migrates a template
// database once per set of migrations and clones it for every test.
package apptest

import (
	"context"
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

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/db"
)

// Harness is one running app with its own database.
type Harness struct {
	URL    string
	Client *http.Client

	databaseURL string
}

// Option customises the app a test starts.
type Option func(*settings)

type settings struct {
	config     func(*app.Config)
	unmigrated bool
}

// WithConfig lets a test change the app's configuration, on top of any
// earlier change.
func WithConfig(configure func(*app.Config)) Option {
	return func(s *settings) {
		earlier := s.config
		s.config = func(c *app.Config) {
			if earlier != nil {
				earlier(c)
			}
			configure(c)
		}
	}
}

// WithUnmigratedDatabase starts the app on an empty database, as on its
// first deploy, so the app's own startup applies every migration.
func WithUnmigratedDatabase() Option {
	return func(s *settings) { s.unmigrated = true }
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

	adminURL := os.Getenv("DANDORI_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("DANDORI_TEST_DATABASE_URL is not set; run tests through mise and start the test database with `mise run testdb`")
	}
	admin, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse DANDORI_TEST_DATABASE_URL: %v", err)
	}
	password, _ := admin.User.Password()
	port := admin.Port()
	if port == "" {
		port = "5432"
	}
	var migrator pgtestdb.Migrator = goosemigrator.New("migrations", goosemigrator.WithFS(db.Migrations()))
	if s.unmigrated {
		migrator = pgtestdb.NoopMigrator{}
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
	}, migrator)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	cfg := app.Config{DatabaseURL: instance.URL()}
	if s.config != nil {
		s.config(&cfg)
	}
	a, err := app.New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.Close()
		a.Close()
	})
	return &Harness{URL: srv.URL, Client: srv.Client(), databaseURL: instance.URL()}
}

// Exec runs sql on the app's database, for fixtures of what the app can't
// create through its pages yet.
func (h *Harness) Exec(t testing.TB, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, h.databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
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
	// Method and Path are those of the request that got this response:
	// a GET of the new location when a POST was redirected.
	Method string
	Path   string
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
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body), Method: res.Request.Method, Path: res.Request.URL.Path}
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
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body), Method: res.Request.Method, Path: res.Request.URL.Path}
}
